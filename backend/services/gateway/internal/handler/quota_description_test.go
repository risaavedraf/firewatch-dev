package handler

import (
	"os"
	"strings"
	"testing"
)

// The contract is read by consumers and by BH-08's parity tests, so a
// description that claims behavior the code does not have is a defect in the
// contract itself, independent of any response body. Both phrases below were
// published once and were false: the first described a single global limiter
// after the per-subject layer existed, and the second promised that an
// interrupted retry budget resumes after a consumer restart, which it does not.
func TestQuotaAndAttemptDescriptionsDoNotPromiseUnsupportedBehavior(t *testing.T) {
	body, err := os.ReadFile("../../../../contracts/openapi/firewatch-v1.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, unsupported := range []string{
		"single global limiter, not per caller",
		"so the budget survives a consumer restart",
	} {
		if strings.Contains(string(body), unsupported) {
			t.Errorf("OpenAPI contains unsupported description %q", unsupported)
		}
	}
}
