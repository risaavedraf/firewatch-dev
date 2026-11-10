package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"firewatch.local/gateway/internal/handler"
	"github.com/coreos/go-oidc/v3/oidc"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("server_stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	required := func(key string) string {
		value := os.Getenv(key)
		if value == "" {
			panic("missing configuration: " + key)
		}
		return value
	}
	parse := func(key string) *url.URL {
		u, err := url.Parse(required(key))
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			panic("invalid URL: " + key)
		}
		return u
	}
	issuer, jwks := parse("OIDC_ISSUER"), parse("OIDC_JWKS_URL")
	if os.Getenv("APP_ENV") != "development" && (issuer.Scheme != "https" || jwks.Scheme != "https") {
		return errors.New("OIDC requires HTTPS outside development")
	}
	// The internal CA is loaded before any outbound client is built, so the key-set
	// fetch and the proxy share one pool and a bundle that cannot be loaded stops
	// startup instead of leaving a client with roots it was not configured with.
	rootCAs, err := startupRootCAs()
	if err != nil {
		return err
	}
	keyCtx := oidc.ClientContext(ctx, newJWKSClient(rootCAs))
	verifier := oidc.NewVerifier(issuer.String(), oidc.NewRemoteKeySet(keyCtx, jwks.String()), &oidc.Config{ClientID: required("OIDC_AUDIENCE"), SupportedSigningAlgs: []string{"RS256"}})
	// WEB_ORIGIN is optional on purpose: an unset value disables CORS entirely.
	webOrigin := os.Getenv("WEB_ORIGIN")
	// The upstreams are parsed before the guard, so a missing or malformed address
	// keeps the startup refusal it has today. The guard then decides whether the
	// addresses it just accepted are encrypted.
	jobsURL, riskURL, preventionURL := parse("JOBS_URL"), parse("RISK_URL"), parse("PREVENTION_URL")
	if err := startupTransport(os.Getenv("APP_ENV")); err != nil {
		return err
	}
	// The listener's transport is decided before the server is built, so a
	// deployment with no key pair stops here instead of binding a plaintext port
	// nobody notices.
	certFile, keyFile, err := startupListenerTLS(os.Getenv("APP_ENV"))
	if err != nil {
		return err
	}
	srv := &http.Server{Addr: ":8080", Handler: handler.NewWithRootCAs(verifier, jobsURL, riskURL, preventionURL, webOrigin, rootCAs), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	err = listenAndServe(srv, certFile, keyFile)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
