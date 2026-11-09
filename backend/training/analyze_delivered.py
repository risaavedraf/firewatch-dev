"""Analyse the three delivered data files, file by file, and say what each can support.

THE POINT OF THIS MODULE IS A SPECIFICATION, NOT A SCORE.

The team delivered three CSV files and no documentation: a 90-day regional CONAF
dashboard, a station catalogue, and one hourly snapshot of every station. The owner asked
what they contain and what is missing. `train_delivered.py` already answered the model
half of that question -- it trained two models on the dashboard and neither beat
climatology, and `REPORT-delivered-data.md` is that measurement, not repeated here. This
module asks the prior question and answers it per file, with numbers: what is in each
file, what can it not support, and which concrete dataset feature would fix it.

The output is a committed report, `ANALYSIS-delivered-data.md`, written by this module.
The inputs are the three untracked files under `data/`, so the report cannot be rebuilt
in CI; what CI can do is run the same code over trimmed fixtures and prove the claims
are computed rather than typed.

WHAT IS MEASURED HERE, AND WHAT IS REFUSED
------------------------------------------
Every number in the report comes from the bytes it read:

* The label is derived, never assumed. `nro_inc_act` is season-cumulative, so a
  region-day is positive when the counter advanced. A region-day with no predecessor has
  a structurally unknown label and is counted separately instead of being called a zero.
* A cumulative counter that moves backwards is a revision, not a number to clamp
  silently. The revisions of `sup_afec_act` are listed with both dates and both values.
* Coverage is a set difference. The grid the file should have is regions x dates; the
  rows it has are counted against it.
* Emptiness is measured per column: a column whose every cell is blank is reported as
  empty, so a quality flag that was never populated cannot pass as a quality flag.
* A value is checked against a **stated physical bound**, and the bound's reason is part
  of the table. A bound that is merely the observed range would bless whatever the sensor
  wrote, which is how a solar irradiance above the solar constant ends up inside a
  model's declared range.
* Duplicates are counted by key, and the count is reported next to the rule a reader
  needs to collapse them.
* The station-to-comuna chain is checked in two steps: the value must pad to a
  structurally valid CUT, and the comuna must exist in the tracked DPA layer for the two
  regions this repository carries one for. The rest of the country is checked structurally
  only, and the report says so instead of implying a registry it does not have.

Run it as::

    python backend/training/analyze_delivered.py

    python backend/training/analyze_delivered.py \\
        --dashboard backend/training/tests/fixtures/delivered/dashboard-conaf-analysis-sample.csv \\
        --emas backend/training/tests/fixtures/delivered/emas-sample.csv \\
        --rtdata backend/training/tests/fixtures/delivered/rtdata-sample.csv \\
        --out backend/training/out/analysis/fixture/ANALYSIS.md
"""

from __future__ import annotations

import argparse
import csv
import datetime as dt
import hashlib
import json
import re
import sys
import unicodedata
from collections.abc import Iterable, Mapping, Sequence
from dataclasses import dataclass
from pathlib import Path
from typing import Any

# ---------------------------------------------------------------------------
# Paths and defaults
# ---------------------------------------------------------------------------

REPO_ROOT = Path(__file__).resolve().parents[2]
PACKAGE_ROOT = Path(__file__).resolve().parent

#: The three delivered files, as they sit in the repository root. Untracked and named
#: here so a run with no arguments is the real run.
DEFAULT_DASHBOARD = REPO_ROOT / "data" / "dashboard_conaf_202609281623 (1).csv"
DEFAULT_EMAS = REPO_ROOT / "data" / "emas_202609281634 (1).csv"
DEFAULT_RTDATA = REPO_ROOT / "data" / "rtdata_y2026_202609281633 (1).csv"

#: Committed on purpose: `out/` is git-ignored and a measurement nobody can read is not
#: a deliverable. The report is a product -- regenerate it, never edit it.
DEFAULT_REPORT = PACKAGE_ROOT / "ANALYSIS-delivered-data.md"

#: The artifact descriptor is the authority on the feature vocabulary, exactly as it is
#: for `assemble.py`. The seven names `risk` sends are read from here, not restated.
DEFAULT_ARTIFACT = REPO_ROOT / "backend/services/inference/testdata/logistic.json"

#: The tracked DPA entries this package already carries. Region 05 is the product's
#: region and the one the coverage split is computed for.
VALPARAISO_COMUNAS = PACKAGE_ROOT / "data" / "comunas-valparaiso-centroids.json"
MAULE_COMUNAS = PACKAGE_ROOT / "data" / "comunas-maule-cut.json"

# ---------------------------------------------------------------------------
# Column names, spelled once
# ---------------------------------------------------------------------------

COL_ID = "id"
COL_REGION = "region"
COL_ACT = "nro_inc_act"
COL_ANT = "nro_inc_ant"
COL_PROM = "nro_inc_prom_quint"
COL_SUP_ACT = "sup_afec_act"
COL_SUP_ANT = "sup_afec_ant"
COL_SUP_PROM = "sup_afec_prom_quint"
COL_CREACION = "fecha_creacion"
COL_FECHA = "fecha"

DASHBOARD_COLUMNS = (
    COL_ID,
    COL_REGION,
    COL_ACT,
    COL_ANT,
    COL_PROM,
    COL_SUP_ACT,
    COL_SUP_ANT,
    COL_SUP_PROM,
    COL_CREACION,
    COL_FECHA,
)

#: The numeric columns of the dashboard, in the order the file writes them. `id` is an
#: identifier and is left out of every statistic.
DASHBOARD_NUMERIC = (
    COL_ACT,
    COL_ANT,
    COL_PROM,
    COL_SUP_ACT,
    COL_SUP_ANT,
    COL_SUP_PROM,
)

#: Tokens that would make a dashboard column a weather measurement. The delivered file
#: has none, and the cascade table reports the count rather than asserting the absence.
WEATHER_TOKENS = ("temp", "humedad", "viento", "precip", "radiacion", "presion")

EMA_ID = "ema_ia_id"
EMA_NAME = "ema_c_nombre"
EMA_CODE = "ema_c_codigo"
EMA_FIRST = "primera_lectura"
EMA_LAST = "ultima_lectura"
EMA_ACTIVE = "ema_l_estacionactiva"
EMA_COMUNA = "com_ia_id"
EMA_REGION = "region_nombre"
EMA_LAT = "ema_f_latitudwgs84"
EMA_LON = "ema_f_longitudwgs84"

EMAS_COLUMNS = (
    EMA_ID,
    EMA_NAME,
    EMA_CODE,
    EMA_FIRST,
    EMA_LAST,
    EMA_COMUNA,
    EMA_ACTIVE,
    EMA_LAT,
    EMA_LON,
    EMA_REGION,
)

RT_ID = "rtd_ia_id"
RT_STATION = "ema_ia_id"
RT_VARIABLE = "rtd_c_variableid"
RT_INSTANT = "rtd_dt_marcatiempo"
RT_VALUE = "rtd_f_valor"
RT_VERSION_INSTANT = "rtd_dt_versiondt"
RT_QUALITY = "rtd_i_calidad"
RT_CHECKS = "rtd_i_resultadochecks"

RTDATA_COLUMNS = (
    RT_ID,
    RT_STATION,
    RT_VARIABLE,
    RT_INSTANT,
    RT_VALUE,
    RT_VERSION_INSTANT,
    RT_QUALITY,
    RT_CHECKS,
)

#: Which CONAF variable could feed which artifact feature. The table is checked in both
#: directions against the artifact before a row is read: a feature with no entry is a
#: failure, and an entry naming a feature the artifact does not declare is a failure.
VARIABLE_FEATURES: dict[str, str] = {
    "TEMP_MAX": "temp_max",
    "TEMP_MIN": "temp_min",
    "HUMEDAD_REL": "humidity_rel",
    "VEL_VIENTO": "wind_speed",
    "DIR_VIENTO": "wind_dir",
    "PRECIPITACION": "precip_sum",
    "RADIACION_SOL": "radiation_sol",
}

#: The delivered `region_nombre` spellings folded to ASCII, with the CUT prefix the
#: region actually has. A label with no entry here is a named failure, because silence
#: would let a mis-keyed region pass a check that exists to catch exactly that.
REGION_CUT_PREFIX = {
    "arica y parinacota": "15",
    "tarapaca": "01",
    "antofagasta": "02",
    "atacama": "03",
    "coquimbo": "04",
    "valparaiso": "05",
    "libertador general bernardo o'higgins": "06",
    "maule": "07",
    "nuble": "16",
    "biobio": "08",
    "la araucania": "09",
    "los rios": "14",
    "los lagos": "10",
    "aysen del general carlos ibanez del campo": "11",
    "magallanes y de la antartica chilena": "12",
    "metropolitana de santiago": "13",
}

CUT_PATTERN = re.compile(r"^\d{1,5}$")

#: The solar constant in W/m2: the hard ceiling for surface global irradiance.
SOLAR_CONSTANT = 1361.0


class AnalysisError(Exception):
    """A failure the operator must fix; the CLI prints it instead of a traceback."""


# ---------------------------------------------------------------------------
# Small helpers
# ---------------------------------------------------------------------------


def fold(text: str) -> str:
    """Fold a label to ASCII lowercase, the same way the rest of the package does."""
    decomposed = unicodedata.normalize("NFKD", text)
    return "".join(ch for ch in decomposed if not unicodedata.combining(ch)).casefold()


def sha256_of(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(65536), b""):
            digest.update(chunk)
    return digest.hexdigest()


def display(path: Path) -> str:
    """Show a path relative to the repository when it lives inside it."""
    try:
        return path.resolve().relative_to(REPO_ROOT).as_posix()
    except ValueError:
        return path.as_posix()


def number(value: str, *, column: str, line: int) -> float:
    """Parse a numeric cell, naming the column and row when it is not one."""
    try:
        return float(value)
    except (TypeError, ValueError) as error:
        raise AnalysisError(f"{column} line {line}: {value!r} is not a number") from error


def rate(numerator: int, denominator: int) -> float:
    return numerator / denominator if denominator else 0.0


@dataclass(frozen=True)
class Table:
    """A CSV as read: the header in file order, and one dict per row."""

    columns: tuple[str, ...]
    rows: tuple[dict[str, str], ...]


def read_table(path: Path) -> Table:
    if not path.is_file():
        raise AnalysisError(f"the input file {path} does not exist")
    try:
        with path.open("r", encoding="utf-8", newline="") as handle:
            reader = csv.DictReader(handle)
            columns = tuple(reader.fieldnames or ())
            rows = tuple(dict(row) for row in reader)
    except OSError as error:
        raise AnalysisError(f"cannot read {path} ({error})") from error
    except csv.Error as error:
        raise AnalysisError(f"{path} is not readable CSV ({error})") from error
    if not columns:
        raise AnalysisError(f"{path} has no header row")
    return Table(columns=columns, rows=rows)


def require_columns(table: Table, required: Sequence[str], path: Path) -> None:
    missing = [name for name in required if name not in table.columns]
    if missing:
        raise AnalysisError(
            f"{path} does not carry the column(s) {', '.join(missing)}; a rule that "
            f"reads a missing column would measure nothing"
        )


