package migrations

import _ "embed"

//go:embed 001_initial.sql
var Initial string

//go:embed 002_zones.sql
var Zones string

// SyntheticScenario adds the optional, paired scenario/evaluation metadata to
// the stored job. It is forward-only and guarded; historical jobs stay NULL.
//
//go:embed 003_synthetic_scenario.sql
var SyntheticScenario string
