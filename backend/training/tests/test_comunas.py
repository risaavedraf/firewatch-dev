"""Comuna name -> CUT: the mapping is complete for this source and checksummed."""

from __future__ import annotations

import json
from pathlib import Path

import pytest

import comunas

MAPPING = comunas.load_mapping(
    Path(__file__).resolve().parents[1] / "data" / "comunas-maule-cut.json"
)


def test_the_mapping_covers_the_thirty_comunas_of_region_07():
    assert len(MAPPING.comunas) == 30
    assert MAPPING.region_cut == "07"
    assert all(comuna.cut.startswith("07") for comuna in MAPPING.comunas)
    assert len({comuna.cut for comuna in MAPPING.comunas}) == 30


@pytest.mark.parametrize(
    ("name", "cut"),
    [
        ("Talca", "07101"),
        ("Constitución", "07102"),
        ("Río Claro", "07108"),
        ("Cauquenes", "07201"),
        ("Vichuquén", "07309"),
        ("Yerbas Buenas", "07408"),
    ],
)
def test_official_spellings_resolve(name, cut):
    comuna = MAPPING.resolve(name)
    assert comuna is not None
    assert comuna.cut == cut


@pytest.mark.parametrize(
    ("written_by_the_source", "cut"),
    [
        # The registry drops accents and one abbreviation ("Colbun"), so resolution
        # folds them instead of carrying eight hand-written aliases.
        ("Constitucion", "07102"),
        ("Curico", "07301"),
        ("Hualane", "07302"),
        ("Licanten", "07303"),
        ("Longavi", "07403"),
        ("Rio Claro", "07108"),
        ("Vichuquen", "07309"),
        ("Colbun", "07402"),
        ("  sagrada   familia ", "07307"),
    ],
)
def test_source_spellings_resolve_through_folding(written_by_the_source, cut):
    comuna = MAPPING.resolve(written_by_the_source)
    assert comuna is not None
    assert comuna.cut == cut


def test_an_unknown_name_resolves_to_nothing_rather_than_a_guess():
    assert MAPPING.resolve("Antartica") is None
    assert MAPPING.resolve("") is None
    assert MAPPING.resolve("S. Clemente") is None


def test_the_mapping_checksum_matches_the_committed_entries():
    assert comunas.entries_checksum(MAPPING.comunas) == MAPPING.provenance["entries_sha256"]
    assert MAPPING.provenance["record_count"] == 30


def test_the_provenance_names_the_official_layer():
    assert "rest-sit.mop.gob.cl" in MAPPING.provenance["url"]
    assert MAPPING.provenance["retrieved_at"]


def test_two_entries_that_fold_to_one_key_are_refused():
    with pytest.raises(ValueError) as refusal:
        comunas.ComunaMap(
            region_cut="07",
            region_name="Maule",
            comunas=(
                comunas.Comuna(cut="07301", name="Curicó", provincia="Curicó"),
                comunas.Comuna(cut="07999", name="Curico", provincia="Curicó"),
            ),
            provenance={},
        )
    assert "fold" in str(refusal.value)


def test_a_mapping_without_entries_is_refused(tmp_path):
    path = tmp_path / "mapping.json"
    path.write_text(json.dumps({"entries": []}), encoding="utf-8")
    with pytest.raises(ValueError):
        comunas.load_mapping(path)
