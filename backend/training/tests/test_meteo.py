"""The meteorology client: cache, units, missing days, and no silent source switch.

Every test runs offline. Two of them prove that property twice over: the autouse
socket guard in `conftest.py` blocks the real `urlopen` and the socket constructor,
and the cache test swaps in an opener that raises if it is ever called. The recorded
responses under `fixtures/meteo/` are real bytes from both sources; see the
PROVENANCE.md next to them.
"""

from __future__ import annotations

import datetime as dt
import hashlib
import json
import socket
import urllib.error
import urllib.request
from pathlib import Path

import pytest

import meteo
from centroids import load_centroids

TESTS_ROOT = Path(__file__).resolve().parent
METEO_FIXTURES = TESTS_ROOT / "fixtures" / "meteo"

#: The window both fixtures were recorded for: seven days across the New Year, so the
#: two-calendar-year arithmetic is exercised by real data as well as by the
#: constructed cases in test_seasons.py.
START = dt.date(2015, 12, 28)
END = dt.date(2016, 1, 3)

OPEN_METEO_FIXTURE = "open-meteo-05101-2015-12-28_2016-01-03.json"
NASA_POWER_FIXTURE = "nasa-power-05101-2015-12-28_2016-01-03.json"

#: The fixture was recorded for this comuna's committed centroid, so a test can drive
#: `fetch_for_comuna` and land on exactly the URL the bytes came from.
CENTROID = load_centroids().centroid("05101")
COORDINATE = meteo.Coordinate.of(CENTROID)


def body(name: str) -> bytes:
    return (METEO_FIXTURES / name).read_bytes()


def mutated(name: str, edit) -> bytes:
    """Return a RECORDED response with one field changed.

    The fixture on disk is never edited: the mutation is what a test needs to drive a
    refusal path (a changed unit, a changed calendar, a fill value) that no honest
    recording would contain.
    """
    document = json.loads(body(name).decode("utf-8"))
    edit(document)
    return json.dumps(document).encode("utf-8")


def parse_open_meteo(raw: bytes | None = None) -> meteo.MeteoResponse:
    return meteo.parse(
        meteo.source_named("open-meteo"),
        raw if raw is not None else body(OPEN_METEO_FIXTURE),
        coordinate=COORDINATE,
        start=START,
        end=END,
    )


def parse_nasa_power(raw: bytes | None = None) -> meteo.MeteoResponse:
    return meteo.parse(
        meteo.source_named("nasa-power"),
        raw if raw is not None else body(NASA_POWER_FIXTURE),
        coordinate=COORDINATE,
        start=START,
        end=END,
    )


def opener_returning(payload: bytes, calls: list[str] | None = None):
    def opener(url, timeout):
        if calls is not None:
            calls.append(url)
        return payload

    return opener


def opener_refusing(url, timeout):
    raise urllib.error.URLError("no route to host")


# ---------------------------------------------------------------------------
# Parsing real responses
# ---------------------------------------------------------------------------


def test_the_open_meteo_fixture_is_seven_civil_days_of_one_calendar():
    response = parse_open_meteo()
    assert response.days_returned == 7
    assert response.days_requested == 7
    assert response.missing_days == ()
    assert [day.date for day in response.days] == list(response.dates_requested)
    assert response.declared["timezone"] == meteo.TIMEZONE
    assert response.declared["utc_offset_used_for_days"] is False


def test_open_meteo_wind_is_converted_from_kmh_after_the_unit_is_matched():
    document = json.loads(body(OPEN_METEO_FIXTURE).decode("utf-8"))
    raw_values = document["daily"]["wind_speed_10m_max"]
    response = parse_open_meteo()
    assert response.source_units["wind_speed"] == "km/h"
    for index, day in enumerate(response.days):
        assert day.values["wind_speed"] == pytest.approx(raw_values[index] / 3.6)
    # The conversion is a real one: km/h and m/s are never the same number.
    assert response.days[0].values["wind_speed"] != pytest.approx(raw_values[0])


def test_radiation_is_a_daily_integral_in_mj_per_m2_not_watts_per_m2():
    document = json.loads(body(OPEN_METEO_FIXTURE).decode("utf-8"))
    raw_values = document["daily"]["shortwave_radiation_sum"]
    response = parse_open_meteo()
    assert meteo.FEATURE_UNITS["radiation_sol"] == "MJ/m2"
    assert response.source_units["radiation_sol"] == "MJ/m\u00b2"
    # No conversion: the source already delivers the declared unit.
    assert [day.values["radiation_sol"] for day in response.days] == pytest.approx(raw_values)
    # A real Chilean summer day is 15-40 MJ/m2 per day, not 15-40 W/m2.
    assert all(5.0 < value < 45.0 for value in raw_values)


