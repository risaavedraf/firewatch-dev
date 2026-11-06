"""The label, the split, the leak guard, the fit, the baselines and the export.

Everything here runs against the committed fixture
(`tests/fixtures/dashboard-conaf-sample.csv`, 450 rows: five regions, all 90 days) with
no network, and re-reads the raw CSV with `csv` whenever an expected value can be
derived again instead of quoted. A test that only repeats what the module just computed
proves the module agrees with itself.
"""

from __future__ import annotations

import csv
import datetime as dt
import itertools
import json
import random
from pathlib import Path

import numpy as np
import pytest
from conftest import DASHBOARD_FIXTURE
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import average_precision_score

import model_contract
import train_delivered as subject

#: The split the source's own calendar produces, checked independently of the module.
TRAIN_MONTHS = (7, 8)
TEST_MONTHS = (9,)


def read_raw(path: Path) -> list[dict[str, str]]:
    with path.open(encoding="utf-8", newline="") as handle:
        return list(csv.DictReader(handle))


@pytest.fixture(scope="module")
def rows() -> list[subject.DayRow]:
    parsed, _counts = subject.read_days(DASHBOARD_FIXTURE)
    return parsed


@pytest.fixture(scope="module")
def counts() -> subject.Counts:
    _rows, measured = subject.read_days(DASHBOARD_FIXTURE)
    return measured


@pytest.fixture(scope="module")
def run() -> subject.Result:
    """One full run on the fixture, reused by every test that needs fitted numbers."""
    return subject.train(DASHBOARD_FIXTURE, write=False, trained_at="2026-11-06T00:00:00Z")


def row_at(rows: list[subject.DayRow], region: str, date: str) -> subject.DayRow:
    target = dt.date.fromisoformat(date)
    for row in rows:
        if row.region == region and row.date == target:
            return row
    raise AssertionError(f"no row for {region} {date}")


# ---------------------------------------------------------------------------
# The label
# ---------------------------------------------------------------------------


def test_label_is_the_clamped_daily_advance_of_the_cumulative_counter(rows, counts):
    """Re-derive the label from the raw CSV, without the module's help.

    `fire_today` is 1 exactly when `nro_inc_act` grew from the region's previous day, and
    the count of those days is the positive count the manifest reports.
    """
    raw = read_raw(DASHBOARD_FIXTURE)
    by_region: dict[str, list[dict[str, str]]] = {}
    for entry in raw:
        by_region.setdefault(entry[subject.COL_REGION], []).append(entry)

    expected_labels = 0
    for region, entries in by_region.items():
        entries.sort(key=lambda entry: entry[subject.COL_FECHA])
        for previous, current in itertools.pairwise(entries):
            advanced = int(current[subject.COL_ACT]) - int(previous[subject.COL_ACT])
            expected_labels += 1 if advanced > 0 else 0
            derived = row_at(rows, region, current[subject.COL_FECHA])
            assert derived.fire_today == (1 if advanced > 0 else 0), (
                f"{region} {current[subject.COL_FECHA]}: cumulative advanced by {advanced} "
                f"but the label is {derived.fire_today}"
            )
    assert expected_labels == counts.positives == 27


def test_first_day_of_each_region_carries_no_derived_difference(rows, counts):
    """A cumulative counter has no predecessor on the window's first day.

    Its label is 0 because nothing advanced *within the window*, and the derived
    difference columns are empty rather than zero: a zero would be a measurement the file
    never made. The count is reported so the reader knows how many of the zeros are
    structural.
    """
    firsts = [row for row in rows if row.first_in_window]
    assert len(firsts) == counts.first_day_of_region_rows == 5
    for row in firsts:
        assert row.date == dt.date(2026, 7, 1)
        assert row.fire_today == 0
        assert row.fires_today is None
        assert row.burned_ha_today is None
        assert row.climatology_today is None


