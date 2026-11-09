"""Tests for `analyze_delivered.py`: the claims the analysis report makes.

The report is generated from three files that CI does not have, so what is tested here
is the **machinery that produces the claims**, against trimmed fixtures of the same
three files (see `fixtures/delivered/PROVENANCE.md`):

* a duplicate count is computed from the rows, not typed;
* a column whose every cell is blank is found by reading the cells;
* a reading outside a stated physical bound is flagged, and the bound is the stated one
  rather than the observed maximum;
* the complete / partial / none split of the comunas is computed from the station and
  variable sets;
* the report is a pure function of the analysis: two runs over the same bytes produce
  the same bytes.

The last group is the guard against a hard-coded number. Each test there changes a
fixture, re-runs the analysis, and requires the report to carry the changed number -- so
a literal typed into the generator where a computed value belongs fails the test.
"""

from __future__ import annotations

import csv
import re
from dataclasses import dataclass
from pathlib import Path

import pytest
from conftest import (
    DASHBOARD_ANALYSIS_FIXTURE,
    EMAS_FIXTURE,
    RTDATA_FIXTURE,
)

import analyze_delivered as analysis

ARTIFACT = analysis.DEFAULT_ARTIFACT

#: Scratch space for the mutated copies. Inside the package and git-ignored (`out/`),
#: so a test never writes outside the directory it is allowed to touch.
SCRATCH = Path(__file__).resolve().parents[1] / "out" / "analysis" / "tests"


# ---------------------------------------------------------------------------
# Reading, writing and mutating the fixtures
# ---------------------------------------------------------------------------


def read_csv(path: Path) -> tuple[list[str], list[dict[str, str]]]:
    with path.open("r", encoding="utf-8", newline="") as handle:
        reader = csv.DictReader(handle)
        columns = list(reader.fieldnames or [])
        return columns, [dict(row) for row in reader]


def write_csv(path: Path, columns: list[str], rows: list[dict[str, str]]) -> Path:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8", newline="") as handle:
        writer = csv.DictWriter(handle, fieldnames=columns, lineterminator="\n")
        writer.writeheader()
        for row in rows:
            writer.writerow(row)
    return path


@dataclass(frozen=True)
class Bundle:
    dashboard: Path
    emas: Path
    rtdata: Path

    def analyse(self) -> analysis.Analysis:
        return analysis.analyze(self.dashboard, self.emas, self.rtdata)


def copy_fixtures(name: str) -> Bundle:
    """A writable copy of the three analysis fixtures under `out/analysis/tests/`."""
    directory = SCRATCH / name
    directory.mkdir(parents=True, exist_ok=True)
    bundle = Bundle(
        dashboard=directory / "dashboard.csv",
        emas=directory / "emas.csv",
        rtdata=directory / "rtdata.csv",
    )
    for source, target in (
        (DASHBOARD_ANALYSIS_FIXTURE, bundle.dashboard),
        (EMAS_FIXTURE, bundle.emas),
        (RTDATA_FIXTURE, bundle.rtdata),
    ):
        target.write_bytes(source.read_bytes())
    return bundle


@pytest.fixture(scope="module")
def result() -> analysis.Analysis:
    """The analysis of the committed fixtures, computed once for the read-only tests."""
    return analysis.analyze(DASHBOARD_ANALYSIS_FIXTURE, EMAS_FIXTURE, RTDATA_FIXTURE)


# ---------------------------------------------------------------------------
# The fixture is what its provenance says it is
# ---------------------------------------------------------------------------


def test_the_fixture_dashboard_is_the_trim_the_provenance_describes(result):
    dashboard = result.dashboard
    assert dashboard.facts.rows == 270
    assert dashboard.regions == ("Biobío", "Tarapacá", "Valparaíso")
    assert len(dashboard.dates) == 90
    assert dashboard.dates[0] == "2026-07-01"
    assert dashboard.dates[-1] == "2026-09-28"
    assert dashboard.expected_cells == dashboard.present_cells == 270
    assert dashboard.missing_cells == 0
    assert dashboard.duplicate_cells == 0


