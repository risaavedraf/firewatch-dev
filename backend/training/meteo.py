"""Keyless meteorology: two named sources, one interface, and a disk cache in between.

Both sources answer with no credential and no registration, which is the whole reason
they are here: `backend/training/assemble.py` established that the CONAF registry
cannot supply a daily series (its meteorology exists only on days with a fire), so
the model's six daily-aggregate features have to come from an independent source
joined by (comuna, date). This module is that source, one layer below the season
aggregation in `seasons.py`.

    open-meteo    https://archive-api.open-meteo.com/v1/archive
    nasa-power    https://power.larc.nasa.gov/api/temporal/daily/point

WHAT EACH SIDE IS RESPONSIBLE FOR. This module fetches, caches and PARSES. It does
not aggregate and it does not choose a source: `fetch()` takes a source name and
raises `MeteoError` naming that source when anything goes wrong. There is no
fallback, on purpose -- if Open-Meteo is down the caller decides what to do, and an
automatic switch would silently move every downstream row onto a coarser grid under
the same column names.

DECLARED UNITS, AND WHY THE MULTIPLICATION IS PINNED TO A STRING.
Each source publishes a unit for every variable it returns, so the parse checks the
declared unit against the one the client expects BEFORE it converts anything:

* Open-Meteo returns `wind_speed_10m_max` in **km/h**; the model contract declares
  `wind_speed` in **m/s**, so the value is multiplied by 1/3.6. If Open-Meteo ever
  answers m/s instead, the unit check fails and the run stops -- converting km/h and
  m/s with the same factor is a silent 3.6x error no downstream reader can see.
* Both sources deliver radiation as a **daily integral in MJ/m^2**, never W/m^2. That
  is the unit `seasons.py` declares for `radiation_sol`. The artifact descriptor still
  declares W/m2, and `seasons.check_artifact_units` records the disagreement instead
  of converting it: a mean irradiance and a daily integral are different quantities.
* NASA POWER labels the two accumulations `mm/day` and `MJ/m^2/day`. For a DAILY
  endpoint each value is that day's total expressed per day, so summing N days gives
  mm and MJ/m^2. The unit string is matched exactly as POWER publishes it and the
  meaning is stated in `Variable.semantics`, because "per day" on a daily series is
  the shape that produced the `precip_sum` mm/day-vs-mm defect already recorded in the
  feature log.

THE DAILY BOUNDARY IS NOT THE SAME ON BOTH SIDES, AND THE MODULE SAYS SO.

* Open-Meteo is asked for `timezone=America/Santiago` and echoes that calendar back,
  so its days ARE the civil days `seasons.py` cuts in. The echoed `timezone` is what
  vouches for that. `utc_offset_seconds` is recorded as declared metadata and is
  NOT used to derive days: it is a single number for the whole response, it cannot
  describe a range that crosses a DST change, and on a July 2023 range it answered
  -10800 where tzdata says -04:00 (observed, see tests/fixtures/meteo/PROVENANCE.md).
* NASA POWER answers in **LST**, its own local standard time with no DST (the
  response's `header.time_standard` says so), on a 0.5 x 0.625 degree grid. Its days
  are therefore not guaranteed to be America/Santiago civil days: in Chilean summer
  (UTC-3, permanent since 2022) an LST day is shifted by an hour at each boundary.
  `SourceSpec.resolves_civil_day` is False for POWER and every row `seasons.py` writes
  records the alignment it was built from; nothing here claims the two calendars agree.

REFUSING TO INVENT DATA. A day the response omits, and a day whose value is the
source's fill value (`null` for Open-Meteo, `-999` for POWER), both end up as a day
with no reading for that variable. Nothing is filled with a zero, because a zero is a
measurement in these sources -- 0.0 mm of rain is a fact and an absent day is not.
`MeteoResponse.days_requested`, `.days_returned` and `.missing_days` make the gap
reportable per request, which is what `seasons.py` turns into an absent feature with
its coverage count.

THE CACHE. `MeteoCache` stores, per (source, coordinate, date range), the raw response
bytes plus a provenance record: the URL, the retrieval instant, the sha256 of the
bytes and that source's licence and terms. The bytes are served back unchanged on a
second run -- no socket is opened -- and the stored sha256 is verified on every read,
so an edited cache entry is refused rather than parsed. The default root is
`backend/training/data/raw/meteo/`, which the package's `.gitignore` already covers
because everything under `data/raw/` is regenerable.

Run it as::

    python backend/training/meteo.py --source open-meteo --cut 05101 \\
        --start 2015-12-28 --end 2016-01-03
"""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import sys
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass, field
from pathlib import Path

