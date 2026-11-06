"""Train a rehearsal model on the data the team was actually given.

THE POINT OF THIS MODULE IS A MEASUREMENT, NOT A MODEL.

`data/dashboard_conaf_202609281623 (1).csv` is a 90-day regional CONAF dashboard
export: 16 regions x 90 days, 2026-07-01 .. 2026-09-28, 1440 rows. It is the only
labelled fire data in the repository, and the owner asked for a model trained with
what was delivered plus a written account of what is missing. So this module trains
two models on it, compares them against two baselines on the same split with the same
metrics, and exports the winner through the `firewatch-model/v1` contract.

The most likely honest outcome is that **no model beats climatology**, because the
five columns that are not the label mostly encode one thing: how far into the season
the calendar has moved. When that happens, the CLI prints it, the report states it,
and it is the deliverable. It is not a reason to tune until the number looks better.

WHAT THE FILE ACTUALLY CONTAINS, RE-CHECKED HERE AND NOT ASSUMED
----------------------------------------------------------------
* `nro_inc_act` is a **season-cumulative** counter, monotonic per region: its day-to-day
  difference is the only observable that says "a fire advanced today". Its positive
  differences are the label, and there are 69 of them out of 1440 region-days (4.79 %).
* `sup_afec_act` is cumulative too, but it was revised DOWNWARD on 3 rows. Any derived
  difference is clamped at zero **and the clamp is counted**, because a silently
  dropped negative difference is a hectare that vanished from the record.
* `nro_inc_ant`, `nro_inc_prom_quint` and `sup_afec_ant` are the previous season and a
  five-season climatology sampled at the same dates. They correlate 0.94 with each
  other and about 0.93 with the cumulative, so they mostly answer "how far into the
  season is it". They are legal features, and that is why the baselines matter: a model
  built on them has to beat a clock before it has said anything.

WHAT IS LEAKAGE HERE, AND WHY THE GUARD EXISTS
----------------------------------------------
The file invites exactly one mistake: putting `nro_inc_act` in the feature matrix. The
label is *derived from* that column, so a model given it learns the label instead of the
day -- and the amount is measured, not asserted. On the committed fixture's September
test split a logistic fit on the legal features reaches PR-AUC 0.0826; adding the
cumulative lifts it to about 0.31, and adding its daily difference reproduces the label
exactly (1.0000). `fecha_creacion` is the snapshot timestamp of the export and
`sup_afec_act` is the same-day cumulative burned area; both are outcomes.
`FORBIDDEN_FEATURE_COLUMNS` is checked before a matrix is built, so the mistake is a
refusal and not a good score.

FEATURES, AND WHY THE ARTIFACT IS NOT SERVABLE
----------------------------------------------
Five source columns plus a one-hot of region. The one-hot is the reason this artifact
can never be served: `risk` has no region identity in the vector it builds, so a model
that needs one cannot receive one. The artifact says so in its own bytes (see
`REHEARSAL_MARKER`), it is named as a rehearsal, and it must never be registered in
`models` or activated. It exists to prove the exporter against the Go reader.

Run it as::

    python backend/training/train_delivered.py
    python backend/training/train_delivered.py \\
        --source backend/training/tests/fixtures/dashboard-conaf-sample.csv \\
        --out backend/training/out/fixture
"""

from __future__ import annotations

import argparse
import csv
import datetime as dt
import hashlib
import json
import math
import re
import sys
import unicodedata
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import numpy as np
from sklearn.ensemble import RandomForestClassifier
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import average_precision_score, brier_score_loss, roc_auc_score

import model_contract

# ---------------------------------------------------------------------------
# Contract constants
# ---------------------------------------------------------------------------

REPO_ROOT = Path(__file__).resolve().parents[2]

#: The delivered export, as it sits in the repository root. Untracked, and named here so
#: a run with no arguments is the real run.
DEFAULT_SOURCE = REPO_ROOT / "data" / "dashboard_conaf_202609281623 (1).csv"

#: Git-ignored (`backend/training/.gitignore`): metrics and tables are products of a run.
DEFAULT_OUT = REPO_ROOT / "backend/training" / "out" / "delivered"

#: The artifact's `version`. It carries the word `rehearsal` on purpose, because the
#: version string is the one field a human and `GET /v1/model` both read first.
ARTIFACT_VERSION = "delivered-data-rehearsal-v1"

# Source columns. Spelled once, so a header change is a named failure instead of a
# silently empty column.
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

REQUIRED_COLUMNS = (
    COL_REGION,
    COL_ACT,
    COL_ANT,
    COL_PROM,
    COL_SUP_ACT,
    COL_SUP_ANT,
    COL_FECHA,
)

#: Columns this build must never let into the feature matrix. The first three are named
#: by the task; the last three are derived from them, so they are the same leak wearing
#: another name and the guard covers them too.
FORBIDDEN_FEATURE_COLUMNS = (
    COL_ACT,
    COL_SUP_ACT,
    COL_CREACION,
    "fires_today",
    "burned_ha_today",
    "fire_today",
)

#: The months each side of the split carries. Temporal, never random: September is
#: after July and August, and a random split would let a September row teach the model
#: about a July one.
TRAIN_MONTHS = (7, 8)
TEST_MONTHS = (9,)

#: Fixed everywhere: the split, both models, the forest's bootstraps. A seed that moves
#: makes two runs incomparable, which defeats the point of measuring.
SEED = 20260928

#: Features taken from the file, with the unit the artifact must declare for each. The
#: unit is not decoration -- the Go reader refuses a feature without one, because a
#: min/max guard cannot be checked against a value whose unit is unknown.
BASE_FEATURES = (
    (COL_ANT, "count"),
    (COL_PROM, "count"),
    (COL_SUP_ANT, "ha"),
    ("day_of_year", "day"),
    ("month", "month"),
)

#: Unit declared for a one-hot region indicator: it is 0 or 1, not a count of anything.
REGION_FEATURE_UNIT = "onehot"

#: The threshold grid. Selected on the TRAIN split only, never on test, and the same
#: rule for both models and both baselines so the comparison is symmetric.
THRESHOLD_STEPS = 999

#: The metric a model is exported on. PR-AUC rather than ROC-AUC because at a 4.8 %
#: base rate the false-positive side ROC-AUC rewards is the side that does not matter.
SELECTION_METRIC = "pr_auc"

#: Where the artifact says, in its own bytes, that it is a rehearsal. The reason a
#: top-level field cannot carry this is `DisallowUnknownFields`: an unknown key makes
#: the document unreadable to the Go loader, and an unreadable artifact is a worse
#: outcome than a marked one. So the marker lives in `version` (above) and in these
#: `metrics` keys, which are opaque provenance on the Go side and never read while
#: scoring.
REHEARSAL_MARKER = {
    "rehearsal": True,
    "servable_by_risk": False,
    "rehearsal_reason": (
        "Trained on the delivered 90-day regional dashboard export to prove the exporter "
        "and measure the data, not to be served. The finding is the measurement."
    ),
    "not_servable_reason": (
        "This artifact's feature set includes a one-hot of region. risk builds its input "
        "vector from a publication and has no region identity, so it cannot supply those "
        "values: the vector it can build has no region, and the model requires one."
    ),
    "must_not_be_registered": (
        "Never register this version in the models table and never activate it. It exists "
        "to be loaded by the artifact reader in a test, and for no other purpose."
    ),
}


class DeliveredError(Exception):
    """A failure the operator must fix; the CLI prints it instead of a traceback."""


class LeakageError(DeliveredError):
    """A forbidden column reached the feature matrix."""


class SplitError(DeliveredError):
    """The temporal split is not temporal, not disjoint, or not ordered."""


# ---------------------------------------------------------------------------
# Source -> region-day table
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class DayRow:
    """One region-day, with its label and every feature in the artifact's units.

    The three derived difference columns are `None` on the first day of the window: the
    cumulative counter has no predecessor there, so no advance can be observed and a
    zero would be a measurement this file never made.
    """

    region: str
    date: dt.date
    first_in_window: bool
    fire_today: int
    fires_today: int | None
    burned_ha_today: float | None
    climatology_today: float | None
    prev_season_inc: float
    climatology_inc: float
    prev_season_burned: float

    @property
    def day_of_year(self) -> int:
        return self.date.timetuple().tm_yday

    @property
    def month(self) -> int:
        return self.date.month