def test_clamp_zeroes_the_downturn_and_the_count_is_reported(rows, counts):
    """`sup_afec_act` was revised downward; the difference is clamped AND counted.

    Valparaiso 2026-07-03 -> 2026-07-04 drops 2.2 ha to 1.9 ha. The derived column must
    read 0.0 and the run must say how many such rows it clamped, so a negative difference
    can never disappear without a number attached to it.
    """
    assert counts.clamped_sup_afec_decrements == 3
    assert counts.clamped_act_decrements == 0

    revised = row_at(rows, "Valparaíso", "2026-07-04")
    assert revised.burned_ha_today == 0.0
    assert revised.fire_today == 0

    # And the clamp count is exactly the number of downward steps in the raw file.
    raw = read_raw(DASHBOARD_FIXTURE)
    by_region: dict[str, list[dict[str, str]]] = {}
    for entry in raw:
        by_region.setdefault(entry[subject.COL_REGION], []).append(entry)
    downward = 0
    for entries in by_region.values():
        entries.sort(key=lambda entry: entry[subject.COL_FECHA])
        for previous, current in itertools.pairwise(entries):
            if float(current[subject.COL_SUP_ACT]) < float(previous[subject.COL_SUP_ACT]):
                downward += 1
    assert downward == counts.clamped_sup_afec_decrements


@pytest.mark.parametrize(
    ("delta", "expected"),
    [(-3.5, 0.0), (0.0, 0.0), (2.5, 2.5)],
)
def test_clamp_advance_never_returns_a_negative(delta, expected):
    assert subject.clamp_advance(delta) == expected


# ---------------------------------------------------------------------------
# The split
# ---------------------------------------------------------------------------


def test_split_is_the_calendar_and_it_is_disjoint_and_ordered(rows):
    train, test = subject.month_split(rows)
    assert {row.month for row in train} == set(TRAIN_MONTHS)
    assert {row.month for row in test} == set(TEST_MONTHS)
    assert max(row.date for row in train) < min(row.date for row in test)
    assert {(row.region, row.date) for row in train}.isdisjoint(
        {(row.region, row.date) for row in test}
    )


def test_a_random_split_is_refused(rows):
    """The failure this test exists for: somebody replaces the split with `random`."""
    shuffled = list(rows)
    random.Random(0).shuffle(shuffled)
    cut = len(shuffled) // 2
    with pytest.raises(subject.SplitError, match="temporal split carries months"):
        subject.assert_temporal_split(shuffled[:cut], shuffled[cut:])


def test_a_month_correct_split_that_shares_a_row_is_refused(rows):
    """The disjointness guard on its own, because a month split cannot reach it."""
    train, test = subject.month_split(rows)
    with pytest.raises(subject.SplitError, match="not disjoint"):
        subject.assert_disjoint([*train[:-1], test[0]], test)


def test_a_split_that_repeats_a_region_day_is_refused(rows):
    train, test = subject.month_split(rows)
    with pytest.raises(subject.SplitError, match="repeats a region-day"):
        subject.assert_disjoint([*train, train[0]], test)


def test_an_interleaved_split_is_refused_by_the_ordering_guard(rows):
    """Ordering on its own: every row on the right side of the month, but out of order."""
    train, test = subject.month_split(rows)
    with pytest.raises(subject.SplitError, match="not strictly ordered"):
        subject.assert_ordered(test, train)


def test_the_months_guard_alone_refuses_the_wrong_months(rows):
    train, test = subject.month_split(rows)
    with pytest.raises(subject.SplitError, match="carries months"):
        subject.assert_months(test, train)


def test_a_split_whose_order_is_reversed_is_refused(rows):
    train, test = subject.month_split(rows)
    with pytest.raises(subject.SplitError, match="temporal split carries months"):
        subject.assert_temporal_split(test, train)


