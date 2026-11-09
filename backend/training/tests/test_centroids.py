"""Comuna centroids: 38 of region 05, derived from the tracked DPA geometry.

The file is data, so the tests here are about the two things data can get wrong
silently: an entry that no longer matches the geometry it claims to come from, and a
coordinate that is not where the comuna is. Both are recomputed rather than trusted.
"""

from __future__ import annotations

import hashlib
import json
from pathlib import Path

import pytest

import centroids

DATA = Path(__file__).resolve().parents[1] / "data" / "comunas-valparaiso-centroids.json"
MAPPING = centroids.load_centroids(DATA)


def document() -> dict:
    return json.loads(DATA.read_text(encoding="utf-8"))


def test_the_mapping_covers_all_thirty_eight_comunas_of_region_05():
    assert len(MAPPING.entries) == 38
    assert MAPPING.region_cut == "05"
    assert all(entry.cut.startswith("05") for entry in MAPPING.entries)
    assert len({entry.cut for entry in MAPPING.entries}) == 38


@pytest.mark.parametrize(
    ("cut", "name"),
    [
        ("05101", "Valparaíso"),
        ("05109", "Viña del Mar"),
        ("05601", "San Antonio"),
        ("05201", "Isla de Pascua"),
        ("05104", "Juan Fernández"),
    ],
)
def test_every_committed_comuna_resolves_by_cut_and_by_name(cut, name):
    assert MAPPING.centroid(cut).name == name
    resolved = MAPPING.by_name(name)
    assert resolved is not None
    assert resolved.cut == cut


def test_name_resolution_folds_accents_like_the_rest_of_the_package():
    foldable = MAPPING.by_name("Valparaiso")
    assert foldable is not None
    assert foldable.cut == "05101"
    assert MAPPING.by_name("Vina del Mar") is not None
    assert MAPPING.by_name("Antartica") is None


def test_an_unknown_cut_is_refused_rather_than_approximated():
    with pytest.raises(centroids.CentroidError) as refusal:
        MAPPING.centroid("07101")  # Talca, region 07
    assert "07101" in str(refusal.value)
    assert "38" in str(refusal.value)


def test_the_provenance_checksum_is_recomputed_from_the_committed_entries():
    """Independent of the module: hash the file's own entries and compare."""
    raw = document()
    canonical = json.dumps(
        raw["entries"], ensure_ascii=False, sort_keys=True, separators=(",", ":")
    )
    digest = hashlib.sha256(canonical.encode("utf-8")).hexdigest()
    assert digest == raw["provenance"]["entries_sha256"]
    assert raw["provenance"]["record_count"] == 38


def test_the_provenance_names_the_geometry_it_came_from_and_its_hash():
    provenance = document()["provenance"]
    assert provenance["url"].startswith("https://rest-sit.mop.gob.cl/")
    assert "CUT_REG='05'" in provenance["query"]
    assert provenance["retrieved_at"]
    assert provenance["derived_at"]
    geometry = centroids.DEFAULT_GEOJSON
    digest = hashlib.sha256(geometry.read_bytes()).hexdigest()
    assert provenance["derived_from"]["sha256"] == digest
    assert provenance["derived_from"]["record_count"] == 38


def test_the_derivation_reproduces_the_committed_entries():
    """The file is not hand-edited: rerunning the derivation gives the same rows."""
    derived = centroids.entries_from_geojson(centroids.DEFAULT_GEOJSON)
    assert [entry.as_entry() for entry in derived] == document()["entries"]


@pytest.mark.parametrize(
    ("cut", "latitude", "longitude", "tolerance"),
    [
        # Published city coordinates, loose tolerance: an area-weighted planar centroid
        # sits somewhere inside a comuna, not on its plaza.
        ("05101", -33.0472, -71.6127, 0.15),  # Valparaíso
        ("05109", -33.0245, -71.5518, 0.15),  # Viña del Mar
        ("05601", -33.5933, -71.6128, 0.20),  # San Antonio
        ("05501", -32.8805, -71.2478, 0.10),  # Quillota
        ("05701", -32.7507, -70.7255, 0.10),  # San Felipe
        ("05201", -27.1127, -109.3497, 0.10),  # Isla de Pascua
    ],
)
def test_each_coordinate_lands_where_the_comuna_is(cut, latitude, longitude, tolerance):
    entry = MAPPING.centroid(cut)
    assert entry.latitude == pytest.approx(latitude, abs=tolerance)
    assert entry.longitude == pytest.approx(longitude, abs=tolerance)


def test_the_archipelagos_are_marked_as_multi_part_because_their_centroid_is_water():
    juan_fernandez = MAPPING.centroid("05104")
    easter_island = MAPPING.centroid("05201")
    assert juan_fernandez.parts > 1
    assert easter_island.parts > 1
    assert -81.0 < juan_fernandez.longitude < -78.0
    assert easter_island.longitude < -100.0