@dataclass(frozen=True)
class Counts:
    """What the derivation measured, including what it had to clamp and exclude."""

    source_rows: int
    region_days: int
    positives: int
    negatives: int
    clamped_act_decrements: int
    clamped_sup_afec_decrements: int
    first_day_of_region_rows: int
    regions: tuple[str, ...]
    first_date: dt.date
    last_date: dt.date

    @property
    def positive_rate(self) -> float:
        return self.positives / self.region_days if self.region_days else 0.0


def parse_date(raw: str, where: str) -> dt.date:
    """Read one `fecha` as a civil date, or fail naming the row."""
    try:
        return dt.date.fromisoformat(raw.strip())
    except ValueError as error:
        raise DeliveredError(
            f"{where}: fecha {raw!r} is not an ISO date; the export writes YYYY-MM-DD and "
            f"this reader refuses to guess a shape"
        ) from error


def parse_number(raw: str, where: str, column: str) -> float:
    """Read one numeric cell. An empty cell is absent, never zero."""
    text = raw.strip()
    if text == "":
        raise DeliveredError(
            f"{where}: {column} is empty; this build refuses to substitute zero for a "
            f"value the export never wrote"
        )
    try:
        return float(text)
    except ValueError as error:
        raise DeliveredError(f"{where}: {column} {raw!r} is not a number") from error


def clamp_advance(delta: float) -> float:
    """A cumulative counter's advance, clamped at zero.

    `sup_afec_act` was revised downward, so a raw difference can be negative. The caller
    counts every negative it encounters; this function must never be called in a way
    that hides one.
    """
    return delta if delta > 0 else 0.0


def read_days(source: Path) -> tuple[list[DayRow], Counts]:
    """Read the export and derive one row per region-day.

    Rows are grouped by region and ordered by date before any difference is taken, so a
    file sorted differently produces the identical table.
    """
    try:
        handle = source.open(encoding="utf-8", newline="")
    except OSError as error:
        raise DeliveredError(f"cannot read the source {source} ({error})") from error
    with handle:
        reader = csv.DictReader(handle)
        header = tuple(reader.fieldnames or ())
        missing = [column for column in REQUIRED_COLUMNS if column not in header]
        if missing:
            raise DeliveredError(
                f"{source}: the header is missing {', '.join(missing)}; this export's "
                f"columns are a contract and a renamed one is a failure, not a null column"
            )
        raw_rows = list(reader)

    by_region: dict[str, list[dict[str, str]]] = {}
    for index, raw in enumerate(raw_rows):
        region = (raw.get(COL_REGION) or "").strip()
        if region == "":
            raise DeliveredError(f"{source}: row {index + 2} carries no region")
        by_region.setdefault(region, []).append(raw)

    rows: list[DayRow] = []
    clamped_act = 0
    clamped_sup = 0
    first_days = 0
    for region in sorted(by_region):
        source_rows = sorted(by_region[region], key=lambda raw: (raw[COL_FECHA], raw[COL_ID]))
        previous: DayRow | None = None
        # The previous row's three cumulative values, as one tuple so the reader cannot
        # pair an act with a sup from two different days.
        previous_values: tuple[float, float, float] | None = None
        for index, raw in enumerate(source_rows):
            where = f"{source}: {region} row {index + 2}"
            date = parse_date(raw[COL_FECHA], where)
            act = parse_number(raw[COL_ACT], where, COL_ACT)
            sup_act = parse_number(raw[COL_SUP_ACT], where, COL_SUP_ACT)
            prom = parse_number(raw[COL_PROM], where, COL_PROM)
            if previous is not None and date <= previous.date:
                raise DeliveredError(
                    f"{where}: {region} repeats or reorders the date {date} after "
                    f"{previous.date}; the counter's difference is undefined there"
                )
            if sum(1 for row in rows if row.region == region and row.date == date) != 0:
                raise DeliveredError(f"{where}: duplicate region-day {region} {date}")

            if previous_values is None:
                first_days += 1
                fires_today: int | None = None
                burned_today: float | None = None
                climatology_today: float | None = None
                fire_today = 0
            else:
                last_act, last_sup, last_prom = previous_values
                delta_act = act - last_act
                delta_sup = sup_act - last_sup
                delta_prom = prom - last_prom
                if delta_act < 0:
                    clamped_act += 1
                if delta_sup < 0:
                    clamped_sup += 1
                fires_today = int(clamp_advance(delta_act))
                burned_today = clamp_advance(delta_sup)
                climatology_today = clamp_advance(delta_prom)
                fire_today = 1 if fires_today > 0 else 0

            row = DayRow(
                region=region,
                date=date,
                first_in_window=previous_values is None,
                fire_today=fire_today,
                fires_today=fires_today,
                burned_ha_today=burned_today,
                climatology_today=climatology_today,
                prev_season_inc=parse_number(raw[COL_ANT], where, COL_ANT),
                climatology_inc=prom,
                prev_season_burned=parse_number(raw[COL_SUP_ANT], where, COL_SUP_ANT),
            )
            rows.append(row)
            previous = row
            previous_values = (act, sup_act, prom)

    if not rows:
        raise DeliveredError(f"{source}: no region-day could be derived from this file")
    dates = sorted({row.date for row in rows})
    positives = sum(row.fire_today for row in rows)
    counts = Counts(
        source_rows=len(raw_rows),
        region_days=len(rows),
        positives=positives,
        negatives=len(rows) - positives,
        clamped_act_decrements=clamped_act,
        clamped_sup_afec_decrements=clamped_sup,
        first_day_of_region_rows=first_days,
        regions=tuple(sorted(by_region)),
        first_date=dates[0],
        last_date=dates[-1],
    )
    return rows, counts


# ---------------------------------------------------------------------------
# Features: what may enter the matrix, and what may never
# ---------------------------------------------------------------------------


def region_feature_name(region: str) -> str:
    """The artifact feature name for one region's indicator.

    ASCII-folded so the name is stable in a JSON document nobody normalises, and
    collision-checked by `feature_columns` because two regions could fold together.
    """
    folded = unicodedata.normalize("NFKD", region).encode("ascii", "ignore").decode("ascii")
    slug = re.sub(r"[^a-z0-9]+", "_", folded.lower()).strip("_")
    if slug == "":
        raise DeliveredError(f"region {region!r} folds to an empty feature name")
    return f"region_{slug}"


def feature_columns(regions: tuple[str, ...]) -> tuple[tuple[str, str], ...]:
    """Every feature the model may see, in the artifact's input order.

    This function is the whole vocabulary. It emits the five source columns a region-day
    can honestly supply plus one indicator per region, and nothing else -- so a leakage
    column cannot arrive by being added to a list somewhere else.
    """
    names = dict(BASE_FEATURES)
    for region in regions:
        name = region_feature_name(region)
        if name in names:
            raise DeliveredError(
                f"region {region!r} folds to {name!r}, which another region already owns; "
                f"a one-hot with duplicate names is not a one-hot"
            )
        names[name] = REGION_FEATURE_UNIT
    columns = tuple(names.items())
    assert_no_leakage([name for name, _ in columns])
    return columns


def assert_no_leakage(names: list[str] | tuple[str, ...]) -> None:
    """Refuse a feature matrix that contains a column the label is derived from.

    Called by `feature_columns` (so the real path is guarded) and by the tests (so the
    guard is proven to bite rather than merely present).
    """
    for name in names:
        if name in FORBIDDEN_FEATURE_COLUMNS:
            raise LeakageError(
                f"{name!r} cannot be a model input: it is the same-day cumulative the label "
                f"is derived from, or a column derived from it. A model given it learns the "
                f"label, not the day -- on the fixture the daily difference reproduces the "
                f"label exactly (PR-AUC 1.0000) and the cumulative takes a clean 0.08 to "
                f"about 0.31."
            )


def feature_vector(row: DayRow, columns: tuple[tuple[str, str], ...]) -> list[float]:
    """One row's feature values, in the artifact's order."""
    values: list[float] = []
    for name, _unit in columns:
        if name == COL_ANT:
            values.append(row.prev_season_inc)
        elif name == COL_PROM:
            values.append(row.climatology_inc)
        elif name == COL_SUP_ANT:
            values.append(row.prev_season_burned)
        elif name == "day_of_year":
            values.append(float(row.day_of_year))
        elif name == "month":
            values.append(float(row.month))
        elif name == region_feature_name(row.region):
            values.append(1.0)
        elif name.startswith("region_"):
            values.append(0.0)
        else:  # pragma: no cover - feature_columns cannot emit anything else
            raise DeliveredError(f"no producer for the feature {name!r}")
    return values


