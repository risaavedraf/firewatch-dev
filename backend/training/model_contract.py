"""The `firewatch-model/v1` contract, re-implemented on the training side.

This module is a **reader and a checker**, not a second definition. It exists so the
exporter in `train_delivered.py` cannot certify its own homework: the writer builds a
document, and this module refuses it with the same rules
`backend/services/inference/internal/artifact/artifact.go` applies at startup. Without
it, "the artifact loads" would mean "the JSON parses", and the first real failure would
be a Go service that refuses to start on a document nobody checked.

WHAT IS REPLICATED, RULE FOR RULE
---------------------------------
* `Parse` decodes with `DisallowUnknownFields`, so **an unknown key is an error at
  every level that Go types**: the top level, each entry of `features`, each entry of
  `trees[].nodes`, and `thresholds`. `metrics` and `baseline` are `json.RawMessage`
  there and stay opaque here too -- they are only required to be JSON objects.
* Go's `*int` and `*float64` fields are pointers, so a `null` or a missing key is a
  distinct state from `0`. `Node.feature == nil` is a leaf test, not a zero test, and a
  node that carries `value` and `feature` at once is rejected.
* Every check `Validate` performs: format/version/trained_at/data_hash, feature names
  and units, `min < max`, positional arrays of the feature width, `scale > 0`,
  `impute` inside its feature's guard, `0 < medium < high < 1`, and the per-kind model
  rules -- including `validateTree`, which proves a tree terminates and is fully
  reachable, because a cycle there is a hang at scoring time rather than an error a
  caller can see.
* `ensureEOF`: trailing content after the document is a refusal, not a second read.

WHAT IS DELIBERATELY NOT REPLICATED
-----------------------------------
The scorer. This module never computes a probability, so it cannot agree with the Go
scorer by sharing a mistake with it; `backend/services/inference/tools/reference_scorer.py`
is the independent scorer, and the Go equivalence test is the one that pins the numbers.
The min/max guards are checked here as declarations, not applied to values: Go applies
them to a request, and this artifact has no request surface.
"""

from __future__ import annotations

import json
import math
import re
from pathlib import Path

#: The only format this reader accepts, spelled exactly as `artifact.Format`.
FORMAT = "firewatch-model/v1"

#: The two model kinds the format defines.
KIND_LOGISTIC = "logistic"
KIND_FOREST = "forest"

#: Every top-level key Go can decode. Anything else is rejected by
#: `DisallowUnknownFields`, so this tuple is the artifact's whole allowed vocabulary.
ARTIFACT_KEYS = (
    "format",
    "version",
    "trained_at",
    "data_hash",
    "features",
    "impute",
    "center",
    "scale",
    "kind",
    "intercept",
    "coef",
    "trees",
    "metrics",
    "baseline",
    "thresholds",
)

#: Keys of one `features[]` entry (`artifact.Feature`).
FEATURE_KEYS = ("name", "unit", "min", "max")

#: Keys of one `trees[].nodes[]` entry (`artifact.Node`). All five are optional; a leaf
#: carries `value` and nothing else, every other node carries the four split fields.
NODE_KEYS = ("feature", "threshold", "left", "right", "value")

#: Keys of `thresholds` (`artifact.Thresholds`).
THRESHOLD_KEYS = ("medium", "high")

#: `time.RFC3339` as Go parses it: seconds are mandatory, a fractional part and either
#: `Z` or a numeric offset are optional. `2026-09-28 16:23:00` is not RFC3339 and Go
#: rejects it, so this reader does too.
RFC3339 = re.compile(r"^\d{4}-\d{2}-\d{2}[Tt]\d{2}:\d{2}:\d{2}(\.\d+)?([Zz]|[+-]\d{2}:\d{2})$")


class ArtifactError(Exception):
    """A document the Go service would refuse; the CLI prints it without a traceback."""


# ---------------------------------------------------------------------------
# Scalar checks
# ---------------------------------------------------------------------------


