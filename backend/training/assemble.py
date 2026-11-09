"""Assemble the comuna-day panel the FireWatch risk model is trained on.

The panel is one row per (Chilean CUT, local calendar date): the key `risk` already
uses at publication time, and the aggregation unit a daily extreme needs. This
module turns the CONAF per-event registry into that shape: event -> comuna-day,
counted and summed, never silently deduplicated, plus the model's feature columns
in the artifact's own order.

Three things this module deliberately does NOT do, because a plausible-looking
value is worse than an absent one:

* It does not invent a CUT. The source names the comuna, so the name is resolved
  through the committed DPA mapping (`backend/training/data/comunas-maule-cut.json`);
  a name the mapping does not know keeps ``cut = ""`` and is reported in
  ``comuna_unresolved`` and in the manifest, never given a nearby code.
* It does not impute a date. A row whose ``Inicio`` is blank or unparsable is
  excluded and counted; the panel's date is a fact or the row is not a comuna-day.
* It does not fill a feature with zero when the source cannot support it. Every
  artifact feature is either produced from a named source column in the artifact's
  own unit, or written as an empty cell and declared ``unavailable`` with a reason.
  A zero is a measurement here, so a zero from absence is a lie the model cannot see.

WHERE THE FEATURE LIST COMES FROM. The artifact descriptor -- what the inference
service serves at ``GET /v1/model`` -- is the single authority on which features
exist, their order and their units. This module reads that document and mirrors
its names; the ``PRODUCERS`` table below is the only place in Python that says how
a name could be produced from this source, and it is validated against the artifact
in both directions (an artifact name with no producer entry is a failure, and so is
a requested name the artifact does not declare). Nothing else in this package
repeats the seven names.

WHAT THIS SAMPLE CANNOT DO. The downloadable sample is one season of one region
(Maule, 2018) and it is a schema and pipeline fixture, not a training set. Its
meteorology is attached to events, so a comuna-day without a fire has no reading at
all and the panel's feature columns are empty exactly on the days that are the
label's zeros. The manifest records that; it is the reason this panel proves the
schema and the windows instead of training anything.

Run it as::

    python backend/training/assemble.py \\
        --source backend/training/tests/fixtures/conaf-maule-sample.csv \\
        --out backend/training/out
"""

from __future__ import annotations

import argparse
import csv
import datetime as dt
import hashlib
import json
import re
import sys
from dataclasses import dataclass, field
from pathlib import Path
from zoneinfo import ZoneInfo, ZoneInfoNotFoundError

from comunas import ComunaMap, load_mapping

# ---------------------------------------------------------------------------
# Contract constants
# ---------------------------------------------------------------------------

#: The civil time zone every daily aggregate is cut in. It mirrors
#: `risk/internal/features.Location` character for character: a panel cut in
#: another zone answers a different question about the same day, and Chile's DST
#: means a "day" is 23 or 25 hours long twice a year.
TIMEZONE = "America/Santiago"

#: How many civil days `precip_sum` accumulates. Mirrors
#: `risk/internal/features/precipitationWindowDays`. A six-day sum is a different
#: feature, not a smaller one.
PRECIPITATION_WINDOW_DAYS = 7

#: Columns of the CONAF registry this assembler reads. Spelled once, so a header
#: change in the source is a named failure instead of a silently empty column.
COL_REGION = "Region"
COL_PROVINCIA = "Provincia"
COL_COMUNA = "Comuna"
COL_INICIO = "Inicio"
COL_CAUSA = "Causa general"
COL_SUPERFICIE = "Superficie total"
COL_TEMPERATURA = "Temperatura"
COL_HUMEDAD = "Humedad"
COL_VIENTO_VELOCIDAD = "Velocidad viento"
COL_VIENTO_DIRECCION = "Direccion viento"
COL_PENDIENTE = "Pendiente"

REQUIRED_COLUMNS = (
    COL_REGION,
    COL_PROVINCIA,
    COL_COMUNA,
    COL_INICIO,
    COL_CAUSA,
    COL_SUPERFICIE,
    COL_TEMPERATURA,
    COL_HUMEDAD,
    COL_VIENTO_VELOCIDAD,
    COL_VIENTO_DIRECCION,
    COL_PENDIENTE,
)

#: Columns whose value coverage the manifest reports. It is the columns the unit audit
#: talks about, so "Humedad is 0-97" is a measured statement about the file rather than
#: a recollection: the manifest prints how many rows carry a value and how many are empty.
AUDITED_COLUMNS = (
    COL_TEMPERATURA,
    COL_HUMEDAD,
    COL_VIENTO_VELOCIDAD,
    COL_VIENTO_DIRECCION,
    COL_PENDIENTE,
    COL_SUPERFICIE,
)

#: The panel's key, label and provenance columns, in output order.
PANEL_COLUMNS = (
    "cut",
    "comuna",
    "comuna_unresolved",
    "provincia",
    "region",
    "local_date",
    "fire_today",
    "fire_count",
    "burned_ha",
    "meteorology_readings",
)

REPO_ROOT = Path(__file__).resolve().parents[2]
#: Resolved against the repository, not the working directory: the panel is meant to be
#: runnable from anywhere (the CI job runs from backend/training), and an artifact that
#: silently failed to load would make every feature name look undeclared.
#:
#: `logistic.json` rather than `forest.json`: it is the descriptor the running system
#: references -- backend/docker-compose.yml mounts it as FIREWATCH_MODEL_PATH,
#: migration 011 records it as the artifact of `ml-fixture-v1`, and the risk
#: classifier's factor table mirrors it. Both committed descriptors declare the same
#: seven names and units today, so pointing at the referenced one is the difference
#: between mirroring the contract in force and mirroring a scorer test fixture.
DEFAULT_ARTIFACT = REPO_ROOT / "backend/services/inference/testdata/logistic.json"
DEFAULT_MAPPING = REPO_ROOT / "backend/training/data/comunas-maule-cut.json"