def file_facts(path: Path, table: Table) -> FileFacts:
    return FileFacts(
        name=path.name,
        sha256=sha256_of(path),
        size_bytes=path.stat().st_size,
        rows=len(table.rows),
        columns=len(table.columns),
    )


def pearson(xs: Sequence[float], ys: Sequence[float]) -> float | None:
    """Pearson's r, or None when one side has no variance and r is undefined."""
    count = len(xs)
    if count < 2 or count != len(ys):
        return None
    mean_x = sum(xs) / count
    mean_y = sum(ys) / count
    covariance = sum((x - mean_x) * (y - mean_y) for x, y in zip(xs, ys))
    spread_x = sum((x - mean_x) ** 2 for x in xs)
    spread_y = sum((y - mean_y) ** 2 for y in ys)
    if spread_x == 0.0 or spread_y == 0.0:
        return None
    return covariance / (spread_x * spread_y) ** 0.5


def correlation_matrix(
    columns: Sequence[str], values: Mapping[str, Sequence[float]]
) -> tuple[tuple[float | None, ...], ...]:
    return tuple(
        tuple(pearson(values[left], values[right]) for right in columns) for left in columns
    )


def strongest_pair(
    columns: Sequence[str], matrix: Sequence[Sequence[float | None]]
) -> tuple[str, str, float] | None:
    """The off-diagonal pair with the largest absolute correlation."""
    best: tuple[str, str, float] | None = None
    for left in range(len(columns)):
        for right in range(left + 1, len(columns)):
            value = matrix[left][right]
            if value is None:
                continue
            if best is None or abs(value) > abs(best[2]):
                best = (columns[left], columns[right], value)
    return best


def standard_atmosphere_height(pressure_hpa: float) -> float:
    """Metres at which the 1976 standard atmosphere reaches this pressure."""
    ratio = pressure_hpa / 1013.25
    return 44330.0 * (1.0 - ratio ** (1.0 / 5.25588))


def cut_from(raw: str) -> str | None:
    """Pad a `com_ia_id` to a five-digit CUT, or None when it cannot be one.

    The delivered column is an integer-ish CUT: regions 01..09 lose their leading zero,
    so `5102` is comuna 05102. Nothing is guessed -- a value that does not pad to a
    structurally valid CUT is refused and counted.
    """
    text = raw.strip()
    if not CUT_PATTERN.match(text):
        return None
    padded = text.zfill(5)
    region = int(padded[:2])
    comuna = int(padded[2:])
    if not 1 <= region <= 16 or comuna == 0:
        return None
    return padded


def date_of(stamp: str) -> str | None:
    """The civil date printed at the head of a delivered timestamp, or None."""
    text = stamp.strip()
    if len(text) < 10:
        return None
    try:
        return dt.date.fromisoformat(text[:10]).isoformat()
    except ValueError:
        return None


# ---------------------------------------------------------------------------
# Shared shapes
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class FileFacts:
    name: str
    sha256: str
    size_bytes: int
    rows: int
    columns: int


@dataclass(frozen=True)
class Revision:
    """A cumulative counter that moved backwards between two region-days."""

    region: str
    previous_date: str
    date: str
    previous: float
    current: float


@dataclass(frozen=True)
class StationInfo:
    name: str
    latitude: str
    longitude: str


@dataclass(frozen=True)
class LimitViolation:
    variable: str
    value: float
    station: str
    station_name: str


# ---------------------------------------------------------------------------
# 1. The dashboard
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class DashboardAnalysis:
    facts: FileFacts
    regions: tuple[str, ...]
    dates: tuple[str, ...]
    expected_cells: int
    present_cells: int
    missing_cells: int
    duplicate_cells: int
    days_per_region_min: int
    days_per_region_max: int
    days_by_region: tuple[tuple[str, int], ...]
    duplicate_ids: int
    creation_mismatches: int
    weather_columns: int
    first_day_rows: int
    first_day_nonzero: int
    positives: int
    incidents: int
    act_decrements: int
    sup_decrements: int
    revisions: tuple[Revision, ...]
    positives_by_region: tuple[tuple[str, int], ...]
    month_rows: tuple[tuple[str, int, int], ...]
    train_months: tuple[str, ...]
    test_months: tuple[str, ...]
    train_rows: int
    train_positives: int
    test_rows: int
    test_positives: int
    columns: tuple[str, ...]
    panel_correlations: tuple[tuple[float | None, ...], ...]
    national_correlations: tuple[tuple[float | None, ...], ...]
    panel_strongest: tuple[str, str, float] | None
    national_strongest: tuple[str, str, float] | None

    @property
    def positive_rate(self) -> float:
        return rate(self.positives, self.present_cells)

    @property
    def train_rate(self) -> float:
        return rate(self.train_positives, self.train_rows)

    @property
    def test_rate(self) -> float:
        return rate(self.test_positives, self.test_rows)

    @property
    def base_rate_ratio(self) -> float | None:
        return self.test_rate / self.train_rate if self.train_rate else None


def analyze_dashboard(path: Path) -> DashboardAnalysis:
    """Measure the delivered dashboard: the grid, the label, the counters, correlation."""
    table = read_table(path)
    require_columns(table, DASHBOARD_COLUMNS, path)
    rows = table.rows
    if not rows:
        raise AnalysisError(f"{path} has no data rows")

    regions = tuple(sorted({row[COL_REGION] for row in rows}))
    dates = tuple(sorted({row[COL_FECHA] for row in rows}))
    for date in dates:
        if date_of(date) != date:
            raise AnalysisError(f"{path}: {date!r} is not an ISO date")

    keys = [(row[COL_REGION], row[COL_FECHA]) for row in rows]
    present = len(set(keys))
    duplicate_cells = len(keys) - present
    if duplicate_cells:
        raise AnalysisError(
            f"{path} carries {duplicate_cells} duplicated region-day(s); a cumulative "
            f"counter cannot be ordered when two rows claim the same day"
        )

    ordered = sorted(rows, key=lambda row: (row[COL_REGION], row[COL_FECHA]))
    positives = 0
    incidents = 0
    act_decrements = 0
    sup_decrements = 0
    revisions: list[Revision] = []
    by_region: dict[str, int] = {region: 0 for region in regions}
    positives_by_month: dict[str, int] = {}
    first_day_rows = 0
    first_day_nonzero = 0
    previous: dict[str, tuple[float, float, str]] = {}

    for index, row in enumerate(ordered, start=2):
        region = row[COL_REGION]
        date = row[COL_FECHA]
        act = number(row[COL_ACT], column=COL_ACT, line=index)
        sup = number(row[COL_SUP_ACT], column=COL_SUP_ACT, line=index)
        if region in previous:
            previous_act, previous_sup, previous_date = previous[region]
            if act < previous_act:
                act_decrements += 1
            elif act > previous_act:
                positives += 1
                incidents += round(act - previous_act)
                by_region[region] = by_region.get(region, 0) + 1
                month = date[:7]
                positives_by_month[month] = positives_by_month.get(month, 0) + 1
            if sup < previous_sup:
                sup_decrements += 1
                revisions.append(
                    Revision(
                        region=region,
                        previous_date=previous_date,
                        date=date,
                        previous=previous_sup,
                        current=sup,
                    )
                )
        else:
            first_day_rows += 1
            if act != 0.0:
                first_day_nonzero += 1
        previous[region] = (act, sup, date)

    months = tuple(sorted({date[:7] for date in dates}))
    if len(months) < 2:
        covered = ", ".join(months) if months else "none"
        raise AnalysisError(
            f"{path} covers the single month {covered}; there is no temporal base-rate "
            f"shift to measure"
        )
    train_months = months[:-1]
    test_months = months[-1:]

    month_rows: list[tuple[str, int, int]] = []
    for month in months:
        rows_in_month = sum(1 for date in dates if date[:7] == month) * len(regions)
        month_rows.append((month, rows_in_month, positives_by_month.get(month, 0)))

    train_rows = sum(count for month, count, _ in month_rows if month in train_months)
    train_positives = sum(pos for month, _, pos in month_rows if month in train_months)
    test_rows = sum(count for month, count, _ in month_rows if month in test_months)
    test_positives = sum(pos for month, _, pos in month_rows if month in test_months)

    days_per_region = {
        region: len({row[COL_FECHA] for row in rows if row[COL_REGION] == region})
        for region in regions
    }

    values: dict[str, list[float]] = {column: [] for column in DASHBOARD_NUMERIC}
    for index, row in enumerate(ordered, start=2):
        for column in DASHBOARD_NUMERIC:
            values[column].append(number(row[column], column=column, line=index))

    national: dict[str, list[float]] = {column: [] for column in DASHBOARD_NUMERIC}
    by_date: dict[str, list[dict[str, str]]] = {date: [] for date in dates}
    for row in ordered:
        by_date[row[COL_FECHA]].append(row)
    for date in dates:
        on_date = by_date[date]
        for column in DASHBOARD_NUMERIC:
            national[column].append(
                sum(number(row[column], column=column, line=0) for row in on_date)
            )

    panel_correlations = correlation_matrix(DASHBOARD_NUMERIC, values)
    national_correlations = correlation_matrix(DASHBOARD_NUMERIC, national)

    return DashboardAnalysis(
        facts=file_facts(path, table),
        regions=regions,
        dates=dates,
        expected_cells=len(regions) * len(dates),
        present_cells=present,
        missing_cells=len(regions) * len(dates) - present,
        duplicate_cells=duplicate_cells,
        days_per_region_min=min(days_per_region.values()),
        days_per_region_max=max(days_per_region.values()),
        days_by_region=tuple(sorted(days_per_region.items())),
        duplicate_ids=len(rows) - len({row[COL_ID] for row in rows}),
        creation_mismatches=sum(1 for row in rows if row[COL_CREACION] != row[COL_FECHA]),
        weather_columns=sum(
            1 for column in table.columns if any(token in fold(column) for token in WEATHER_TOKENS)
        ),
        first_day_rows=first_day_rows,
        first_day_nonzero=first_day_nonzero,
        positives=positives,
        incidents=incidents,
        act_decrements=act_decrements,
        sup_decrements=sup_decrements,
        revisions=tuple(revisions),
        positives_by_region=tuple(sorted(by_region.items())),
        month_rows=tuple(month_rows),
        train_months=train_months,
        test_months=test_months,
        train_rows=train_rows,
        train_positives=train_positives,
        test_rows=test_rows,
        test_positives=test_positives,
        columns=DASHBOARD_NUMERIC,
        panel_correlations=panel_correlations,
        national_correlations=national_correlations,
        panel_strongest=strongest_pair(DASHBOARD_NUMERIC, panel_correlations),
        national_strongest=strongest_pair(DASHBOARD_NUMERIC, national_correlations),
    )


# ---------------------------------------------------------------------------
# 2. The station catalogue
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class CoverageSplit:
    """Three sets of comuna CUTs: one station with all seven, some stations, none."""

    complete: tuple[str, ...]
    partial: tuple[str, ...]
    none: tuple[str, ...]

    @property
    def total(self) -> int:
        return len(self.complete) + len(self.partial) + len(self.none)


@dataclass(frozen=True)
class ComunaCoverage:
    cut: str
    name: str
    stations: int
    active: int
    counted: int
    best_variables: int
    classification: str