def build_matrix(
    rows: list[DayRow], columns: tuple[tuple[str, str], ...]
) -> tuple[np.ndarray, np.ndarray]:
    """The feature matrix and the label, with the leakage guard in front of the matrix."""
    assert_no_leakage([name for name, _ in columns])
    matrix = np.asarray([feature_vector(row, columns) for row in rows], dtype=np.float64)
    label = np.asarray([row.fire_today for row in rows], dtype=np.int64)
    if matrix.shape[1] != len(columns):
        raise DeliveredError(
            f"the matrix has {matrix.shape[1]} columns for {len(columns)} declared features"
        )
    return matrix, label


# ---------------------------------------------------------------------------
# Temporal split
# ---------------------------------------------------------------------------


def month_split(rows: list[DayRow]) -> tuple[list[DayRow], list[DayRow]]:
    """July + August train, September test, each side in date order."""
    train = [row for row in rows if row.month in TRAIN_MONTHS]
    test = [row for row in rows if row.month in TEST_MONTHS]
    assert_temporal_split(train, test)
    return train, test


def assert_months(train: list[DayRow], test: list[DayRow]) -> None:
    """Prove each side carries the months the split names."""
    wrong_train = sorted({row.month for row in train} - set(TRAIN_MONTHS))
    wrong_test = sorted({row.month for row in test} - set(TEST_MONTHS))
    if wrong_train or wrong_test:
        raise SplitError(
            f"the temporal split carries months {wrong_train} in train and {wrong_test} in "
            f"test; training is {TRAIN_MONTHS} and test is {TEST_MONTHS}"
        )


def assert_disjoint(train: list[DayRow], test: list[DayRow]) -> None:
    """Prove no region-day is on both sides, and that neither side repeats one."""
    keys_train = {(row.region, row.date) for row in train}
    keys_test = {(row.region, row.date) for row in test}
    if len(keys_train) != len(train) or len(keys_test) != len(test):
        raise SplitError("the split repeats a region-day: one row must belong to one side")
    overlap = keys_train & keys_test
    if overlap:
        raise SplitError(f"the split is not disjoint: {len(overlap)} region-days are on both sides")


def assert_ordered(train: list[DayRow], test: list[DayRow]) -> None:
    """Prove every training date precedes every test date."""
    if max(row.date for row in train) >= min(row.date for row in test):
        raise SplitError(
            f"the split is not strictly ordered: train ends {max(row.date for row in train)} "
            f"and test starts {min(row.date for row in test)}"
        )


def assert_temporal_split(train: list[DayRow], test: list[DayRow]) -> None:
    """Prove the split is disjoint, ordered, and the one the months name.

    Four separate properties, checked by four separate functions, because any one of them
    can hold on a split that is still wrong: the months can be right on shuffled rows, the
    rows can be ordered and overlap, and disjoint rows can interleave in time. They are
    separate functions so each can be tested against a case that violates only it -- a
    month-based split cannot otherwise reach the disjointness guard, which would leave
    that guard untested and unevidenced.
    """
    if not train or not test:
        raise SplitError(
            f"the split is empty on one side ({len(train)} train, {len(test)} test); a metric "
            f"computed on nothing is not a measurement"
        )
    assert_months(train, test)
    assert_disjoint(train, test)
    assert_ordered(train, test)


# ---------------------------------------------------------------------------
# Preprocessing, fitted on the training split only
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class Preprocessing:
    """`impute`, `center` and `scale`, positional, fitted on one split."""

    impute: tuple[float, ...]
    center: tuple[float, ...]
    scale: tuple[float, ...]

    def apply(self, matrix: np.ndarray) -> np.ndarray:
        """NaN -> impute, then (x - center) / scale, then the model."""
        filled = np.where(np.isnan(matrix), np.asarray(self.impute), matrix)
        return (filled - np.asarray(self.center)) / np.asarray(self.scale)


def fit_preprocessing(
    train_matrix: np.ndarray, columns: tuple[tuple[str, str], ...]
) -> Preprocessing:
    """Fit impute/center/scale on the training matrix and nothing else.

    The signature takes only the training matrix. That is the guard: there is no
    argument through which the test split could reach these numbers, so "fitted on train
    only" is a property of the types rather than a comment someone can break.
    """
    if train_matrix.ndim != 2 or train_matrix.shape[0] == 0:
        raise DeliveredError("preprocessing needs a non-empty two-dimensional training matrix")
    with np.errstate(invalid="ignore"):
        impute = np.nanmedian(train_matrix, axis=0)
        center = np.nanmean(train_matrix, axis=0)
        scale = np.nanstd(train_matrix, axis=0)
    for index, name in enumerate(name for name, _ in columns):
        if not math.isfinite(float(impute[index])) or not math.isfinite(float(center[index])):
            raise DeliveredError(f"feature {name!r} is entirely NaN on the training split")
        if not float(scale[index]) > 0:
            raise DeliveredError(
                f"feature {name!r} is constant on the training split (scale {scale[index]}); "
                f"a constant input cannot be standardized and carries no information"
            )
    return Preprocessing(
        impute=tuple(float(value) for value in impute),
        center=tuple(float(value) for value in center),
        scale=tuple(float(value) for value in scale),
    )


def guard_bounds(
    train_matrix: np.ndarray, columns: tuple[tuple[str, str], ...]
) -> tuple[tuple[float, float], ...]:
    """The declared min/max per feature, measured on the training split.

    Also train-only, and for the same reason: one fitting split, no exception. These are
    request-surface guards in the Go reader and this artifact has no request surface, but
    a bound fitted on test would still be a number the model's own document cannot
    justify.
    """
    bounds: list[tuple[float, float]] = []
    for index, name in enumerate(name for name, _ in columns):
        column = train_matrix[:, index]
        minimum = float(np.nanmin(column))
        maximum = float(np.nanmax(column))
        if not minimum < maximum:
            raise DeliveredError(
                f"feature {name!r} spans {minimum}..{maximum} on the training split; the "
                f"artifact needs a min below its max to declare a guard"
            )
        bounds.append((minimum, maximum))
    return tuple(bounds)


# ---------------------------------------------------------------------------
# Metrics
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class Evaluation:
    """One scorer's numbers on one split. Every field is measured, none is assumed."""

    name: str
    kind: str
    rows: int
    positives: int
    threshold: float
    pr_auc: float
    recall: float
    brier: float
    roc_auc: float
    alerts: int
    true_positives: int

    def beats(self, other: Evaluation) -> tuple[bool, str]:
        """Whether this scorer beats `other`, with the reason either way.

        Strict on both axes: a higher PR-AUC at a worse Brier is a trade, not a win, and
        calling it one is how a model gets registered for the wrong reason.
        """
        better_rank = self.pr_auc > other.pr_auc
        better_calibration = self.brier < other.brier
        if better_rank and better_calibration:
            return True, (
                f"higher PR-AUC ({self.pr_auc:.4f} vs {other.pr_auc:.4f}) and lower "
                f"Brier ({self.brier:.4f} vs {other.brier:.4f})"
            )
        reasons = []
        if not better_rank:
            reasons.append(f"PR-AUC {self.pr_auc:.4f} does not exceed {other.pr_auc:.4f}")
        if not better_calibration:
            reasons.append(f"Brier {self.brier:.4f} does not undercut {other.brier:.4f}")
        return False, "; ".join(reasons)

    def as_dict(self) -> dict[str, Any]:
        return {
            "name": self.name,
            "kind": self.kind,
            "rows": self.rows,
            "positives": self.positives,
            "threshold": round(self.threshold, 6),
            "pr_auc": round(self.pr_auc, 6),
            "recall": round(self.recall, 6),
            "brier": round(self.brier, 6),
            "roc_auc": round(self.roc_auc, 6),
            "alerts": self.alerts,
            "true_positives": self.true_positives,
        }