class AssembleError(Exception):
    """A failure the operator must fix; the CLI prints it instead of a traceback."""


class WindowError(ValueError):
    """An aggregation window that is not the one the contract fixes."""


# ---------------------------------------------------------------------------
# Civil time: loaded once, never falling back to UTC
# ---------------------------------------------------------------------------


def load_zone(name: str = TIMEZONE) -> ZoneInfo:
    """Load a named civil time zone, or refuse to run.

    `zoneinfo` needs a zone database. A host without one cannot answer "which
    local day" at all, and falling back to UTC would produce aggregates that look
    plausible and cut the day at 20:00 or 21:00 local time -- the exact defect the
    readings history's `features.New` guards against on the Go side. So a missing
    zone is an error, not a default.
    """
    try:
        return ZoneInfo(name)
    except (ZoneInfoNotFoundError, ValueError) as error:
        raise AssembleError(
            f"cannot load the civil time zone {name!r} ({error}); the daily windows need "
            f"that zone's data and this assembler has no UTC fallback"
        ) from error


def civil_date(moment: dt.datetime, zone: ZoneInfo) -> dt.date:
    """Return the civil date `moment` falls on inside `zone`."""
    if moment.tzinfo is None:
        raise AssembleError("civil_date needs an aware datetime; a naive one has no civil date")
    return moment.astimezone(zone).date()


def civil_day_window(end: dt.date, days: int) -> tuple[dt.date, ...]:
    """Return the `days` civil dates ending at `end`, inclusive, in ascending order."""
    if days < 1:
        raise WindowError(f"a civil-day window needs at least one day, got {days}")
    return tuple(end - dt.timedelta(days=offset) for offset in range(days - 1, -1, -1))


# ---------------------------------------------------------------------------
# The artifact is the authority on the feature vocabulary
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class ArtifactFeature:
    """One declared model input, as the artifact descriptor spells it."""

    name: str
    unit: str
    minimum: float
    maximum: float


@dataclass(frozen=True)
class Artifact:
    """The part of the artifact descriptor this assembler must agree with."""

    version: str
    features: tuple[ArtifactFeature, ...]

    @property
    def names(self) -> tuple[str, ...]:
        return tuple(feature.name for feature in self.features)

    def feature(self, name: str) -> ArtifactFeature:
        for feature in self.features:
            if feature.name == name:
                return feature
        raise AssembleError(f"the artifact does not declare the feature {name!r}")


def load_artifact(path: Path) -> Artifact:
    """Read the artifact descriptor and take its feature list as the vocabulary.

    The same document `GET /v1/model` serves is what `risk` builds its input vector
    from, so reading it here is what keeps training and inference from drifting
    apart on a name or a unit. A descriptor that does not parse, does not declare
    `firewatch-model/v1`, or repeats a feature name is refused: mirroring a
    half-read contract is worse than not mirroring it.
    """
    try:
        with path.open(encoding="utf-8") as handle:
            document = json.load(handle)
    except (OSError, json.JSONDecodeError) as error:
        raise AssembleError(f"cannot read the artifact descriptor {path} ({error})") from error
    if document.get("format") != "firewatch-model/v1":
        raise AssembleError(
            f"{path}: format is {document.get('format')!r}, expected 'firewatch-model/v1'"
        )
    declared = document.get("features")
    if not isinstance(declared, list) or not declared:
        raise AssembleError(f"{path}: no 'features' list, so there is no vocabulary to mirror")
    features: list[ArtifactFeature] = []
    for entry in declared:
        if not isinstance(entry, dict) or not entry.get("name") or not entry.get("unit"):
            raise AssembleError(f"{path}: a declared feature is missing its name or unit")
        features.append(
            ArtifactFeature(
                name=str(entry["name"]),
                unit=str(entry["unit"]),
                minimum=float(entry.get("min", float("-inf"))),
                maximum=float(entry.get("max", float("inf"))),
            )
        )
    names = [feature.name for feature in features]
    if len(set(names)) != len(names):
        raise AssembleError(f"{path}: a feature name is declared twice, so the order is ambiguous")
    return Artifact(version=str(document.get("version", "")), features=tuple(features))


# ---------------------------------------------------------------------------
# How each feature could be produced from this source (the only such table)
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class Producer:
    """How one artifact feature maps onto one column of the CONAF sample.

    `source_unit` is the unit the SOURCE declares for the column. The sample
    declares none -- its header is a bare column name -- so an entry may state the
    unit only when the values admit exactly one reading, and states None when they
    admit several. That distinction is what makes a feature available or not: the
    assembler never converts a value into the artifact's unit, because a conversion
    it guessed is indistinguishable from a measurement once it is in the panel.
    """

    name: str
    column: str | None
    source_unit: str | None
    #: "mean" | "max" | "min" | "sum"; None when nothing can be produced.
    reducer: str | None
    #: Civil days the reducer spans; 1 for an instantaneous quantity.
    window_days: int | None
    reason: str

    @property
    def available(self) -> bool:
        return self.column is not None and self.reducer is not None and self.source_unit is not None


