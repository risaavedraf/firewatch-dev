"""Download the CONAF event-registry sample this package is built against.

The sample is the open FONDEF repository's `Base de datos incendios forestales
Geprif` CSV: one season of one region (Maule, 2018-2019), 951 events, 91 columns.
It is a SCHEMA AND PIPELINE FIXTURE, not a training set -- the project's training
label is the access-gated CONAF product (DOI 10.71578/UXAUN5) that nobody here can
download, and this file covers neither the seasons nor the region the product needs.

Three behaviours are deliberate:

* The download is verified against a documented sha256 and a byte count before it
  is moved into place. An unverified CSV is not committed anywhere and is not read
  by `assemble.py`, so a changed upstream file has to be re-verified on purpose.
* An existing file is never overwritten silently. Re-downloading over a verified
  file is how a checksum stops meaning anything; `--force` is the explicit opt-in.
* It prints the licence and the citation the licence requires, every run. CC BY 4.0
  obliges attribution, and a script that fetches the data is the natural place to
  hand it over.

Standard library only: a clean clone needs no installation to fetch the sample.
"""

from __future__ import annotations

import argparse
import hashlib
import shutil
import sys
import urllib.error
import urllib.request
from dataclasses import dataclass
from pathlib import Path

#: The exact file this package was built against. Do not edit these three values
#: without re-reading the new file: the sha256 and the byte count are what makes a
#: silently changed upstream visible, and the record counts in the fixture
#: provenance are derived from this version.
SOURCE_URL = (
    "https://raw.githubusercontent.com/cvidalmsu/FONDEF-IDEA-/main/"
    "Base%20de%20datos%20incendios%20forestales%20Geprif%202024-12-17%2008.31.csv"
)
SOURCE_SHA256 = "e5b179b95802e01fc497a1c0a259823c3edd501b6716980d91383a1b1c2e0456"
SOURCE_BYTES = 616347

LICENSE = "CC BY 4.0 (Creative Commons Attribution 4.0 International)"

#: The attribution the licence requires. The repository's README gives a citation
#: whose DOI is elided as "https://doi.org/..."; it is reproduced as published and
#: the DOI is NOT guessed here. The citable, restricted full registry is
#: DOI 10.71578/UXAUN5.
CITATION = (
    "Vidal-Silva, C. et al. (2025). A Ground-Reported Dataset of Wildfire Occurrence and "
    "Damage in Chile (1985-2023). MDPI Data, 10(2), 115. "
    "Ledger URL as published: https://github.com/cvidalmsu/FONDEF-IDEA-"
)

#: Where the sample lands. Git-ignored (backend/training/.gitignore): the raw file is
#: regenerable from this URL plus the checksum, and the repository carries a trimmed
#: fixture instead of the whole thing.
DEST_RELPATH = "backend/training/data/raw/conaf-maule-2018-2019.csv"

USER_AGENT = "FireWatch/1.0 (academic wildfire training pipeline; stdlib urllib)"
TIMEOUT_SECONDS = 120
CHUNK_BYTES = 1 << 16


class FetchError(Exception):
    """A download the operator must look at; the CLI prints it instead of a traceback."""


@dataclass(frozen=True)
class Fetched:
    path: Path
    sha256: str
    bytes: int


def repository_root() -> Path:
    """Resolve the repository root from this file and prove it looks right."""
    root = Path(__file__).resolve().parents[2]
    marker = root / "backend" / "services" / "risk" / "go.mod"
    if not marker.is_file():
        raise FetchError(
            f"refusing to run: {root} does not look like the FireWatch repository root "
            f"(missing {marker})"
        )
    return root


def sha256_of(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(CHUNK_BYTES), b""):
            digest.update(chunk)
    return digest.hexdigest()


def verify(
    path: Path,
    *,
    expected_sha256: str | None = None,
    expected_bytes: int | None = None,
) -> str:
    """Prove a file is the documented version, and return its sha256.

    Both halves are checked: a sha256 match alone would accept a truncated file only
    if the hash also matched, which cannot happen, but the byte count tells an
    operator *how* it went wrong without making them compare hex strings.

    The expected values are read at call time rather than bound as defaults, so a
    test can point the same code at a small file without this function growing a
    second, untested branch.
    """
    expected_sha256 = expected_sha256 or SOURCE_SHA256
    expected_bytes = SOURCE_BYTES if expected_bytes is None else expected_bytes
    size = path.stat().st_size
    digest = sha256_of(path)
    if size != expected_bytes:
        raise FetchError(
            f"{path} is {size} bytes, expected {expected_bytes}; this is not the documented "
            f"version of the sample"
        )
    if digest != expected_sha256:
        raise FetchError(
            f"{path} has sha256 {digest}, expected {expected_sha256}; the upstream file changed "
            f"or the download was corrupted. Re-read it before trusting it, then update "
            f"SOURCE_SHA256, SOURCE_BYTES and the fixture provenance together"
        )
    return digest


