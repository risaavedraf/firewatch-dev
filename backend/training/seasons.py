"""Season-to-date feature aggregates, one row per (comuna, season).

This is the second half of the answer `assemble.py` ends on: the CONAF registry can
only carry meteorology on days that had a fire, so a model's daily aggregates have to
come from an independent source joined by (comuna, date). `meteo.py` fetches that
source; this module turns its daily values into the row the model is trained on.

THE WINDOW, IN CIVIL TIME. A season is named by its start year and runs from
**1 July** to **30 June** of the following year in `America/Santiago`. The feature
window runs from 1 July to a cut-off that defaults to **31 December** of the start
year, so a row is what was known at the cut-off -- season-to-date, not end-of-season.
Both bounds are zone-aware instants built from `assemble.load_zone()`:

* `Window.covers()` compares instants, so a moment at 22:00 local on 31 December is
  inside a window cut at 31 December even though its UTC date is already 1 January,
  and a moment at 23:00 local on 30 June is outside a window that opens on 1 July even
  though its UTC date is 1 July. Cut in UTC, both of those flip -- which is exactly what
  the boundary test pins.
* `Window.dates` is the list of civil dates the aggregates run over, derived from the
  local instants and cross-checked against `assemble.civil_day_window`, the same
  function `precip_sum` uses.

THE AGGREGATES, AND WHERE EACH ONE COMES FROM.

    temp_max       maximum of the daily maxima          degC
    temp_min       minimum of the daily minima          degC
    humidity_rel   MEAN of the daily values             %
    wind_speed     maximum of the daily maxima          m/s   (declared unit)
    precip_sum     accumulated over the window          mm
    radiation_sol  accumulated over the window          MJ/m2

`humidity_rel` is a mean and not a minimum, deliberately. Each source publishes ONE
value per day and that value is the day's average (Open-Meteo's parameter is called
`relative_humidity_2m_mean`; NASA POWER's `RH2M` is its daily value). The window
aggregate is therefore a mean of daily means. The minimum would be a different
quantity -- the driest day of the window -- and reducing with it here would silently
redefine the column the artifact declares.

`radiation_sol` is a daily integral in MJ/m2 on both sources, never W/m2. A mean
irradiance and a daily integral are different quantities, so `check_artifact_units`
reports the descriptor's `W/m2` as a CONFLICT instead of converting anything: the
descriptor has to change, not this module.

AN INCOMPLETE WINDOW IS ABSENT, NEVER ZERO. Every aggregate goes through
`assemble.reduce_window`, which reports `value=None` plus the coverage count when any
day of the window has no reading. So a row built from a source that answered 180 of
184 days has every feature absent with `covered_days=180`, not a smaller sum and not a
zero. This is the same rule the Go side applies to `precip_sum`, reused rather than
reimplemented: a zero is a measurement here, and absence is not one.

NOTHING HERE PICKS A SOURCE. `--source` names it, the row records it, and the
manifest records that source's licence, its day alignment and its grid. NASA POWER's
days are LST (no DST) on a 0.5 x 0.625 degree grid, so its rows say
`resolves_civil_day=False`; Open-Meteo's are the civil days of `TIMEZONE`. Mixing the
two in one panel is a decision a reviewer has to make on purpose, not something this
builder does quietly.

Run it as::

    python backend/training/seasons.py --source open-meteo \\
        --out backend/training/out/seasons --first 2015 --last 2015 --comunas 05101
"""

from __future__ import annotations

import argparse
import csv
import datetime as dt
import json
import sys
from dataclasses import dataclass
from pathlib import Path

import meteo
from assemble import (
    DEFAULT_ARTIFACT,
    Artifact,
    AssembleError,
    civil_day_window,
    load_artifact,
    load_zone,
    reduce_window,
)
from centroids import CentroidError, load_centroids
from meteo import FEATURE_UNITS, MeteoError, MeteoResponse

REPO_ROOT = Path(__file__).resolve().parents[2]

#: The civil calendar the season window is cut in; same string as `assemble.py`.
TIMEZONE = meteo.TIMEZONE

#: Seasons are named by their start year: 1985 means 1985-07-01 .. 1986-06-30. The
#: first and last season the model is built for are fixed here, not in the CLI, so
#: "the training range" is one statement.
FIRST_SEASON = 1985
LAST_SEASON = 2023
SEASON_START_MONTH = 7
SEASON_START_DAY = 1

