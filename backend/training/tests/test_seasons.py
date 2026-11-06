"""Season aggregation: the window, the reducers, and absence instead of zero.

The window is where a UTC/Chile mix-up hides, so two tests exist purely to make that
class of bug fail loudly: one compares instants around both edges of the window, and
one drives a source response whose calendar is UTC through the parser. The rest pin
the aggregate meanings and the rule that an incomplete window is absent with its
coverage count.
"""

from __future__ import annotations

import datetime as dt
import json
from pathlib import Path

import pytest

import assemble
import meteo
import seasons

ZONE = assemble.load_zone()
METEO_FIXTURES = Path(__file__).resolve().parent / "fixtures" / "meteo"
ARTIFACT = assemble.load_artifact(assemble.DEFAULT_ARTIFACT)


def observations_for(window, mapper) -> dict[dt.date, dict[str, float | None]]:
    """`{date: values}` for every civil date of the window, built by `mapper`."""
    return {date: mapper(index, date) for index, date in enumerate(window.dates)}


def constant(values: dict[str, float | None]):
    return lambda index, date: dict(values)


# ---------------------------------------------------------------------------
# The window
# ---------------------------------------------------------------------------


def test_the_default_window_is_first_of_july_to_thirty_first_of_december():
    window = seasons.season_window(2015, zone=ZONE)
    assert window.label == "2015-2016"
    assert window.dates[0] == dt.date(2015, 7, 1)
    assert window.dates[-1] == dt.date(2015, 12, 31)
    assert window.day_count == 184
    assert window.cutoff == dt.date(2015, 12, 31)


def test_the_window_starts_at_local_midnight_and_not_at_utc_midnight():
    window = seasons.season_window(2015, zone=ZONE)
    assert window.start == dt.datetime(2015, 7, 1, 0, 0, tzinfo=ZONE)
    assert window.start.utcoffset() == dt.datetime(2015, 7, 1, 12, 0, tzinfo=ZONE).utcoffset()


def test_the_boundary_is_local_and_would_flip_if_it_became_utc():
    """Both edges are chosen so a UTC cut moves them, in opposite directions."""
    window = seasons.season_window(2015, zone=ZONE)

    # 22:00 local on 31 December is 01:00 UTC on 1 January: inside the window, but its
    # UTC date is NOT one of the window's dates. A UTC cut would drop it.
    inside = dt.datetime(2015, 12, 31, 22, 0, tzinfo=ZONE)
    assert window.covers(inside) is True
    assert inside.astimezone(dt.UTC).date() == dt.date(2016, 1, 1)
    assert inside.astimezone(dt.UTC).date() not in window.dates

    # 23:00 local on 30 June is 03:00 UTC on 1 July: outside the window, but its UTC
    # date IS one of the window's dates. A UTC cut would include it.
    outside = dt.datetime(2015, 6, 30, 23, 0, tzinfo=ZONE)
    assert window.covers(outside) is False
    assert outside.astimezone(dt.UTC).date() == dt.date(2015, 7, 1)
    assert outside.astimezone(dt.UTC).date() in window.dates


def test_a_naive_datetime_is_refused_because_it_names_no_moment():
    with pytest.raises(seasons.WindowError):
        seasons.season_window(2015, zone=ZONE).covers(dt.datetime(2015, 12, 31, 22, 0))


def test_a_cutoff_before_july_belongs_to_the_following_calendar_year():
    window = seasons.season_window(2015, month_day=(3, 31), zone=ZONE)
    assert window.cutoff == dt.date(2016, 3, 31)
    assert window.dates[-1] == dt.date(2016, 3, 31)
    assert window.day_count == 275


def test_a_full_season_window_runs_to_the_end_of_june_and_includes_leap_february():
    window = seasons.season_window(2015, month_day=(6, 30), zone=ZONE)
    assert window.dates[0] == dt.date(2015, 7, 1)
    assert window.dates[-1] == dt.date(2016, 6, 30)
    assert window.day_count == 366  # 2016 is a leap year, and 29 February is inside


