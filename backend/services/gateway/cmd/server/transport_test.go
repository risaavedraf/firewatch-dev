package main

import "testing"

// secureUpstreams is the configuration a production gateway must be given; every
// case below starts from it and changes only the value under test, so a failure
// names the upstream that made the hop plaintext.
func secureUpstreams() map[string]string {
	return map[string]string{
		"JOBS_URL":       "https://jobs:8080",
		"RISK_URL":       "https://risk:8080",
		"PREVENTION_URL": "https://prevention:8080",
	}
}

// plaintextUpstreams is the development configuration this guard has to keep
// accepting, because the local stack runs every service over plain HTTP.
func plaintextUpstreams() map[string]string {
	return map[string]string{
		"JOBS_URL":       "http://jobs:8080",
		"RISK_URL":       "http://risk:8090",
		"PREVENTION_URL": "http://prevention:8084",
	}
}

// withUpstream returns the secure set with one value replaced.
func withUpstream(name, value string) map[string]string {
	upstreams := secureUpstreams()
	upstreams[name] = value
	return upstreams
}

// assertGuardResult states the observable contract of the guard: either it
// accepts the configuration or it refuses it with exactly the message the
// operator will read. An empty want means the guard must accept.
func assertGuardResult(t *testing.T, err error, want string) {
	t.Helper()
	if want == "" {
		if err != nil {
			t.Fatalf("the guard refused a configuration it must accept: %v", err)
		}
		return
	}
	if err == nil {
		t.Fatalf("the guard accepted a configuration it must refuse, want %q", want)
	}
	if err.Error() != want {
		t.Fatalf("the guard refused with %q, want %q", err.Error(), want)
	}
}

// TestRequireSecureUpstreams pins the gateway rule: outside development every
// upstream it proxies to must be https, and the refusal names the variable the
// operator has to change. The only exception is the literal "development"; an
// unset or differently spelled environment fails closed.
func TestRequireSecureUpstreams(t *testing.T) {
	for _, tc := range []struct {
		name        string
		environment string
		upstreams   map[string]string
		want        string
	}{
		{name: "https outside development", environment: "production", upstreams: secureUpstreams()},
		{name: "plaintext jobs outside development", environment: "production", upstreams: withUpstream("JOBS_URL", "http://jobs:8080"), want: "JOBS_URL must use https outside development"},
		{name: "plaintext prevention outside development", environment: "production", upstreams: withUpstream("PREVENTION_URL", "http://prevention:8084"), want: "PREVENTION_URL must use https outside development"},
		{name: "every plaintext upstream names the same stable offender", environment: "production", upstreams: plaintextUpstreams(), want: "JOBS_URL must use https outside development"},
		{name: "unparsable upstream outside development", environment: "production", upstreams: withUpstream("RISK_URL", "https://risk:8080/%zz"), want: "RISK_URL must use https outside development"},
		{name: "plaintext in development", environment: "development", upstreams: plaintextUpstreams()},
		{name: "plaintext with an unset environment", environment: "", upstreams: plaintextUpstreams(), want: "JOBS_URL must use https outside development"},
		{name: "the development exception is not case-insensitive", environment: "Development", upstreams: plaintextUpstreams(), want: "JOBS_URL must use https outside development"},
		// An empty upstream never reaches this guard in the running gateway: its
		// own configuration step rejects a missing URL first. Skipping it keeps that
		// message and this guard's scope unchanged.
		{name: "empty upstreams are not refused here", environment: "production", upstreams: map[string]string{"JOBS_URL": "", "RISK_URL": "", "PREVENTION_URL": ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertGuardResult(t, requireSecureUpstreams(tc.environment, tc.upstreams), tc.want)
		})
	}
}

// TestStartupTransportRefusesPlaintextUpstreamsOutsideDevelopment pins the wiring:
// startup reads the upstream variables and returns the refusal, so the process
// stops before the proxy is built and before the listener is opened.
func TestStartupTransportRefusesPlaintextUpstreamsOutsideDevelopment(t *testing.T) {
	t.Setenv("JOBS_URL", "http://jobs:8080")
	t.Setenv("RISK_URL", "https://risk:8080")
	t.Setenv("PREVENTION_URL", "https://prevention:8080")
	assertGuardResult(t, startupTransport("production"), "JOBS_URL must use https outside development")
}
