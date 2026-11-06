"""The panel: key, label, grid, and the cells this source cannot fill.

Every test here runs against the committed fixtures, offline. The numbers are
hard-coded on purpose: they are the contract of the fixture (44 events, 30 fire
comuna-days, 84 comuna-days), and a change in any of them is a change in what the
panel means rather than a test that needs updating.
"""

from __future__ import annotations

import csv
import datetime as dt
import json
from pathlib import Path

import pytest
from conftest import SAMPLE_FIXTURE, SYNTHETIC_FIXTURE

import assemble


def build(source: Path = SAMPLE_FIXTURE, **kwargs):
    """Assemble a panel from a fixture, with a fixed manifest timestamp."""
    return assemble.assemble(source, generated_at="2026-11-06T00:00:00Z", **kwargs)


@pytest.fixture(scope="module")
def sample():
    return build()


def row_of(panel: assemble.Panel, cut: str, date: str) -> assemble.PanelRow:
    for row in panel.rows:
        if row.cut == cut and row.local_date.isoformat() == date:
            return row
    raise AssertionError(f"no row for cut {cut} on {date}")


def test_two_events_on_one_comuna_day_collapse_into_one_row(sample):
    panel, _manifest = sample
    talca = row_of(panel, "07101", "2018-11-26")
    assert talca.fire_today == 1
    assert talca.fire_count == 2
    assert talca.burned_ha == pytest.approx(0.67)  # 0.02 + 0.65
    matching = [
        row
        for row in panel.rows
        if row.comuna == "Talca" and row.local_date.isoformat() == "2018-11-26"
    ]
    assert len(matching) == 1


def test_four_events_on_one_comuna_day_are_counted_not_deduplicated(sample):
    panel, manifest = sample
    talca = row_of(panel, "07101", "2018-12-18")
    assert talca.fire_count == 4
    assert talca.burned_ha == pytest.approx(2.3)  # 0.3 + 1.6 + 0.2 + 0.2
    assert manifest["counts"]["events_collapsed_into_a_comuna_day"] == 44 - 30


def test_single_event_comuna_day_keeps_its_own_area(sample):
    panel, _manifest = sample
    cauquenes = row_of(panel, "07201", "2018-11-25")
    assert cauquenes.fire_count == 1
    assert cauquenes.burned_ha == pytest.approx(0.1)


def test_panel_is_a_comuna_day_grid_with_real_quiet_days(sample):
    panel, manifest = sample
    # 2 comunas x 42 local dates (2018-11-20 .. 2018-12-31).
    assert manifest["counts"]["comuna_days"] == 84
    assert len(panel.rows) == 84
    assert manifest["counts"]["comuna_days_with_fire"] == 30
    assert manifest["counts"]["comuna_days_without_fire"] == 54
    assert sum(row.fire_count for row in panel.rows) == 44
    quiet = [row for row in panel.rows if not row.fire_today]
    assert quiet
    for row in quiet:
        # A day without a fire burned nothing: this zero is the label's own fact,
        # not a substitute for a missing measurement.
        assert row.burned_ha == 0.0
        assert row.fire_count == 0
    for row in panel.rows:
        assert row.fire_today == (1 if row.fire_count else 0)


def test_dates_come_from_the_source_window(sample):
    _panel, manifest = sample
    assert manifest["dates"] == {
        "first_local_date": "2018-11-20",
        "last_local_date": "2018-12-31",
    }


def test_null_dates_are_excluded_and_counted_never_imputed():
    panel, manifest = build(SYNTHETIC_FIXTURE)
    counts = manifest["counts"]
    assert counts["source_rows"] == 3
    assert counts["source_rows_dated"] == 1
    assert counts["null_date_rows_dropped"] == 2
    assert counts["null_date_blank"] == 1
    assert counts["null_date_unparsable"] == 1
    # The only dated row is 2018-12-05, so the grid is one day of two comunas; the
    # two undated events contribute no row at all and no burned area.
    assert counts["comuna_days"] == 2
    assert counts["comuna_days_with_fire"] == 1
    assert counts["burned_ha_total"] == 0.0
    assert all(row.local_date == dt.date(2018, 12, 5) for row in panel.rows)


def test_zero_burned_area_is_a_measured_zero():
    panel, _manifest = build(SYNTHETIC_FIXTURE)
    fired = [row for row in panel.rows if row.fire_today]
    assert len(fired) == 1
    assert fired[0].burned_ha == 0.0
    assert fired[0].fire_count == 1
    # A feature this source cannot supply is empty even on a day that has a fire.
    assert fired[0].features["temp_max"] is None
    assert fired[0].features["humidity_rel"] == pytest.approx(33.0)