#: The single declaration of how a feature could be produced. Validated against the
#: artifact vocabulary by `check_producers`, in both directions.
PRODUCERS: dict[str, Producer] = {
    "temp_max": Producer(
        name="temp_max",
        column=COL_TEMPERATURA,
        source_unit=None,
        reducer=None,
        window_days=1,
        reason=(
            "Temperatura carries one reading per event, not a series across the civil day, "
            "so a daily maximum cannot be built from it; and those readings exist only on "
            "days with a fire, which makes any reading-shaped feature a restatement of the "
            "label. risk computes temp_max as the maximum of the readings stored for the "
            "current civil day, which this source cannot reproduce."
        ),
    ),
    "temp_min": Producer(
        name="temp_min",
        column=COL_TEMPERATURA,
        source_unit=None,
        reducer=None,
        window_days=1,
        reason=(
            "Same column and same objection as temp_max: one reading per event is not a "
            "daily series, so a minimum would be a single sample presented as a daily extreme."
        ),
    ),
    "humidity_rel": Producer(
        name="humidity_rel",
        column=COL_HUMEDAD,
        source_unit="%",
        reducer="mean",
        window_days=1,
        reason=(
            "Humedad spans 0-97 in this sample and a relative humidity has no unit other than "
            "percent, so the source's undeclared unit admits exactly one reading here and it "
            "is the artifact's '%'. Produced as the mean of the readings attached to the "
            "comuna-day's events; risk sends the latest instantaneous reading, which is the "
            "same number whenever the day has one event and a different estimator otherwise "
            "(recorded under divergences)."
        ),
    ),
    "wind_speed": Producer(
        name="wind_speed",
        column=COL_VIENTO_VELOCIDAD,
        source_unit=None,
        reducer=None,
        window_days=1,
        reason=(
            "Velocidad viento declares no unit. The observed range (0-63.7, median 5.7) is "
            "consistent with both m/s and km/h, so converting it to the artifact's 'm/s' "
            "would be a guess: at km/h every value would be 3.6x too large, and nothing in "
            "the file settles which one it is. Refused rather than converted."
        ),
    ),
    "wind_dir": Producer(
        name="wind_dir",
        column=COL_VIENTO_DIRECCION,
        source_unit=None,
        reducer=None,
        window_days=1,
        reason=(
            "Direccion viento is an 8-point cardinal word (S, SE, ...) plus the literal "
            "'Calmo' and 50 empty cells, while the artifact declares degrees. Mapping a "
            "cardinal onto 45-degree bins would quantise every training value while inference "
            "sends continuous degrees, and 'Calmo' has no direction at all. Refused."
        ),
    ),
    "precip_sum": Producer(
        name="precip_sum",
        column=None,
        source_unit="mm",
        reducer="sum",
        window_days=PRECIPITATION_WINDOW_DAYS,
        reason=(
            "The sample carries no precipitation column at all. Even with one it would need "
            "every one of the seven civil days covered, and this source only has readings on "
            "days with a fire, so the seven-day window could never be complete."
        ),
    ),
    "radiation_sol": Producer(
        name="radiation_sol",
        column=None,
        source_unit=None,
        reducer=None,
        window_days=1,
        reason="The sample carries no radiation column, so there is nothing to read.",
    ),
}


def check_producers(artifact: Artifact) -> None:
    """Prove the producer table and the artifact describe the same vocabulary.

    Called before any row is read. It fails on a name this module was asked to
    produce but the artifact does not declare (a typo or an invented feature), and
    on an artifact feature with no producer entry at all -- silence there would
    drop a declared input from the panel without anyone noticing.
    """
    unknown = sorted(set(PRODUCERS) - set(artifact.names))
    if unknown:
        raise AssembleError(
            f"the artifact declares no feature named {', '.join(unknown)}; a name this "
            f"assembler was asked to produce must appear in its vocabulary"
        )
    missing = sorted(set(artifact.names) - set(PRODUCERS))
    if missing:
        raise AssembleError(
            f"no producer entry for the artifact feature {', '.join(missing)}; every declared "
            f"input needs an entry, even when the answer is that this source cannot supply it"
        )
    for name, producer in PRODUCERS.items():
        if not producer.available:
            continue
        declared = artifact.feature(name)
        if producer.source_unit != declared.unit:
            raise AssembleError(
                f"{name}: the source would produce {producer.source_unit!r} but the artifact "
                f"declares {declared.unit!r}; converting is a reinterpretation of the source's "
                f"value, so the feature is refused instead"
            )


def requested_features(artifact: Artifact, requested: list[str] | None) -> tuple[str, ...]:
    """Validate a requested subset against the artifact and keep the artifact's order."""
    if not requested:
        return artifact.names
    unknown = [name for name in requested if name not in artifact.names]
    if unknown:
        raise AssembleError(
            f"requested feature(s) {', '.join(unknown)} are not in the artifact vocabulary "
            f"({', '.join(artifact.names)})"
        )
    return tuple(name for name in artifact.names if name in requested)


# ---------------------------------------------------------------------------
# Reading the source
# ---------------------------------------------------------------------------

#: Spanish month abbreviations as the registry writes them. Spelled out instead of
#: using `%b`, because `strptime` reads month names in the host's locale and an
#: English-locale host would refuse "ene" -- which is the format 502 of the 951
#: rows use.
SPANISH_MONTHS = {
    "ene": 1,
    "feb": 2,
    "mar": 3,
    "abr": 4,
    "may": 5,
    "jun": 6,
    "jul": 7,
    "ago": 8,
    "sep": 9,
    "set": 9,
    "oct": 10,
    "nov": 11,
    "dic": 12,
}

SLASH_DATE = re.compile(r"^(\d{1,2})/(\d{1,2})/(\d{4})\s+(\d{1,2}):(\d{2})$")
TEXT_DATE = re.compile(r"^(\d{1,2})-([A-Za-zÁÉÍÓÚáéíóú]{3})-(\d{4})\s+(\d{1,2}):(\d{2})$")

#: The label the manifest uses for each recognised `Inicio` shape.
FORMAT_SLASH = "dd/mm/yyyy HH:MM"
FORMAT_TEXT = "d-mmm-yyyy HH:MM"
STATUS_BLANK = "blank"
STATUS_UNPARSABLE = "unparsable"