@dataclass(frozen=True)
class PrefixMismatch:
    region: str
    prefix: str
    stations: int
    example: str


@dataclass(frozen=True)
class EmasAnalysis:
    facts: FileFacts
    stations: int
    active: int
    inactive: int
    blank_region_rows: int
    region_labels: tuple[str, ...]
    stations_by_region: tuple[tuple[str, int], ...]
    active_by_region: tuple[tuple[str, int], ...]
    comunas_by_region: tuple[tuple[str, int], ...]
    comunas: int
    padded_cuts: int
    invalid_cuts: tuple[tuple[str, str], ...]
    prefix_mismatch_rows: int
    prefix_mismatches: tuple[PrefixMismatch, ...]
    dpa_checked: tuple[tuple[str, int, int, tuple[str, ...]], ...]
    earliest_date: str
    at_floor: int
    empty_first_reading: int
    latest_reading_date: str
    codes_nonempty: int
    codes_distinct: int
    active_without_readings: int
    valparaiso: CoverageSplit
    valparaiso_loose: CoverageSplit
    valparaiso_comunas: tuple[ComunaCoverage, ...]
    valparaiso_stations: int
    valparaiso_active: int
    valparaiso_comuna_count: int

    @property
    def stations_with_first_reading(self) -> int:
        return self.stations - self.at_floor - self.empty_first_reading

    @property
    def valparaiso_rule_difference(self) -> tuple[str, ...]:
        return tuple(sorted(set(self.valparaiso_loose.partial) - set(self.valparaiso.partial)))


def load_comuna_entries(path: Path) -> dict[str, str]:
    """Read a tracked DPA mapping as CUT -> comuna name."""
    try:
        document = json.loads(path.read_text(encoding="utf-8"))
    except OSError as error:
        raise AnalysisError(f"cannot read the comuna mapping {path} ({error})") from error
    except json.JSONDecodeError as error:
        raise AnalysisError(f"{path} is not valid JSON ({error})") from error
    entries = document.get("entries")
    if not isinstance(entries, list):
        raise AnalysisError(f"{path} has no 'entries' list")
    mapping: dict[str, str] = {}
    for entry in entries:
        cut = str(entry.get("cut", "")).strip()
        if not CUT_PATTERN.match(cut) or len(cut) != 5:
            raise AnalysisError(f"{path} carries the malformed CUT {cut!r}")
        mapping[cut] = str(entry.get("name", "")).strip()
    if not mapping:
        raise AnalysisError(f"{path} carries no entries")
    return mapping


def coverage_split(
    comunas: Mapping[str, str],
    stations: Iterable[tuple[str, str, bool]],
    variables: Mapping[str, frozenset[str] | set[str]],
    required: frozenset[str],
    *,
    active_only: bool,
) -> tuple[CoverageSplit, tuple[ComunaCoverage, ...]]:
    """Classify every comuna by the best station it has.

    `stations` is (station id, CUT, active). A comuna is `complete` when at least one
    counted station reports every required variable, `partial` when it has a counted
    station that does not, and `none` otherwise. Nothing here is a constant: removing a
    station's readings moves its comuna, which is what the tests mutate.
    """
    catalogue: dict[str, int] = {cut: 0 for cut in comunas}
    counted: dict[str, int] = {cut: 0 for cut in comunas}
    actives: dict[str, int] = {cut: 0 for cut in comunas}
    best: dict[str, int] = {cut: 0 for cut in comunas}
    for station, cut, active in stations:
        if cut not in comunas:
            continue
        catalogue[cut] += 1
        if active:
            actives[cut] += 1
        if active_only and not active:
            continue
        counted[cut] += 1
        covered = len(set(variables.get(station, frozenset())) & required)
        best[cut] = max(best[cut], covered)

    complete: list[str] = []
    partial: list[str] = []
    none: list[str] = []
    per_comuna: list[ComunaCoverage] = []
    for cut in sorted(comunas):
        if counted[cut] == 0:
            classification = "none"
            none.append(cut)
        elif best[cut] == len(required):
            classification = "complete"
            complete.append(cut)
        else:
            classification = "partial"
            partial.append(cut)
        per_comuna.append(
            ComunaCoverage(
                cut=cut,
                name=comunas[cut],
                stations=catalogue[cut],
                active=actives[cut],
                counted=counted[cut],
                best_variables=best[cut],
                classification=classification,
            )
        )
    return (
        CoverageSplit(complete=tuple(complete), partial=tuple(partial), none=tuple(none)),
        tuple(per_comuna),
    )


def analyze_emas(
    path: Path,
    *,
    readings: Mapping[str, frozenset[str] | set[str]],
    required: frozenset[str],
    comunas_path: Path = VALPARAISO_COMUNAS,
    maule_path: Path = MAULE_COMUNAS,
) -> EmasAnalysis:
    """Measure the station catalogue: counts, workable units, the CUT chain, coverage."""
    table = read_table(path)
    require_columns(table, EMAS_COLUMNS, path)
    rows = table.rows
    if not rows:
        raise AnalysisError(f"{path} has no data rows")

    stations = len({row[EMA_ID] for row in rows})
    if stations != len(rows):
        raise AnalysisError(
            f"{path} has {len(rows)} rows for {stations} station id(s); the catalogue is "
            f"not one row per station"
        )

    active = 0
    for row in rows:
        flag = row[EMA_ACTIVE].strip().lower()
        if flag not in {"true", "false"}:
            raise AnalysisError(
                f"{path}: station {row[EMA_ID]} has {row[EMA_ACTIVE]!r} in {EMA_ACTIVE}; "
                f"the flag is not a boolean"
            )
        if flag == "true":
            active += 1

    region_labels = sorted({row[EMA_REGION] for row in rows})
    blank_region_rows = sum(1 for row in rows if not row[EMA_REGION].strip())

    cuts: dict[str, str] = {}
    invalid: list[tuple[str, str]] = []
    padded = 0
    for row in rows:
        raw = row[EMA_COMUNA].strip()
        cut = cut_from(raw)
        if cut is None:
            invalid.append((row[EMA_ID], raw))
            continue
        if len(raw) < 5:
            padded += 1
        cuts[row[EMA_ID]] = cut

    prefix_counts: dict[tuple[str, str], list[str]] = {}
    for row in rows:
        cut = cuts.get(row[EMA_ID])
        label = row[EMA_REGION].strip()
        if cut is None or not label:
            continue
        expected = REGION_CUT_PREFIX.get(fold(label))
        if expected is None:
            raise AnalysisError(
                f"{path}: region label {label!r} has no CUT prefix in REGION_CUT_PREFIX; "
                f"an unmapped label would pass a check that exists to catch it"
            )
        if cut[:2] != expected:
            prefix_counts.setdefault((label, cut[:2]), []).append(row[EMA_NAME])
    mismatches = tuple(
        PrefixMismatch(region=label, prefix=prefix, stations=len(names), example=min(names))
        for (label, prefix), names in sorted(prefix_counts.items())
    )

    valparaiso = load_comuna_entries(comunas_path)
    maule = load_comuna_entries(maule_path)
    dpa_checked: list[tuple[str, int, int, tuple[str, ...]]] = []
    for label, reference in (("Valparaíso", valparaiso), ("Maule", maule)):
        present = {
            cuts[row[EMA_ID]]
            for row in rows
            if row[EMA_ID] in cuts and fold(row[EMA_REGION]) == fold(label)
        }
        if not present:
            continue
        known = len(present & set(reference))
        dpa_checked.append((label, known, len(present), tuple(sorted(present - set(reference)))))

    first_dates = [date_of(row[EMA_FIRST]) for row in rows]
    valid_first = [date for date in first_dates if date is not None]
    empty_first = sum(1 for row in rows if not row[EMA_FIRST].strip())
    if not valid_first:
        raise AnalysisError(f"{path} carries no readable {EMA_FIRST} value")
    earliest = min(valid_first)
    last_dates = [date for date in (date_of(row[EMA_LAST]) for row in rows) if date]

    codes = [row[EMA_CODE].strip() for row in rows if row[EMA_CODE].strip()]

    station_rows = [
        (row[EMA_ID], cuts[row[EMA_ID]], row[EMA_ACTIVE].strip().lower() == "true")
        for row in rows
        if row[EMA_ID] in cuts
    ]
    split, per_comuna = coverage_split(
        valparaiso, station_rows, readings, required, active_only=True
    )
    loose, _ = coverage_split(valparaiso, station_rows, readings, required, active_only=False)
    in_valparaiso = [entry for entry in station_rows if entry[1] in valparaiso]

    return EmasAnalysis(
        facts=file_facts(path, table),
        stations=stations,
        active=active,
        inactive=stations - active,
        blank_region_rows=blank_region_rows,
        region_labels=tuple(region_labels),
        stations_by_region=tuple(
            sorted(
                (label, sum(1 for row in rows if row[EMA_REGION] == label))
                for label in region_labels
            )
        ),
        active_by_region=tuple(
            sorted(
                (
                    label,
                    sum(
                        1
                        for row in rows
                        if row[EMA_REGION] == label and row[EMA_ACTIVE].strip().lower() == "true"
                    ),
                )
                for label in region_labels
            )
        ),
        comunas_by_region=tuple(
            sorted(
                (
                    label,
                    len(
                        {
                            cuts[row[EMA_ID]]
                            for row in rows
                            if row[EMA_REGION] == label and row[EMA_ID] in cuts
                        }
                    ),
                )
                for label in region_labels
            )
        ),
        comunas=len(set(cuts.values())),
        padded_cuts=padded,
        invalid_cuts=tuple(invalid),
        prefix_mismatch_rows=sum(entry.stations for entry in mismatches),
        prefix_mismatches=mismatches,
        dpa_checked=tuple(dpa_checked),
        earliest_date=earliest,
        at_floor=sum(1 for date in valid_first if date == earliest),
        empty_first_reading=empty_first,
        latest_reading_date=max(last_dates) if last_dates else "",
        codes_nonempty=len(codes),
        codes_distinct=len(set(codes)),
        active_without_readings=sum(
            1 for row in rows if row[EMA_ACTIVE] == "true" and row[EMA_ID] not in readings
        ),
        valparaiso=split,
        valparaiso_loose=loose,
        valparaiso_comunas=per_comuna,
        valparaiso_stations=len(in_valparaiso),
        valparaiso_active=sum(1 for entry in in_valparaiso if entry[2]),
        valparaiso_comuna_count=len(valparaiso),
    )


# ---------------------------------------------------------------------------
# 3. The realtime snapshot
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class PhysicalLimit:
    low: float
    high: float
    basis: str


