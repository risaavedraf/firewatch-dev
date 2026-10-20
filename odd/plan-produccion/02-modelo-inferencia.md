# Frente 2 — Servicio real de inferencia y modelo aprobado

- **Grupo de entrega:** G2 — Núcleo de datos y modelo (solo el *seam*).
- **Estado:** Parcial — el clasificador por defecto `academic-rule-v1` quedó entregado en la presentación (modelo v2). El servicio de inferencia con modelo entrenado y aprobado sigue pendiente (pendiente permitido).
- **Riesgo (Task Size):** alto al implementar — ítem 1 (versión/procedencia persistida) e ítem 3 (contratos `Classifier`/`Provider`).
- **Depende de:** frente 1 (observaciones); disponibilidad de modelo y política de aprobación (decisión humana).
- **Seam / superficie:** `backend/services/risk/internal/classify/` (`classifier.go`, `academic_rule.go`); interfaces `Classifier`/`Provider`; persistencia `models` y publicación de lotes.

**Objetivo.** Sustituir la regla académica por inferencia con un modelo aprobado, trazable y utilizable en producción, dejando la frontera `Classifier` estable.

**Documentación real (redirección).**
- [`../../Referencias/EP1_Equipo3_Armijo_Calderon_Saavedra2_produccion.md:30,46-56,80`](../../Referencias/EP1_Equipo3_Armijo_Calderon_Saavedra2_produccion.md) — contrato `Provider`/`Classifier`, metadatos y aprobación.
- [`../../Referencias/Actividad C4_FireWatch_produccion.md:59-65,79-91,171-199,247-248`](<../../Referencias/Actividad C4_FireWatch_produccion.md>) — operación objetivo.
- [`../tasks/backend/firewatch-data-contract.md:18-23,68`](../tasks/backend/firewatch-data-contract.md) — define `academic-rule-v1`; los datos sintéticos no son un modelo validado.
- [`../../backend/docs/verification.md:65`](../../backend/docs/verification.md) — límite declarado.

## Specs

- **S1** — La inferencia consume un modelo **aprobado** y rechaza entradas incompletas o inválidas.
- **S2** — Cada resultado se persiste con zona, período, fecha, **versión del modelo** y datos/procedencia utilizados.
- **S3** — Existe un procedimiento para aprobar o **revertir** versiones de modelo antes de producción.
- **S4** — La frontera `Classifier`/`Provider` queda congelada con una implementación por defecto (regla académica) determinista para las suites actuales.

## Tasks

| ID | Spec | Ruta | Descripción | Commit |
| --- | --- | --- | --- | --- |
| T1 | S4 | inline | Congelar interfaces `Classifier`/`Provider` y la implementación por defecto `academic-rule-v1`. | — |
| T2 | S1 | inline | Definir el contrato de entrada y el rechazo de observaciones incompletas/inválidas. | — |
| T3 | S2 | inline | Persistir cada resultado con zona, período, fecha, versión de modelo y procedencia. | — |
| T4 | S3 | inline | Definir y documentar el procedimiento de aprobación/reversión de versiones. | — |
| T5 | S1,S4 | inline | *Seam* de sustitución del modelo real sin tocar el resto del sistema. | — |
| T6 | S4 | inline | Prueba de sustitución: cambiar el clasificador por configuración y demostrar que ningún archivo fuera de `backend/services/risk/internal/classify/` cambia. | — |

Ruta por defecto: **inline** (seam + fixtures). La sustitución real del modelo queda **fuera** de este frente.

## Dependencias y bloqueos humanos

- Aprobar modelo, métricas/umbrales y reglas de reversión (bloqueo humano): las fuentes exigen aprobación admin, pero no fijan esos umbrales.
- Depende del frente 1 para observaciones reales.

## Criterios de cierre

- Metadatos y aprobación: `../../Referencias/EP1_Equipo3_Armijo_Calderon_Saavedra2_produccion.md:54-56`.
- La prueba de sustitución (T6) pasa sin cambios fuera de `backend/services/risk/internal/classify/`, y `academic-rule-v1` sigue siendo determinista.
- `../../backend/docs/verification.md:65` declara que precisión/calibración y publicación por ruta productiva **no están establecidas**.

## Log

- **L1 (2026-10-16)** — Creación del desglose por spec a partir de [`README.md`](README.md) §1 (frente 2). Documento de planificación; sin implementación.
- **L2 (2026-10-20)** — Estado corregido a Parcial: `academic-rule-v1` entregado (modelo v2); el modelo entrenado sigue pendiente por decisión (README §1 y §7).
- **L3 (2026-10-20)** — Se agrega T6 (prueba de sustitución) y su criterio de cierre; el frente puede completarse sin modelo real según README §5.1.
