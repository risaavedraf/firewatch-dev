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

// JobFailureState persists the consumer's attempt budget and the terminal
// outcome on the job row: attempts, failure_reason and failed_at, plus the
// 'failed' state value. It is forward-only and guarded; historical jobs keep
// attempts 0 and NULL failure fields.
//
//go:embed 004_job_failure_state.sql
var JobFailureState string