#: Every variable the snapshot carries must have an entry: a variable with no declared
#: bound would pass a range check by being skipped, which is the failure mode the table
#: exists to prevent. `basis` is printed with the table so a reader can disagree with the
#: bound instead of having to trust it.
PHYSICAL_LIMITS: dict[str, PhysicalLimit] = {
    "TEMP_MAX": PhysicalLimit(
        -40.0,
        60.0,
        "degC: covers Chile's recorded extremes; outside it is a unit or sensor error",
    ),
    "TEMP_MED": PhysicalLimit(-40.0, 60.0, "degC: the same bound as TEMP_MAX"),
    "TEMP_MIN": PhysicalLimit(-40.0, 60.0, "degC: the same bound as TEMP_MAX"),
    "HUMEDAD_REL": PhysicalLimit(
        0.0,
        100.0,
        "percent by definition; a 0.00 value is counted separately because it is a "
        "sensor floor rather than a measurement",
    ),
    "VEL_VIENTO": PhysicalLimit(
        0.0,
        75.0,
        "m/s: above any surface wind ever recorded, so a larger value is a unit error",
    ),
    "DIR_VIENTO": PhysicalLimit(
        0.0,
        360.0,
        "degrees: outside it the column is in another convention (radians, tenths)",
    ),
    "PRECIPITACION": PhysicalLimit(
        0.0, 200.0, "mm: one reading above this is not a precipitation total"
    ),
    "RADIACION_SOL": PhysicalLimit(
        0.0,
        SOLAR_CONSTANT,
        "W/m2: surface global irradiance cannot exceed the solar constant, which is the "
        "irradiance at the top of the atmosphere",
    ),
    "PRESION_ATM": PhysicalLimit(
        500.0,
        1100.0,
        "hPa of station pressure: 500 hPa is reached only above roughly 5 500 m",
    ),
    "EVAPOTRANSPIRACION": PhysicalLimit(
        0.0, 20.0, "mm/day: a daily reference ET0 above this is not a daily value"
    ),
    "HORAS_FRIO": PhysicalLimit(0.0, 24.0, "hours in a day: the most a daily total can be"),
    "GRADOS_DIA": PhysicalLimit(0.0, 40.0, "degC day: a plausible daily degree-day total"),
}


@dataclass(frozen=True)
class VariableProfile:
    variable: str
    feature: str
    readings: int
    stations: int
    low: float
    high: float
    outside: int
    at_bound: int
    limit_low: float
    limit_high: float
    basis: str
    artifact_low: float | None
    artifact_high: float | None
    artifact_outside: int | None


@dataclass(frozen=True)
class DuplicateProfile:
    rows: int
    unique_keys: int
    extra_rows: int
    rows_in_groups: int
    groups_by_size: tuple[tuple[int, int], ...]
    stations_affected: int
    conflicting_groups: int
    conflicting_example: tuple[str, str, tuple[float, ...]]


@dataclass(frozen=True)
class PressureReading:
    value: float
    station: str
    station_name: str
    latitude: str
    longitude: str
    standard_height: float


@dataclass(frozen=True)
class RtdataAnalysis:
    facts: FileFacts
    stations: int
    variables: int
    instants: tuple[str, ...]
    max_instants_per_station_day: int
    empty_columns: tuple[str, ...]
    constant_columns: tuple[str, ...]
    profiles: tuple[VariableProfile, ...]
    duplicates: DuplicateProfile
    stations_all_seven: int
    coverage_histogram: tuple[tuple[int, int], ...]
    violations: tuple[LimitViolation, ...]
    humidity_boundary: tuple[LimitViolation, ...]
    lowest_pressures: tuple[PressureReading, ...]
    temperature_ordering_violations: int

    @property
    def instant_count(self) -> int:
        return len(self.instants)


def load_artifact(path: Path) -> dict[str, tuple[str, float, float]]:
    """Read the artifact's declared features as name -> (unit, min, max)."""
    try:
        document = json.loads(path.read_text(encoding="utf-8"))
    except OSError as error:
        raise AnalysisError(f"cannot read the artifact descriptor {path} ({error})") from error
    except json.JSONDecodeError as error:
        raise AnalysisError(f"{path} is not valid JSON ({error})") from error
    features = document.get("features")
    if not isinstance(features, list) or not features:
        raise AnalysisError(f"{path} declares no features")
    declared: dict[str, tuple[str, float, float]] = {}
    for entry in features:
        try:
            name = str(entry["name"])
            unit = str(entry["unit"])
            low = float(entry["min"])
            high = float(entry["max"])
        except (KeyError, TypeError, ValueError) as error:
            raise AnalysisError(f"{path} has a malformed feature entry ({error})") from error
        declared[name] = (unit, low, high)
    return declared


def check_variable_mapping(declared: Mapping[str, Any]) -> None:
    """Prove the CONAF variable table and the artifact describe the same vocabulary."""
    unknown = sorted(set(VARIABLE_FEATURES.values()) - set(declared))
    if unknown:
        raise AnalysisError(
            f"the artifact declares no feature named {', '.join(unknown)}; a name this "
            f"analysis maps to is a name `risk` cannot send"
        )
    missing = sorted(set(declared) - set(VARIABLE_FEATURES.values()))
    if missing:
        raise AnalysisError(
            f"no CONAF variable entry for the artifact feature(s) {', '.join(missing)}; "
            f"every declared input has to be accounted for, including the ones this "
            f"source cannot feed"
        )


def duplicate_profile(
    rows: Sequence[Mapping[str, str]], *, key: Sequence[str], value: str, order: str
) -> DuplicateProfile:
    """Count duplicated keys, and the groups whose copies disagree on the value."""
    groups: dict[tuple[str, ...], list[Mapping[str, str]]] = {}
    for row in rows:
        groups.setdefault(tuple(row[column] for column in key), []).append(row)
    sizes: dict[int, int] = {}
    for members in groups.values():
        sizes[len(members)] = sizes.get(len(members), 0) + 1
    repeated = [members for members in groups.values() if len(members) > 1]
    conflicting = [members for members in repeated if len({row[value] for row in members}) > 1]
    example: tuple[str, str, tuple[float, ...]] = ("", "", ())
    if conflicting:
        members = sorted(conflicting[0], key=lambda row: row[order])
        example = (
            members[0][key[0]],
            members[0][key[1]],
            tuple(number(row[value], column=value, line=0) for row in members),
        )
    return DuplicateProfile(
        rows=len(rows),
        unique_keys=len(groups),
        extra_rows=len(rows) - len(groups),
        rows_in_groups=sum(len(members) for members in repeated),
        groups_by_size=tuple(sorted(sizes.items())),
        stations_affected=len({members[0][key[0]] for members in repeated}),
        conflicting_groups=len(conflicting),
        conflicting_example=example,
    )


def analyze_rtdata(
    path: Path,
    *,
    stations: Mapping[str, StationInfo] | None = None,
    artifact_path: Path = DEFAULT_ARTIFACT,
) -> RtdataAnalysis:
    """Measure the snapshot: duplicates, empty columns, ranges, and what it cannot be."""
    table = read_table(path)
    require_columns(table, RTDATA_COLUMNS, path)
    rows = table.rows
    if not rows:
        raise AnalysisError(f"{path} has no data rows")
    catalogue = dict(stations or {})

    declared = load_artifact(artifact_path)
    check_variable_mapping(declared)

    variables = sorted({row[RT_VARIABLE] for row in rows})
    missing_limits = sorted(set(variables) - set(PHYSICAL_LIMITS))
    if missing_limits:
        raise AnalysisError(
            f"{path} carries the variable(s) {', '.join(missing_limits)} with no entry in "
            f"PHYSICAL_LIMITS; a range check that skips them checks nothing"
        )

    variables_by_station: dict[str, set[str]] = {}
    stations_by_variable: dict[str, set[str]] = {}
    values_by_variable: dict[str, list[float]] = {variable: [] for variable in variables}
    instants_by_station_day: dict[tuple[str, str], set[str]] = {}
    violations: list[LimitViolation] = []
    humidity_boundary: list[LimitViolation] = []
    pressure_rows: list[tuple[float, str]] = []

    for line, row in enumerate(rows, start=2):
        station = row[RT_STATION]
        variable = row[RT_VARIABLE]
        value = number(row[RT_VALUE], column=RT_VALUE, line=line)
        info = catalogue.get(station, StationInfo(name="", latitude="", longitude=""))
        variables_by_station.setdefault(station, set()).add(variable)
        stations_by_variable.setdefault(variable, set()).add(station)
        values_by_variable[variable].append(value)
        day = date_of(row[RT_INSTANT]) or ""
        instants_by_station_day.setdefault((station, day), set()).add(row[RT_INSTANT])
        limit = PHYSICAL_LIMITS[variable]
        if value < limit.low or value > limit.high:
            violations.append(
                LimitViolation(
                    variable=variable, value=value, station=station, station_name=info.name
                )
            )
        if variable == "HUMEDAD_REL" and value == limit.low:
            humidity_boundary.append(
                LimitViolation(
                    variable=variable, value=value, station=station, station_name=info.name
                )
            )
        if variable == "PRESION_ATM":
            pressure_rows.append((value, station))

    at_bound = {
        variable: sum(
            1 for value in values_by_variable[variable] if value == PHYSICAL_LIMITS[variable].low
        )
        for variable in variables
    }

    required = frozenset(VARIABLE_FEATURES)
    artifact_order = [
        variable
        for feature in declared
        for variable, mapped in VARIABLE_FEATURES.items()
        if mapped == feature
    ]
    ordered_variables = artifact_order + [
        variable for variable in variables if variable not in artifact_order
    ]

    profiles: list[VariableProfile] = []
    for variable in ordered_variables:
        values = values_by_variable[variable]
        limit = PHYSICAL_LIMITS[variable]
        feature = VARIABLE_FEATURES.get(variable, "")
        artifact_low = declared[feature][1] if feature else None
        artifact_high = declared[feature][2] if feature else None
        artifact_outside = None
        if artifact_low is not None and artifact_high is not None:
            artifact_outside = sum(
                1 for value in values if value < artifact_low or value > artifact_high
            )
        profiles.append(
            VariableProfile(
                variable=variable,
                feature=feature,
                readings=len(values),
                stations=len(stations_by_variable[variable]),
                low=min(values),
                high=max(values),
                outside=sum(1 for value in values if value < limit.low or value > limit.high),
                at_bound=at_bound[variable],
                limit_low=limit.low,
                limit_high=limit.high,
                basis=limit.basis,
                artifact_low=artifact_low,
                artifact_high=artifact_high,
                artifact_outside=artifact_outside,
            )
        )

    temperature_violations = 0
    for station in variables_by_station:
        station_values = {
            row[RT_VARIABLE]: number(row[RT_VALUE], column=RT_VALUE, line=0)
            for row in rows
            if row[RT_STATION] == station
        }
        temperatures = {"TEMP_MIN", "TEMP_MED", "TEMP_MAX"}
        if temperatures <= set(station_values) and not (
            station_values["TEMP_MIN"] <= station_values["TEMP_MED"] <= station_values["TEMP_MAX"]
        ):
            temperature_violations += 1

    coverage_histogram: dict[int, int] = {}
    for station_variables in variables_by_station.values():
        covered = len(set(station_variables) & required)
        coverage_histogram[covered] = coverage_histogram.get(covered, 0) + 1

    lowest_pressures = tuple(
        PressureReading(
            value=value,
            station=station,
            station_name=catalogue.get(station, StationInfo("", "", "")).name,
            latitude=catalogue.get(station, StationInfo("", "", "")).latitude,
            longitude=catalogue.get(station, StationInfo("", "", "")).longitude,
            standard_height=standard_atmosphere_height(value),
        )
        for value, station in sorted(pressure_rows)[:3]
    )

    return RtdataAnalysis(
        facts=file_facts(path, table),
        stations=len({row[RT_STATION] for row in rows}),
        variables=len(variables),
        instants=tuple(sorted({row[RT_INSTANT] for row in rows})),
        max_instants_per_station_day=max(
            (len(seen) for seen in instants_by_station_day.values()), default=0
        ),
        empty_columns=tuple(
            column for column in table.columns if all(not row[column].strip() for row in rows)
        ),
        constant_columns=tuple(
            column
            for column in table.columns
            if len({row[column] for row in rows}) == 1 and any(row[column].strip() for row in rows)
        ),
        profiles=tuple(profiles),
        duplicates=duplicate_profile(
            rows, key=(RT_STATION, RT_VARIABLE), value=RT_VALUE, order=RT_VERSION_INSTANT
        ),
        stations_all_seven=coverage_histogram.get(len(required), 0),
        coverage_histogram=tuple(sorted(coverage_histogram.items(), reverse=True)),
        violations=tuple(
            sorted(violations, key=lambda entry: (entry.variable, -entry.value, entry.station))
        ),
        humidity_boundary=tuple(humidity_boundary),
        lowest_pressures=lowest_pressures,
        temperature_ordering_violations=temperature_violations,
    )


