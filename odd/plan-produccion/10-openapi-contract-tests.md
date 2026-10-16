# Frente 10 — OpenAPI como fuente de verdad y contract tests

- **Grupo de entrega:** G3 — Robustez de backend.
- **Estado:** Pendiente — planificación. El OpenAPI consta validado **solo sintácticamente**.
- **Riesgo (Task Size):** alto al implementar — ítem 3 (contrato publicado que otros consumen).
- **Depende de:** BH-01 (tests ejecutables); coordinar con BH-03; **BH-12 no es requisito**.
- **Seam / superficie:** `contracts/openapi/firewatch-v1.yaml`, handlers y gateway.

**Objetivo.** Alinear el contrato OpenAPI con las respuestas observables y probar esa paridad con rutas reales.

**Documentación real (redirección).**
- [`../tasks/security/backend-hardening/08-openapi-errors.md:7-20`](../tasks/security/backend-hardening/08-openapi-errors.md) — criterios de aceptación (BH-08).
- [`../tasks/security/backend-hardening.md:24`](../tasks/security/backend-hardening.md) — encuadre del plan de hardening.

> Nota de solape: este frente **es** la unidad BH-08 del frente 11. Se mantiene como frente propio por su rol de *fuente de verdad*; al implementar, coordinar con el frente 11 para no duplicar.

## Specs

- **S1** — Se prepara la matriz operación-error según el comportamiento actual; GET 503 y gateway 502/429 sólo donde apliquen.
- **S2** — Se reutilizan esquemas de error y se describe cuerpo, headers y request ID observables.
- **S3** — Se añaden pruebas de contrato contra rutas reales de dependencia no disponible, fallo *upstream* y rate limit.
- **S4** — BH-12 permanece diferido: no es requisito para cerrar la paridad.

## Tasks

| ID | Spec | Ruta | Descripción | Commit |
| --- | --- | --- | --- | --- |
| T1 | S1 | inline | Construir la matriz operación-error desde el comportamiento vigente. | — |
| T2 | S2 | inline | Alinear esquemas de error (cuerpo/headers/request ID). | — |
| T3 | S3 | inline | Añadir contract tests contra rutas reales. | — |

Ruta por defecto: **inline**. Delegar el contrato a un verificador solo si el diff nombra un ítem de riesgo alto.

## Dependencias y bloqueos humanos

- **Bloqueo humano:** confirmar aplicabilidad por operación a partir del comportamiento vigente; **no inventar garantías**.
- Depende de BH-01 para tests ejecutables.

## Criterios de cierre

- `../tasks/security/backend-hardening/08-openapi-errors.md:11-20`.

## Log

- **L1 (2026-10-16)** — Creación del desglose por spec a partir de [`README.md`](README.md) §1 (frente 10). Documento de planificación; sin implementación.