#: The default cut-off: 31 December of the start year, so the feature window is the
#: first half of the season. Configurable, and every row records the window it used.
DEFAULT_CUTOFF = (12, 31)

#: feature -> reducer. The reducer is part of the feature: `precip_sum` is a sum, not
#: a mean of sums, and `temp_max` is a maximum, not a mean of maxima.
AGGREGATIONS = {
    "temp_max": "max",
    "temp_min": "min",
    "humidity_rel": "mean",
    "wind_speed": "max",
    "precip_sum": "sum",
    "radiation_sol": "sum",
}

#: Spelled once, for the CSV header and the manifest.
FEATURES = tuple(AGGREGATIONS)

#: Documentation that belongs next to the number a reader will question.
FEATURE_NOTES = {
    "humidity_rel": (
        "Mean of the daily values over the window, not the minimum. Each source publishes "
        "one daily value and that value is the day's average (Open-Meteo names the "
        "parameter relative_humidity_2m_mean; NASA POWER's RH2M is its daily value), so the "
        "window aggregate is a mean of daily means."
    ),
    "wind_speed": (
        "Maximum of the daily maxima, in m/s. Open-Meteo publishes the 10 m maximum in km/h "
        "and it is divided by 3.6 after the declared unit is matched; NASA POWER publishes "
        "the 2 m maximum already in m/s. Same name and same unit, measured at different "
        "heights -- recorded under divergences."
    ),
    "radiation_sol": (
        "Accumulated MJ/m2 over the window. Both sources deliver a DAILY INTEGRAL, never a "
        "mean irradiance in W/m2; the artifact descriptor still declares W/m2 and that "
        "disagreement is reported, not converted."
    ),
    "precip_sum": (
        "Accumulated mm over the window. Over a season window, not the seven civil days "
        "assemble.precipitation_sum fixes for the comuna-day panel: a season-to-date sum is "
        "a different feature from a seven-day sum and the two are never mixed."
    ),
}

PANEL_COLUMNS = (
    "cut",
    "comuna",
    "provincia",
    "season",
    "window_start",
    "window_end",
    "days_requested",
    "days_returned",
    "days_missing",
    "source",
    "day_alignment",
    "resolves_civil_day",
)


class SeasonError(Exception):
    """A failure the operator must fix; the CLI prints it instead of a traceback."""


class WindowError(ValueError):
    """A season window that is not the one the contract describes."""


# ---------------------------------------------------------------------------
# The window
# ---------------------------------------------------------------------------


def season_label(start_year: int) -> str:
    """The season's name: its start year and the year it ends in."""
    return f"{start_year}-{start_year + 1}"


def cutoff_for(start_year: int, month_day: tuple[int, int] | None = None) -> dt.date:
    """The cut-off date of one season, from a yearless `(month, day)`.

    A season straddles two calendar years, so the year is not the caller's to choose:
    a cut-off from July onwards belongs to the start year and one from January to June
    belongs to the following year. Without this, `--cutoff 03-31` would silently mean
    the PREVIOUS season's March.
    """
    month, day = month_day or DEFAULT_CUTOFF
    year = start_year if month >= SEASON_START_MONTH else start_year + 1
    return dt.date(year, month, day)


@dataclass(frozen=True)
class Window:
    """One season's feature window: local instants in, civil dates out."""

    label: str
    start_year: int
    start: dt.datetime
    end: dt.datetime
    dates: tuple[dt.date, ...]
    cutoff: dt.date

    @property
    def day_count(self) -> int:
        return len(self.dates)

    def covers(self, instant: dt.datetime) -> bool:
        """Whether an aware moment falls inside the window, compared as an INSTANT.

        The comparison is between aware datetimes, so it is timezone-correct by
        construction: 22:00 on 31 December at UTC-3 is inside, although its UTC date is
        already 1 January. A naive datetime is refused -- it names no moment.
        """
        if instant.tzinfo is None:
            raise WindowError(
                "Window.covers needs an aware datetime; a naive one has no civil date and "
                "no instant to compare"
            )
        return self.start <= instant <= self.end