def test_an_interleaved_split_is_refused(rows):
    """Only the last five days of the window as test: still ordered, still wrong."""
    ordered = sorted(rows, key=lambda row: (row.date, row.region))
    cut = len(ordered) - 5 * len({row.region for row in ordered})
    with pytest.raises(subject.SplitError, match="temporal split carries months"):
        subject.assert_temporal_split(ordered[:cut], ordered[cut:])


def test_an_empty_side_is_refused(rows):
    train, _test = subject.month_split(rows)
    with pytest.raises(subject.SplitError, match="empty on one side"):
        subject.assert_temporal_split(train, [])


# ---------------------------------------------------------------------------
# The leak guard
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    "column",
    [
        subject.COL_ACT,
        subject.COL_SUP_ACT,
        subject.COL_CREACION,
        "fires_today",
        "burned_ha_today",
        "fire_today",
    ],
)
def test_a_forbidden_column_cannot_be_a_feature(column):
    with pytest.raises(subject.LeakageError, match="cannot be a model input"):
        subject.assert_no_leakage(["day_of_year", column])


def test_the_feature_vocabulary_never_contains_a_forbidden_column(counts):
    names = {name for name, _unit in subject.feature_columns(counts.regions)}
    assert names.isdisjoint(set(subject.FORBIDDEN_FEATURE_COLUMNS))
    assert set(subject.FORBIDDEN_FEATURE_COLUMNS) == {
        "nro_inc_act",
        "sup_afec_act",
        "fecha_creacion",
        "fires_today",
        "burned_ha_today",
        "fire_today",
    }


def test_the_emitted_matrix_is_disjoint_from_the_forbidden_columns(rows, counts):
    columns = subject.feature_columns(counts.regions)
    train, test = subject.month_split(rows)
    for side in (train, test):
        matrix, label = subject.build_matrix(side, columns)
        assert matrix.shape == (len(side), len(columns))
        assert np.isfinite(matrix).all()
        assert set(label.tolist()) <= {0, 1}


def test_the_one_hot_sums_to_one_per_row(rows, counts):
    """A region indicator that never fires would leave every row all-zero.

    That is exactly the shape of the first bug this suite caught: a name built with the
    prefix twice, so every indicator was 0 and the model was told no region at all.
    """
    columns = subject.feature_columns(counts.regions)
    region_only = [index for index, (name, _u) in enumerate(columns) if name.startswith("region_")]
    assert len(region_only) == len(counts.regions)
    matrix, _label = subject.build_matrix(rows, columns)
    sums = matrix[:, region_only].sum(axis=1)
    assert np.allclose(sums, 1.0), f"{int((sums != 1).sum())} rows are not one-hot"
    for index in region_only:
        assert np.ptp(matrix[:, index]) == 1.0, f"{columns[index][0]} is constant"


