package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	observations "firewatch.local/contracts/observations/v1"
)

// HTTPProviderID is the stable identifier recorded for the configurable remote
// source. It is a constant rather than derived from the URL so the traceability
// table and the freshness panel keep naming one source when the deployment
// changes endpoint.
const HTTPProviderID = "http-json-source"

// maxPayloadBytes bounds one remote response. A source that streams an unbounded
// body into memory is a denial-of-service surface, so the read is truncated and
// an over-long body is refused instead of being parsed in part.
const maxPayloadBytes = 5 << 20

// HTTPProvider fetches observations from a configured HTTP/JSON endpoint and
// normalizes them into the canonical ObservationSet.
//
// It is the seam that lets a real source replace the fixture without touching
// the classifier, the publisher or the API: everything downstream reads the
// canonical shape, and the transport details (status code, payload hash, query
// fingerprint) travel with the provenance.
type HTTPProvider struct {
	endpoint string
	client   *http.Client
	now      func() time.Time
}

// HTTPProviderOptions carries the injectable collaborators. Client and Now are
// optional: production uses an HTTP client with a timeout and the wall clock, and
// a test pins both so a fetch is reproducible without sleeping or reaching the
// network.
type HTTPProviderOptions struct {
	Client *http.Client
	Now    func() time.Time
}

// NewHTTPProvider validates the endpoint and returns the remote provider. A URL
// without a scheme or host is refused at construction, so a misconfigured
// environment fails at startup instead of on the first cycle.
func NewHTTPProvider(rawURL string, options HTTPProviderOptions) (HTTPProvider, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return HTTPProvider{}, fmt.Errorf("invalid source url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return HTTPProvider{}, fmt.Errorf("invalid source url %q: scheme must be http or https", rawURL)
	}
	if parsed.Host == "" {
		return HTTPProvider{}, fmt.Errorf("invalid source url %q: host is empty", rawURL)
	}
	client := options.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return HTTPProvider{endpoint: parsed.String(), client: client, now: now}, nil
}

// ProviderFromEnv selects the configured provider.
//
// An empty value, or the literal "fixture", keeps the deterministic built-in
// sample and performs no network call, so every suite and a fresh deployment
// behave identically without configuration. Anything else is the HTTP/JSON
// endpoint named by FIREWATCH_SOURCE_URL.
func ProviderFromEnv(rawURL string) (Provider, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" || strings.EqualFold(trimmed, "fixture") {
		return NewFixtureProvider(), nil
	}
	return NewHTTPProvider(trimmed, HTTPProviderOptions{})
}

// Describe implements Provider. The descriptor is deliberately honest about the
// transport: the licence and attribution name the configured provider as the
// owner, and the content is not marked synthetic, because a configured endpoint
// is expected to serve real observations. Registering it in a Registry is what
// validates the descriptor.
func (p HTTPProvider) Describe() observations.SourceDescriptor {
	return observations.SourceDescriptor{
		ID:               HTTPProviderID,
		DisplayName:      "Fuente HTTP/JSON configurada",
		Kind:             observations.SourceKindHTTPAPI,
		Origin:           observations.OriginPublicOfficial,
		Endpoint:         p.endpoint,
		License:          "Según los términos de la fuente configurada en FIREWATCH_SOURCE_URL",
		Attribution:      "Fuente externa configurada; la atribución exacta depende del proveedor publicado en el endpoint.",
		Variables:        observations.VariableNames(),
		GeoCoverage:      "Cobertura declarada por la fuente configurada",
		TemporalCoverage: "Ventana solicitada por el ciclo",
		Cadence:          "cada 15 minutos",
		Purpose:          "Ingesta de observaciones reales para la clasificación de riesgo",
		Priority:         10,
		Synthetic:        false,
	}
}

// httpPayload is the wire shape a configured source serves. Only the members
// this provider needs are declared; unknown members are ignored so a source may
// carry extra metadata without breaking normalization.
type httpPayload struct {
	Records     []httpRecord `json:"records"`
	RetrievedAt *time.Time   `json:"retrieved_at,omitempty"`
	CutoffAt    *time.Time   `json:"cutoff_at,omitempty"`
	Complete    *bool        `json:"complete,omitempty"`
}