def season_window(
    start_year: int,
    *,
    month_day: tuple[int, int] | None = None,
    zone=None,
) -> Window:
    """Build one season's feature window, in `TIMEZONE`.

    The start is 1 July 00:00 LOCAL and the end is the cut-off's last local instant.
    Both are aware, so no arithmetic here passes through UTC.
    """
    if start_year < FIRST_SEASON or start_year > LAST_SEASON:
        raise WindowError(
            f"season {season_label(start_year)} is outside the training range "
            f"{season_label(FIRST_SEASON)} .. {season_label(LAST_SEASON)}"
        )
    resolved_zone = zone or load_zone(TIMEZONE)
    start = dt.datetime(start_year, SEASON_START_MONTH, SEASON_START_DAY, tzinfo=resolved_zone)
    opening = cutoff_for(start_year, month_day)
    end = dt.datetime(
        opening.year, opening.month, opening.day, 23, 59, 59, 999999, tzinfo=resolved_zone
    )
    # `cutoff_for` resolves the cut-off's year from its month, so `opening` is always
    # inside the season; that property is what `test_every_legal_cutoff_lands_inside_its
    # _season` holds down, rather than a branch here that no month/day can reach.
    first = start.astimezone(resolved_zone).date()
    last = end.astimezone(resolved_zone).date()
    dates = civil_day_window(last, (last - first).days + 1)
    return Window(
        label=season_label(start_year),
        start_year=start_year,
        start=start,
        end=end,
        dates=dates,
        cutoff=opening,
    )


def season_windows(
    *,
    first: int = FIRST_SEASON,
    last: int = LAST_SEASON,
    month_day: tuple[int, int] | None = None,
    zone=None,
) -> tuple[Window, ...]:
    """Every season of the range, ascending."""
    if first > last:
        raise WindowError(f"the season range starts at {first} and ends at {last}")
    resolved_zone = zone or load_zone(TIMEZONE)
    return tuple(
        season_window(year, month_day=month_day, zone=resolved_zone)
        for year in range(first, last + 1)
    )


def parse_cutoff(text: str) -> tuple[int, int]:
    """Read a `MM-DD` cut-off, which is yearless because each season supplies its own."""
    parts = (text or "").strip().split("-")
    if len(parts) != 2 or not all(part.isdigit() for part in parts):
        raise WindowError(f"the cut-off {text!r} is not MM-DD (for example 12-31)")
    month, day = int(parts[0]), int(parts[1])
    try:
        dt.date(2000, month, day)  # a leap year, so 02-29 is a legal cut-off
    except ValueError as error:
        raise WindowError(f"the cut-off {text!r} is not a calendar date ({error})") from error
    return month, day


# ---------------------------------------------------------------------------
# Aggregation
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class SeasonFeature:
    """One aggregate, with the coverage that justifies it being present or absent."""

    feature: str
    unit: str
    value: float | None
    covered_days: int
    window_days: int
    reason: str

    @property
    def present(self) -> bool:
        return self.value is not None


@dataclass(frozen=True)
class SeasonRow:
    """One (comuna, season): the window, the coverage, the source and the aggregates."""

    cut: str
    comuna: str
    provincia: str
    season: str
    window_start: dt.date
    window_end: dt.date
    days_requested: int
    days_returned: int
    source: str
    day_alignment: str
    resolves_civil_day: bool
    features: dict[str, SeasonFeature]

    @property
    def days_missing(self) -> int:
        return self.days_requested - self.days_returned

    def as_row(self) -> list[str]:
        cells = [
            self.cut,
            self.comuna,
            self.provincia,
            self.season,
            self.window_start.isoformat(),
            self.window_end.isoformat(),
            str(self.days_requested),
            str(self.days_returned),
            str(self.days_missing),
            self.source,
            self.day_alignment,
            "true" if self.resolves_civil_day else "false",
        ]
        for feature in FEATURES:
            entry = self.features[feature]
            cells.append(format_number(entry.value))
            cells.append(str(entry.covered_days))
        return cells


def format_number(value: float | None) -> str:
    """Render a number for the CSV: empty when absent, never a zero in its place."""
    if value is None:
        return ""
    text = f"{value:.6f}".rstrip("0").rstrip(".")
    return text if text not in ("", "-0") else "0"