from centroids import Centroid, CentroidError, load_centroids

REPO_ROOT = Path(__file__).resolve().parents[2]

#: The civil calendar the season windows are cut in. It is spelled identically in
#: `assemble.py` and in `risk/internal/features.Location`; a source that resolved days
#: in another calendar cannot be treated as the same day.
TIMEZONE = "America/Santiago"

#: Where the raw responses land. Ignored by backend/training/.gitignore through
#: `data/raw/`: every file here is regenerable from the URL plus the recorded sha256.
DEFAULT_CACHE_DIR = REPO_ROOT / "backend/training/data/raw/meteo"

USER_AGENT = "FireWatch/1.0 (academic wildfire training pipeline; stdlib urllib)"
TIMEOUT_SECONDS = 120

#: The units `seasons.py` declares, one per feature. This is the training-side
#: contract; `seasons.check_artifact_units` compares it with the artifact descriptor
#: and reports the one place they disagree (`radiation_sol`) instead of converting.
FEATURE_UNITS = {
    "temp_max": "degC",
    "temp_min": "degC",
    "humidity_rel": "%",
    "wind_speed": "m/s",
    "precip_sum": "mm",
    "radiation_sol": "MJ/m2",
}

OPEN_METEO = "open-meteo"
NASA_POWER = "nasa-power"


class MeteoError(Exception):
    """A failure the operator must look at; the CLI prints it instead of a traceback.

    Every raise names the source it happened on, so a failure cannot be mistaken for
    the other source's, and there is never a silent switch to a working one.
    """


@dataclass(frozen=True)
class Coordinate:
    """The point a source answers for. WGS84 degrees, in the order the model uses."""

    latitude: float
    longitude: float

    def __post_init__(self) -> None:
        if not -90.0 <= self.latitude <= 90.0:
            raise MeteoError(f"latitude {self.latitude} is outside [-90, 90]")
        if not -180.0 <= self.longitude <= 180.0:
            raise MeteoError(f"longitude {self.longitude} is outside [-180, 180]")

    @classmethod
    def of(cls, centroid: Centroid) -> Coordinate:
        return cls(latitude=centroid.latitude, longitude=centroid.longitude)

    def as_dict(self) -> dict:
        return {"latitude": round(self.latitude, 6), "longitude": round(self.longitude, 6)}


@dataclass(frozen=True)
class Variable:
    """One feature as one source publishes it."""

    #: The artifact's name for the feature, which is also `seasons.py`'s column.
    feature: str
    #: The source's own parameter name, sent in the request and read from the reply.
    parameter: str
    #: The unit the source declares for that parameter, matched before converting.
    source_unit: str
    #: Multiplier applied to the source value to reach `FEATURE_UNITS[feature]`.
    factor: float = 1.0
    #: What one day's value means in this source, in one sentence.
    semantics: str = ""

    @property
    def unit(self) -> str:
        return FEATURE_UNITS[self.feature]


@dataclass(frozen=True)
class SourceSpec:
    """Everything that makes one source that source, including its terms."""

    name: str
    url: str
    variables: tuple[Variable, ...]
    #: The licence/terms note stored with every cached response and in the manifest.
    licence: str
    attribution: str
    terms: str
    #: True when the source's own days are the civil days of `TIMEZONE`.
    resolves_civil_day: bool
    day_alignment: str
    #: Where the value actually comes from, in one sentence, for the manifest.
    grid: str
    notes: tuple[str, ...] = ()
    #: Fixed query parameters, in a stable order, so the cached URL is reproducible.
    fixed_params: tuple[tuple[str, str], ...] = ()

    def variable(self, feature: str) -> Variable:
        for entry in self.variables:
            if entry.feature == feature:
                return entry
        raise MeteoError(f"{self.name} carries no variable for the feature {feature!r}")

    @property
    def features(self) -> tuple[str, ...]:
        return tuple(entry.feature for entry in self.variables)

    def url_for(self, coordinate: Coordinate, start: dt.date, end: dt.date) -> str:
        if start > end:
            raise MeteoError(f"{self.name}: the range ends ({end}) before it starts ({start})")
        pairs = self._request_pairs(coordinate, start, end)
        # `safe=":,"` keeps the parameter lists readable while still escaping the
        # slash in `timezone=America%2FSantiago`, which is the form the source was
        # verified against.
        return f"{self.url}?{urllib.parse.urlencode(pairs, safe=':,')}"

    def _request_pairs(
        self, coordinate: Coordinate, start: dt.date, end: dt.date
    ) -> list[tuple[str, str]]:
        raise NotImplementedError

    def parameter_signature(self) -> str:
        """A short digest of the request shape, so a changed request cannot reuse a cache."""
        canonical = "|".join(
            [self.name, *self.fixed_params_flattened(), *(v.parameter for v in self.variables)]
        )
        return hashlib.sha256(canonical.encode("utf-8")).hexdigest()[:16]

    def fixed_params_flattened(self) -> tuple[str, ...]:
        return tuple(f"{key}={value}" for key, value in self.fixed_params)


