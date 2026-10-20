# Frente 5 — Observabilidad productiva y métricas de modelo

- **Grupo de entrega:** G4 — Operación.
- **Estado:** Parcial — S5 y S1 implementados, S2 implementado en los cuatro servicios (`internal/metrics/http.go` y `process.go` en gateway, jobs, risk y prevention), S3/S4 documentados con reglas de alerta. La medición (RPS/p95/memoria/errores/distribución) sigue pendiente de entorno.
- **Riesgo (Task Size):** alto al implementar — ítem 5 (operación/observabilidad) e ítem 6 (señales sin *ground truth*).
- **Depende de:** *ground truth* para evaluar calidad; umbrales y herramientas acordados.
- **Seam / superficie:** `/metrics`; logging de inputs/outputs/ground truth; Prometheus → Grafana; `backend/docs/architecture.md:55-56`.

**Objetivo.** Completar observabilidad que correlacione modelo, servicio e infraestructura, separando señales operativas de métricas que exigen *ground truth*.

**Documentación real (redirección).**
- [`../urgent/04-operacion-monitoreo.md:3-5,15-25`](../urgent/04-operacion-monitoreo.md) — configuración y reporte reproducible.
- [`../../Referencias/tareas_verificacion_implementacion_IA.md:363-396,398-411,484-491`](../../Referencias/tareas_verificacion_implementacion_IA.md) — criterios por ID.
- [`../../backend/docs/architecture.md:55-56`](../../backend/docs/architecture.md) — contadores internos actuales.

## Specs

- **S1** — Se registran inputs, outputs y *ground truth* de forma que permita reevaluar calidad y cambios.
- **S2** — `/metrics` cubre demanda, latencia, recursos y señales/calidad del modelo (prioridad: CPU, RAM, latencia, RPS, errores, PostGIS).
- **S3** — Se define herramienta, prueba estadística y frecuencia de reevaluación de drift, y se compara con *ground truth*.
- **S4** — Cada alerta conduce a una acción concreta; existen umbrales y acciones de respuesta acordados.
- **S5** — Se completa `high_risk_predictions_total` y el análisis de RPS, p95, memoria, errores y distribución de riesgo.

## Tasks

| ID | Spec | Ruta | Descripción | Commit |
| --- | --- | --- | --- | --- |
| T1 | S1 | inline | Completar logging de inputs/outputs/ground truth. | — |
| T2 | S2 | inline | Extender `/metrics` con las señales acordadas. | — |
| T3 | S3 | inline | Definir y verificar método/frecuencia de drift. | — |
| T4 | S4 | inline | Definir umbrales y acciones; verificar que cada alerta accione. | — |
| T5 | S5 | inline | Implementar `high_risk_predictions_total` y el análisis asociado. | — |

Ruta por defecto: **inline**. La carga y las suites largas pueden delegarse por regla por-acción.

**Pendientes al cierre de la Sesión B (2026-10-16).** Lo implementado y verificado está en el `## Log` (L3–L5). Puntos abiertos, cada uno con su causa:

- **T1** — inputs/outputs hechos; **`LOG-I3` (ground truth) pendiente**: bloqueado por el pendiente permitido (fuente de datos real), no por entorno. El punto de unión es `job_id`.
- **T2** — implementado en `risk`; **falta instrumentar `gateway` y `jobs`** con latencia/errores/recursos. Es **código pendiente y NO requiere entorno**; cada servicio es un módulo Go propio, así que implica duplicar `internal/metrics`.
- **T3** — método/frecuencia documentados; **falta la comparación de calidad contra ground truth**: bloqueada por el dato real.
- **T4** — reglas y acciones escritas; **falta la validación semántica** con `promtool`/Prometheus (requiere entorno).
- **T5** — contador hecho; **falta el análisis** de RPS/p95/memoria/errores/distribución (requiere entorno).

## Dependencias y bloqueos humanos

