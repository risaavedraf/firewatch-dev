# training

The training-side half of the model's contract: it turns the CONAF per-event registry
into a **comuna-day panel** with the same feature names, the same units and the same
aggregation windows that `risk` sends to the inference service. Python, because
training is the one place the project allows it; the rest of the backend stays Go, and
nothing here is imported by a Go service.

**This package does not train a model and the sample it ships is not a training set.**
The project's training label is the access-gated CONAF product (DOI `10.71578/UXAUN5`,
19 seasons, `"restricted": true` on every file). What is downloadable today is the
FONDEF repository's open sample -- **one season of one region**, Maule 2018-2019, 951
events -- and it is used for exactly what it can support: the schema, the name-to-CUT
join, the event-to-day aggregation and the window machinery. Say that wherever a reader
might mistake a panel for a model's training data.

## Commands

```bash
# Fetch the whole sample into the git-ignored data/raw/, verified against the
# documented sha256. Prints the licence and the citation it requires.
python backend/training/fetch_sample.py

# Build the panel and its manifest from the committed fixture (offline).
cd backend/training
python assemble.py --source tests/fixtures/conaf-maule-sample.csv --out out

# Same, against the verified full sample (951 events, 30 comunas, 8 580 comuna-days).
python assemble.py --source data/raw/conaf-maule-2018-2019.csv --out out/full

# Tests and lint (the CI job runs exactly these, offline).
python -m pytest -q
python -m ruff check .
```

Outputs are `panel.csv` (one row per comuna-day, then the seven feature columns in the
artifact's order) and `manifest.json` (counts, dropped rows, date range, feature status
and the reasons). Both are products: rebuild them, never hand-edit them.

## The feature list is not written here

`backend/services/inference/testdata/logistic.json` is the artifact descriptor -- the same
document the inference service serves at `GET /v1/model` and the one `risk` builds its
input vector from. `assemble.py` reads it and mirrors `features[].name`, in its order,
with its units. `PRODUCERS` in `assemble.py` is the only table in Python that says how a
name could be produced from this source, and it is validated against the artifact **in
both directions** before a row is read:

* a name the module would produce that the artifact does not declare is a failure;
* an artifact feature with no producer entry is a failure (silence there would drop a
  declared input without anyone noticing);
* a feature whose source unit contradicts the artifact's unit is **refused, not
  converted** -- the same guard that caught `precip_sum` declared in `mm/day` against
  the `mm` `risk` sends.

So renaming a feature in the artifact breaks this build loudly instead of producing a
panel with a column nobody serves.

## What this source can and cannot supply

| Feature | Status | Why |
| --- | --- | --- |
| `humidity_rel` | **from the source** | `Humedad`, 0-97, admits only percent; the mean of the comuna-day's readings |
| `temp_max` | unavailable | `Temperatura` is one reading per event, not a series across the day |
| `temp_min` | unavailable | same |
| `wind_speed` | unavailable | `Velocidad viento` declares no unit and its range fits both m/s and km/h |
| `wind_dir` | unavailable | `Direccion viento` is an 8-point cardinal word plus `Calmo`, not degrees |
| `precip_sum` | unavailable | no precipitation column at all |
| `radiation_sol` | unavailable | no radiation column |

Six of seven stay empty, and empty is the point: a zero would be a measurement the
source never made, and the model cannot tell the difference. The manifest records each
reason, and `filled_comuna_days` proves how much of the panel a column actually covers.

**The gap that matters.** This source attaches meteorology to events, so a comuna-day
without a fire has no reading at all: every feature cell is empty exactly on the rows
whose label is `0`. A model trained on this panel would learn from *whether a reading
exists* -- which is the label -- rather than from its value. Any future source has to
carry readings on quiet days for these columns to mean anything.

`precip_sum` is the most expensive feature to get wrong, so the seven-day rule is code,
not a parameter: `precipitation_sum` refuses any window other than seven civil days and
refuses to report a partial window as a smaller sum. Today no column can feed it; the
tests pin both halves of the rule so the contract exists before the data does.

## Labels

* `fire_today` is `1` on a comuna-day with one or more **dated** events, `0` otherwise;
  `fire_count` counts them and `burned_ha` sums `Superficie total`.
* Multiple events on one comuna-day collapse into **one** row. They are counted, never
  deduplicated away: the fixture has a four-event day (Talca 2018-12-18, `fire_count = 4`,
  2.3 ha) and the manifest reports how many events collapsed.
* The panel is a **grid**: every comuna the source names, every local date between the
  first and last dated event. Without the quiet rows the label has no zeros. Their
  `burned_ha` is `0.0` because a day without a fire burned nothing -- the label's own
  zero, which is not the same thing as an absent feature. A fire day with an unreadable
  `Superficie total` reports `burned_ha` as empty instead, so a missing area never becomes
  a zero.
* A row whose `Inicio` is blank or unparsable is **excluded and counted**, never imputed
  to a date. The registry writes two date shapes in one column (`07/09/2018 09:38` and
  `30-ene-2019 17:20`); a parser that knows only the first manufactures exactly 502 null
  dates out of 951 rows, which is where the widely repeated "502 null dates" comes from.
  All 951 rows carry a parseable date, and the manifest counts both shapes separately.

## Comuna name to CUT

The registry names the comuna (`Vichuquen`), the rest of FireWatch keys on the CUT
(`07109`). `data/comunas-maule-cut.json` maps the 30 comunas of region 07, taken from the
official DPA layer of IDE Chile -- the same layer `backend/scripts/fetch_comunas.py`
reads for region 05 -- with the URL, the exact query, the retrieval date and the sha256 of
its own entries recorded inside the file. Resolution folds accents and case instead of
carrying an alias per spelling. A name that still does not resolve keeps `cut = ""`, its
own name in `comuna_unresolved`, and appears in the manifest: a nearby code would look
like a real comuna to every join downstream. For this source all 30 names resolve.

## Date and time

Every daily window is cut in `America/Santiago`, loaded once at startup. A host without
that zone's data is a hard failure, never a silent UTC fallback -- cut in UTC, the day
would end at 20:00 or 21:00 local time, and Chile's DST makes a day 23 or 25 hours long
twice a year. `Inicio` carries local wall-clock time with no offset and is read as that
zone's civil time.

## Files

| Path | What |
| --- | --- |
| `fetch_sample.py` | downloads the sample into `data/raw/`, verifies sha256 + byte count, refuses to overwrite silently, prints the licence and citation |
| `assemble.py` | the panel, the windows, the feature contract, the manifest |
| `comunas.py` | name -> CUT resolution and the mapping checksum |
| `data/comunas-maule-cut.json` | the committed mapping (data, not code; read offline) |
| `tests/fixtures/PROVENANCE.md` | the URL, licence, retrieval date and exact selection rule of both fixtures |
| `tests/` | pytest, offline: sockets are blocked by an autouse fixture |
