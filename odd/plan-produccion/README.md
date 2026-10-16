# Plan — versión de producción (no-académica) de FireWatch

**Objetivo:** dejar la versión de producción lista de modo que, al cierre de la próxima semana, lo único pendiente sean **la fuente de datos real** y **el servicio del modelo (inferencia)**. Todo lo demás debe quedar implementado y verificado, con un *seam* limpio donde esos dos se conectan.

**Alcance:** cerrar el trabajo pendiente que vuelve operable la versión real, excluyendo explícitamente esos dos pendientes permitidos. Este documento **redirige** a la documentación real de cada frente; no la reemplaza ni la reescribe.

**Cómo leerlo:** la §1 enlaza cada frente pendiente con el documento que lo especifica y con los criterios de aceptación del checklist. El **estado** de cada entrega vive en [`odd/tasks/README.md`](../tasks/README.md) y en [`odd/urgent/README.md`](../urgent/README.md); los criterios por ID, en [`Referencias/tareas_verificacion_implementacion_IA.md`](../../Referencias/tareas_verificacion_implementacion_IA.md). Los estados del índice se leen con sus significados: «Listo» en un plan no significa código implementado, y ningún estado aprueba producción.

## 1. Redirecciones a la documentación real

Rutas relativas a la raíz del repositorio. «¿Dato?» y «¿Modelo?» marcan los frentes que dependen de los dos pendientes permitidos.

