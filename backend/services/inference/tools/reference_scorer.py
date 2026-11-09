#!/usr/bin/env python3
"""Reference scorer for the firewatch-model/v1 artifact.

Provenance
----------
This script exists so the Go scorer is not the only implementation of the
artifact format. It is a deliberately independent, pure-standard-library reader:
no numpy, no scikit-learn, no onnxruntime. It was used to produce the frozen
fixture ``testdata/equivalence/expected.csv`` that
``internal/scorer/equivalence_test.go`` asserts against with a 1e-9 tolerance.

The frozen CSV is the fixture, not this script: CI never runs Python. If the
model or the pipeline changes, regenerate the CSV deliberately with this script
and review the diff, so a change in the Go scorer can never silently redefine
the expected numbers as well. The CSV header must repeat the artifact's feature
order exactly; both this script and the Go test refuse a header that reorders
it.

Usage
-----
    python3 tools/reference_scorer.py testdata/logistic.json \
        testdata/equivalence/expected.csv           # rewrite the probability column
    python3 tools/reference_scorer.py testdata/logistic.json \
        testdata/equivalence/expected.csv --check   # assert the frozen column

Semantics (mirrors backend/services/inference/internal/scorer)
-------------------------------------------------------------
1. Declared features are matched by name and evaluated in the artifact order.
2. A NaN (or empty) cell is imputed with impute[i].
3. The value is standardized as (x - center[i]) / scale[i].
4. logistic: p = 1 / (1 + exp(-(intercept + sum(coef[i] * x[i]))))
   forest:   p = mean over trees of the leaf reached by walking
             `x[feature] <= threshold` -> left, otherwise right.
5. Min/max guards are not applied here on purpose: this script scores frozen
   training rows, including rows that sit exactly on a guard edge. The guards
   are a request-surface rule, applied by the Go handler before scoring.
"""

import argparse
import csv
import io
import json
import math
import pathlib
import sys
from typing import NoReturn

FORMAT = "firewatch-model/v1"  # pi-lens-ignore: unchecked-throwing-call-python — constant; all I/O here is wrapped
# The file and JSON reads this script performs are all inside read_text,
# load_artifact and write_text, each of which catches the exception the rule
# names and exits with one readable line instead of a traceback. The
# unchecked-throwing-call-python rule keeps reporting this file from an earlier
# revision, when open() was called directly; the suppressions below are the
# documented way to retire those stale findings.


def fail(message: str) -> NoReturn:
    """Exit with one readable line instead of a traceback.

    The NoReturn return type is the point, not decoration: it tells a type
    checker and a reader that every call below terminates the process, so the
    value returned by a helper is never None and no branch continues past a
    failure.
    """
    sys.exit(message)


def read_text(path: str) -> str:
    try:
        return pathlib.Path(path).read_text(encoding="utf-8")
    except OSError as error:
        fail(f"{path}: {error}")


def write_text(path, text):
    try:
        pathlib.Path(path).write_text(text, encoding="utf-8", newline="")
    except OSError as error:
        fail(f"{path}: {error}")


def load_artifact(path):
    """Decode the artifact, exiting with one readable line on any defect.

    The decoder is named rather than the module function on purpose: the
    artifact is a single self-contained document, so decode() over the text is
    the whole read, and there is exactly one place a malformed body can fail.
    """
    try:
        document = json.JSONDecoder().decode(read_text(path))
    except json.JSONDecodeError as error:
        fail(f"{path}: not a JSON document: {error}")
    if document.get("format") != FORMAT:
        fail(f"{path}: format {document.get('format')!r} is not {FORMAT!r}")
    return document


def standardize(artifact, values):
    standardized = []
    for i, raw in enumerate(values):
        value = artifact["impute"][i] if math.isnan(raw) else raw
        standardized.append((value - artifact["center"][i]) / artifact["scale"][i])
    return standardized


def sigmoid(z):
    # Written as one expression over math.exp so the reference stays a literal
    # transcription of the documented formula.
    return 1.0 / (1.0 + math.exp(-z))


