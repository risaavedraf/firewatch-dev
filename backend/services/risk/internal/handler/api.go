package handler

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"

	observations "firewatch.local/contracts/observations/v1"
	"firewatch.local/risk/internal/repository"
	"firewatch.local/risk/internal/source"
)

type Repository interface {
	List(context.Context, string, int) ([]repository.Batch, error)
	// ZoneExists reports whether the zone catalogue holds code. Membership is a
	// property of the data, not of a literal compiled into this service, so the
	// read path asks the catalogue instead of comparing to a hardcoded zone.
	ZoneExists(ctx context.Context, code string) (bool, error)
	// ListZones reads the catalogue. A nil bbox asks for every zone that has a
	// geometry; a non-nil bbox asks for the ones intersecting it. An empty
	// result is a valid answer, not an error.
	ListZones(ctx context.Context, bbox *repository.BBox) ([]repository.Zone, error)
	// ListModels reads the model registry for the version panel.
	ListModels(ctx context.Context) ([]repository.ModelVersion, error)
	// ActivateModel makes one version active and records its approval atomically.
	ActivateModel(ctx context.Context, version string) (repository.ModelVersion, error)
}

// DescriptorProvider is the read-only description surface of the active publisher provider.
type DescriptorProvider interface {
	Describe() observations.SourceDescriptor
}

func New(store Repository, verifier Verifier, providers ...DescriptorProvider) http.Handler {
	return NewWithCycle(store, verifier, nil, nil, providers...)
}

// NewWithStatus preserves the descriptor-only New API and its provider identity.
func NewWithStatus(store Repository, verifier Verifier, check StatusCheck, providers ...DescriptorProvider) http.Handler {
	return NewWithCycle(store, verifier, check, nil, providers...)
}

// NewWithCycle is the full constructor: it adds the 15 minute cycle to the read
// routes. A nil runner keeps every existing route working and makes the manual
// trigger answer cycle_unavailable, so a deployment that does not schedule a
// cycle is still valid. The sources route (sources.go) carries the freshness.
func NewWithCycle(store Repository, verifier Verifier, check StatusCheck, runner CycleRunner, providers ...DescriptorProvider) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/risk/status", statusHandler(check))
	get := func(latest bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			zone := r.URL.Query().Get("zone_id")
			// Exactly one zone_id parameter is required. A missing, repeated or
			// empty parameter is a malformed request, not an unknown zone.
			if zone == "" || len(r.URL.Query()) != 1 || len(r.URL.Query()["zone_id"]) != 1 {
				fail(w, 400, "invalid_zone")
				return
			}
			exists, err := store.ZoneExists(r.Context(), zone)
			if err != nil {
				fail(w, 503, "storage_unavailable")
				return
			}
			if !exists {
				fail(w, 400, "invalid_zone")
				return
			}
			limit := 100
			if latest {
				limit = 1
			}
			batches, err := store.List(r.Context(), zone, limit)
			if err != nil {
				fail(w, 503, "storage_unavailable")
				return
			}
			if latest {
				if len(batches) == 0 {
					respond(w, 404, map[string]string{"code": "no_published_batch", "data_status": "insufficient"})
					return
				}
				respond(w, 200, batches[0])
				return
			}
			respond(w, 200, batches)
		}
	}
	mux.HandleFunc("GET /v1/risk/latest", get(true))
	mux.HandleFunc("GET /v1/risk/history", get(false))
	mux.HandleFunc("GET /v1/risk/zones", listZones(store))
	mux.HandleFunc("GET /v1/risk/synthetic-scenarios", listSyntheticScenarios())
	// Model registry: the list is a read route; activation is admin-only through
	// requireAdmin, defined in security.go.
	mux.HandleFunc("GET /v1/models", listModels(store))
	mux.HandleFunc("POST /v1/models/{version}/activate", requireAdmin(activateModel(store)))
	mux.HandleFunc("POST /v1/risk/cycle/run", requireAdmin(cycleRun(runner)))
	var provider DescriptorProvider
	if len(providers) == 1 {
		provider = providers[0]
	}
	mux.HandleFunc("GET /v1/risk/sources", sourcesWithCycle(provider, runner))
	return Authorize(verifier, mux)
}

// zoneFeatureCollection is an RFC 7946 FeatureCollection of catalogue zones.
type zoneFeatureCollection struct {
	Type     string        `json:"type"`
	Features []zoneFeature `json:"features"`
}