| # | Frente pendiente | Documentación real (redirección) | Qué especifica | ¿Dato? | ¿Modelo? |
|--:|---|---|---|:--:|:--:|
| 1 | Ingesta de fuentes reales y ciclo/scheduler de 15 min | [`Referencias/EP1_Equipo3_Armijo_Calderon_Saavedra2_produccion.md:42-45,88`](../../Referencias/EP1_Equipo3_Armijo_Calderon_Saavedra2_produccion.md); [`Actividad C4_...:193-199,239-247`](<../../Referencias/Actividad C4_FireWatch_produccion.md>); [`odd/tasks/backend/firewatch-data-contract.md:25-28`](../tasks/backend/firewatch-data-contract.md) | Fuentes, validación y proceso automático; el contrato ODD deja los adaptadores reales fuera de su alcance actual | Sí | No |
| 2 | Servicio real de inferencia / modelo aprobado | [`odd/tasks/backend/firewatch-data-contract.md:4,17-23`](../tasks/backend/firewatch-data-contract.md); [`Referencias/EP1_...:30,46-56`](../../Referencias/EP1_Equipo3_Armijo_Calderon_Saavedra2_produccion.md) | Define los contratos `Provider`/`Classifier` y `academic-rule-v1`; los datos sintéticos no son un modelo validado | No | Sí |
| 3 | Servicio `prevention` y ciclo de acciones preventivas | [`backend/docs/architecture.md:34`](../../backend/docs/architecture.md); [`Referencias/EP1_...:62-66`](../../Referencias/EP1_Equipo3_Armijo_Calderon_Saavedra2_produccion.md); [`odd/tasks/frontend/frontend-mockup/README.md:46-73`](../tasks/frontend/frontend-mockup/README.md) | Servicio futuro; el backlog FWP03 define asignación, estados, responsable e historial, pero sigue propuesto | No | No |
| 4 | Propagación de fallos terminales y replay de DLQ | [`backend/docs/architecture.md:42-46`](../../backend/docs/architecture.md); [`backend/README.md:94-98`](../../backend/README.md) | La DLQ conserva fallos; falta su replay y la propagación a estado terminal del job | No | No |
| 5 | Observabilidad productiva y métricas de modelo | [`Referencias/tareas_...:361-411,484-491`](../../Referencias/tareas_verificacion_implementacion_IA.md); [`backend/docs/architecture.md:55-57`](../../backend/docs/architecture.md); [`odd/urgent/04-operacion-monitoreo.md:15-17`](../urgent/04-operacion-monitoreo.md) | OpenTelemetry, Grafana, dashboards, alertas y `high_risk_predictions_total`; separar señales operativas de métricas que exigen ground truth | No | No |
| 6 | Autoscaling/HPA, carga y medición p95/SLO | [`Referencias/tareas_...:289-301,320-357`](../../Referencias/tareas_verificacion_implementacion_IA.md); [`Actividad Dimensionamiento_...:34-48,153-157`](<../../Referencias/Actividad Dimensionamiento_FireWatch_produccion.md>) | Elegir herramienta, acordar objetivos y medir; las cifras de dimensionamiento son metas iniciales, no resultados | No | No |
| 7 | Firma de imágenes, promoción de registry y rollout | [`backend/docs/architecture.md:57`](../../backend/docs/architecture.md); [`backend/docs/verification.md:62-68`](../../backend/docs/verification.md); [`Actividad C4_...:229-273`](<../../Referencias/Actividad C4_FireWatch_produccion.md>) | La CI ya produce SBOM/provenance; siguen pendientes firma, promoción y despliegue productivo verificado en Kubernetes | No | No |
| 8 | Gobierno, seguridad y protección de datos/modelo (U1) | [`odd/urgent/01-seguridad-gobierno.md:1-23`](../urgent/01-seguridad-gobierno.md); requisitos: [`Referencias/tareas_...:51-187`](../../Referencias/tareas_verificacion_implementacion_IA.md) | Asignar responsable/aprobación y registrar controles de seguridad, procedencia, privacidad, aislamiento y respuesta | Sí | Sí |
| 9 | Login real en navegador, móvil y accesibilidad | [`odd/tasks/README.md:77-88`](../tasks/README.md); [`odd/tasks/frontend/client-proposal-page.md:24-26,39-45`](../tasks/frontend/client-proposal-page.md) | La página existe; el recorrido browser/Keycloak y la revisión móvil/accesibilidad siguen sin cierre | No | No |
| 10 | OpenAPI como fuente de verdad y contract tests | [`odd/tasks/security/backend-hardening/08-openapi-errors.md:7-20`](../tasks/security/backend-hardening/08-openapi-errors.md); [`odd/urgent/README.md:44-48`](../urgent/README.md) | El OpenAPI consta validado **solo sintácticamente**; BH-08 exige contrastar respuestas reales con el contrato y probar handlers | No | No |
| 11 | Hardening backend BH-01…BH-11 | [`odd/tasks/security/backend-hardening.md`](../tasks/security/backend-hardening.md) y sus unidades en `odd/tasks/security/backend-hardening/` | 12 unidades planificadas; BH-01…BH-11 pendientes, BH-12 diferida. El plan no acredita scans ni implementación | No | No |
| 12 | Evaluación científica (sesgo, leakage, robustez, Champion/Challenger, drift) | [`odd/urgent/02-pruebas-datos.md:7-14`](../urgent/02-pruebas-datos.md); [`odd/urgent/03-pruebas-modelo.md:7-15`](../urgent/03-pruebas-modelo.md); [`Referencias/tareas_...:217-280,303-318`](../../Referencias/tareas_verificacion_implementacion_IA.md) | Bloqueada por datos, ground truth y umbrales aprobados; no puede cerrarse sin los pendientes permitidos | Sí | Sí |
| 13 | Go/No-Go y firma de aprobación de producción | [`odd/urgent/05-ct-aprobacion-demo.md:7-16`](../urgent/05-ct-aprobacion-demo.md); [`Referencias/tareas_...:457-476`](../../Referencias/tareas_verificacion_implementacion_IA.md) | Criterios de aceptación y firmas; depende del resto | No | No |
| 14 | Decisión del motor de datos productivo y continuidad/recuperación | [`backend/docs/architecture.md:59-63`](../../backend/docs/architecture.md); [`backend/docs/verification.md:62-67`](../../backend/docs/verification.md) | PostGIS local está aprobado; la decisión de motor productivo sigue explícita; fallover, backups y SLA no establecidos | No | No |
| 15 | Procedencia geoespacial (restricción DIFROL) | [`odd/tasks/backend/firewatch-geospatial-map.md:27-32`](../tasks/backend/firewatch-geospatial-map.md) | Verificación de procedencia del producto cartográfico antes de asumir la fuente lista para producción | Sí | No |

**Nota de evidencia:** la CI **no** está pendiente. `odd/tasks/security/ci-go-1.27.2-green.md:52,131` y `backend/docs/verification.md:68` registran corridas verdes en `main` (Jobs, Risk, Gateway y Backend integration), incluidos build con attestations y scan de imágenes. Este frente se retira del plan.

