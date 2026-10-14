package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"firewatch.local/contracts/geo"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Zone is one row of the jobs-side zones catalogue.
//
// There is deliberately no geometry here: jobs only asks whether a code is a
// known zone, so its local catalogue is code/name/province/kind and nothing
// else. The geometry lives once, in the shared asset, and is materialised only
// by the service that performs spatial work.
type Zone struct {
	Code     string
	Name     string
	Province string
	Kind     string
}

// featureCollection is the shape of the shared asset. Only the members the
// projection needs are declared; the geometry member is intentionally ignored.
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
}

// ParseZones parses the shared geographic asset into the jobs catalogue.
//
// It extracts only the attributes the foreign key needs and ignores the
// geometry. It refuses what cannot be loaded honestly: a document that is not a
// FeatureCollection, a feature without a CUT_COM or a COMUNA, a repeated code,
// or an empty set. The expected number of communes is deliberately NOT pinned
// here -- a test pins it -- so that a legitimate source change does not look like
// a defect in production code.
func ParseZones(data []byte) ([]Zone, error) {
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
		})
	}
	sort.Slice(parsed, func(i, j int) bool { return parsed[i].Code < parsed[j].Code })
	return parsed, nil
}

// SeedZones upserts the catalogue in one transaction.
//
// It is the idempotent load step of the migrate path: running it again updates
// name and province in place and leaves the same rows. The legacy demo row is
// not touched because it is not in the asset and no seed targets it.
func SeedZones(ctx context.Context, db *pgxpool.Pool, zones []Zone) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("seed zones: nil database pool")
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("seed zones: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	const upsert = `INSERT INTO zones(code,name,province,kind)
VALUES($1,$2,$3,$4)
ON CONFLICT (code) DO UPDATE SET name=EXCLUDED.name, province=EXCLUDED.province`
	for _, zone := range zones {
		if zone.Code == "" || zone.Name == "" {
			return 0, fmt.Errorf("seed zones: a zone has an empty code or name")
		}
		kind := zone.Kind
		if kind == "" {
			kind = "comuna"
		}
		if _, err = tx.Exec(ctx, upsert, zone.Code, zone.Name, zone.Province, kind); err != nil {
			return 0, fmt.Errorf("seed zones: upsert %s: %w", zone.Code, err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("seed zones: commit: %w", err)
	}
	return len(zones), nil
}

// SeedZonesEmbedded parses the embedded shared asset and loads it into the local
// zones table.
func SeedZonesEmbedded(ctx context.Context, db *pgxpool.Pool) (int, error) {
	parsed, err := ParseZones(geo.ComunasGeoJSON)
	if err != nil {
		return 0, err
	}
	return SeedZones(ctx, db, parsed)
}

// ZoneExists reports whether the local catalogue holds code.
//
// Membership is decided by the data in zones, never by a literal in Go: the
// transport uses this instead of comparing against a hardcoded zone. A
// well-formed unknown code is simply absent, not an error; a driver failure is
// returned so the caller can distinguish "no such zone" from "cannot answer".
func (s Store) ZoneExists(ctx context.Context, code string) (bool, error) {
	var exists bool
	if err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM zones WHERE code=$1)`, code).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}
