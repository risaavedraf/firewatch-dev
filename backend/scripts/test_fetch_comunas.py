"""Unit tests for the Esri JSON -> GeoJSON conversion.

No network is used: every case runs against the small fixture embedded below.
Run with ``python backend/scripts/test_fetch_comunas.py``.
"""
from __future__ import annotations

import unittest

import fetch_comunas  # type: ignore[import-not-found]


def ring(*points):
    return [list(point) for point in points]


# Esri source convention: outer rings clockwise (negative area), holes
# counter-clockwise (positive area). Rings here are intentionally unclosed to
# prove the converter closes them.
SIMPLE_OUTER = ring((0, 0), (0, 10), (10, 10), (10, 0))
HOLE_OUTER = ring((0, 0), (0, 20), (20, 20), (20, 0))
HOLE_INNER = ring((5, 5), (10, 5), (10, 10), (5, 10))
MULTI_OUTER_A = ring((30, 30), (30, 40), (40, 40), (40, 30))
MULTI_OUTER_B = ring((100, 100), (100, 110), (110, 110), (110, 100))

PAYLOAD = {
    "geometryType": "esriGeometryPolygon",
    "features": [
        {
            "attributes": {
                "CUT_COM": "05804",
                "COMUNA": "Comuna C",
                "PROVINCIA": "Provincia",
                "CUT_PROV": "058",
            },
            "geometry": {"rings": [MULTI_OUTER_B, MULTI_OUTER_A]},
        },
        {
            "attributes": {
                "CUT_COM": "05301",
                "COMUNA": "Comuna B",
                "PROVINCIA": "Provincia",
                "CUT_PROV": "053",
            },
            "geometry": {"rings": [SIMPLE_OUTER]},
        },
        {
            "attributes": {
                "CUT_COM": "05101",
                "COMUNA": "Comuna A",
                "PROVINCIA": "Provincia",
                "CUT_PROV": "051",
            },
            "geometry": {"rings": [HOLE_OUTER, HOLE_INNER]},
        },
    ],
}

TRUNCATED = dict(PAYLOAD, exceededTransferLimit=True)


def feature_by_cut(collection, cut):
    (feature,) = [
        item for item in collection["features"] if item["properties"]["CUT_COM"] == cut
    ]
    return feature


class TestGeometryConversion(unittest.TestCase):
    def setUp(self):
        self.collection = fetch_comunas.esri_to_geojson(PAYLOAD)

    def test_feature_collection_with_sorted_features(self):
        self.assertEqual(self.collection["type"], "FeatureCollection")
        cuts = [item["properties"]["CUT_COM"] for item in self.collection["features"]]
        self.assertEqual(cuts, sorted(cuts))
        self.assertEqual(cuts, ["05101", "05301", "05804"])

    def test_single_outer_ring_becomes_counter_clockwise_polygon(self):
        geometry = feature_by_cut(self.collection, "05301")["geometry"]
        self.assertEqual(geometry["type"], "Polygon")
        self.assertEqual(len(geometry["coordinates"]), 1)
        outer = geometry["coordinates"][0]
        self.assertEqual(outer[0], outer[-1])
        self.assertGreater(fetch_comunas.signed_area(outer), 0)

    def test_hole_is_classified_and_oriented_per_rfc_7946(self):
        geometry = feature_by_cut(self.collection, "05101")["geometry"]
        self.assertEqual(geometry["type"], "Polygon")
        self.assertEqual(len(geometry["coordinates"]), 2)
        outer, hole = geometry["coordinates"]
        self.assertEqual(outer[0], outer[-1])
        self.assertEqual(hole[0], hole[-1])
        self.assertGreater(fetch_comunas.signed_area(outer), 0)
        self.assertLess(fetch_comunas.signed_area(hole), 0)
        self.assertTrue(fetch_comunas.point_in_ring(hole[0], outer))

    def test_several_outer_rings_become_multipolygon(self):
        geometry = feature_by_cut(self.collection, "05804")["geometry"]
        self.assertEqual(geometry["type"], "MultiPolygon")
        self.assertEqual(len(geometry["coordinates"]), 2)
        for polygon in geometry["coordinates"]:
            for element in polygon:
                self.assertEqual(element[0], element[-1])
                self.assertGreater(fetch_comunas.signed_area(element), 0)

    def test_unclosed_input_ring_is_closed_in_output(self):
        self.assertNotEqual(SIMPLE_OUTER[0], SIMPLE_OUTER[-1])
        outer = feature_by_cut(self.collection, "05301")["geometry"]["coordinates"][0]
        self.assertEqual(outer[0], outer[-1])

    def test_close_ring_is_idempotent(self):
        closed = fetch_comunas.close_ring(SIMPLE_OUTER)
        self.assertEqual(fetch_comunas.close_ring(closed), closed)

    def test_properties_keep_the_four_declared_fields(self):
        properties = feature_by_cut(self.collection, "05101")["properties"]
        self.assertEqual(
            set(properties), {"CUT_COM", "COMUNA", "PROVINCIA", "CUT_PROV"}
        )