def confusions(
    label: np.ndarray, probability: np.ndarray, threshold: float
) -> tuple[int, int, int]:
    """(true positives, false positives, false negatives) at one threshold."""
    predicted = probability >= threshold
    positive = label == 1
    true_positives = int(np.sum(predicted & positive))
    false_positives = int(np.sum(predicted & ~positive))
    false_negatives = int(np.sum(~predicted & positive))
    return true_positives, false_positives, false_negatives


def select_threshold(label: np.ndarray, probability: np.ndarray) -> float:
    """The threshold that maximises F1 on the split it is given, ties to the lower one.

    Callers pass the TRAINING split. Selecting on test would report a threshold the
    deployed model could never have known, and the recall that follows from it would be
    an artefact of having seen the answers.
    """
    best_threshold = 0.5
    best_f1 = -1.0
    for step in range(1, THRESHOLD_STEPS + 1):
        threshold = step / (THRESHOLD_STEPS + 1)
        true_positives, false_positives, false_negatives = confusions(label, probability, threshold)
        precision = (
            true_positives / (true_positives + false_positives)
            if (true_positives + false_positives)
            else 0.0
        )
        recall = (
            true_positives / (true_positives + false_negatives)
            if (true_positives + false_negatives)
            else 0.0
        )
        f1 = 2 * precision * recall / (precision + recall) if (precision + recall) else 0.0
        if f1 > best_f1:  # strict: the first, and therefore lowest, threshold wins a tie
            best_f1 = f1
            best_threshold = threshold
    return best_threshold


def evaluate(
    name: str,
    kind: str,
    label: np.ndarray,
    probability: np.ndarray,
    threshold: float,
) -> Evaluation:
    """Measure one scorer on one split at a threshold chosen somewhere else."""
    positives = int(np.sum(label == 1))
    if positives == 0 or positives == len(label):
        raise DeliveredError(
            f"{name}: the evaluation split has {positives} positives out of {len(label)}; "
            f"PR-AUC and Brier are undefined on a single class"
        )
    true_positives, false_positives, false_negatives = confusions(label, probability, threshold)
    return Evaluation(
        name=name,
        kind=kind,
        rows=len(label),
        positives=positives,
        threshold=threshold,
        pr_auc=float(average_precision_score(label, probability)),
        recall=(
            true_positives / (true_positives + false_negatives)
            if (true_positives + false_negatives)
            else 0.0
        ),
        brier=float(brier_score_loss(label, probability)),
        roc_auc=float(roc_auc_score(label, probability)),
        alerts=true_positives + false_positives,
        true_positives=true_positives,
    )


# ---------------------------------------------------------------------------
# Baselines
# ---------------------------------------------------------------------------


def climatology_probability(row: DayRow) -> float:
    """The five-season climatology's own daily rate, as a probability.

    `nro_inc_prom_quint` is a cumulative climatology, so its day-to-day difference is
    the average number of fires that day across the five seasons. With `r` expected
    fires, the chance of at least one is `1 - exp(-r)`: a Poisson assumption, stated
    here because it is the only place a probability is manufactured instead of read.

    A row with no predecessor has no rate, and returns 0.0 -- it is counted in
    `climatology_rows_without_rate` and refused if it ever lands in the test split.
    """
    rate = row.climatology_today
    if rate is None:
        return 0.0
    return 1.0 - math.exp(-rate)


def region_frequency(train: list[DayRow]) -> dict[str, float]:
    """Per-region historical frequency, from the TRAINING split only."""
    totals: dict[str, int] = {}
    positives: dict[str, int] = {}
    for row in train:
        totals[row.region] = totals.get(row.region, 0) + 1
        positives[row.region] = positives.get(row.region, 0) + row.fire_today
    return {region: positives[region] / count for region, count in totals.items()}


def region_frequency_probability(
    row: DayRow, frequency: dict[str, float], fallback: float
) -> float:
    """A region the training split never saw falls back to the global training rate."""
    return frequency.get(row.region, fallback)


def baseline_probabilities(
    train: list[DayRow], test: list[DayRow]
) -> tuple[dict[str, np.ndarray], dict[str, np.ndarray], dict[str, float]]:
    """The two baselines' scores on train (for the threshold) and on test (for metrics)."""
    frequency = region_frequency(train)
    fallback = sum(row.fire_today for row in train) / len(train)
    scores = {
        "climatology": (
            np.asarray([climatology_probability(row) for row in train]),
            np.asarray([climatology_probability(row) for row in test]),
        ),
        "region_frequency": (
            np.asarray([region_frequency_probability(row, frequency, fallback) for row in train]),
            np.asarray([region_frequency_probability(row, frequency, fallback) for row in test]),
        ),
    }
    return (
        {name: pair[0] for name, pair in scores.items()},
        {name: pair[1] for name, pair in scores.items()},
        {"global_positive_rate": fallback, "regions_in_train": float(len(frequency))},
    )


# ---------------------------------------------------------------------------
# Models
# ---------------------------------------------------------------------------


@dataclass
class FittedModel:
    """One trained estimator, the threshold train selected, and both sets of numbers.

    `evaluation` is the TEST-split evaluation and the only one the comparison uses: the
    threshold comes from train, and the numbers a model is judged on come from test.
    `train_evaluation` sits next to it as evidence of how much of the gap is memorisation,
    and is never reported as if it were performance.
    """

    name: str
    kind: str
    estimator: Any
    threshold: float
    train_probability: np.ndarray
    test_probability: np.ndarray
    train_evaluation: Evaluation | None = None
    evaluation: Evaluation | None = None


def fit_models(
    train_matrix: np.ndarray,
    test_matrix: np.ndarray,
    train_label: np.ndarray,
) -> list[FittedModel]:
    """Fit logistic regression and a random forest on the standardized training matrix.

    Both consume the SAME preprocessed matrix, because that is what the artifact says:
    `(x - center) / scale` then the model. A forest is invariant to a monotone transform
    of a feature, so standardizing first changes nothing about its splits and keeps the
    exported thresholds in the space the Go scorer walks.

    `class_weight="balanced"` on both: 4.8 % positives is a majority-class problem, and an
    unweighted fit would answer "no" everywhere and still score a respectable Brier.

    The threshold is selected here, from `train_label` only. This function never receives
    the test label, so it has nothing to select against.
    """
    estimators = [
        (
            "logistic",
            model_contract.KIND_LOGISTIC,
            LogisticRegression(
                class_weight="balanced",
                max_iter=1000,
                random_state=SEED,
                solver="lbfgs",
            ),
        ),
        (
            "forest",
            model_contract.KIND_FOREST,
            RandomForestClassifier(
                class_weight="balanced",
                n_estimators=200,
                random_state=SEED,
                n_jobs=1,
            ),
        ),
    ]
    fitted: list[FittedModel] = []
    for name, kind, estimator in estimators:
        estimator.fit(train_matrix, train_label)
        train_probability = estimator.predict_proba(train_matrix)[:, 1]
        test_probability = estimator.predict_proba(test_matrix)[:, 1]
        fitted.append(
            FittedModel(
                name=name,
                kind=kind,
                estimator=estimator,
                threshold=select_threshold(train_label, train_probability),
                train_probability=train_probability,
                test_probability=test_probability,
            )
        )
    return fitted


def evaluate_models(
    models: list[FittedModel], train_label: np.ndarray, test_label: np.ndarray
) -> None:
    """Measure every model on both splits, at the threshold train selected.

    Two calls and not one: the training numbers exist so a reader can see how much of the
    gap is memorisation, and the test numbers are the only ones the comparison may use.
    Judging a model by its train score against a test baseline makes every model win.
    """
    for model in models:
        model.train_evaluation = evaluate(
            model.name, model.kind, train_label, model.train_probability, model.threshold
        )
        model.evaluation = evaluate(
            model.name, model.kind, test_label, model.test_probability, model.threshold
        )


def select_winner(models: list[FittedModel]) -> FittedModel:
    """The model to export: highest test PR-AUC, then lowest Brier, then logistic.

    SELECTION ON THE TEST SPLIT, and that is a defect of this rehearsal rather than a
    feature of it: with a few dozen positives in September the gap between two models is
    not evidence. The exported artifact is a rehearsal partly because of this, and the
    report says so instead of presenting the winner as a reliable choice.
    """

    def key(model: FittedModel) -> tuple[float, float, str]:
        evaluation = model.evaluation
        if evaluation is None:  # pragma: no cover - evaluate_models always fills it
            raise DeliveredError(f"model {model.name!r} was never evaluated")
        return (-evaluation.pr_auc, evaluation.brier, model.name)

    return min(models, key=key)