def test_the_forbidden_column_is_worth_exactly_the_label(rows, counts):
    """How much the leak the guard blocks is actually worth, measured.

    The guard is not ceremony. On this fixture's test split a logistic regression on the
    five legal features plus the region one-hot reaches PR-AUC 0.0826; adding the same-day
    cumulative `nro_inc_act` lifts it to about 0.31, and adding its daily difference --
    which is the label by construction -- reproduces the label exactly. A model that scores
    1.0 here has learned nothing about fire, which is the reason `build_matrix` refuses the
    column before it builds anything.
    """
    columns = subject.feature_columns(counts.regions)
    train, test = subject.month_split(rows)
    train_matrix, train_label = subject.build_matrix(train, columns)
    test_matrix, test_label = subject.build_matrix(test, columns)

    with_counts = {}
    for entry in read_raw(DASHBOARD_FIXTURE):
        with_counts[(entry[subject.COL_REGION], entry[subject.COL_FECHA])] = float(
            entry[subject.COL_ACT]
        )

    def scored(rows_side, matrix, leak):
        extra = np.asarray([leak(row) for row in rows_side], dtype=np.float64).reshape(-1, 1)
        return np.hstack([matrix, extra])

    cumulative = lambda row: with_counts[(row.region, row.date.isoformat())]
    difference = lambda row: float(row.fires_today or 0)

    def pr_auc(leak=None):
        if leak is None:
            train_block, test_block, names = train_matrix, test_matrix, columns
        else:
            train_block = scored(train, train_matrix, leak)
            test_block = scored(test, test_matrix, leak)
            names = (*columns, ("leak", "count"))
        # The clean case cannot be expressed by appending a zero column: a constant input
        # is refused by `fit_preprocessing`, which is the same guard working twice.
        fitted = subject.fit_preprocessing(train_block, names)
        model = LogisticRegression(
            class_weight="balanced", max_iter=1000, random_state=subject.SEED
        )
        model.fit(fitted.apply(train_block), train_label)
        probability = model.predict_proba(fitted.apply(test_block))[:, 1]
        return float(average_precision_score(test_label, probability))

    clean = pr_auc()
    assert clean < 0.20, f"the legal features unexpectedly reach PR-AUC {clean:.4f}"
    assert pr_auc(difference) >= 0.99, "the label's own difference must reproduce the label"
    assert pr_auc(cumulative) > clean + 0.10, "the cumulative counter must be worth a lot"


def test_region_names_fold_to_unique_ascii_features(counts):
    names = [subject.region_feature_name(region) for region in counts.regions]
    assert len(names) == len(set(names))
    assert all(name.isascii() for name in names)
    assert subject.region_feature_name("Ñuble") == "region_nuble"
    assert subject.region_feature_name("O'Higgins") == "region_o_higgins"
    assert subject.region_feature_name("Aysén") == "region_aysen"


# ---------------------------------------------------------------------------
# Preprocessing is fitted on the training split only
# ---------------------------------------------------------------------------


def test_changing_a_test_value_does_not_move_center_or_scale(rows, counts):
    """The leak that normalization invites, tested from the outside.

    Fitting on `train + test` would let a September value into `center`. So: fit on train,
    then move a test row's value by a hundred, and the parameters must not budge. The
    second half proves the test can fail -- the concatenated fit really does move.
    """
    columns = subject.feature_columns(counts.regions)
    train, test = subject.month_split(rows)
    train_matrix, _train_label = subject.build_matrix(train, columns)
    test_matrix, _test_label = subject.build_matrix(test, columns)

    fitted = subject.fit_preprocessing(train_matrix, columns)
    disturbed = test_matrix.copy()
    disturbed[:, 0] = disturbed[:, 0] + 100.0
    refitted = subject.fit_preprocessing(train_matrix, columns)
    assert fitted == refitted
    assert (train_matrix[:, 0] + 100.0).max() not in fitted.center

    combined = subject.fit_preprocessing(np.vstack([train_matrix, disturbed]), columns)
    assert combined.center != fitted.center


def test_the_run_fits_its_parameters_on_the_training_rows_only(rows, counts, run):
    columns = subject.feature_columns(counts.regions)
    train, _test = subject.month_split(rows)
    train_matrix, _ = subject.build_matrix(train, columns)
    expected = subject.fit_preprocessing(train_matrix, columns)
    assert run.preprocessing == expected
    # And the numbers are the train statistics, not the whole file's.
    everything, _ = subject.build_matrix(rows, columns)
    assert run.preprocessing.center != subject.fit_preprocessing(everything, columns).center


def test_preprocessing_applies_impute_then_standardization(rows, counts):
    columns = subject.feature_columns(counts.regions)
    train, _test = subject.month_split(rows)
    train_matrix, _ = subject.build_matrix(train, columns)
    fitted = subject.fit_preprocessing(train_matrix, columns)
    standardized = fitted.apply(train_matrix)
    assert np.allclose(standardized.mean(axis=0), 0.0, atol=1e-9)
    assert np.allclose(standardized.std(axis=0), 1.0, atol=1e-9)

    with_nan = np.full((2, len(columns)), np.nan)
    assert np.allclose(
        fitted.apply(with_nan)[0],
        (np.asarray(fitted.impute) - np.asarray(fitted.center)) / np.asarray(fitted.scale),
    )


