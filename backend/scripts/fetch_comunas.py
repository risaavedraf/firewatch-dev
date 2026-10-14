"""Acquire the 38 communes of Valparaiso from the official DPA layer.

Standard library only, so a clean clone needs no installation. The script turns
the official MOP/IDE Chile Esri JSON response into a versioned GeoJSON product
asset plus a provenance record for the Bases traceability table. It never
touches the database and never talks to any API other than the one declared in
``QUERY_URL``.
"""
from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import sys
import urllib.request
from pathlib import Path

# Verified query. Do not change it and do not re-derive it: the sha256 of this
# exact string is recorded in the provenance file as ``query_sha256``.
QUERY_URL = "https://rest-sit.mop.gob.cl/arcgis/rest/services/INTEROP/SERVICIO_DPA/MapServer/1/query?where=CUT_REG%3D%2705%27&outFields=CUT_COM,COMUNA,PROVINCIA,CUT_PROV&returnGeometry=true&outSR=4326&f=json"

USER_AGENT = "FireWatch/1.0 (academic geospatial ingestion; stdlib urllib)"
TIMEOUT_SECONDS = 60

# The 38 communes of region 05 (Valparaiso). The Antarctic commune is not part
# of this layer and is deliberately not expected.
EXPECTED_RECORD_COUNT = 38
REQUESTED_SRID = 4326
SOURCE_WKID = 5360

DERIVED_RELPATH = "backend/contracts/gen/geo/valparaiso-comunas.geojson"
PROVENANCE_RELPATH = "backend/contracts/gen/geo/valparaiso-comunas.provenance.json"
RAW_DIR_RELPATH = "data/raw/geo"

ATTRIBUTION = (
    "Subsecretaria de Desarrollo Regional y Administrativo (SUBDERE), "
    "Instituto Geografico Militar (IGM) e Instituto Nacional de Estadisticas (INE)"
)
RESTRICTION = (
    "las publicaciones y productos derivados de esta cartografia deben ser "
    "revisadas y validadas por DIFROL."
)
RESTRICTION_SOURCE = (
    "Metadatos del servicio INTEROP/SERVICIO_DPA, capa Comunas (MapServer/1) de IDE Chile: "
    "https://rest-sit.mop.gob.cl/arcgis/rest/services/INTEROP/SERVICIO_DPA/MapServer/1?f=json"
)
NOTES = (
    "El servicio no publica fecha de corte (cutoff): cutoff_at se fija identico a "
    "retrieved_at y no se estima ni se inventa otro valor. La comuna Antartica no existe "
    "en esta capa y no se inventa. La respuesta cabe en una sola pagina (maxRecordCount "
    "1000, 38 filas); la bandera exceeded_transfer_limit se registra y una respuesta "
    "truncada aborta la ejecucion por defecto. outSR=4326 evita reproyectar aunque la "
    "capa origen declare wkid 5360."
)


# ---------------------------------------------------------------------------
# Repository handling and network access
# ---------------------------------------------------------------------------

def repository_root() -> Path:
    """Resolve the repository root from this file and prove it looks right."""
    root = Path(__file__).resolve().parents[2]
    marker = root / "backend" / "services" / "risk" / "go.mod"
    if not marker.is_file():
        raise SystemExit(
            f"Refusing to run: {root} does not look like the FireWatch repository root "
            f"(missing {marker})."
        )
    return root


def fetch_query(url: str = QUERY_URL, timeout: int = TIMEOUT_SECONDS) -> bytes:
    """Fetch the exact verified query and return the raw bytes as received."""
    # The URL is the hardcoded https constant; no user input reaches it.
    request = urllib.request.Request(  # noqa: S310
        url, headers={"User-Agent": USER_AGENT, "Accept": "application/json"}
    )
    with urllib.request.urlopen(request, timeout=timeout) as response:  # noqa: S310
        if response.status != 200:
            raise SystemExit(f"Source returned HTTP {response.status} for {url}")
        return response.read()


def parse_payload(raw: bytes) -> dict:
    """Decode the response and fail loudly when it is not a JSON object."""
    try:
        payload = json.loads(raw.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError) as error:
        raise SystemExit(f"Source did not return JSON: {error}") from error
    if not isinstance(payload, dict):
        raise SystemExit("Source returned JSON but not an object")
    if "error" in payload:
        raise SystemExit(f"Source reported an error: {json.dumps(payload['error'])[:500]}")
    return payload


def validate_payload(payload: dict) -> list:
    """Prove the payload is a polygon layer with at least one feature."""
    if payload.get("geometryType") != "esriGeometryPolygon":
        raise SystemExit(
            f"Unexpected geometryType {payload.get('geometryType')!r}; "
            "expected 'esriGeometryPolygon'"
        )
    features = payload.get("features")
    if not isinstance(features, list) or not features:
        raise SystemExit("Source returned an empty feature list")
    return features


# ---------------------------------------------------------------------------
# Completeness
# ---------------------------------------------------------------------------