@dataclass(frozen=True)
class OpenMeteoSpec(SourceSpec):
    """The ERA5-backed archive endpoint, the only one asked for a civil calendar."""

    def _request_pairs(
        self, coordinate: Coordinate, start: dt.date, end: dt.date
    ) -> list[tuple[str, str]]:
        return [
            ("latitude", f"{coordinate.latitude:.6f}"),
            ("longitude", f"{coordinate.longitude:.6f}"),
            ("start_date", start.isoformat()),
            ("end_date", end.isoformat()),
            ("daily", ",".join(entry.parameter for entry in self.variables)),
            ("timezone", TIMEZONE),
        ]


@dataclass(frozen=True)
class NasaPowerSpec(SourceSpec):
    """The POWER daily point endpoint. Months are YYYYMMDD and longitude comes first."""

    def _request_pairs(
        self, coordinate: Coordinate, start: dt.date, end: dt.date
    ) -> list[tuple[str, str]]:
        return [
            ("parameters", ",".join(entry.parameter for entry in self.variables)),
            *self.fixed_params,
            ("longitude", f"{coordinate.longitude:.6f}"),
            ("latitude", f"{coordinate.latitude:.6f}"),
            ("start", start.strftime("%Y%m%d")),
            ("end", end.strftime("%Y%m%d")),
            ("format", "JSON"),
        ]


#: The two named sources. Adding a third means adding an entry here and nothing else:
#: `fetch` looks the name up and refuses a name it does not know.
SOURCES: dict[str, SourceSpec] = {
    OPEN_METEO: OpenMeteoSpec(
        name=OPEN_METEO,
        url="https://archive-api.open-meteo.com/v1/archive",
        variables=(
            Variable(
                "temp_max",
                "temperature_2m_max",
                "\u00b0C",
                semantics="the civil day's maximum 2 m air temperature",
            ),
            Variable(
                "temp_min",
                "temperature_2m_min",
                "\u00b0C",
                semantics="the civil day's minimum 2 m air temperature",
            ),
            Variable(
                "humidity_rel",
                "relative_humidity_2m_mean",
                "%",
                semantics=(
                    "the civil day's MEAN relative humidity at 2 m (the parameter name says "
                    "so; the source derives it from temperature and dew point)"
                ),
            ),
            Variable(
                "wind_speed",
                "wind_speed_10m_max",
                "km/h",
                factor=1.0 / 3.6,
                semantics=(
                    "the civil day's maximum 10 m wind speed; km/h is converted to m/s "
                    "after the declared unit is matched"
                ),
            ),
            Variable(
                "precip_sum",
                "precipitation_sum",
                "mm",
                semantics="the civil day's precipitation total in millimetres",
            ),
            Variable(
                "radiation_sol",
                "shortwave_radiation_sum",
                "MJ/m\u00b2",
                semantics="the civil day's shortwave radiation integral in MJ/m^2",
            ),
        ),
        licence="Free for non-commercial use; no API key and no registration.",
        attribution="Open-Meteo (https://open-meteo.com), ERA5 reanalysis archive.",
        terms=(
            "Open-Meteo's archive is free for non-commercial use and needs no credential. "
            "The data is reanalysis-derived and licensed by Open-Meteo, not by FireWatch. "
            "Attribution to Open-Meteo is required; this repository is academic and "
            "non-commercial."
        ),
        resolves_civil_day=True,
        day_alignment=(
            "civil day in America/Santiago: the request names that calendar and the "
            "response echoes it back"
        ),
        grid=(
            "ERA5-backed archive, snapped to Open-Meteo's grid; the response echoes the "
            "grid node it actually answered for"
        ),
        notes=(
            (
                "The response's utc_offset_seconds is a single value for the whole range and "
                "is recorded, not used: it cannot describe a range crossing a DST change."
            ),
        ),
    ),
    NASA_POWER: NasaPowerSpec(
        name=NASA_POWER,
        url="https://power.larc.nasa.gov/api/temporal/daily/point",
        variables=(
            Variable(
                "temp_max",
                "T2M_MAX",
                "C",
                semantics="the LST day's maximum 2 m air temperature",
            ),
            Variable(
                "temp_min",
                "T2M_MIN",
                "C",
                semantics="the LST day's minimum 2 m air temperature",
            ),
            Variable(
                "humidity_rel",
                "RH2M",
                "%",
                semantics=(
                    "the LST day's relative humidity at 2 m: one value per day, the day's average"
                ),
            ),
            Variable(
                "wind_speed",
                "WS2M_MAX",
                "m/s",
                semantics="the LST day's maximum 2 m wind speed, already in m/s",
            ),
            Variable(
                "precip_sum",
                "PRECTOTCORR",
                "mm/day",
                semantics=(
                    "the LST day's corrected precipitation total; 'per day' on a daily "
                    "series is that day's accumulation, so N days sum to mm"
                ),
            ),
            Variable(
                "radiation_sol",
                "ALLSKY_SFC_SW_DWN",
                "MJ/m^2/day",
                semantics=(
                    "the LST day's all-sky shortwave downward irradiance integral; 'per "
                    "day' on a daily series is that day's integral, so N days sum to MJ/m^2"
                ),
            ),
        ),
        licence=(
            "No credential and no non-commercial restriction; NASA POWER asks to be cited "
            "and asks users to read its data disclaimer."
        ),
        attribution=(
            "NASA Langley Research Center (LaRC) Prediction Of Worldwide Energy Resources "
            "(POWER) project, https://power.larc.nasa.gov"
        ),
        terms=(
            "No credential and no registration. POWER publishes no non-commercial "
            "restriction, and this repository is academic; citation of the POWER project "
            "and awareness of its data disclaimer are the conditions it asks of users."
        ),
        resolves_civil_day=False,
        day_alignment=(
            "LST (the response's header.time_standard): a local standard-time day with no "
            "DST, which is NOT guaranteed to be an America/Santiago civil day"
        ),
        grid=(
            "source-native daily data on a 0.5 x 0.625 degree grid; the response's geometry "
            "echoes the grid cell it answered for, and it is coarser than Open-Meteo's"
        ),
        fixed_params=(("community", "AG"),),
        notes=(
            (
                "Fill values arrive as header.fill_value (-999.0). They are read as 'no "
                "reading' and never as a measurement."
            ),
        ),
    ),
}