- **Bloqueo humano:** acordar herramientas, método/frecuencia de drift, umbrales y respuesta; reentrenar solo con evidencia.
- Requiere *ground truth* real (ligado a los pendientes permitidos) para las métricas de calidad.

## Criterios de cierre

- `../urgent/04-operacion-monitoreo.md:19-21` (configuración y reporte reproducible, cifras identificadas como objetivo o medición).
- `../../Referencias/tareas_verificacion_implementacion_IA.md:394-411` (interpretación y acciones verificadas).

## Log

- **L1 (2026-10-16)** — Creación del desglose por spec a partir de [`README.md`](README.md) §1 (frente 5). Documento de planificación; sin implementación.
- **L2 (2026-10-16)** — Acuerdo de observabilidad aprobado por el usuario; cierra el bloqueo humano de S3/S4 y desbloquea la implementación.
  - **Herramientas:** flujo `aplicación (/metrics) → Prometheus (scrape + series) → Grafana (dashboards y alertas)`. Drift con **Evidently AI**, distinguiendo **Data Drift** de **Concept Drift**; no se activa reentrenamiento solo por un cambio de distribución.
  - **Señales prioritarias de `/metrics`:** CPU, RAM, latencia, RPS, errores y acceso a datos (PostGIS). **No** se prioriza GPU/VRAM: el modelo es tabular sobre CPU.
  - **Umbrales de alerta y acciones (fijados por el usuario):**
    - p95 de latencia > **500 ms sostenido 5 min** → investigar (código, PostGIS, red, memoria, dependencias). Si sube el p95 **sin** más RPS, el problema no es la carga.
    - Errores 5xx > **1% sostenido 5 min** → investigar.
    - Backlog de DLQ > 0 → revisar la DLQ (ya señalizado en readiness y `/metrics`).
    - Reentrenar **solo** con evidencia de ground truth.
  - **Frecuencia de reevaluación de drift:** por lote, **mínimo diaria**; antes de ground truth la reevaluación es sobre distribución de entradas, no sobre calidad.
  - Ninguna cifra es medición: el análisis de RPS/p95/memoria/errores/distribución (S5) sigue pendiente de un entorno aislado.
- **L3 (2026-10-16)** — Implementación de S5 y S1 en el servicio `risk` (Sesión B).
  - **S5:** nuevo paquete `backend/services/risk/internal/metrics` con `RiskOutcomes`. `firewatch_high_risk_predictions_total` y la distribución `firewatch_risk_predictions_total{level="bajo|medio|alto|desconocido"}` se exponen en `/metrics`. El contador se puebla desde `Publisher.Publish` (nuevo seam `RiskObserver`), **solo tras persistir** el lote: una publicación fallida se reintenta entera, así que contarlas antes reportaría un nivel descartado. Un nivel no canónico se cuenta como `desconocido`, nunca se descarta.
  - **S1 (parcial):** `Publisher.Publish` registra una línea estructurada `risk_assessed` con job, zona, proveedor, `synthetic`, `model_version`, `params_hash`, `risk_level` y las cinco entradas (`temperatura_c`, `humedad_relativa_pct`, `viento_kmh`, `cobertura_vegetal_pct`, `incendios_historicos`); una entrada ausente se registra como `"unknown"`, nunca como 0. **`LOG-I3` (ground truth) sigue pendiente** porque el dato real está diferido; se unirá por `job_id`.
  - **Evidencia observada:** `gofmt -l .` sin salida; `go build ./...` y `go vet ./...` correctos; `go test ./... -count=1` verde en los 10 paquetes. Pruebas nuevas (todas ejecutadas, no saltadas): `internal/metrics` (`TestRiskOutcomesCountsEachLevel`, `TestRiskOutcomesCountsAnUnknownLevelAsDesconocido`), `internal/service` (`TestPublisherReportsTheOutcomeOnlyAfterPersistence`, `TestPublisherDoesNotReportAnOutcomeWhenPersistenceFails`) y `cmd/server` (`TestMetricsKeepsReportingBrokerAndDeadLetterTruth`, ampliada para pinchar las líneas nuevas del contador y la distribución).