def test_nasa_power_declares_its_own_day_boundary_and_it_is_not_civil():
    response = parse_nasa_power()
    assert response.days_returned == 7
    assert response.declared["time_standard"] == "LST"
    assert response.declared["fill_value"] == -999.0
    assert response.spec.resolves_civil_day is False
    assert "NOT guaranteed" in response.spec.day_alignment


def test_nasa_power_keeps_millimetres_and_the_daily_integral_unconverted():
    response = parse_nasa_power()
    assert response.source_units["precip_sum"] == "mm/day"
    assert response.source_units["radiation_sol"] == "MJ/m^2/day"
    assert response.readings("precip_sum")
    assert all(value >= 0.0 for value in response.readings("precip_sum").values())
    assert all(value > 0.0 for value in response.readings("radiation_sol").values())


# ---------------------------------------------------------------------------
# Missing data: recorded, never zero-filled
# ---------------------------------------------------------------------------


def test_a_day_the_response_omits_is_recorded_as_missing_and_never_zero():
    def drop_one_day(document):
        dropped = document["daily"]["time"][0]
        document["daily"]["_dropped"] = dropped
        for series in document["daily"].values():
            if isinstance(series, list):
                # `time` is one of these lists, so the first element of every series
                # goes, including the date itself -- the whole day disappears.
                series.pop(0)

    dropped_text: list[str] = []
    raw = mutated(
        OPEN_METEO_FIXTURE,
        lambda document: (
            drop_one_day(document),
            dropped_text.append(document["daily"]["_dropped"]),
        ),
    )
    dropped = dt.date.fromisoformat(dropped_text[0])
    response = parse_open_meteo(raw)
    assert response.days_returned == 6
    assert response.days_requested == 7
    assert response.missing_days == (dropped,)
    assert dropped not in response.observations()
    # And nothing anywhere turned that absence into a measurement.
    for feature in meteo.FEATURE_UNITS:
        assert dropped not in response.readings(feature)


def test_a_null_value_on_a_day_that_is_present_is_absent_not_zero():
    def null_one_day(document):
        document["daily"]["precipitation_sum"][2] = None

    response = parse_open_meteo(mutated(OPEN_METEO_FIXTURE, null_one_day))
    absent = response.dates_requested[2]
    assert response.days_returned == 7  # the day is there
    assert absent in response.observations()  # and it is a day
    assert absent not in response.readings("precip_sum")  # but it has no reading
    assert response.readings("temp_max")  # other variables on that day are untouched


def test_the_nasa_power_fill_value_is_read_as_no_reading():
    def fill_one_day(document):
        document["properties"]["parameter"]["RH2M"]["20160101"] = -999.0

    response = parse_nasa_power(mutated(NASA_POWER_FIXTURE, fill_one_day))
    assert len(response.readings("humidity_rel")) == 6
    assert dt.date(2016, 1, 1) not in response.readings("humidity_rel")
    assert dt.date(2016, 1, 1) in response.observations()


# ---------------------------------------------------------------------------
# Refusals: an error naming the source, never an empty panel
# ---------------------------------------------------------------------------


@pytest.mark.parametrize("source", ["open-meteo", "nasa-power"])
def test_an_empty_body_is_an_error_naming_the_source(source):
    with pytest.raises(meteo.MeteoError) as refusal:
        meteo.parse(meteo.source_named(source), b"", coordinate=COORDINATE, start=START, end=END)
    assert source in str(refusal.value)
    assert "empty" in str(refusal.value)


@pytest.mark.parametrize("source", ["open-meteo", "nasa-power"])
def test_a_malformed_body_is_an_error_naming_the_source(source):
    with pytest.raises(meteo.MeteoError) as refusal:
        meteo.parse(
            meteo.source_named(source),
            b"<html><body>502 Bad Gateway</body></html>",
            coordinate=COORDINATE,
            start=START,
            end=END,
        )
    assert source in str(refusal.value)
    assert "not JSON" in str(refusal.value)


