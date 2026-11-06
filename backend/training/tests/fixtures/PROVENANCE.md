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

## `dashboard-conaf-sample.csv` -- real, trimmed

The third file here, and the one `train_delivered.py` reads. It is a trim of the
delivered regional dashboard export.

| | |
| --- | --- |
| Source file | `dashboard_conaf_202609281623 (1).csv`, delivered to the repository root as `data/dashboard_conaf_202609281623 (1).csv` |
| URL | none -- the file was handed over directly, not downloaded |
| Licence / citation | **not stated by the source.** The file carries no licence, citation or metadata row; its columns are CONAF dashboard fields and its name carries the export instant (2026-09-28 16:23). Recorded as delivered, and the absence is recorded too rather than filled in |
| Whole file | 87 435 bytes, sha256 `586bfbb568c4487a16186a993a5c60a0cdb13a4175cc6d55050fc093e0a890c7`, 1 440 rows, 10 columns, 16 regions x 90 days, 2026-07-01 .. 2026-09-28 |
| Trimmed file | 27 208 bytes, 450 rows, sha256 `b9fbc6334ba0876e329f29c58c3f1ad03060d318e8aff25222fe9d25c05a1ac8` |
| Encoding | UTF-8, no BOM, comma separated, CRLF line endings as the source writes them, header quoted exactly as the source writes it |

### Selection rule

Exactly, and only, this:

> Every source row whose `region` is one of `Biobío`, `O'Higgins`, `Tarapacá`,
> `Valparaíso`, `Ñuble` -- every date present for those regions, which is all 90 -- in
> the source's own column order, sorted by (`fecha`, `id`).

450 of the 1 440 rows match. Nothing was dropped afterwards, no column was removed, no
value was edited, and the header is byte-for-byte the source's.

### What the selection is chosen to cover

| Case | Where in the fixture |
| --- | --- |
| All three downward revisions of `sup_afec_act` | Valparaíso 2026-07-03 -> 07-04 (2.2 -> 1.9), Tarapacá 2026-07-31 -> 08-01 (2.5 -> 1.23), O'Higgins 2026-09-19 -> 09-20 (3.9 -> 3.6). Two land in the training months and one in September, so both sides of the split see a clamp |
| Positive days on both sides of the temporal split | 11 in July + August, 16 in September; every positive in the whole file comes from a day the cumulative counter advanced |
| Regions that never advanced the counter, and one that advanced a lot | `Biobío` 7, `Valparaíso` 9, `O'Higgins` 5, `Tarapacá` 5, `Ñuble` 1 |
| Every month the split names | July, August, September are all present, so `TRAIN_MONTHS`/`TEST_MONTHS` are all exercised |
| An accented name, an apostrophe and a tilde | `Valparaíso`, `O'Higgins`, `Ñuble` -- three different folds to an ASCII feature name |

The five regions were chosen for the three revisions, not to make the numbers look
better. They do not: the fixture reproduces the real file's finding (no model beats
either baseline) and that is the point of using it.

### What this fixture is not

It is not a training set and it is not a model. It is 450 rows of one region-day grid
for one 90-day window. Its only job is to let the label, the split, the leak guard, the
train-only fit and the artifact export be tested offline against bytes somebody can
point at.