def test_local_day_is_santiago_and_not_utc(monkeypatch):
    from zoneinfo import ZoneInfo

    santiago = assemble.load_zone()
    assert assemble.TIMEZONE == "America/Santiago"
    assert santiago.key == "America/Santiago"
    # 02:30 UTC on 1 January is still 31 December in Santiago: the two zones disagree
    # about the day, so if this module ever cuts in UTC these two answers collapse and
    # the assertion below fails.
    moment = dt.datetime(2019, 1, 1, 2, 30, tzinfo=ZoneInfo("UTC"))
    assert assemble.civil_date(moment, santiago) == dt.date(2018, 12, 31)
    assert assemble.civil_date(moment, ZoneInfo("UTC")) == dt.date(2019, 1, 1)


def test_missing_zone_data_is_a_failure_not_a_utc_fallback(monkeypatch):
    from zoneinfo import ZoneInfoNotFoundError

    def unavailable(name):
        raise ZoneInfoNotFoundError(name)

    monkeypatch.setattr(assemble, "ZoneInfo", unavailable)
    with pytest.raises(assemble.AssembleError) as refusal:
        assemble.load_zone()
    assert "no UTC fallback" in str(refusal.value)


def test_unavailable_features_are_empty_cells_and_never_zero(sample):
    panel, manifest = sample
    statuses = {feature["name"]: feature["status"] for feature in manifest["features"]}
    assert statuses == {
        "temp_max": "unavailable",
        "temp_min": "unavailable",
        "humidity_rel": "source",
        "wind_speed": "unavailable",
        "wind_dir": "unavailable",
        "precip_sum": "unavailable",
        "radiation_sol": "unavailable",
    }
    for name, status in statuses.items():
        if status == "source":
            continue
        values = {row.features[name] for row in panel.rows}
        assert values == {None}, f"{name} must stay absent, not become zero"
        feature = next(item for item in manifest["features"] if item["name"] == name)
        assert feature["reason"]
        assert feature["filled_comuna_days"] == 0


def test_humidity_is_the_mean_of_the_comuna_days_readings(sample):
    panel, manifest = sample
    # Talca 2018-12-10 holds three events reading 29, 24.2 and 30.5.
    row = row_of(panel, "07101", "2018-12-10")
    assert row.features["humidity_rel"] == pytest.approx((29 + 24.2 + 30.5) / 3)
    assert row.meteorology_readings == 3
    humidity = next(item for item in manifest["features"] if item["name"] == "humidity_rel")
    assert humidity["aggregation"] == "mean over 1 civil day(s)"
    assert humidity["source_unit"] == "%"


def test_a_reading_that_is_absent_stays_absent_not_zero(sample):
    panel, _manifest = sample
    # Cauquenes 2018-12-12 has one event whose meteorology cells are all empty.
    row = row_of(panel, "07201", "2018-12-12")
    assert row.fire_today == 1
    assert row.meteorology_readings == 0
    assert row.features["humidity_rel"] is None


def test_quiet_comuna_days_have_no_features_at_all(sample):
    panel, _manifest = sample
    quiet = [row for row in panel.rows if not row.fire_today]
    assert all(row.features["humidity_rel"] is None for row in quiet)
    assert all(row.meteorology_readings == 0 for row in quiet)


def test_an_unreadable_burned_area_is_absent_not_zero():
    """Built from constructed events: the source has no unreadable area to point at."""
    inicio = assemble.Inicio(
        raw="05/12/2018 12:00", date=dt.date(2018, 12, 5), status=assemble.FORMAT_SLASH
    )
    events = [
        assemble.Event("Talca", "Talca", "Maule", inicio, "1.7", 0.5, {"Humedad": 30.0}),
        assemble.Event("Talca", "Talca", "Maule", inicio, "1.8", None, {"Humedad": 40.0}),
    ]
    panel = assemble.build_panel(
        events,
        mapping=assemble.load_mapping(assemble.DEFAULT_MAPPING),
        features=("humidity_rel",),
    )
    assert len(panel.rows) == 1
    row = panel.rows[0]
    assert row.fire_count == 2
    assert row.burned_ha is None  # one event's area is unknown, so the total is unknown
    assert row.features["humidity_rel"] == pytest.approx(35.0)


def test_both_source_date_shapes_are_counted_separately(sample):
    _panel, manifest = sample
    assert manifest["counts"]["date_formats"] == {
        assemble.FORMAT_SLASH: 7,
        assemble.FORMAT_TEXT: 37,
    }