// zoneFeature is one RFC 7946 Feature. Geometry is raw JSON so it is embedded
// exactly as ST_AsGeoJSON produced it: re-encoding or string-escaping it would
// turn a geometry object into a string, which RFC 7946 does not allow.
type zoneFeature struct {
	Type       string             `json:"type"`
	Properties zoneFeatureDetails `json:"properties"`
	Geometry   json.RawMessage    `json:"geometry"`
}

type zoneFeatureDetails struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Province string `json:"province"`
	Kind     string `json:"kind"`
}

// newZoneCollection maps catalogue rows to a FeatureCollection. The slice is
// built with an explicit capacity and never nil, so an empty catalogue serialises
// as an empty array rather than JSON null: an empty result is a valid, honest
// answer and not an error.
func newZoneCollection(zones []repository.Zone) zoneFeatureCollection {
	features := make([]zoneFeature, 0, len(zones))
	for _, zone := range zones {
		features = append(features, zoneFeature{
			Type: "Feature",
			Properties: zoneFeatureDetails{
				Code:     zone.Code,
				Name:     zone.Name,
				Province: zone.Province,
				Kind:     zone.Kind,
			},
			Geometry: zone.Geometry,
		})
	}
	return zoneFeatureCollection{Type: "FeatureCollection", Features: features}
}

// listZones serves the catalogue. bbox is the only accepted parameter: an
// unknown parameter or a repeated bbox is malformed input, not an unknown
// zone, so both are rejected with the same code as an unparseable bbox.
func listZones(store Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		for key := range query {
			if key != "bbox" {
				fail(w, 400, "invalid_bbox")
				return
			}
		}
		values := query["bbox"]
		if len(values) > 1 {
			fail(w, 400, "invalid_bbox")
			return
		}
		var bbox *repository.BBox
		if len(values) == 1 {
			parsed, err := parseBBox(values[0])
			if err != nil {
				fail(w, 400, "invalid_bbox")
				return
			}
			bbox = parsed
		}
		zones, err := store.ListZones(r.Context(), bbox)
		if err != nil {
			fail(w, 503, "storage_unavailable")
			return
		}
		respond(w, 200, newZoneCollection(zones))
	}
}

// listSyntheticScenarios serves the built-in demonstration catalogue. The list
// comes from the provider package, so the API, the generator and the tests read
// one vocabulary instead of the IDs being duplicated in a UI-facing constant. It
// reads no database and no broker: the profiles are compiled in, and the route
// must stay answerable when storage is down. Authorization is the same
// viewer/admin read gate as every other risk read route.
func listSyntheticScenarios() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, source.SyntheticScenarios())
	}
}

// parseBBox parses the optional bbox query value, exactly four comma-separated
// WGS84 numbers minLon,minLat,maxLon,maxLat.
//
// It is pure so the whole validation table is testable without a server. It
// rejects what cannot describe a box: a wrong part count, a non-numeric part, a
// non-finite part (NaN or an infinity), an out-of-range longitude or latitude,
// and a minimum that is not strictly below its maximum. Rejecting rather than
// clamping matters because a clamped box would silently answer a question the
// caller did not ask.
func parseBBox(raw string) (*repository.BBox, error) {
	parts := strings.Split(raw, ",")
	if len(parts) != 4 {
		return nil, errors.New("bbox needs four comma-separated numbers")
	}
	values := make([]float64, len(parts))
	for index, part := range parts {
		value, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return nil, errors.New("bbox part is not a number")
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, errors.New("bbox part is not finite")
		}
		values[index] = value
	}
	minLon, minLat, maxLon, maxLat := values[0], values[1], values[2], values[3]
	if minLon < -180 || minLon > 180 || maxLon < -180 || maxLon > 180 {
		return nil, errors.New("bbox longitude out of range")
	}
	if minLat < -90 || minLat > 90 || maxLat < -90 || maxLat > 90 {
		return nil, errors.New("bbox latitude out of range")
	}
	if minLon >= maxLon || minLat >= maxLat {
		return nil, errors.New("bbox minimum is not below its maximum")
	}
	return &repository.BBox{MinLon: minLon, MinLat: minLat, MaxLon: maxLon, MaxLat: maxLat}, nil
}
