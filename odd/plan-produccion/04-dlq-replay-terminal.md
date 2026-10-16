# Frente 4 — Propagación de fallos terminales y replay de DLQ

- **Grupo de entrega:** G3 — Robustez de backend.
- **Estado:** Pendiente — planificación. No acredita implementación ni evidencia.
- **Riesgo (Task Size):** alto al implementar — ítem 4 (orden/duplicados en el consumo) e ítem 1 (estado persistido del job).
- **Depende de:** BH-01 (toolchain/baseline); política de recuperación acordada; coordinar con BH-10 (idempotencia concurrente).
- **Seam / superficie:** consumidor (`broker.go`), persistencia/ACK, broker DLQ y estado terminal del job.

**Objetivo.** Hacer observables los fallos terminales y permitir recuperación/replay **idempotente** de trabajos fallidos sin duplicar efectos.

**Documentación real (redirección).**
- [`../../backend/docs/architecture.md:42-46`](../../backend/docs/architecture.md) — la DLQ conserva fallos; falta replay y propagación a estado terminal.
- [`../../backend/README.md:96-98`](../../backend/README.md) — estado operativo.
- [`../tasks/security/backend-hardening/07-job-recovery.md:7-20`](../tasks/security/backend-hardening/07-job-recovery.md) — criterios de aceptación.

## Specs

- **S1** — Se acuerdan fallos transitorios/permanentes, presupuesto de reintentos, backoff y estados terminales visibles.
- **S2** — Existe recuperación durable, reconciliación y **replay sin duplicar efectos**.
- **S3** — Se cubren reinicios y entregas duplicadas o reordenadas en los límites de persistencia/ACK.
- **S4** — Hay alertas y *runbook* para reintentos agotados, DLQ y trabajos atascados.

## Tasks

| ID | Spec | Ruta | Descripción | Commit |
| --- | --- | --- | --- | --- |
| T1 | S1 | inline | Acordar y documentar la política de reintentos/backoff/estado terminal. | — |
| T2 | S2 | inline | Implementar replay idempotente de la DLQ y reconciliación. | — |
| T3 | S3 | inline | Probar reinicios y entregas duplicadas/reordenadas. | — |
| T4 | S4 | inline | Alertas y runbook para reintentos agotados, DLQ y trabajos atascados. | — |

Ruta por defecto: **inline**. Delegar escritura solo por disparador.

## Dependencias y bloqueos humanos

- **Bloqueo humano:** acordar estados observables y política de recuperación **antes** de implementar.
- Depende de BH-01; coordinar con BH-10.

## Criterios de cierre

- `../tasks/security/backend-hardening/07-job-recovery.md:11-20`.

## Log

- **L1 (2026-10-16)** — Creación del desglose por spec a partir de [`README.md`](README.md) §1 (frente 4). Documento de planificación; sin implementación.