# ---------------------------------------------------------------------------
# The whole analysis
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class Analysis:
    dashboard: DashboardAnalysis
    emas: EmasAnalysis
    rtdata: RtdataAnalysis
    artifact: Mapping[str, tuple[str, float, float]]

    @property
    def required_variables(self) -> frozenset[str]:
        return frozenset(VARIABLE_FEATURES)


def analyze(
    dashboard_path: Path,
    emas_path: Path,
    rtdata_path: Path,
    *,
    artifact_path: Path = DEFAULT_ARTIFACT,
) -> Analysis:
    """Run the three analyses, in the order the cross-file checks need."""
    declared = load_artifact(artifact_path)
    check_variable_mapping(declared)
    emas_table = read_table(emas_path)
    require_columns(emas_table, EMAS_COLUMNS, emas_path)
    catalogue = {
        row[EMA_ID]: StationInfo(name=row[EMA_NAME], latitude=row[EMA_LAT], longitude=row[EMA_LON])
        for row in emas_table.rows
    }
    rtdata_table = read_table(rtdata_path)
    require_columns(rtdata_table, RTDATA_COLUMNS, rtdata_path)
    readings: dict[str, set[str]] = {}
    for row in rtdata_table.rows:
        readings.setdefault(row[RT_STATION], set()).add(row[RT_VARIABLE])
    return Analysis(
        dashboard=analyze_dashboard(dashboard_path),
        emas=analyze_emas(
            emas_path,
            readings={station: frozenset(seen) for station, seen in readings.items()},
            required=frozenset(VARIABLE_FEATURES),
        ),
        rtdata=analyze_rtdata(rtdata_path, stations=catalogue, artifact_path=artifact_path),
        artifact=declared,
    )


# ---------------------------------------------------------------------------
# Report
# ---------------------------------------------------------------------------


def _fmt(value: float | None, digits: int = 3) -> str:
    return "n/a" if value is None else f"{value:.{digits}f}"


def _pct(numerator: int, denominator: int) -> str:
    return f"{rate(numerator, denominator):.2%}"


def _table(headers: Sequence[str], rows: Iterable[Sequence[Any]]) -> list[str]:
    lines = [
        "| " + " | ".join(str(header) for header in headers) + " |",
        "| " + " | ".join("---" for _ in headers) + " |",
    ]
    for row in rows:
        lines.append("| " + " | ".join(str(cell) for cell in row) + " |")
    return lines


def _matrix(columns: Sequence[str], matrix: Sequence[Sequence[float | None]]) -> list[str]:
    rows = [[name] + [_fmt(value) for value in matrix[index]] for index, name in enumerate(columns)]
    return _table([""] + list(columns), rows)


def _pair(
    columns: Sequence[str],
    matrix: Sequence[Sequence[float | None]],
    left: str,
    right: str,
) -> str:
    return _fmt(matrix[columns.index(left)][columns.index(right)])


def _comuna_labels(emas: EmasAnalysis, cuts: Sequence[str]) -> str:
    """Render comuna CUTs as "Name (CUT)" for the report."""
    names = {entry.cut: entry.name for entry in emas.valparaiso_comunas}
    return ", ".join(f"{names.get(cut, '?')} ({cut})" for cut in cuts)


def _report_header(analysis: Analysis) -> list[str]:
    return (
        [
            "# Analysis of the delivered data",
            "",
            "This report is generated by `backend/training/analyze_delivered.py` and committed,",
            "the same way `REPORT-delivered-data.md` is. Its inputs are the three untracked",
            "files under `data/`, so it is not rebuilt in CI; re-run",
            "",
            "    python backend/training/analyze_delivered.py",
            "",
            "and diff. A hand edit is a defect, not a shortcut: every number below is computed",
            "from the bytes in the table under it.",
            "",
            "**What this is not.** It is not the model comparison. `train_delivered.py` already",
            "trained two models on the dashboard and measured them against two baselines, and",
            "`REPORT-delivered-data.md` is that measurement. This report asks the prior",
            "question, file by file: what each delivered file contains, what it cannot support,",
            "and which concrete dataset feature would fix it.",
            "",
            "## The three files",
            "",
        ]
        + _table(
            ["File", "sha256", "Bytes", "Rows", "Columns"],
            [
                [
                    f"`{facts.name}`",
                    f"`{facts.sha256}`",
                    facts.size_bytes,
                    facts.rows,
                    facts.columns,
                ]
                for facts in (
                    analysis.dashboard.facts,
                    analysis.emas.facts,
                    analysis.rtdata.facts,
                )
            ],
        )
        + [
            "",
            "All three were handed over directly. None carries a licence, a citation or a",
            "metadata row, and the catalogue and the snapshot carry no declared unit for any",
            "measurement. That absence is recorded here instead of being filled in: a unit",
            "guessed by the reader is the defect this analysis exists to find.",
            "",
        ]
    )


def _dashboard_section(analysis: Analysis) -> list[str]:
    dashboard = analysis.dashboard
    facts = dashboard.facts
    lines = [
        f"## 1. `{facts.name}` -- a season-to-date counter, one row per region-day",
        "",
        "### Structure",
        "",
    ]
    lines += _table(
        ["Measure", "Value"],
        [
            ["Source rows", facts.rows],
            ["Columns", facts.columns],
            ["Regions", len(dashboard.regions)],
            [
                "Dates",
                f"{len(dashboard.dates)} ({dashboard.dates[0]} .. {dashboard.dates[-1]})",
            ],
            ["Numeric columns", len(dashboard.columns)],
            ["Weather measurement columns", dashboard.weather_columns],
        ],
    )
    lines += [
        "",
        (
            f"`{COL_ID}` has {facts.rows - dashboard.duplicate_ids} distinct values over "
            f"{facts.rows} rows, and `{COL_CREACION}` repeats `{COL_FECHA}` on "
            f"{facts.rows - dashboard.creation_mismatches} of {facts.rows} rows, so neither "
            "is a snapshot instant: the file carries no export timestamp at all."
        ),
        "",
        "### The label",
        "",
        f"`{COL_ACT}` is a season-cumulative count, so the only observable that says a fire",
        "advanced on a region-day is its day-to-day difference. The label derived here is",
        "that difference, and a region-day is positive when it is greater than zero.",
        "",
    ]
    lines += _table(
        ["Measure", "Value"],
        [
            ["Positive region-days", dashboard.positives],
            ["Positive rate", f"{dashboard.positive_rate:.2%}"],
            ["Incidents behind those positives", dashboard.incidents],
            [
                "Region-days with no predecessor (label unknown, not zero)",
                dashboard.first_day_rows,
            ],
            [
                f"First-day rows whose `{COL_ACT}` was not zero",
                dashboard.first_day_nonzero,
            ],
            [f"Downward moves of `{COL_ACT}`", dashboard.act_decrements],
            [f"Downward moves of `{COL_SUP_ACT}`", dashboard.sup_decrements],
        ],
    )
    lines += [
        "",
        f"The label is binary per region-day, so {dashboard.positives} positive days carry",
        f"{dashboard.incidents} incident increments: one day can advance the counter by more",
        "than one. Every positive in this file comes from an advance of the same counter, so",
        "the label is never an independent measurement -- it is the counter's own first",
        "difference, which is why `train_delivered.py` forbids the counter as a feature.",
        "",
        "### Positives by region",
        "",
    ]
    lines += _table(
        ["Region", "Positive region-days", "Share of that region's days"],
        [
            [region, count, _pct(count, dict(dashboard.days_by_region)[region])]
            for region, count in sorted(
                dashboard.positives_by_region, key=lambda item: (-item[1], item[0])
            )
        ],
    )
    lines += ["", "### Positives by month", ""]
    lines += _table(
        ["Month", "Region-days", "Positive region-days", "Rate"],
        [
            [month, rows, positives, _pct(positives, rows)]
            for month, rows, positives in dashboard.month_rows
        ],
    )
    lines += [
        "",
        "The split is the one `train_delivered.py` uses -- every month before the last is",
        "training data, the last is the test month -- so the base-rate shift below is the",
        "shift that rehearsal had to live with.",
        "",
    ]
    lines += _table(
        ["Side", "Months", "Region-days", "Positive region-days", "Base rate"],
        [
            [
                "Train",
                ", ".join(dashboard.train_months),
                dashboard.train_rows,
                dashboard.train_positives,
                f"{dashboard.train_rate:.2%}",
            ],
            [
                "Test",
                ", ".join(dashboard.test_months),
                dashboard.test_rows,
                dashboard.test_positives,
                f"{dashboard.test_rate:.2%}",
            ],
        ],
    )
    ratio = dashboard.base_rate_ratio
    ratio_text = "n/a" if ratio is None else f"{ratio:.1f}x"
    lines += [
        "",
        f"The base rate is **{ratio_text} higher** in the month the model is judged on.",
        "A threshold selected on the training months therefore under-alerts in the test",
        "month, and that is a property of the delivered window, not of a modelling choice.",
        "",
        "### Coverage",
        "",
    ]
    lines += _table(
        ["Measure", "Value"],
        [
            ["Expected region-days (regions x dates)", dashboard.expected_cells],
            ["Present region-days", dashboard.present_cells],
            ["Missing region-days", dashboard.missing_cells],
            ["Duplicated region-days", dashboard.duplicate_cells],
            [
                "Days per region (min / max)",
                f"{dashboard.days_per_region_min} / {dashboard.days_per_region_max}",
            ],
        ],
    )
    complete = dashboard.missing_cells == 0 and dashboard.duplicate_cells == 0
    lines += [
        "",
        (
            "Every region has every day: the grid is complete and no cell is claimed twice,"
            if complete
            else (
                "The grid is **not** complete: the missing and duplicated counts above are "
                "the size of the gap."
            )
        ),
        "so the file's own coverage is not the limit -- its resolution is. One row per",
        "region-day carries no comuna, no event identity and no weather.",
        "",
        "### The cumulative counters",
        "",
        (
            f"`{COL_ACT}` never moved backwards: {dashboard.act_decrements} decrements "
            f"across all {len(dashboard.regions)} regions."
        ),
        (
            f"`{COL_SUP_ACT}` did, on {dashboard.sup_decrements} rows. Each one is listed "
            "rather than clamped, because a cumulative area that shrinks is a revision of "
            "an earlier number, and a reader that clamps it silently loses the revision:"
        ),
        "",
    ]
    if dashboard.revisions:
        lines += _table(
            ["Region", "From", "To", "Previous `sup_afec_act`", "Revised `sup_afec_act`"],
            [
                [
                    revision.region,
                    revision.previous_date,
                    revision.date,
                    f"{revision.previous:.2f}",
                    f"{revision.current:.2f}",
                ]
                for revision in dashboard.revisions
            ],
        )
        lines += [
            "",
            "The revised values are the delivered ones; this report does not correct them.",
            "It records that the same column mixes two questions -- how much burned by this",
            "date, and how much the registry currently attributes to that date -- and that a",
            "model trained on its differences learns the second as if it were the first.",
            "",
        ]
    else:
        lines += ["No downward move was found in this file.", ""]

    lines += [
        "### Correlation",
        "",
        "Pearson's r over the panel, one observation per region-day:",
        "",
    ]
    lines += _matrix(dashboard.columns, dashboard.panel_correlations)
    lines += [
        "",
        "The same columns summed across regions for each date, one observation per day:",
        "",
    ]
    lines += _matrix(dashboard.columns, dashboard.national_correlations)
    lines += [""]
    if dashboard.panel_strongest is not None:
        left, right, value = dashboard.panel_strongest
        lines += [
            f"The strongest pair on the panel is `{left}` with `{right}` at r = {value:.3f}.",
        ]
    ant_prom = _pair(dashboard.columns, dashboard.panel_correlations, COL_ANT, COL_PROM)
    ant_prom_national = _pair(dashboard.columns, dashboard.national_correlations, COL_ANT, COL_PROM)
    act_ant = _pair(dashboard.columns, dashboard.panel_correlations, COL_ACT, COL_ANT)
    act_prom = _pair(dashboard.columns, dashboard.panel_correlations, COL_ACT, COL_PROM)
    act_ant_national = _pair(dashboard.columns, dashboard.national_correlations, COL_ACT, COL_ANT)
    act_prom_national = _pair(dashboard.columns, dashboard.national_correlations, COL_ACT, COL_PROM)
    lines += [
        "",
        (
            f"**The collinearity that matters.** `{COL_ANT}` and `{COL_PROM}` are "
            f"near-duplicates of each other: r = {ant_prom} on the panel and "
            f"{ant_prom_national} on the national daily total. Both are climatology "
            f"sampled at the same calendar date, so they answer one question twice. "
            f"`{COL_ACT}` tracks them less closely on the panel ({act_ant} and "
            f"{act_prom}) because it also carries how much each region has burned, but at "
            f"the national resolution the three move almost together ({act_ant_national} "
            f"and {act_prom_national}), which is what a season clock looks like when the "
            f"regions are summed."
        ),
        "",
        "A hand profile of this file reported the two climatology columns correlating about",
        "0.94 with each other and about 0.93 with the cumulative. Neither matrix above",
        "reproduces that pair of numbers, and the report does not repeat them: the panel",
        f"matrix gives {ant_prom} between the climatology columns and {act_ant} / {act_prom}",
        f"with the cumulative, and the national matrix gives {ant_prom_national} and",
        f"{act_ant_national} / {act_prom_national}. The bytes are the authority here; the",
        "difference is reported rather than smoothed over.",
        "",
        "### Verdict",
        "",
        "**Verdict: this file locates fires in time at region granularity and nothing finer.**",
        (
            f"It provides a defensible region-day label (the counter's advance) at a "
            f"complete {len(dashboard.regions)} x {len(dashboard.dates)} grid, and "
            f"{len(dashboard.columns) - 1} other numeric columns that mostly restate how "
            f"far into the season the calendar has moved. It cannot answer a question "
            f"about a comuna, a weather day or an individual fire, and at this resolution "
            f"the only thing it can teach a model is the season. "
            f"`REPORT-delivered-data.md` records what that was worth."
        ),
        "",
    ]
    return lines