- **L4 (2026-10-16)** — Implementación de **S2** en el servicio `risk`.
  - `/metrics` gana tres bloques nuevos, todos renderizados tras los contadores de entrega de `messaging.MetricsText` (que quedan intactos y pinados por su propia prueba):
    - **Servicio:** `firewatch_http_responses_total{class="2xx|3xx|4xx|5xx"}` y `firewatch_http_errors_total` (la serie 5xx contra la que se escribe el umbral de error acordado), más el histograma `firewatch_http_request_duration_seconds` con buckets acumulativos (de 5 ms a 10 s) que habilita p50/p95 vía `histogram_quantile`.
    - **Infraestructura:** `firewatch_process_goroutines`, `firewatch_process_cpu_seconds_total` y `firewatch_process_memory_bytes`, leídos de `runtime/metrics` en cada scrape. Sin GPU: el modelo es tabular y corre en CPU.
    - **Acceso a datos (PostGIS):** `firewatch_db_pool_{total,acquired,idle,max}_conns` desde `pgxpool.Stat()`. **RPS** ya está cubierto por `firewatch_http_requests_total` (`rate()`).
  - La instrumentación HTTP es un `statusRecorder` en el middleware exterior, así que no cambia ningún handler. Un código fuera de rango se cuenta como `unknown` y una respuesta sin `WriteHeader` se cuenta como 200: las clases suman todo lo registrado.
  - **Evidencia observada:** `gofmt -l .` sin salida; `go build ./...` y `go vet ./...` correctos; `go test ./... -count=1` verde en los 10 paquetes. Pruebas nuevas ejecutadas: `TestHTTPMetricsRendersClassesBucketsAndErrors`, `TestHTTPMetricsZeroValueRendersEmptySeries`, `TestStatusClass`, `TestPoolStatsText`, `TestRuntimeTextRendersProcessSignals` (esta última parsea los valores y exige memoria ≠ 0, para que una muestra de runtime mal leída no pase como cero silencioso) y la ampliación de `TestMetricsKeepsReportingBrokerAndDeadLetterTruth` en `cmd/server`.
- **L5 (2026-10-16)** — Implementación de **S3 y S4** (documentación + reglas de alerta).
  - **S3:** método, herramienta y frecuencia de drift documentados en `backend/docs/operations.md` §4 (Evidently AI; Data Drift vs Concept Drift; reevaluación por lote, mínimo diaria; reentrenar solo con evidencia de ground truth). La comparación de calidad contra ground truth queda pendiente: no existe todavía. Los inputs/outputs necesarios ya se registran (L3).
  - **S4:** `backend/deploy/local/alerts.yml` con las reglas `FireWatchHighLatencyP95`, `FireWatchHighErrorRate`, `FireWatchDeadLetterBacklog`, `FireWatchBrokerUnavailable`, `FireWatchDatabasePoolSaturated` y `FireWatchTargetDown`. Cada regla nombra su acción concreta (`ALR-V2`/`ALR-V3`). Cargadas por Prometheus vía `rule_files` en `deploy/local/prometheus.yml` y montadas en `docker-compose.yml`. Sin Alertmanager: las alertas se ven en la UI de Prometheus y como la serie `ALERTS`.
  - **Evidencia observada (estructural):** los YAML nuevos y modificados (`alerts.yml`, `prometheus.yml`, `docker-compose.yml`) parsean con `yaml.safe_load_all` sin error. **No ejecutado:** no hay `promtool` ni Prometheus corriendo en este entorno, así que las reglas no se validaron semánticamente ni se vieron disparar. Se declara.
  - **Límite declarado:** las señales de latencia/errores/recursos se implementaron **solo en el servicio `risk`**. `gateway` y `jobs` conservan por ahora sus contadores de entrega; su latencia y tasa de error no están medidas, y el p95 end-to-end lo aporta el benchmark k6 del frente 6.