## 2. Los dos pendientes permitidos y sus *seams*

Estos dos son los únicos que pueden quedar sin implementar. El resto del plan debe dejarles un punto de conexión limpio, con una implementación por defecto (fixture/stub) que siga siendo determinista y verificable.

| Pendiente permitido | Seam donde se conecta | Documento |
|---|---|---|
| **Fuente de datos real** | `backend/services/risk/internal/source/` (`provider.go`, `fixture.go`); interfaz `Provider`; migración `007_synthetic_evaluation.sql` | [`odd/tasks/backend/firewatch-data-contract.md`](../tasks/backend/firewatch-data-contract.md) |
| **Servicio del modelo (inferencia)** | `backend/services/risk/internal/classify/` (`classifier.go`, `academic_rule.go`); interfaces `Classifier`/`Provider` | [`odd/tasks/backend/firewatch-data-contract.md:17-23`](../tasks/backend/firewatch-data-contract.md) |

El trabajo de la próxima semana debe dejar ambos seams estables: contrato congelado, una implementación por defecto que las suites usan hoy, y la frontera documentada para enchufar los reales sin tocar el resto.

## 3. Secuencia propuesta (independiente de datos/modelo salvo lo marcado)

Ordenada por dependencia; los grupos pueden solaparse.

1. **Base de entrega:** frente 7 (firma/registry/rollout) y frente 14 (decisión de motor de datos + continuidad) — habilitan todo lo demás.
2. **Núcleo de datos y modelo (solo el seam):** frentes 1 y 2 — congelar el contrato y la implementación por defecto; la sustitución real queda fuera.
3. **Robustez de backend:** frentes 4 (replay/estado terminal), 10 (contract tests), 11 (resto de hardening) y 8 (U1 gobierno/seguridad).
4. **Operación:** frentes 5 (observabilidad) y 6 (autoscaling/carga/p95/SLO).
5. **Producto y cierre:** frente 3 (`prevention`), frente 9 (frontend browser/móvil/a11y), frente 15 (procedencia geoespacial).

**No cierran esta semana** (dependen de los permitidos): frente 12 (evaluación científica) y 13 (Go/No-Go firmado). Se preparan como plan, no como resultado.

El trabajo está desglosado en **un archivo por spec** (frente) en esta misma carpeta; ver §7. Cada archivo es autónomo y puede tomarse en una sesión o agente distinto sin cargar el resto.

## 4. Bloqueos y decisiones humanas

- **Motor de datos productivo** (frente 14): decisión pendiente, hoy solo aprobado PostGIS local.
- **Umbrales, responsables y firmas** (frentes 8, 12, 13): no avanzan sin acuerdo explícito del equipo.
- **`prevention`**: el backlog FWP03 está **propuesto**, no aprobado como alcance — requiere revisión antes de implementarse (ver [`odd/tasks/frontend/frontend-mockup/README.md:48-54`](../tasks/frontend/frontend-mockup/README.md)).
- **Procedencia DIFROL** (frente 15): verificar antes de asumir la fuente lista.

## 5. Definición de terminado por frente

Para cada frente, «terminado» exige el criterio de cierre de sus IDs: evidencia reproducible, umbrales acordados, responsable y, cuando aplique, datos reales. Una prueba en PASS no marca una casilla del checklist (ver [`odd/urgent/README.md:27-34`](../urgent/README.md)). Registrar el estado en [`odd/tasks/README.md`](../tasks/README.md) con **qué está entregado** y **qué falta** explícitos.

## 6. Fuentes autoritativas

- **Requisitos funcionales/no funcionales de producción:** [`Referencias/EP1_Equipo3_Armijo_Calderon_Saavedra2_produccion.md:16-104`](../../Referencias/EP1_Equipo3_Armijo_Calderon_Saavedra2_produccion.md).
- **Arquitectura y operación objetivo:** [`Actividad C4_FireWatch_produccion.md:37-91,145-199,229-273`](<../../Referencias/Actividad C4_FireWatch_produccion.md>).
- **Dimensionamiento y metas iniciales:** [`Actividad Dimensionamiento_FireWatch_produccion.md:18-50,75-95,153-157`](<../../Referencias/Actividad Dimensionamiento_FireWatch_produccion.md>).
- **Criterios de aceptación por ID:** [`Referencias/tareas_verificacion_implementacion_IA.md`](../../Referencias/tareas_verificacion_implementacion_IA.md); los valores de plantilla no son metas acordadas.
- **Acciones preventivas (presentación):** [`Referencias/Presentacion_FireWatch_Acciones_Preventivas.html`](../../Referencias/Presentacion_FireWatch_Acciones_Preventivas.html).

