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
	"firewatch.local/gateway/internal/metrics"
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
			// jobs and risk re-stamp X-Request-ID on their own responses
			// (`uuid.Parse` the forwarded id, write back `id.String()`). ReverseProxy
			// copies response headers with Add, so that value would arrive next to
			// the one this gateway already set, and the browser would join both with
			// ", " so the page renders two ids in two formats. ModifyResponse runs
			// before the copy, so dropping the upstream value here leaves the
			// gateway's own id as the only value of the header.
			ModifyResponse: func(res *http.Response) error {
				res.Header.Del("X-Request-ID")
				return nil
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
	httpMetrics := &metrics.HTTPMetrics{}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		// Every response of this chain, including the error and health answers, is
		// timed and bucketed by class through one wrapper. The real writer stays
		// reachable through Unwrap, so the proxy's flush still works.
		recorder := &statusRecorder{ResponseWriter: w}
		w = recorder
		// S4: one correlation id, one format. This is the canonical dashed UUID
		// string the jobs and risk services emit: they parse the value forwarded
		// here and write back id.String(), and the JSON error body, the exposed
		// header and the log line below quote this same literal. The bytes are
		// formatted here because this module has no uuid dependency and this route
		// does not add one.
		b := randomID()
		id := fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
		r.Header.Set("X-Request-ID", id)
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; script-src 'self'; connect-src 'self' http://localhost:8081; img-src 'self'; object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		defer func() {
			duration := time.Since(start)
			slog.Info("request", "request_id", id, "method", r.Method, "duration_ms", duration.Milliseconds())
			// Observed after the response is written, so a scrape never renders its
			// own request inside the body it is producing; that sample lands in the
			// next scrape, exactly like every other request.
			status := recorder.status
			if status == 0 {
				status = http.StatusOK
			}
			httpMetrics.Observe(status, duration)
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
		// /metrics is answered here, next to the health checks and before any
		// quota or bearer check, for two reasons: a Prometheus scrape has no way to
		// obtain a token, and monitoring must not consume the budgets that bound
		// caller traffic. The body never includes the scrape that produced it.
		if r.URL.Path == "/metrics" {
			w.Header().Set("Content-Type", "text/plain; version=0.0.4")
			fmt.Fprint(w, httpMetrics.Text())
			fmt.Fprint(w, metrics.RuntimeText())
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
	// RFC 9562 version 4 and variant bits. The id this gateway mints is exposed to
	// the page and quoted in three logs, and jobs and risk parse it as a UUID and
	// write it back unchanged: without these two lines the string carries a UUID's
	// shape while its version nibble is random, which a strict consumer is free to
	// reject. The services' parse/print round trip preserves both bits, so every
	// consumer sees the same conformant value.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
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
