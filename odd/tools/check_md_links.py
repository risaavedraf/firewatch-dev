#!/usr/bin/env python3
"""Relative-link checker for this repository's Markdown.

Only files Git would commit are inspected: tracked files plus untracked,
non-ignored files. That answers the question that matters for a clone rather
than the question that happens to hold on this machine, so a link whose target
exists locally but could never be committed (it lives under an ignored
directory such as ``tmp/``) is reported as broken.

What it checks, per Markdown file:

* inline links and images, including angle-delimited destinations
  (``<path with spaces.md>``), destinations containing balanced parentheses and
  destinations that continue on the next line;
* reference-style links and images (``[text][label]``, ``[text][]``,
  ``[label]``) resolved through their ``[label]: target`` definition, and the
  definition destinations themselves;
* relative targets that resolve to a file Git would commit, or to a directory
  that would contain at least one committed file;
* heading anchors in Markdown targets and in-page ``#anchor`` links, using
  GitHub's slug rules including duplicate-heading ``-1``/``-2`` suffixes and
  Setext headings.

Deliberately out of scope, so the result is read with its limits in mind:
external URLs, ``mailto:``, absolute ``/``-rooted paths, bare single-character
fragments, footnote definitions (``[^1]:``), shortcut references that have no
matching definition, and CommonMark's rarer corners: backslash-escaped
brackets, inline code written with more than one backtick, HTML link syntax
and label text compared literally rather than case- or whitespace-folded. It
never rewrites anything and never touches the network.

It reads the working tree, not ``HEAD``. When the two differ it says so, so a
"BROKEN: 0" line is never silently mistaken for a statement about a commit.

Exit code 0 means no broken relative link. Exit code 1 means at least one
finding, printed one per line.

Usage:
    python -B odd/tools/check_md_links.py
    python -B odd/tools/check_md_links.py --self-test
"""

from __future__ import annotations

import posixpath
import re
import subprocess
import sys
import unicodedata
from pathlib import Path
from urllib.parse import unquote

EXTERNAL_RE = re.compile(r"^[a-z][a-z0-9+.-]*:", re.IGNORECASE)
FENCE_RE = re.compile(r"^ {0,3}(`{3,}|~{3,})")
HEADING_RE = re.compile(r"^ {0,3}#{1,6}\s+(.*?)\s*#*\s*$")
SETEXT_RE = re.compile(r"^ {0,3}(=+|-+)\s*$")
DEFINITION_RE = re.compile(r"^ {0,3}\[([^\]]+)\]:[ \t]*(\S+)", re.MULTILINE)
BLOCK_START_RE = re.compile(r"^ {0,3}(?:[|>#`~]|[-*+][ \t]|\d+[.)][ \t])")
INLINE_CODE_RE = re.compile(r"(`+)(?!`)[\s\S]*?(?<!`)\1(?!`)")


def repository_files(root: Path) -> list[str]:
    """Every path Git would commit, as repository-relative POSIX strings."""
    out = subprocess.run(
        ["git", "ls-files", "-z", "--cached", "--others", "--exclude-standard"],
        cwd=root,
        capture_output=True,
        check=True,
    ).stdout
    return sorted(p.decode("utf-8") for p in out.split(b"\0") if p)


def working_tree_differs_from_head(root: Path) -> bool:
    """True when the tree this checker reads is not the tree ``HEAD`` holds.

    Untracked files count too: the checker inspects committable untracked
    Markdown, so a clean tracked tree is not enough for the result to be a
    statement about the commit.
    """
    out = subprocess.run(
        ["git", "status", "--porcelain"],
        cwd=root,
        capture_output=True,
        check=False,
    ).stdout
    return bool(out.strip())


def mask_fences(text: str) -> str:
    """Blank out fenced code blocks while preserving line numbering.

    A fence closes only on a run of the SAME delimiter character that is at
    least as long as the one that opened it, so ``` inside a ```` block stays
    inside it, as CommonMark requires.
    """
    lines = text.split("\n")
    fence: str | None = None
    for index, line in enumerate(lines):
        match = FENCE_RE.match(line)
        if fence is None:
            if match:
                fence = match.group(1)
                lines[index] = ""
        else:
            if match and match.group(1)[0] == fence[0] and len(match.group(1)) >= len(fence):
                fence = None
            lines[index] = ""
    return "\n".join(lines)


def neutralize_inline_code(text: str) -> str:
    """Replace inline code spans with plain text of the same line shape."""
    return INLINE_CODE_RE.sub(
        lambda match: "code" + "\n" * match.group(0).count("\n"), text
    )