## 7. Desglose por spec (una sesión por archivo)

Cada frente tiene su propio documento con el contrato ODD (**Specs → Tasks → Log**), dependencias, bloqueos humanos y criterios de cierre por redirección. Son **documentos de planificación**: no acreditan implementación ni evidencia. Toma un archivo por sesión/agente; los archivos no se pisan entre sí.

| Grupo | Frente | Archivo | Depende de | Bloqueo |
| --- | --- | --- | --- | --- |
| G1 Base de entrega | 7 — Firma, registry y rollout | [`07-firma-registry-rollout.md`](07-firma-registry-rollout.md) | registry/llaves/clúster | Decisión de registry y claves |
| G1 Base de entrega | 14 — Motor de datos y continuidad | [`14-motor-datos-continuidad.md`](14-motor-datos-continuidad.md) | decisión de motor/infra | **Motor productivo pendiente** |
| G2 Núcleo datos/modelo (solo *seam*) | 1 — Ingesta y scheduler 15 min | [`01-ingesta-scheduler.md`](01-ingesta-scheduler.md) | acceso a fuentes | Fuentes reales (permitido) |
| G2 Núcleo datos/modelo (solo *seam*) | 2 — Servicio de inferencia | [`02-modelo-inferencia.md`](02-modelo-inferencia.md) | frente 1 | Modelo real (permitido) |
| G3 Robustez backend | 4 — Fallos terminales y replay DLQ | [`04-dlq-replay-terminal.md`](04-dlq-replay-terminal.md) | BH-01 | Política de recuperación |
| G3 Robustez backend | 10 — OpenAPI y contract tests | [`10-openapi-contract-tests.md`](10-openapi-contract-tests.md) | BH-01 | Aplicabilidad por operación |
| G3 Robustez backend | 11 — Hardening BH-01…BH-11 | [`11-hardening-bh.md`](11-hardening-bh.md) | BH-01 | Autorizar alcance BH |
| G3 Robustez backend | 8 — Gobierno y seguridad (U1) | [`08-gobierno-seguridad-u1.md`](08-gobierno-seguridad-u1.md) | responsables/decisiones | Designar responsable y aprobar |
| G4 Operación | 5 — Observabilidad y métricas de modelo | [`05-observabilidad.md`](05-observabilidad.md) | *ground truth* | Umbrales/herramientas — **resuelto, implementado (ver §8)** |
| G4 Operación | 6 — Autoscaling, carga y p95/SLO | [`06-autoscaling-carga-slo.md`](06-autoscaling-carga-slo.md) | SLO/carga/RTO | Acordar objetivos — **resuelto, artefactos escritos (ver §8)** |
| G5 Producto y cierre | 3 — Servicio `prevention` | [`03-prevention.md`](03-prevention.md) | FWP01–02 | **Alcance FWP03 propuesto** |
| G5 Producto y cierre | 9 — Login browser/móvil/a11y | [`09-login-browser-a11y.md`](09-login-browser-a11y.md) | realm/cliente/clúster | Ajustes en Admin Console |
| G5 Producto y cierre | 15 — Procedencia geoespacial (DIFROL) | [`15-procedencia-geoespacial.md`](15-procedencia-geoespacial.md) | metadatos geo | Validación DIFROL |
| No cierra esta semana | 12 — Evaluación científica | [`12-evaluacion-cientifica.md`](12-evaluacion-cientifica.md) | datos + modelo | **Bloqueado por permitidos** |
| No cierra esta semana | 13 — Go/No-Go y firma | [`13-go-no-go.md`](13-go-no-go.md) | datos + modelo + infra | **Bloqueado por permitidos** |

**Reparto sugerido de sesiones (paralelizable):**

