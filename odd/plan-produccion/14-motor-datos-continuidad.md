# Frente 14 — Decisión del motor de datos y continuidad/recuperación

- **Grupo de entrega:** G1 — Base de entrega.
- **Estado:** Pendiente — planificación. PostGIS local está aprobado; la decisión de motor productivo sigue explícita.
- **Riesgo (Task Size):** alto al implementar — ítem 1 (motor/persistencia y backups) e ítem 5 (infraestructura/despliegue).
- **Depende de:** decisión de motor/infraestructura; frente 7 para despliegue redundante; procedimientos operativos y pruebas.
- **Seam / superficie:** persistencia `risk`/`jobs`, migraciones PostgreSQL/PostGIS y el motor productivo elegido; almacenamiento de objetos para modelos/datasets/backups.

**Objetivo.** Decidir el motor productivo y demostrar continuidad/recuperación que respalde el SLA objetivo.

**Documentación real (redirección).**
- [`../../Referencias/EP1_Equipo3_Armijo_Calderon_Saavedra2_produccion.md:76,90,102-104`](../../Referencias/EP1_Equipo3_Armijo_Calderon_Saavedra2_produccion.md) — SLA/frescura y metas iniciales (p. ej. disponibilidad mensual de 99,5%).
- [`../../Referencias/Actividad C4_FireWatch_produccion.md:63-65,253-263`](<../../Referencias/Actividad C4_FireWatch_produccion.md>) — continuidad y recuperación.
- [`../../backend/docs/architecture.md:61-63`](../../backend/docs/architecture.md) — decisión de motor pendiente.
- [`../../backend/docs/verification.md:64`](../../backend/docs/verification.md) — recuperación y SLA no probados.
- [`../tasks/backend/firewatch-data-contract.md:25-29`](../tasks/backend/firewatch-data-contract.md) — PostGIS local aprobado.

## Specs

- **S1** — Se resuelve el motor de datos de producción (referencias citan Oracle; local está aprobado PostgreSQL/PostGIS; el backend **no** declara compatibilidad Oracle).
- **S2** — Se definen continuidad y recuperación, con redundancia y procedimientos que sostengan las últimas estimaciones válidas ante fallos.
- **S3** — Se define y **prueba** la política de backups/restauración.
- **S4** — Se valida el objetivo de disponibilidad mensual y se acuerdan metas medibles de recuperación (RPO/RTO).
- **S5** — Se confirman las metas de rendimiento y carga mediante pruebas (son metas iniciales sujetas a validación).

## Tasks

| ID | Spec | Ruta | Descripción | Commit |
| --- | --- | --- | --- | --- |
| T1 | S1 | inline | Decidir el motor productivo y documentar el impacto en migraciones/persistencia. | — |
| T2 | S2 | inline | Definir continuidad/recuperación y procedimientos de degradación. | — |
| T3 | S3 | inline | Definir y probar backups/restauración. | — |
| T4 | S4 | inline | Validar disponibilidad mensual y acordar RPO/RTO. | — |
| T5 | S5 | inline | Confirmar metas de rendimiento/carga con pruebas. | — |

Ruta por defecto: **inline**. T1 es una decisión humana que bloquea T2–T5.

## Dependencias y bloqueos humanos

- **Bloqueo humano:** elegir motor productivo y aprobar SLA/metas de recuperación (motor de producción explícitamente pendiente).
- No hay RPO/RTO ni criterio de restauración definido; deben acordarse.

## Criterios de cierre

- SLA/frescura y metas: `../../Referencias/EP1_Equipo3_Armijo_Calderon_Saavedra2_produccion.md:76,90,104`.
- Decisión de motor: `../../backend/docs/architecture.md:61-63`; recuperación/SLA no probados: `../../backend/docs/verification.md:64`.

## Log

- **L1 (2026-10-16)** — Creación del desglose por spec a partir de [`README.md`](README.md) §1 (frente 14). Documento de planificación; sin implementación.