def test_the_committed_fixtures_match_their_documented_checksums():
    """The provenance file records bytes; an edited fixture must fail here first."""
    assert assemble.sha256_of(SAMPLE_FIXTURE) == (
        "178d350022b06b73f72a58851741a124adde9b22595efbcc6e27ea89a7921d0b"
    )
    assert SAMPLE_FIXTURE.stat().st_size == 30264
    assert assemble.sha256_of(SYNTHETIC_FIXTURE) == (
        "edde38296d85425624580a08b08cab3caadef89607ed7b9223d66db0a854d68a"
    )


def test_the_unit_audit_is_measured_on_the_rows_that_were_read(sample):
    _panel, manifest = sample
    audit = {entry["column"]: entry for entry in manifest["source_units"]}
    # 44 rows, of which 2 have empty meteorology cells (Cauquenes 12-12 and 12-18).
    assert audit[assemble.COL_TEMPERATURA]["values"] == {"present": 42, "empty": 2}
    assert audit[assemble.COL_HUMEDAD]["values"] == {"present": 42, "empty": 2}
    assert audit[assemble.COL_HUMEDAD]["declared"] is None  # the source declares no unit
    assert audit[assemble.COL_HUMEDAD]["inferred"] == "%"
    assert audit[assemble.COL_VIENTO_VELOCIDAD]["ambiguous"] is True
    assert audit[assemble.COL_VIENTO_VELOCIDAD]["inferred"] is None
    assert audit[assemble.COL_PENDIENTE]["declared"] == "%"
    assert manifest["source_units_note"]


def test_manifest_counts_match_the_fixture(sample):
    _panel, manifest = sample
    with SAMPLE_FIXTURE.open(encoding="utf-8-sig", newline="") as handle:
        rows = list(csv.DictReader(handle))
    assert manifest["counts"]["source_rows"] == len(rows) == 44
    assert manifest["counts"]["source_rows_dated"] == len(rows)
    assert manifest["source"]["bytes"] == SAMPLE_FIXTURE.stat().st_size
    total = sum(float(row["Superficie total"]) for row in rows)
    assert manifest["counts"]["burned_ha_total"] == pytest.approx(round(total, 6))
    assert manifest["counts"]["burned_ha_unknown_comuna_days"] == 0
    assert manifest["counts"]["events_in_panel"] == len(rows)
    assert manifest["source"]["is_training_set"] is False
    assert manifest["source"]["license"] == "CC BY 4.0"


def test_manifest_is_json_with_the_feature_contract(sample, tmp_path):
    panel, manifest = sample
    destination = tmp_path / "out"
    assemble.write_panel(destination / "panel.csv", panel)
    assemble.write_manifest(destination / "manifest.json", manifest)
    document = json.loads((destination / "manifest.json").read_text(encoding="utf-8"))
    assert [feature["name"] for feature in document["features"]] == list(panel.features)
    with (destination / "panel.csv").open(encoding="utf-8", newline="") as handle:
        reader = csv.DictReader(handle)
        assert reader.fieldnames == [*assemble.PANEL_COLUMNS, *panel.features]
        written = list(reader)
    assert len(written) == len(panel.rows)


def test_unresolved_comuna_keeps_its_name_and_is_reported(tmp_path):
    mapping = json.loads(assemble.DEFAULT_MAPPING.read_text(encoding="utf-8"))
    mapping["entries"] = [entry for entry in mapping["entries"] if entry["name"] != "Cauquenes"]
    path = tmp_path / "partial-mapping.json"
    path.write_text(json.dumps(mapping, ensure_ascii=False), encoding="utf-8")
    _panel, manifest = build(mapping_path=path)
    assert manifest["comunas"]["unresolved_names"] == ["Cauquenes"]
    assert manifest["comunas"]["panel_comunas"] == 2
    assert manifest["comunas"]["mapping_entries"] == 29
    cauquenes = [row for row in _panel.rows if row.comuna == "Cauquenes"]
    assert cauquenes
    for row in cauquenes:
        assert row.cut == ""
        assert row.comuna_unresolved == "Cauquenes"


def test_panel_rows_are_ordered_by_key(sample):
    panel, _manifest = sample
    keys = [(row.cut, row.local_date) for row in panel.rows]
    assert keys == sorted(keys)


def test_summary_reports_the_null_dates_and_the_unavailable_features():
    panel, manifest = build(SYNTHETIC_FIXTURE)
    lines = assemble.summary(panel, manifest)
    joined = "\n".join(lines)
    assert "2 dropped for a null date" in joined
    assert "features unavailable:" in joined