def test_the_fixture_catalogue_is_the_trim_the_provenance_describes(result):
    emas = result.emas
    assert emas.facts.rows == 61
    assert emas.stations == 61
    assert emas.active == 42
    assert emas.inactive == 19
    assert emas.comunas == 32
    assert emas.blank_region_rows == 1
    assert emas.earliest_date == "2010-01-01"


def test_the_fixture_snapshot_is_the_trim_the_provenance_describes(result):
    rtdata = result.rtdata
    assert rtdata.facts.rows == 369
    assert rtdata.stations == 42
    assert rtdata.instant_count == 1
    assert rtdata.duplicates.unique_keys == 276
    assert rtdata.duplicates.extra_rows == 93


# ---------------------------------------------------------------------------
# The label is derived from the counter, never assumed
# ---------------------------------------------------------------------------


def test_the_label_is_the_counters_advance(result):
    dashboard = result.dashboard
    assert dashboard.positives == 21
    assert dashboard.incidents == 24
    assert dashboard.act_decrements == 0
    assert dashboard.first_day_rows == len(dashboard.regions)
    assert dashboard.first_day_nonzero == 0
    assert sum(count for _, count in dashboard.positives_by_region) == dashboard.positives
    assert sum(positives for _, _, positives in dashboard.month_rows) == dashboard.positives


def test_the_downward_revisions_are_listed_not_clamped(result):
    dashboard = result.dashboard
    assert dashboard.sup_decrements == 2
    assert len(dashboard.revisions) == 2
    for revision in dashboard.revisions:
        assert revision.current < revision.previous


def test_a_counter_that_moves_backwards_is_counted_as_a_decrement():
    """A counter that shrinks is counted, and the shrink never counts as a positive."""
    baseline = analysis.analyze(DASHBOARD_ANALYSIS_FIXTURE, EMAS_FIXTURE, RTDATA_FIXTURE)
    bundle = copy_fixtures("dashboard-decrement")
    columns, rows = read_csv(bundle.dashboard)
    ordered = sorted(rows, key=lambda row: (row["region"], row["fecha"]))
    last = [row for row in ordered if row["region"] == "Biobío"][-1]
    last["nro_inc_act"] = str(int(last["nro_inc_act"]) - 1)
    write_csv(bundle.dashboard, columns, rows)
    mutated = bundle.analyse().dashboard
    assert mutated.act_decrements == baseline.dashboard.act_decrements + 1
    # The fixture's last Biobío day is not a positive advance, so shrinking it changes
    # neither the positive count nor the number of incidents behind it.
    assert mutated.positives == baseline.dashboard.positives
    assert mutated.incidents == baseline.dashboard.incidents


# ---------------------------------------------------------------------------
# Duplicates are computed
# ---------------------------------------------------------------------------


def test_the_duplicate_count_is_rows_minus_keys(result):
    duplicates = result.rtdata.duplicates
    assert duplicates.rows == 369
    assert duplicates.extra_rows == duplicates.rows - duplicates.unique_keys
    assert duplicates.groups_by_size == ((1, 233), (2, 18), (4, 25))
    assert duplicates.rows_in_groups == 18 * 2 + 25 * 4
    assert duplicates.conflicting_groups == 7
    assert duplicates.stations_affected == 5


def test_appending_one_copy_raises_the_duplicate_count_by_one():
    bundle = copy_fixtures("rtdata-extra-copy")
    columns, rows = read_csv(bundle.rtdata)
    original = rows[0]
    rows.append(
        {
            **original,
            "rtd_ia_id": "99999999",
            "rtd_dt_versiondt": "2026-09-28 15:30:12.000 -0300",
        }
    )
    write_csv(bundle.rtdata, columns, rows)
    mutated = bundle.analyse().rtdata
    assert mutated.duplicates.rows == 370
    assert mutated.duplicates.unique_keys == 276
    assert mutated.duplicates.extra_rows == 94