def finite(value: float) -> bool:
    """Whether `value` is a real number. NaN and both infinities are not.

    Mirrors `artifact.finite`: a NaN reaching a score is a defect a caller must see.
    """
    return isinstance(value, (int, float)) and not isinstance(value, bool) and math.isfinite(value)


def _object(document: object, where: str) -> dict:
    if not isinstance(document, dict):
        raise ArtifactError(f"{where} must be a JSON object, got {type(document).__name__}")
    return document


def _number(document: dict, key: str, where: str) -> float:
    value = document.get(key)
    if value is None:
        raise ArtifactError(f"{where}.{key} is required and must be a number")
    if not isinstance(value, (int, float)) or isinstance(value, bool):
        raise ArtifactError(f"{where}.{key} must be a number, got {type(value).__name__}")
    if not finite(value):
        raise ArtifactError(f"{where}.{key} is not finite")
    return float(value)


def _integer(document: dict, key: str, where: str) -> int:
    """One `*int` field. A float with a fractional part is not an int to Go either."""
    value = document.get(key)
    if value is None:
        raise ArtifactError(f"{where}.{key} is required and must be an integer")
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise ArtifactError(f"{where}.{key} must be an integer, got {type(value).__name__}")
    if isinstance(value, float) and not value.is_integer():
        raise ArtifactError(f"{where}.{key} is {value}: an integer field cannot carry a fraction")
    return int(value)


def _string(document: dict, key: str, where: str) -> str:
    value = document.get(key)
    if value is None:
        raise ArtifactError(f"{where}.{key} is required and must be a string")
    if not isinstance(value, str):
        raise ArtifactError(f"{where}.{key} must be a string, got {type(value).__name__}")
    return value


def _unknown_keys(document: dict, allowed: tuple[str, ...], where: str) -> None:
    unexpected = sorted(set(document) - set(allowed))
    if unexpected:
        raise ArtifactError(
            f"{where} carries {', '.join(unexpected)}, which the format does not define "
            f"(the Go reader decodes with DisallowUnknownFields)"
        )


def _array(document: dict, key: str, where: str) -> list:
    value = document.get(key)
    if value is None:
        raise ArtifactError(f"{where}.{key} is required and must be an array")
    if not isinstance(value, list):
        raise ArtifactError(f"{where}.{key} must be an array, got {type(value).__name__}")
    return value


def _float_array(document: dict, key: str, where: str) -> list[float]:
    return [_number({key: item}, key, where) for item in _array(document, key, where)]


# ---------------------------------------------------------------------------
# Validation, section by section (the order `Validate` uses)
# ---------------------------------------------------------------------------


def _validate_header(document: dict) -> None:
    if document.get("format") != FORMAT:
        raise ArtifactError(
            f"format {document.get('format')!r} is not {FORMAT!r}: a document written for "
            f"another reader is not interpreted with the assumptions of this one"
        )
    if _string(document, "version", "artifact") == "":
        raise ArtifactError("version is required")
    trained_at = _string(document, "trained_at", "artifact")
    if not RFC3339.match(trained_at):
        raise ArtifactError(f"trained_at {trained_at!r} is not an RFC3339 instant")
    if _string(document, "data_hash", "artifact") == "":
        raise ArtifactError("data_hash is required")


def _validate_features(document: dict) -> int:
    """Check `features` and return the model's input width."""
    features = _array(document, "features", "artifact")
    if not features:
        raise ArtifactError("features is empty: the model declares no input")
    declared: set[str] = set()
    for index, raw in enumerate(features):
        where = f"features[{index}]"
        feature = _object(raw, where)
        _unknown_keys(feature, FEATURE_KEYS, where)
        name = _string(feature, "name", where)
        if name == "":
            raise ArtifactError(f"{where} has no name")
        if name in declared:
            raise ArtifactError(f"feature {name!r} is declared twice")
        declared.add(name)
        unit = _string(feature, "unit", where)
        if unit == "":
            raise ArtifactError(
                f"feature {name!r} has no unit: a guard without a unit cannot be checked"
            )
        minimum = _number(feature, "min", where)
        maximum = _number(feature, "max", where)
        if minimum >= maximum:
            raise ArtifactError(
                f"feature {name!r} has min {minimum} and max {maximum}: min must be below max"
            )
    return len(features)