// httpRecord is one raw observation. Quality is carried as text and normalized;
// an absent value becomes the canonical "unknown" rather than a guessed "ok".
type httpRecord struct {
	SourceID   string    `json:"source_id"`
	Variable   string    `json:"variable"`
	ZoneID     string    `json:"zone_id"`
	ObservedAt time.Time `json:"observed_at"`
	Value      float64   `json:"value"`
	Unit       string    `json:"unit"`
	Quality    string    `json:"quality"`
}

// Fetch implements Provider. It issues one bounded GET, hashes the exact bytes
// the server returned and normalizes them into canonical records. A transport
// failure, a non-2xx status or an unparseable body is an error, never an empty
// success: an empty set and a failed retrieval mean different things.
func (p HTTPProvider) Fetch(ctx context.Context, window observations.Window) (observations.ObservationSet, error) {
	if err := ctx.Err(); err != nil {
		return observations.ObservationSet{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint, nil)
	if err != nil {
		return observations.ObservationSet{}, err
	}
	query := request.URL.Query()
	if !window.From.IsZero() {
		query.Set("from", window.From.UTC().Format(time.RFC3339))
	}
	if !window.To.IsZero() {
		query.Set("to", window.To.UTC().Format(time.RFC3339))
	}
	request.URL.RawQuery = query.Encode()
	response, err := p.client.Do(request)
	if err != nil {
		return observations.ObservationSet{}, fmt.Errorf("fetch %s: %w", HTTPProviderID, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxPayloadBytes+1))
	if err != nil {
		return observations.ObservationSet{}, fmt.Errorf("read %s response: %w", HTTPProviderID, err)
	}
	if len(body) > maxPayloadBytes {
		return observations.ObservationSet{}, fmt.Errorf("%s response exceeds %d bytes", HTTPProviderID, maxPayloadBytes)
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return observations.ObservationSet{}, fmt.Errorf("%s returned HTTP %d", HTTPProviderID, response.StatusCode)
	}
	var payload httpPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return observations.ObservationSet{}, fmt.Errorf("parse %s payload: %w", HTTPProviderID, err)
	}
	records := make([]observations.Observation, 0, len(payload.Records))
	for _, raw := range payload.Records {
		records = append(records, observations.Observation{
			SourceID:   raw.SourceID,
			Variable:   raw.Variable,
			ZoneID:     raw.ZoneID,
			ObservedAt: raw.ObservedAt.UTC(),
			Value:      raw.Value,
			Unit:       raw.Unit,
			Quality:    normalizeQuality(raw.Quality),
		})
	}
	digest, err := RecordsHash(records)
	if err != nil {
		return observations.ObservationSet{}, err
	}
	payloadSum := sha256.Sum256(body)
	retrievedAt := p.now().UTC()
	if payload.RetrievedAt != nil {
		retrievedAt = payload.RetrievedAt.UTC()
	}
	cutoffAt := retrievedAt
	if payload.CutoffAt != nil {
		cutoffAt = payload.CutoffAt.UTC()
	}
	descriptor := p.Describe()
	provenance := ProvenanceFrom(descriptor)
	provenance.QueryFingerprint = request.URL.String()
	provenance.RetrievedAt = retrievedAt
	provenance.CutoffAt = cutoffAt
	provenance.Format = "http_json"
	provenance.Records = len(records)
	provenance.PayloadSHA256 = hex.EncodeToString(payloadSum[:])
	provenance.NormalizedSHA256 = digest
	provenance.HTTPStatus = response.StatusCode
	provenance.Complete = payload.Complete == nil || *payload.Complete
	return observations.ObservationSet{Window: window, Records: records, Provenance: provenance}, nil
}

// normalizeQuality maps the served quality text onto the canonical vocabulary.
// Anything unrecognized, including an absent value, becomes "unknown": guessing
// "ok" for a source that did not state its quality would overstate trust.
func normalizeQuality(raw string) observations.Quality {
	switch observations.Quality(strings.TrimSpace(raw)) {
	case observations.QualityOK:
		return observations.QualityOK
	case observations.QualityPartial:
		return observations.QualityPartial
	case observations.QualityEstimated:
		return observations.QualityEstimated
	default:
		return observations.QualityUnknown
	}
}