def source_named(name: str) -> SourceSpec:
    """Look a source up by name, refusing anything that is not one of them.

    There is deliberately no default and no "first match": a typo in `--source` must
    not quietly fetch from whichever source happens to be first.
    """
    try:
        return SOURCES[name]
    except KeyError:
        raise MeteoError(
            f"unknown source {name!r}; this client knows {', '.join(sorted(SOURCES))}"
        ) from None


# ---------------------------------------------------------------------------
# What a response turns into
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class Day:
    """One civil day as the source returned it, with its own missing values kept."""

    date: dt.date
    #: feature -> value in the declared unit, or None when the source had no reading.
    values: dict[str, float | None] = field(default_factory=dict)


@dataclass(frozen=True)
class MeteoResponse:
    """One parsed response, plus everything a reader needs to distrust it properly."""

    source: str
    coordinate: Coordinate
    start: dt.date
    end: dt.date
    days: tuple[Day, ...]
    #: The units the source declared, per feature, before conversion.
    source_units: dict[str, str]
    #: Whatever else the source declared: calendar echo, offset, grid node, fill value.
    declared: dict
    #: The cache record: url, retrieved_at, sha256, bytes, licence, terms.
    provenance: dict
    cache_hit: bool
    spec: SourceSpec

    @property
    def dates_requested(self) -> tuple[dt.date, ...]:
        return tuple(
            self.start + dt.timedelta(days=offset) for offset in range(self.days_requested)
        )

    @property
    def days_requested(self) -> int:
        return (self.end - self.start).days + 1

    @property
    def days_returned(self) -> int:
        return len(self.days)

    @property
    def missing_days(self) -> tuple[dt.date, ...]:
        """The requested days the source did not return, ascending. Never zero-filled."""
        returned = {day.date for day in self.days}
        return tuple(date for date in self.dates_requested if date not in returned)

    def readings(self, feature: str) -> dict[dt.date, float]:
        """The days that carry a value for this feature; absent days are simply absent."""
        readings: dict[dt.date, float] = {}
        for day in self.days:
            value = day.values.get(feature)
            if value is not None:
                readings[day.date] = value
        return readings

    def observations(self) -> dict[dt.date, dict[str, float | None]]:
        """`{date: {feature: value}}`, the shape `seasons.py` aggregates."""
        return {day.date: dict(day.values) for day in self.days}