def _validate_preprocessing(document: dict, width: int, features: list[dict]) -> None:
    arrays = {}
    for name in ("impute", "center", "scale"):
        values = _float_array(document, name, "artifact")
        if len(values) != width:
            raise ArtifactError(
                f"{name} has {len(values)} values for {width} features: preprocessing must "
                f"be positional and complete"
            )
        arrays[name] = values
    for index, scale in enumerate(arrays["scale"]):
        if scale <= 0:
            raise ArtifactError(
                f"scale[{index}] ({features[index]['name']}) is {scale}: a scale must be positive"
            )
    # The imputed value stands in for a missing observation, so the guard that governs
    # every supplied value governs it too.
    for index, value in enumerate(arrays["impute"]):
        minimum = float(features[index]["min"])
        maximum = float(features[index]["max"])
        if value < minimum or value > maximum:
            raise ArtifactError(
                f"impute[{index}] ({features[index]['name']}) is {value}, outside "
                f"[{minimum}, {maximum}]"
            )


def _validate_thresholds(document: dict) -> None:
    raw = document.get("thresholds")
    if raw is None:
        return
    where = "thresholds"
    thresholds = _object(raw, where)
    _unknown_keys(thresholds, THRESHOLD_KEYS, where)
    medium = _number(thresholds, "medium", where)
    high = _number(thresholds, "high", where)
    if not (0 < medium < high < 1):
        raise ArtifactError(
            f"thresholds medium {medium} and high {high} must satisfy 0 < medium < high < 1"
        )


def _validate_provenance(document: dict) -> None:
    # metrics and baseline are opaque objects: carried so the numbers a model was
    # chosen on stay attached to the parameters that produce it, and never read while
    # scoring. Go only requires the first byte to be `{`.
    for name in ("metrics", "baseline"):
        if name not in document or document[name] is None:
            continue
        if not isinstance(document[name], dict):
            raise ArtifactError(f"{name} must be a JSON object")


def _validate_tree(index: int, raw: object, width: int) -> None:
    where = f"trees[{index}]"
    tree = _object(raw, where)
    _unknown_keys(tree, ("nodes",), where)
    nodes = _array(tree, "nodes", where)
    if not nodes:
        raise ArtifactError(f"{where} has no nodes")
    for position, raw_node in enumerate(nodes):
        node_where = f"{where}.nodes[{position}]"
        node = _object(raw_node, node_where)
        _unknown_keys(node, NODE_KEYS, node_where)
        if node.get("value") is not None:
            value = _number(node, "value", node_where)
            if value < 0 or value > 1:
                raise ArtifactError(
                    f"{node_where} leaf value {value} is not a probability in [0, 1]"
                )
            for split_key in ("feature", "threshold", "left", "right"):
                if node.get(split_key) is not None:
                    raise ArtifactError(f"{node_where} is a leaf and also carries {split_key}")
            continue
        if node.get("feature") is None or node.get("threshold") is None:
            raise ArtifactError(f"{node_where} is not a leaf and is missing feature or threshold")
        feature = _integer(node, "feature", node_where)
        if feature < 0 or feature >= width:
            raise ArtifactError(f"{node_where} feature index {feature} is outside [0, {width})")
        _number(node, "threshold", node_where)
        for child_name in ("left", "right"):
            if node.get(child_name) is None:
                raise ArtifactError(f"{node_where} is not a leaf and is missing {child_name}")
            child = _integer(node, child_name, node_where)
            if child < 0 or child >= len(nodes):
                raise ArtifactError(
                    f"{node_where} {child_name} index {child} is outside [0, {len(nodes)})"
                )
            if child == position:
                raise ArtifactError(f"{node_where} {child_name} points at itself")

    # Proven walkable: no cycle, nothing unreachable, and a depth bound. A cycle here
    # is an infinite loop in the Go scorer, so it is refused as a structure defect, and
    # the recursion is depth-limited so a hostile document cannot exhaust the Python
    # stack before the check reports anything.
    state = [0] * len(nodes)  # 0 unvisited, 1 on path, 2 done

    def walk(index: int, depth: int) -> None:
        if depth > len(nodes):
            raise ArtifactError(
                f"{where} is deeper than its {len(nodes)} nodes: the tree does not terminate"
            )
        if state[index] == 1:
            raise ArtifactError(
                f"{where}.nodes[{index}] is reached twice on one path: the tree has a cycle"
            )
        if state[index] == 2:
            return
        state[index] = 1
        node = nodes[index]
        if node.get("value") is None:
            walk(int(node["left"]), depth + 1)
            walk(int(node["right"]), depth + 1)
        state[index] = 2

    walk(0, 0)
    for position, value in enumerate(state):
        if value != 2:
            raise ArtifactError(f"{where}.nodes[{position}] is unreachable from the root")