def aggregate(
    observations: dict[dt.date, dict[str, float | None]],
    window: Window,
) -> dict[str, SeasonFeature]:
    """Aggregate one comuna's daily observations over one window.

    Days outside the window are dropped -- a source answers for the range it was
    asked, and a wide answer must not widen the feature. A day inside the window with
    no value for a feature is simply absent for that feature, which is what makes the
    window incomplete and the aggregate absent instead of smaller.
    """
    window_dates = set(window.dates)
    inside = {date: values for date, values in observations.items() if date in window_dates}
    aggregates: dict[str, SeasonFeature] = {}
    for feature in FEATURES:
        readings: dict[dt.date, list[float]] = {}
        for date, values in inside.items():
            value = values.get(feature)
            if value is not None:
                readings.setdefault(date, []).append(float(value))
        accumulation = reduce_window(
            readings,
            end=window.dates[-1],
            window_days=window.day_count,
            reducer=AGGREGATIONS[feature],
        )
        aggregates[feature] = SeasonFeature(
            feature=feature,
            unit=FEATURE_UNITS[feature],
            value=accumulation.value,
            covered_days=accumulation.covered_days,
            window_days=accumulation.required_days,
            reason=accumulation.reason,
        )
    return aggregates


def season_row(
    *,
    cut: str,
    comuna: str,
    provincia: str,
    window: Window,
    observations: dict[dt.date, dict[str, float | None]],
    source: str,
) -> SeasonRow:
    """Build one row, recording the source that produced it and how its days are cut."""
    spec = meteo.source_named(source)
    window_dates = set(window.dates)
    inside = [date for date in observations if date in window_dates]
    return SeasonRow(
        cut=cut,
        comuna=comuna,
        provincia=provincia,
        season=window.label,
        window_start=window.dates[0],
        window_end=window.dates[-1],
        days_requested=window.day_count,
        days_returned=len(inside),
        source=spec.name,
        day_alignment=spec.day_alignment,
        resolves_civil_day=spec.resolves_civil_day,
        features=aggregate(observations, window),
    )


# ---------------------------------------------------------------------------
# The artifact's units, checked instead of assumed
# ---------------------------------------------------------------------------


def check_artifact_units(artifact: Artifact) -> dict:
    """Compare the units this module declares with the artifact descriptor's.

    This is not a conversion and it fixes nothing: it reports, in the manifest, the
    features whose declared unit differs from the one the artifact publishes, and the
    artifact features this source cannot produce. The artifact is the contract `risk`
    and the inference service share, so a unit that disagrees has to be resolved
    THERE; converting here would put a number in the panel that the artifact's own
    guard would later reject -- or accept and misread.
    """
    conflicts: list[dict] = []
    for feature, unit in FEATURE_UNITS.items():
        if feature not in artifact.names:
            conflicts.append(
                {
                    "feature": feature,
                    "source_unit": unit,
                    "artifact_unit": None,
                    "kind": "missing-from-artifact",
                    "resolution": (
                        "the artifact does not declare this feature; either add it or stop "
                        "producing it"
                    ),
                }
            )
            continue
        declared = artifact.feature(feature).unit
        if declared != unit:
            conflicts.append(
                {
                    "feature": feature,
                    "source_unit": unit,
                    "artifact_unit": declared,
                    "kind": "unit-conflict",
                    "resolution": (
                        "the artifact descriptor has to declare the unit this data is in; the "
                        "two are different quantities and nothing here converts between them"
                    ),
                }
            )
    not_produced = [name for name in artifact.names if name not in FEATURE_UNITS]
    return {
        "artifact_path": _display_path(DEFAULT_ARTIFACT),
        "artifact_features": list(artifact.names),
        "conflicts": conflicts,
        "not_produced_here": not_produced,
        "not_produced_reason": (
            "wind_dir: neither source's verified parameter set carries a daily wind "
            "direction, and a cardinal word or a per-hour value would not match the degrees "
            "the artifact declares. Absent, and named."
            if "wind_dir" in not_produced
            else ""
        ),
    }


# ---------------------------------------------------------------------------
# Building the panel
# ---------------------------------------------------------------------------


@dataclass
class SeasonPanel:
    """The rows plus the window they were built over.

    The source is on every row rather than here: a panel is one source's output today,
    and keeping the fact on the row means a future mixed panel cannot lose it.
    """

    rows: list[SeasonRow]
    windows: tuple[Window, ...]


