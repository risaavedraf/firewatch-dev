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

# Derive the region-05 centroids from the tracked DPA geometry (offline) and commit
# the result. Rerunning it reproduces the committed file.
python centroids.py

# One real request per named source, through the on-disk cache, for one comuna.
# The second run of the same command hits the cache and opens no socket.
python meteo.py --source open-meteo --cut 05101 --start 2015-12-28 --end 2016-01-03
python meteo.py --source nasa-power --cut 05101 --start 2015-12-28 --end 2016-01-03

# Season-to-date aggregates for one season and one comuna, from one named source.
python seasons.py --source open-meteo --first 2015 --last 2015 --comunas 05101 \
    --out out/seasons
# The whole range is 38 comunas x 39 seasons = 1 482 cached requests per source.
python seasons.py --source open-meteo --out out/seasons

# Tests and lint (the CI job runs exactly these, offline).
python -m pytest -q
python -m ruff check .
```

Outputs are `panel.csv` (one row per comuna-day, then the seven feature columns in the
artifact's order) and `manifest.json` (counts, dropped rows, date range, feature status
and the reasons). `seasons.py` writes `seasons.csv` (one row per comuna-season) and its
own `manifest.json`. All of them are products: rebuild them, never hand-edit them.

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

## Meteorology from a source that has quiet days (`meteo.py`)

The gap `assemble.py` ends on -- a registry that only carries weather on days with a
fire -- is answered here, not there. Two reanalysis archives answer for a point with no
credential and no registration:

| Source | Name | Licence | Day boundary |
| --- | --- | --- | --- |
| [Open-Meteo archive](https://archive-api.open-meteo.com/v1/archive) | `open-meteo` | free for non-commercial use, no key | civil day in `America/Santiago` (asked for, and echoed back) |
| [NASA POWER](https://power.larc.nasa.gov/api/temporal/daily/point) | `nasa-power` | no credential, no non-commercial restriction; cite POWER | `LST` (the response says so): local standard time, **no DST** |

`meteo.fetch(source, coordinate, start, end)` is the single interface. It:

* checks the unit the source DECLARED for every variable before converting anything, so
  `wind_speed_10m_max` in `km/h` becomes `m/s` only because the reply says `km/h`; a
  source that switched to `m/s` stops the run instead of being divided by 3.6 twice;
* records a day the response omitted and a day carrying the source's fill value (`null`
  for Open-Meteo, `-999` for POWER) as **no reading**, never as zero, and reports
  `days_returned` against `days_requested` per request;
* stores the raw bytes in `data/raw/meteo/<source>/<key>.json` next to a provenance
  record (URL, retrieval instant, sha256, licence and terms). A second run reads those
  bytes and opens no socket; an entry whose bytes no longer match its sha256 is refused;
* raises `MeteoError` naming the source. **There is no fallback.** If Open-Meteo fails the
  caller decides, because switching automatically would move every downstream row onto a
  coarser grid under the same column names.

Radiation is the unit that matters most: both sources deliver a **daily integral in
MJ/m²**, never a mean irradiance in W/m². `meteo.FEATURE_UNITS` declares `MJ/m2`, and the
artifact descriptor still declares `W/m2` for `radiation_sol`. That disagreement is
reported (`seasons.check_artifact_units`, printed by the CLI, recorded in the manifest)
and **not converted**: a mean irradiance and a daily integral are different quantities,
and the descriptor is the document that has to change.

## Season aggregates (`seasons.py`)

One row per (comuna, season). A season is named by its start year and runs 1 July ->
30 June in `America/Santiago`; the feature window runs 1 July -> a cut-off that defaults
to **31 December**, so a row is what was known at that date. `--cutoff MM-DD` moves it,
and every row records the window it used.

| Feature | Aggregate | Unit |
| --- | --- | --- |
| `temp_max` | maximum of the daily maxima | `degC` |
| `temp_min` | minimum of the daily minima | `degC` |
| `humidity_rel` | **mean** of the daily values | `%` |
| `wind_speed` | maximum of the daily maxima | `m/s` |
| `precip_sum` | accumulated over the window | `mm` |
| `radiation_sol` | accumulated over the window | `MJ/m2` |

`humidity_rel` is a mean and not a minimum because each source publishes one value per
day and that value is already the day's average (Open-Meteo's parameter is
`relative_humidity_2m_mean`; POWER's `RH2M` is its daily value). A minimum would be the
driest day of the window -- a different quantity under the same name.

**An incomplete window is absent, never zero.** Every aggregate goes through
`assemble.reduce_window`, the rule the comuna-day panel and the Go side already use, so a
window covered 180 of 184 days yields `value=None` with `covered_days=180` written next to
it in the CSV. A zero is a measurement here (0.0 mm of rain is a fact), and absence is not.

Every row records the source that produced it and that source's day alignment, because
the two are not interchangeable: NASA POWER's `LST` day is not guaranteed to be an
`America/Santiago` civil day, and its grid is 0.5 x 0.625 degrees against Open-Meteo's
finer one. `wind_dir` is not produced by either source's verified parameter set and is
named as such in the manifest.

## Comuna centroids for region 05 (`centroids.py`)

A point, because both sources answer for a point. `data/comunas-valparaiso-centroids.json`
holds one coordinate for each of the 38 comunas, derived offline from the tracked DPA
GeoJSON that `backend/scripts/fetch_comunas.py` produced, with the same provenance block
the Maule mapping carries: source URL, exact query, retrieval date, the geometry's sha256
and the sha256 of its own entries. The method is recorded in the file: an area-weighted
planar centroid in EPSG:4326, holes subtracted, parts weighted by area. Not a geodesic
centroid -- and for the two archipelago comunas (Isla de Pascua, Juan Fernández) the
centroid is a point between islands, which is why each entry records its part count.

Nothing fetches a coordinate at runtime. `tests/test_centroids.py` recomputes the entries
from the geometry and the checksum from the file, so an entry edited by hand fails.

## Files

| Path | What |
| --- | --- |
| `fetch_sample.py` | downloads the sample into `data/raw/`, verifies sha256 + byte count, refuses to overwrite silently, prints the licence and citation |
| `assemble.py` | the comuna-day panel, its windows, its feature contract and manifest |
| `comunas.py` | name -> CUT resolution and the mapping checksum |
| `meteo.py` | the two keyless meteorology sources, their units, the on-disk cache and its provenance |
| `seasons.py` | the season window, the six season-to-date aggregates, the absence rule and the season manifest |
| `centroids.py` | derives the region-05 centroids from the tracked DPA geometry and reads the committed file |
| `data/comunas-maule-cut.json` | the committed name -> CUT mapping for region 07 (data, not code; read offline) |
| `data/comunas-valparaiso-centroids.json` | the committed centroids for the 38 comunas of region 05, with provenance |
| `data/raw/` | git-ignored: the downloaded CONAF sample and the cached meteorology responses |
| `tests/fixtures/PROVENANCE.md` | the URL, licence, retrieval date and exact selection rule of both CONAF fixtures |
| `tests/fixtures/meteo/PROVENANCE.md` | the same, for the four recorded meteorology responses (two sources, two coordinates) |
| `tests/` | pytest, offline: sockets are blocked by an autouse fixture that a test proves bites |