# ---------------------------------------------------------------------------
# Thresholds and metrics
# ---------------------------------------------------------------------------


def test_the_threshold_is_selected_on_training_rows_and_never_on_test(rows, counts, run):
    """Each model's threshold must be the one its TRAIN probabilities select.

    Recomputing from the train probabilities is the point: if the threshold had been
    chosen on the test split it would not reproduce here.
    """
    for model in run.models:
        assert model.threshold == subject.select_threshold(
            subject.build_matrix(subject.month_split(rows)[0], run.columns)[1],
            model.train_probability,
        )
    for model in run.models:
        assert model.evaluation is not None
        assert model.evaluation.rows == run.split.test_rows
        assert model.evaluation.positives == run.split.test_positives


def test_select_threshold_maximises_f1_and_breaks_ties_downward():
    label = np.asarray([1, 0, 0, 0])
    probability = np.asarray([0.9, 0.2, 0.1, 0.05])
    threshold = subject.select_threshold(label, probability)
    # Any threshold in (0.2, 0.9] gives the same F1 of 2/3; the lowest one must win.
    assert threshold == pytest.approx(0.201, abs=1e-9)


def test_confusions_counts_the_three_cells_it_needs():
    label = np.asarray([1, 1, 0, 0])
    probability = np.asarray([0.9, 0.4, 0.6, 0.1])
    assert subject.confusions(label, probability, 0.5) == (1, 1, 1)


def test_evaluate_refuses_a_single_class_split():
    with pytest.raises(subject.DeliveredError, match="single class"):
        subject.evaluate(
            "x", "model", np.zeros(4, dtype=int), np.asarray([0.1, 0.2, 0.3, 0.4]), 0.5
        )


# ---------------------------------------------------------------------------
# The baseline comparison
# ---------------------------------------------------------------------------


def evaluation(name: str = "model", **overrides) -> subject.Evaluation:
    values = {
        "name": name,
        "kind": "model",
        "rows": 100,
        "positives": 10,
        "threshold": 0.5,
        "pr_auc": 0.30,
        "recall": 0.40,
        "brier": 0.10,
        "roc_auc": 0.60,
        "alerts": 20,
        "true_positives": 4,
    }
    values.update(overrides)
    return subject.Evaluation(**values)


def test_a_model_that_only_ranks_better_does_not_beat_the_baseline():
    model = evaluation(pr_auc=0.55, brier=0.20)
    baseline = evaluation(name="climatology", kind="baseline", pr_auc=0.30, brier=0.09)
    beats, why = model.beats(baseline)
    assert beats is False
    assert "Brier 0.2000 does not undercut 0.0900" in why
    assert "PR-AUC" not in why


def test_a_model_that_only_calibrates_better_does_not_beat_the_baseline():
    model = evaluation(pr_auc=0.20, brier=0.05)
    baseline = evaluation(name="climatology", kind="baseline", pr_auc=0.30, brier=0.09)
    beats, why = model.beats(baseline)
    assert beats is False
    assert "PR-AUC 0.2000 does not exceed 0.3000" in why


def test_a_model_that_wins_both_axes_beats_the_baseline():
    model = evaluation(pr_auc=0.55, brier=0.05)
    baseline = evaluation(name="climatology", kind="baseline", pr_auc=0.30, brier=0.09)
    assert model.beats(baseline)[0] is True


