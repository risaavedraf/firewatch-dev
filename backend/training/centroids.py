"""Comuna centroids for region 05, derived from geometry the repository already has.

The meteorology sources in `meteo.py` answer for a POINT, not for a polygon, so the
season panel needs one representative coordinate per comuna. That coordinate is
DATA: it is derived once from the official DPA layer, committed, and read offline
from then on. Nothing here fetches at runtime, and `fetch_comunas.py` is the script
that retrieved the underlying geometry.

WHICH GEOMETRY. `backend/contracts/gen/geo/valparaiso-comunas.geojson` -- the
tracked GeoJSON that `backend/scripts/fetch_comunas.py` produced from the official
layer, with its own provenance next door (URL, exact query, sha256 of the response
and of the derived file). Deriving from the tracked artifact rather than from the
git-ignored `data/raw/geo/` Esri response means a reader can rerun this derivation
from a clean clone. Both files are the same 38 features with
`outSR=4326`, so the coordinates here are WGS84 degrees, the frame both meteo
sources take.

WHAT THE CENTROID IS, EXACTLY. The area-weighted centroid of the feature's polygon
parts, computed in the EPSG:4326 frame with the shoelace formula:

* every ring contributes `signed_area * ring_centroid` and the sum is divided by the
  total signed area, so holes (clockwise, negative area in RFC 7946) are removed from
  the part that contains them instead of inflating it;
* parts are weighted by their own area, so an archipelago comuna lands near its
  largest island rather than midway between two of them;
* the frame is treated as planar, and the weights are square degrees rather than
  square metres. Across one region at 32-34 S the resulting error is far below the
  sources' own grid spacing (NASA POWER is 0.5 x 0.625 degrees, Open-Meteo snaps to
  its own grid), which is the only thing the number is used for. It is NOT a
  geodesic centroid and must not be published as one.

The one case worth naming out loud: Isla de Pascua and Juan Fernandez are
archipelagos, so their centroid is a point between islands -- water. That is fine
for a gridded reanalysis lookup and it is why the method is recorded in the file
instead of being left implicit.

Refusals, same discipline as the rest of the package:

* a feature whose `CUT_COM` is not in region 05, or a repeated CUT, is an error
  rather than a row: a wrong code silently attributes one comuna's weather to
  another;
* a feature with no usable polygon is an error, never a guessed city coordinate;
* a committed file whose entries do not match `entries_sha256` is refused.
"""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import sys
from dataclasses import dataclass
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]

#: The tracked geometry this derivation reads. `fetch_comunas.py` wrote it.
DEFAULT_GEOJSON = REPO_ROOT / "backend/contracts/gen/geo/valparaiso-comunas.geojson"
#: The provenance `fetch_comunas.py` wrote next to it; copied into the output.
DEFAULT_SOURCE_PROVENANCE = (
    REPO_ROOT / "backend/contracts/gen/geo/valparaiso-comunas.provenance.json"
)
#: The committed product. Data the season builder reads, not code.
DEFAULT_PATH = REPO_ROOT / "backend/training/data/comunas-valparaiso-centroids.json"

#: Region 05 is Valparaiso. Its DPA layer has 38 comunas, including the two island
#: comunas (Isla de Pascua, Juan Fernandez) and excluding the Antarctic territory,
#: which is not in this layer at all.
REGION_CUT = "05"
REGION_NAME = "Valparaiso"
EXPECTED_RECORD_COUNT = 38

#: Coordinates are committed with six decimals: about 0.11 m, far finer than either
#: source's grid, and it keeps the checksum stable across float printing.
COORDINATE_DECIMALS = 6

METHOD = (
    "area-weighted planar centroid of the feature's polygon parts in the EPSG:4326 "
    "frame (shoelace formula, holes subtracted, parts weighted by their own signed "
    "area). Not a geodesic centroid; the error is far below both sources' grid spacing."
)


class CentroidError(Exception):
    """A failure the operator must fix; the CLI prints it instead of a traceback."""


@dataclass(frozen=True)
class Centroid:
    """One comuna's representative coordinate, in WGS84 degrees."""

    cut: str
    name: str
    provincia: str
    latitude: float
    longitude: float
    #: The area the weighting used, in square degrees. Recorded so the derivation is
    #: auditable, never consumed by the model.
    area_deg2: float
    #: How many polygon parts the comuna has. An archipelago centroid is a point
    #: between islands, and this is where a reader sees that.
    parts: int

    def as_entry(self) -> dict:
        """The committed shape, so the checksum and the file cannot drift apart."""
        return {
            "cut": self.cut,
            "name": self.name,
            "provincia": self.provincia,
            "latitude": round(self.latitude, COORDINATE_DECIMALS),
            "longitude": round(self.longitude, COORDINATE_DECIMALS),
            "area_deg2": round(self.area_deg2, COORDINATE_DECIMALS),
            "parts": self.parts,
        }