def build_panel(
    centroids,
    windows: tuple[Window, ...],
    *,
    source: str,
    observations_for,
    progress=None,
) -> SeasonPanel:
    """One row per (comuna, season). `observations_for(centroid, window)` supplies days."""
    rows: list[SeasonRow] = []
    for centroid in centroids.entries:
        for window in windows:
            observations = observations_for(centroid, window)
            rows.append(
                season_row(
                    cut=centroid.cut,
                    comuna=centroid.name,
                    provincia=centroid.provincia,
                    window=window,
                    observations=observations,
                    source=source,
                )
            )
            if progress is not None:
                progress(rows[-1])
    return SeasonPanel(rows=rows, windows=windows)


def panel_columns() -> list[str]:
    """The CSV header: the key, the coverage, the source, then each feature and its count."""
    header: list[str] = list(PANEL_COLUMNS)
    for feature in FEATURES:
        header.append(feature)
        header.append(f"{feature}_covered_days")
    return header


def write_panel(path: Path, panel: SeasonPanel) -> None:
    """Write the panel as CSV with a fixed column order and Unix line endings."""
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8", newline="") as handle:
        writer = csv.writer(handle, lineterminator="\n")
        writer.writerow(panel_columns())
        for row in panel.rows:
            writer.writerow(row.as_row())