def check_completeness(payload: dict, expected: int = EXPECTED_RECORD_COUNT) -> dict:
    """Assess truncation and record count without trusting the source."""
    features = payload.get("features") or []
    record_count = len(features)
    exceeded = bool(payload.get("exceededTransferLimit", False))
    matches = record_count == expected
    return {
        "record_count": record_count,
        "record_count_matches_expected": matches,
        "exceeded_transfer_limit": exceeded,
        "complete": (not exceeded) and matches,
    }


# ---------------------------------------------------------------------------
# Pure Esri JSON -> GeoJSON conversion (no network, importable, testable)
# ---------------------------------------------------------------------------

def signed_area(ring: list) -> float:
    """Shoelace area of a ring. Positive means counter-clockwise."""
    total = 0.0
    for index in range(len(ring) - 1):
        x1, y1 = ring[index][0], ring[index][1]
        x2, y2 = ring[index + 1][0], ring[index + 1][1]
        total += x1 * y2 - x2 * y1
    return total / 2.0


def close_ring(ring: list) -> list:
    """Return a copy of the ring with the first point repeated as the last."""
    points = [list(point) for point in ring]
    if points[0] != points[-1]:
        points.append(list(points[0]))
    return points


def orient_ring(ring: list, counter_clockwise: bool) -> list:
    """Orient a closed ring as requested, reversing it only when needed."""
    is_counter_clockwise = signed_area(ring) > 0
    if is_counter_clockwise == counter_clockwise:
        return ring
    return ring[::-1]


def point_in_ring(point: list, ring: list) -> bool:
    """Ray-casting point-in-polygon test for the two first coordinates."""
    x, y = point[0], point[1]
    inside = False
    previous = len(ring) - 1
    for index in range(len(ring)):
        xi, yi = ring[index][0], ring[index][1]
        xj, yj = ring[previous][0], ring[previous][1]
        if ((yi > y) != (yj > y)) and (x < (xj - xi) * (y - yi) / (yj - yi) + xi):
            inside = not inside
        previous = index
    return inside


def convert_geometry(rings: list) -> dict:
    """Convert one Esri ring set into an RFC 7946 Polygon or MultiPolygon.

    Esri declares outer rings clockwise (negative signed area) and holes
    counter-clockwise (positive signed area); RFC 7946 requires the opposite, so
    the orientation is corrected from the measured signed area rather than
    assumed. Each hole is assigned to the smallest outer ring that contains it.
    """
    closed = [close_ring(ring) for ring in rings]
    outers = sorted(
        (ring for ring in closed if signed_area(ring) < 0),
        key=lambda ring: abs(signed_area(ring)),
    )
    holes = [ring for ring in closed if signed_area(ring) > 0]
    if not outers:
        raise ValueError("Esri ring set has no outer ring (clockwise orientation)")

    assigned = [[] for _ in outers]
    for hole in holes:
        target = None
        for index, outer in enumerate(outers):
            if point_in_ring(hole[0], outer):
                target = index
                break
        if target is None:
            raise ValueError("Esri hole ring is not contained by any outer ring")
        assigned[target].append(orient_ring(hole, counter_clockwise=False))

    polygons = [
        [orient_ring(outer, counter_clockwise=True), *hole_rings]
        for outer, hole_rings in zip(outers, assigned, strict=True)
    ]
    if len(polygons) == 1:
        return {"type": "Polygon", "coordinates": polygons[0]}
    return {"type": "MultiPolygon", "coordinates": polygons}


def esri_to_geojson(payload: dict) -> dict:
    """Turn an Esri JSON polygon response into a sorted FeatureCollection."""
    features = []
    for feature in payload["features"]:
        attributes = feature["attributes"]
        features.append(
            {
                "type": "Feature",
                "properties": {
                    "CUT_COM": attributes["CUT_COM"],
                    "COMUNA": attributes["COMUNA"],
                    "PROVINCIA": attributes["PROVINCIA"],
                    "CUT_PROV": attributes["CUT_PROV"],
                },
                "geometry": convert_geometry(feature["geometry"]["rings"]),
            }
        )
    features.sort(key=lambda item: item["properties"]["CUT_COM"])
    return {"type": "FeatureCollection", "features": features}


# ---------------------------------------------------------------------------
# Provenance and serialisation
# ---------------------------------------------------------------------------