def test_a_centroid_is_inside_its_own_geometry_bounding_box():
    """The weighted centroid must fall inside the shape it is derived from."""
    geometry = json.loads(centroids.DEFAULT_GEOJSON.read_text(encoding="utf-8"))
    boxes = {}
    for feature in geometry["features"]:
        cut = feature["properties"]["CUT_COM"]
        kind = feature["geometry"]["type"]
        polygons = (
            feature["geometry"]["coordinates"]
            if kind == "MultiPolygon"
            else [feature["geometry"]["coordinates"]]
        )
        points = [point for polygon in polygons for ring in polygon for point in ring]
        boxes[cut] = (
            min(point[0] for point in points),
            max(point[0] for point in points),
            min(point[1] for point in points),
            max(point[1] for point in points),
        )
    for entry in MAPPING.entries:
        west, east, south, north = boxes[entry.cut]
        assert west <= entry.longitude <= east, entry.cut
        assert south <= entry.latitude <= north, entry.cut


def test_a_file_edited_after_its_checksum_is_refused(tmp_path):
    tampered = document()
    tampered["entries"][0]["longitude"] = tampered["entries"][0]["longitude"] + 1.0
    path = tmp_path / "centroids.json"
    path.write_text(json.dumps(tampered, ensure_ascii=False), encoding="utf-8")
    with pytest.raises(centroids.CentroidError) as refusal:
        centroids.load_centroids(path)
    assert "checksum" in str(refusal.value)


def test_an_entry_missing_its_coordinate_is_refused_with_the_file_named(tmp_path):
    broken = document()
    del broken["entries"][3]["latitude"]
    path = tmp_path / "centroids.json"
    path.write_text(json.dumps(broken, ensure_ascii=False), encoding="utf-8")
    with pytest.raises(centroids.CentroidError) as refusal:
        centroids.load_centroids(path)
    assert "centroids.json" in str(refusal.value)


def test_a_geometry_from_another_region_is_refused(tmp_path):
    geometry = json.loads(centroids.DEFAULT_GEOJSON.read_text(encoding="utf-8"))
    geometry["features"] = geometry["features"][:1]
    geometry["features"][0]["properties"]["CUT_COM"] = "07101"
    path = tmp_path / "other.geojson"
    path.write_text(json.dumps(geometry), encoding="utf-8")
    with pytest.raises(centroids.CentroidError) as refusal:
        centroids.entries_from_geojson(path)
    assert "07101" in str(refusal.value)
    assert "not in region 05" in str(refusal.value)


def test_a_repeated_cut_is_refused_rather_than_picking_one(tmp_path):
    geometry = json.loads(centroids.DEFAULT_GEOJSON.read_text(encoding="utf-8"))
    geometry["features"] = geometry["features"][:2]
    geometry["features"][1]["properties"]["CUT_COM"] = geometry["features"][0]["properties"][
        "CUT_COM"
    ]
    path = tmp_path / "repeated.geojson"
    path.write_text(json.dumps(geometry), encoding="utf-8")
    with pytest.raises(centroids.CentroidError) as refusal:
        centroids.entries_from_geojson(path)
    assert "repeated" in str(refusal.value)


def test_a_geometry_with_no_positive_area_is_refused():
    with pytest.raises(centroids.CentroidError) as refusal:
        centroids.geometry_centroid(
            {"type": "Polygon", "coordinates": [[[0.0, 0.0], [1.0, 0.0], [1.0, 0.0], [0.0, 0.0]]]}
        )
    assert "no outer ring" in str(refusal.value)


def test_an_unsupported_geometry_type_is_refused():
    with pytest.raises(centroids.CentroidError):
        centroids.geometry_centroid({"type": "Point", "coordinates": [0.0, 0.0]})


def test_a_hole_is_subtracted_from_the_part_that_contains_it():
    """A square with a centred hole keeps the square's centroid and loses the hole's area."""
    outer = [[0.0, 0.0], [4.0, 0.0], [4.0, 4.0], [0.0, 4.0], [0.0, 0.0]]  # 16 deg^2, CCW
    hole = [[2.5, 1.5], [1.5, 1.5], [1.5, 2.5], [2.5, 2.5], [2.5, 1.5]]  # 1 deg^2, CW
    longitude, latitude, area, parts = centroids.geometry_centroid(
        {"type": "Polygon", "coordinates": [outer, hole]}
    )
    assert area == pytest.approx(15.0)
    assert parts == 1
    assert longitude == pytest.approx(2.0, abs=1e-9)
    assert latitude == pytest.approx(2.0, abs=1e-9)
