// Package zones turns the versioned geospatial asset into rows in the zones
// table.
//
// The embedded asset is the single source of truth for the 38 communes: this
// package parses it and upserts it, so no CUT, name, province or geometry is
// duplicated in SQL. Loading is idempotent, and it runs in the migrate path so a
// fresh volume is seeded automatically rather than by hand.
package zones

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"firewatch.local/contracts/geo"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Zone is one row of the zones table as it arrives from the geographic asset.
//
// Geometry is raw GeoJSON and may be empty: a zone without geometry is a
// declared state, not a missing value to be invented.
type Zone struct {
	Code     string
	Name     string
	Province string
	Kind     string
	Geometry json.RawMessage
}

// featureCollection is the shape of the derived asset. Only the members the
// loader needs are declared; unknown members are ignored.
type featureCollection struct {
	Type     string    `json:"type"`
	Features []feature `json:"features"`
}

type feature struct {
	Properties struct {
		CutCom    string `json:"CUT_COM"`
		Comuna    string `json:"COMUNA"`
		Provincia string `json:"PROVINCIA"`
	} `json:"properties"`
	Geometry json.RawMessage `json:"geometry"`
}

// ParseComunas parses the derived FeatureCollection into zones.
//
// It refuses what cannot be loaded honestly: a document that is not a
// FeatureCollection, a feature without a CUT or a commune name, a repeated code,
// or an empty set. The expected number of communes is deliberately NOT pinned
// here -- it is pinned by a test -- so that a legitimate source change does not
// look like a defect in production code.
func ParseComunas(data []byte) ([]Zone, error) {
	var collection featureCollection
	if err := json.Unmarshal(data, &collection); err != nil {
		return nil, fmt.Errorf("parse comunas geojson: %w", err)
	}
	if collection.Type != "FeatureCollection" {
		return nil, fmt.Errorf("comunas geojson has type %q, want FeatureCollection", collection.Type)
	}
	if len(collection.Features) == 0 {
		return nil, fmt.Errorf("comunas geojson has no features")
	}
	seen := make(map[string]bool, len(collection.Features))
	parsed := make([]Zone, 0, len(collection.Features))
	for index, current := range collection.Features {
		if current.Properties.CutCom == "" {
			return nil, fmt.Errorf("feature %d has no CUT_COM", index)
		}
		if current.Properties.Comuna == "" {
			return nil, fmt.Errorf("feature %d (%s) has no COMUNA", index, current.Properties.CutCom)
		}
		if seen[current.Properties.CutCom] {
			return nil, fmt.Errorf("duplicate CUT_COM %q", current.Properties.CutCom)
		}
		seen[current.Properties.CutCom] = true
		parsed = append(parsed, Zone{
			Code:     current.Properties.CutCom,
			Name:     current.Properties.Comuna,
			Province: current.Properties.Provincia,
			Kind:     "comuna",
			Geometry: normalizeGeometry(current.Geometry),
		})
	}
	sort.Slice(parsed, func(i, j int) bool { return parsed[i].Code < parsed[j].Code })
	return parsed, nil
}

// normalizeGeometry maps the two empty forms -- absent and JSON null -- to a nil
// geometry, which Seed writes as NULL. Non-empty geometries are kept verbatim.
func normalizeGeometry(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return raw
}

// Seed upserts the zones in one transaction.
//
// It is the idempotent load step of the migrate path: running it again leaves the
// same rows. A zone whose geometry is empty is written with a NULL geometry,
// declared rather than invented. The legacy demo row is not touched because it is
// not in the asset and no seed targets it.
func Seed(ctx context.Context, db *pgxpool.Pool, zones []Zone) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("seed zones: nil database pool")
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("seed zones: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	const upsert = `INSERT INTO zones(code,name,province,kind,geometry)
VALUES($1,$2,$3,$4,ST_SetSRID(ST_GeomFromGeoJSON($5),4326))
ON CONFLICT (code) DO UPDATE SET name=EXCLUDED.name, province=EXCLUDED.province, geometry=EXCLUDED.geometry`
	for _, zone := range zones {
		if zone.Code == "" || zone.Name == "" {
			return 0, fmt.Errorf("seed zones: a zone has an empty code or name")
		}
		kind := zone.Kind
		if kind == "" {
			kind = "comuna"
		}
		// A typed *string keeps pgx from sending the raw bytes as bytea, which
		// ST_GeomFromGeoJSON cannot read, and keeps a NULL geometry as NULL.
		var geometry *string
		if len(zone.Geometry) > 0 {
			value := string(zone.Geometry)
			geometry = &value
		}
		if _, err = tx.Exec(ctx, upsert, zone.Code, zone.Name, zone.Province, kind, geometry); err != nil {
			return 0, fmt.Errorf("seed zones: upsert %s: %w", zone.Code, err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("seed zones: commit: %w", err)
	}
	return len(zones), nil
}

// SeedEmbedded parses the embedded asset and loads it into the zones table.
func SeedEmbedded(ctx context.Context, db *pgxpool.Pool) (int, error) {
	parsed, err := ParseComunas(geo.ComunasGeoJSON)
	if err != nil {
		return 0, err
	}
	return Seed(ctx, db, parsed)
}