def test_the_window_dates_are_the_same_civil_day_window_the_panel_uses():
    window = seasons.season_window(2015, zone=ZONE)
    assert assemble.civil_day_window(window.dates[-1], window.day_count) == window.dates


def test_the_training_range_is_the_1985_to_2023_seasons():
    windows = seasons.season_windows(zone=ZONE)
    assert len(windows) == 39
    assert windows[0].label == "1985-1986"
    assert windows[-1].label == "2023-2024"
    assert windows[0].dates[0] == dt.date(1985, 7, 1)
    assert windows[-1].dates[-1] == dt.date(2023, 12, 31)


@pytest.mark.parametrize("year", [1984, 2024])
def test_a_season_outside_the_training_range_is_refused(year):
    with pytest.raises(seasons.WindowError) as refusal:
        seasons.season_window(year, zone=ZONE)
    assert "outside the training range" in str(refusal.value)


def test_every_legal_cutoff_lands_inside_its_season():
    """The year is resolved from the month, so no MM-DD can escape the season."""
    day = dt.date(2000, 1, 1)  # a leap year, so 02-29 is exercised
    season_open = dt.date(2015, 7, 1)
    season_close = dt.date(2016, 6, 30)
    while day.year == 2000:
        window = seasons.season_window(2015, month_day=(day.month, day.day), zone=ZONE)
        assert season_open <= window.dates[0] <= window.dates[-1] <= season_close
        assert window.dates[-1] == window.cutoff
        assert window.day_count >= 1
        day += dt.timedelta(days=1)
    # February 29th is a legal cut-off, and it belongs to the second calendar year.
    assert seasons.season_window(2015, month_day=(2, 29), zone=ZONE).cutoff == dt.date(2016, 2, 29)


@pytest.mark.parametrize("text", ["31-12", "12/31", "13-01", "", "febrero"])
def test_a_malformed_cutoff_is_refused(text):
    with pytest.raises(seasons.WindowError):
        seasons.parse_cutoff(text)


# ---------------------------------------------------------------------------
# The aggregates
# ---------------------------------------------------------------------------


def test_a_season_spanning_two_calendar_years_aggregates_both_of_them():
    window = seasons.season_window(2015, month_day=(6, 30), zone=ZONE)
    assert window.dates[0].year == 2015
    assert window.dates[-1].year == 2016

    def mapper(index, date):
        return {
            # The highest maximum is in January 2016, so only a window that reaches the
            # second calendar year can produce it.
            "temp_max": 40.0 if date.year == 2016 else 20.0,
            "temp_min": 1.0 if date.year == 2016 else 9.0,
            "humidity_rel": 50.0,
            "wind_speed": 7.0 if date.year == 2016 else 2.0,
            "precip_sum": 1.0,
            "radiation_sol": 5.0,
        }

    aggregates = seasons.aggregate(observations_for(window, mapper), window)
    assert aggregates["temp_max"].value == pytest.approx(40.0)
    assert aggregates["temp_min"].value == pytest.approx(1.0)
    assert aggregates["wind_speed"].value == pytest.approx(7.0)
    # 366 days, wet on every one of them: the sum spans both years.
    assert aggregates["precip_sum"].value == pytest.approx(366.0)
    assert aggregates["radiation_sol"].value == pytest.approx(5.0 * 366)