# ---------------------------------------------------------------------------
# Parsing: units are checked before anything is converted
# ---------------------------------------------------------------------------


def _decode(source: str, raw: bytes) -> dict:
    """Decode a response body into a JSON object, naming the source on every failure."""
    if not raw or not raw.strip():
        raise MeteoError(f"{source}: the response body is empty; an empty panel is not a result")
    try:
        document = json.loads(raw.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError) as error:
        raise MeteoError(f"{source}: the response is not JSON ({error})") from error
    if not isinstance(document, dict):
        raise MeteoError(f"{source}: the response is JSON but not an object")
    return document


def _open_meteo_days(
    spec: SourceSpec, document: dict, start: dt.date, end: dt.date
) -> tuple[tuple[Day, ...], dict[str, str], dict]:
    declared_timezone = document.get("timezone")
    if declared_timezone != TIMEZONE:
        raise MeteoError(
            f"{spec.name}: resolved the days in {declared_timezone!r}, not the civil calendar "
            f"{TIMEZONE!r}; a day cut somewhere else is a different question about the same "
            f"date, so the response is refused"
        )
    offset = document.get("utc_offset_seconds")
    if offset == 0:
        raise MeteoError(
            f"{spec.name}: utc_offset_seconds is 0, so the day boundary is UTC; "
            f"{TIMEZONE} is never UTC and this response cannot be a civil day"
        )
    daily_units = document.get("daily_units") or {}
    daily = document.get("daily") or {}
    times = daily.get("time")
    if not isinstance(times, list) or not times:
        raise MeteoError(
            f"{spec.name}: no 'daily.time' array, so there are no days to read "
            f"(the source may have rejected the range with {document.get('reason')!r})"
        )
    units: dict[str, str] = {}
    for variable in spec.variables:
        declared = daily_units.get(variable.parameter)
        if declared != variable.source_unit:
            raise MeteoError(
                f"{spec.name}: {variable.parameter} is declared in {declared!r} but this "
                f"client reads {variable.source_unit!r}; converting a unit it guessed is a "
                f"silent factor error, so the response is refused"
            )
        units[variable.feature] = str(declared)
    days: list[Day] = []
    for index, stamp in enumerate(times):
        date = _iso_date(spec.name, stamp)
        if not start <= date <= end:
            continue
        values: dict[str, float | None] = {}
        for variable in spec.variables:
            series = daily.get(variable.parameter)
            value = None
            if isinstance(series, list) and index < len(series):
                raw_value = series[index]
                if isinstance(raw_value, int | float) and not isinstance(raw_value, bool):
                    value = float(raw_value) * variable.factor
            values[variable.feature] = value
        days.append(Day(date=date, values=values))
    declared = {
        "timezone": declared_timezone,
        "utc_offset_seconds": offset,
        "timezone_abbreviation": document.get("timezone_abbreviation"),
        "grid_latitude": document.get("latitude"),
        "grid_longitude": document.get("longitude"),
        "elevation_m": document.get("elevation"),
        "utc_offset_used_for_days": False,
        "utc_offset_note": (
            "Recorded as declared by the source. The days are taken from the calendar the "
            "response echoes, because a single offset cannot describe a range that crosses "
            "a DST change."
        ),
    }
    return tuple(days), units, declared


def _nasa_power_days(
    spec: SourceSpec, document: dict, start: dt.date, end: dt.date
) -> tuple[tuple[Day, ...], dict[str, str], dict]:
    header = document.get("header") or {}
    time_standard = header.get("time_standard")
    if time_standard != "LST":
        raise MeteoError(
            f"{spec.name}: the response declares time_standard {time_standard!r}; this client "
            f"expects LST and will not guess what a different day boundary means"
        )
    fill = header.get("fill_value", -999.0)
    try:
        fill_value = float(fill)
    except (TypeError, ValueError) as error:
        raise MeteoError(f"{spec.name}: header.fill_value {fill!r} is not a number") from error
    metadata = document.get("parameters") or {}
    parameter = (document.get("properties") or {}).get("parameter")
    if not isinstance(parameter, dict) or not parameter:
        raise MeteoError(
            f"{spec.name}: no 'properties.parameter' object, so there are no days to read"
        )
    units: dict[str, str] = {}
    for variable in spec.variables:
        declared = (metadata.get(variable.parameter) or {}).get("units")
        if declared != variable.source_unit:
            raise MeteoError(
                f"{spec.name}: {variable.parameter} is declared in {declared!r} but this "
                f"client reads {variable.source_unit!r}; the response is refused rather than "
                f"converted with a factor it did not earn"
            )
        units[variable.feature] = str(declared)
    by_date: dict[dt.date, dict[str, float | None]] = {}
    for variable in spec.variables:
        series = parameter.get(variable.parameter) or {}
        if not isinstance(series, dict):
            raise MeteoError(f"{spec.name}: {variable.parameter} is not a date -> value map")
        for stamp, raw_value in series.items():
            date = _compact_date(spec.name, stamp)
            if not start <= date <= end:
                continue
            values = by_date.setdefault(date, dict.fromkeys(spec.features))
            if isinstance(raw_value, int | float) and not isinstance(raw_value, bool):
                number = float(raw_value)
                values[variable.feature] = (
                    None if number == fill_value else number * variable.factor
                )
    days = tuple(
        Day(date=date, values=values)
        for date, values in sorted(by_date.items(), key=lambda item: item[0])
    )
    declared = {
        "time_standard": time_standard,
        "fill_value": fill_value,
        "grid_coordinates": (document.get("geometry") or {}).get("coordinates"),
        "sources": header.get("sources"),
        "api_version": (header.get("api") or {}).get("version"),
        "resolves_civil_day": False,
    }
    return days, units, declared