def normalize(text: str) -> str:
    return neutralize_inline_code(mask_fences(text))


def slug(text: str) -> str:
    """GitHub heading slug: keep letters, digits, ``-``, ``_`` and spaces.

    Backticks are stripped rather than replaced: GitHub slugs the RENDERED
    heading, so ``# `Hello` `` anchors as ``hello``, not as ``code``.
    """
    text = text.replace("`", "")
    text = re.sub(r"<[^>]+>", "", text)
    text = re.sub(r"\[([^\]]*)\]\([^)]*\)", r"\1", text)
    text = "".join(
        ch
        for ch in text.lower()
        if ch in "-_ " or unicodedata.category(ch)[0] in "LN"
    )
    return text.replace(" ", "-")


def heading_slugs(text: str) -> set[str]:
    """Every anchor GitHub would generate for this document.

    Fences are masked, but inline code is NOT neutralised here: the code text
    belongs to the heading and therefore to its anchor.
    """
    lines = mask_fences(text).split("\n")
    slugs: list[str] = []
    seen: dict[str, int] = {}
    for index, line in enumerate(lines):
        heading: str | None = None
        match = HEADING_RE.match(line)
        if match:
            heading = match.group(1)
        else:
            underline = SETEXT_RE.match(line)
            if underline and index > 0:
                previous = lines[index - 1]
                if previous.strip() and not BLOCK_START_RE.match(previous):
                    heading = previous.strip()
        if not heading:
            continue
        base = slug(heading)
        count = seen.get(base, 0)
        seen[base] = count + 1
        slugs.append(base if count == 0 else f"{base}-{count}")
    return set(slugs)


def line_of(text: str, index: int) -> int:
    return text.count("\n", 0, index) + 1


def parse_inline_destination(text: str, start: int) -> tuple[str | None, int | None]:
    """Parse the destination of an inline link.

    ``start`` is the index of the ``(`` that opens it. Returns the destination
    and the index just past the closing ``)``, or ``(None, None)`` when the
    construct is malformed.
    """
    index = start + 1
    while index < len(text) and text[index] in " \t\n":
        index += 1

    if index < len(text) and text[index] == "<":
        close = text.find(">", index)
        if close == -1:
            return None, None
        destination = text[index + 1:close]
        index = close + 1
        depth = 0
        while index < len(text):
            if text[index] == "(":
                depth += 1
            elif text[index] == ")":
                if depth == 0:
                    return destination, index + 1
                depth -= 1
            index += 1
        return None, None

    depth = 0
    begin = index
    while index < len(text):
        if text[index] == "(":
            depth += 1
        elif text[index] == ")":
            if depth == 0:
                inner = text[begin:index]
                return (inner.split()[0] if inner.split() else ""), index + 1
            depth -= 1
        index += 1
    return None, None


def definition_destinations(text: str) -> dict[str, str]:
    """``label -> target`` for every link reference definition, footnotes aside."""
    found: dict[str, str] = {}
    for match in DEFINITION_RE.finditer(normalize(text)):
        label = match.group(1)
        if not label.startswith("^"):
            found.setdefault(label.strip().lower(), match.group(2))
    return found


def matching_bracket(text: str, start: int) -> int:
    """Index of the ``]`` that closes the ``[`` at ``start``, or ``-1``.

    Brackets nest in Markdown labels, so ``[outer [inner]](target)`` must find
    the OUTER ``]``; a plain ``find("]")`` stops at the inner one and hides
    the destination entirely.
    """
    depth = 0
    for index in range(start + 1, len(text)):
        if text[index] == "[":
            depth += 1
        elif text[index] == "]":
            if depth == 0:
                return index
            depth -= 1
    return -1


def references(text: str) -> list[tuple[int, str]]:
    """Every link destination as ``(line, target)``, deduplicated."""
    body = normalize(text)
    definitions = definition_destinations(text)
    items: list[tuple[int, str]] = []
    for match in DEFINITION_RE.finditer(body):
        if not match.group(1).startswith("^"):
            items.append((line_of(body, match.start()), match.group(2)))

    index = 0
    while index < len(body):
        if body[index] != "[":
            index += 1
            continue
        close = matching_bracket(body, index)
        if close == -1:
            index += 1
            continue
        label = body[index + 1:close]
        after = close + 1
        if after < len(body) and body[after] == "(":
            destination, _ = parse_inline_destination(body, after)
            if destination is not None:
                items.append((line_of(body, index), destination))
        elif after < len(body) and body[after] == "[":
            after_label = body.find("]", after + 1)
            if after_label != -1:
                name = (body[after + 1:after_label] or label).strip().lower()
                destination = definitions.get(name)
                items.append(
                    (
                        line_of(body, index),
                        destination
                        if destination
                        else f"\x00undefined:{label or name}",
                    )
                )
        else:
            destination = definitions.get(label.strip().lower())
            if destination:
                items.append((line_of(body, index), destination))
        # Advance by one so a label nested inside another one is still visited
        # (for example the image in ``[![alt](a.png)](page)``).
        index += 1

    seen = set()
    unique = []
    for item in items:
        if item not in seen:
            seen.add(item)
            unique.append(item)
    return unique


