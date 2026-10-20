"""Verify documentation structure only; never import or execute application code."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import unicodedata
from urllib.parse import unquote, urlsplit

HERE = Path(__file__).resolve().parent
ROOT = HERE.parent.parent
MANIFEST = HERE / "source-snapshot.json"


def forbidden(path):
    return any(part.startswith(".env") for part in path.parts) or path.name == "firewatch-realm.json"


def generated(path):
    return "__pycache__" in path.parts or ".pytest_cache" in path.parts or path.suffix == ".pyc"


def source_bytes(relative):
    path = (ROOT / relative).resolve()
    if not path.is_relative_to(ROOT) or forbidden(path):
        raise ValueError(f"Forbidden source: {relative}")
    # Line endings belong to the checkout, not to the source: a CRLF working tree
    # and an LF one hold the same file. Hash the normalised bytes so the manifest
    # describes the repository instead of one machine.
    return path.read_bytes().replace(b"\r\n", b"\n")


def headings(text):
    counts, result = {}, set()
    in_fence = False
    for line in text.splitlines():
        if line.startswith("```"):
            in_fence = not in_fence
        if in_fence or not re.match(r"^#{1,6} ", line):
            continue
        label = re.sub(r"^#{1,6} +", "", line).strip().rstrip("#").strip()
        label = re.sub(r"\[([^]]+)\]\([^)]*\)", r"\1", label).replace("`", "")
        slug = "".join(c for c in label.lower() if c in "-_ " or unicodedata.category(c)[0] in "LN").replace(" ", "-")
        n = counts.get(slug, 0)
        counts[slug] = n + 1
        result.add(slug if n == 0 else f"{slug}-{n}")
    return result


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--snapshot", action="store_true", help="Write the authorized source manifest inside this directory")
    parser.add_argument("--list-citations", action="store_true", help="Print each resolved citation and inclusive interval")
    args = parser.parse_args()
    errors = []
    ledger = []
    index = (HERE / "README.md").read_text(encoding="utf-8")
    for match in re.finditer(r"^\| `([^`]+)` \| ([^|]+) \| (Explicado|Parcial|Contexto|Excluido) \|", index, re.M):
        name, interval, status = match.groups()
        ledger.append((name, interval.strip(), status))
    names = [row[0] for row in ledger]
    if len(names) != len(set(names)):
        errors.append("Duplicate ledger paths")
    # Enumerate names only for excluded files; never open secrets or lockfiles.
    actual = {p.relative_to(ROOT).as_posix() for p in ROOT.rglob("*") if p.is_file() and not p.is_relative_to(HERE) and not generated(p)}
    for name in sorted(actual - set(names)):
        errors.append(f"Uninventoried file: {name}")
    # An excluded file is recorded by name only and never opened. Its absence is
    # not a defect: .env is local, git-ignored and created by the bootstrap.
    excluded_rows = {name for name, _, status in ledger if status == "Excluido"}
    for name in sorted(set(names) - actual - excluded_rows):
        errors.append(f"Missing inventory file: {name}")
    snapshot = {}
    for name, interval, status in ledger:
        if status == "Excluido":
            if interval != "—":
                errors.append(f"Excluded file has read range: {name}")
            continue
        data = source_bytes(name)
        total = len(data.decode("utf-8").splitlines())
        if interval != f"1-{total}":
            errors.append(f"Whole-file range mismatch: {name}: {interval}, actual 1-{total}")
        snapshot[name] = {"sha256": hashlib.sha256(data).hexdigest(), "bytes": len(data), "lines": total}
    if args.snapshot:
        MANIFEST.write_text(json.dumps({"algorithm": "SHA-256", "basis": "file bytes with CRLF normalised to LF; no Git revision available", "files": snapshot}, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    elif not MANIFEST.exists():
        errors.append("Missing source manifest")
    else:
        recorded = json.loads(MANIFEST.read_text(encoding="utf-8"))["files"]
        if recorded != snapshot:
            errors.append("Source snapshot differs from recorded hashes, sizes or line totals")
    docs = sorted(HERE.glob("*.md"))
    link_count = anchor_count = citation_count = 0
    citation_pattern = re.compile(r"(?P<path>(?:services|contracts|deploy|scripts|docs)/[\w./-]+\.(?:go|sql|md|yaml|yml|py|sh)|docker-compose\.yml|go\.work|Makefile|\.dockerignore):(?P<ranges>\d+(?:-\d+)?(?:,\d+(?:-\d+)?)*)")
    resolved_citations = []
    contextual_paths = {
        "01-gateway.md": {"main.go": ["services/gateway/cmd/server/main.go"], "gateway.go": ["services/gateway/internal/handler/gateway.go"]},
        "02-jobs.md": {"api.go": ["services/jobs/internal/handler/api.go"], "security.go": ["services/jobs/internal/handler/security.go"], "store.go": ["services/jobs/internal/repository/store.go"]},
        "03-risk.md": {name: [f"services/risk/internal/{folder}/{name}"] for name, folder in (("api.go", "handler"), ("security.go", "handler"), ("store.go", "repository"), ("demo.go", "service"))},
        "06-database.md": {"main.go": [f"services/{service}/cmd/server/main.go" for service in ("jobs", "risk")], "embed.go": [f"services/{service}/migrations/embed.go" for service in ("jobs", "risk")]},
        "07-deployment.md": {"main": [f"services/{service}/cmd/server/main.go" for service in ("jobs", "risk")], "broker.go": [f"services/{service}/internal/messaging/broker.go" for service in ("jobs", "risk")]},
        "05-contracts.md": {"events.go": ["contracts/gen/events/v1/events.go"], "firewatch-v1.yaml": ["contracts/openapi/firewatch-v1.yaml"]},
    }
    for doc in docs:
        text = doc.read_text(encoding="utf-8")
        citations = [(m.group("path"), m.group("ranges")) for m in citation_pattern.finditer(text)]
        aliases = dict(contextual_paths.get(doc.name, {}))
        if doc.name in ("04-messaging.md", "07-deployment.md", "README.md", "06-database.md"):
            aliases["cmd/server/main.go"] = [f"services/{service}/cmd/server/main.go" for service in ("jobs", "risk")]
        if doc.name == "06-database.md":
            aliases["migrations/embed.go"] = [f"services/{service}/migrations/embed.go" for service in ("jobs", "risk")]
        if doc.name == "04-messaging.md":
            aliases["jobs"] = ["services/jobs/internal/messaging/broker.go"]
            aliases["risk"] = ["services/risk/internal/messaging/broker.go"]
        for short, paths in aliases.items():
            pattern = r"(?<![\w/])" + re.escape(short) + r":(\d+(?:-\d+)?(?:,\d+(?:-\d+)?)*)"
            for match in re.finditer(pattern, text):
                citations.extend((path, match.group(1)) for path in paths)
        for match in re.finditer(r"\[[^\]]*\]\(([^)]+)\)", text):
            # Ranges after links are processed separately below.
            target = urlsplit(match.group(1))
            if target.scheme or target.netloc:
                continue
            link_count += 1
            path = (doc.parent / unquote(target.path)).resolve() if target.path else doc
            if not path.is_relative_to(ROOT) or forbidden(path):
                errors.append(f"{doc.name}: forbidden link {match.group(1)}")
                continue
            if not path.is_file():
                errors.append(f"{doc.name}: missing link {match.group(1)}")
                continue
            if target.fragment:
                anchor_count += 1
                if path.suffix != ".md" or unquote(target.fragment) not in headings(path.read_text(encoding="utf-8")):
                    errors.append(f"{doc.name}: missing heading anchor {match.group(1)}")
        for match in re.finditer(r"\[[^\]]*\]\(([^)#]+)\):(\d+(?:-\d+)?(?:,\d+(?:-\d+)?)*)", text):
            path = (doc.parent / match.group(1)).resolve()
            if path.is_relative_to(ROOT):
                citations.append((path.relative_to(ROOT).as_posix(), match.group(2)))
        for name, ranges in citations:
            try:
                total = len(source_bytes(name).decode("utf-8").splitlines())
                for interval in ranges.split(","):
                    bounds = interval.split("-")
                    start, end = int(bounds[0]), int(bounds[-1])
                    citation_count += 1
                    resolved_citations.append(f"{doc.name} -> {name}:{start}-{end}")
                    if not 1 <= start <= end <= total:
                        errors.append(f"{doc.name}: invalid citation {name}:{interval} (total {total})")
            except (OSError, ValueError) as exc:
                errors.append(f"{doc.name}: citation unavailable {name}: {exc}")
    counts = {status: sum(row[2] == status for row in ledger) for status in ("Explicado", "Parcial", "Contexto", "Excluido")}
    if args.list_citations:
        for citation in resolved_citations:
            print(citation)
    outputs = sorted(p.name for p in HERE.iterdir() if p.is_file())
    print(json.dumps({"markdown_files": len(docs), "local_links": link_count, "heading_anchor_links": anchor_count, "inclusive_citation_ranges": citation_count, "inventory_files": len(ledger), "actual_source_files": len(actual), "statuses": counts, "hashed_files": len(snapshot), "output_files": outputs, "errors": errors}, ensure_ascii=False, indent=2))
    raise SystemExit(1 if errors else 0)


if __name__ == "__main__":
    main()
