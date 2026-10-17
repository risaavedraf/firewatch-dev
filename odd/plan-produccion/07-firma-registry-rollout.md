# Frente 7 — Firma de imágenes, promoción de registry y rollout

- **Grupo de entrega:** G1 — Base de entrega.
- **Estado:** Pendiente — planificación. La CI ya produce SBOM/provenance; falta firma, promoción y despliegue productivo verificado.
- **Riesgo (Task Size):** alto al implementar — ítem 5 (release/deploy/CI) e ítem 2 (claves de firma).
- **Depende de:** registry, identidad/llaves de firma, pipeline de promoción y clúster Kubernetes.
- **Seam / superficie:** CI/build de imágenes; registry; manifiestos Kubernetes (`backend/README.md:98`).

**Objetivo.** Establecer una cadena verificable de imágenes desde build/registry hasta despliegue Kubernetes, con rollback.

**Documentación real (redirección).**
- [`../../Referencias/Actividad C4_FireWatch_produccion.md:229-273`](<../../Referencias/Actividad C4_FireWatch_produccion.md>) — firma, promoción y rollout objetivo.
- [`../../backend/docs/architecture.md:57`](../../backend/docs/architecture.md) — ausencia de firma/promoción.
- [`../../backend/docs/verification.md:62-68`](../../backend/docs/verification.md) — estado de firma/rollout.
- [`../../backend/README.md:98`](../../backend/README.md) — base Kubernetes de staging.

## Specs

- **S1** — Las imágenes se firman y su firma se verifica **antes** de permitir promoción/despliegue.
- **S2** — La promoción entre registry/entornos está definida y automatizada.
- **S3** — El rollout en Kubernetes se automatiza y verifica resultado y rollback.
- **S4** — Se ejecuta verificación real del rollout y del comportamiento ante fallo.

## Tasks

| ID | Spec | Ruta | Descripción | Commit |
| --- | --- | --- | --- | --- |
| T1 | S1 | inline | Firmar imágenes y verificar firma en el pipeline. | — |
| T2 | S2 | inline | Definir y automatizar la promoción entre entornos. | — |
| T3 | S3 | inline | Automatizar rollout y verificar resultado/rollback. | — |
| T4 | S4 | inline | Ejecutar verificación real del rollout y del fallo. | — |

Ruta por defecto: **inline**; el rollout/Kubernetes (larga duración) puede delegarse por regla por-acción.

## Dependencias y bloqueos humanos

- **Bloqueo humano:** determinar registry, confianza/gestión de claves, entornos y política de promoción (las fuentes no fijan esos valores).
- Depende de la base Kubernetes; no fue desplegada como producción.

## Criterios de cierre

- `../../backend/docs/architecture.md:57` y `../../backend/docs/verification.md:68` (firma/promoción ausentes); `../../backend/docs/verification.md:64` (rollout no verificado).

## Log

- **L1 (2026-10-16)** — Creación del desglose por spec a partir de [`README.md`](README.md) §1 (frente 7). Documento de planificación; sin implementación.
