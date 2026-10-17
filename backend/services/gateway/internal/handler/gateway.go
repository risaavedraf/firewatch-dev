package handler

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"firewatch.local/gateway/internal/limit"
	"github.com/coreos/go-oidc/v3/oidc"
)

// Quota policy agreed on 2026-10-16 (BH-03). The aggregate bound is the
// emergency per-replica ceiling that already existed; the subject and
// anonymous bounds are the fair quotas layered on top of it.
const (
	aggregateRate  = 20
	aggregateBurst = 40

	subjectRate  = 5
	subjectBurst = 10
	// subjectBuckets caps how many verified subjects are tracked at once and
	// subjectIdle evicts a subject's bucket after this long unused, so a stream
	// of distinct subjects cannot grow memory without bound.
	subjectBuckets = 4096
	subjectIdle    = 5 * time.Minute

	anonymousRate  = 1
	anonymousBurst = 5
)

type Verifier interface {
	Verify(context.Context, string) (*oidc.IDToken, error)
}

func New(verifier Verifier, jobs, risk *url.URL, webOrigin string) http.Handler {
	mux := http.NewServeMux()
	proxy := func(target *url.URL) http.Handler {
		return &httputil.ReverseProxy{
			Rewrite: func(r *httputil.ProxyRequest) {
				r.SetURL(target)
				// Never forward a caller-supplied identity or forwarding chain.
				r.Out.Header.Del("X-User-ID")
				r.Out.Header.Del("X-User-Role")
			},
			Transport:    &http.Transport{MaxIdleConns: 100, MaxIdleConnsPerHost: 20, IdleConnTimeout: 60 * time.Second, ResponseHeaderTimeout: 10 * time.Second},
			ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) { failure(w, 502, "upstream_unavailable") },
		}
	}
	mux.Handle("/v1/jobs", proxy(jobs))
	mux.Handle("/v1/jobs/", proxy(jobs))
	mux.Handle("/v1/audit", proxy(jobs))
	mux.Handle("/v1/risk/", proxy(risk))
	// Three quota layers, per the agreed BH-03 policy: an emergency aggregate
	// bound charged before authentication, a per-verified-subject bound, and one
	// deliberately shared anonymous bound. Identity is the verified token
	// subject only — no forwarded IP, no remote address, no trusted proxy.
	aggregate := limit.NewBucket(limit.Policy{Rate: aggregateRate, Burst: aggregateBurst}, limit.SystemClock)
	subject := limit.NewKeyed(limit.KeyedOptions{
		Policy: limit.Policy{Rate: subjectRate, Burst: subjectBurst},
		Max:    subjectBuckets,
		Idle:   subjectIdle,
	}, limit.SystemClock)
	anonymous := limit.NewBucket(limit.Policy{Rate: anonymousRate, Burst: anonymousBurst}, limit.SystemClock)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		id := fmt.Sprintf("%x", randomID())
		r.Header.Set("X-Request-ID", id)
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; script-src 'self'; connect-src 'self' http://localhost:8081; img-src 'self'; object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		defer func() {
			slog.Info("request", "request_id", id, "method", r.Method, "duration_ms", time.Since(start).Milliseconds())
		}()
		// CORS is fail-closed: an unset origin never matches, and the request
		// origin is compared exactly, so it is never echoed back unvalidated.
		crossOrigin := webOrigin != "" && r.Header.Get("Origin") == webOrigin
		if crossOrigin {
			w.Header().Set("Access-Control-Allow-Origin", webOrigin)
			// The page renders the correlation id of an error, and X-Request-ID is
			// not a CORS-safelisted response header: without this declaration the
			// browser hides it from page JavaScript and the id the gateway sent is
			// lost before the message is built. It is named explicitly rather than
			// wildcarded, like every other CORS value here.
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
			w.Header().Add("Vary", "Origin")
		}
		// A preflight is answered before the rate limiter and before the bearer
		// check, so it consumes no tokens and needs no Authorization header.
		if crossOrigin && r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			// An explicit allow-list, not a wildcard: the page sends
			// Idempotency-Key on the job POST and the jobs service rejects the
			// request without it, so the browser must be allowed to send it.
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key")
			w.Header().Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Path == "/health/live" || r.URL.Path == "/health/ready" {
			w.WriteHeader(204)
			return
		}
		// The aggregate bound is the only pre-authentication quota: it protects
		// this deployment without trusting any caller-supplied identity.
		if !aggregate.Allow() {
			rateLimited(w)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/v1/") {
			failure(w, 404, "not_found")
			return
		}
		auth := strings.Fields(r.Header.Get("Authorization"))
		if len(auth) != 2 || !strings.EqualFold(auth[0], "Bearer") {
			rejectUnauthenticated(w, anonymous)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		token, err := verifier.Verify(ctx, auth[1])
		cancel()
		if err != nil || token.Subject == "" {
			rejectUnauthenticated(w, anonymous)
			return
		}
		// The subject only exists after Verify, so this quota can only be
		// charged here, after authentication.
		if !subject.Allow(token.Subject) {
			rateLimited(w)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		mux.ServeHTTP(w, r)
	})
}

func randomID() []byte {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}
func failure(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "request_id": w.Header().Get("X-Request-ID")})
}

// rateLimited answers 429 with the Retry-After header and rate_limited body
// that BH-08 pins against the OpenAPI contract.
func rateLimited(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "1")
	failure(w, 429, "rate_limited")
}

// rejectUnauthenticated charges the single shared anonymous bucket for an
// unauthenticated request; once that coarse bucket is exhausted the noise it
// bounds is answered 429 instead of another 401.
func rejectUnauthenticated(w http.ResponseWriter, anonymous *limit.Bucket) {
	if !anonymous.Allow() {
		rateLimited(w)
		return
	}
	failure(w, 401, "unauthorized")
}
