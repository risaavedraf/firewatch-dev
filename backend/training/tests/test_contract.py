"""The feature vocabulary comes from the artifact, and nothing else repeats it.

These tests are the reason the panel cannot drift from the model: the artifact
descriptor `risk` reads is the only list of names, order and units, and the module's
producer table is checked against it in both directions before a single row is read.
"""

from __future__ import annotations

import json
from pathlib import Path

import pytest
from conftest import SAMPLE_FIXTURE

import assemble

ARTIFACT = assemble.DEFAULT_ARTIFACT


def artifact_copy(tmp_path: Path, mutate) -> Path:
    """Write a mutated copy of the real descriptor and return its path."""
    document = json.loads(ARTIFACT.read_text(encoding="utf-8"))
    mutate(document)
    path = tmp_path / "artifact.json"
    path.write_text(json.dumps(document, ensure_ascii=False), encoding="utf-8")
    return path


def test_the_artifact_declares_exactly_the_seven_panel_features():
    artifact = assemble.load_artifact(ARTIFACT)
    assert artifact.names == (
        "temp_max",
        "temp_min",
        "humidity_rel",
        "wind_speed",
        "wind_dir",
        "precip_sum",
        "radiation_sol",
    )
    assert set(assemble.PRODUCERS) == set(artifact.names)
    assemble.check_producers(artifact)


def test_an_artifact_feature_without_a_producer_is_refused(tmp_path):
    path = artifact_copy(
        tmp_path,
        lambda document: document["features"].append(
            {"name": "wind_gust", "unit": "m/s", "min": 0, "max": 60}
        ),
    )
    with pytest.raises(assemble.AssembleError) as refusal:
        assemble.assemble(SAMPLE_FIXTURE, artifact_path=path, generated_at="2026-11-06T00:00:00Z")
    assert "wind_gust" in str(refusal.value)


def test_a_requested_feature_outside_the_vocabulary_is_refused():
    with pytest.raises(assemble.AssembleError) as refusal:
        assemble.assemble(
            SAMPLE_FIXTURE,
            requested=["temp_max", "temperatura_media"],
            generated_at="2026-11-06T00:00:00Z",
        )
    assert "temperatura_media" in str(refusal.value)


def test_a_requested_subset_keeps_the_artifact_order():
    _panel, manifest = assemble.assemble(
        SAMPLE_FIXTURE,
        requested=["precip_sum", "temp_max"],
        generated_at="2026-11-06T00:00:00Z",
    )
    assert [feature["name"] for feature in manifest["features"]] == ["temp_max", "precip_sum"]


def test_a_source_unit_that_contradicts_the_artifact_is_refused(tmp_path):
    def relabel(document):
        for feature in document["features"]:
            if feature["name"] == "humidity_rel":
                feature["unit"] = "kg/kg"

    path = artifact_copy(tmp_path, relabel)
    with pytest.raises(assemble.AssembleError) as refusal:
        assemble.assemble(SAMPLE_FIXTURE, artifact_path=path, generated_at="2026-11-06T00:00:00Z")
    assert "kg/kg" in str(refusal.value)


def test_a_descriptor_with_the_wrong_format_is_refused(tmp_path):
    path = artifact_copy(tmp_path, lambda document: document.update({"format": "onnx"}))
    with pytest.raises(assemble.AssembleError) as refusal:
        assemble.load_artifact(path)
    assert "firewatch-model/v1" in str(refusal.value)


def test_a_duplicated_feature_name_is_refused(tmp_path):
    def duplicate(document):
        document["features"][1]["name"] = "temp_max"

    path = artifact_copy(tmp_path, duplicate)
    with pytest.raises(assemble.AssembleError) as refusal:
        assemble.load_artifact(path)
    assert "twice" in str(refusal.value)


def test_a_missing_descriptor_is_refused(tmp_path):
    with pytest.raises(assemble.AssembleError):
        assemble.load_artifact(tmp_path / "absent.json")


def test_the_cli_reports_a_refusal_as_exit_2_and_a_message(capsys):
    code = assemble.main(
        ["--source", str(SAMPLE_FIXTURE), "--features", "not_a_feature", "--out", "unused"]
    )
    assert code == 2
    assert "not_a_feature" in capsys.readouterr().err


def test_the_cli_writes_the_panel_and_manifest(tmp_path, capsys):
    code = assemble.main(
        [
            "--source",
            str(SAMPLE_FIXTURE),
            "--out",
            str(tmp_path / "out"),
            "--generated-at",
            "2026-11-06T00:00:00Z",
        ]
    )
    assert code == 0
    assert (tmp_path / "out" / "panel.csv").is_file()
    assert (tmp_path / "out" / "manifest.json").is_file()
    printed = capsys.readouterr().out
    assert "comuna-days: 84" in printed
    assert "features unavailable:" in printed
    notes = json.loads((tmp_path / "out" / "manifest.json").read_text(encoding="utf-8"))["notes"]
    assert "not a training set" in notes[0]
