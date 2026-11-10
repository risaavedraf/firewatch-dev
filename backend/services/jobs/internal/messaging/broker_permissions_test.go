package messaging

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	v1 "firewatch.local/contracts/events/v1"
)

// The tests in this file exercise the broker's own permission enforcement, so
// they need a live broker with the BH-04 identities provisioned. They are opt-in
// like the migration tests: without the two URLs they skip, and the default
// suite stays offline. Run them with the identities' own AMQP URLs; the values
// carry passwords and are never echoed.
//
//	BH04_JOBS_URL=... BH04_RISK_URL=... go test ./internal/messaging -run TestBroker

// bh04URL reads an identity URL and never prints it: it carries a password.
func bh04URL(t *testing.T, key string) string {
	t.Helper()
	url := os.Getenv(key)
	if url == "" {
		t.Skipf("%s not set: broker permission enforcement is not exercised", key)
	}
	return url
}

// peerChannel opens a channel on the given identity. Every caller closes it.
func peerChannel(t *testing.T, url string) (*amqp.Connection, *amqp.Channel) {
	t.Helper()
	conn, err := amqp.Dial(url)
	if err != nil {
		t.Fatalf("dial %s: %v", identityName(url), err)
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		t.Fatalf("channel %s: %v", identityName(url), err)
	}
	t.Cleanup(func() {
		ch.Close()
		conn.Close()
	})
	return conn, ch
}

// identityName returns the user of an AMQP URL for error messages, never the
// password.
func identityName(url string) string {
	rest := strings.TrimPrefix(url, "amqp://")
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		rest = rest[:at]
	}
	if colon := strings.Index(rest, ":"); colon >= 0 {
		rest = rest[:colon]
	}
	return rest
}

// refusal performs attempt and waits for the broker to close the channel, which
// is how AMQP reports a permission denial. A quiet timeout is a failure: the
// operation was supposed to be refused and was not.
func refusal(t *testing.T, ch *amqp.Channel, attempt func() error) *amqp.Error {
	t.Helper()
	closed := ch.NotifyClose(make(chan *amqp.Error, 1))
	if err := attempt(); err != nil {
		// A synchronous failure is also a refusal; report it through the same
		// type so callers assert one shape.
		return toAMQPError(err)
	}
	select {
	case err := <-closed:
		if err == nil {
			t.Fatal("channel closed without an error")
		}
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("the broker did not refuse the operation")
		return nil
	}
}

func toAMQPError(err error) *amqp.Error {
	return &amqp.Error{Code: 0, Reason: err.Error()}
}

func assertRefused(t *testing.T, err *amqp.Error, resource, access string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s was not refused", resource)
	}
	reason := err.Reason
	if !strings.Contains(reason, "ACCESS_REFUSED") {
		t.Fatalf("refusal reason=%q want ACCESS_REFUSED", reason)
	}
	if resource != "" && !strings.Contains(reason, resource) {
		t.Fatalf("refusal reason=%q must name %q", reason, resource)
	}
	if access != "" && !strings.Contains(reason, access) {
		t.Fatalf("refusal reason=%q must name %q access", reason, access)
	}
}

// TestBrokerRefusesPeerExchangePublish proves the isolation the split exchanges
// exist for: risk holds write on its own exchange only, so publishing the jobs
// contract into the jobs exchange is denied by the broker, not by convention.
func TestBrokerRefusesPeerExchangePublish(t *testing.T) {
	_, ch := peerChannel(t, bh04URL(t, "BH04_RISK_URL"))

	err := refusal(t, ch, func() error {
		return ch.PublishWithContext(context.Background(), v1.JobsExchange,
			v1.JobRequestedType, false, false, amqp.Publishing{
				ContentType: "application/json",
				Body:        []byte(`{"probe":"deny-peer-exchange"}`),
			})
	})
	assertRefused(t, err, v1.JobsExchange, "write")
}

// TestBrokerRefusesDeadExchangePublish protects the decision that no service may
// write the shared dead exchange: a forged dead letter would land directly in
// its peer's dead-letter queue, skipping the dead-letter path entirely.
func TestBrokerRefusesDeadExchangePublish(t *testing.T) {
	_, ch := peerChannel(t, bh04URL(t, "BH04_JOBS_URL"))

	err := refusal(t, ch, func() error {
		return ch.PublishWithContext(context.Background(), v1.DeadExchange,
			v1.RiskQueue, false, false, amqp.Publishing{
				ContentType: "application/json",
				Body:        []byte(`{"probe":"deny-dead-exchange"}`),
			})
	})
	assertRefused(t, err, v1.DeadExchange, "write")
}

// TestBrokerRefusesPeerQueueConsume proves consumption is scoped too: jobs may
// read its own results queue and not the risk work queue, so one compromised
// service cannot take over its peer's work.
func TestBrokerRefusesPeerQueueConsume(t *testing.T) {
	_, ch := peerChannel(t, bh04URL(t, "BH04_JOBS_URL"))

	err := refusal(t, ch, func() error {
		_, consumeErr := ch.Consume(v1.RiskQueue, "", false, false, false, false, nil)
		return consumeErr
	})
	assertRefused(t, err, v1.RiskQueue, "read")
}

// TestBrokerAllowsOwnExchangePublish is the positive control: without it, the
// denials above could be explained by the identity being broken rather than
// being narrow. The routing key matches no binding, so nothing is enqueued and
// no running consumer sees a probe message; the publisher confirm proves the
// broker accepted the write.
func TestBrokerAllowsOwnExchangePublish(t *testing.T) {
	checkOwners := []struct {
		env      string
		exchange string
		keys     []string
	}{
		{"BH04_JOBS_URL", v1.JobsExchange, []string{v1.JobRequestedType}},
		{"BH04_RISK_URL", v1.RiskExchange, []string{v1.BatchPublishedType, v1.JobFailedType}},
	}
	for _, owner := range checkOwners {
		t.Run(owner.exchange, func(t *testing.T) {
			_, ch := peerChannel(t, bh04URL(t, owner.env))

			// Deferred confirmations are returned only on a channel placed in
			// confirm mode; without this the confirmation is nil and the ack the
			// control asserts is unobservable.
			if err := ch.Confirm(false); err != nil {
				t.Fatalf("confirm mode on %s: %v", owner.exchange, err)
			}

			for _, key := range owner.keys {
				// Unroutable on purpose: the key is prefixed so no binding
				// matches, which keeps the live queues untouched.
				confirm, err := ch.PublishWithDeferredConfirmWithContext(context.Background(),
					owner.exchange, "bh04.probe."+key, false, false, amqp.Publishing{
						ContentType: "application/json",
						Body:        []byte(`{"probe":"allow-own-exchange"}`),
					})
				if err != nil {
					t.Fatalf("publish to own exchange %s: %v", owner.exchange, err)
				}
				if confirm == nil {
					t.Fatalf("no confirmation for %s: the channel is not in confirm mode", owner.exchange)
				}
				acked, err := confirm.WaitContext(context.Background())
				if err != nil {
					t.Fatalf("confirm on %s: %v", owner.exchange, err)
				}
				if !acked {
					t.Fatalf("broker did not confirm the write to %s", owner.exchange)
				}
			}
		})
	}
}