def _iso_date(source: str, stamp: object) -> dt.date:
    try:
        return dt.date.fromisoformat(str(stamp))
    except ValueError as error:
        raise MeteoError(f"{source}: {stamp!r} is not an ISO date in the daily series") from error


def _compact_date(source: str, stamp: object) -> dt.date:
    text = str(stamp)
    if len(text) != 8 or not text.isdigit():
        raise MeteoError(f"{source}: {stamp!r} is not a YYYYMMDD date in the daily series")
    try:
        return dt.date(int(text[0:4]), int(text[4:6]), int(text[6:8]))
    except ValueError as error:
        raise MeteoError(f"{source}: {stamp!r} is not a calendar date") from error


def parse(
    spec: SourceSpec,
    raw: bytes,
    *,
    coordinate: Coordinate,
    start: dt.date,
    end: dt.date,
    provenance: dict | None = None,
    cache_hit: bool = False,
) -> MeteoResponse:
    """Parse a raw response into days, refusing anything that is not the expected shape."""
    document = _decode(spec.name, raw)
    for key in ("error", "reason"):
        if document.get(key):
            detail = document.get("reason") or document.get(key)
            raise MeteoError(f"{spec.name}: the source reported an error: {detail}")
    if spec.name == OPEN_METEO:
        days, units, declared = _open_meteo_days(spec, document, start, end)
    elif spec.name == NASA_POWER:
        days, units, declared = _nasa_power_days(spec, document, start, end)
    else:  # pragma: no cover - unreachable while SOURCES has exactly two entries
        raise MeteoError(f"{spec.name}: no parser for this source")
    if not days:
        raise MeteoError(
            f"{spec.name}: the response covers none of {start} .. {end}; "
            f"{len(days)} of {(end - start).days + 1} requested days were read"
        )
    return MeteoResponse(
        source=spec.name,
        coordinate=coordinate,
        start=start,
        end=end,
        days=days,
        source_units=units,
        declared=declared,
        provenance=provenance or {},
        cache_hit=cache_hit,
        spec=spec,
    )


# ---------------------------------------------------------------------------
# The cache
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class CacheEntry:
    """A cached response: the bytes as received, and what they are."""

    key: str
    body: bytes
    provenance: dict