@dataclass(frozen=True)
class Inicio:
    """One `Inicio` cell, parsed into the civil date it names."""

    raw: str
    #: The civil date, or None when the cell is blank or unparsable.
    date: dt.date | None
    #: One of FORMAT_SLASH, FORMAT_TEXT, STATUS_BLANK, STATUS_UNPARSABLE.
    status: str


def parse_inicio(raw: str) -> Inicio:
    """Parse an `Inicio` cell into a local civil date.

    The registry writes two shapes in the same column: `07/09/2018 09:38` and
    `30-ene-2019 17:20`. A parser that knows only the first sees exactly 502 of the
    951 rows as unparsable, which is how "502 null dates" is produced by a reader
    rather than by the data. Both shapes are handled, and only a cell that matches
    neither is reported as a null date.

    The value carries no UTC offset, so it is read as America/Santiago wall-clock
    time, which is what a CONAF operator writes down. The date is therefore the
    local civil date, never a UTC-shifted one.
    """
    text = (raw or "").strip()
    if not text:
        return Inicio(raw=text, date=None, status=STATUS_BLANK)
    match = SLASH_DATE.match(text)
    if match:
        day, month, year = (int(part) for part in match.group(1, 2, 3))
        return Inicio(raw=text, date=_safe_date(year, month, day), status=FORMAT_SLASH)
    match = TEXT_DATE.match(text)
    if match:
        month = SPANISH_MONTHS.get(match.group(2).casefold())
        if month is None:
            return Inicio(raw=text, date=None, status=STATUS_UNPARSABLE)
        return Inicio(
            raw=text,
            date=_safe_date(int(match.group(3)), month, int(match.group(1))),
            status=FORMAT_TEXT,
        )
    return Inicio(raw=text, date=None, status=STATUS_UNPARSABLE)


def _safe_date(year: int, month: int, day: int) -> dt.date | None:
    """Build a date, or None when the cell names one that does not exist."""
    try:
        return dt.date(year, month, day)
    except ValueError:
        return None


@dataclass(frozen=True)
class Event:
    """One row of the registry, reduced to what the panel needs."""

    comuna: str
    provincia: str
    region: str
    inicio: Inicio
    cause: str
    #: `Superficie total` in hectares, or None when the cell is empty.
    burned_ha: float | None
    #: One entry per meteorology column; None when the cell is empty or unreadable.
    readings: dict[str, float | None]


def parse_number(raw: str) -> float | None:
    """Parse a numeric cell; an empty cell is absent, not zero."""
    text = (raw or "").strip().replace(",", ".")
    if not text:
        return None
    try:
        return float(text)
    except ValueError as error:
        raise ValueError(f"not a number: {raw!r}") from error


@dataclass
class SourceCounts:
    """Everything the manifest says about how the source was read."""

    rows: int = 0
    dated: int = 0
    blank_dates: int = 0
    unparsable_dates: int = 0
    unreadable_numbers: dict[str, int] = field(default_factory=dict)
    date_formats: dict[str, int] = field(default_factory=dict)
    #: Per source column: how many rows carry a readable value and how many are empty.
    #: This is what makes the unit audit in the manifest measured rather than asserted.
    column_values: dict[str, dict[str, int]] = field(default_factory=dict)

    def count_number(self, column: str) -> None:
        self.unreadable_numbers[column] = self.unreadable_numbers.get(column, 0) + 1

    def count_format(self, status: str) -> None:
        self.date_formats[status] = self.date_formats.get(status, 0) + 1

    def count_value(self, column: str, raw: str) -> None:
        bucket = self.column_values.setdefault(column, {"present": 0, "empty": 0})
        bucket["empty" if not (raw or "").strip() else "present"] += 1


def read_events(source: Path, counts: SourceCounts) -> list[Event]:
    """Read the registry into events, counting every reason a row cannot be used."""
    try:
        handle = source.open(encoding="utf-8-sig", newline="")
    except OSError as error:
        raise AssembleError(f"cannot read the source {source} ({error})") from error
    with handle:
        reader = csv.DictReader(handle)
        missing = [column for column in REQUIRED_COLUMNS if column not in (reader.fieldnames or [])]
        if missing:
            raise AssembleError(
                f"{source}: missing column(s) {', '.join(repr(name) for name in missing)}; "
                f"the source schema changed and the panel cannot be built from it"
            )
        events: list[Event] = []
        for row in reader:
            counts.rows += 1
            for column in AUDITED_COLUMNS:
                counts.count_value(column, row.get(column, ""))
            inicio = parse_inicio(row.get(COL_INICIO, ""))
            counts.count_format(inicio.status)
            if inicio.date is None:
                if inicio.status == STATUS_BLANK:
                    counts.blank_dates += 1
                else:
                    counts.unparsable_dates += 1
            else:
                counts.dated += 1
            events.append(
                Event(
                    comuna=(row.get(COL_COMUNA) or "").strip(),
                    provincia=(row.get(COL_PROVINCIA) or "").strip(),
                    region=(row.get(COL_REGION) or "").strip(),
                    inicio=inicio,
                    cause=(row.get(COL_CAUSA) or "").strip(),
                    burned_ha=_number_or_count(row.get(COL_SUPERFICIE, ""), COL_SUPERFICIE, counts),
                    readings={
                        # Only the columns that hold a NUMBER. `Direccion viento` (S, SE,
                        # Calmo) and `Pendiente` ("20%") are text, and parsing them as
                        # numbers would report 86 unreadable cells that are perfectly
                        # readable -- they are just not measurements any feature consumes.
                        # Their units are still audited, in SOURCE_UNITS.
                        column: _number_or_count(row.get(column, ""), column, counts)
                        for column in (COL_TEMPERATURA, COL_HUMEDAD, COL_VIENTO_VELOCIDAD)
                    },
                )
            )
    return events