# ---------------------------------------------------------------------------
# Emptiness is measured per column
# ---------------------------------------------------------------------------


def test_the_empty_columns_are_the_ones_that_are_blank_on_every_row(result):
    assert set(result.rtdata.empty_columns) == {
        "ins_ia_id",
        "rtd_c_emaid",
        "rtd_i_resultadochecks",
        "rtd_i_calidad",
        "rtd_c_ultimocomentario",
    }
    assert "rtd_i_calidad" in result.rtdata.empty_columns
    assert "rtd_i_resultadochecks" in result.rtdata.empty_columns


def test_a_column_added_empty_is_detected_as_empty(result):
    bundle = copy_fixtures("rtdata-empty-column")
    columns, rows = read_csv(bundle.rtdata)
    for row in rows:
        row["rtd_c_nueva"] = ""
    write_csv(bundle.rtdata, columns + ["rtd_c_nueva"], rows)
    mutated = bundle.analyse().rtdata
    assert "rtd_c_nueva" in mutated.empty_columns
    assert len(mutated.empty_columns) == len(result.rtdata.empty_columns) + 1


# ---------------------------------------------------------------------------
# Plausibility is checked against a stated bound, not against the data
# ---------------------------------------------------------------------------


def test_radiation_above_the_solar_constant_is_flagged(result):
    radiation = [entry for entry in result.rtdata.violations if entry.variable == "RADIACION_SOL"]
    assert len(radiation) == 4
    assert all(entry.value > analysis.SOLAR_CONSTANT for entry in radiation)
    profile = next(entry for entry in result.rtdata.profiles if entry.variable == "RADIACION_SOL")
    assert profile.outside == 4
    assert profile.limit_high == analysis.SOLAR_CONSTANT
    # The bound is the physical ceiling, not the observed maximum: the maximum is above it.
    assert profile.high > profile.limit_high


def test_a_value_lifted_above_the_bound_is_flagged():
    bundle = copy_fixtures("rtdata-implausible")
    columns, rows = read_csv(bundle.rtdata)
    changed = 0
    for row in rows:
        if row["rtd_c_variableid"] == "RADIACION_SOL":
            row["rtd_f_valor"] = "2000.000"
            changed += 1
    assert changed > 0
    write_csv(bundle.rtdata, columns, rows)
    mutated = bundle.analyse().rtdata
    profile = next(entry for entry in mutated.profiles if entry.variable == "RADIACION_SOL")
    assert profile.outside == changed
    assert profile.high == 2000.0


def test_the_zero_humidity_reading_is_called_out(result):
    assert len(result.rtdata.humidity_boundary) == 1
    assert result.rtdata.humidity_boundary[0].variable == "HUMEDAD_REL"
    assert result.rtdata.humidity_boundary[0].value == 0.0


def test_pressure_inside_the_bound_still_needs_an_elevation(result):
    pressure = [entry for entry in result.rtdata.violations if entry.variable == "PRESION_ATM"]
    assert pressure == []


def test_the_standard_atmosphere_gives_the_altitude_a_pressure_implies():
    """The report's "plausible at altitude" reading is computed, not asserted."""
    assert analysis.standard_atmosphere_height(1013.25) == pytest.approx(0.0, abs=1.0)
    assert analysis.standard_atmosphere_height(593.03) == pytest.approx(4295, abs=60)
    # A lower pressure can only mean a higher station.
    assert analysis.standard_atmosphere_height(593.03) > analysis.standard_atmosphere_height(700.0)


def test_a_variable_without_a_stated_bound_is_a_failure():
    bundle = copy_fixtures("rtdata-unknown-variable")
    columns, rows = read_csv(bundle.rtdata)
    for row in rows:
        if row["rtd_c_variableid"] == "PRESION_ATM":
            row["rtd_c_variableid"] = "PRESION_SIN_DECLARAR"
    write_csv(bundle.rtdata, columns, rows)
    with pytest.raises(analysis.AnalysisError) as raised:
        bundle.analyse()
    assert "PRESION_SIN_DECLARAR" in str(raised.value)