def write_manifest(path: Path, manifest: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8", newline="") as handle:
        json.dump(manifest, handle, ensure_ascii=False, indent=2)
        handle.write("\n")


DIVERGENCES = (
    (
        "radiation_sol: this module declares MJ/m2 because both sources deliver a DAILY "
        "INTEGRAL; the artifact descriptor declares W/m2, a mean irradiance. Same name, "
        "different quantity -- reported in contract.conflicts and never converted here."
    ),
    (
        "wind_speed: Open-Meteo publishes the 10 m daily maximum, NASA POWER the 2 m daily "
        "maximum. Same name and same declared unit (m/s), different measurement height, so a "
        "panel built from one is not interchangeable with a panel built from the other."
    ),
    (
        "The two sources are different reanalyses on different grids (Open-Meteo's ERA5 "
        "archive vs NASA POWER's 0.5 x 0.625 degree product), so their values for the same "
        "comuna-day differ. A panel records which one produced each row."
    ),
    (
        "NASA POWER's days are LST with no DST; Open-Meteo's are America/Santiago civil days. "
        "In Chilean summer the two boundaries differ by an hour, and every row states which "
        "alignment it was built from."
    ),
    (
        "humidity_rel: both sources give one daily mean per day, but Open-Meteo derives it "
        "from temperature and dew point while POWER averages its own field. The estimator is "
        "the same (mean of daily means); the underlying model is not."
    ),
)

NOTES = (
    (
        "A season whose window is not fully covered yields an absent feature with its "
        "coverage count, never a zero: every aggregate goes through "
        "assemble.reduce_window, the same rule the comuna-day panel and the Go side use."
    ),
    (
        "days_returned counts the requested days the source actually answered for inside the "
        "window. A day it omitted and a day it filled with its fill value both reduce a "
        "feature's coverage, and neither becomes a measurement."
    ),
    (
        "The cut-off is configurable and every row records the window it used, so a panel "
        "built to 31 December can never be mistaken for an end-of-season panel."
    ),
)


def build_manifest(
    *,
    panel: SeasonPanel,
    source: str,
    cutoff_note: str,
    artifact: Artifact,
    artifact_check: dict,
    generated_at: str,
) -> dict:
    """Describe the panel: the source and its terms, the window, and what is absent."""
    spec = meteo.source_named(source)
    windows = panel.windows
    counts = {
        "rows": len(panel.rows),
        "comunas": len({row.cut for row in panel.rows}),
        "seasons": len(windows),
        "first_season": windows[0].label if windows else None,
        "last_season": windows[-1].label if windows else None,
        "rows_with_no_feature": sum(
            1 for row in panel.rows if not any(entry.present for entry in row.features.values())
        ),
        "features": {
            feature: {
                "present_rows": sum(1 for row in panel.rows if row.features[feature].present),
                "absent_rows": sum(1 for row in panel.rows if not row.features[feature].present),
                "completely_covered_rows": sum(
                    1
                    for row in panel.rows
                    if row.features[feature].covered_days == row.features[feature].window_days
                ),
            }
            for feature in FEATURES
        },
    }
    return {
        "generated_by": "backend/training/seasons.py",
        "generated_at": generated_at,
        "source": {
            "name": spec.name,
            "url": spec.url,
            "licence": spec.licence,
            "attribution": spec.attribution,
            "terms": spec.terms,
            "day_alignment": spec.day_alignment,
            "resolves_civil_day": spec.resolves_civil_day,
            "grid": spec.grid,
            "notes": list(spec.notes),
            "variables": [
                {
                    "feature": variable.feature,
                    "parameter": variable.parameter,
                    "source_unit": variable.source_unit,
                    "factor": variable.factor,
                    "declared_unit": FEATURE_UNITS[variable.feature],
                    "semantics": variable.semantics,
                }
                for variable in spec.variables
            ],
            "cache": _display_path(meteo.DEFAULT_CACHE_DIR),
        },
        "window": {
            "timezone": TIMEZONE,
            "season_start": f"{SEASON_START_MONTH:02d}-{SEASON_START_DAY:02d}",
            "season_end": "06-30",
            "season_label": "start year .. start year + 1",
            "cutoff": cutoff_note,
            "cutoff_default": f"{DEFAULT_CUTOFF[0]:02d}-{DEFAULT_CUTOFF[1]:02d} of the start year",
            "days_per_season": {window.label: window.day_count for window in windows},
            "windows": [
                {
                    "season": window.label,
                    "first_local_date": window.dates[0].isoformat(),
                    "last_local_date": window.dates[-1].isoformat(),
                    "days": window.day_count,
                    "start_instant": window.start.isoformat(),
                    "end_instant": window.end.isoformat(),
                }
                for window in windows
            ],
        },
        "features": [
            {
                "name": feature,
                "unit": FEATURE_UNITS[feature],
                "aggregation": AGGREGATIONS[feature],
                "over": "the feature window, in America/Santiago civil days",
                "note": FEATURE_NOTES.get(feature, ""),
            }
            for feature in FEATURES
        ],
        "contract": {**artifact_check, "artifact_version": artifact.version},
        "counts": counts,
        "divergences": list(DIVERGENCES),
        "notes": list(NOTES),
    }


def _display_path(path: Path) -> str:
    """Show a path relative to the repository when it lives inside it."""
    try:
        return path.resolve().relative_to(REPO_ROOT).as_posix()
    except ValueError:
        return path.as_posix()


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------


def _resolve(path: Path) -> Path:
    return path if path.is_absolute() else (Path.cwd() / path)


def _select(centroids, comunas: list[str] | None):
    """Restrict the mapping to the requested CUTs, refusing one it does not know."""
    if not comunas:
        return centroids
    selected = tuple(centroids.centroid(cut) for cut in comunas)
    return type(centroids)(
        region_cut=centroids.region_cut,
        region_name=centroids.region_name,
        entries=selected,
        method=centroids.method,
        provenance=centroids.provenance,
    )


def build(
    *,
    source: str,
    first: int = FIRST_SEASON,
    last: int = LAST_SEASON,
    month_day: tuple[int, int] | None = None,
    cutoff_note: str = "12-31",
    comunas: list[str] | None = None,
    centroids_path: Path | None = None,
    cache: meteo.MeteoCache | None = None,
    artifact_path: Path = DEFAULT_ARTIFACT,
    generated_at: str | None = None,
    progress=None,
) -> tuple[SeasonPanel, dict]:
    """Build the season panel for one source. Every failure names its cause."""
    load_zone(TIMEZONE)
    try:
        centroids = load_centroids(_resolve(centroids_path) if centroids_path else None)
        centroids = _select(centroids, comunas)
    except CentroidError as error:
        raise SeasonError(str(error)) from error
    windows = season_windows(first=first, last=last, month_day=month_day)
    resolved_cache = cache or meteo.MeteoCache(meteo.DEFAULT_CACHE_DIR)

    def observations_for(centroid, window):
        response: MeteoResponse = meteo.fetch(
            source,
            meteo.Coordinate.of(centroid),
            window.dates[0],
            window.dates[-1],
            cache=resolved_cache,
        )
        return response.observations()

    panel = build_panel(
        centroids, windows, source=source, observations_for=observations_for, progress=progress
    )
    artifact = load_artifact(_resolve(artifact_path))
    manifest = build_manifest(
        panel=panel,
        source=source,
        cutoff_note=cutoff_note,
        artifact=artifact,
        artifact_check=check_artifact_units(artifact),
        generated_at=generated_at
        or dt.datetime.now(dt.UTC).replace(microsecond=0).isoformat().replace("+00:00", "Z"),
    )
    return panel, manifest


def summary(panel: SeasonPanel, manifest: dict) -> list[str]:
    """The counts a reviewer needs, including what is absent and the unit conflict."""
    counts = manifest["counts"]
    lines = [
        (
            f"rows: {counts['rows']} ({counts['comunas']} comunas x {counts['seasons']} "
            f"seasons, {counts['first_season']} .. {counts['last_season']})"
        ),
        f"source: {manifest['source']['name']} -- {manifest['source']['day_alignment']}",
        f"window: 07-01 .. {manifest['window']['cutoff']} ({TIMEZONE})",
    ]
    for feature in FEATURES:
        stats = counts["features"][feature]
        lines.append(
            f"  {feature:<14} {FEATURE_UNITS[feature]:<7} present in {stats['present_rows']} "
            f"row(s), absent in {stats['absent_rows']}, fully covered in "
            f"{stats['completely_covered_rows']}"
        )
    if counts["rows_with_no_feature"]:
        lines.append(
            f"note: {counts['rows_with_no_feature']} row(s) have every feature absent -- a "
            f"window the source did not cover, not a season with no weather"
        )
    conflicts = manifest["contract"]["conflicts"]
    if conflicts:
        lines.append("contract conflicts (reported, never converted):")
        for conflict in conflicts:
            lines.append(
                f"  {conflict['feature']}: source {conflict['source_unit']!r} vs artifact "
                f"{conflict['artifact_unit']!r}"
            )
    return lines


def main(argv: list[str] | None = None) -> int:
    """Build `seasons.csv` and `manifest.json` for one source."""
    parser = argparse.ArgumentParser(
        description=(
            "Build season-to-date feature aggregates, one row per (comuna, season), from one "
            "named meteorology source, through the on-disk cache."
        )
    )
    parser.add_argument("--source", required=True, help=", ".join(sorted(meteo.SOURCES)))
    parser.add_argument("--out", type=Path, default=Path("out/seasons"), help="output directory")
    parser.add_argument("--first", type=int, default=FIRST_SEASON, help="first season start year")
    parser.add_argument("--last", type=int, default=LAST_SEASON, help="last season start year")
    parser.add_argument(
        "--cutoff",
        default=None,
        help="feature-window cut-off as MM-DD (default: 12-31 of the season's start year)",
    )
    parser.add_argument(
        "--comunas", default="", help="comma-separated CUTs; empty means all 38 of region 05"
    )
    parser.add_argument("--centroids", type=Path, default=None, help="centroid data file")
    parser.add_argument("--cache", type=Path, default=None, help="cache root")
    parser.add_argument(
        "--artifact", type=Path, default=DEFAULT_ARTIFACT, help="artifact descriptor"
    )
    parser.add_argument("--generated-at", default=None, help="override the manifest timestamp")
    arguments = parser.parse_args(argv)
    try:
        month_day = parse_cutoff(arguments.cutoff) if arguments.cutoff else None
        panel, manifest = build(
            source=arguments.source,
            first=arguments.first,
            last=arguments.last,
            month_day=month_day,
            cutoff_note=arguments.cutoff or "12-31",
            comunas=[cut.strip() for cut in arguments.comunas.split(",") if cut.strip()],
            centroids_path=arguments.centroids,
            cache=meteo.MeteoCache(arguments.cache) if arguments.cache else None,
            artifact_path=arguments.artifact,
            generated_at=arguments.generated_at,
        )
    except (MeteoError, SeasonError, CentroidError, WindowError, AssembleError) as error:
        print(f"error: {error}", file=sys.stderr)
        return 2
    write_panel(arguments.out / "seasons.csv", panel)
    write_manifest(arguments.out / "manifest.json", manifest)
    for line in summary(panel, manifest):
        print(line)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