def _urlopen(url: str, timeout: int):
    """Open the URL. Kept as one function so the tests can replace it."""
    # urllib raises the S310 audit hint here; that rule is not part of the enabled
    # set, so this is a note rather than a suppression: url is the frozen https
    # SOURCE_URL (or the opener seam the tests replace), never network input.
    request = urllib.request.Request(url, headers={"User-Agent": USER_AGENT})
    return urllib.request.urlopen(request, timeout=timeout)


def download(destination: Path, *, url: str = SOURCE_URL, opener=_urlopen) -> Fetched:
    """Download to a temporary sibling, verify, then move into place.

    Nothing is written at `destination` until the bytes are verified, so a failed or
    interrupted download cannot leave a half file where a verified one is expected.
    The temporary file is left in place on failure and named in the error, rather
    than deleted: an operator may want to look at what arrived.
    """
    destination.parent.mkdir(parents=True, exist_ok=True)
    partial = destination.with_name(destination.name + ".part")
    try:
        with opener(url, TIMEOUT_SECONDS) as response, partial.open("wb") as handle:
            shutil.copyfileobj(response, handle, CHUNK_BYTES)
    except (urllib.error.URLError, OSError) as error:
        raise FetchError(f"cannot download {url} ({error})") from error
    try:
        digest = verify(partial)
    except FetchError as error:
        raise FetchError(f"{error} (the unverified download is at {partial})") from error
    partial.replace(destination)
    return Fetched(path=destination, sha256=digest, bytes=destination.stat().st_size)


def fetch(
    destination: Path,
    *,
    force: bool = False,
    url: str = SOURCE_URL,
    opener=_urlopen,
) -> Fetched:
    """Fetch the sample unless a verified copy is already there.

    An existing file is re-verified rather than trusted: if it is still the
    documented version there is nothing to do, and if it is not, the operator asked
    for a re-download or the file is not what the pipeline expects. Overwriting either
    way without `force` is refused.
    """
    if destination.exists() and not force:
        digest = verify(destination)
        return Fetched(path=destination, sha256=digest, bytes=destination.stat().st_size)
    return download(destination, url=url, opener=opener)


def notice(destination: Path | None = None) -> list[str]:
    """The licence, the citation and what the file is allowed to be used for."""
    lines = [
        f"source:  {SOURCE_URL}",
        "sample:  CONAF per-event registry, region Maule, season 2018-2019 (951 events, 91 columns)",
        f"licence: {LICENSE}",
        f"cite:    {CITATION}",
        (
            "note:    the source README publishes that citation with its DOI elided as "
            "'https://doi.org/...'; it is reproduced as published and the DOI is not guessed. "
            "The citable full registry (19 seasons) is DOI 10.71578/UXAUN5 and is access-gated."
        ),
        (
            "note:    this file is a schema and pipeline fixture, NOT a training set. It is one "
            "season of one region and the project's training label is the gated CONAF product."
        ),
        f"sha256:  {SOURCE_SHA256} ({SOURCE_BYTES} bytes)",
    ]
    if destination is not None:
        lines.append(f"dest:    {destination}")
    return lines


def main(argv: list[str] | None = None, *, opener=_urlopen) -> int:
    """Download the sample and print the attribution it requires."""
    parser = argparse.ArgumentParser(
        description=(
            "Download the CONAF event-registry sample into the git-ignored data/raw/ "
            "directory, verifying the documented checksum. Prints the licence and citation."
        )
    )
    parser.add_argument(
        "--dest",
        type=Path,
        default=None,
        help=f"destination file (default: <repo>/{DEST_RELPATH})",
    )
    parser.add_argument(
        "--force",
        action="store_true",
        help="download even when a verified copy already exists (default: refuse)",
    )
    parser.add_argument(
        "--print-license",
        action="store_true",
        help="print the licence and citation and exit without downloading",
    )
    arguments = parser.parse_args(argv)
    try:
        destination = arguments.dest or (repository_root() / DEST_RELPATH)
    except FetchError as error:
        print(f"error: {error}", file=sys.stderr)
        return 2
    if arguments.print_license:
        for line in notice():
            print(line)
        return 0
    try:
        fetched = fetch(destination, force=arguments.force, opener=opener)
    except FetchError as error:
        print(f"error: {error}", file=sys.stderr)
        return 2
    for line in notice(fetched.path):
        print(line)
    print(f"verified: sha256 {fetched.sha256}, {fetched.bytes} bytes")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