def _number_or_count(raw: str, column: str, counts: SourceCounts) -> float | None:
    try:
        return parse_number(raw)
    except ValueError:
        counts.count_number(column)
        return None


# ---------------------------------------------------------------------------
# Windowed aggregation
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class Accumulation:
    """The outcome of one windowed aggregation, including why it may be absent."""

    value: float | None
    covered_days: int
    required_days: int
    reason: str


REDUCERS = {
    "mean": lambda values: sum(values) / len(values),
    "max": max,
    "min": min,
    "sum": sum,
}


def reduce_window(
    readings_by_day: dict[dt.date, list[float]],
    *,
    end: dt.date,
    window_days: int,
    reducer: str,
) -> Accumulation:
    """Aggregate one variable over a civil-day window ending at `end`.

    Every day in the window must hold at least one reading. A window with five of
    seven days covered is not a smaller sum worth reporting -- it is a different
    quantity, and reporting it under the feature's name would understate whatever
    the missing day held. So an incomplete window yields ``value=None`` with the
    coverage count in the reason, which the panel writes as an empty cell and the
    manifest records as absent.
    """
    if reducer not in REDUCERS:
        raise WindowError(f"unknown reducer {reducer!r}; expected one of {sorted(REDUCERS)}")
    days = civil_day_window(end, window_days)
    values: list[float] = []
    covered = 0
    for day in days:
        present = readings_by_day.get(day) or []
        if present:
            covered += 1
            values.extend(present)
    if covered < len(days):
        return Accumulation(
            value=None,
            covered_days=covered,
            required_days=len(days),
            reason=(
                f"ventana incompleta: {covered} de {len(days)} días locales con lectura; "
                f"se declara ausente y no se rellena"
            ),
        )
    return Accumulation(
        value=REDUCERS[reducer](values),
        covered_days=covered,
        required_days=len(days),
        reason=f"{reducer} de {len(values)} lecturas en {len(days)} día(s) locales",
    )


def precipitation_sum(
    readings_by_day: dict[dt.date, list[float]],
    *,
    end: dt.date,
    window_days: int = PRECIPITATION_WINDOW_DAYS,
) -> Accumulation:
    """Accumulate precipitation in millimetres over seven civil days.

    The window is part of the feature, not a parameter: `risk` sends a seven-day
    accumulation under the unit `mm`, so a six-day sum, a daily mean or a rate
    would all be a different input under the same name. Any other window is
    refused with `WindowError` rather than computed.

    Today's downloadable sample has no precipitation column, so `PRODUCERS`
    records `precip_sum` as unavailable and the panel never calls this. It is the
    contract a source must satisfy to make the feature producible, and the tests
    pin both halves of it: seven days accumulate, six are refused.
    """
    if window_days != PRECIPITATION_WINDOW_DAYS:
        raise WindowError(
            f"precip_sum is a {PRECIPITATION_WINDOW_DAYS}-civil-day accumulation (mm) as both "
            f"the artifact and risk declare it; a {window_days}-day window is a different "
            f"feature and is refused"
        )
    return reduce_window(readings_by_day, end=end, window_days=window_days, reducer="sum")


# ---------------------------------------------------------------------------
# The panel
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class PanelRow:
    """One comuna-day: the key, the label and the feature cells."""

    cut: str
    comuna: str
    comuna_unresolved: str
    provincia: str
    region: str
    local_date: dt.date
    fire_today: int
    fire_count: int
    #: Sum of the comuna-day's events in hectares; 0.0 on a quiet day, and None when
    #: any event of a fire day has an unreadable area. A missing area is not a zero.
    burned_ha: float | None
    meteorology_readings: int
    features: dict[str, float | None]

    def as_row(self, features: tuple[str, ...]) -> list[str]:
        cells = [
            self.cut,
            self.comuna,
            self.comuna_unresolved,
            self.provincia,
            self.region,
            self.local_date.isoformat(),
            str(self.fire_today),
            str(self.fire_count),
            format_number(self.burned_ha),
            str(self.meteorology_readings),
        ]
        cells.extend(format_number(self.features[name]) for name in features)
        return cells


@dataclass
class Panel:
    """The assembled rows plus the counts the manifest reports."""

    rows: list[PanelRow]
    features: tuple[str, ...]
    first_date: dt.date
    last_date: dt.date
    comunas: list[str]
    unresolved: list[str]
    events_used: int
    events_collapsed: int
    reading_coverage: dict[str, int]


def format_number(value: float | None) -> str:
    """Render a number for the CSV: empty when absent, never a zero in its place."""
    if value is None:
        return ""
    text = f"{value:.6f}".rstrip("0").rstrip(".")
    return text if text not in ("", "-0") else "0"