def test_a_source_error_document_names_the_source_and_the_reason():
    raw = json.dumps({"error": True, "reason": "Parameter 'daily' is invalid"}).encode("utf-8")
    with pytest.raises(meteo.MeteoError) as refusal:
        meteo.parse(
            meteo.source_named("open-meteo"), raw, coordinate=COORDINATE, start=START, end=END
        )
    assert "open-meteo" in str(refusal.value)
    assert "Parameter 'daily' is invalid" in str(refusal.value)


def test_a_response_that_covers_none_of_the_requested_range_is_refused():
    with pytest.raises(meteo.MeteoError) as refusal:
        meteo.parse(
            meteo.source_named("open-meteo"),
            body(OPEN_METEO_FIXTURE),
            coordinate=COORDINATE,
            start=dt.date(1999, 1, 1),
            end=dt.date(1999, 1, 7),
        )
    assert "open-meteo" in str(refusal.value)
    assert "covers none of" in str(refusal.value)


def test_a_changed_declared_unit_is_refused_rather_than_converted():
    def change_unit(document):
        document["daily_units"]["wind_speed_10m_max"] = "m/s"

    with pytest.raises(meteo.MeteoError) as refusal:
        parse_open_meteo(mutated(OPEN_METEO_FIXTURE, change_unit))
    message = str(refusal.value)
    assert "open-meteo" in message
    assert "'m/s'" in message
    assert "'km/h'" in message


def test_a_response_that_resolved_another_calendar_is_refused():
    def to_utc(document):
        document["timezone"] = "GMT"
        document["utc_offset_seconds"] = 0

    with pytest.raises(meteo.MeteoError) as refusal:
        parse_open_meteo(mutated(OPEN_METEO_FIXTURE, to_utc))
    message = str(refusal.value)
    assert "open-meteo" in message
    assert "GMT" in message
    assert meteo.TIMEZONE in message


def test_a_zero_offset_is_refused_even_when_the_calendar_echo_still_says_santiago():
    def zero_offset(document):
        document["utc_offset_seconds"] = 0

    with pytest.raises(meteo.MeteoError) as refusal:
        parse_open_meteo(mutated(OPEN_METEO_FIXTURE, zero_offset))
    assert "utc_offset_seconds is 0" in str(refusal.value)


def test_an_unknown_source_name_is_refused_and_the_known_ones_are_named():
    with pytest.raises(meteo.MeteoError) as refusal:
        meteo.source_named("era5")
    message = str(refusal.value)
    assert "era5" in message
    assert "nasa-power" in message and "open-meteo" in message


# ---------------------------------------------------------------------------
# The cache
# ---------------------------------------------------------------------------


def test_the_cache_key_is_keyed_by_source_coordinate_and_range(tmp_path):
    cache = meteo.MeteoCache(tmp_path)
    open_meteo = meteo.source_named("open-meteo")
    power = meteo.source_named("nasa-power")
    base = cache.key(open_meteo, COORDINATE, START, END)
    assert base != cache.key(power, COORDINATE, START, END)
    moved = meteo.Coordinate(COORDINATE.latitude + 0.5, COORDINATE.longitude)
    assert base != cache.key(open_meteo, moved, START, END)
    assert base != cache.key(open_meteo, COORDINATE, START, END - dt.timedelta(days=1))


def test_a_fresh_fetch_stores_the_bytes_with_url_instant_sha256_and_terms(tmp_path):
    payload = body(OPEN_METEO_FIXTURE)
    response = meteo.fetch(
        "open-meteo",
        COORDINATE,
        START,
        END,
        cache=meteo.MeteoCache(tmp_path),
        opener=opener_returning(payload),
        retrieved_at="2026-11-06T00:00:00Z",
    )
    assert response.cache_hit is False
    provenance = response.provenance
    assert provenance["url"] == meteo.source_named("open-meteo").url_for(COORDINATE, START, END)
    assert provenance["retrieved_at"] == "2026-11-06T00:00:00Z"
    assert provenance["sha256"] == hashlib.sha256(payload).hexdigest()
    assert provenance["bytes"] == len(payload)
    assert "non-commercial" in provenance["licence"]
    assert "credential" in provenance["terms"]
    key = meteo.MeteoCache(tmp_path).key(meteo.source_named("open-meteo"), COORDINATE, START, END)
    stored = tmp_path / "open-meteo" / f"{key}.json"
    assert stored.read_bytes() == payload