@dataclass(frozen=True)
class CentroidMap:
    """The committed centroids plus the provenance they were derived under."""

    region_cut: str
    region_name: str
    entries: tuple[Centroid, ...]
    method: str
    provenance: dict

    def centroid(self, cut: str) -> Centroid:
        """Return the comuna's centroid, or fail naming the code."""
        for entry in self.entries:
            if entry.cut == cut:
                return entry
        raise CentroidError(
            f"no centroid for CUT {cut!r} in the committed mapping; it declares "
            f"{len(self.entries)} comunas of region {self.region_cut}"
        )

    def by_name(self, name: str) -> Centroid | None:
        """Return the comuna with this official name, folded, or None."""
        from comunas import normalise

        key = normalise(name)
        return next((entry for entry in self.entries if normalise(entry.name) == key), None)


def _ring_area_and_centroid(ring: list) -> tuple[float, float, float]:
    """Shoelace area and centroid of one closed ring, in the plane of its coordinates.

    Returns `(signed_area, centroid_x, centroid_y)` with x = longitude and y =
    latitude. A hole ring comes back with a negative area under RFC 7946
    orientation, which is what makes the weighted sum subtract it.
    """
    if len(ring) < 4:
        raise CentroidError(f"a polygon ring has {len(ring)} points; at least 4 are needed")
    area = 0.0
    x_sum = 0.0
    y_sum = 0.0
    for index in range(len(ring) - 1):
        x1, y1 = float(ring[index][0]), float(ring[index][1])
        x2, y2 = float(ring[index + 1][0]), float(ring[index + 1][1])
        cross = x1 * y2 - x2 * y1
        area += cross
        x_sum += (x1 + x2) * cross
        y_sum += (y1 + y2) * cross
    area /= 2.0
    if area == 0.0:
        return 0.0, 0.0, 0.0
    return area, x_sum / (6.0 * area), y_sum / (6.0 * area)


def geometry_centroid(geometry: dict) -> tuple[float, float, float, int]:
    """Area-weighted centroid of a GeoJSON Polygon or MultiPolygon.

    Returns `(longitude, latitude, area_deg2, parts)`. Holes are subtracted by their
    own signed area rather than ignored, and every part is weighted by its area.
    A geometry whose signed areas cancel out (a ring wound the other way, a
    degenerate shape) is refused instead of producing a point in the ocean.
    """
    kind = geometry.get("type")
    if kind == "Polygon":
        polygons = [geometry.get("coordinates") or []]
    elif kind == "MultiPolygon":
        polygons = geometry.get("coordinates") or []
    else:
        raise CentroidError(f"unsupported geometry type {kind!r}; expected Polygon or MultiPolygon")
    total_area = 0.0
    x_sum = 0.0
    y_sum = 0.0
    parts = 0
    for polygon in polygons:
        for ring_index, ring in enumerate(polygon):
            area, x, y = _ring_area_and_centroid(ring)
            if area == 0.0:
                continue
            if ring_index == 0:
                parts += 1
            total_area += area
            x_sum += area * x
            y_sum += area * y
    if total_area <= 0.0 or parts == 0:
        raise CentroidError(
            f"the geometry has no outer ring with positive area (total signed area "
            f"{total_area}); a centroid from it would be a guess"
        )
    return x_sum / total_area, y_sum / total_area, total_area, parts