def _validate_model(document: dict, width: int) -> None:
    kind = document.get("kind")
    if kind is None or kind == "":
        raise ArtifactError("kind is required")
    if kind not in (KIND_LOGISTIC, KIND_FOREST):
        raise ArtifactError(f"kind {kind!r} is neither {KIND_LOGISTIC!r} nor {KIND_FOREST!r}")
    trees = document.get("trees")
    intercept = document.get("intercept")
    coef = document.get("coef")
    if kind == KIND_LOGISTIC:
        if trees is not None and len(trees) != 0:
            raise ArtifactError("kind logistic carries trees: one artifact must describe one model")
        if intercept is None:
            raise ArtifactError("kind logistic requires intercept")
        _number(document, "intercept", "artifact")
        coefficients = _float_array(document, "coef", "artifact")
        if len(coefficients) != width:
            raise ArtifactError(f"coef has {len(coefficients)} values for {width} features")
        return
    if intercept is not None or (coef is not None and len(coef) != 0):
        raise ArtifactError(
            "kind forest carries intercept or coef: one artifact must describe one model"
        )
    if trees is None or len(trees) == 0:
        raise ArtifactError("kind forest requires at least one tree")
    for index, tree in enumerate(trees):
        _validate_tree(index, tree, width)


def validate_artifact(document: object) -> dict:
    """Validate one parsed artifact and return it unchanged.

    Every failure is an `ArtifactError` naming the field, which is what the Go reader
    does and what makes a rejected export readable instead of a stack trace.
    """
    root = _object(document, "artifact")
    _unknown_keys(root, ARTIFACT_KEYS, "artifact")
    _validate_header(root)
    width = _validate_features(root)
    features = root["features"]
    _validate_preprocessing(root, width, features)
    _validate_thresholds(root)
    _validate_provenance(root)
    _validate_model(root, width)
    return root


def parse_artifact(text: str) -> dict:
    """Decode and validate one artifact document from text."""
    decoder = json.JSONDecoder()
    stripped = text.lstrip()
    try:
        document, end = decoder.raw_decode(stripped)
    except json.JSONDecodeError as error:
        raise ArtifactError(f"decode artifact: {error}") from error
    if stripped[end:].strip():
        raise ArtifactError("artifact contains more than one JSON document")
    return validate_artifact(document)


def load_artifact(path: Path) -> dict:
    """Read one artifact from disk and validate it. Every failure is fatal."""
    try:
        text = path.read_text(encoding="utf-8")
    except OSError as error:
        raise ArtifactError(f"read model artifact {path}: {error}") from error
    try:
        return parse_artifact(text)
    except ArtifactError as error:
        raise ArtifactError(f"model artifact {path}: {error}") from error


def write_artifact(path: Path, document: dict) -> None:
    """Validate `document` and only then write it.

    The order is the point: a writer that wrote first and validated second would leave
    an unreadable artifact on disk whenever a check failed, and the artifact's whole
    contract is that the file either loads or does not exist.
    """
    validate_artifact(document)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(document, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