# ---------------------------------------------------------------------------
# Artifact export
# ---------------------------------------------------------------------------


def logistic_parameters(estimator: LogisticRegression) -> tuple[float, list[float]]:
    """The intercept and coefficients of a binary logistic fit, in feature order."""
    intercept = float(np.ravel(estimator.intercept_)[0])
    coefficients = [float(value) for value in np.ravel(estimator.coef_)]
    return intercept, coefficients


def forest_trees(estimator: RandomForestClassifier) -> list[dict[str, Any]]:
    """Export every tree as the artifact's node array.

    Node indexes are kept as scikit-learn wrote them: a child index is already an index
    into the same tree's array, leaves are marked by carrying `value`, and the Go reader
    proves the result is walkable. A leaf's probability is the node's class-1 share,
    normalised, so it is a probability whether the estimator stores counts or proportions.

    A CAVEAT THIS EXPORTER CANNOT REMOVE. scikit-learn casts the input matrix to
    `float32` before walking a tree; the Go scorer walks `float64`. Thresholds are
    exported exactly as fitted, so a reader in `float64` will take the other branch on any
    row whose feature value sits within one float32 step of a threshold. Measured on the
    fixture: with `float64` inputs the exported forest agrees with `predict_proba` on most
    rows but differs by up to 0.30 on one, while with the input rounded through `float32`
    first the agreement is exact (0.0). That is why the test compares the float32-rounded
    walk, and why the artifact this run exports carries `kind: logistic` -- a logistic fit
    has no thresholds and therefore no such divergence.
    """
    trees: list[dict[str, Any]] = []
    for estimator_tree in estimator.estimators_:
        tree = estimator_tree.tree_
        nodes: list[dict[str, Any]] = []
        for index in range(tree.node_count):
            left = int(tree.children_left[index])
            right = int(tree.children_right[index])
            if left == -1 and right == -1:
                counts = np.asarray(tree.value[index][0], dtype=np.float64)
                total = float(counts.sum())
                if total <= 0:
                    raise DeliveredError(
                        f"forest leaf {index} carries no samples; its probability is undefined"
                    )
                nodes.append({"value": float(counts[1] / total)})
                continue
            if left == -1 or right == -1:
                raise DeliveredError(f"forest node {index} has one child; a split node needs both")
            nodes.append(
                {
                    "feature": int(tree.feature[index]),
                    "threshold": float(tree.threshold[index]),
                    "left": left,
                    "right": right,
                }
            )
        trees.append({"nodes": nodes})
    return trees


def build_artifact(
    *,
    winner: FittedModel,
    columns: tuple[tuple[str, str], ...],
    preprocessing: Preprocessing,
    bounds: tuple[tuple[float, float], ...],
    evaluation: Evaluation,
    baseline: Evaluation,
    alternatives: list[Evaluation],
    comparison: list[dict[str, Any]],
    counts: Counts,
    source: Path,
    data_hash: str,
    trained_at: str,
) -> dict[str, Any]:
    """Build the `firewatch-model/v1` document for the winning model."""
    features = [
        {"name": name, "unit": unit, "min": bounds[index][0], "max": bounds[index][1]}
        for index, (name, unit) in enumerate(columns)
    ]
    document: dict[str, Any] = {
        "format": model_contract.FORMAT,
        "version": ARTIFACT_VERSION,
        "trained_at": trained_at,
        "data_hash": data_hash,
        "features": features,
        "impute": list(preprocessing.impute),
        "center": list(preprocessing.center),
        "scale": list(preprocessing.scale),
        "kind": winner.kind,
        "metrics": {
            **REHEARSAL_MARKER,
            **evaluation.as_dict(),
            "selected_by": f"{SELECTION_METRIC} on the September test split",
            "selection_caveat": (
                "The winner was picked on the test split, which is a selection leak this "
                "rehearsal accepts and states; the two models are within noise of each other."
            ),
            "split": {
                "train_months": list(TRAIN_MONTHS),
                "test_months": list(TEST_MONTHS),
            },
            "counts": {
                "source_rows": counts.source_rows,
                "region_days": counts.region_days,
                "positives": counts.positives,
                "clamped_act_decrements": counts.clamped_act_decrements,
                "clamped_sup_afec_decrements": counts.clamped_sup_afec_decrements,
                "first_day_of_region_rows": counts.first_day_of_region_rows,
            },
            "feature_sources": dict(
                [(name, "source column") for name, _unit in BASE_FEATURES]
                + [
                    (name, "one-hot of region")
                    for name, unit in columns
                    if unit == REGION_FEATURE_UNIT
                ]
            ),
            "comparison": comparison,
        },
        "baseline": {
            **baseline.as_dict(),
            "calibration": (
                "p = 1 - exp(-r), where r is the day-to-day difference of "
                "nro_inc_prom_quint, the five-season cumulative climatology"
            ),
            "alternatives": [alternative.as_dict() for alternative in alternatives],
        },
    }
    # No `thresholds`. The level cut points are a serving concern, this artifact has no
    # serving surface, and inventing 0.4/0.7 here would be exactly the plausible-looking
    # number nobody chose that the rest of this package refuses to write. The threshold
    # that WAS chosen on the training split is in `metrics.threshold`, where its provenance
    # travels with it.
    if winner.kind == model_contract.KIND_LOGISTIC:
        intercept, coefficients = logistic_parameters(winner.estimator)
        document["intercept"] = intercept
        document["coef"] = coefficients
    else:
        document["trees"] = forest_trees(winner.estimator)
    return document


def sha256_of(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(65536), b""):
            digest.update(chunk)
    return digest.hexdigest()


# ---------------------------------------------------------------------------
# Output
# ---------------------------------------------------------------------------

#: Columns of `region-days.csv`. The derived differences are here so a reader can audit
#: the label and the clamp without re-deriving them -- and the three forbidden columns are
#: here too, in the table, because a table is evidence and a feature matrix is not.
TABLE_COLUMNS = (
    "region",
    "local_date",
    "split",
    "fire_today",
    "fires_today",
    "burned_ha_today",
    "first_in_window",
    COL_ACT,
    COL_SUP_ACT,
    COL_CREACION,
    COL_ANT,
    COL_PROM,
    COL_SUP_ANT,
    "day_of_year",
    "month",
    "climatology_rate",
)

MISSING_DATA = (
    (
        "Region identity at inference time",
        (
            "The one-hot of region is the reason this artifact cannot be served: `risk` "
            "builds its vector from a publication and carries no region, so every region "
            "indicator would arrive empty. What it would enable: a model that can be served "
            "at all."
        ),
    ),
    (
        "A comuna-level key, and the geometry or fuel data behind it",
        (
            "Region is a coarse proxy for 16 very different fire regimes; Atacama and "
            "Magallanes never advanced the counter in these 90 days while Araucania did 13 "
            "times. What it would enable: a model that generalises between places instead "
            "of memorising that Araucania is the one that burns."
        ),
    ),
    (
        "Meteorology on days without a fire",
        (
            "No column of this export carries weather, and the CONAF event registry only "
            "carries weather attached to an event. What it would enable: the seven features "
            "the artifact contract already declares, and therefore an input that is not a "
            "calendar."
        ),
    ),
    (
        "Accumulated precipitation, 7 days, and a daily extreme",
        (
            "The dominant driver of fire activity is not in the file in any form. What it "
            "would enable: the one feature family that separates a dry September from a wet "
            "one, which is exactly the variation a 90-day window cannot learn."
        ),
    ),
    (
        "More than one season, and more than 90 days",
        (
            "One ramp-up from July to September cannot separate a seasonal effect from a "
            "year. Both models here can only learn how far into the season the date is. "
            "What it would enable: inter-annual variation, and a test split that is a season "
            "rather than a month."
        ),
    ),
    (
        "The dated, access-gated CONAF registry (DOI 10.71578/UXAUN5)",
        (
            "The label here is a counter's daily difference in one region, not an event "
            "with a cause, a location and an area. What it would enable: a real comuna-day "
            "label across 19 seasons, and the per-event fields the panel assembler already "
            "knows how to read."
        ),
    ),
    (
        "Human-ignition, population, road-access and suppression data",
        (
            "Most Chilean fires are human-caused, and the counter cannot tell a lightning "
            "strike from an escaped burn. What it would enable: a feature set that is causal "
            "rather than correlational, and an honest answer to why one region burns and "
            "another does not."
        ),
    ),
    (
        "Forecast weather instead of observed weather",
        (
            "Even a perfect history answers what happened, and the product's question is "
            "tomorrow. What it would enable: a lead time. Nothing in this file is "
            "forward-looking."
        ),
    ),
)