def entries_from_geojson(path: Path) -> tuple[Centroid, ...]:
    """Derive one centroid per feature of the DPA layer, sorted by CUT."""
    try:
        with path.open(encoding="utf-8") as handle:
            document = json.load(handle)
    except (OSError, json.JSONDecodeError) as error:
        raise CentroidError(f"cannot read the geometry {path} ({error})") from error
    if document.get("type") != "FeatureCollection":
        raise CentroidError(f"{path}: expected a GeoJSON FeatureCollection")
    features = document.get("features")
    if not isinstance(features, list) or not features:
        raise CentroidError(f"{path}: no features, so there is no geometry to derive from")
    entries: list[Centroid] = []
    for feature in features:
        properties = feature.get("properties") or {}
        cut = str(properties.get("CUT_COM") or "").strip()
        name = str(properties.get("COMUNA") or "").strip()
        provincia = str(properties.get("PROVINCIA") or "").strip()
        if not cut or not name:
            raise CentroidError(f"{path}: a feature has no CUT_COM or no COMUNA")
        if not cut.startswith(REGION_CUT):
            raise CentroidError(
                f"{cut} ({name}) is not in region {REGION_CUT}; the layer is not the one "
                f"this mapping declares"
            )
        longitude, latitude, area, parts = geometry_centroid(feature.get("geometry") or {})
        entries.append(
            Centroid(
                cut=cut,
                name=name,
                provincia=provincia,
                latitude=latitude,
                longitude=longitude,
                area_deg2=area,
                parts=parts,
            )
        )
    entries.sort(key=lambda entry: entry.cut)
    cuts = [entry.cut for entry in entries]
    if len(set(cuts)) != len(cuts):
        repeated = sorted({cut for cut in cuts if cuts.count(cut) > 1})
        raise CentroidError(f"{path}: repeated CUT(s) {', '.join(repeated)}; refusing to pick one")
    return tuple(entries)


