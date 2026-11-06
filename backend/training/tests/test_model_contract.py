"""The Python transcription of `artifact.Validate` has to bite.

`backend/training/model_contract.py` exists so the exporter cannot certify its own
homework, which means the checker itself has to be checked: every rule below is one of
the rules `backend/services/inference/internal/artifact/artifact.go` enforces at startup,
and each test proves the transcription refuses a document the Go reader would refuse.
A checker that passes everything is worse than no checker, because it looks like
evidence.
"""

from __future__ import annotations

import copy
import json

import pytest

import model_contract


def valid_logistic() -> dict:
    """The smallest document that satisfies every rule, used as the mutation base."""
    return {
        "format": "firewatch-model/v1",
        "version": "unit-test-v1",
        "trained_at": "2026-11-06T00:00:00Z",
        "data_hash": "0" * 64,
        "features": [
            {"name": "day_of_year", "unit": "day", "min": 1, "max": 366},
            {"name": "month", "unit": "month", "min": 1, "max": 12},
        ],
        "impute": [180.0, 7.0],
        "center": [183.0, 7.5],
        "scale": [40.0, 1.5],
        "kind": "logistic",
        "intercept": -2.5,
        "coef": [0.1, -0.2],
        "metrics": {"rows": 10},
        "baseline": {"kind": "climatology"},
    }


def valid_forest() -> dict:
    document = valid_logistic()
    del document["intercept"]
    del document["coef"]
    document["kind"] = "forest"
    document["trees"] = [
        {
            "nodes": [
                {"feature": 0, "threshold": 0.5, "left": 1, "right": 2},
                {"value": 0.1},
                {"value": 0.9},
            ]
        }
    ]
    return document


# ---------------------------------------------------------------------------
# What must be accepted
# ---------------------------------------------------------------------------


def test_the_minimal_logistic_document_is_accepted():
    assert model_contract.validate_artifact(valid_logistic()) == valid_logistic()


def test_the_minimal_forest_document_is_accepted():
    assert model_contract.validate_artifact(valid_forest())["kind"] == "forest"


def test_the_optional_blocks_may_be_absent():
    document = valid_logistic()
    for key in ("metrics", "baseline", "thresholds"):
        document.pop(key, None)
    model_contract.validate_artifact(document)


def test_thresholds_inside_the_contract_are_accepted():
    document = valid_logistic()
    document["thresholds"] = {"medium": 0.4, "high": 0.7}
    model_contract.validate_artifact(document)


def test_the_three_arrays_are_positional_and_complete():
    document = valid_logistic()
    for key in ("impute", "center", "scale"):
        document[key] = [0.0] * 5
    with pytest.raises(model_contract.ArtifactError, match="positional and complete"):
        model_contract.validate_artifact(document)


# ---------------------------------------------------------------------------
# Header
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    ("field", "value", "message"),
    [
        ("format", "firewatch-model/v2", "is not"),
        ("version", "", "version is required"),
        ("data_hash", "", "data_hash is required"),
    ],
)
def test_a_broken_header_is_refused(field, value, message):
    document = valid_logistic()
    document[field] = value
    with pytest.raises(model_contract.ArtifactError, match=message):
        model_contract.validate_artifact(document)


@pytest.mark.parametrize(
    "trained_at",
    ["2026-11-06", "2026-11-06 00:00:00", "06/11/2026T00:00:00Z", "2026-11-06T00:00Z", "yesterday"],
)
def test_trained_at_must_be_rfc3339(trained_at):
    document = valid_logistic()
    document["trained_at"] = trained_at
    with pytest.raises(model_contract.ArtifactError, match="RFC3339"):
        model_contract.validate_artifact(document)


def test_trained_at_accepts_an_offset_and_a_fraction():
    document = valid_logistic()
    document["trained_at"] = "2026-11-06T03:00:00.250-03:00"
    model_contract.validate_artifact(document)


# ---------------------------------------------------------------------------
# Unknown fields: the reader decodes with DisallowUnknownFields
# ---------------------------------------------------------------------------


def test_an_unknown_top_level_key_is_refused():
    document = valid_logistic()
    document["rehearsal"] = True
    with pytest.raises(model_contract.ArtifactError, match="does not define"):
        model_contract.validate_artifact(document)


def test_an_unknown_key_inside_a_feature_is_refused():
    document = valid_logistic()
    document["features"][0]["scale"] = 1.0
    with pytest.raises(model_contract.ArtifactError, match=r"features\[0\].*does not define"):
        model_contract.validate_artifact(document)