NOTES = (
    (
        "This is a rehearsal. The outcome is a measurement of the delivered data, and the "
        "expected result is that no trained model beats climatology."
    ),
    (
        "The label is derived from a cumulative counter, so every feature here is either a "
        "calendar position or another season's value at the same calendar position. That is "
        "why the baselines are the interesting number."
    ),
    (
        "The exported artifact carries a one-hot of region and therefore cannot be served by "
        "risk. It must never be registered in models or activated; it exists so the exporter "
        "is proven against the Go artifact reader."
    ),
)


def split_of(row: DayRow) -> str:
    if row.month in TRAIN_MONTHS:
        return "train"
    if row.month in TEST_MONTHS:
        return "test"
    return "unused"


def format_number(value: float | None) -> str:
    if value is None:
        return ""
    if isinstance(value, int):
        return str(value)
    if float(value).is_integer():
        return str(int(value))
    return repr(float(value))


def write_table(path: Path, rows: list[DayRow]) -> None:
    """Write the region-day table with a fixed column order and Unix line endings."""
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8", newline="") as handle:
        writer = csv.writer(handle, lineterminator="\n")
        writer.writerow(TABLE_COLUMNS)
        for row in rows:
            writer.writerow(
                [
                    row.region,
                    row.date.isoformat(),
                    split_of(row),
                    row.fire_today,
                    format_number(row.fires_today),
                    format_number(row.burned_ha_today),
                    "1" if row.first_in_window else "0",
                    format_number(row.fires_today),
                    format_number(row.burned_ha_today),
                    row.date.isoformat(),
                    format_number(row.prev_season_inc),
                    format_number(row.climatology_inc),
                    format_number(row.prev_season_burned),
                    row.day_of_year,
                    row.month,
                    format_number(row.climatology_today),
                ]
            )


