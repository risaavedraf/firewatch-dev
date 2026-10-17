# Frente 8 — Gobierno, seguridad y protección de datos/modelo (U1)

- **Grupo de entrega:** G3 — Robustez de backend.
- **Estado:** Pendiente — planificación. No acredita implementación ni evidencia.
- **Riesgo (Task Size):** alto al implementar — ítem 2 (seguridad: auth, permisos, secretos, guardas).
- **Depende de:** responsables y decisiones de clasificación/aprobación/controles aplicables.
- **Seam / superficie:** gobierno de modelos, datos/datasets, API y pipeline de inferencia, contenedores, observabilidad y respuesta operacional.

**Objetivo.** Reunir amenazas, controles de acceso, procedencia, privacidad y operación segura en una unidad verificable.

**Documentación real (redirección).**
- [`../urgent/01-seguridad-gobierno.md:3-23`](../urgent/01-seguridad-gobierno.md) — unidad U1 y criterio de cierre.
- [`../../Referencias/tareas_verificacion_implementacion_IA.md:51-187`](../../Referencias/tareas_verificacion_implementacion_IA.md) — requisitos por ID.

## Specs

- **S1** — Se define clasificación de riesgo, responsable de seguridad IA y aprobación previa a producción; se verifica consistencia regulatoria.
- **S2** — Se modelan amenazas IA con MITRE ATLAS, vinculando controles y justificando cada N/A.
- **S3** — Se documentan segmentación, autenticación, mínimo privilegio, linaje e integridad de fuentes y auditoría de dependencias/modelos.
- **S4** — Se evalúa sensibilidad, anonimización/privacidad y riesgo de reidentificación; se mide robustez adversaria.
- **S5** — Se verifican guardrails, aislamiento, alertas de deriva, red teaming, rollback y respuesta a incidentes.
- **S6** — Para cada ID existe estado, evidencia, criterio de aprobación y resultado; cada N/A se justifica.

## Tasks

| ID | Spec | Ruta | Descripción | Commit |
| --- | --- | --- | --- | --- |
| T1 | S1 | inline | Definir clasificación de riesgo, responsable y aprobación previa a producción. | — |
| T2 | S2 | inline | Modelar amenazas con MITRE ATLAS y vincular controles. | — |
| T3 | S3 | inline | Documentar segmentación, autenticación, mínimo privilegio y linaje. | — |
| T4 | S4 | inline | Evaluar privacidad/reidentificación y robustez adversaria. | — |
| T5 | S5 | inline | Verificar guardrails, aislamiento, deriva, red teaming y respuesta. | — |
| T6 | S6 | inline | Registrar estado/evidencia/criterio/resultado por ID (41 IDs U1). | — |

Ruta por defecto: **inline** (trabajo documental/de gobierno). Delegar solo por disparador.

## Dependencias y bloqueos humanos

- **Bloqueo humano:** designar responsable y aprobar política/categoría; justificar cada N/A.
- Depende de los requisitos por ID y de los pendientes permitidos (datos/modelo) para los ID que los exigen.

## Criterios de cierre

- `../urgent/01-seguridad-gobierno.md:21-23`.

## Log

- **L1 (2026-10-16)** — Creación del desglose por spec a partir de [`README.md`](README.md) §1 (frente 8). Documento de planificación; sin implementación.