def _emas_section(analysis: Analysis) -> list[str]:
    emas = analysis.emas
    facts = emas.facts
    lines = [
        f"## 2. `{facts.name}` -- the station catalogue, one row per station",
        "",
        "### Structure",
        "",
    ]
    lines += _table(
        ["Measure", "Value"],
        [
            ["Source rows", facts.rows],
            ["Stations (`ema_ia_id` unique)", emas.stations],
            ["Active (`ema_l_estacionactiva = true`)", emas.active],
            ["Inactive", emas.inactive],
            ["Region labels", len(emas.region_labels)],
            ["Rows with no region label", emas.blank_region_rows],
            ["Comunas (padded CUT)", emas.comunas],
            ["CUTs that needed a leading zero", emas.padded_cuts],
            ["CUTs that are not a valid CUT", len(emas.invalid_cuts)],
            ["`ema_c_codigo` non-empty", emas.codes_nonempty],
            ["`ema_c_codigo` distinct values", emas.codes_distinct],
            ["Active stations with no row in the snapshot", emas.active_without_readings],
        ],
    )
    lines += [
        "",
        (
            f"`{EMA_CODE}` is not a station key: {emas.codes_nonempty} non-empty values "
            f"cover only {emas.codes_distinct} distinct codes, so the same code is shared "
            "by many stations. `ema_ia_id` is the key the snapshot uses and the only one "
            "this analysis joins on."
        ),
        "",
    ]
    lines += _table(
        ["Region label", "Stations", "Active", "Comunas"],
        [
            [
                label if label else "(no label)",
                stations,
                active,
                comunas,
            ]
            for (label, stations), (_, active), (_, comunas) in zip(
                emas.stations_by_region, emas.active_by_region, emas.comunas_by_region
            )
        ],
    )
    lines += [
        "",
        "The catalogue has no reading in it -- it is metadata. What it does carry is the",
        "station-to-comuna key, which is the only one of the three files that does.",
        "",
        "### The Valparaíso coverage split",
        "",
        (
            f"Region 05 has **{emas.valparaiso_comuna_count} comunas** in the tracked DPA "
            f"layer. The catalogue lists {emas.valparaiso_stations} stations there, "
            f"{emas.valparaiso_active} of them active."
        ),
        "",
        "The rule, applied by `coverage_split` and never as a constant:",
        "",
        (
            f"* **complete** -- at least one active station reports all "
            f"{len(analysis.required_variables)} variables `risk` needs;"
        ),
        "* **partial** -- the comuna has an active station, but none reports all of them;",
        "* **none** -- no active station in the comuna.",
        "",
        "An inactive station is not counted, because it carries no row in the snapshot: its",
        "comuna has no reading today whatever its status says.",
        "",
    ]
    lines += _table(
        ["Classification", "Comunas", "Of"],
        [
            ["Complete", len(emas.valparaiso.complete), emas.valparaiso.total],
            ["Partial", len(emas.valparaiso.partial), emas.valparaiso.total],
            ["None", len(emas.valparaiso.none), emas.valparaiso.total],
            ["Total", emas.valparaiso.total, emas.valparaiso_comuna_count],
        ],
    )
    lines += [
        "",
        (
            "Counting any listed station instead of an active one moves "
            f"{len(emas.valparaiso_rule_difference)} "
            f"{'comuna' if len(emas.valparaiso_rule_difference) == 1 else 'comunas'} "
            f"from none to partial: "
            f"{_comuna_labels(emas, emas.valparaiso_rule_difference) or 'none'}. Under "
            f"that looser rule the split is {len(emas.valparaiso_loose.complete)} complete "
            f"/ {len(emas.valparaiso_loose.partial)} partial / "
            f"{len(emas.valparaiso_loose.none)} none."
        ),
        "",
    ]
    lines += _table(
        [
            "CUT",
            "Comuna",
            "Stations",
            "Active",
            f"Best station's variables (of {len(analysis.required_variables)})",
            "Class",
        ],
        [
            [
                entry.cut,
                entry.name,
                entry.stations,
                entry.active,
                entry.best_variables,
                entry.classification,
            ]
            for entry in emas.valparaiso_comunas
        ],
    )
    none_comunas = [entry for entry in emas.valparaiso_comunas if entry.classification == "none"]
    empty_comunas = [entry for entry in none_comunas if entry.stations == 0]
    inactive_comunas = [entry for entry in none_comunas if entry.stations > 0]
    no_station_clause = (
        f"{len(empty_comunas)} of them have no station at all in the catalogue "
        f"({', '.join(entry.name for entry in empty_comunas) or 'none'}). "
    )
    inactive_clause = ""
    if inactive_comunas:
        other_verb = "has" if len(inactive_comunas) == 1 else "have"
        inactive_clause = (
            f"The other {len(inactive_comunas)} {other_verb} only inactive stations "
            f"({', '.join(f'{entry.name} ({entry.cut})' for entry in inactive_comunas)}). "
        )
    lines += [
        "",
        (
            f"Read the {len(none_comunas)} `none` rows as the procurement list. "
            + no_station_clause
            + inactive_clause
            + "Nothing in either group reports today."
        ),
        "",
        "### The earliest reading",
        "",
    ]
    lines += _table(
        ["Measure", "Value"],
        [
            ["Earliest `primera_lectura`", emas.earliest_date],
            ["Stations whose first reading is exactly that date", emas.at_floor],
            ["Stations with an empty `primera_lectura`", emas.empty_first_reading],
            ["Stations with a first reading after that date", emas.stations_with_first_reading],
            ["Latest `ultima_lectura` date", emas.latest_reading_date],
        ],
    )
    lines += [
        "",
        (
            f"Nothing in the network is dated before **{emas.earliest_date}**, and "
            f"{emas.at_floor} stations start at exactly that midnight. A first reading "
            f"shared by {emas.at_floor} of {emas.stations} stations is a migration epoch, "
            "not a coincidence: the date is the oldest value the system can hold, so it "
            "cannot be read as evidence that those stations were installed then. The honest "
            "statement is the one the numbers make -- there is no reading before that date "
            "-- and not 'the network starts in 2010'."
        ),
        "",
        "### The identifier chain: `com_ia_id` -> CUT",
        "",
        "The column is the Chilean CUT stored without its leading zero for regions 01..09,",
        f"so {emas.padded_cuts} of {emas.stations} values are four digits. The chain this",
        "analysis applies is:",
        "",
        "1. pad to five digits;",
        "2. the region part must be 01..16 and the comuna part must not be 000;",
        "3. the region label must agree with the region part;",
        "4. the comuna must exist in the tracked DPA layer, for the two regions that",
        "   repository carries a map for.",
        "",
    ]
    lines += _table(
        ["Step", "Result"],
        [
            ["Padded to five digits", emas.padded_cuts],
            ["Structurally valid CUTs", emas.stations - len(emas.invalid_cuts)],
            ["Structurally invalid CUTs", len(emas.invalid_cuts)],
            ["Rows whose region label disagrees with the CUT prefix", emas.prefix_mismatch_rows],
        ],
    )
    lines += [""]
    if emas.prefix_mismatches:
        lines += _table(
            ["Region label", "CUT prefix", "Stations", "Example station"],
            [
                [entry.region, entry.prefix, entry.stations, entry.example]
                for entry in emas.prefix_mismatches
            ],
        )
        lines += [
            "",
            "The prefix each label should have is declared in `REGION_CUT_PREFIX`; a label",
            "missing from that table is a hard failure, not a skip. The rows above are the",
            "ones where the delivered label and the delivered code disagree. The pattern is",
            "the one the split dates explain -- Ñuble was part of region 08 until 2018 and",
            "Los Ríos part of region 10 until 2007 -- so the most likely reading is that the",
            "codes are the older numbering and the label was updated. That is an",
            "interpretation, and the effect is the same under either one: a join on",
            "`com_ia_id` without re-coding puts those stations in another region.",
            "",
        ]
    if emas.dpa_checked:
        lines += [
            "Membership in the tracked DPA layer:",
            "",
        ]
        lines += _table(
            ["Region", "Comunas with stations", "Found in the DPA layer", "Not found"],
            [
                [label, present, known, ", ".join(unknown) if unknown else "--"]
                for label, known, present, unknown in emas.dpa_checked
            ],
        )
        checked = [entry[0] for entry in emas.dpa_checked]
        unchecked = len([label for label in emas.region_labels if label]) - len(checked)
        lines += [
            "",
            (
                f"Only {', '.join(checked)} are checked against a comuna registry, because "
                f"that is what this repository carries a DPA map for. For the other "
                f"{unchecked} labelled regions, step 3 above is the whole check -- and that "
                f"is stated instead of being implied."
            ),
            "",
        ]
    lines += [
        "### Verdict",
        "",
        "**Verdict: `emas` is a station catalogue and the only comuna key among the three",
        "files, and it is usable as a coverage map only after two repairs.** It says where",
        "the stations are, which of them are active and which comuna each one serves; it",
        "carries no reading, no elevation, no licence and no unit. The repairs are the",
        "pre-split CUTs above -- a join on the raw column moves stations between regions --",
        "and the inactive station that removes a comuna from the map. Even repaired, the map",
        'answers "is there a station here", not "is there a measurement today".',
        "",
    ]
    return lines