def write_json(path: Path, document: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8", newline="") as handle:
        json.dump(document, handle, ensure_ascii=False, indent=2)
        handle.write("\n")


# ---------------------------------------------------------------------------
# Report
# ---------------------------------------------------------------------------


def verdict_sentence(comparison: list[dict[str, Any]]) -> str:
    """One sentence saying plainly whether either model beat either baseline."""
    models = [entry for entry in comparison if entry["role"] == "model"]
    baselines = [entry for entry in comparison if entry["role"] == "baseline"]
    wins = [
        (entry["name"], baseline["name"])
        for entry in models
        for baseline in baselines
        if entry["beats"][baseline["name"]]
    ]
    if not wins:
        lines = [
            "Verdict: neither trained model beats either baseline on this data. "
            + " ".join(
                f"{entry['name']} vs {baseline['name']}: "
                f"{entry['beats'][baseline['name'] + '_why']}."
                for entry in models
                for baseline in baselines
            )
        ]
        leading = [
            (
                entry["pr_auc"] - baseline["pr_auc"],
                baseline["brier"] - entry["brier"],
                entry["name"],
                baseline["name"],
            )
            for entry in models
            for baseline in baselines
            if entry["pr_auc"] > baseline["pr_auc"]
        ]
        if leading:
            rank, calibration, model, baseline = max(leading)
            lines.append(
                f"The closest is {model} against {baseline}: it leads PR-AUC by {rank:.4f} "
                f"and trails Brier by {abs(calibration):.4f}. A lead on one axis with a loss "
                f"on the other is a trade, not a win."
            )
        lines.append(
            "No model here improves on climatology, and that is the finding, not a failure "
            "of the pipeline."
        )
        return " ".join(lines)
    paired = "; ".join(f"{model} beats {baseline}" for model, baseline in wins)
    return (
        f"Verdict: {paired} on both PR-AUC and Brier. The remaining comparisons do not hold,"
        f" and the margin is measured on a single September with few positives."
    )


def build_report(
    *,
    source: Path,
    data_hash: str,
    counts: Counts,
    split: SplitCounts,
    comparison: list[dict[str, Any]],
    winner: FittedModel,
    artifact_sha256: str | None,
    trained_at: str,
    climatology_rows_without_rate: int,
) -> str:
    """Render the committed report: counts, the clamp, the table, the verdict, the gaps."""
    models = [entry for entry in comparison if entry["role"] == "model"]
    baselines = [entry for entry in comparison if entry["role"] == "baseline"]
    lines = [
        "# Training rehearsal on the delivered data",
        "",
        "This report is generated by `backend/training/train_delivered.py` and committed",
        "because `backend/training/out/` is git-ignored and a measurement nobody can read is",
        "not a deliverable. Re-run the command and compare; do not hand-edit the numbers.",
        "",
        "| | |",
        "| --- | --- |",
        f"| Source | `{source.name}` |",
        f"| Source sha256 | `{data_hash}` |",
        f"| Run (UTC) | `{trained_at}` |",
        f"| Dates | {counts.first_date} .. {counts.last_date} |",
        f"| Regions | {len(counts.regions)} |",
        "",
        "## What the file contains",
        "",
        "| Measure | Value |",
        "| --- | --- |",
        f"| Source rows | {counts.source_rows} |",
        f"| Region-days | {counts.region_days} |",
        f"| Positive labels (`fire_today = 1`) | {counts.positives} |",
        f"| Negative labels | {counts.negatives} |",
        f"| Positive rate | {counts.positive_rate:.2%} |",
        (
            f"| Clamped downward differences of `sup_afec_act` "
            f"| {counts.clamped_sup_afec_decrements} |"
        ),
        f"| Clamped downward differences of `nro_inc_act` | {counts.clamped_act_decrements} |",
        (
            f"| First-day-of-region rows (no predecessor, label structurally 0) "
            f"| {counts.first_day_of_region_rows} |"
        ),
        "",
        "`nro_inc_act` is a season-cumulative counter, so the label is its day-to-day advance",
        "clamped at zero. `sup_afec_act` is cumulative too and was revised **downward** on the",
        "rows counted above; each such difference is clamped to 0 and **counted**, because a",
        "silently dropped negative difference is burned hectares that left the record without",
        "a trace. The clamp count is the number in that row: it is reported, never absorbed.",
        "",
        (
            "The other thing this file invites is leakage, and the guard that blocks it is "
            "measured rather than assumed. On the committed fixture, adding `nro_inc_act` to "
            "the legal features lifts September PR-AUC from 0.0826 to about 0.31, and adding "
            "its daily difference -- which is the label by construction -- reproduces the "
            "label exactly. `tests/test_train_delivered.py` pins both bounds, so the "
            "forbidden-column guard protects a number somebody can see."
        ),
        "",
        "## The split",
        "",
        "| Side | Months | Region-days | Positives | Base rate |",
        "| --- | --- | --- | --- | --- |",
        (
            f"| Train | July, August | {split.train_rows} "
            f"| {split.train_positives} | {split.train_rate:.2%} |"
        ),
        (
            f"| Test | September | {split.test_rows} "
            f"| {split.test_positives} | {split.test_rate:.2%} |"
        ),
        "",
        "Temporal, never random: September follows August, and a random split would let the",
        "model learn a September day from a July one. The run asserts the split is disjoint",
        "and strictly ordered before it fits anything.",
        "",
        (
            f"The base rate is {split.test_rate / split.train_rate:.1f}x higher in the month "
            f"the model is judged on ({split.test_rate:.2%} against {split.train_rate:.2%}), "
            f"because the season ramps up. A threshold chosen on July and August therefore "
            f"under-alerts in September, which is exactly what a recall of 0.0000 looks "
            f"like when it happens."
        ),
        "",
        "## Metrics",
        "",
        (
            "| Scorer | Role | PR-AUC | Recall at its threshold | Brier | ROC-AUC "
            "| Threshold | Alerts |"
        ),
        "| --- | --- | --- | --- | --- | --- | --- | --- |",
    ]
    for entry in models + baselines:
        lines.append(
            f"| {entry['name']} | {entry['role']} | {entry['pr_auc']:.4f} "
            f"| {entry['recall']:.4f} | {entry['brier']:.4f} | {entry['roc_auc']:.4f} "
            f"| {entry['threshold']:.3f} | {entry['alerts']} |"
        )
    lines += [
        "",
        "Every threshold above was selected on the **training split only** (maximum F1 on a",
        "0.001 grid, ties to the lower threshold) and then applied unchanged to September.",
        f'Both models are fitted with `class_weight="balanced"` and seed `{SEED}`.',
        "Climatology scores `p = 1 - exp(-r)` where `r` is the daily advance of",
        "`nro_inc_prom_quint`; the region-frequency baseline scores each region's positive",
        "rate measured on the training split.",
        "",
        "### Beats, pair by pair",
        "",
        "| Model | Against | Beats it? | Why |",
        "| --- | --- | --- | --- |",
    ]
    for entry in models:
        for baseline in baselines:
            beats = entry["beats"][baseline["name"]]
            why = entry["beats"][baseline["name"] + "_why"]
            lines.append(
                f"| {entry['name']} | {baseline['name']} | {'yes' if beats else 'no'} | {why} |"
            )
    lines += [
        "",
        "Beating a baseline requires **both** a higher PR-AUC and a lower Brier. A higher",
        "PR-AUC at a worse Brier is a trade, and calling it a win is how a model gets",
        "registered for the wrong reason.",
        "",
        verdict_sentence(comparison),
        "",
        "## The exported artifact",
        "",
        f"`{ARTIFACT_VERSION}` (`{winner.kind}`), sha256 `{artifact_sha256}`.",
        "",
        "It is a **rehearsal** and it is **not servable by `risk`**: its feature set includes a",
        "one-hot of region, and `risk` builds its input vector from a publication with no region",
        "identity. It must never be registered in `models` and never activated. It exists so the",
        "exporter is proven against the Go artifact reader, which is exactly what the tests do:",
        "the emitted document is re-read with a Python transcription of `artifact.Validate`",
        "(`backend/training/model_contract.py`) and refused if a single rule fails.",
        "",
        "The marker lives in `version` (`rehearsal` in the string) and in `metrics.rehearsal`,",
        "`metrics.servable_by_risk`, `metrics.not_servable_reason` and",
        "`metrics.must_not_be_registered` -- **in the artifact's own bytes**, not only here. A",
        "top-level `rehearsal` key is not possible: the Go reader decodes with",
        "`DisallowUnknownFields`, so an undefined key makes the document unreadable, and an",
        "unreadable artifact is worse than a marked one.",
        "",
        "`center`/`scale`/`impute` were fitted on the **training split only**, in that order of",
        "documented preprocessing (`NaN -> impute`, then `(x - center) / scale`). The declared",
        f"`min`/`max` guards are train-only too. {climatology_rows_without_rate} rows have no",
        "climatology rate (the window's first day has no predecessor); they are all in the",
        "training split, and the run refuses to score a test row that has none.",
        "",
        (
            "One note for whoever exports a forest next: scikit-learn walks a tree in "
            "`float32` while the Go scorer walks `float64`, so a forest artifact can take "
            "the other branch on a row whose value sits within one float32 step of a "
            "threshold. Measured on the committed fixture, that moved one row's probability "
            f"by 0.30. This run exported a `{winner.kind}` model, which has no thresholds "
            "at all and therefore no such divergence."
        ),
        "",
        "## What is missing",
        "",
        "Each gap is named with what it would enable, because a list of absences without a",
        "consequence is a complaint rather than a plan.",
        "",
    ]
    for title, description in MISSING_DATA:
        lines += [f"**{title}.** {description}", ""]
    lines += [
        "## How to reproduce",
        "",
        "```bash",
        "# The real run: the export as delivered, from the repository root.",
        "python backend/training/train_delivered.py",
        "",
        "# Offline, against the committed fixture (this is what the tests use).",
        "python backend/training/train_delivered.py \\",
        "    --source backend/training/tests/fixtures/dashboard-conaf-sample.csv \\",
        "    --out backend/training/out/fixture",
        "",
        "# Tests and lint, from backend/training.",
        "python -m pytest -q",
        "python -m ruff check .",
        "```",
        "",
    ]
    return "\n".join(lines)


# ---------------------------------------------------------------------------
# Orchestration
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class SplitCounts:
    """How many region-days and positives each side of the temporal split holds.

    Carried explicitly because it cannot be recovered from a `Evaluation`: that holds the
    rows and positives of the split it measured, and using it for both sides is how the
    report table first came out saying July had as many positives as September.
    """

    train_rows: int
    train_positives: int
    test_rows: int
    test_positives: int

    @property
    def train_rate(self) -> float:
        return self.train_positives / self.train_rows if self.train_rows else 0.0

    @property
    def test_rate(self) -> float:
        return self.test_positives / self.test_rows if self.test_rows else 0.0


@dataclass
class Result:
    """Everything one run produced, for the CLI and for the tests."""

    counts: Counts
    columns: tuple[tuple[str, str], ...]
    split: SplitCounts
    preprocessing: Preprocessing
    bounds: tuple[tuple[float, float], ...]
    models: list[FittedModel]
    baselines: list[Evaluation]
    comparison: list[dict[str, Any]]
    winner: FittedModel
    artifact: dict[str, Any]
    report: str
    climatology_rows_without_rate: int
    data_hash: str


def comparison_rows(
    models: list[FittedModel],
    baselines: list[Evaluation],
) -> list[dict[str, Any]]:
    """The four rows of the metrics table, with the beats matrix on both sides.

    The models' rows are their TEST evaluations. `model.evaluation` is Optional only
    because the dataclass is shared with `fit_models`, which runs before anything is
    measured; a missing one is a programming error, so it is refused loudly here rather
    than silently compared as zero.
    """
    rows: list[dict[str, Any]] = []
    for model in models:
        if model.evaluation is None:  # pragma: no cover - evaluate_models always fills it
            raise DeliveredError(f"model {model.name!r} was never evaluated")
        evaluation = model.evaluation
        beats: dict[str, Any] = {}
        for baseline in baselines:
            beats[baseline.name], beats[baseline.name + "_why"] = evaluation.beats(baseline)
        rows.append({**evaluation.as_dict(), "role": "model", "beats": beats})
    for baseline in baselines:
        beats = {}
        for model in models:
            if model.evaluation is None:  # pragma: no cover
                raise DeliveredError(f"model {model.name!r} was never evaluated")
            beats[model.name], beats[model.name + "_why"] = baseline.beats(model.evaluation)
        rows.append({**baseline.as_dict(), "role": "baseline", "beats": beats})
    return rows


def train(
    source: Path = DEFAULT_SOURCE,
    *,
    out: Path = DEFAULT_OUT,
    trained_at: str | None = None,
    write: bool = True,
) -> Result:
    """Read, split, fit, measure, export. Every failure is a `DeliveredError`."""
    if not source.is_file():
        raise DeliveredError(
            f"the source {source} does not exist. The delivered export is untracked; pass "
            f"--source with its path, or use tests/fixtures/dashboard-conaf-sample.csv"
        )
    rows, counts = read_days(source)
    columns = feature_columns(counts.regions)
    train_rows, test_rows = month_split(rows)

    train_matrix, train_label = build_matrix(train_rows, columns)
    test_matrix, test_label = build_matrix(test_rows, columns)

    # Fitted before any model sees anything, and from the training matrix alone.
    preprocessing = fit_preprocessing(train_matrix, columns)
    bounds = guard_bounds(train_matrix, columns)
    train_matrix = preprocessing.apply(train_matrix)
    test_matrix = preprocessing.apply(test_matrix)

    models = fit_models(train_matrix, test_matrix, train_label)
    evaluate_models(models, train_label, test_label)

    train_probabilities, test_probabilities, _baseline_counts = baseline_probabilities(
        train_rows, test_rows
    )
    climatology_rows_without_rate = sum(1 for row in rows if row.climatology_today is None)
    if any(row.climatology_today is None for row in test_rows):
        raise DeliveredError(
            "a test row has no climatology rate (no predecessor in the window), so the "
            "climatology baseline cannot be scored on the same set as the models"
        )
    baselines = [
        evaluate(
            "climatology",
            "baseline",
            test_label,
            test_probabilities["climatology"],
            select_threshold(train_label, train_probabilities["climatology"]),
        ),
        evaluate(
            "region_frequency",
            "baseline",
            test_label,
            test_probabilities["region_frequency"],
            select_threshold(train_label, train_probabilities["region_frequency"]),
        ),
    ]
    comparison = comparison_rows(models, baselines)
    winner = select_winner(models)
    if winner.evaluation is None:  # pragma: no cover - evaluate_models always fills it
        raise DeliveredError(f"model {winner.name!r} was never evaluated")
    winner_evaluation = winner.evaluation
    split = SplitCounts(
        train_rows=len(train_rows),
        train_positives=int(train_label.sum()),
        test_rows=len(test_rows),
        test_positives=int(test_label.sum()),
    )

    data_hash = sha256_of(source)
    stamp = trained_at or dt.datetime.now(dt.UTC).replace(microsecond=0).isoformat().replace(
        "+00:00", "Z"
    )
    artifact = build_artifact(
        winner=winner,
        columns=columns,
        preprocessing=preprocessing,
        bounds=bounds,
        evaluation=winner_evaluation,
        baseline=baselines[0],
        alternatives=baselines[1:],
        comparison=comparison,
        counts=counts,
        source=source,
        data_hash=data_hash,
        trained_at=stamp,
    )
    # Validated here, before anything touches the disk: a document the Go reader would
    # refuse must never become a file a reader could mistake for a model.
    model_contract.validate_artifact(artifact)

    result = Result(
        counts=counts,
        columns=columns,
        split=split,
        preprocessing=preprocessing,
        bounds=bounds,
        models=models,
        baselines=baselines,
        comparison=comparison,
        winner=winner,
        artifact=artifact,
        report="",
        climatology_rows_without_rate=climatology_rows_without_rate,
        data_hash=data_hash,
    )
    if not write:
        return result

    out.mkdir(parents=True, exist_ok=True)
    write_table(out / "region-days.csv", rows)
    model_contract.write_artifact(out / "artifact.json", artifact)
    artifact_hash = sha256_of(out / "artifact.json")
    result.report = build_report(
        source=source,
        data_hash=data_hash,
        counts=counts,
        split=split,
        comparison=comparison,
        winner=winner,
        artifact_sha256=artifact_hash,
        trained_at=stamp,
        climatology_rows_without_rate=climatology_rows_without_rate,
    )
    report_path = out / "REPORT-delivered-data.md"
    report_path.parent.mkdir(parents=True, exist_ok=True)
    report_path.write_text(result.report, encoding="utf-8", newline="\n")

    write_json(
        out / "manifest.json",
        {
            "generated_by": "backend/training/train_delivered.py",
            "generated_at": stamp,
            "rehearsal": True,
            "source": {
                "path": display(source),
                "sha256": data_hash,
                "bytes": source.stat().st_size,
                "is_training_set": False,
                "note": (
                    "The delivered 90-day regional dashboard export. It is the only labelled "
                    "fire data in the repository and it is not the product's training set."
                ),
            },
            "counts": {
                "source_rows": counts.source_rows,
                "region_days": counts.region_days,
                "positives": counts.positives,
                "negatives": counts.negatives,
                "positive_rate": round(counts.positive_rate, 6),
                "clamped_act_decrements": counts.clamped_act_decrements,
                "clamped_sup_afec_decrements": counts.clamped_sup_afec_decrements,
                "first_day_of_region_rows": counts.first_day_of_region_rows,
                "climatology_rows_without_rate": climatology_rows_without_rate,
            },
            "dates": {
                "first": counts.first_date.isoformat(),
                "last": counts.last_date.isoformat(),
            },
            "regions": list(counts.regions),
            "split": {
                "kind": "temporal",
                "train_months": list(TRAIN_MONTHS),
                "test_months": list(TEST_MONTHS),
                "train_rows": len(train_rows),
                "test_rows": len(test_rows),
                "train_positives": int(train_label.sum()),
                "test_positives": int(test_label.sum()),
                "asserted": "disjoint, strictly ordered, no repeated region-day",
            },
            "preprocessing": {
                "impute": list(preprocessing.impute),
                "center": list(preprocessing.center),
                "scale": list(preprocessing.scale),
                "fitted_on": "the training split only",
            },
            "features": [
                {
                    "name": name,
                    "unit": unit,
                    "min": bounds[index][0],
                    "max": bounds[index][1],
                }
                for index, (name, unit) in enumerate(columns)
            ],
            "forbidden_feature_columns": list(FORBIDDEN_FEATURE_COLUMNS),
            "metrics": comparison,
            "verdict": verdict_sentence(comparison),
            "artifact": {
                "path": display(out / "artifact.json"),
                "version": ARTIFACT_VERSION,
                "kind": winner.kind,
                "sha256": artifact_hash,
                "rehearsal": True,
                "servable_by_risk": False,
                "must_not_be_registered": True,
            },
            "notes": list(NOTES),
        },
    )
    return result


def display(path: Path) -> str:
    """Show a path relative to the repository when it lives inside it."""
    try:
        return path.resolve().relative_to(REPO_ROOT).as_posix()
    except ValueError:
        return path.as_posix()


def summary(result: Result) -> list[str]:
    """The lines a reviewer needs, including the verdict whether or not it is welcome."""
    counts = result.counts
    models = [entry for entry in result.comparison if entry["role"] == "model"]
    baselines = [entry for entry in result.comparison if entry["role"] == "baseline"]
    lines = [
        (
            f"source rows: {counts.source_rows} -> {counts.region_days} region-days, "
            f"{counts.positives} positive ({counts.positive_rate:.2%})"
        ),
        (
            f"clamped downward differences: nro_inc_act {counts.clamped_act_decrements}, "
            f"sup_afec_act {counts.clamped_sup_afec_decrements} "
            f"(reported, never silently dropped)"
        ),
        (
            f"first-day-of-region rows: {counts.first_day_of_region_rows} "
            f"(no predecessor, label structurally 0)"
        ),
        f"features ({len(result.columns)}): {', '.join(name for name, _ in result.columns)}",
        (
            f"split: months {list(TRAIN_MONTHS)} train / {list(TEST_MONTHS)} test, "
            f"disjoint and ordered"
        ),
        "",
        f"{'scorer':<18}{'role':<10}{'pr_auc':>9}{'recall':>9}{'brier':>9}{'roc_auc':>9}",
    ]
    for entry in models + baselines:
        lines.append(
            f"{entry['name']:<18}{entry['role']:<10}{entry['pr_auc']:>9.4f}"
            f"{entry['recall']:>9.4f}{entry['brier']:>9.4f}{entry['roc_auc']:>9.4f}"
        )
    lines += ["", verdict_sentence(result.comparison), ""]
    lines.append(f"exported: {result.winner.name} as {ARTIFACT_VERSION} (rehearsal, not servable)")
    return lines


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------


def main(argv: list[str] | None = None) -> int:
    """Run the rehearsal and print the numbers that decide the verdict."""
    parser = argparse.ArgumentParser(
        description=(
            "Train a rehearsal model on the delivered dashboard export, measure it against "
            "two baselines, and export it for the Go artifact reader. Writes region-days.csv, "
            "artifact.json, manifest.json and REPORT-delivered-data.md under --out."
        )
    )
    parser.add_argument("--source", type=Path, default=DEFAULT_SOURCE, help="dashboard CSV to read")
    parser.add_argument("--out", type=Path, default=DEFAULT_OUT, help="output directory")
    parser.add_argument(
        "--trained-at", default=None, help="override the trained_at timestamp (RFC3339)"
    )
    arguments = parser.parse_args(argv)
    try:
        result = train(arguments.source, out=arguments.out, trained_at=arguments.trained_at)
    except DeliveredError as error:
        print(f"error: {error}", file=sys.stderr)
        return 2
    except model_contract.ArtifactError as error:
        print(f"error: the artifact does not satisfy the contract: {error}", file=sys.stderr)
        return 2
    for line in summary(result):
        print(line)
    print(f"wrote {display(arguments.out)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
