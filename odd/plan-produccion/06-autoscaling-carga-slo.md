# Frente 6 — Autoscaling/HPA, carga y medición p95/SLO

- **Grupo de entrega:** G4 — Operación.
- **Estado:** Parcial — S1 acordado; artefactos de S2–S5 escritos (k6, HPA, runbook). La ejecución (benchmark, escalado, RTO) sigue pendiente de un entorno aislado y de un clúster.
- **Riesgo (Task Size):** alto al implementar — ítem 5 (infraestructura de despliegue/escalado).
- **Depende de:** SLO/SLA, carga y RTO acordados; Kubernetes confirmado como destino.
- **Seam / superficie:** objetivos p95/RPS y configuración de despliegue Kubernetes/autoscaling; `backend/docs/architecture.md:61-63`.

**Objetivo.** Acordar objetivos medibles y demostrar carga, escalado y recuperación con benchmark reproducible.

**Documentación real (redirección).**
- [`../../Referencias/tareas_verificacion_implementacion_IA.md:289-301,320-328,348-357`](../../Referencias/tareas_verificacion_implementacion_IA.md) — criterios por ID.
- [`../../Referencias/Actividad Dimensionamiento_FireWatch_produccion.md:34-44,77-81,153-155`](<../../Referencias/Actividad Dimensionamiento_FireWatch_produccion.md>) — metas iniciales (p95 < 500 ms bajo carga; máximo inicial de 2 s en condiciones normales; 20 usuarios concurrentes y 4 RPS).
- [`../urgent/04-operacion-monitoreo.md:9-25`](../urgent/04-operacion-monitoreo.md) — acuerdo previo a medir.
- [`../../backend/docs/architecture.md:61-63`](../../backend/docs/architecture.md) — destino Kubernetes.

## Specs

- **S1** — El SLO/SLA se acuerda y cada cifra se marca como **objetivo** o **medición** (los ejemplos `<200 ms` y `500 req/s` **no** son objetivos FireWatch). Acordado en L2; todos los valores son **objetivo**, ninguno es medición todavía.
- **S2** — Se ejecuta benchmark de carga representativa (p. ej. 20 usuarios concurrentes y 4 RPS), midiendo p95/p99, RPS y errores, con reporte pasa/falla.
- **S3** — Se define política de autoscaling y se comprueba escalado horizontal con carga.
- **S4** — Se acuerda y mide el RTO frente al objetivo.
- **S5** — Se valida la capacidad inicial indicada para API/Web, worker y réplicas de predicción.

## Tasks

| ID | Spec | Ruta | Descripción | Commit |
| --- | --- | --- | --- | --- |
| T1 | S1 | inline | Acordar SLO/SLA y marcar objetivo vs medición. | — |
| T2 | S2 | inline | Elegir herramienta y ejecutar benchmark reproducible. | — |
| T3 | S3 | inline | Definir política de autoscaling y verificar escalado. | — |
| T4 | S4 | inline | Acordar RTO y medir recuperación real. | — |
| T5 | S5 | inline | Validar capacidad inicial con pruebas de carga. | — |

Ruta por defecto: **inline** para definición; la ejecución del benchmark (larga) puede delegarse por regla por-acción.

**Pendientes al cierre de la Sesión B (2026-10-16).** El frente 6 es de **medición**: no le queda código por escribir, sino artefactos por ejecutar. Lo escrito y su evidencia estructural están en el `## Log` (L2–L3).

- **S1** — hecho: acuerdo registrado (L2).
- **S2** — `backend/deploy/load/k6-risk.js` escrito; **falta ejecutarlo** y producir el reporte pasa/falla.
- **S3** — `backend/deploy/k8s/base/autoscaling.yaml` escrito; **falta verificar** el escalado horizontal con carga en un clúster.
- **S4** — objetivo RTO ≤ 15 min documentado; **falta medirlo** con una prueba de recuperación.
- **S5** — plan de validación documentado; **falta ejecutarlo**.

Todo lo anterior requiere **entorno**: stack aislado (Docker + k6 + Prometheus) para S2/S5 y un **clúster de Kubernetes** con `metrics-server` para S3/S4.

