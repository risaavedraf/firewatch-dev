# inference

The production risk model, executed by Go over a versioned JSON artifact.

This is the fifth service beside `gateway`, `jobs`, `risk` and `prevention`. It is
not published through the gateway: `risk` calls it on the internal network, so
there is no bearer token, no CORS and no quota here — the network is the boundary
that keeps arbitrary callers off the model (S3). There is no `onnxruntime` and no
cgo: the artifact is data, Go interprets it, and the image stays
`CGO_ENABLED=0` on `gcr.io/distroless/static-debian12:nonroot` like the other four
(S2).

## Endpoints

| Route | What it answers |
| --- | --- |
| `POST /predict` | One scored sample: `{"level":"low\|medium\|high","probability":0.42,"model_version":"..."}` |
| `GET /health` | Readiness plus the identity of the active model (`model_version`, `data_hash`, `feature_count`, `thresholds`) |
| `GET /v1/model` | Provenance: identity, feature order and guards, thresholds in force, and the artifact's `metrics`/`baseline` verbatim. Parameters are not exposed |
| `GET /metrics` | Prometheus text: predictions by level, refusals by code, model info, HTTP traffic, process resources |

The published document is `backend/contracts/openapi/inference-v1.yaml`, and the
contract test in `internal/handler` validates real responses against it, so the
document cannot drift from what the handlers emit.

## The artifact: `firewatch-model/v1`

One JSON document. Startup fails on a missing, unparsable or contract-violating
artifact: the service never serves with a zero model, because a zero model answers
0.5 for every input and looks healthy from the outside.

```jsonc
{
  "format": "firewatch-model/v1",       // must equal this literal
  "version": "valparaiso-logistic-v1",  // reported by /health and /predict
  "trained_at": "2026-11-01T00:00:00Z", // RFC3339
  "data_hash": "9f2c…",                 // provenance, reported by /health
  "features": [                          // THIS ORDER is the model's input order
    { "name": "temp_max", "unit": "degC", "min": -5, "max": 45 }
  ],
  "impute": [24.0],                      // positional, one value per feature
  "center": [18.5],                      // positional
  "scale":  [6.5],                       // positional, must be > 0
  "kind": "logistic",                    // "logistic" or "forest"
  "intercept": -2.25,                    // logistic only
  "coef": [0.62],                        // logistic only, one per feature
  "thresholds": { "medium": 0.35, "high": 0.65 }, // optional
  "metrics": { "roc_auc": 0.78 },        // opaque, provenance only
  "baseline": { "kind": "climatology" }  // opaque, provenance only
}
```

A forest artifact replaces `intercept`/`coef` with `trees`, each
`{"nodes": [{"feature": 0, "threshold": 30, "left": 1, "right": 2}, {"value": 0.1}]}`.
`left`/`right` are indexes into the same tree, node `0` is the root, `value` is set
only on leaves, and a forest answers with the arithmetic mean of its trees' leaf
probabilities — a modelling choice, documented here because averaging in logit
space would give a different number for the same trees.

Validation refuses: a wrong `format`; an unknown or misspelled field (so a typo is
never silently dropped); a missing `version`, `trained_at` or `data_hash`; an
empty, duplicate or unit-less feature; `min >= max`; a non-finite number anywhere;
arrays whose length differs from the feature count; `scale <= 0`; an imputed value
outside its feature's guard; a coefficient count that differs from the feature
count; a forest node index out of bounds, pointing at itself, unreachable from the
root, or reachable twice on one path (a cycle is a hang, not an error a caller can
see); a leaf value outside `[0, 1]`; `medium >= high`; and `metrics`/`baseline`
that are not JSON objects.

### Preprocessing order

Per feature, in this order — the order is part of the model, not an
implementation detail:

1. `NaN` (or a JSON `null`) → `impute[i]`
2. `(x - center[i]) / scale[i]`
3. the model

A JSON `null` means "declared but unobserved" and is imputed. Omitting the key is
a missing feature and is refused. A value outside `[min, max]` is refused — never
clamped, because clamping answers confidently about a reading the model was never
trained on.

### Units and guards

`unit` is required and `min`/`max` are inclusive guards, not decoration: the real
source mixes units (`radiation_sol` reaches 1758 W/m², `pressure_atm` drops to 593
hPa with no unit declared), so the guard is what keeps a reading in the wrong unit
from being scored as if it were in the right one. The committed fixtures use
`temp_max`, `temp_min`, `humidity_rel`, `wind_speed`, `wind_dir`, `precip_sum`
and `radiation_sol`.

