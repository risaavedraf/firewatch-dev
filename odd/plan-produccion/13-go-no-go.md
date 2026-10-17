# Frente 13 — Go/No-Go y firma de aprobación de producción

- **Grupo de entrega:** No cierra esta semana — **bloqueado por los dos pendientes permitidos** (datos reales + modelo).
- **Estado:** Bloqueado — planificación.
- **Riesgo (Task Size):** alto — ítem 6 (no se declara aprobación sin evidencia reproducible).
- **Depende de:** evidencia de datos y modelo, reportes de infraestructura y criterios acordados.
- **Seam / superficie:** matriz Go/No-Go, reportes enlazados y registro de versiones.

**Objetivo.** Registrar una decisión de producción basada en evidencia y **firmada** por el responsable. **Se prepara como plan, no como resultado.**

**Documentación real (redirección).**
- [`../urgent/05-ct-aprobacion-demo.md:3-23`](../urgent/05-ct-aprobacion-demo.md) — matriz y firma.
- [`../../Referencias/tareas_verificacion_implementacion_IA.md:457-476`](../../Referencias/tareas_verificacion_implementacion_IA.md) — criterios por ID.

## Specs

- **S1** — Existe historial de versiones y matriz de pruebas con criterio, evidencia y resultado.
- **S2** — Se registran versiones, resultados y verificaciones de datos, modelo e infraestructura.
- **S3** — La decisión de despliegue se documenta como **APROBADO PARA PRODUCCIÓN** o **RECHAZADO**.
- **S4** — Se obtiene y registra la firma del responsable; no se declara aprobación antes de contar con la evidencia requerida.

## Tasks

| ID | Spec | Ruta | Descripción | Commit |
| --- | --- | --- | --- | --- |
| T1 | S1 | *bloqueada* | Completar historial de versiones y matriz de pruebas. | — |
| T2 | S2 | *bloqueada* | Enlazar verificaciones de datos/modelo/infraestructura. | — |
| T3 | S3 | *bloqueada* | Documentar decisión APROBADO/RECHAZADO. | — |
| T4 | S4 | *bloqueada* | Registrar la firma del responsable. | — |

Ruta por defecto: **bloqueada**. No iniciar hasta reunir la evidencia requerida.

## Dependencias y bloqueos humanos

- **Bloqueo doble:** datos reales y modelo aprobado (los dos pendientes permitidos).
- **Bloqueo humano:** el responsable debe decidir y firmar; sin firma no hay aprobación de producción.

## Criterios de cierre

- `../urgent/05-ct-aprobacion-demo.md:14-23`; `../../Referencias/tareas_verificacion_implementacion_IA.md:459-476`.

## Log

- **L1 (2026-10-16)** — Creación del desglose por spec a partir de [`README.md`](README.md) §1 (frente 13). Documento de planificación; sin implementación.