def _rtdata_section(analysis: Analysis) -> list[str]:
    rtdata = analysis.rtdata
    emas = analysis.emas
    facts = rtdata.facts
    required = len(analysis.required_variables)
    lines = [
        f"## 3. `{facts.name}` -- one instant, not a series",
        "",
        "### Structure",
        "",
    ]
    lines += _table(
        ["Measure", "Value"],
        [
            ["Source rows", facts.rows],
            ["Columns", facts.columns],
            ["Stations", rtdata.stations],
            ["Variables", rtdata.variables],
            ["Distinct `rtd_dt_marcatiempo` values", rtdata.instant_count],
            ["The instant", rtdata.instants[0] if rtdata.instants else "none"],
            [
                "Widest coverage of one day by one station (instants)",
                rtdata.max_instants_per_station_day,
            ],
            [f"Stations reporting all {required} variables", rtdata.stations_all_seven],
            ["Stations in the catalogue with no row here", emas.stations - rtdata.stations],
        ],
    )
    lines += f"""
The station set is the active half of the catalogue: every station with a row here is
marked active in `emas`, and {emas.stations - rtdata.stations} catalogue stations have no
row. {rtdata.stations_all_seven} of the {rtdata.stations} stations report all {required}
variables; the rest carry a subset.
""".splitlines()

    lines += ["", "### Readings and stations per variable", ""]
    lines += _table(
        ["Variable", "`risk` feature", "Readings", "Stations", "Min", "Max"],
        [
            [
                profile.variable,
                profile.feature or "--",
                profile.readings,
                profile.stations,
                _fmt(profile.low),
                _fmt(profile.high),
            ]
            for profile in rtdata.profiles
        ],
    )
    lines += [
        "",
        "The seven feature-bearing variables come first, in the artifact's own order; the",
        "rest follow alphabetically. `HORAS_FRIO` and `GRADOS_DIA` are the two every station",
        "reports, and neither is a weather observation: they are derived quantities whose",
        "aggregation this file does not declare.",
        "",
        "### Duplicates",
        "",
    ]
    duplicates = rtdata.duplicates
    lines += _table(
        ["Measure", "Value"],
        [
            ["Rows", duplicates.rows],
            ["Unique (station, variable) keys", duplicates.unique_keys],
            ["Extra rows", duplicates.extra_rows],
            ["Rows inside a repeated group", duplicates.rows_in_groups],
            ["Stations with a repeated group", duplicates.stations_affected],
            ["Groups whose copies disagree on the value", duplicates.conflicting_groups],
        ],
    )
    lines += ["", "Rows per group and the number of groups of that size:", ""]
    lines += _table(
        ["Rows in the group", "Groups"],
        [[size, groups] for size, groups in duplicates.groups_by_size],
    )
    lines += f"""
The copies do not share `rtd_dt_versiondt`: they are the same reading re-inserted by a
later ingestion batch, so the table holds {duplicates.rows} rows for {duplicates.unique_keys}
observations. A reader that takes the first row returns a stale value in
{duplicates.conflicting_groups} groups; the rule has to be the latest `rtd_dt_versiondt`
per (station, variable), and the count above is the size of the mistake that rule avoids.
""".splitlines()
    if duplicates.conflicting_example[0]:
        station, variable, values = duplicates.conflicting_example
        rendered = ", ".join(f"{value:.3f}" for value in values)
        lines += [
            "",
            (
                f"Example: station `{station}` on `{variable}` carries the values "
                f"{rendered} at different version instants, and only one of them is the "
                f"reading to keep."
            ),
        ]

    empty = rtdata.empty_columns
    lines += [
        "",
        "### Empty and constant columns",
        "",
    ]
    lines += _table(
        ["Kind", "Columns"],
        [
            ["Empty on every row", ", ".join(f"`{name}`" for name in empty) or "--"],
            [
                "One value on every row",
                ", ".join(f"`{name}`" for name in rtdata.constant_columns) or "--",
            ],
        ],
    )
    lines += f"""
`{RT_QUALITY}` and `{RT_CHECKS}` are among the empty ones: both quality columns are
blank on all {facts.rows} rows. A quality flag that is empty on every row is not a
quality flag, it is a column that exists -- and any rule that treats "no flag" as "no
problem" reads those {facts.rows} rows as clean.
""".splitlines()

    lines += ["", "### Value ranges against physical limits", ""]
    lines += _table(
        ["Variable", "Physical bound", "Outside", "At the bound", "Why this bound"],
        [
            [
                profile.variable,
                f"{_fmt(profile.limit_low, 1)} .. {_fmt(profile.limit_high, 1)}",
                profile.outside,
                profile.at_bound,
                profile.basis,
            ]
            for profile in rtdata.profiles
        ],
    )
    lines += [""]
    radiation = [entry for entry in rtdata.violations if entry.variable == "RADIACION_SOL"]
    if radiation:
        values = sorted({f"{entry.value:.2f}" for entry in radiation})
        stations = sorted({f"{entry.station} ({entry.station_name})" for entry in radiation})
        lines += f"""\
**Radiation.** {len(radiation)} readings sit above the solar constant of {SOLAR_CONSTANT:.0f}
W/m2, at {", ".join(stations)}: {", ".join(values)}. Surface global irradiance cannot
exceed the irradiance at the top of the atmosphere, so these are sensor or scaling
errors, whatever the timestamp says.
""".splitlines()
    boundary = rtdata.humidity_boundary
    if boundary:
        stations = ", ".join(f"{entry.station} ({entry.station_name})" for entry in boundary)
        verb = "sits" if len(boundary) == 1 else "sit"
        lines += [
            "",
            (
                f"**Humidity.** {len(boundary)} reading {verb} at exactly "
                f"{_fmt(boundary[0].value, 2)} %, at {stations}. A relative humidity of "
                f"0.00 % is not a measurement a coastal station makes; it is the floor a "
                f"sensor reports when it has nothing to report."
            ),
        ]
    if rtdata.lowest_pressures:
        lowest = rtdata.lowest_pressures[0]
        lines += [
            "",
            (
                f"**Pressure.** No reading falls outside the declared physical bound, and "
                f"the lowest is {lowest.value:.2f} hPa at station {lowest.station} "
                f"({lowest.station_name}), at {lowest.latitude}, {lowest.longitude}. The "
                f"standard atmosphere reaches that pressure at about "
                f"{lowest.standard_height:.0f} m, which is what the coordinates imply for "
                f"the altiplano: it is a plausible **station** pressure, not a defective "
                f"one. The defect is the missing elevation column and the missing "
                f"reduction convention -- in one column, a station at 4 000 m and one at "
                f"sea level cannot be compared."
            ),
        ]
    evapotranspiration = next(
        (profile for profile in rtdata.profiles if profile.variable == "EVAPOTRANSPIRACION"),
        None,
    )
    if evapotranspiration is not None:
        lines += [
            "",
            (
                f"**Evapotranspiration.** {evapotranspiration.outside} of "
                f"{evapotranspiration.readings} readings sit outside a daily ET0 bound, "
                f"with a maximum of {_fmt(evapotranspiration.high)}. The column declares no "
                f"unit and no period, and a value in the hundreds is an accumulated "
                f"quantity, not a daily one: it cannot be used until the aggregation is "
                f"declared."
            ),
        ]
    derived = [
        profile for profile in rtdata.profiles if profile.variable in {"HORAS_FRIO", "GRADOS_DIA"}
    ]
    if derived:
        rendered = "; ".join(
            f"`{profile.variable}` runs {_fmt(profile.low)} to {_fmt(profile.high)}"
            for profile in derived
        )
        lines += [
            "",
            (
                f"**Derived quantities.** {rendered}. A daily cold-hours total that never "
                f"exceeds one, and a degree-day total that never exceeds one either, are "
                f"not daily totals in the units their names claim; the aggregation is "
                f"undeclared and the columns are unusable as they stand."
            ),
        ]
    lines += f"""\

**Zero is a measurement here.** {
        next(profile.at_bound for profile in rtdata.profiles if profile.variable == "PRECIPITACION")
    } precipitation readings are exactly zero, and that
is a fact about the weather, not a floor. Counting values at a bound is only evidence of
a floor when the bound is physically unreachable, which is why the humidity row is
called out and the precipitation row is not.
""".splitlines()

    lines += ["", "### The single timestamp", ""]
    lines += f"""\
Every row carries the same `rtd_dt_marcatiempo`: {", ".join(rtdata.instants)}. One
instant is not a day, so `TEMP_MAX`, `TEMP_MIN` and `PRECIPITACION` cannot be daily
aggregates here, whatever the column names say -- they are one reading each. The
snapshot is a picture of the network at 09:00, and the widest a station gets is
{rtdata.max_instants_per_station_day} instant for its station-day.
""".splitlines()
    lines += ["", "### The artifact's own range does not catch it", ""]
    lines += _table(
        ["Feature", "Range declared by the artifact", "Readings outside it"],
        [
            [
                profile.feature,
                f"{_fmt(profile.artifact_low, 1)} .. {_fmt(profile.artifact_high, 1)}",
                profile.artifact_outside,
            ]
            for profile in rtdata.profiles
            if profile.feature
        ],
    )
    radiation_profile = next(
        (profile for profile in rtdata.profiles if profile.variable == "RADIACION_SOL"),
        None,
    )
    lines += [""]
    if radiation_profile is not None:
        equality = (
            "exactly the highest reading in the file"
            if (radiation_profile.artifact_high == radiation_profile.high)
            else "the observed range"
        )
        lines += f"""\
The artifact's declared maximum for `{radiation_profile.feature}` is
{_fmt(radiation_profile.artifact_high, 1)} W/m2 -- {equality}. A bound taken from the
data cannot fail on the data: every one of those readings is inside the range the model
declares, and {len(radiation)} of them are physically impossible. The physical bound in
the table above is the check that works, and it is the one the artifact should carry.
""".splitlines()
    lines += f"""\

Where the snapshot is internally consistent, say so: every station reporting
`TEMP_MIN`, `TEMP_MED` and `TEMP_MAX` keeps them ordered
({rtdata.temperature_ordering_violations} violations), so the failure here is not the
temperature column.
""".splitlines()

    lines += [
        "",
        "### Verdict",
        "",
        "**Verdict: `rtdata` is one instantaneous cross-section of the network, and it cannot",
        "support a daily label or a daily feature.** It shows which stations were reporting",
        "at one instant and with what values; it carries a single timestamp, no quality flag",
        (
            f"on any row, no unit for any variable, and "
            f"{rtdata.duplicates.extra_rows:,} extra rows for "
            f"{rtdata.duplicates.unique_keys:,} observations."
        ),
        (
            f"{len(derived)} derived variables "
            f"({', '.join(f'`{profile.variable}`' for profile in derived)}) have undeclared "
            f"aggregation, and {len(radiation)} of its {rtdata.facts.rows} readings exceed "
            f"the solar constant. What survives is a snapshot that can be used to test a "
            f"parser, never to train a model."
        ),
        "",
    ]
    return lines