### Levels

`probability < medium` is `low`, `< high` is `medium`, otherwise `high`. Cut
points come from the artifact when it declares them; otherwise the documented
defaults `0.40`/`0.70` apply. `/v1/model` reports the pair in force.

## Refusals

Every refusal is HTTP 400 with a stable machine-readable `code`, the echoed
`request_id`, and a human `detail`. A non-finite probability is a 500 with code
`non_finite_probability`, never a response.

| Code | Meaning |
| --- | --- |
| `invalid_body` | Not the documented object shape (malformed JSON, unknown member, non-number value, trailing document) |
| `duplicate_feature` | One feature name declared twice |
| `missing_feature` | A feature the model needs was not declared |
| `unknown_feature` | A name the model does not take |
| `out_of_range` | A value outside the feature's declared guard |
| `non_finite_probability` | The model produced a NaN or infinity (500) |
| `scoring_failed` | Unexpected scoring failure (500) |

A name-addressed body means "wrong number of features" is exactly the union of
`missing_feature` and `unknown_feature`, so the count is reported in
`missing_feature`'s detail rather than as a fourth code that could never be
reached on its own.

## Running

```sh
export FIREWATCH_MODEL_PATH=testdata/logistic.json
go run ./cmd/server                      # :8080
go run ./cmd/server -validate            # parse + validate, print identity, exit
```

`-validate` loads through the same gate startup uses, so a pipeline that runs it
cannot pass a document the server would reject.

Docker build context is `backend/` (the module lives under `services/inference`),
and the artifact is mounted, not baked into the image:

```sh
docker build -f services/inference/Dockerfile -t firewatch-inference .
docker run --rm -p 8080:8080 \
  -v "$PWD/artifacts:/models:ro" \
  -e FIREWATCH_MODEL_PATH=/models/model.json \
  firewatch-inference
```

## Tests

```sh
GOWORK=off go test ./...
```

The suite covers, deliberately:

- logistic scoring against arithmetic written out in the test (1e-12);
- forest scoring walking to the correct leaf on the two-tree fixture, including
  the inclusive `<=` boundary;
- preprocessing order, asserted on the standardized value itself so a swapped
  order fails (impute-after-standardize gives 11, scale-then-impute-then-center
  gives 2.5);
- feature order enforcement: the same values in a different declaration order
  score identically, while swapping two values moves the answer;
- every refusal code, the inclusive guard edges, and that an out-of-guard value is
  refused rather than clamped;
- startup failure on a bad `format`, a coefficient length mismatch, an
  out-of-range `min`/`max`, a forest node index out of bounds, a cycle and an
  unreachable node;
- HTTP contract tests for `/predict`, `/health`, `/v1/model` and `/metrics`,
  validated against `contracts/openapi/inference-v1.yaml`;
- equivalence with the frozen Python reference (below), `|diff| < 1e-9` on every
  row.

### Equivalence fixture

`testdata/equivalence/expected.csv` holds twelve realistic Valparaíso rows (one
with a `NaN` reading, one sitting on every guard edge) and the probability each
must produce. `internal/scorer/equivalence_test.go` replays it through the Go
scorer.

`tools/reference_scorer.py` is the independent implementation that produced the
column: pure standard library, no numpy, no scikit-learn. **CI never runs
Python** — the CSV is the fixture and the script documents provenance. To
regenerate after a deliberate model change:

```sh
python3 tools/reference_scorer.py testdata/logistic.json testdata/equivalence/expected.csv
python3 tools/reference_scorer.py testdata/logistic.json testdata/equivalence/expected.csv --check
```

## Wired into the stack

`backend/docker-compose.yml` runs this service on the internal network with the
fixture artifact mounted at `/models/logistic.json`. It publishes no host port and
the gateway has no route to it, so a browser cannot reach it and the model surface
is not exposable from the public edge.

`risk` reaches it through `INFERENCE_URL` with `INFERENCE_TIMEOUT`. A timeout,
transport failure, 5xx, refused sample or unreadable body fails that unit closed:
nothing is persisted and no rule classifier is consulted, because falling back
silently would break the traceability the published batch guarantees. The
variables are documented in [`../docs/model-activation.md`](../docs/model-activation.md).

The production Compose stack and the Kubernetes manifests do not know this service
yet; that belongs to the deployment slice, together with the deploy workflow.
