# Fixture provenance: the three delivered files

These three CSVs are trims of the three files the team delivered, and they exist so
`tests/test_analyze_delivered.py` can exercise the analysis offline. The whole files live
in the repository root under `data/` and are untracked; CI does not have them, so the
fixtures are the only copy the suite can see.

The trim rule for each file is stated exactly, and the selection contains nothing that
was edited: no column was removed, no value was rewritten, no row was added, and the
header is byte-for-byte the source's. The trim keeps the source's own field quoting and
its CRLF line endings.

## The three sources

| File | Bytes | sha256 | Rows | Columns |
| --- | --- | --- | --- | --- |
| `dashboard_conaf_202609281623 (1).csv` | 87 435 | `586bfbb568c4487a16186a993a5c60a0cdb13a4175cc6d55050fc093e0a890c7` | 1 440 | 10 |
| `emas_202609281634 (1).csv` | 153 407 | `f72be0364f8a6937df5ed655d5e650b24c4d280a6b8bcb1c779aabe52e2bb380` | 535 | 20 |
| `rtdata_y2026_202609281633 (1).csv` | 519 353 | `3b20283b7c5f1cb3bf69376a0bb1a16a6dfd02651d8d4204e2571913e5f3319f` | 5 122 | 13 |

All three were handed over directly. None carries a licence, a citation, a retrieval URL
or a declared unit, and that absence is recorded in the analysis report rather than
filled in here. The dashboard export is the same file `fixtures/PROVENANCE.md` describes
under `dashboard-conaf-sample.csv`; that fixture is a wider trim of the same bytes, used
by `train_delivered.py`.

## The three fixtures

| File | Bytes | sha256 | Rows |
| --- | --- | --- | --- |
| `dashboard-conaf-analysis-sample.csv` | 16 621 | `bec7fe96900178b6f57489237a22d67eb77ad005d93b30e81bdc5475a7f236ef` | 270 |
| `emas-sample.csv` | 17 612 | `9459f627ab8ac22b5d3b3520d60e96589ab5b78037ee3c00924737662b3439c2` | 61 |
| `rtdata-sample.csv` | 37 549 | `0a1ba18ed19eba69882b659d027f5f97ff860ac0842cae97dbfadccb3068721d` | 369 |

### `dashboard-conaf-analysis-sample.csv` -- real, trimmed

Selection rule, exactly:

> Every source row whose `region` is one of `Biobío`, `Tarapacá`, `Valparaíso` -- every
> date present for those regions, which is all 90 -- in the source's own column order,
> sorted by (`fecha`, `id`).

270 of the 1 440 rows match. The three regions were chosen for what the analysis has to
compute, not to make the numbers look better:

| Case | Where in the fixture |
| --- | --- |
| A complete region-day grid | 3 regions x 90 dates, 0 missing, 0 duplicated |
| Downward revisions of `sup_afec_act` | Valparaíso 2026-07-03 -> 07-04 and Tarapacá 2026-07-31 -> 08-01 (2 of the whole file's 3) |
| A cumulative counter that never shrinks | `nro_inc_act` has 0 decrements, so a decrement has to be **introduced** by the test that asserts one is counted |
| Positives in every month | 21 positive region-days over July (6), August (2) and September (13), and 24 incident increments behind them |
| A base-rate shift across the split | the test month's rate is higher than the training months', as in the whole file |
| Accented and apostrophed region names | `Biobío`, `Tarapacá`, `Valparaíso` |

The trim keeps 2 of the 3 revisions on purpose: the third (O'Higgins) is not needed to
test that a revision is listed, and `fixtures/dashboard-conaf-sample.csv` already covers
all three for the rehearsal.

### `emas-sample.csv` -- real, trimmed

Selection rule, exactly:

> Every source row whose `region_nombre` is `Valparaíso`, plus the rows for the station
> ids `41`, `120`, `175`, `326`, `355` -- in the source's own order.

61 of the 535 rows match. The five extra stations are each the only carrier of a defect
the analysis is supposed to find:

| Station | What it carries |
| --- | --- |
| `326` La Serena | the 4 `RADIACION_SOL` readings above the solar constant (1713.67 and 1758.00 W/m2) |
| `120` Tierras Nobles | the only reading at exactly 0.00 % relative humidity |
| `175` Ninhue | a `Ñuble` label with a `08` (pre-2018 Biobío) CUT -- the identifier-chain mismatch |
| `355` Islote Pájaros | a row with no `region_nombre` at all |
| `41` Til Til | a five-digit `com_ia_id` (`13303`), so the padding rule sees both shapes |

The `Valparaíso` half is the whole region, all 56 rows, plus the five rows above from
other regions; the remaining 474 catalogue rows are not in the trim. The whole
`Valparaíso` set is what makes the fixture reproduce the real coverage split exactly: of
the 38 DPA comunas, **11 complete / 15 partial / 12 none** under the active-station rule,
and 11 / 16 / 11 if an inactive station is counted.

### `rtdata-sample.csv` -- real, trimmed

Selection rule, exactly:

> Every source row whose `ema_ia_id` is one of the 61 station ids in
> `emas-sample.csv`, in the source's own order.

369 of the 5 122 rows match, and every row of the catalogue trim that has a reading is
kept -- no variable was filtered. What it covers:

| Case | Where in the fixture |
| --- | --- |
| A single timestamp | 1 distinct `rtd_dt_marcatiempo` (`2026-09-28 09:00:00.000 -0300`) |
| Duplicate (station, variable) rows | 93 extra rows over 276 keys, in groups of 2 and 4, on 5 stations |
| Copies that disagree on the value | 7 groups carry more than one distinct `rtd_f_valor` at different `rtd_dt_versiondt` instants |
| Empty columns | all 5 (`ins_ia_id`, `rtd_c_emaid`, `rtd_i_resultadochecks`, `rtd_i_calidad`, `rtd_c_ultimocomentario`) |
| A reading outside a stated physical bound | `RADIACION_SOL`, 4 rows, all station `326` |
| A reading at the edge of its bound | `HUMEDAD_REL` = 0.00 %, station `120` |
| A variable with an undeclared aggregation | `EVAPOTRANSPIRACION` (max 686.778) and the two derived `HORAS_FRIO` / `GRADOS_DIA` |
| A pressure minimum inside the bound | not in this trim: the altiplano stations are not in the catalogue fixture, so the altitude reading is tested through `standard_atmosphere_height` instead |

### What these fixtures are not

They are not samples of the population the model will see, and no statistic computed from
them means anything about Chile. Their only job is to let every claim in
`ANALYSIS-delivered-data.md` be computed by code somebody can run offline, and to let a
test **change** an input and require the output to move -- which is what proves the report
is generated rather than typed.
