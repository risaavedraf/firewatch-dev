# Fixture provenance

Two CSV files live here. One is real data, trimmed. The other is fabricated, and it
is named so nobody can mistake it for the first.

## `conaf-maule-sample.csv` -- real, trimmed

| | |
| --- | --- |
| Source file | `Base de datos incendios forestales Geprif 2024-12-17 08.31.csv` |
| URL | <https://raw.githubusercontent.com/cvidalmsu/FONDEF-IDEA-/main/Base%20de%20datos%20incendios%20forestales%20Geprif%202024-12-17%2008.31.csv> |
| Repository | <https://github.com/cvidalmsu/FONDEF-IDEA-> |
| Licence | CC BY 4.0 (Creative Commons Attribution 4.0 International) |
| Citation | Vidal-Silva, C. et al. (2025). *A Ground-Reported Dataset of Wildfire Occurrence and Damage in Chile (1985-2023)*. MDPI Data, 10(2), 115. The source README elides the DOI as `https://doi.org/...`; it is reproduced as published and not guessed. |
| Retrieved | 2026-11-06 |
| Whole file | 616 347 bytes, sha256 `e5b179b95802e01fc497a1c0a259823c3edd501b6716980d91383a1b1c2e0456`, 951 rows, 91 columns |
| Trimmed file | 30 264 bytes, 44 rows, sha256 `178d350022b06b73f72a58851741a124adde9b22595efbcc6e27ea89a7921d0b` |
| Encoding | UTF-8 with BOM, comma separated, CRLF as the source writes it |

### Selection rule

Exactly, and only, this:

> Every source row whose `Comuna` is `Talca` or `Cauquenes` and whose `Inicio` falls
> on a local calendar date in `2018-11-20 .. 2018-12-31` (America/Santiago), in the
> source's own column order, sorted by (`Inicio` date, `Comuna`, `Nombre`).

44 of the 951 rows match. Nothing was dropped afterwards, no column was removed, no
value was edited, and the header is byte-for-byte the source's. `fetch_sample.py`
re-downloads the whole file and verifies it against the sha256 above, so the
selection can be re-run against a re-verified source.

### What the selection is chosen to cover

| Case | Where in the fixture |
| --- | --- |
| A comuna-day with several events | Talca 2018-12-18 has **4**; Talca 2018-12-10 and Cauquenes 2018-12-13 have 3; five more days have 2 |
| A comuna-day with one event | 21 of the fixture's 30 comuna-days |
| A non-null cause | every row carries a `Causa general` code (`1.7 - ...`, `4.1 - ...`) |
| Both `Inicio` shapes | 7 rows `dd/mm/yyyy HH:MM`, 37 rows `d-mmm-yyyy HH:MM` |
| Meteorology missing on a fired day | Cauquenes 2018-12-12 and 2018-12-18 (all meteorology cells empty) |

### Two cases this source CANNOT supply

The registry has no row with a null `Inicio` and no row with `Superficie total` of
zero (its minimum is 0.003 ha), so no honest trim of it can contain either. Rather
than fabricate rows inside a file stamped with a URL and a checksum, those two cases
live in the file below, and the fixture above stays exactly what its provenance says.

(The "502 null dates" figure that circulates for this file is a **reader** artefact,
not a property of the data: exactly 502 rows write `Inicio` as `30-ene-2019 17:20`,
and a parser that only knows `dd/mm/yyyy` throws those away. All 951 rows carry a
parseable date. `assemble.py` parses both shapes and the manifest counts them
separately.)

## `synthetic-edge-cases.csv` -- FABRICATED, do not cite as data

Three rows written by hand for the two cases above and nothing else. They carry the
real 91-column header so they exercise the same reader, the invented values are
labelled `SYNTHETIC-...` in `Nombre`, and no statistic computed from them means
anything about Maule:

| Row | Purpose |
| --- | --- |
| `SYNTHETIC-NO-DATE` | `Inicio` empty -- must be excluded from the panel and counted, never imputed to a date |
| `SYNTHETIC-BAD-DATE` | `Inicio` = `sin fecha` -- same, counted separately from a blank cell |
| `SYNTHETIC-ZERO-AREA` | `Superficie total` = `0` on 2018-12-05 -- a real measured zero in the label |

The file is 2 018 bytes, sha256 `edde38296d85425624580a08b08cab3caadef89607ed7b9223d66db0a854d68a`.

They must never be joined with the real sample. They exist so the exclusion path and
the zero-area label path are covered by a committed file instead of by an in-test
literal nobody can point at later.