def test_every_stated_bound_has_a_reason(result):
    for profile in result.rtdata.profiles:
        assert profile.limit_low < profile.limit_high
        assert profile.basis.strip()


# ---------------------------------------------------------------------------
# The coverage split is computed from the data
# ---------------------------------------------------------------------------


def test_the_fixtures_valparaiso_split_is_the_one_the_report_asserts(result):
    emas = result.emas
    assert emas.valparaiso.total == emas.valparaiso_comuna_count == 38
    assert len(emas.valparaiso.complete) == 11
    assert len(emas.valparaiso.partial) == 15
    assert len(emas.valparaiso.none) == 12
    assert emas.valparaiso_stations == 56
    assert emas.valparaiso_active == 38


def test_the_classification_follows_the_variables_reported():
    comunas = {"05101": "Uno", "05102": "Dos", "05103": "Tres"}
    stations = [("1", "05101", True), ("2", "05102", True), ("3", "05103", False)]
    required = frozenset({"A", "B"})
    variables = {"1": {"A", "B"}, "2": {"A"}, "3": {"A", "B"}}

    split, per_comuna = analysis.coverage_split(
        comunas, stations, variables, required, active_only=True
    )
    assert split.complete == ("05101",)
    assert split.partial == ("05102",)
    assert split.none == ("05103",)
    assert [entry.classification for entry in per_comuna] == [
        "complete",
        "partial",
        "none",
    ]

    # The same station, one variable short, moves its comuna out of complete.
    variables["1"] = {"A"}
    moved, _ = analysis.coverage_split(comunas, stations, variables, required, active_only=True)
    assert moved.complete == ()
    assert "05101" in moved.partial


def test_an_inactive_station_does_not_complete_a_comuna():
    comunas = {"05101": "Uno"}
    stations = [("1", "05101", False)]
    required = frozenset({"A"})
    variables = {"1": {"A"}}
    strict, _ = analysis.coverage_split(comunas, stations, variables, required, active_only=True)
    loose, _ = analysis.coverage_split(comunas, stations, variables, required, active_only=False)
    assert strict.none == ("05101",)
    assert loose.complete == ("05101",)


def test_dropping_a_stations_readings_moves_its_comuna():
    bundle = copy_fixtures("rtdata-drop-station")
    _, emas_rows = read_csv(bundle.emas)
    target = next(row for row in emas_rows if row["com_ia_id"].zfill(5) == "05302")
    station_id = target["ema_ia_id"]
    columns, rows = read_csv(bundle.rtdata)
    kept = [row for row in rows if row["ema_ia_id"] != station_id]
    assert len(kept) < len(rows)
    write_csv(bundle.rtdata, columns, kept)
    mutated = bundle.analyse().emas
    assert "05302" not in mutated.valparaiso.complete
    assert "05302" in mutated.valparaiso.partial
    assert len(mutated.valparaiso.complete) == 10
    assert len(mutated.valparaiso.partial) == 16


# ---------------------------------------------------------------------------
# The identifier chain
# ---------------------------------------------------------------------------


def test_a_four_digit_comuna_is_padded_to_a_five_digit_cut():
    assert analysis.cut_from("5102") == "05102"
    assert analysis.cut_from("13303") == "13303"
    assert analysis.cut_from("14101") == "14101"


def test_a_value_that_cannot_be_a_cut_is_refused():
    assert analysis.cut_from("") is None
    assert analysis.cut_from("abc") is None
    assert analysis.cut_from("0") is None
    assert analysis.cut_from("17000") is None
    assert analysis.cut_from("99001") is None