def build_provenance(
    *,
    endpoint: str,
    query_sha256: str,
    retrieved_at: str,
    cutoff_at: str,
    record_count: int,
    expected_record_count: int,
    complete: bool,
    exceeded_transfer_limit: bool,
    raw_path: str,
    raw_sha256: str,
    derived_path: str,
    derived_sha256: str,
) -> dict:
    """Build the first real row of the Bases traceability table."""
    return {
        "source_id": "mop-idechile-dpa-comunas",
        "provider": "MOP - IDE Chile (SERVICIO_DPA, capa Comunas)",
        "endpoint": endpoint,
        "query_sha256": query_sha256,
        "requested_srid": REQUESTED_SRID,
        "source_wkid": SOURCE_WKID,
        "source_format": "Esri JSON",
        "derived_format": "GeoJSON (RFC 7946)",
        "retrieved_at": retrieved_at,
        "cutoff_at": cutoff_at,
        "record_count": record_count,
        "expected_record_count": expected_record_count,
        "record_count_matches_expected": record_count == expected_record_count,
        "complete": complete,
        "exceeded_transfer_limit": exceeded_transfer_limit,
        "raw_path": raw_path,
        "raw_sha256": raw_sha256,
        "derived_path": derived_path,
        "derived_sha256": derived_sha256,
        "attribution": ATTRIBUTION,
        "restriction": RESTRICTION,
        "restriction_source": RESTRICTION_SOURCE,
        "notes": NOTES,
    }


def dumps_geojson(collection: dict) -> bytes:
    """Serialise the derived asset deterministically with a trailing newline."""
    text = json.dumps(collection, ensure_ascii=False, separators=(",", ":"))
    return (text + "\n").encode("utf-8")


def dumps_provenance(provenance: dict) -> bytes:
    """Serialise the provenance record for human and machine review."""
    text = json.dumps(provenance, ensure_ascii=False, indent=2)
    return (text + "\n").encode("utf-8")


def geometry_types(collection: dict) -> dict:
    """Count the distinct geometry types in a FeatureCollection."""
    counts: dict = {}
    for feature in collection["features"]:
        name = feature["geometry"]["type"]
        counts[name] = counts.get(name, 0) + 1
    return counts


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------

def main(argv=None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Fetch and convert, print the summary and write nothing",
    )
    parser.add_argument(
        "--allow-incomplete",
        action="store_true",
        help="Deliberately write outputs even if the response is truncated or the count differs",
    )
    args = parser.parse_args(argv)

    root = repository_root()
    raw = fetch_query()
    payload = parse_payload(raw)
    validate_payload(payload)

    completeness = check_completeness(payload)
    collection = esri_to_geojson(payload)

    retrieved_at = dt.datetime.now(dt.UTC).replace(microsecond=0).isoformat()
    raw_relpath = (
        f"{RAW_DIR_RELPATH}/valparaiso-comunas-{retrieved_at[:10].replace('-', '')}.esri.json"
    )
    derived_bytes = dumps_geojson(collection)
    raw_sha256 = hashlib.sha256(raw).hexdigest()
    derived_sha256 = hashlib.sha256(derived_bytes).hexdigest()

    provenance = build_provenance(
        endpoint=QUERY_URL,
        query_sha256=hashlib.sha256(QUERY_URL.encode("utf-8")).hexdigest(),
        retrieved_at=retrieved_at,
        cutoff_at=retrieved_at,
        record_count=completeness["record_count"],
        expected_record_count=EXPECTED_RECORD_COUNT,
        complete=completeness["complete"],
        exceeded_transfer_limit=completeness["exceeded_transfer_limit"],
        raw_path=raw_relpath,
        raw_sha256=raw_sha256,
        derived_path=DERIVED_RELPATH,
        derived_sha256=derived_sha256,
    )
    provenance_bytes = dumps_provenance(provenance)

    print("source endpoint:", QUERY_URL)
    print("retrieved_at:", retrieved_at, "(cutoff_at is identical: the service publishes no cutoff)")
    print("record_count:", completeness["record_count"], "expected:", EXPECTED_RECORD_COUNT,
          "matches:", completeness["record_count_matches_expected"])
    print("exceeded_transfer_limit:", completeness["exceeded_transfer_limit"],
          "complete:", completeness["complete"])
    print("features converted:", len(collection["features"]), "geometry types:", geometry_types(collection))
    print("derived:", DERIVED_RELPATH, derived_sha256)
    print("raw:", raw_relpath, raw_sha256)
    print("provenance:", PROVENANCE_RELPATH)

    if not completeness["complete"] and not args.allow_incomplete:
        print(
            "INCOMPLETE response (record_count "
            f"{completeness['record_count']} != {EXPECTED_RECORD_COUNT} or truncated); "
            "refusing to write. Pass --allow-incomplete to override deliberately.",
            file=sys.stderr,
        )
        return 1

    if args.dry_run:
        print("Dry run: fetched and converted, nothing written.")
        return 0

    raw_abs = root / raw_relpath
    raw_abs.parent.mkdir(parents=True, exist_ok=True)
    raw_abs.write_bytes(raw)

    derived_abs = root / DERIVED_RELPATH
    derived_abs.parent.mkdir(parents=True, exist_ok=True)
    derived_abs.write_bytes(derived_bytes)

    provenance_abs = root / PROVENANCE_RELPATH
    provenance_abs.parent.mkdir(parents=True, exist_ok=True)
    provenance_abs.write_bytes(provenance_bytes)

    print("Wrote raw, derived and provenance files.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