class WorkingTree:
    """Answers where a relative link target lands inside this repository."""

    def __init__(self, root: Path):
        self.root = root
        self.committable = set(repository_files(root))
        self._headings: dict[str, set[str]] = {}

    def status(self, document: str, target: str) -> tuple[str, str]:
        resolved = (self.root / document).parent / target
        try:
            inside = resolved.resolve().relative_to(self.root).as_posix()
        except ValueError:
            return "escape", ""
        if resolved.is_dir():
            if any(f.startswith(inside + "/") for f in self.committable):
                return "directory", inside
            return "empty-directory", inside
        if not resolved.is_file():
            return "missing", inside
        if inside not in self.committable:
            return "uncommittable", inside
        return "file", inside

    def headings(self, relative: str) -> set[str]:
        if relative not in self._headings:
            try:
                text = (self.root / relative).read_text(encoding="utf-8")
            except (OSError, UnicodeDecodeError):
                text = ""
            self._headings[relative] = heading_slugs(text)
        return self._headings[relative]


def check_reference(document: str, line: int, target: str, repository) -> str | None:
    """Return a finding message, or ``None`` when the target is sound."""
    if target.startswith("\x00undefined:"):
        return (
            f"{document}:{line}: link reference has no definition -> "
            f"{target[len(chr(0) + 'undefined:'):]}"
        )
    decoded = unquote(target)
    if EXTERNAL_RE.match(decoded) or decoded.startswith("/"):
        return None
    if decoded.startswith("#"):
        if len(decoded) > 1 and target != "#" and not _anchor_ok(
            document, decoded[1:], repository
        ):
            return f"{document}:{line}: anchor not in this document -> {decoded}"
        return None
    path_part, _, anchor = decoded.partition("#")
    if not path_part:
        return None
    kind, inside = repository.status(document, path_part)
    if kind == "escape":
        return f"{document}:{line}: escapes the repository -> {decoded}"
    if kind == "missing":
        return f"{document}:{line}: missing file -> {decoded}"
    if kind == "uncommittable":
        return (
            f"{document}:{line}: target would never be committed "
            f"(ignored or excluded) -> {decoded}"
        )
    if kind == "empty-directory":
        return (
            f"{document}:{line}: directory would be empty in the repository -> "
            f"{decoded}"
        )
    if anchor and inside.lower().endswith(".md") and not _anchor_ok(
        inside, anchor, repository
    ):
        return f"{document}:{line}: anchor not in {path_part} -> #{anchor}"
    return None


def _anchor_ok(document: str, anchor: str, repository) -> bool:
    return unquote(anchor) in repository.headings(document)


def scan(document: str, text: str, repository) -> list[str]:
    findings = []
    for line, target in references(text):
        finding = check_reference(document, line, target, repository)
        if finding:
            findings.append(finding)
    return findings


SELF_TEST_DOCUMENT = "docs/guide.md"
SELF_TEST_FILES = {SELF_TEST_DOCUMENT, "docs/real.md", "docs/other.md"}
# Any target under `docs/tmp/` stands in for an ignored path: it exists locally
# but would never be committed, which is the case this checker must catch.
SELF_TEST_IGNORED_SEGMENT = "tmp"
SELF_TEST_HEADINGS = {
    "docs/guide.md": heading_slugs("# Hello\n\n## Same\n\n## Same\n"),
}