class TestCompleteness(unittest.TestCase):
    def test_complete_when_count_matches_and_not_truncated(self):
        result = fetch_comunas.check_completeness(PAYLOAD, expected=3)
        self.assertEqual(result["record_count"], 3)
        self.assertTrue(result["record_count_matches_expected"])
        self.assertFalse(result["exceeded_transfer_limit"])
        self.assertTrue(result["complete"])

    def test_truncated_payload_is_incomplete(self):
        result = fetch_comunas.check_completeness(TRUNCATED, expected=3)
        self.assertTrue(result["exceeded_transfer_limit"])
        self.assertFalse(result["complete"])

    def test_count_mismatch_is_incomplete(self):
        result = fetch_comunas.check_completeness(PAYLOAD, expected=38)
        self.assertFalse(result["record_count_matches_expected"])
        self.assertFalse(result["complete"])


class TestProvenance(unittest.TestCase):
    def test_only_embedded_provenance_destination(self):
        self.assertEqual(
            fetch_comunas.PROVENANCE_RELPATH,
            "backend/contracts/gen/geo/valparaiso-comunas.provenance.json",
        )

    def test_serialization_is_deterministic(self):
        first = fetch_comunas.dumps_provenance(self.provenance())
        self.assertEqual(first, fetch_comunas.dumps_provenance(self.provenance()))
        self.assertTrue(first.endswith(b"\n"))

    def provenance(self):
        return fetch_comunas.build_provenance(
            endpoint=fetch_comunas.QUERY_URL,
            query_sha256="0" * 64,
            retrieved_at="2026-01-01T00:00:00+00:00",
            cutoff_at="2026-01-01T00:00:00+00:00",
            record_count=38,
            expected_record_count=38,
            complete=True,
            exceeded_transfer_limit=False,
            raw_path="data/raw/geo/valparaiso-comunas-20260101.esri.json",
            raw_sha256="a" * 64,
            derived_path="backend/contracts/gen/geo/valparaiso-comunas.geojson",
            derived_sha256="b" * 64,
        )

    def test_attribution_verbatim(self):
        expected = (
            "Subsecretaria de Desarrollo Regional y Administrativo (SUBDERE), "
            "Instituto Geografico Militar (IGM) e Instituto Nacional de Estadisticas (INE)"
        )
        self.assertEqual(fetch_comunas.ATTRIBUTION, expected)
        self.assertEqual(self.provenance()["attribution"], expected)

    def test_restriction_verbatim(self):
        expected = (
            "las publicaciones y productos derivados de esta cartografia deben ser "
            "revisadas y validadas por DIFROL."
        )
        self.assertEqual(fetch_comunas.RESTRICTION, expected)
        self.assertEqual(self.provenance()["restriction"], expected)

    def test_formats_and_spatial_references(self):
        provenance = self.provenance()
        self.assertEqual(provenance["requested_srid"], 4326)
        self.assertEqual(provenance["source_wkid"], 5360)
        self.assertEqual(provenance["source_format"], "Esri JSON")
        self.assertEqual(provenance["derived_format"], "GeoJSON (RFC 7946)")

    def test_cutoff_is_retrieved_at_and_notes_declare_it(self):
        provenance = self.provenance()
        self.assertEqual(provenance["cutoff_at"], provenance["retrieved_at"])
        self.assertIn("cutoff_at", provenance["notes"])
        self.assertIn("Antartica", provenance["notes"])

    def test_required_fields_are_present(self):
        required = {
            "source_id",
            "provider",
            "endpoint",
            "query_sha256",
            "requested_srid",
            "source_wkid",
            "source_format",
            "derived_format",
            "retrieved_at",
            "cutoff_at",
            "record_count",
            "expected_record_count",
            "complete",
            "exceeded_transfer_limit",
            "raw_path",
            "raw_sha256",
            "derived_path",
            "derived_sha256",
            "attribution",
            "restriction",
            "restriction_source",
            "notes",
        }
        self.assertLessEqual(required, set(self.provenance()))


if __name__ == "__main__":
    unittest.main()