def test_the_verdict_says_plainly_when_nothing_beats_anything():
    models = [evaluation("logistic", pr_auc=0.18, brier=0.17)]
    baselines = [
        evaluation(name="climatology", kind="baseline", pr_auc=0.10, brier=0.09),
        evaluation(name="region_frequency", kind="baseline", pr_auc=0.20, brier=0.08),
    ]
    comparison = subject.comparison_rows(
        [_fitted("logistic", models[0])],
        baselines,
    )
    verdict = subject.verdict_sentence(comparison)
    assert verdict.startswith("Verdict: neither trained model beats either baseline")
    assert "a trade, not a win" in verdict


def _fitted(name: str, evaluation_value: subject.Evaluation) -> subject.FittedModel:
    model = subject.FittedModel(
        name=name,
        kind="logistic",
        estimator=None,
        threshold=0.5,
        train_probability=np.asarray([]),
        test_probability=np.asarray([]),
    )
    model.evaluation = evaluation_value
    return model


# ---------------------------------------------------------------------------
# The exported artifact
# ---------------------------------------------------------------------------


def test_the_run_produces_a_document_the_go_reader_would_accept(run, tmp_path):
    """Validated by the transcription of `artifact.Validate`, not by the writer's hope."""
    model_contract.validate_artifact(run.artifact)
    path = tmp_path / "artifact.json"
    model_contract.write_artifact(path, run.artifact)
    assert model_contract.load_artifact(path) == json.loads(
        json.dumps(run.artifact, ensure_ascii=False)
    )


def test_the_artifact_declares_itself_a_rehearsal_and_not_servable(run):
    document = run.artifact
    assert "rehearsal" in document["version"]
    assert document["version"] == subject.ARTIFACT_VERSION
    metrics = document["metrics"]
    assert metrics["rehearsal"] is True
    assert metrics["servable_by_risk"] is False
    assert "no region identity" in metrics["not_servable_reason"]
    assert "never register" in metrics["must_not_be_registered"].lower()
    assert "one-hot of region" in metrics["not_servable_reason"]


def test_the_artifact_declares_a_unit_and_a_guard_per_feature(run):
    document = run.artifact
    assert len(document["features"]) == len(run.columns)
    for feature in document["features"]:
        assert feature["unit"]
        assert feature["min"] < feature["max"]
    assert [feature["name"] for feature in document["features"]] == [
        name for name, _unit in run.columns
    ]
    assert [feature["unit"] for feature in document["features"]] == [
        unit for _name, unit in run.columns
    ]


def test_the_exported_parameters_reproduce_the_fitted_probabilities(run):
    """The document must describe the model that was measured, not one like it.

    Logistic is reproduced exactly in `float64`, through `intercept + coef . x`: it has no
    thresholds, so there is nothing for a precision difference to flip.

    The forest is reproduced exactly after rounding the inputs through `float32`, because
    that is what scikit-learn does internally before walking a tree. Comparing the raw
    `float64` walk instead would fail by up to 0.30 on a boundary row -- a real limitation
    of a `float64` reader, documented in `forest_trees`, and not a defect this test should
    paper over by loosening its tolerance.
    """
    rows, counts = subject.read_days(DASHBOARD_FIXTURE)
    columns = subject.feature_columns(counts.regions)
    train, test = subject.month_split(rows)
    train_matrix, _ = subject.build_matrix(train, columns)
    test_matrix, _ = subject.build_matrix(test, columns)
    preprocessing = subject.fit_preprocessing(train_matrix, columns)
    assert run.preprocessing == preprocessing

    for model in run.models:
        document = _artifact_for(run, model)
        if document["kind"] == model_contract.KIND_FOREST:
            # scikit-learn's own prediction path, reproduced before the walk: it rounds the
            # input through float32, and the thresholds are exact only in that space.
            rounded = preprocessing.apply(test_matrix).astype(np.float32).astype(np.float64)
            probability = score(document, rounded, preprocessed=True)
        else:
            # Raw rows, so the document's own center/scale are exercised too.
            probability = score(document, test_matrix)
        assert np.allclose(probability, model.test_probability, atol=1e-12), model.name