def test_each_aggregate_uses_the_reducer_the_feature_declares():
    window = seasons.season_window(2015, zone=ZONE)
    count = window.day_count

    def mapper(index, date):
        return {
            "temp_max": 10.0 + index / count,
            "temp_min": 5.0 - index / count,
            "humidity_rel": 60.0,
            "wind_speed": 3.0 + index,
            "precip_sum": 1.0 if index % 2 == 0 else 0.0,
            "radiation_sol": 10.0,
        }

    aggregates = seasons.aggregate(observations_for(window, mapper), window)
    assert aggregates["temp_max"].value == pytest.approx(10.0 + (count - 1) / count)
    assert aggregates["temp_min"].value == pytest.approx(5.0 - (count - 1) / count)
    assert aggregates["humidity_rel"].value == pytest.approx(60.0)
    assert aggregates["wind_speed"].value == pytest.approx(3.0 + count - 1)
    assert aggregates["precip_sum"].value == pytest.approx(float((count + 1) // 2))
    assert aggregates["radiation_sol"].value == pytest.approx(10.0 * count)
    for feature, entry in aggregates.items():
        assert entry.covered_days == count, feature
        assert entry.window_days == count, feature


@pytest.mark.parametrize("feature", seasons.FEATURES)
def test_an_incomplete_window_is_absent_with_its_coverage_never_zero(feature):
    window = seasons.season_window(2015, zone=ZONE)
    complete = observations_for(window, constant(dict.fromkeys(seasons.FEATURES, 1.0)))
    missing = window.dates[10]
    del complete[missing]
    aggregates = seasons.aggregate(complete, window)
    entry = aggregates[feature]
    assert entry.value is None, feature
    assert entry.covered_days == window.day_count - 1, feature
    assert entry.window_days == window.day_count, feature
    assert feature in entry.reason or "ventana" in entry.reason


def test_a_season_with_no_observations_at_all_is_absent_rather_than_zero():
    window = seasons.season_window(2015, zone=ZONE)
    aggregates = seasons.aggregate({}, window)
    for feature in seasons.FEATURES:
        assert aggregates[feature].value is None
        assert aggregates[feature].covered_days == 0


def test_a_measured_zero_is_kept_and_is_not_confused_with_absence():
    """Dry days are real measurements: 0.0 mm must survive a complete window."""
    window = seasons.season_window(2015, zone=ZONE)
    dry = constant(
        {
            "temp_max": 25.0,
            "temp_min": 10.0,
            "humidity_rel": 40.0,
            "wind_speed": 0.0,
            "precip_sum": 0.0,
            "radiation_sol": 0.0,
        }
    )
    aggregates = seasons.aggregate(observations_for(window, dry), window)
    assert aggregates["precip_sum"].value == pytest.approx(0.0)
    assert aggregates["radiation_sol"].value == pytest.approx(0.0)
    assert aggregates["wind_speed"].value == pytest.approx(0.0)
    for feature in ("precip_sum", "radiation_sol", "wind_speed"):
        assert aggregates[feature].present is True, feature


def test_days_outside_the_window_are_dropped_and_cannot_widen_the_feature():
    window = seasons.season_window(2015, zone=ZONE)
    observations = observations_for(window, constant(dict.fromkeys(seasons.FEATURES, 1.0)))
    before = window.dates[0] - dt.timedelta(days=1)
    after = window.dates[-1] + dt.timedelta(days=1)
    observations[before] = dict.fromkeys(seasons.FEATURES, 1000.0)
    observations[after] = dict.fromkeys(seasons.FEATURES, 1000.0)
    aggregates = seasons.aggregate(observations, window)
    assert aggregates["temp_max"].value == pytest.approx(1.0)
    assert aggregates["precip_sum"].value == pytest.approx(float(window.day_count))


# ---------------------------------------------------------------------------
# The row
# ---------------------------------------------------------------------------


def test_a_row_records_the_source_that_produced_it_and_its_day_alignment():
    window = seasons.season_window(2015, zone=ZONE)
    observations = observations_for(window, constant(dict.fromkeys(seasons.FEATURES, 1.0)))
    civil = seasons.season_row(
        cut="05101",
        comuna="Valparaíso",
        provincia="Valparaíso",
        window=window,
        observations=observations,
        source="open-meteo",
    )
    assert civil.source == "open-meteo"
    assert civil.resolves_civil_day is True
    assert civil.days_requested == civil.days_returned == window.day_count
    assert civil.days_missing == 0

    lst = seasons.season_row(
        cut="05101",
        comuna="Valparaíso",
        provincia="Valparaíso",
        window=window,
        observations=observations,
        source="nasa-power",
    )
    assert lst.source == "nasa-power"
    assert lst.resolves_civil_day is False
    assert "LST" in lst.day_alignment
    assert "false" in lst.as_row()


def test_the_csv_cell_for_an_absent_feature_is_empty_and_its_coverage_is_written():
    window = seasons.season_window(2015, zone=ZONE)
    row = seasons.season_row(
        cut="05101",
        comuna="Valparaíso",
        provincia="Valparaíso",
        window=window,
        observations={},
        source="open-meteo",
    )
    header = seasons.panel_columns()
    cells = row.as_row()
    assert len(cells) == len(header)
    record = dict(zip(header, cells, strict=True))
    for feature in seasons.FEATURES:
        assert record[feature] == "", feature
        assert record[f"{feature}_covered_days"] == "0", feature


def test_a_row_built_from_the_recorded_fixture_is_absent_with_the_days_it_covers():
    """Real bytes, real window: 4 of 184 days cannot produce a season aggregate."""
    window = seasons.season_window(2015, zone=ZONE)
    raw = (METEO_FIXTURES / "open-meteo-05101-2015-12-28_2016-01-03.json").read_bytes()
    response = meteo.parse(
        meteo.source_named("open-meteo"),
        raw,
        coordinate=meteo.Coordinate.of(__import__("centroids").load_centroids().centroid("05101")),
        start=dt.date(2015, 12, 28),
        end=dt.date(2016, 1, 3),
    )
    row = seasons.season_row(
        cut="05101",
        comuna="Valparaíso",
        provincia="Valparaíso",
        window=window,
        observations=response.observations(),
        source="open-meteo",
    )
    # The fixture is 2015-12-28 .. 2016-01-03; four of those days are in the window.
    assert row.days_returned == 4
    assert row.days_returned < row.days_requested
    assert row.days_missing == window.day_count - 4
    for feature in seasons.FEATURES:
        assert row.features[feature].value is None, feature
        assert row.features[feature].covered_days == 4, feature


# ---------------------------------------------------------------------------
# The artifact contract
# ---------------------------------------------------------------------------


def test_the_radiation_unit_conflict_is_reported_and_nothing_is_converted():
    check = seasons.check_artifact_units(ARTIFACT)
    conflicts = {conflict["feature"]: conflict for conflict in check["conflicts"]}
    assert set(conflicts) == {"radiation_sol"}
    assert conflicts["radiation_sol"]["source_unit"] == "MJ/m2"
    assert conflicts["radiation_sol"]["artifact_unit"] == "W/m2"
    assert conflicts["radiation_sol"]["kind"] == "unit-conflict"
    assert "artifact descriptor" in conflicts["radiation_sol"]["resolution"]


def test_the_artifact_feature_this_source_cannot_produce_is_named():
    check = seasons.check_artifact_units(ARTIFACT)
    assert check["not_produced_here"] == ["wind_dir"]
    assert "wind_dir" in check["not_produced_reason"]


def test_an_artifact_that_declares_the_same_units_has_no_conflict():
    matching = assemble.Artifact(
        version="test-v1",
        features=tuple(
            assemble.ArtifactFeature(
                name=feature, unit=meteo.FEATURE_UNITS[feature], minimum=0.0, maximum=1.0
            )
            for feature in meteo.FEATURE_UNITS
        )
        + (assemble.ArtifactFeature(name="wind_dir", unit="deg", minimum=0.0, maximum=360.0),),
    )
    check = seasons.check_artifact_units(matching)
    assert check["conflicts"] == []
    assert check["not_produced_here"] == ["wind_dir"]


# ---------------------------------------------------------------------------
# The manifest
# ---------------------------------------------------------------------------


def build_two_centroid_panel():
    centroids = __import__("centroids").load_centroids()
    selected = [centroids.centroid("05101"), centroids.centroid("05601")]
    mapping = type(centroids)(
        region_cut=centroids.region_cut,
        region_name=centroids.region_name,
        entries=tuple(selected),
        method=centroids.method,
        provenance=centroids.provenance,
    )
    windows = seasons.season_windows(first=2015, last=2016, zone=ZONE)
    complete = constant(dict.fromkeys(seasons.FEATURES, 2.0))
    panel = seasons.build_panel(
        mapping,
        windows,
        source="open-meteo",
        observations_for=lambda centroid, window: observations_for(window, complete),
    )
    manifest = seasons.build_manifest(
        panel=panel,
        source="open-meteo",
        cutoff_note="12-31",
        artifact=ARTIFACT,
        artifact_check=seasons.check_artifact_units(ARTIFACT),
        generated_at="2026-11-06T00:00:00Z",
    )
    return panel, manifest


def test_the_manifest_counts_rows_seasons_and_coverage():
    _, manifest = build_two_centroid_panel()
    assert manifest["counts"]["rows"] == 4  # 2 comunas x 2 seasons
    assert manifest["counts"]["comunas"] == 2
    assert manifest["counts"]["seasons"] == 2
    assert manifest["counts"]["first_season"] == "2015-2016"
    assert manifest["counts"]["last_season"] == "2016-2017"
    assert manifest["counts"]["rows_with_no_feature"] == 0
    for feature in seasons.FEATURES:
        stats = manifest["counts"]["features"][feature]
        assert stats["present_rows"] == 4, feature
        assert stats["absent_rows"] == 0, feature
        assert stats["completely_covered_rows"] == 4, feature


def test_the_manifest_declares_the_terms_the_window_and_the_source_alignment():
    _, manifest = build_two_centroid_panel()
    assert manifest["source"]["name"] == "open-meteo"
    assert "non-commercial" in manifest["source"]["licence"]
    assert manifest["source"]["resolves_civil_day"] is True
    assert manifest["window"]["timezone"] == seasons.TIMEZONE
    assert manifest["window"]["season_start"] == "07-01"
    assert manifest["window"]["cutoff"] == "12-31"
    assert manifest["window"]["days_per_season"] == {"2015-2016": 184, "2016-2017": 184}
    assert manifest["window"]["windows"][0]["start_instant"].endswith("-03:00")
    assert manifest["contract"]["conflicts"][0]["feature"] == "radiation_sol"


def test_the_manifest_records_the_absences_rather_than_hiding_them():
    centroids = __import__("centroids").load_centroids()
    selected = [centroids.centroid("05101")]
    mapping = type(centroids)(
        region_cut=centroids.region_cut,
        region_name=centroids.region_name,
        entries=tuple(selected),
        method=centroids.method,
        provenance=centroids.provenance,
    )
    windows = seasons.season_windows(first=2015, last=2015, zone=ZONE)
    panel = seasons.build_panel(
        mapping, windows, source="nasa-power", observations_for=lambda centroid, window: {}
    )
    manifest = seasons.build_manifest(
        panel=panel,
        source="nasa-power",
        cutoff_note="12-31",
        artifact=ARTIFACT,
        artifact_check=seasons.check_artifact_units(ARTIFACT),
        generated_at="2026-11-06T00:00:00Z",
    )
    assert manifest["counts"]["rows_with_no_feature"] == 1
    for feature in seasons.FEATURES:
        assert manifest["counts"]["features"][feature]["absent_rows"] == 1
    assert panel.rows[0].resolves_civil_day is False


def test_the_panel_is_written_with_each_feature_and_its_coverage(tmp_path):
    panel, _ = build_two_centroid_panel()
    path = tmp_path / "seasons.csv"
    seasons.write_panel(path, panel)
    lines = path.read_text(encoding="utf-8").splitlines()
    header = lines[0].split(",")
    assert len(lines) == len(panel.rows) + 1
    for feature in seasons.FEATURES:
        assert feature in header
        assert f"{feature}_covered_days" in header
    assert header[: len(seasons.PANEL_COLUMNS)] == list(seasons.PANEL_COLUMNS)


def test_the_manifest_is_json_with_the_source_terms_and_the_window():
    _, manifest = build_two_centroid_panel()
    round_tripped = json.loads(json.dumps(manifest, ensure_ascii=False))
    assert round_tripped["generated_by"] == "backend/training/seasons.py"
    assert round_tripped["features"][0]["unit"] in meteo.FEATURE_UNITS.values()
