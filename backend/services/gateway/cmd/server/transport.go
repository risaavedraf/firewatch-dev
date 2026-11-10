package main

import (
	"fmt"
	"net/url"
	"os"
	"sort"
)

// requireSecureUpstreams refuses plaintext service-to-service hops outside
// development. The gateway is the one place that knows every upstream name.
func requireSecureUpstreams(environment string, upstreams map[string]string) error {
	// Same exception as the OIDC URL checks: the literal value "development" and
	// nothing else, so an unset or differently spelled APP_ENV fails closed.
	if environment == "development" {
		return nil
	}
	names := make([]string, 0, len(upstreams))
	for name := range upstreams {
		names = append(names, name)
	}
	// A map has no order, but the operator reads one message: sorting keeps the
	// named offender stable across replicas of the same misconfigured deployment.
	sort.Strings(names)
	for _, name := range names {
		raw := upstreams[name]
		// An empty upstream never reaches this guard in the running gateway: its own
		// configuration step rejects a missing URL first. Skipping it keeps that
		// message and this guard's scope unchanged.
		if raw == "" {
			continue
		}
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Scheme != "https" {
			return fmt.Errorf("%s must use https outside development", name)
		}
	}
	return nil
}

// startupTransport reads the upstream variables and refuses a plaintext hop
// before the proxy is built. It is the seam run() calls, so the refusal is
// testable without a token issuer, an upstream or a listener.
func startupTransport(environment string) error {
	return requireSecureUpstreams(environment, map[string]string{
		"JOBS_URL":       os.Getenv("JOBS_URL"),
		"RISK_URL":       os.Getenv("RISK_URL"),
		"PREVENTION_URL": os.Getenv("PREVENTION_URL"),
	})
}