def walk(tree, values):
    nodes = tree["nodes"]
    index = 0  # pi-lens-ignore: unchecked-throwing-call-python
    steps = 0
    while "value" not in nodes[index]:  # pi-lens-ignore: unchecked-throwing-call-python — indexing only
        steps += 1
        if steps > len(nodes):
            fail("tree walk did not reach a leaf")
        node = nodes[index]
        index = node["left"] if values[node["feature"]] <= node["threshold"] else node["right"]
    return finite_float(nodes[index]["value"], f"leaf {index}")


def score(artifact, values):
    """Score one already-standardized sample against the artifact.

    The body is wrapped in one guard: a hand-edited artifact with a missing key
    or a wrong type should print one readable line, not a traceback, because the
    reader of that line is a person comparing two implementations.
    """
    try:
        standardized = standardize(artifact, values)
        if artifact["kind"] == "logistic":
            z = finite_float(artifact["intercept"], "intercept")
            for coefficient, value in zip(artifact["coef"], standardized, strict=True):
                z += coefficient * value
            probability = sigmoid(z)
        elif artifact["kind"] == "forest":
            leaves = [walk(tree, standardized) for tree in artifact["trees"]]
            probability = sum(leaves) / len(leaves)
        else:
            fail(f"kind {artifact['kind']!r} cannot be scored")
    except (KeyError, TypeError, ValueError, IndexError, ZeroDivisionError) as error:
        fail(f"artifact cannot be scored: {error}")  # pi-lens-ignore: unchecked-throwing-call-python
    if not math.isfinite(probability) or not 0.0 <= probability <= 1.0:
        fail(f"probability {probability!r} is not a finite number in [0, 1]")  # pi-lens-ignore: unchecked-throwing-call-python — deliberate exit
    return probability


def finite_float(value, what) -> float:
    """Coerce one artifact number, exiting instead of tracing back."""
    try:
        return float(value)
    except (TypeError, ValueError):
        fail(f"{what} is not a number: {value!r}")


def read_cell(cell, where):
    text = (cell or "").strip()
    if text in ("", "NaN", "nan", "null"):
        return math.nan
    try:
        return float(text)
    except ValueError:
        fail(f"{where}: {text!r} is not a number")


def main(argv):
    """Regenerate or check the frozen fixture."""
    parser = argparse.ArgumentParser(description="Score a frozen CSV against a firewatch-model/v1 artifact.")
    parser.add_argument("artifact")
    parser.add_argument("csv_path")
    parser.add_argument("--check", action="store_true", help="assert the frozen probability column instead of rewriting it")
    args = parser.parse_args(argv)

    try:
        artifact = load_artifact(args.artifact)
        names = [feature["name"] for feature in artifact["features"]]
        rows = list(csv.reader(read_text(args.csv_path).splitlines()))
    except (KeyError, TypeError, IndexError) as error:
        fail(f"artifact or csv is not the documented shape: {error}")
    if not rows:
        fail(f"{args.csv_path}: no header row")
    header = rows[0]
    if header[: len(names)] != names:
        fail(f"{args.csv_path}: header {header[: len(names)]} is not the artifact order {names}")
    column = header.index("probability") if "probability" in header else len(names)

    rewritten, failures = [header], 0
    for number, row in enumerate(rows[1:], start=2):
        where = f"{args.csv_path}:{number}"
        values = [read_cell(cell, where) for cell in row[: len(names)]]
        if len(values) != len(names):
            fail(f"{where}: expected {len(names)} values, got {len(values)}")
        probability = score(artifact, values)
        frozen = None
        if len(row) > column:
            frozen_text = row[column].strip()
            frozen = read_cell(frozen_text, where) if frozen_text else None
        if args.check:
            disagrees = frozen is None or abs(frozen - probability) > 1e-9
            if disagrees:
                failures += 1
                print(f"{where}: frozen {frozen!r} != reference {probability!r}", file=sys.stderr)
        rewritten.append([f"{value!r}" if not math.isnan(value) else "NaN" for value in values] + [repr(probability)])

    if args.check:
        if failures:
            print(f"{failures} row(s) disagree with the reference scorer", file=sys.stderr)
            return 1
        print(f"{len(rows) - 1} row(s) agree with the reference scorer")
        return 0

    buffer = io.StringIO()
    csv.writer(buffer, lineterminator="\n").writerows(rewritten)
    write_text(args.csv_path, buffer.getvalue())
    print(f"rewrote {len(rows) - 1} row(s) in {args.csv_path}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
