# Frente 1 — Ingesta de fuentes reales y ciclo/scheduler de 15 min

- **Grupo de entrega:** G2 — Núcleo de datos y modelo (solo el *seam*).
- **Estado:** Parcial — el contrato `Provider`/`ObservationSet`/`Provenance` y el fixture determinista quedaron entregados en la presentación (2026-10-18). La ingesta de fuentes reales y el ciclo de 15 min con datos reales siguen pendientes (pendiente permitido).
- **Riesgo (Task Size):** alto al implementar — ítem 1 (datos/procedencia persistidos) e ítem 3 (contrato `Provider` que otros consumen).
- **Depende de:** acceso y periodicidad real de las fuentes (decisión humana); frente 2 (misma frontera de observaciones).
- **Seam / superficie:** `backend/services/risk/internal/source/` (`provider.go`, `fixture.go`); interfaz `Provider`; `contracts/gen/observations/v1` (`Observation`, `ObservationSet`, `Provenance`); migración `007_synthetic_evaluation.sql`.

**Objetivo.** Dejar congelada la frontera de ingesta (contrato + implementación por defecto determinista) para enchufar las fuentes reales del ciclo de 15 min sin tocar el resto del sistema.

**Documentación real (redirección).** Este documento no reescribe la fuente:
- [`../../Referencias/EP1_Equipo3_Armijo_Calderon_Saavedra2_produccion.md:42-45,88,102-104`](../../Referencias/EP1_Equipo3_Armijo_Calderon_Saavedra2_produccion.md) — fuentes, validación, requisito de ciclo/frescura.
- [`../../Referencias/Actividad C4_FireWatch_produccion.md:151-169,193-199,245-246`](<../../Referencias/Actividad C4_FireWatch_produccion.md>) — proceso automático.
- [`../tasks/backend/firewatch-data-contract.md:7-9,16-19,25-28`](../tasks/backend/firewatch-data-contract.md) — alcance del contrato; excluye la descarga real por decisión del usuario.
- [`../../backend/README.md:96-98`](../../backend/README.md) — estado operativo.
- [`../../backend/docs/verification.md:65`](../../backend/docs/verification.md) — límite declarado.

## Specs

- **S1** — El sistema ingiere fuentes reales de incendios y meteorología, valida formato/fecha/coordenadas/duplicados/faltantes antes de usarlos, y conserva la procedencia por referencia.
- **S2** — El ciclo automático dispara ingesta → validación → inferencia → publicación **cada 15 min** (frecuencia condicionada a la disponibilidad real de las fuentes).
- **S3** — No se publican lotes incompletos ni inválidos; se expone estado y frescura de los datos.
- **S4** — El contrato queda congelado en `Provider` / `ObservationSet` / `Provenance`, con una implementación por defecto (fixture) determinista que las suites usan hoy.

## Tasks

| ID | Spec | Ruta | Descripción | Commit |
| --- | --- | --- | --- | --- |
| T1 | S4 | inline | Congelar interfaz `Provider` y `fixture.go` como implementación por defecto determinista. | — |
| T2 | S1 | inline | Definir y documentar la validación por fuente (formato, fechas, coordenadas, duplicados, faltantes). | — |
| T3 | S1 | inline | Registrar procedencia/linaje por observación (`Provenance`). | — |
| T4 | S2 | inline | Scheduler de 15 min que dispare el ciclo completo, con seam de sustitución de fuente. | — |
| T5 | S3 | inline | Rechazar lotes incompletos/inválidos y exponer estado/frescura. | — |
| T6 | S4 | inline | Prueba de sustitución: cambiar el proveedor por configuración y demostrar que ningún archivo fuera de `backend/services/risk/internal/source/` cambia. | — |

Ruta por defecto: **inline** (seam + fixtures). Delegar escritura solo por disparador (paralelismo/contexto).

## Dependencias y bloqueos humanos

- Autorizar y validar el acceso y la periodicidad real de las fuentes (bloqueo humano).
- Coordinar con el frente 2: comparten la frontera `Provider`/`ObservationSet`.

## Criterios de cierre

- Requisito de ciclo y frescura: `../../Referencias/EP1_Equipo3_Armijo_Calderon_Saavedra2_produccion.md:88,104`.
- La prueba de sustitución (T6) pasa sin cambios fuera de `backend/services/risk/internal/source/`, y el fixture es determinista con semilla.
- `../../backend/docs/verification.md:65` declara que scheduler y datos reales **no están establecidos**: no marcar cierre sin evidencia reproducible y umbrales acordados.

## Log

- **L1 (2026-10-16)** — Creación del desglose por spec a partir de [`README.md`](README.md) §1 (frente 1). Documento de planificación; sin implementación.
- **L2 (2026-10-20)** — Estado corregido a Parcial: seam y fixture entregados en la presentación; la fuente real sigue pendiente por decisión (README §1 y §7).
- **L3 (2026-10-20)** — Se agrega T6 (prueba de sustitución) y su criterio de cierre; el frente puede completarse sin fuente real según README §5.1.