- **Sesión A (paralela):** frentes 4, 10, 11 (mismo frente de backend/hardening; superficies disjuntas, coordinar BH-07/BH-08 con 4/10).
- **Sesión B (paralela):** frente 5 y 6 (operación; comparten el acuerdo de umbrales/SLO — resolver ese acuerdo primero). **Ejecutada el 2026-10-16:** acuerdo resuelto, frente 5 implementado y verificado, frente 6 con artefactos escritos y ejecución pendiente. Ver §8.
- **Sesión C (decisión):** frente 14 (decisión de motor) y frente 7 (registry/claves): son decisiones humanas que habilitan al resto.
- **Sesión D:** frente 9 (frontend/browser) y frente 15 (DIFROL): acotadas y documentales.
- **Sesión E (gobierno):** frente 8 (U1) — requiere responsables.
- **Detenidas hasta aprobación/datos:** frentes 1, 2, 3, 12, 13.

## 8. Registro de avance — Sesión B (2026-10-16)

Sesión ejecutada sobre los frentes **5 (observabilidad)** y **6 (autoscaling/carga/SLO)**. El detalle autoritativo vive en el `## Log` de [`05-observabilidad.md`](05-observabilidad.md) (L2–L5) y [`06-autoscaling-carga-slo.md`](06-autoscaling-carga-slo.md) (L2–L3); esta sección es el resumen y el estado. **Ningún frente queda «terminado»** según §5: hay implementación y evidencia de código, pero **falta la medición**.

### 8.1 Bloqueo humano resuelto

El acuerdo que los dos frentes compartían quedó aprobado y registrado:

- **Objetivos:** p95 < 500 ms bajo carga; máximo 2 s normal; 4 RPS pico (20 usuarios concurrentes, 1 consulta/5 s); ciclo de 15 min; API/Web 2 réplicas × 8 GB, réplica de predicción 4 GB, worker 16 GB / 4 vCPU.
- **Fijados por el usuario (sin fuente documental):** disponibilidad 99.5 %; RTO ≤ 15 min; alerta p95 > 500 ms sostenido 5 min; alerta 5xx > 1 % sostenido 5 min.
- **Herramientas:** carga con **k6**; observabilidad con **Prometheus + Grafana**; drift con **Evidently AI** (Data vs Concept). Reentrenar solo con evidencia de ground truth; reevaluación de drift por lote, mínimo diaria.
- Todas las cifras son **objetivo**, ninguna medición.

### 8.2 Frente 5 — implementado y verificado

| Spec | Qué se hizo | Dónde |
| --- | --- | --- |
| **S5** | `firewatch_high_risk_predictions_total` y la distribución de riesgo por nivel (`firewatch_risk_predictions_total` con una serie para `bajo`, `medio`, `alto` y `desconocido`). Se pueblan desde `Publisher.Publish` con el seam nuevo `RiskObserver`, **solo tras persistir** (una publicación fallida se reintenta entera). Un nivel no canónico se cuenta como `desconocido`, nunca se descarta. | `risk/internal/metrics/metrics.go`, `risk/internal/service/publisher.go`, `risk/cmd/server/main.go` |
| **S1** | Línea estructurada `risk_assessed` con job, zona, proveedor, `synthetic`, `model_version`, `params_hash`, `risk_level` y las cinco entradas. Una entrada ausente se registra como `"unknown"`, nunca como `0`. **`LOG-I3` (ground truth) queda pendiente**: el dato real está diferido. | `risk/internal/service/publisher.go` |
| **S2** | `/metrics` gana latencia (histograma acumulativo → p95 vía `histogram_quantile`), errores por clase + `firewatch_http_errors_total`, recursos de proceso (goroutines, CPU s, memoria) y pool PostgreSQL/PostGIS. Instrumentado con un `statusRecorder` en el middleware: **ningún handler cambió**. RPS ya cubierto por `firewatch_http_requests_total`. | `risk/internal/metrics/{http,process}.go`, `risk/cmd/server/main.go` |
| **S3** | Método, herramienta y frecuencia de drift documentados (Evidently AI; Data vs Concept; por lote, mínimo diaria; reentrenar solo con evidencia). La comparación de calidad contra ground truth queda pendiente. | `backend/docs/operations.md` §4 |
| **S4** | `backend/deploy/local/alerts.yml` con 6 reglas donde **cada alerta nombra su acción concreta** (`ALR-V3`); cargadas por `rule_files` y montadas en compose. Sin Alertmanager: se ven en la UI de Prometheus y como la serie `ALERTS`. | `backend/deploy/local/{alerts,prometheus}.yml`, `backend/docker-compose.yml` |