def test_the_region_label_is_checked_against_the_cut_prefix(result):
    assert result.emas.prefix_mismatch_rows == 1
    mismatch = result.emas.prefix_mismatches[0]
    assert mismatch.region == "Ñuble"
    assert mismatch.prefix == "08"


def test_an_unknown_region_label_is_a_failure():
    bundle = copy_fixtures("emas-unknown-region")
    columns, rows = read_csv(bundle.emas)
    rows[0]["region_nombre"] = "Region Inventada"
    write_csv(bundle.emas, columns, rows)
    with pytest.raises(analysis.AnalysisError) as raised:
        bundle.analyse()
    assert "Region Inventada" in str(raised.value)


def test_the_dpa_membership_check_runs_for_the_two_tracked_regions(result):
    labels = [entry[0] for entry in result.emas.dpa_checked]
    assert labels == ["Valparaíso"]
    _, known, present, unknown = result.emas.dpa_checked[0]
    assert present == 27
    assert known == 27
    assert unknown == ()


# ---------------------------------------------------------------------------
# The report is generated, and its numbers are computed
# ---------------------------------------------------------------------------


def test_two_runs_produce_byte_identical_reports():
    first, first_path = analysis.run(
        DASHBOARD_ANALYSIS_FIXTURE,
        EMAS_FIXTURE,
        RTDATA_FIXTURE,
        out=SCRATCH / "identical-a" / "ANALYSIS.md",
    )
    second, second_path = analysis.run(
        DASHBOARD_ANALYSIS_FIXTURE,
        EMAS_FIXTURE,
        RTDATA_FIXTURE,
        out=SCRATCH / "identical-b" / "ANALYSIS.md",
    )
    assert first_path.read_bytes() == second_path.read_bytes()
    assert analysis.render_report(first) == analysis.render_report(second)


def test_the_rendered_report_is_the_string_the_analysis_produces(result):
    written = (SCRATCH / "identical-a" / "ANALYSIS.md").read_text(encoding="utf-8")
    assert analysis.render_report(result) == written


def test_the_report_carries_every_required_section(result):
    report = analysis.render_report(result)
    for required in (
        "# Analysis of the delivered data",
        "## The three files",
        "## 1. `dashboard-conaf-analysis-sample.csv`",
        "### The label",
        "### Positives by region",
        "### Positives by month",
        "### Coverage",
        "### The cumulative counters",
        "### Correlation",
        "## 2. `emas-sample.csv`",
        "### The Valparaíso coverage split",
        "### The earliest reading",
        "### The identifier chain",
        "## 3. `rtdata-sample.csv`",
        "### Duplicates",
        "### Empty and constant columns",
        "### Value ranges against physical limits",
        "### The single timestamp",
        "## 4. The cascade",
        "### What to ask for, in order",
        "## What this analysis is not",
    ):
        assert required in report, f"the report no longer contains {required!r}"
    assert report.count("### Verdict") == 3
    assert report.count("**Verdict:") == 3
    assert "REPORT-delivered-data.md" in report


def test_the_report_carries_the_computed_numbers(result):
    report = analysis.render_report(result)
    assert f"| Positive region-days | {result.dashboard.positives} |" in report
    assert f"| Incidents behind those positives | {result.dashboard.incidents} |" in report
    assert f"| Extra rows | {result.rtdata.duplicates.extra_rows} |" in report
    assert f"| Distinct `rtd_dt_marcatiempo` values | {result.rtdata.instant_count} |" in report
    assert f"| Earliest `primera_lectura` | {result.emas.earliest_date} |" in report
    assert f"| Complete | {len(result.emas.valparaiso.complete)} | 38 |" in report
    assert f"| Partial | {len(result.emas.valparaiso.partial)} | 38 |" in report
    assert f"| None | {len(result.emas.valparaiso.none)} | 38 |" in report