def test_an_unknown_key_inside_a_node_is_refused():
    document = valid_forest()
    document["trees"][0]["nodes"][0]["weight"] = 1.0
    with pytest.raises(model_contract.ArtifactError, match="does not define"):
        model_contract.validate_artifact(document)


def test_an_unknown_key_inside_thresholds_is_refused():
    document = valid_logistic()
    document["thresholds"] = {"medium": 0.4, "high": 0.7, "critical": 0.9}
    with pytest.raises(model_contract.ArtifactError, match="does not define"):
        model_contract.validate_artifact(document)


# ---------------------------------------------------------------------------
# Features
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    ("mutate", "message"),
    [
        (lambda document: document.update(features=[]), "features is empty"),
        (lambda document: document["features"][0].update(name=""), "has no name"),
        (lambda document: document["features"][1].update(name="day_of_year"), "declared twice"),
        (lambda document: document["features"][0].update(unit=""), "has no unit"),
        (lambda document: document["features"][0].update(min=366, max=1), "min must be below"),
        (lambda document: document["features"][0].update(min=5, max=5), "min must be below"),
        (lambda document: document["features"][0].update(max="366"), "must be a number"),
    ],
)
def test_a_broken_feature_is_refused(mutate, message):
    document = valid_logistic()
    mutate(document)
    with pytest.raises(model_contract.ArtifactError, match=message):
        model_contract.validate_artifact(document)


# ---------------------------------------------------------------------------
# Preprocessing
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    ("mutate", "message"),
    [
        (lambda document: document.update(center=[0.0]), "positional and complete"),
        (lambda document: document.update(scale=[0.0, 1.0]), "a scale must be positive"),
        (lambda document: document.update(scale=[40.0, -1.0]), "a scale must be positive"),
        (lambda document: document.update(impute=[180.0, 99.0]), "outside"),
        (lambda document: document.update(center=[0.0, None]), "required"),
    ],
)
def test_broken_preprocessing_is_refused(mutate, message):
    document = valid_logistic()
    mutate(document)
    with pytest.raises(model_contract.ArtifactError, match=message):
        model_contract.validate_artifact(document)


def test_a_non_finite_value_is_refused():
    document = valid_logistic()
    document["coef"] = [float("nan"), -0.2]
    with pytest.raises(model_contract.ArtifactError, match="not finite"):
        model_contract.validate_artifact(document)


# ---------------------------------------------------------------------------
# Thresholds and provenance
# ---------------------------------------------------------------------------


@pytest.mark.parametrize("thresholds", [{"medium": 0.7, "high": 0.4}, {"medium": 0, "high": 1}])
def test_incoherent_thresholds_are_refused(thresholds):
    document = valid_logistic()
    document["thresholds"] = thresholds
    with pytest.raises(model_contract.ArtifactError, match="0 < medium < high < 1"):
        model_contract.validate_artifact(document)


@pytest.mark.parametrize("block", ["metrics", "baseline"])
def test_a_provenance_block_that_is_not_an_object_is_refused(block):
    document = valid_logistic()
    document[block] = [1, 2, 3]
    with pytest.raises(model_contract.ArtifactError, match="must be a JSON object"):
        model_contract.validate_artifact(document)


# ---------------------------------------------------------------------------
# One artifact, one model
# ---------------------------------------------------------------------------


def test_a_logistic_that_carries_trees_is_refused():
    document = valid_logistic()
    document["trees"] = valid_forest()["trees"]
    with pytest.raises(model_contract.ArtifactError, match="carries trees"):
        model_contract.validate_artifact(document)


def test_a_logistic_without_an_intercept_is_refused():
    document = valid_logistic()
    del document["intercept"]
    with pytest.raises(model_contract.ArtifactError, match="requires intercept"):
        model_contract.validate_artifact(document)


def test_a_coef_of_the_wrong_width_is_refused():
    document = valid_logistic()
    document["coef"] = [0.1]
    with pytest.raises(model_contract.ArtifactError, match="coef has 1 values for 2 features"):
        model_contract.validate_artifact(document)


def test_a_forest_that_carries_an_intercept_is_refused():
    document = valid_forest()
    document["intercept"] = 0.0
    with pytest.raises(model_contract.ArtifactError, match="carries intercept or coef"):
        model_contract.validate_artifact(document)


def test_a_forest_without_trees_is_refused():
    document = valid_forest()
    document["trees"] = []
    with pytest.raises(model_contract.ArtifactError, match="at least one tree"):
        model_contract.validate_artifact(document)


def test_an_unknown_kind_is_refused():
    document = valid_logistic()
    document["kind"] = "onnx"
    with pytest.raises(model_contract.ArtifactError, match="neither"):
        model_contract.validate_artifact(document)