def build_panel(
    events: list[Event],
    *,
    mapping: ComunaMap,
    features: tuple[str, ...],
) -> Panel:
    """Turn the registry's events into one row per (comuna, local date).

    The panel is a grid: every comuna the source names, every local date between
    the first and the last dated event. A comuna-day with no event is a real row
    with ``fire_today = 0`` -- without those rows the label has no zeros and there
    is nothing to train against -- and its ``burned_ha`` is a true zero, because a
    day without a fire burned nothing. That is the one zero this module writes, and
    it is a fact about the label rather than a substitute for a missing measurement.
    """
    dated = [event for event in events if event.inicio.date is not None]
    names = sorted({event.comuna for event in events if event.comuna})
    if not dated or not names:
        raise AssembleError("no dated events in the source, so there is no panel to build")
    dates = [event.inicio.date for event in dated if event.inicio.date is not None]
    first_date, last_date = min(dates), max(dates)

    grouped: dict[tuple[str, dt.date], list[Event]] = {}
    for event in dated:
        assert event.inicio.date is not None  # narrowed by the filter above
        grouped.setdefault((event.comuna, event.inicio.date), []).append(event)

    unresolved: list[str] = []
    rows: list[PanelRow] = []
    collapsed = 0
    reading_coverage = dict.fromkeys(features, 0)
    calendar = civil_day_window(last_date, (last_date - first_date).days + 1)
    for name in names:
        comuna = mapping.resolve(name)
        if comuna is None and name not in unresolved:
            unresolved.append(name)
        for date in calendar:
            day_events = grouped.get((name, date), [])
            if len(day_events) > 1:
                collapsed += len(day_events) - 1
            cells = _feature_cells(day_events, features)
            areas = [event.burned_ha for event in day_events]
            for feature in features:
                if cells[feature] is not None:
                    reading_coverage[feature] += 1
            rows.append(
                PanelRow(
                    cut=comuna.cut if comuna else "",
                    comuna=name,
                    comuna_unresolved="" if comuna else name,
                    provincia=(
                        day_events[0].provincia if day_events else _province_of(name, events)
                    ),
                    region=(day_events[0].region if day_events else _region_of(name, events)),
                    local_date=date,
                    fire_today=1 if day_events else 0,
                    fire_count=len(day_events),
                    burned_ha=None if any(area is None for area in areas) else sum(areas),
                    meteorology_readings=sum(
                        1
                        for event in day_events
                        if any(value is not None for value in event.readings.values())
                    ),
                    features=cells,
                )
            )
    rows.sort(key=lambda row: (row.cut or "", row.comuna, row.local_date))
    return Panel(
        rows=rows,
        features=features,
        first_date=first_date,
        last_date=last_date,
        comunas=names,
        unresolved=unresolved,
        events_used=len(dated),
        events_collapsed=collapsed,
        reading_coverage=reading_coverage,
    )


def _feature_cells(day_events: list[Event], features: tuple[str, ...]) -> dict[str, float | None]:
    """Fill one comuna-day's feature cells from the readings its events carry.

    Only a producer marked available is attempted, and it goes through the same
    window rule as any other aggregate. Everything else stays None: the panel's
    empty cell is how "this source cannot answer it" reaches the manifest.
    """
    cells: dict[str, float | None] = {}
    for feature in features:
        producer = PRODUCERS[feature]
        if not producer.available or not day_events:
            cells[feature] = None
            continue
        end = day_events[0].inicio.date
        if end is None:
            cells[feature] = None
            continue
        readings_by_day: dict[dt.date, list[float]] = {}
        for event in day_events:
            if event.inicio.date is None:
                continue
            value = event.readings.get(producer.column or "")
            if value is None:
                continue
            readings_by_day.setdefault(event.inicio.date, []).append(value)
        accumulation = reduce_window(
            readings_by_day,
            end=end,
            window_days=producer.window_days or 1,
            reducer=producer.reducer or "mean",
        )
        cells[feature] = accumulation.value
    return cells


def _province_of(name: str, events: list[Event]) -> str:
    return next((event.provincia for event in events if event.comuna == name), "")


def _region_of(name: str, events: list[Event]) -> str:
    return next((event.region for event in events if event.comuna == name), "")


# ---------------------------------------------------------------------------
# Output
# ---------------------------------------------------------------------------

#: How the source declares the unit of each column it carries. The header is a bare
#: column name with no unit row, so "declared" is empty everywhere except
#: `Pendiente`, whose values carry their own '%'. Recording the difference is the
#: point: an inferred unit is a judgement this file states out loud.
#:
#: The ranges, medians and counts quoted in these notes were measured on the FULL
#: downloadable sample (951 rows), while `values` in the manifest is measured on
#: whichever rows were actually assembled. Keeping the two apart is what stops a
#: fixture-sized run from appearing to contradict the audit.
SOURCE_UNITS = (
    {
        "column": COL_TEMPERATURA,
        "declared": None,
        "inferred": "C",
        "ambiguous": False,
        "note": "no unit in the header; 2.6-51 in a Maule summer admits only degrees Celsius.",
    },
    {
        "column": COL_HUMEDAD,
        "declared": None,
        "inferred": "%",
        "ambiguous": False,
        "note": "no unit in the header; a relative humidity of 0-97 admits only percent.",
    },
    {
        "column": COL_VIENTO_VELOCIDAD,
        "declared": None,
        "inferred": None,
        "ambiguous": True,
        "note": "0-63.7 with median 5.7 is consistent with both m/s and km/h; not resolvable from the file.",
    },
    {
        "column": COL_VIENTO_DIRECCION,
        "declared": None,
        "inferred": None,
        "ambiguous": True,
        "note": "8-point cardinal words plus the literal 'Calmo'; not degrees, and 'Calmo' has no direction.",
    },
    {
        "column": COL_PENDIENTE,
        "declared": "%",
        "inferred": "%",
        "ambiguous": False,
        "note": (
            "each value carries its own '%' (685 of 951 read 0%, which looks like a default "
            "rather than a slope)."
        ),
    },
    {
        "column": COL_SUPERFICIE,
        "declared": None,
        "inferred": "ha",
        "ambiguous": False,
        "note": (
            "hectares by CONAF convention and by magnitude (0.003-1100); the label is written "
            "as burned_ha."
        ),
    },
)

NOTES = (
    (
        "This source is a schema and pipeline fixture for one region and one season, not a "
        "training set: the project's training label is the gated CONAF product (DOI "
        "10.71578/UXAUN5, restricted) and nothing here replaces it."
    ),
    (
        "Inicio carries local wall-clock time with no UTC offset. It is read as America/Santiago "
        "civil time, which is the zone every daily window is cut in; the panel never converts a "
        "date to UTC and back."
    ),
    (
        "The registry writes two date shapes in the same column (dd/mm/yyyy and d-mmm-yyyy). A "
        "reader that knows only the first manufactures exactly 502 null dates out of 951 rows; "
        "both shapes are parsed and the manifest counts them separately."
    ),
    (
        "Meteorology in this source is attached to events, so a comuna-day without a fire has no "
        "reading at all: every feature cell is empty on the label's quiet days. A model trained "
        "on this panel would learn from whether a reading exists -- which is the label -- rather "
        "than from its value."
    ),
    (
        "burned_ha on a quiet comuna-day is 0.0 because a day without a fire burned nothing. It is "
        "the label's own zero and it is not the same thing as an absent feature, which is written "
        "as an empty cell; a fire day with an unreadable Superficie total reports burned_ha as "
        "empty for the same reason."
    ),
)