class MeteoCache:
    """Raw responses on disk, one directory per source, one record per request.

    The key is derived from (source, coordinate, date range) plus a digest of the
    request shape, so a changed parameter list cannot be served bytes fetched for the
    old one. Each entry is two files: the response as received (byte for byte) and a
    provenance record with the URL, the retrieval instant, the sha256 of the bytes and
    the source's licence and terms.
    """

    def __init__(self, root: Path) -> None:
        self.root = root

    def key(self, spec: SourceSpec, coordinate: Coordinate, start: dt.date, end: dt.date) -> str:
        canonical = "|".join(
            [
                spec.name,
                f"{coordinate.latitude:.6f}",
                f"{coordinate.longitude:.6f}",
                start.isoformat(),
                end.isoformat(),
                spec.parameter_signature(),
            ]
        )
        return hashlib.sha256(canonical.encode("utf-8")).hexdigest()

    def _paths(self, spec: SourceSpec, key: str) -> tuple[Path, Path]:
        directory = self.root / spec.name
        return directory / f"{key}.json", directory / f"{key}.provenance.json"

    def read(self, spec: SourceSpec, key: str) -> CacheEntry | None:
        """Return the cached entry, verified against its own sha256, or None.

        A present-but-unreadable or present-but-modified entry is a failure, not a
        miss: refetching over it would hide the fact that the bytes changed under a
        recorded checksum.
        """
        body_path, provenance_path = self._paths(spec, key)
        if not body_path.is_file() or not provenance_path.is_file():
            return None
        try:
            body = body_path.read_bytes()
            with provenance_path.open(encoding="utf-8") as handle:
                provenance = json.load(handle)
        except (OSError, json.JSONDecodeError) as error:
            raise MeteoError(
                f"{spec.name}: the cache entry {body_path} cannot be read ({error})"
            ) from error
        actual = hashlib.sha256(body).hexdigest()
        declared = provenance.get("sha256")
        if actual != declared:
            raise MeteoError(
                f"{spec.name}: the cache entry {body_path} hashes to {actual} but its provenance "
                f"declares {declared}; the stored bytes changed, so it is refused instead of "
                f"parsed. Delete the entry to refetch."
            )
        return CacheEntry(key=key, body=body, provenance=provenance)

    def write(self, spec: SourceSpec, key: str, body: bytes, provenance: dict) -> CacheEntry:
        """Store the bytes, then the record that describes them.

        Order matters for a reader: a crash between the two writes leaves bytes with no
        provenance, which `read` treats as a miss, rather than a record describing bytes
        that are not there.
        """
        body_path, provenance_path = self._paths(spec, key)
        body_path.parent.mkdir(parents=True, exist_ok=True)
        record = dict(provenance)
        record["sha256"] = hashlib.sha256(body).hexdigest()
        record["bytes"] = len(body)
        body_path.write_bytes(body)
        with provenance_path.open("w", encoding="utf-8", newline="") as handle:
            json.dump(record, handle, ensure_ascii=False, indent=2)
            handle.write("\n")
        return CacheEntry(key=key, body=body, provenance=record)


# ---------------------------------------------------------------------------
# Fetching
# ---------------------------------------------------------------------------


def _urlopen(url: str, timeout: int = TIMEOUT_SECONDS) -> bytes:
    """Open the URL and return the body. Kept as one function so tests can replace it.

    `urllib.request.urlopen` is looked up on the module at call time, not bound at
    import: the test suite's socket guard patches that attribute, and a binding here
    would slip past it.
    """
    request = urllib.request.Request(
        url, headers={"User-Agent": USER_AGENT, "Accept": "application/json"}
    )
    with urllib.request.urlopen(request, timeout=timeout) as response:
        status = getattr(response, "status", 200)
        body = response.read()
    if status != 200:
        raise MeteoError(f"HTTP {status} for {url}")
    return body


def _now() -> str:
    return dt.datetime.now(dt.UTC).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def fetch(
    source: str,
    coordinate: Coordinate,
    start: dt.date,
    end: dt.date,
    *,
    cache: MeteoCache | None = None,
    opener=None,
    retrieved_at: str | None = None,
) -> MeteoResponse:
    """Fetch one range from one source, through the cache, and parse it.

    A cached entry is used without opening a socket. There is no fallback to another
    source: a failure raises `MeteoError` naming `source`, and the caller decides.
    """
    spec = source_named(source)
    if start > end:
        raise MeteoError(f"{spec.name}: the range ends ({end}) before it starts ({start})")
    cache = cache or MeteoCache(DEFAULT_CACHE_DIR)
    key = cache.key(spec, coordinate, start, end)
    entry = cache.read(spec, key)
    if entry is not None:
        return parse(
            spec,
            entry.body,
            coordinate=coordinate,
            start=start,
            end=end,
            provenance=entry.provenance,
            cache_hit=True,
        )
    url = spec.url_for(coordinate, start, end)
    open_url = opener or _urlopen
    try:
        body = open_url(url, TIMEOUT_SECONDS)
    except MeteoError as error:
        raise MeteoError(f"{spec.name}: cannot fetch {url} ({error})") from error
    except (urllib.error.URLError, OSError, TimeoutError) as error:
        raise MeteoError(f"{spec.name}: cannot fetch {url} ({error})") from error
    if not isinstance(body, bytes):
        raise MeteoError(f"{spec.name}: the opener for {url} did not return bytes")
    provenance = {
        "source": spec.name,
        "url": url,
        "retrieved_at": retrieved_at or _now(),
        "coordinate": coordinate.as_dict(),
        "start": start.isoformat(),
        "end": end.isoformat(),
        "licence": spec.licence,
        "attribution": spec.attribution,
        "terms": spec.terms,
        "day_alignment": spec.day_alignment,
        "resolves_civil_day": spec.resolves_civil_day,
        "grid": spec.grid,
        "parameters": [entry.parameter for entry in spec.variables],
        "source_units": {entry.parameter: entry.source_unit for entry in spec.variables},
        "declared_units_map": {
            entry.parameter: f"{entry.source_unit} x {entry.factor} = {entry.unit}"
            for entry in spec.variables
        },
    }
    entry = cache.write(spec, key, body, provenance)
    return parse(
        spec,
        entry.body,
        coordinate=coordinate,
        start=start,
        end=end,
        provenance=entry.provenance,
        cache_hit=False,
    )