## Dependencias y bloqueos humanos

- **Bloqueo humano:** acordar SLO/SLA, carga, RTO y herramientas **antes** de medir.
- No se identifica aquí una configuración HPA concreta; el path de HPA debe determinarse.

## Criterios de cierre

- Benchmark reproducible y comparación medido vs objetivo pasa/falla: `../../Referencias/tareas_verificacion_implementacion_IA.md:298-301`.
- Acuerdo previo a medir: `../urgent/04-operacion-monitoreo.md:21,25`.

## Log

- **L1 (2026-10-16)** — Creación del desglose por spec a partir de [`README.md`](README.md) §1 (frente 6). Documento de planificación; sin implementación.
- **L2 (2026-10-16)** — Acuerdo de operación aprobado por el usuario; cierra el bloqueo humano de S1. Cada cifra queda marcada como **objetivo** (no medición), tomada de [`Actividad Dimensionamiento_FireWatch_produccion.md`](<../../Referencias/Actividad Dimensionamiento_FireWatch_produccion.md>):
  - **Latencia (consulta de riesgo/mapa):** p95 < **500 ms** bajo carga; máximo **2 s** en condiciones normales.
  - **Throughput:** **4 RPS** en el pico; escenario de **20 usuarios concurrentes** (1 consulta cada 5 s).
  - **Ciclo de datos:** actualización cada **15 min** + prueba de actualización.
  - **Capacidad inicial:** API/Web **2 réplicas**, 8 GB y 2–4 vCPU cada una; réplica de predicción 4 GB y 2 vCPU; worker 16 GB y 4 vCPU.
  - **Herramientas:** carga con **k6**; observabilidad con **Prometheus + Grafana**; drift con **Evidently AI**.
  - **Valores sin fuente en la documentación, fijados por decisión del usuario:** SLA de disponibilidad **99.5 %**; RTO **≤ 15 min**; umbral de latencia p95 > 500 ms sostenido 5 min; umbral de errores 5xx > 1 % sostenido 5 min; reevaluación de drift por lote, mínimo diaria.
  - Ninguna cifra es medición: la ejecución del benchmark y la comprobación de escalado/RTO (S2–S5) siguen pendientes de un entorno aislado y de un clúster de destino.
- **L3 (2026-10-16)** — Artefactos de los frentes S2–S5 escritos (Sesión B). Todos son reproducibles; **ninguno se ha ejecutado todavía**, así que no convierten los objetivos en mediciones.
  - **S2 — benchmark:** `backend/deploy/load/k6-risk.js` reproduce el escenario acordado (tasa constante 4 RPS, 20 VUs) contra `GET /v1/risk/latest?zone_id=valparaiso-demo` por el gateway, con umbrales pasa/falla `p(95)<500`, `max<2000`, `http_req_failed<0.01`. El token se pasa por entorno (`TOKEN`), nunca en el repositorio. Sin ejecutar.
  - **S3 — autoscaling:** `backend/deploy/k8s/base/autoscaling.yaml` con un `HorizontalPodAutoscaler` por servicio (2→6 réplicas, CPU al 70 % de la request, scale-up 60 s / scale-down 300 s), añadido a `kustomization.yaml`. Escala sobre medición, no sobre una carga adivinada. La comprobación de escalado real con carga queda pendiente de clúster.
  - **S4 — RTO:** objetivo ≤ 15 min documentado en `backend/docs/operations.md` §6, junto con la base que lo sostiene (2 réplicas, `PodDisruptionBudget` `minAvailable: 1`, rollout). La prueba de recuperación (matar una réplica o una dependencia y medir) no se ejecutó.
  - **S5 — capacidad:** plan de validación documentado (§8); la capacidad inicial sigue siendo objetivo, no resultado.
  - **Evidencia observada (estructural):** `autoscaling.yaml` parsea con `yaml.safe_load_all` (3 documentos) y `kustomization.yaml` es válido; el script k6 pasa `node --check` (sintaxis) con Node v24.11.1. **No ejecutado:** no hay clúster, ni stack levantado, ni k6 en este entorno. Se declara.