def test_a_missing_kind_is_refused():
    document = valid_logistic()
    del document["kind"]
    with pytest.raises(model_contract.ArtifactError, match="kind is required"):
        model_contract.validate_artifact(document)


# ---------------------------------------------------------------------------
# Trees: a cycle here is a hang at scoring time, not an error a caller can see
# ---------------------------------------------------------------------------


def test_a_leaf_that_also_carries_a_split_is_refused():
    document = valid_forest()
    document["trees"][0]["nodes"][1]["feature"] = 0
    with pytest.raises(model_contract.ArtifactError, match="leaf and also carries feature"):
        model_contract.validate_artifact(document)


@pytest.mark.parametrize("value", [-0.01, 1.01, 250])
def test_a_leaf_whose_value_is_not_a_probability_is_refused(value):
    document = valid_forest()
    document["trees"][0]["nodes"][1]["value"] = value
    with pytest.raises(model_contract.ArtifactError, match=r"not a probability"):
        model_contract.validate_artifact(document)


def test_a_split_node_missing_a_child_is_refused():
    document = valid_forest()
    del document["trees"][0]["nodes"][0]["right"]
    with pytest.raises(model_contract.ArtifactError, match="missing right"):
        model_contract.validate_artifact(document)


def test_a_feature_index_outside_the_declared_width_is_refused():
    document = valid_forest()
    document["trees"][0]["nodes"][0]["feature"] = 2
    with pytest.raises(model_contract.ArtifactError, match="feature index 2 is outside"):
        model_contract.validate_artifact(document)


def test_a_child_index_outside_the_tree_is_refused():
    document = valid_forest()
    document["trees"][0]["nodes"][0]["left"] = 7
    with pytest.raises(model_contract.ArtifactError, match="left index 7 is outside"):
        model_contract.validate_artifact(document)


def test_a_node_pointing_at_itself_is_refused():
    document = valid_forest()
    document["trees"][0]["nodes"][0]["left"] = 0
    with pytest.raises(model_contract.ArtifactError, match="points at itself"):
        model_contract.validate_artifact(document)


def test_a_cycle_is_refused():
    document = valid_forest()
    document["trees"][0]["nodes"] = [
        {"feature": 0, "threshold": 0.5, "left": 1, "right": 1},
        {"feature": 1, "threshold": 0.5, "left": 0, "right": 0},
    ]
    with pytest.raises(model_contract.ArtifactError, match="reached twice on one path|cycle"):
        model_contract.validate_artifact(document)


def test_an_unreachable_node_is_refused():
    document = valid_forest()
    document["trees"][0]["nodes"].append({"value": 0.5})
    with pytest.raises(model_contract.ArtifactError, match="unreachable from the root"):
        model_contract.validate_artifact(document)


def test_a_tree_with_no_nodes_is_refused():
    document = valid_forest()
    document["trees"] = [{"nodes": []}]
    with pytest.raises(model_contract.ArtifactError, match="has no nodes"):
        model_contract.validate_artifact(document)


def test_a_float_in_an_integer_field_is_refused():
    document = valid_forest()
    document["trees"][0]["nodes"][0]["left"] = 1.5
    with pytest.raises(model_contract.ArtifactError, match="integer field cannot carry a fraction"):
        model_contract.validate_artifact(document)


# ---------------------------------------------------------------------------
# Reading and writing
# ---------------------------------------------------------------------------


def test_trailing_content_after_the_document_is_refused():
    text = json.dumps(valid_logistic()) + "\n{}"
    with pytest.raises(model_contract.ArtifactError, match="more than one JSON document"):
        model_contract.parse_artifact(text)


def test_a_document_that_is_not_json_is_refused():
    with pytest.raises(model_contract.ArtifactError, match="decode artifact"):
        model_contract.parse_artifact("{not json")


def test_loading_a_missing_file_names_the_path(tmp_path):
    with pytest.raises(model_contract.ArtifactError, match="read model artifact"):
        model_contract.load_artifact(tmp_path / "absent.json")


def test_the_writer_validates_before_it_writes(tmp_path):
    document = copy.deepcopy(valid_logistic())
    document["features"][0]["unit"] = ""
    path = tmp_path / "never.json"
    with pytest.raises(model_contract.ArtifactError, match="has no unit"):
        model_contract.write_artifact(path, document)
    assert not path.exists()


def test_the_writer_round_trips_an_accepted_document(tmp_path):
    document = valid_forest()
    path = tmp_path / "model.json"
    model_contract.write_artifact(path, document)
    assert model_contract.load_artifact(path) == document
    assert path.read_text(encoding="utf-8").endswith("\n")