def build_manifest(
    *,
    source: Path,
    source_sha256: str,
    source_bytes: int,
    counts: SourceCounts,
    panel: Panel,
    artifact: Artifact,
    artifact_path: Path,
    mapping_path: Path,
    mapping: ComunaMap,
    features: tuple[str, ...],
    generated_at: str,
) -> dict:
    """Describe the panel: where it came from, what it dropped and what it cannot say."""
    fire_rows = [row for row in panel.rows if row.fire_today]
    return {
        "generated_by": "backend/training/assemble.py",
        "generated_at": generated_at,
        "source": {
            "path": _display(source),
            "sha256": source_sha256,
            "bytes": source_bytes,
            "license": "CC BY 4.0",
            "citation": CITATION,
            "citation_note": CITATION_NOTE,
            "is_training_set": False,
        },
        "artifact": {
            "path": _display(artifact_path),
            "version": artifact.version,
            "features": list(artifact.names),
            "authority": (
                "The artifact descriptor is the single authority on feature names, order and "
                "units; this panel mirrors it and states where it cannot follow it."
            ),
        },
        "timezone": {
            "name": TIMEZONE,
            "used_for": "the civil-day boundary of every window and the panel's date key",
        },
        "counts": {
            "source_rows": counts.rows,
            "source_rows_dated": counts.dated,
            "null_date_rows_dropped": counts.blank_dates + counts.unparsable_dates,
            "null_date_blank": counts.blank_dates,
            "null_date_unparsable": counts.unparsable_dates,
            "date_formats": dict(sorted(counts.date_formats.items())),
            "events_in_panel": panel.events_used,
            "events_collapsed_into_a_comuna_day": panel.events_collapsed,
            "comuna_days": len(panel.rows),
            "comuna_days_with_fire": len(fire_rows),
            "comuna_days_without_fire": len(panel.rows) - len(fire_rows),
            "burned_ha_total": round(sum(row.burned_ha or 0.0 for row in panel.rows), 6),
            "burned_ha_unknown_comuna_days": sum(1 for row in panel.rows if row.burned_ha is None),
            "unreadable_numbers": dict(sorted(counts.unreadable_numbers.items())),
        },
        "dates": {
            "first_local_date": panel.first_date.isoformat(),
            "last_local_date": panel.last_date.isoformat(),
        },
        "comunas": {
            "mapping_path": _display(mapping_path),
            "mapping_region": f"{mapping.region_cut} {mapping.region_name}".strip(),
            "mapping_entries": len(mapping.comunas),
            "mapping_provenance": mapping.provenance,
            "panel_comunas": len(panel.comunas),
            "unresolved_count": len(panel.unresolved),
            "unresolved_names": panel.unresolved,
        },
        "features": [
            {
                "name": name,
                "unit": artifact.feature(name).unit,
                "status": "source" if PRODUCERS[name].available else "unavailable",
                "source_column": PRODUCERS[name].column,
                "source_unit": PRODUCERS[name].source_unit,
                "aggregation": (
                    None
                    if not PRODUCERS[name].available
                    else f"{PRODUCERS[name].reducer} over {PRODUCERS[name].window_days or 1} civil day(s)"
                ),
                "filled_comuna_days": panel.reading_coverage.get(name, 0),
                "empty_comuna_days": len(panel.rows) - panel.reading_coverage.get(name, 0),
                "reason": PRODUCERS[name].reason,
            }
            for name in features
        ],
        "source_units": [
            {
                **entry,
                "values": counts.column_values.get(entry["column"], {"present": 0, "empty": 0}),
            }
            for entry in SOURCE_UNITS
        ],
        "source_units_note": (
            "The ranges, medians and 0% counts in the notes were measured on the full 951-row "
            "downloadable sample; 'values' below is measured on the rows this run assembled."
        ),
        "divergences": DIVERGENCES,
        "notes": list(NOTES),
    }


CITATION = (
    "Vidal-Silva, C. et al. (2025). A Ground-Reported Dataset of Wildfire Occurrence and "
    "Damage in Chile (1985-2023). MDPI Data, 10(2), 115. Ledger URL as published by the "
    "source repository: https://github.com/cvidalmsu/FONDEF-IDEA- (CC BY 4.0)."
)

CITATION_NOTE = (
    "The source repository's README gives this citation with the DOI elided as "
    "'https://doi.org/...'; that is how it is reproduced here, and the DOI is not guessed. "
    "The citable, access-gated full registry is DOI 10.71578/UXAUN5 (19 seasons), which this "
    "sample is not."
)

DIVERGENCES = (
    (
        "humidity_rel: risk/features sends the latest instantaneous reading of the current civil "
        "day, while this panel stores the mean of the readings attached to the comuna-day's "
        "events. Same name and same unit, different estimator; they agree only when the day "
        "holds one reading."
    ),
    (
        "The panel cannot reproduce a daily extreme or a seven-day accumulation, because the "
        "source carries one reading per event and none on quiet days. temp_max, temp_min, "
        "wind_speed, wind_dir, precip_sum and radiation_sol are therefore absent, not zero."
    ),
)


def _display(path: Path) -> str:
    """Show a path relative to the repository when it lives inside it."""
    try:
        return path.resolve().relative_to(REPO_ROOT).as_posix()
    except ValueError:
        return path.as_posix()


