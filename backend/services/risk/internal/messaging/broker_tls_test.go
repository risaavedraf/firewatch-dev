package messaging

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// internalBrokerURL is the URL shape the deployment dials once the broker listens
// with TLS: the host in it is what the certificate must match.
const internalBrokerURL = "amqps://user:pass@broker:5671/firewatch"

// testCAFile writes a throwaway CA certificate into the test's own temp directory
// and returns its path, so these tests load their trust anchors from a file the
// same way startup does. Nothing is shared with the repository or the deployment:
// the real CA is issued by bootstrap-secrets.sh.
func testCAFile(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ca key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "firewatch-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("ca certificate: %v", err)
	}
	path := filepath.Join(t.TempDir(), "ca.crt")
	if err = os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("write ca: %v", err)
	}
	return path
}

// testCAPool loads the CA bundle the test wrote, so the pool under assertion is
// built from a PEM file rather than handed in as an opaque value.
func testCAPool(t *testing.T) *x509.CertPool {
	t.Helper()
	bundle, err := os.ReadFile(testCAFile(t))
	if err != nil {
		t.Fatalf("read ca: %v", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(bundle) {
		t.Fatal("the test CA is not a usable root")
	}
	return pool
}

// TestBrokerDialConfigTrustsTheInternalCA pins T7.3's outbound AMQP half: a
// configured CA is the root pool the handshake verifies with, the certificate is
// checked against the host the URL names instead of a literal, and nothing else
// about the configuration moves - the heartbeat and the five-second dial stay.
func TestBrokerDialConfigTrustsTheInternalCA(t *testing.T) {
	pool := testCAPool(t)
	config := (&Broker{URL: internalBrokerURL, RootCAs: pool}).dialConfig()
	if config.TLSClientConfig == nil || config.TLSClientConfig.RootCAs != pool {
		t.Fatalf("TLSClientConfig = %#v, want the configured pool", config.TLSClientConfig)
	}
	if config.TLSClientConfig.ServerName != "broker" {
		t.Fatalf("ServerName = %q, want the host of the dialed URL", config.TLSClientConfig.ServerName)
	}
	if config.Heartbeat != 10*time.Second {
		t.Fatalf("heartbeat = %v, want the 10s it always had", config.Heartbeat)
	}
	want := amqp.Config{Dial: amqp.DefaultDial(5 * time.Second)}
	if config.Dial == nil || reflect.ValueOf(config.Dial).Pointer() != reflect.ValueOf(want.Dial).Pointer() {
		t.Fatal("the broker stopped dialing with the library's default dial")
	}
	// The CA branch must not reshape the rest of the configuration: with the TLS
	// client config removed, what remains is exactly the configuration this broker
	// dialed before TLS_CA_FILE existed.
	bare := (&Broker{URL: internalBrokerURL}).dialConfig()
	trimmed := config
	trimmed.TLSClientConfig = nil
	trimmed.Dial, bare.Dial = nil, nil
	if !reflect.DeepEqual(trimmed, bare) {
		t.Fatalf("the CA branch changed more than TLSClientConfig: with=%+v without=%+v", trimmed, bare)
	}
}

// TestBrokerDialConfigWithoutTheInternalCAIsUnchanged pins the other half: with no
// CA, TLSClientConfig stays nil, so the library keeps deriving the whole TLS
// configuration from the URL exactly as it did before this unit.
func TestBrokerDialConfigWithoutTheInternalCAIsUnchanged(t *testing.T) {
	config := (&Broker{URL: internalBrokerURL}).dialConfig()
	if config.TLSClientConfig != nil {
		t.Fatalf("TLSClientConfig = %#v, want nil so the library derives it from the URL", config.TLSClientConfig)
	}
	if config.Heartbeat != 10*time.Second || config.Dial == nil {
		t.Fatalf("heartbeat=%v dial==nil=%v, want the heartbeat and default dial this broker always used", config.Heartbeat, config.Dial == nil)
	}
}
