# Frente 12 — Evaluación científica (sesgo, leakage, robustez, Champion/Challenger, drift)

- **Grupo de entrega:** No cierra esta semana — **bloqueado por los dos pendientes permitidos** (datos reales + modelo).
- **Estado:** **Bloqueado para acreditar** — planificación. La evidencia sintética no cierra los IDs; el protocolo, las métricas y los criterios pueden prepararse sin datos reales.
- **Riesgo (Task Size):** alto al implementar — ítem 1 (datos/partición) e ítem 6 (resultados sin *ground truth* no son válidos).
- **Depende de:** datos reales representativos; artefacto de modelo evaluable/*ground truth*; grupos, métricas y umbrales acordados.
- **Seam / superficie:** fuentes/esquema y partición de datos; artefacto/versionado del modelo; logging de inputs, outputs y *ground truth*.

**Objetivo.** Completar la evaluación con datos reales, protocolo reproducible y criterios aprobados. **Se prepara como plan, no como resultado.**

**Documentación real (redirección).**
- [`../urgent/02-pruebas-datos.md:22-35`](../urgent/02-pruebas-datos.md) — calidad, sesgo y fugas de datos.
- [`../urgent/03-pruebas-modelo.md:21-34`](../urgent/03-pruebas-modelo.md) — evaluación y robustez del modelo.
- [`../../Referencias/tareas_verificacion_implementacion_IA.md:217-239,257-280,303-318`](../../Referencias/tareas_verificacion_implementacion_IA.md) — criterios por ID.

## Specs

- **S1** — Con datos reales, se definen grupos/métrica/mitigación de sesgo, se mide cobertura/paridad y se justifica cada N/A.
- **S2** — Se documenta la partición temporal/geográfica y se verifica que el test sea posterior y sin fuga (*leakage*).
- **S3** — Se acuerdan escenarios y tolerancia de robustez; se mide degradación y fallos catastróficos.
- **S4** — Se compara Champion/Challenger con métricas y umbrales acordados; no se sustituye ante regresión sin justificación aceptada.
- **S5** — Se define monitoreo/frecuencia de drift y, con *ground truth*, se recalculan métricas.

## Tasks

| ID | Spec | Ruta | Descripción | Commit |
| --- | --- | --- | --- | --- |
| T1 | S1 | *bloqueada* | Protocolo de sesgo con datos reales. | — |
| T2 | S2 | *bloqueada* | Partición temporal/geográfica sin fuga. | — |
| T3 | S3 | *bloqueada* | Robustez: escenarios, tolerancia, degradación. | — |
| T4 | S4 | *bloqueada* | Champion/Challenger con umbrales. | — |
| T5 | S5 | *bloqueada* | Monitoreo/frecuencia de drift + recálculo. | — |

Ruta por defecto: **bloqueada**. No iniciar hasta que datos y modelo reales estén disponibles y aprobados.

Preparación permitida sin datos: protocolo, métricas, tolerancias y criterios de evaluación. La ejecución y la acreditación quedan bloqueadas.

## Dependencias y bloqueos humanos

- **Bloqueo doble:** datos reales y modelo aprobado (los dos pendientes permitidos).
- **Bloqueo humano:** el equipo define grupos, métricas, tolerancias y umbrales; no presentar cifras "por medir" como resultados.

## Criterios de cierre

- `../urgent/02-pruebas-datos.md:26-35`; `../urgent/03-pruebas-modelo.md:25-34`.

## Log

- **L1 (2026-10-16)** — Creación del desglose por spec a partir de [`README.md`](README.md) §1 (frente 12). Documento de planificación; sin implementación.
- **L2 (2026-10-20)** — Estado precisado a «bloqueado para acreditar»: protocolo y criterios se preparan sin datos; la ejecución sigue bloqueada (README §5.1).