def _cascade_section(analysis: Analysis) -> list[str]:
    dashboard = analysis.dashboard
    emas = analysis.emas
    rtdata = analysis.rtdata
    required = len(analysis.required_variables)
    lines = [
        "## 4. The cascade: what the product needs and which file supplies it",
        "",
        "One row per property the model's input needs. A `no` is a measured absence: the",
        "count next to it is the column, row or timestamp count that decided it.",
        "",
    ]
    rows = [
        [
            "A daily weather value for a comuna-day",
            f"no -- {dashboard.weather_columns} weather columns of {dashboard.facts.columns}",
            "no -- station metadata, no reading",
            (
                f"no -- {rtdata.instant_count} instant, at most "
                f"{rtdata.max_instants_per_station_day} reading per station-day"
            ),
            (
                f"hourly readings over every civil day for a point in each comuna, or a "
                f"daily aggregate with a declared day boundary -- {required} variables"
            ),
        ],
        [
            "Weather on days without a fire",
            "no -- the file has no weather at all",
            "no",
            "no -- one day, and it is not a labelled one",
            (
                "an archive that carries quiet days; `meteo.py` already fetches two of "
                "them, which is the only part of this need the repository can meet today"
            ),
        ],
        [
            "A fire event with a date and a comuna",
            (f"no -- {len(dashboard.regions)} region labels, no comuna, no event identity"),
            "no -- a station is not an event",
            "no",
            (
                "an event-level registry keyed by comuna CUT with a local date; the "
                "access-gated CONAF product the plan's Log L5 identifies is the only "
                "source the log names"
            ),
        ],
        [
            "Station coverage per comuna",
            "no",
            (
                f"yes -- {emas.stations} stations over {emas.comunas} comunas, with "
                f"{emas.prefix_mismatch_rows} rows whose region label and CUT disagree"
            ),
            f"yes for this instant -- {rtdata.stations} stations",
            (
                f"the post-2018 CUT on every row, and an active station in the "
                f"{len(emas.valparaiso.none)} Valparaíso comunas that have none"
            ),
        ],
        [
            "A quality flag per reading",
            "no -- no flag column",
            f"station level only -- {emas.active} active, {emas.inactive} inactive",
            (
                f"no -- {len(rtdata.empty_columns)} columns are empty on every row, "
                f"including both flag columns"
            ),
            (
                "a per-reading quality code with a documented code set, and a rule for what "
                "an absent flag means"
            ),
        ],
        [
            "A declared unit and a station elevation",
            "no unit declared; the quantities are counters",
            "no elevation column; coordinates only",
            "no unit per variable",
            (
                "a unit and an aggregation period per variable, an elevation in metres "
                "per station, and a pressure-reduction convention"
            ),
        ],
        [
            "One comuna key that joins across the files",
            "no -- region only",
            f"CUT yes, with {emas.prefix_mismatch_rows} rows to re-code",
            "station id only; joins through `emas`",
            "a single post-2018 CUT on all three",
        ],
        [
            "A time span long enough for a temporal split",
            (f"{len(dashboard.dates)} days ({dashboard.dates[0]} .. {dashboard.dates[-1]})"),
            "no -- first-reading dates, not a series",
            f"no -- {rtdata.instant_count} instant",
            "several seasons of the same panel, so a test season is not also the only one",
        ],
    ]
    lines += _table(
        ["Property", "`dashboard_conaf`", "`emas`", "`rtdata`", "What would supply it"],
        rows,
    )
    lines += [
        "",
        (
            "Only station coverage is answered `yes` by any file, and `rtdata` can confirm "
            "it for one instant only. That is the whole finding, in one table: the "
            "delivered set is a fire *count* per region-day plus a station *catalogue*, and "
            "the measurement that would turn either into a model input -- daily weather at "
            "a comuna, on every day, with a unit and a quality flag -- is in none of them."
        ),
        "",
        "### What to ask for, in order",
        "",
        "1. **An event registry keyed by comuna CUT with a local date.** One row per fire,",
        "   not per region-day: that is the only way the label stops being a counter's",
        "   first difference. The product the plan's Log L5 names is access-gated; nothing",
        "   in these three files substitutes for it.",
        "2. **Daily weather on every day, including quiet ones, at a point in each comuna.**",
        "   Hourly station readings aggregated to a declared civil day, or a reanalysis",
        "   already aggregated -- `meteo.py` shows both shapes. Without the quiet days the",
        "   features exist only where the label is 1.",
        "3. **A declared unit and an aggregation period per variable, and an elevation per",
        "   station.** The snapshot cannot be validated without them, and its pressure",
        "   column cannot be compared across the altiplano and the coast.",
        "4. **A per-reading quality flag with a documented code set.** Two empty columns are",
        "   not a quality signal; they are a schema that was never filled.",
        "5. **The post-2018 CUT on every row, with the region label consistent with it.**",
        "   Otherwise the comuna join that the whole panel and the whole product depend on",
        "   puts stations in the wrong region.",
        "",
    ]
    return lines


def render_report(analysis: Analysis) -> str:
    lines = _report_header(analysis)
    lines += _dashboard_section(analysis)
    lines += _emas_section(analysis)
    lines += _rtdata_section(analysis)
    lines += _cascade_section(analysis)
    lines += [
        "## What this analysis is not",
        "",
        "It is not a model, and it does not re-run the rehearsal. `REPORT-delivered-data.md`",
        "records what a model trained on the dashboard scored against climatology; this",
        "report records what the dashboard, the catalogue and the snapshot contain, which is",
        "the prior question. A reader who wants the score reads the other report; a reader",
        "who wants to know which delivered field to repair reads this one.",
        "",
        "It is also not a licence review. None of the three files states a licence, a",
        "citation or a retrieval URL, and this report says so rather than assigning one.",
        "",
        "The claims above are tested against trimmed fixtures of the same three files under",
        "`backend/training/tests/fixtures/delivered/`, where `PROVENANCE.md` records the",
        "exact selection rule. Those tests change the fixtures and require the numbers to",
        "move, so a number typed into this report instead of computed would fail them.",
        "",
    ]
    return "\n".join(lines).rstrip("\n") + "\n"


def write_report(path: Path, report: str) -> None:
    """Write the report with Unix line endings, so a re-run is byte-identical."""
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(report, encoding="utf-8", newline="\n")


def run(
    dashboard_path: Path = DEFAULT_DASHBOARD,
    emas_path: Path = DEFAULT_EMAS,
    rtdata_path: Path = DEFAULT_RTDATA,
    *,
    out: Path = DEFAULT_REPORT,
    artifact_path: Path = DEFAULT_ARTIFACT,
) -> tuple[Analysis, Path]:
    """Analyse the three files, write the report, and return both."""
    analysis = analyze(dashboard_path, emas_path, rtdata_path, artifact_path=artifact_path)
    write_report(out, render_report(analysis))
    return analysis, out


def summary(analysis: Analysis) -> list[str]:
    """The headline numbers a reviewer needs, printed by the CLI."""
    dashboard = analysis.dashboard
    emas = analysis.emas
    rtdata = analysis.rtdata
    return [
        (
            f"dashboard: {dashboard.facts.rows} rows, {len(dashboard.regions)} regions x "
            f"{len(dashboard.dates)} days, {dashboard.positives} positive region-days "
            f"({dashboard.positive_rate:.2%}), {dashboard.incidents} incidents, "
            f"{dashboard.sup_decrements} sup_afec_act revisions"
        ),
        (
            f"emas: {emas.stations} stations ({emas.active} active), {emas.comunas} "
            f"comunas, {emas.prefix_mismatch_rows} rows with a CUT/region disagreement"
        ),
        (
            f"valparaiso: {len(emas.valparaiso.complete)} complete / "
            f"{len(emas.valparaiso.partial)} partial / {len(emas.valparaiso.none)} none "
            f"of {emas.valparaiso_comuna_count} comunas"
        ),
        (
            f"rtdata: {rtdata.facts.rows} rows, {rtdata.stations} stations, "
            f"{rtdata.instant_count} instant, {rtdata.duplicates.extra_rows} extra rows, "
            f"{len(rtdata.violations)} readings outside a physical bound"
        ),
    ]


def main(argv: list[str] | None = None) -> int:
    """Analyse the three delivered files and write the committed report."""
    parser = argparse.ArgumentParser(
        description=(
            "Measure the three delivered data files file by file and write "
            "ANALYSIS-delivered-data.md. The report is a product: regenerate it, never "
            "edit it."
        )
    )
    parser.add_argument("--dashboard", type=Path, default=DEFAULT_DASHBOARD)
    parser.add_argument("--emas", type=Path, default=DEFAULT_EMAS)
    parser.add_argument("--rtdata", type=Path, default=DEFAULT_RTDATA)
    parser.add_argument("--artifact", type=Path, default=DEFAULT_ARTIFACT)
    parser.add_argument("--out", type=Path, default=DEFAULT_REPORT, help="report path")
    arguments = parser.parse_args(argv)
    try:
        analysis, out = run(
            arguments.dashboard,
            arguments.emas,
            arguments.rtdata,
            out=arguments.out,
            artifact_path=arguments.artifact,
        )
    except AnalysisError as error:
        print(f"error: {error}", file=sys.stderr)
        return 2
    for line in summary(analysis):
        print(line)
    print(f"wrote {display(out)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