def sha256_of(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(65536), b""):
            digest.update(chunk)
    return digest.hexdigest()


def write_panel(path: Path, panel: Panel) -> None:
    """Write the panel as CSV with a fixed column order and Unix line endings."""
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8", newline="") as handle:
        writer = csv.writer(handle, lineterminator="\n")
        writer.writerow([*PANEL_COLUMNS, *panel.features])
        for row in panel.rows:
            writer.writerow(row.as_row(panel.features))


def write_manifest(path: Path, manifest: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8", newline="") as handle:
        json.dump(manifest, handle, ensure_ascii=False, indent=2)
        handle.write("\n")


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------


def assemble(
    source: Path,
    *,
    artifact_path: Path = DEFAULT_ARTIFACT,
    mapping_path: Path = DEFAULT_MAPPING,
    requested: list[str] | None = None,
    generated_at: str | None = None,
) -> tuple[Panel, dict]:
    """Assemble the panel and its manifest. Every failure is an `AssembleError`."""
    # Loaded before anything else, for the failure: a host with no zone database must
    # stop here rather than cut the day in UTC.
    load_zone()
    artifact = load_artifact(_resolve(artifact_path))
    check_producers(artifact)
    features = requested_features(artifact, requested)
    mapping = _load_mapping(_resolve(mapping_path))
    counts = SourceCounts()
    events = read_events(source, counts)
    panel = build_panel(events, mapping=mapping, features=features)
    manifest = build_manifest(
        source=source,
        source_sha256=sha256_of(source),
        source_bytes=source.stat().st_size,
        counts=counts,
        panel=panel,
        artifact=artifact,
        artifact_path=_resolve(artifact_path),
        mapping_path=_resolve(mapping_path),
        mapping=mapping,
        features=features,
        generated_at=generated_at
        or dt.datetime.now(dt.UTC).replace(microsecond=0).isoformat().replace("+00:00", "Z"),
    )
    return panel, manifest


def _resolve(path: Path) -> Path:
    """Resolve an operator-supplied path against the working directory."""
    return path if path.is_absolute() else (Path.cwd() / path)


def _load_mapping(path: Path) -> ComunaMap:
    try:
        return load_mapping(path)
    except ValueError as error:
        raise AssembleError(str(error)) from error


def main(argv: list[str] | None = None) -> int:
    """Run the assembler and print the counts a reviewer needs."""
    parser = argparse.ArgumentParser(
        description=(
            "Build the comuna-day panel from the CONAF per-event registry sample. "
            "Writes panel.csv and manifest.json under --out."
        )
    )
    parser.add_argument("--source", required=True, type=Path, help="registry CSV to read")
    parser.add_argument(
        "--out", type=Path, default=Path("backend/training/out"), help="output directory"
    )
    parser.add_argument(
        "--artifact", type=Path, default=DEFAULT_ARTIFACT, help="artifact descriptor"
    )
    parser.add_argument("--mapping", type=Path, default=DEFAULT_MAPPING, help="comuna name -> CUT")
    parser.add_argument(
        "--features",
        default="",
        help="comma-separated subset of the artifact's features; empty means all of them",
    )
    parser.add_argument("--generated-at", default=None, help="override the manifest timestamp")
    arguments = parser.parse_args(argv)
    requested = [name.strip() for name in arguments.features.split(",") if name.strip()]
    try:
        panel, manifest = assemble(
            arguments.source,
            artifact_path=arguments.artifact,
            mapping_path=arguments.mapping,
            requested=requested or None,
            generated_at=arguments.generated_at,
        )
    except AssembleError as error:
        print(f"error: {error}", file=sys.stderr)
        return 2
    out = arguments.out
    write_panel(out / "panel.csv", panel)
    write_manifest(out / "manifest.json", manifest)
    for line in summary(panel, manifest):
        print(line)
    return 0


def summary(panel: Panel, manifest: dict) -> list[str]:
    """The counts summary, including the two things this source forces on the panel."""
    counts = manifest["counts"]
    comunas = manifest["comunas"]
    available = [
        feature["name"] for feature in manifest["features"] if feature["status"] == "source"
    ]
    unavailable = [
        feature["name"] for feature in manifest["features"] if feature["status"] != "source"
    ]
    unresolved = comunas["unresolved_names"]
    resolved = len(panel.comunas) - len(panel.unresolved)
    lines = [
        (
            f"source rows: {counts['source_rows']} ({counts['source_rows_dated']} dated, "
            f"{counts['null_date_rows_dropped']} dropped for a null date)"
        ),
        f"date formats: {counts['date_formats']}",
        (
            f"comuna-days: {counts['comuna_days']} ({counts['comuna_days_with_fire']} with fire, "
            f"{counts['comuna_days_without_fire']} without) from {counts['events_in_panel']} "
            f"events; {counts['events_collapsed_into_a_comuna_day']} events collapsed into a "
            f"shared comuna-day"
        ),
        (
            f"dates: {manifest['dates']['first_local_date']} .. "
            f"{manifest['dates']['last_local_date']} ({TIMEZONE})"
        ),
        (
            f"comunas: {resolved} of {len(panel.comunas)} resolved by CUT, "
            f"{len(unresolved)} unresolved" + (f" ({', '.join(unresolved)})" if unresolved else "")
        ),
        f"features from the source: {', '.join(available) if available else 'none'}",
        f"features unavailable: {', '.join(unavailable) if unavailable else 'none'}",
        f"burned_ha total: {counts['burned_ha_total']}",
    ]
    if not available:
        lines.append(
            "warning: no feature could be produced from this source; the panel declares them "
            "absent instead of zero"
        )
    return lines


if __name__ == "__main__":
    raise SystemExit(main())