def test_a_change_in_the_data_changes_the_report():
    """The anti-hard-coding guard: the report has to follow the fixture."""
    plain = copy_fixtures("report-plain")
    baseline = analysis.render_report(plain.analyse())
    expected = plain.analyse().dashboard.positives
    assert f"| Positive region-days | {expected} |" in baseline

    mutated = copy_fixtures("report-mutated")
    columns, rows = read_csv(mutated.dashboard)
    for row in rows:
        if row["region"] == "Biobío" and row["fecha"] == "2026-09-28":
            row["nro_inc_act"] = str(int(row["nro_inc_act"]) + 3)
    write_csv(mutated.dashboard, columns, rows)
    changed = analysis.render_report(mutated.analyse())
    assert f"| Positive region-days | {expected + 1} |" in changed
    assert f"| Incidents behind those positives | {plain.analyse().dashboard.incidents + 3} |" in (
        changed
    )
    assert f"| Positive region-days | {expected} |" not in changed


def test_a_report_number_that_is_only_in_the_module_fails_this_scan():
    """Every headline total has to come from the bytes, not from a literal.

    The delivered-file totals are the ones a hurried edit would paste in. The module
    computes them; if one appears as a standalone literal in the source, the check below
    fires and names it.
    """
    source = Path(analysis.__file__).read_text(encoding="utf-8")
    headlines = ("1440", "1437", "5122", "535", "437", "3685", "235")
    pattern = re.compile(r"(?<![\d.])(" + "|".join(headlines) + r")(?![\d.])")
    found = sorted(set(pattern.findall(source)))
    assert found == [], f"headline total(s) written as literals in the module: {found}"


def test_the_cli_writes_the_report_and_returns_zero():
    destination = SCRATCH / "cli" / "ANALYSIS.md"
    exit_code = analysis.main(
        [
            "--dashboard",
            str(DASHBOARD_ANALYSIS_FIXTURE),
            "--emas",
            str(EMAS_FIXTURE),
            "--rtdata",
            str(RTDATA_FIXTURE),
            "--out",
            str(destination),
        ]
    )
    assert exit_code == 0
    report = destination.read_text(encoding="utf-8")
    assert report.startswith("# Analysis of the delivered data\n")
    assert report.endswith("\n")
    assert "\r\n" not in report


def test_the_cli_refuses_a_missing_input():
    destination = SCRATCH / "cli-missing" / "ANALYSIS.md"
    exit_code = analysis.main(
        [
            "--dashboard",
            str(DASHBOARD_ANALYSIS_FIXTURE),
            "--emas",
            str(EMAS_FIXTURE),
            "--rtdata",
            str(SCRATCH / "does-not-exist.csv"),
            "--out",
            str(destination),
        ]
    )
    assert exit_code == 2
    assert not destination.exists()


# ---------------------------------------------------------------------------
# The artifact is the vocabulary, checked in both directions
# ---------------------------------------------------------------------------


def test_the_variable_mapping_matches_the_artifact():
    declared = analysis.load_artifact(ARTIFACT)
    analysis.check_variable_mapping(declared)
    assert set(analysis.VARIABLE_FEATURES.values()) == set(declared)


def test_a_feature_the_analysis_does_not_map_is_a_failure():
    declared = analysis.load_artifact(ARTIFACT)
    del declared["temp_max"]
    with pytest.raises(analysis.AnalysisError) as raised:
        analysis.check_variable_mapping(declared)
    assert "temp_max" in str(raised.value)


def test_a_mapped_feature_the_artifact_lacks_is_a_failure():
    declared = analysis.load_artifact(ARTIFACT)
    declared["invented_feature"] = ("degC", 0.0, 1.0)
    with pytest.raises(analysis.AnalysisError) as raised:
        analysis.check_variable_mapping(declared)
    assert "invented_feature" in str(raised.value)


def test_the_artifact_range_is_reported_next_to_the_readings(result):
    radiation = next(entry for entry in result.rtdata.profiles if entry.variable == "RADIACION_SOL")
    assert radiation.artifact_high == 1758.0
    assert radiation.artifact_outside == 0
    assert radiation.outside == 4