def test_a_second_run_reads_the_cache_and_never_opens_a_socket(tmp_path):
    cache = meteo.MeteoCache(tmp_path)
    calls: list[str] = []
    first = meteo.fetch(
        "open-meteo",
        COORDINATE,
        START,
        END,
        cache=cache,
        opener=opener_returning(body(OPEN_METEO_FIXTURE), calls),
    )
    assert first.cache_hit is False
    assert len(calls) == 1

    def poison(url, timeout):
        raise AssertionError("the cache was not used: a network call happened")

    second = meteo.fetch("open-meteo", COORDINATE, START, END, cache=cache, opener=poison)
    assert second.cache_hit is True
    assert [day.values for day in second.days] == [day.values for day in first.days]
    assert second.provenance["sha256"] == first.provenance["sha256"]
    # The retrieval instant belongs to the fetch, not to the read.
    assert second.provenance["retrieved_at"] == first.provenance["retrieved_at"]


def test_a_cache_entry_whose_bytes_changed_is_refused_not_parsed(tmp_path):
    cache = meteo.MeteoCache(tmp_path)
    meteo.fetch(
        "open-meteo",
        COORDINATE,
        START,
        END,
        cache=cache,
        opener=opener_returning(body(OPEN_METEO_FIXTURE)),
    )
    spec = meteo.source_named("open-meteo")
    key = cache.key(spec, COORDINATE, START, END)
    stored = tmp_path / "open-meteo" / f"{key}.json"
    stored.write_bytes(stored.read_bytes().replace(b"19.", b"99."))
    with pytest.raises(meteo.MeteoError) as refusal:
        meteo.fetch("open-meteo", COORDINATE, START, END, cache=cache, opener=opener_refusing)
    assert "open-meteo" in str(refusal.value)
    assert "cache entry" in str(refusal.value)


def test_fetch_for_comuna_asks_for_the_committed_centroid(tmp_path):
    calls: list[str] = []
    meteo.fetch_for_comuna(
        "open-meteo",
        "05101",
        START,
        END,
        cache=meteo.MeteoCache(tmp_path),
        opener=opener_returning(body(OPEN_METEO_FIXTURE), calls),
    )
    assert len(calls) == 1
    assert f"latitude={CENTROID.latitude:.6f}" in calls[0]
    assert f"longitude={CENTROID.longitude:.6f}" in calls[0]


# ---------------------------------------------------------------------------
# No silent source switch
# ---------------------------------------------------------------------------


def test_a_failing_source_is_named_and_the_other_one_is_never_called(tmp_path):
    calls: list[str] = []

    def failing(url, timeout):
        calls.append(url)
        raise urllib.error.URLError("connection refused")

    with pytest.raises(meteo.MeteoError) as refusal:
        meteo.fetch(
            "open-meteo",
            COORDINATE,
            START,
            END,
            cache=meteo.MeteoCache(tmp_path),
            opener=failing,
        )
    message = str(refusal.value)
    assert "open-meteo" in message
    assert "connection refused" in message
    # One attempt, on one host: nothing fell back to POWER.
    assert len(calls) == 1
    assert "archive-api.open-meteo.com" in calls[0]


def test_the_socket_guard_actually_bites():
    """The autouse guard is the reason every other test is offline; prove it works."""
    with pytest.raises(RuntimeError, match="must not open a network connection"):
        urllib.request.urlopen("https://example.invalid/")
    with pytest.raises(RuntimeError, match="must not open a network connection"):
        socket.socket().connect(("127.0.0.1", 9))
    with pytest.raises(RuntimeError, match="must not open a network connection"):
        socket.create_connection(("127.0.0.1", 9))


# ---------------------------------------------------------------------------
# The vocabulary is the artifact's, and the fixtures are the recorded bytes
# ---------------------------------------------------------------------------


def test_the_feature_vocabulary_is_the_artifact_vocabulary():
    from assemble import load_artifact

    artifact = load_artifact(meteo.REPO_ROOT / "backend/services/inference/testdata/logistic.json")
    assert set(meteo.FEATURE_UNITS) | {"wind_dir"} == set(artifact.names)


@pytest.mark.parametrize("name", [OPEN_METEO_FIXTURE, NASA_POWER_FIXTURE])
def test_every_fixture_is_listed_in_the_provenance_with_its_own_sha256(name):
    provenance = (METEO_FIXTURES / "PROVENANCE.md").read_text(encoding="utf-8")
    digest = hashlib.sha256(body(name)).hexdigest()
    assert name in provenance
    assert digest in provenance