Límite declarado: latencia/errores/recursos se implementaron **solo en el servicio `risk`**. `gateway` y `jobs` conservan por ahora sus contadores de entrega; su latencia y tasa de error no están medidas (cada servicio es un módulo Go propio, así que extenderlo implica duplicar `internal/metrics`).

### 8.3 Frente 6 — artefactos escritos, **sin ejecutar**

| Spec | Qué se hizo | Estado |
| --- | --- | --- |
| **S1** | Acuerdo de SLO/SLA marcado objetivo vs medición. | Hecho (§8.1) |
| **S2** | `backend/deploy/load/k6-risk.js`: escenario acordado (4 RPS constantes, 20 VUs) contra `GET /v1/risk/latest`, con umbrales pasa/falla `p(95)<500`, `max<2000`, `http_req_failed<0.01`. Token por entorno. | Escrito, **no ejecutado** |
| **S3** | `backend/deploy/k8s/base/autoscaling.yaml`: un HPA por servicio (2→6 réplicas, CPU al 70 %, scale-up 60 s / scale-down 300 s), añadido a `kustomization.yaml`. | Escrito, **no verificado en clúster** |
| **S4** | Objetivo RTO ≤ 15 min y la base que lo sostiene (2 réplicas, `PodDisruptionBudget` `minAvailable: 1`, rollout). | Documentado, **no medido** |
| **S5** | Plan de validación de la capacidad inicial. | Documentado, **no ejecutado** |

### 8.4 Evidencia observada

- `gofmt -l .` sin salida; `go build ./...` y `go vet ./...` correctos; `go test ./... -count=1` **verde en los 10 paquetes** del módulo `risk`.
- Pruebas nuevas que **corrieron** (no saltadas): S5 (`TestRiskOutcomesCountsEachLevel`, `TestRiskOutcomesCountsAnUnknownLevelAsDesconocido`, `TestPublisherReportsTheOutcomeOnlyAfterPersistence`, `TestPublisherDoesNotReportAnOutcomeWhenPersistenceFails`) y S2 (`TestHTTPMetricsRendersClassesBucketsAndErrors`, `TestHTTPMetricsZeroValueRendersEmptySeries`, `TestStatusClass`, `TestPoolStatsText`, `TestRuntimeTextRendersProcessSignals`), más la ampliación de `TestMetricsKeepsReportingBrokerAndDeadLetterTruth`.
- Validación **estructural** de los YAML nuevos/modificados con `yaml.safe_load_all`; el script k6 pasa `node --check` (Node v24.11.1). **No** hay `promtool` ni Prometheus corriendo aquí, así que las reglas de alerta no se validaron semánticamente ni se vieron disparar.

### 8.5 Pendientes y límites declarados

- **Código pendiente (NO requiere entorno):** instrumentar `gateway` y `jobs` con latencia/errores/recursos — hoy solo `risk` las expone (frente 5, S2). Es el único ítem de código abierto de la sesión.
- **`LOG-I3` (ground truth) pendiente:** bloqueado por el pendiente permitido (fuente de datos real), no por entorno.
- **No se midió nada:** benchmark, escalado y RTO requieren entorno aislado y clúster de destino. Los valores siguen siendo objetivos.
- Sin Alertmanager no hay enrutamiento de alertas.
- Sin ground truth no hay evaluación de drift de calidad.
- La acción de la alerta de DLQ es manual hasta que el frente 4 (replay/estado terminal) exista.
- **Sin commit:** el repositorio sigue en `nogit`; además el árbol de trabajo contiene cambios de la **Sesión A** en paralelo (contract tests de `kin-openapi` y su `go.mod`). Se verificó que estos últimos **no** provienen de esta sesión y no se tocaron.

**Para retomar:** el detalle accionable por spec está en las tablas "Pendientes al cierre de la Sesión B" de [`05-observabilidad.md`](05-observabilidad.md) y [`06-autoscaling-carga-slo.md`](06-autoscaling-carga-slo.md).