SELF_TEST_CASES: list[tuple[str, str, int, str | None]] = [
    ("plain relative link resolves", "[ok](real.md)", 0, None),
    ("second resolvable relative link", "[ok](other.md)", 0, None),
    ("missing file", "[broken](missing.md)", 1, "missing file"),
    ("ignored directory", "[ignored](tmp/missing.md)", 1, "never be committed"),
    # A broken reference target is reported twice on purpose: once at the
    # definition that holds the dead target, and once at the usage that
    # resolves to it, because both lines are link sites a reader can follow.
    ("reference-style link", "[ref][label]\n\n[label]: tmp/missing.md", 2, "never be committed"),
    ("parentheses in destination", "[paren](tmp/missing(file).md)", 1, "never be committed"),
    ("angle-delimited destination", "[spaced](<tmp/missing file.md>)", 1, "never be committed"),
    ("destination on the next line", "[multi](\n tmp/missing.md\n)", 1, "never be committed"),
    ("undefined link reference", "[text][nope]", 1, "no definition"),
    ("anchor that does not exist", "[a](#hello!!!)", 1, "anchor not in this document"),
    ("anchor that exists", "[a](#hello)", 0, None),
    ("duplicate-heading suffix valid", "[a](#same-1)", 0, None),
    ("duplicate-heading suffix invalid", "[a](#same-2)", 1, "anchor not in this document"),
    ("target outside the repository", "[e](../../outside.md)", 1, "escapes the repository"),
    ("nested label does not hide its destination", "[outer [inner]](tmp/missing.md)", 1, "never be committed"),
    ("image nested in a link is still visited", "[![alt](tmp/missing.png)](real.md)", 1, "never be committed"),
]

SELF_TEST_FENCE = (
    "````\n"
    "[inside](tmp/missing.md)\n"
    "```\n"
    "[still inside](tmp/missing.md)\n"
    "````\n"
    "[outside](tmp/missing.md)\n"
)

SELF_TEST_HEADING_CASES: list[tuple[str, set[str]]] = [
    ("# 8. Monitoreo: Prometheus + Grafana", {"8-monitoreo-prometheus--grafana"}),
    ("## 11. Aprobaci\u00f3n final Go / No-Go", {"11-aprobaci\u00f3n-final-go--no-go"}),
    ("Same\n====\n\nSame\n----", {"same", "same-1"}),
    ("# `Hello`", {"hello"}),
    ("## `init_env.py`: creaci\u00f3n exclusiva", {"init_envpy-creaci\u00f3n-exclusiva"}),
]


class SelfTestRepository:
    def __init__(self, document: str):
        self.document = document

    def status(self, document: str, target: str) -> tuple[str, str]:
        joined = posixpath.normpath(
            posixpath.join(posixpath.dirname(document), target or "")
        )
        if joined.startswith(".."):
            return "escape", joined
        if joined in SELF_TEST_FILES:
            return "file", joined
        if SELF_TEST_IGNORED_SEGMENT in joined.split("/"):
            return "uncommittable", joined
        if any(f.startswith(joined + "/") for f in SELF_TEST_FILES):
            return "directory", joined
        return "missing", joined

    def headings(self, relative: str) -> set[str]:
        return SELF_TEST_HEADINGS.get(relative, set())


def self_test() -> int:
    failures: list[str] = []
    checks = 0

    for source, expected in SELF_TEST_HEADING_CASES:
        checks += 1
        actual = heading_slugs(source)
        if actual != expected:
            failures.append(f"heading slugs for {source!r}: {actual} != {expected}")

    for name, text, count, needle in SELF_TEST_CASES:
        checks += 1
        findings = scan(SELF_TEST_DOCUMENT, text, SelfTestRepository(SELF_TEST_DOCUMENT))
        if len(findings) != count:
            failures.append(f"{name}: expected {count} finding(s), got {findings}")
        elif needle and needle not in " ".join(findings):
            failures.append(f"{name}: {needle!r} not in {findings}")

    checks += 1
    findings = scan(
        SELF_TEST_DOCUMENT, SELF_TEST_FENCE, SelfTestRepository(SELF_TEST_DOCUMENT)
    )
    if len(findings) != 1:
        failures.append(
            f"four-backtick fence: expected exactly 1 finding, got {findings}"
        )

    for failure in failures:
        print(f"  FAIL {failure}")
    print(f"self-test: {checks - len(failures)}/{checks} checks passed")
    return 1 if failures else 0


def main(argv: list[str]) -> int:
    if "--self-test" in argv:
        return self_test()

    root = Path(__file__).resolve().parents[2]
    repository = WorkingTree(root)
    documents = sorted(f for f in repository.committable if f.lower().endswith(".md"))
    findings: list[str] = []

    for document in documents:
        try:
            text = (root / document).read_text(encoding="utf-8")
        except (OSError, UnicodeDecodeError) as error:
            findings.append(f"{document}: unreadable ({error})")
            continue
        findings.extend(scan(document, text, repository))

    if working_tree_differs_from_head(root):
        print(
            "NOTE: the working tree differs from HEAD; this result describes the "
            "working tree, not the commit."
        )
    print(f"Markdown files inspected: {len(documents)}")
    print(f"BROKEN: {len(findings)}")
    for finding in findings:
        print(f"  - {finding}")
    return 1 if findings else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