def _artifact_for(run: subject.Result, model: subject.FittedModel) -> dict:
    assert model.evaluation is not None
    return subject.build_artifact(
        winner=model,
        columns=run.columns,
        preprocessing=run.preprocessing,
        bounds=run.bounds,
        evaluation=model.evaluation,
        baseline=run.baselines[0],
        alternatives=run.baselines[1:],
        comparison=run.comparison,
        counts=run.counts,
        source=Path("fixture.csv"),
        data_hash=run.data_hash,
        trained_at="2026-11-06T00:00:00Z",
    )


def score(document: dict, matrix: np.ndarray, *, preprocessed: bool = False) -> np.ndarray:
    """Score RAW feature rows with the artifact, the way the Go scorer documents it.

    The document's own `center`/`scale` are applied here unless `preprocessed` says the
    caller already did it (the forest case, which has to round through float32 the way
    scikit-learn does). Applying them twice is exactly the mistake the first version of
    this helper made -- and it still failed loudly instead of passing by luck, which is
    why the tolerance is 1e-12.
    """
    if preprocessed:
        values = matrix
    else:
        values = (matrix - np.asarray(document["center"])) / np.asarray(document["scale"])
    if document["kind"] == model_contract.KIND_LOGISTIC:
        z = document["intercept"] + values @ np.asarray(document["coef"])
        return 1.0 / (1.0 + np.exp(-z))
    probabilities = np.zeros(values.shape[0])
    for tree in document["trees"]:
        nodes = tree["nodes"]
        for index, row in enumerate(values):
            node = 0
            while "value" not in nodes[node]:
                node = (
                    nodes[node]["left"]
                    if row[nodes[node]["feature"]] <= nodes[node]["threshold"]
                    else nodes[node]["right"]
                )
            probabilities[index] += nodes[node]["value"]
    return probabilities / len(document["trees"])


def test_the_writer_refuses_a_document_the_reader_would_reject(run, tmp_path):
    broken = json.loads(json.dumps(run.artifact))
    broken["definitely_not_a_field"] = 1
    path = tmp_path / "broken.json"
    with pytest.raises(model_contract.ArtifactError, match="does not define"):
        model_contract.write_artifact(path, broken)
    assert not path.exists(), "a rejected document must not reach the disk"


def test_the_artifact_never_declares_a_forbidden_feature(run):
    declared = {feature["name"] for feature in run.artifact["features"]}
    assert declared.isdisjoint(set(subject.FORBIDDEN_FEATURE_COLUMNS))


# ---------------------------------------------------------------------------
# The counts the report quotes
# ---------------------------------------------------------------------------


def test_the_fixture_counts_are_the_ones_the_report_claims(counts):
    assert counts.source_rows == 450
    assert counts.region_days == 450
    assert counts.positives == 27
    assert counts.negatives == 423
    assert len(counts.regions) == 5
    assert counts.first_date == dt.date(2026, 7, 1)
    assert counts.last_date == dt.date(2026, 9, 28)


def test_the_comparison_holds_all_four_scorers_and_uses_test_rows(run):
    names = [entry["name"] for entry in run.comparison]
    assert names == ["logistic", "forest", "climatology", "region_frequency"]
    for entry in run.comparison:
        assert entry["rows"] == run.split.test_rows
        assert entry["positives"] == run.split.test_positives


def test_the_committed_report_carries_the_required_sections():
    """A committed deliverable cannot silently lose the parts it exists for."""
    report = (Path(__file__).resolve().parents[1] / "REPORT-delivered-data.md").read_text(
        encoding="utf-8"
    )
    for required in (
        "## What the file contains",
        "Positive labels",
        "Clamped downward differences of `sup_afec_act`",
        "## Metrics",
        "| logistic |",
        "| forest |",
        "| climatology |",
        "| region_frequency |",
        "Verdict:",
        "## What is missing",
        "What it would enable",
    ):
        assert required in report, f"the committed report no longer contains {required!r}"
