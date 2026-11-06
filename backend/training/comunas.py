"""Comuna name -> Chilean CUT, for the one source that only names the comuna.

The CONAF event registry identifies a comuna by NAME ("Vichuquen"), never by the
CUT the rest of FireWatch keys on (`emas.com_ia_id`, the geo contracts, the panel
key). The mapping therefore has to come from a declared data file, not from a
guess and not from the network: the panel has to be reproducible offline, and a
wrong CUT silently attributes one comuna's fires to another.

Two rules are load-bearing here:

1. Resolution folds accents and case rather than carrying an alias list. The
   official DPA layer writes "Constitución", "Curicó", "Río Claro" and this source
   writes "Constitucion", "Curico", "Rio Claro". An alias table would grow with
   every new spelling; folding is one rule that covers them all, and it does not
   need to know which source it is reading.
2. An unresolved name is NOT given a nearby code. Callers keep the source name and
   report it, because "-1" or "07000" would look like a real commune to every
   reader downstream including the model's join.
"""

from __future__ import annotations

import hashlib
import json
import unicodedata
from dataclasses import dataclass
from pathlib import Path


@dataclass(frozen=True)
class Comuna:
    """One row of the official administrative division."""

    #: The 5-digit Chilean CUT, for example "07101".
    cut: str
    #: The official spelling, with accents, as the DPA layer publishes it.
    name: str
    provincia: str


def normalise(name: str) -> str:
    """Fold a comuna name to the key resolution compares.

    Accents are stripped after NFKD decomposition, case is folded and runs of
    whitespace collapse, so "Río Claro", "RIO CLARO" and "Rio  Claro" are one key.
    Nothing else is normalised: an abbreviation ("S. Clemente") is still a
    different name, and guessing which comuna it means is exactly what this module
    refuses to do.
    """
    decomposed = unicodedata.normalize("NFKD", name)
    stripped = "".join(char for char in decomposed if not unicodedata.combining(char))
    return " ".join(stripped.casefold().split())


@dataclass(frozen=True)
class ComunaMap:
    """Name -> CUT resolution for one region."""

    region_cut: str
    region_name: str
    comunas: tuple[Comuna, ...]
    provenance: dict

    def __post_init__(self) -> None:
        keys = [normalise(comuna.name) for comuna in self.comunas]
        if len(set(keys)) != len(keys):
            raise ValueError(
                "comunas: the mapping declares two names that fold to one key; "
                "resolution would silently pick the first, so the file is refused"
            )

    def resolve(self, name: str) -> Comuna | None:
        """Return the comuna the name refers to, or None when it does not resolve."""
        key = normalise(name)
        for comuna in self.comunas:
            if normalise(comuna.name) == key:
                return comuna
        return None


def load_mapping(path: Path) -> ComunaMap:
    """Read the committed mapping file.

    A malformed mapping is a hard failure: resolution is the join the whole panel
    rests on, so a missing or unreadable file must stop the run instead of
    degrading every row to "unresolved".
    """
    try:
        with path.open(encoding="utf-8") as handle:
            document = json.load(handle)
    except (OSError, json.JSONDecodeError) as error:
        raise ValueError(f"{path}: the comuna mapping cannot be read ({error})") from error
    entries = document.get("entries")
    if not isinstance(entries, list) or not entries:
        raise ValueError(
            f"{path}: no 'entries' list; refusing to resolve comunas without a mapping"
        )
    comunas = tuple(
        Comuna(cut=str(entry["cut"]), name=str(entry["name"]), provincia=str(entry["provincia"]))
        for entry in entries
    )
    region = document.get("region") or {}
    return ComunaMap(
        region_cut=str(region.get("cut", "")),
        region_name=str(region.get("name", "")),
        comunas=comunas,
        provenance=document.get("provenance") or {},
    )


def entries_checksum(comunas: tuple[Comuna, ...]) -> str:
    """Hash the resolved rows, so the committed mapping is auditable by itself.

    The HTTP response it came from is not committed and would be a moving target
    anyway; hashing the canonical serialisation of the rows means a reader can
    prove the file was not edited after the retrieval date it declares.
    """
    rows = [
        {"cut": comuna.cut, "name": comuna.name, "provincia": comuna.provincia}
        for comuna in comunas
    ]
    canonical = json.dumps(rows, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
    return hashlib.sha256(canonical.encode("utf-8")).hexdigest()
