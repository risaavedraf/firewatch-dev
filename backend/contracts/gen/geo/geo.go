// Package geo carries the versioned geospatial data asset shared by the
// FireWatch services.
//
// This package is DATA ONLY: contracts/gen deliberately carries no domain
// logic, and it exposes bytes rather than behaviour.
package geo

import _ "embed"

// ComunasGeoJSON is the versioned data asset of the official commune boundaries
// of the Valparaiso region, derived from the MOP/IDE Chile DPA layer
// (SERVICIO_DPA, capa Comunas) with outSR=4326.
//
// It is bytes, not a model: no CUT, name, province or geometry is parsed here.
// Each consumer computes its own in-memory projection of the catalogue from
// these bytes, so the asset stays the single shared source of truth without
// putting domain logic in contracts/gen.
//
//go:embed valparaiso-comunas.geojson
var ComunasGeoJSON []byte

// ComunasProvenanceJSON contains the unmodified provenance artifact.
//
//go:embed valparaiso-comunas.provenance.json
var ComunasProvenanceJSON []byte