def entries_checksum(entries: tuple[Centroid, ...]) -> str:
    """Hash the committed rows, so the file is auditable without the geometry.

    Same rule as `comunas.entries_checksum`, over the rounded entries exactly as the
    file writes them: sha256 of `json.dumps(entries, ensure_ascii=False,
    sort_keys=True, separators=(',', ':'))`.
    """
    rows = [entry.as_entry() for entry in entries]
    canonical = json.dumps(rows, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
    return hashlib.sha256(canonical.encode("utf-8")).hexdigest()


def sha256_of(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(65536), b""):
            digest.update(chunk)
    return digest.hexdigest()


def build_provenance(
    *,
    geojson: Path,
    entries: tuple[Centroid, ...],
    source_provenance: dict | None,
    derived_at: str,
) -> dict:
    """Record where the geometry came from, so the data file stands on its own.

    The URL and the exact query are copied from the provenance the fetch script
    wrote, not retyped: they are the same strings that produced the layer, and a
    transcription here would be a second, unverifiable source of truth.
    """
    source = source_provenance or {}
    return {
        "authority": (
            "Official DPA (Division Politico Administrativa) layer of IDE Chile, retrieved "
            "by backend/scripts/fetch_comunas.py; the same layer data/comunas-maule-cut.json "
            "comes from."
        ),
        "url": source.get("endpoint", ""),
        "query": (
            "where=CUT_REG='05'&outFields=CUT_COM,COMUNA,PROVINCIA,CUT_PROV"
            "&returnGeometry=true&outSR=4326&f=json"
        ),
        "source_srid": 4326,
        "retrieved_at": str(source.get("retrieved_at", "")),
        "retrieved_at_note": (
            "The retrieval date of the GEOMETRY, copied from "
            "backend/contracts/gen/geo/valparaiso-comunas.provenance.json. The centroids "
            "were derived from that file on derived_at; nothing was fetched to build this "
            "file."
        ),
        "derived_at": derived_at,
        "derived_from": {
            "path": _display(geojson),
            "sha256": sha256_of(geojson),
            "record_count": len(entries),
        },
        "method": METHOD,
        "record_count": len(entries),
        "entries_sha256": entries_checksum(entries),
        "entries_sha256_rule": (
            "sha256 of json.dumps(entries, ensure_ascii=False, sort_keys=True, "
            "separators=(',', ':')) over the entries exactly as this file writes them "
            "(coordinates rounded to 6 decimals). Recompute it with tests/test_centroids.py; "
            "it is what makes this file auditable without the geometry."
        ),
        "attribution": str(source.get("attribution", "")),
        "restriction": str(source.get("restriction", "")),
        "restriction_source": str(source.get("restriction_source", "")),
    }


def build_document(
    *,
    entries: tuple[Centroid, ...],
    provenance: dict,
) -> dict:
    """The committed file: data, the method and the provenance, in that order."""
    return {
        "_comment": [
            "Comuna centroid for region 05 (Valparaiso), one representative point per CUT.",
            "This is DATA, not source code: meteo.py reads it for the point each source",
            "answers for, and never fetches a coordinate at runtime. Derived from the",
            "tracked DPA GeoJSON by backend/training/centroids.py; rerun that script to",
            "reproduce the file. Two of the 38 comunas are archipelagos, so their centroid",
            "is a point between islands -- water, which is fine for a gridded lookup and is",
            "why 'method' and 'parts' are recorded per entry.",
        ],
        "region": {"cut": REGION_CUT, "name": REGION_NAME},
        "method": METHOD,
        "entries": [entry.as_entry() for entry in entries],
        "provenance": provenance,
    }


def load_centroids(path: Path | None = None) -> CentroidMap:
    """Read the committed centroids and verify them against their own checksum.

    A malformed file, or one whose entries were edited after the checksum was
    written, is a hard failure: every season row is anchored on one of these
    coordinates, and a silently wrong point attributes another comuna's weather.
    """
    path = path or DEFAULT_PATH
    try:
        with path.open(encoding="utf-8") as handle:
            document = json.load(handle)
    except (OSError, json.JSONDecodeError) as error:
        raise CentroidError(f"{path}: the centroid mapping cannot be read ({error})") from error
    raw_entries = document.get("entries")
    if not isinstance(raw_entries, list) or not raw_entries:
        raise CentroidError(f"{path}: no 'entries' list; refusing to guess coordinates")
    try:
        entries = tuple(
            Centroid(
                cut=str(entry["cut"]),
                name=str(entry["name"]),
                provincia=str(entry.get("provincia", "")),
                latitude=float(entry["latitude"]),
                longitude=float(entry["longitude"]),
                area_deg2=float(entry.get("area_deg2", 0.0)),
                parts=int(entry.get("parts", 0)),
            )
            for entry in raw_entries
        )
    except (KeyError, TypeError, ValueError) as error:
        raise CentroidError(
            f"{path}: an entry is missing a CUT, a name or a coordinate ({error})"
        ) from error
    provenance = document.get("provenance") or {}
    declared = provenance.get("entries_sha256")
    actual = entries_checksum(entries)
    if declared and declared != actual:
        raise CentroidError(
            f"{path}: entries hash to {actual} but the provenance declares {declared}; the "
            f"file was edited after its checksum was written, so it is refused"
        )
    region = document.get("region") or {}
    return CentroidMap(
        region_cut=str(region.get("cut", "")),
        region_name=str(region.get("name", "")),
        entries=entries,
        method=str(document.get("method", "")),
        provenance=provenance,
    )


def write_document(path: Path, document: dict) -> None:
    """Write the file with the package's JSON style: utf-8, two-space indent, LF."""
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8", newline="") as handle:
        json.dump(document, handle, ensure_ascii=False, indent=2)
        handle.write("\n")


def _display(path: Path) -> str:
    """Show a path relative to the repository when it lives inside it."""
    try:
        return path.resolve().relative_to(REPO_ROOT).as_posix()
    except ValueError:
        return path.as_posix()


def _read_json(path: Path) -> dict | None:
    try:
        with path.open(encoding="utf-8") as handle:
            document = json.load(handle)
    except (OSError, json.JSONDecodeError):
        return None
    return document if isinstance(document, dict) else None


def main(argv: list[str] | None = None) -> int:
    """Derive the centroids from the tracked geometry and write the data file."""
    parser = argparse.ArgumentParser(
        description=(
            "Derive comuna centroids for region 05 from the tracked DPA GeoJSON and write "
            "them as data with a provenance block. Offline: it reads files, it fetches nothing."
        )
    )
    parser.add_argument("--geojson", type=Path, default=DEFAULT_GEOJSON, help="source geometry")
    parser.add_argument(
        "--source-provenance",
        type=Path,
        default=DEFAULT_SOURCE_PROVENANCE,
        help="the provenance fetch_comunas.py wrote next to the geometry",
    )
    parser.add_argument("--out", type=Path, default=DEFAULT_PATH, help="destination data file")
    parser.add_argument(
        "--derived-at",
        default=None,
        help="override the derivation timestamp (default: now, UTC)",
    )
    arguments = parser.parse_args(argv)
    try:
        entries = entries_from_geojson(arguments.geojson)
    except CentroidError as error:
        print(f"error: {error}", file=sys.stderr)
        return 2
    provenance = build_provenance(
        geojson=arguments.geojson,
        entries=entries,
        source_provenance=_read_json(arguments.source_provenance),
        derived_at=arguments.derived_at
        or dt.datetime.now(dt.UTC).replace(microsecond=0).isoformat().replace("+00:00", "Z"),
    )
    write_document(arguments.out, build_document(entries=entries, provenance=provenance))
    print(f"comunas: {len(entries)} of region {REGION_CUT}")
    print(f"entries sha256: {provenance['entries_sha256']}")
    print(f"geometry sha256: {provenance['derived_from']['sha256']}")
    print(f"out: {_display(arguments.out)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