def fetch_for_comuna(
    source: str,
    cut: str,
    start: dt.date,
    end: dt.date,
    *,
    centroids=None,
    cache: MeteoCache | None = None,
    opener=None,
) -> MeteoResponse:
    """Fetch for a comuna, using the committed centroid as the point."""
    mapping = centroids or load_centroids()
    try:
        centroid = mapping.centroid(cut)
    except CentroidError as error:
        raise MeteoError(str(error)) from error
    return fetch(source, Coordinate.of(centroid), start, end, cache=cache, opener=opener)


def notice(response: MeteoResponse) -> list[str]:
    """What the source asks a user to know, and what it answered: for the CLI and logs."""
    lines = [
        f"source:       {response.source}",
        f"url:          {response.provenance.get('url', '')}",
        f"licence:      {response.spec.licence}",
        f"attribution:  {response.spec.attribution}",
        f"terms:        {response.spec.terms}",
        f"day:          {response.spec.day_alignment}",
        f"grid:         {response.spec.grid}",
        (
            f"days:         {response.days_returned} returned of {response.days_requested} "
            f"requested ({len(response.missing_days)} missing)"
        ),
        (
            f"cache:        {'hit' if response.cache_hit else 'miss'} "
            f"({response.provenance.get('sha256', '')})"
        ),
        f"retrieved:    {response.provenance.get('retrieved_at', '')}",
    ]
    if response.missing_days:
        shown = ", ".join(date.isoformat() for date in response.missing_days[:5])
        remaining = len(response.missing_days) - 5
        more = "" if remaining <= 0 else f" (+{remaining} more)"
        lines.append(f"missing days: {shown}{more} -- recorded as missing, never as zero")
    return lines


def summary(response: MeteoResponse) -> list[str]:
    """Per-feature coverage of this response, which is what the aggregation will see."""
    lines = []
    for feature in response.spec.features:
        readings = response.readings(feature)
        unit = FEATURE_UNITS[feature]
        if not readings:
            lines.append(f"  {feature:<14} {unit:<7} no reading in this response")
            continue
        values = list(readings.values())
        lines.append(
            f"  {feature:<14} {unit:<7} {len(readings)}/{response.days_requested} days, "
            f"min {min(values):.3f}, max {max(values):.3f}"
        )
    return lines


def main(argv: list[str] | None = None) -> int:
    """Fetch one comuna/range and print what came back, for one named source."""
    parser = argparse.ArgumentParser(
        description=(
            "Fetch a date range for one comuna from one named meteorology source, through the "
            "on-disk cache, and print the days returned against the days requested."
        )
    )
    parser.add_argument("--source", required=True, help=f"one of {', '.join(sorted(SOURCES))}")
    parser.add_argument("--cut", required=True, help="comuna CUT, for example 05101")
    parser.add_argument("--start", required=True, help="first date, YYYY-MM-DD")
    parser.add_argument("--end", required=True, help="last date, YYYY-MM-DD")
    parser.add_argument(
        "--cache", type=Path, default=None, help="cache root (default: data/raw/meteo)"
    )
    parser.add_argument(
        "--centroids", type=Path, default=None, help="centroid data file (default: committed)"
    )
    arguments = parser.parse_args(argv)
    try:
        start = dt.date.fromisoformat(arguments.start)
        end = dt.date.fromisoformat(arguments.end)
        centroids = load_centroids(arguments.centroids) if arguments.centroids else None
        response = fetch_for_comuna(
            arguments.source,
            arguments.cut,
            start,
            end,
            centroids=centroids,
            cache=MeteoCache(arguments.cache) if arguments.cache else None,
        )
    except (MeteoError, CentroidError, ValueError) as error:
        print(f"error: {error}", file=sys.stderr)
        return 2
    for line in notice(response):
        print(line)
    print("declared units:")
    for feature, unit in response.source_units.items():
        print(f"  {feature:<14} {unit}")
    print("coverage:")
    for line in summary(response):
        print(line)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
