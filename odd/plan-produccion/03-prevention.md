# Frente 3 — Servicio `prevention` y ciclo de acciones preventivas

- **Grupo de entrega:** G5 — Producto y cierre.
- **Estado:** Pendiente — planificación. El backlog FWP03 está **propuesto**, no aprobado como alcance.
- **Riesgo (Task Size):** alto al implementar — ítem 3 (nuevo contrato/servicio que otros consumirán).
- **Depende de:** FWP01–02 y del acuerdo de partes interesadas sobre vocabulario y ciclo.
- **Seam / superficie:** límite futuro del servicio `prevention`; contrato/modelo de operación preventiva aún por validar; `backend/docs/architecture.md:34`.

**Objetivo.** Definir y habilitar operaciones preventivas asignables, trazables y revisables (acuse, avance, bloqueo, envío, aceptación, devolución, reapertura, cancelación).

**Documentación real (redirección).** **Requiere revisión de alcance antes de implementar.**
- [`../tasks/frontend/frontend-mockup/README.md:46-54`](../tasks/frontend/frontend-mockup/README.md) — el backlog FWP03 sigue propuesto.
- [`../tasks/frontend/frontend-mockup/README.md:70-73`](../tasks/frontend/frontend-mockup/README.md) — aceptación y pruebas propuestas (FWP03).
- [`../../backend/docs/architecture.md:34`](../../backend/docs/architecture.md) — servicio futuro.

## Specs

- **S1** — Cada operación preventiva se vincula a comunidad/sector y, si está validado, a objetivo/acción del plan preventivo.
- **S2** — Cada operación registra responsable/equipo, fecha límite e historial atribuible.
- **S3** — El ciclo cubre acuse, avance, bloqueo, envío, aceptación, devolución, reapertura y cancelación.
- **S4** — Las transiciones válidas e inválidas se prueban **antes** de implementar el servicio.

## Tasks

| ID | Spec | Ruta | Descripción | Commit |
| --- | --- | --- | --- | --- |
| T1 | S1,S2 | inline | Validar el modelo de operación (vocabulario, vínculos, responsable, historial) con las partes. | — |
| T2 | S3 | inline | Formalizar la máquina de estados del ciclo preventivo. | — |
| T3 | S4 | inline | Definir pruebas de transiciones válidas e inválidas. | — |
| T4 | S1-S4 | inline | Implementar el servicio `prevention` **solo tras aprobación de alcance**. | — |

Ruta por defecto: **inline**. T4 queda bloqueado hasta aprobación de alcance.

## Dependencias y bloqueos humanos

- **Bloqueo humano:** revisar y aprobar la propuesta FWP03; su existencia no autoriza implementación.
- Depende de FWP01–02.

## Criterios de cierre

- `../tasks/frontend/frontend-mockup/README.md:71-73`: aceptación y pruebas propuestas; backlog pendiente de revisión en `:46-48`.

## Log

- **L1 (2026-10-16)** — Creación del desglose por spec a partir de [`README.md`](README.md) §1 (frente 3). Documento de planificación; sin implementación.
