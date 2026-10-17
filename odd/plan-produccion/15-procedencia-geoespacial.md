# Frente 15 — Procedencia geoespacial (restricción DIFROL)

- **Grupo de entrega:** G5 — Producto y cierre.
- **Estado:** Pendiente — planificación. La fuente indica que la cartografía **no está validada**.
- **Riesgo (Task Size):** alto al implementar — ítem 1 (trazabilidad/procedencia persistida) e ítem 3 (producto que se publica/consume).
- **Depende de:** datos y metadatos del servicio geoespacial; revisión/validación DIFROL como condición de producción.
- **Seam / superficie:** `data/trazabilidad/`, `backend/scripts/`, `backend/services/risk/assets/geo/`, `backend/services/risk/migrations/` y README.

**Objetivo.** Registrar procedencia y declarar la restricción DIFROL para la cartografía derivada antes de asumir la fuente lista para producción.

**Documentación real (redirección).**
- [`../tasks/backend/firewatch-geospatial-map.md:21-32,34-40`](../tasks/backend/firewatch-geospatial-map.md) — trazabilidad y restricción DIFROL.

## Specs

- **S1** — Se registra la primera fila real de trazabilidad con endpoint, huella de consulta, `retrieved_at`, `cutoff_at` y formatos de origen/derivado.
- **S2** — Se registra atribución a SUBDERE, IGM e INE según metadatos del servicio.
- **S3** — Se declara **textualmente** en la tabla y el README que los productos derivados deben revisarse y validarse por DIFROL.
- **S4** — Se mantiene el límite de procedencia: la cartografía no está validada; el mapa comunal cuenta como producto derivado.

## Tasks

| ID | Spec | Ruta | Descripción | Commit |
| --- | --- | --- | --- | --- |
| T1 | S1 | inline | Registrar la fila real de trazabilidad (endpoint, huella, `retrieved_at`, `cutoff_at`, formatos). | — |
| T2 | S2 | inline | Registrar atribución SUBDERE/IGM/INE. | — |
| T3 | S3 | inline | Declarar la restricción DIFROL en tabla y README. | — |

Ruta por defecto: **inline** (trabajo documental acotado).

## Dependencias y bloqueos humanos

- **Bloqueo humano:** determinar cómo se obtiene y acredita la revisión/validación DIFROL antes de producción; **no afirmar que ya existe**.

## Criterios de cierre

- Procedencia completa y restricción declarada en tabla y README: `../tasks/backend/firewatch-geospatial-map.md:27-32`.

## Log

- **L1 (2026-10-16)** — Creación del desglose por spec a partir de [`README.md`](README.md) §1 (frente 15). Documento de planificación; sin implementación.
