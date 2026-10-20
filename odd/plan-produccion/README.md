# Plan — versión de producción (no-académica) de FireWatch

**Objetivo:** dejar la versión de producción lista de modo que, al cierre de la próxima semana, lo único pendiente sean **la fuente de datos real** y **el servicio del modelo (inferencia)**. Todo lo demás debe quedar implementado y verificado, con un *seam* limpio donde esos dos se conectan.

**Alcance:** cerrar el trabajo pendiente que vuelve operable la versión real, excluyendo explícitamente esos dos pendientes permitidos. Este documento **redirige** a la documentación real de cada frente; no la reemplaza ni la reescribe.

**Cómo leerlo:** la §1 enlaza cada frente pendiente con el documento que lo especifica y con los criterios de aceptación del checklist. El **estado** de cada entrega vive en [`odd/tasks/README.md`](../tasks/README.md) y en [`odd/urgent/README.md`](../urgent/README.md); los criterios por ID, en [`Referencias/tareas_verificacion_implementacion_IA.md`](../../Referencias/tareas_verificacion_implementacion_IA.md). Los estados del índice se leen con sus significados: «Listo» en un plan no significa código implementado, y ningún estado aprueba producción.

## 1. Redirecciones a la documentación real

Rutas relativas a la raíz del repositorio. «¿Dato?» y «¿Modelo?» marcan los frentes que dependen de los dos pendientes permitidos.

**Base de planificación histórica (2026-10-16).** Las descripciones de «qué especifica» resumen cada documento fuente en su fecha; no son el estado actual. El estado vigente está en §10 y §11: la entrega de presentación quedó en `f509666` (PR #31) y el baseline actual es `ef686c0` (PRs #33–#41).

| # | Frente pendiente | Documentación real (redirección) | Qué especifica | ¿Dato? | ¿Modelo? |
|--:|---|---|---|:--:|:--:|
| 1 | Ingesta de fuentes reales y ciclo/scheduler de 15 min | [`Referencias/EP1_Equipo3_Armijo_Calderon_Saavedra2_produccion.md:42-45,88`](../../Referencias/EP1_Equipo3_Armijo_Calderon_Saavedra2_produccion.md); [`Actividad C4_...:193-199,239-247`](<../../Referencias/Actividad C4_FireWatch_produccion.md>); [`odd/tasks/backend/firewatch-data-contract.md:25-28`](../tasks/backend/firewatch-data-contract.md) | Fuentes, validación y proceso automático; el contrato ODD deja los adaptadores reales fuera de su alcance actual | Sí | No |
| 2 | Servicio real de inferencia / modelo aprobado | [`odd/tasks/backend/firewatch-data-contract.md:4,17-23`](../tasks/backend/firewatch-data-contract.md); [`Referencias/EP1_...:30,46-56`](../../Referencias/EP1_Equipo3_Armijo_Calderon_Saavedra2_produccion.md) | Define los contratos `Provider`/`Classifier` y `academic-rule-v1`; los datos sintéticos no son un modelo validado | No | Sí |
| 3 | Servicio `prevention` y ciclo de acciones preventivas | [`backend/docs/architecture.md:34`](../../backend/docs/architecture.md); [`Referencias/EP1_...:62-66`](../../Referencias/EP1_Equipo3_Armijo_Calderon_Saavedra2_produccion.md); [`odd/tasks/frontend/frontend-mockup/README.md:46-73`](../tasks/frontend/frontend-mockup/README.md) | Servicio entregado en la presentación (L13/L14); el backlog FWP03 fue su origen y ya no está propuesto | No | No |
| 4 | Propagación de fallos terminales y replay de DLQ | [`backend/docs/architecture.md:42-46`](../../backend/docs/architecture.md); [`backend/README.md:121`](../../backend/README.md) | La DLQ conserva fallos; el estado terminal `failed` y el replay de operador están implementados (frente 4, BH-07); falta el replay automático de los mensajes que ningún job puede reclamar | No | No |
| 5 | Observabilidad productiva y métricas de modelo | [`Referencias/tareas_...:361-411,484-491`](../../Referencias/tareas_verificacion_implementacion_IA.md); [`backend/docs/architecture.md:55-57`](../../backend/docs/architecture.md); [`odd/urgent/04-operacion-monitoreo.md:15-17`](../urgent/04-operacion-monitoreo.md) | OpenTelemetry y ground truth siguen pendientes; Grafana, dashboards y alertas ya existen (L26/L27); separar señales operativas de métricas que exigen ground truth | No | No |
| 6 | Autoscaling/HPA, carga y medición p95/SLO | [`Referencias/tareas_...:289-301,320-357`](../../Referencias/tareas_verificacion_implementacion_IA.md); [`Actividad Dimensionamiento_...:34-48,153-157`](<../../Referencias/Actividad Dimensionamiento_FireWatch_produccion.md>) | Elegir herramienta, acordar objetivos y medir; las cifras de dimensionamiento son metas iniciales, no resultados | No | No |
| 7 | Firma de imágenes, promoción de registry y rollout | [`backend/docs/architecture.md:57`](../../backend/docs/architecture.md); [`backend/docs/verification.md:62-68`](../../backend/docs/verification.md); [`Actividad C4_...:229-273`](<../../Referencias/Actividad C4_FireWatch_produccion.md>) | La CI ya produce SBOM/provenance; siguen pendientes firma, promoción y despliegue productivo verificado en Kubernetes | No | No |
| 8 | Gobierno, seguridad y protección de datos/modelo (U1) | [`odd/urgent/01-seguridad-gobierno.md:1-23`](../urgent/01-seguridad-gobierno.md); requisitos: [`Referencias/tareas_...:51-187`](../../Referencias/tareas_verificacion_implementacion_IA.md) | Asignar responsable/aprobación y registrar controles de seguridad, procedencia, privacidad, aislamiento y respuesta | Sí | Sí |
| 9 | Login real en navegador, móvil y accesibilidad | [`odd/tasks/README.md:95`](../tasks/README.md); [`odd/tasks/frontend/client-proposal-page.md:24-26,39-45`](../tasks/frontend/client-proposal-page.md) | La página existe; el recorrido browser/Keycloak y la revisión móvil/accesibilidad se midieron después (L14/L23); los límites de prueba están en §10 | No | No |
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

**Resueltas el 2026-10-16** (sesión A2 y sesión B; el detalle autoritativo vive en el `## Log` de cada frente y en el documento de su unidad):

- **Política de recuperación** (frente 4 / BH-07): estado terminal `failed` solo con evidencia real, reintentos sin cambios y replay de operador; el hueco de un job `queued` se resolvió **extendiendo el replay** con confirmación explícita, no aceptándolo.
- **Cuotas del gateway** (BH-03) y **política de tokens** (BH-09, ratificada por escrito).
- **Umbrales y herramientas de operación** (frentes 5 y 6): objetivos fijados y registrados — **ninguno medido todavía**.
- **Rama y entrega**: `fix/backend-hardening-bh`, publicada como PR #29.

Dos decisiones de la sesión A2 no aparecen arriba porque no son políticas de producto; su handoff de origen se retiró el 2026-10-17 y quedan registradas aquí:

- **D1 — Alcance de A2 en dos niveles.** Nivel 1, cerrable sin entorno: BH-03, BH-07 (política, implementación y tests unitarios), BH-08 y BH-09. Nivel 2, diferido con razón registrada y **sin marcar hecho**: BH-02 (scan de imágenes), verificación end-to-end de BH-07, BH-10, BH-04/05/06/11 y BH-12.
- **D7 — Archivos.** `.agents/` fuera de control de versiones; la documentación commiteada completa; los artefactos de los frentes 5/6 (sesión B) junto a `backend/docs/operations.md` en el mismo commit.

**Siguen abiertas:**

- **Motor de datos productivo** (frente 14): decisión pendiente; hoy solo está aprobado PostGIS local.
- **Responsables y firmas** (frentes 8, 12, 13): no avanzan sin acuerdo explícito del equipo.
- **`prevention`**: implementado en la entrega de presentación (servicio, migración `009`, contrato congelado y vista `#actions`, L13/L14). La separación de la base de `risk` sigue como decisión de producción.
- **Procedencia DIFROL** (frente 15): verificar antes de asumir la fuente lista.
- **Entorno y accesos** para BH-02 (imágenes), BH-04/05/06/11, BH-10 y el escenario end-to-end de BH-07: no esperan una decisión de producto, sino infraestructura disponible (Docker/`trivy`, PostgreSQL y broker aislados, clúster).
- **PR #29 y duplicado residual**: resueltos. El PR quedó contenido en `main` y se cerró con un comentario (L24); `odd/tasks/backend-hardening/` ya no existe (L2) y el índice lo registra como eliminado.
- **Modelo entrenado real y fuente de datos real**: siguen siendo los dos pendientes permitidos; el resto del alcance de presentación está implementado y verificado (L22/L23).

## 5. Definición de terminado por frente

Para cada frente, «terminado» exige el criterio de cierre de sus IDs: evidencia reproducible, umbrales acordados, responsable y, cuando aplique, datos reales. Una prueba en PASS no marca una casilla del checklist (ver [`odd/urgent/README.md:27-34`](../urgent/README.md)). Registrar el estado en [`odd/tasks/README.md`](../tasks/README.md) con **qué está entregado** y **qué falta** explícitos.

### 5.1 Terminado en modo sintético

Un frente puede darse por terminado sin datos reales ni modelo entrenado cuando cumple:

- Contrato congelado: `ObservationSet`, `Provenance`, `Provider` y `Classifier`.
- Fixture determinista con semilla: la misma entrada produce la misma salida.
- **Prueba de sustitución:** cambiar el proveedor o el clasificador por configuración y demostrar que ningún archivo fuera del adaptador cambia.
- Front con estados vacío, carga, error y dato vencido, y procedencia visible.
- Lotes definidos y probados: tamaño, cadencia, idempotencia, deduplicación y retención.
- HPA, alertas y dashboards escritos y etiquetados como objetivo, no como medición.
- Suites, `go vet` y contract tests en verde.

Eso habilita «listo para conectar». No habilita marcar IDs del checklist ni declarar mediciones.

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
| G5 Producto y cierre | 3 — Servicio `prevention` | [`03-prevention.md`](03-prevention.md) | FWP01–02 | **Implementado (L13/L14); separar la base es decisión de producción** |
| G5 Producto y cierre | 9 — Login browser/móvil/a11y | [`09-login-browser-a11y.md`](09-login-browser-a11y.md) | realm/cliente/clúster | **Recorrido real medido (L14/L23); límites de prueba: sin certificación WCAG ni lector de pantalla** |
| G5 Producto y cierre | 15 — Procedencia geoespacial (DIFROL) | [`15-procedencia-geoespacial.md`](15-procedencia-geoespacial.md) | metadatos geo | Validación DIFROL |
| No cierra esta semana | 12 — Evaluación científica | [`12-evaluacion-cientifica.md`](12-evaluacion-cientifica.md) | datos + modelo | **Bloqueado por permitidos** |
| No cierra esta semana | 13 — Go/No-Go y firma | [`13-go-no-go.md`](13-go-no-go.md) | datos + modelo + infra | **Bloqueado por permitidos** |

**Reparto sugerido de sesiones (paralelizable):**

- **Sesión A (paralela):** frentes 4, 10, 11 (mismo frente de backend/hardening; superficies disjuntas, coordinar BH-07/BH-08 con 4/10). **Ejecutada y cerrada el 2026-10-16:** política acordada, BH-01/02/03/07/08/09 al día y el último hueco de recuperación resuelto; ver §9.
- **Sesión B (paralela):** frente 5 y 6 (operación; comparten el acuerdo de umbrales/SLO — resolver ese acuerdo primero). **Ejecutada el 2026-10-16:** acuerdo resuelto, frente 5 implementado y verificado, frente 6 con artefactos escritos y ejecución pendiente. Ver §8.
- **Sesión C (decisión):** frente 14 (decisión de motor) y frente 7 (registry/claves): son decisiones humanas que habilitan al resto.
- **Sesión D:** frente 9 (frontend/browser) y frente 15 (DIFROL): el frente 9 se ejecutó (L14/L23); DIFROL sigue pendiente.
- **Sesión E (gobierno):** frente 8 (U1) — requiere responsables.
- **Detenidas hasta aprobación/datos:** frentes 12 y 13 (modelo entrenado y evaluación); el frente 3 quedó entregado y los frentes 1–2 dejaron su seam y su implementación por defecto, con la fuente y el modelo reales como pendientes permitidos.

## 8. Registro de avance — Sesión B (2026-10-16)

**Sección histórica.** Describe el estado del 2026-10-16 y no se reescribe. Los ítems de código que deja abiertos —latencia/errores en `gateway` y `jobs`, y Grafana— se cerraron después (T10–T11, T25–T27); el estado vigente está en §10–§11.

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

**Límite de entonces (histórico; cerrado después por T10–T11 y T25–T27):** latencia/errores/recursos se implementaron **solo en el servicio `risk`**. `gateway` y `jobs` conservan por ahora sus contadores de entrega; su latencia y tasa de error no están medidas (cada servicio es un módulo Go propio, así que extenderlo implica duplicar `internal/metrics`).

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

## 9. Registro de avance — Sesión A (cerrada el 2026-10-16)

**Sección histórica.** Describe el estado del 2026-10-16 y no se reescribe. El PR #29 quedó contenido en `main` y cerrado con un comentario (L24), el duplicado residual ya no existe y el estado vigente está en §10–§11.

Sesión ejecutada sobre los frentes **4 (fallos terminales y replay), 10 (OpenAPI y contract tests) y 11 (hardening BH-01…BH-11)**, en la rama `fix/backend-hardening-bh`. El detalle autoritativo vive en el `## Log` de [`04-dlq-replay-terminal.md`](04-dlq-replay-terminal.md) (L3–L4), [`10-openapi-contract-tests.md`](10-openapi-contract-tests.md), [`11-hardening-bh.md`](11-hardening-bh.md) (L3) y en el documento de cada unidad (`../tasks/security/backend-hardening/NN-*.md`).

### 9.1 Estado por unidad

| Unidad | Estado | Evidencia / límite |
| --- | --- | --- |
| BH-01 — Toolchain y baseline | **Cerrada** | `02911d5` |
| BH-02 — Vulnerabilidades e imágenes | **Parcial** | `702aff6`: `govulncheck` limpio en los 4 módulos; el inventario y el scan de imágenes exigen daemon de Docker y `trivy`, y este host no los tiene |
| BH-03 — Cuotas del gateway | **Implementada** | `2981dc5`; `-race` solo corre en CI |
| BH-07 / frente 4 — Recuperación y replay | **Implementada**, incluido el replay de un job `queued` | `54c1ce8`, `0b0f784`, `537f83a`, `90f48a6`; la verificación end-to-end sigue necesitando PostgreSQL y broker |
| BH-08 / frente 10 — OpenAPI y contract tests | **Implementada y re-verificada en línea** | `750cd80`, `1436dae`, `cf23d20`; sin verificador independiente: la admisión lo rechazó |
| BH-09 — Autenticación negativa | **Implementada y ratificada por escrito** | `03baa09`; ratificación D5 el 2026-10-16 |
| BH-04/05/06/10/11 | **Diferidas** | Decisiones de entorno y accesos; BH-10 exige DB y broker aislados |
| BH-12 — Errores centralizados | **Diferida** | Requiere activación explícita |

### 9.2 Decisión humana que cerró el último hueco

Un job `queued` cuya entrega no se puede rastrear no tenía camino de reparación: el replay solo aceptaba `failed`, porque un job encolado puede seguir en vuelo. **La decisión fue extender el replay, no aceptar el hueco**, con un *guard* explícito: el operador confirma con `{"confirm":"queued_may_be_in_flight"}` (sin ella, `409 replay_confirmation_required` y nada escrito), y en `risk` la rama de conflicto de lote re-emite el completion del lote ya almacenado en vez de omitirlo, para que el replay también repare el caso «el lote se guardó pero el resultado se perdió». `attempts` vuelve a 0 y `requested_at` no se reescribe: un job viejo re-despachado produce un lote ya vencido, a propósito.

### 9.3 Evidencia observada y límites

- Verdes: `gateway` (2 paquetes), `risk` (10), `jobs` (5) y `contracts/gen`; `gofmt -l` sin salida; `go vet` limpio en los cuatro módulos; `kin-openapi v0.133.0 validate` en 0; pruebas de contrato y de replay en PASS.
- RED observados: el del contrato previo (reproducido en un export aislado) y el de las reglas nuevas del replay en el límite del transporte.
- **No ejecutado aquí:** las pruebas de integración con PostgreSQL (saltan sin `TEST_DATABASE_URL`; CI las corre con los perfiles `test`) y `-race` (sin compilador de C). La regla de S6 no tiene RED ejecutable en este host: su única superficie es la rama SQL, y por eso queda a cargo de CI.

### 9.4 Qué no cierra esta sesión

Nada por decisión pendiente: lo que queda abierto lo está por **entorno** (Docker, PostgreSQL, broker, clúster) o por **otra sesión** (frentes 1, 2, 3, 5/6, 7, 8, 9, 12, 13, 14, 15). El frente 6 conserva su ejecución pendiente y el frente 5 su único ítem de código abierto, ambos de la sesión B. El inventario completo, con qué desbloquea cada cosa, está en §10.

### 9.5 Historial de la rama y rebanadas de revisión del PR #29

Registro trasladado desde el handoff de la sesión A2, retirado el 2026-10-17. La rama `fix/backend-hardening-bh` acumula el frente completo (≈6 000 líneas autoradas), así que el PR #29 conviene revisarlo **por rebanada, una unidad por commit**, no como un diff único.

| Commit | Unidad | Contenido |
| --- | --- | --- |
| `02911d5` | BH-01 | Baseline de toolchain (cherry-pick de `ee215b4`) |
| `750cd80` | BH-08 | Contract tests contra rutas reales (cherry-pick de `b91714b`) |
| `702aff6` | BH-02 | Evidencia de scans de dependencias (cherry-pick de `aed5e09`) |
| `03baa09` | BH-09 | Tests negativos de la política de tokens (cherry-pick de `678a13a`) |
| `2ab9cc4` | — | `.agents/` fuera de control de versiones |
| `9326daa` | frentes 5/6 | Métricas de resultado y artefactos operativos de la sesión B, con `operations.md` |
| `784f589` | — | El plan de producción, un archivo por frente |
| `e025643` | — | Arranque del frontend, semántica de health y citas obsoletas |
| `29811ef` | A2 | Decisiones D1–D7 y las políticas acordadas |
| `2981dc5` | **BH-03** | Tres capas de cuota del gateway |
| `54c1ce8` | **BH-07 u1** | Estado terminal `failed` en la fila del job |
| `1436dae` | BH-08 | Corrección de dos descripciones falsas del contrato, más las sondas del verificador |
| `0b0f784` | **BH-07 u2** | Reporte de fallo `risk`→`jobs` y señal de job atascado |
| `537f83a` | **BH-07 u3** | Replay de operador |
| `a00b483` | BH-07 | Corrección del bloqueador del runbook, más las sondas del verificador |
| `90f48a6` | **BH-07 u4** | Replay de un job `queued` con confirmación explícita, y compuerta de conflicto de lote en `risk` |
| `cf23d20` | BH-08 | Corrección de la descripción de `ErrorResponse.code` |
| `c2f5044` | — | Cierre documental |
| `8ac654e` | CI | `risk`: copia de `contracts/openapi/` en la imagen de pruebas |
| `761c16c` | CI | Citas de guards del backend en el test de frontend |

Los SHA de la sesión A cambiaron al reaplicarse sobre `main`: `ee215b4`→`02911d5`, `b91714b`→`750cd80`, `aed5e09`→`702aff6`, `678a13a`→`03baa09`. El detalle por unidad vive en su documento en [`../tasks/security/backend-hardening/`](../tasks/security/backend-hardening/).

## 10. Pendientes consolidados (2026-10-16; actualizado el 2026-10-18)

Todo lo que sigue abierto de este plan, en un solo lugar, con **qué lo desbloquea**: una decisión humana, un entorno, otra sesión o otro frente. El estado por unidad de hardening vive en [`../tasks/security/backend-hardening/`](../tasks/security/backend-hardening/) y el índice del §5 se mantiene en [`../tasks/README.md`](../tasks/README.md). **Actualización del 2026-10-18:** las filas de los frentes 1, 2, 3, 5 y 9 incorporan la evidencia de la entrega de presentación y de la ola de observabilidad; el detalle autoritativo vive en el `## Log` de [`../tasks/presentation-client-ready.md`](../tasks/presentation-client-ready.md) L22–L28.

| Frente | Estado | Qué falta | Qué lo desbloquea |
| --- | --- | --- | --- |
| 1 — Ingesta y scheduler | **Parcial — seam y ciclo entregados** | La fuente de datos real y sus rangos de validación de producción (pendiente permitido) | Acceso y periodicidad reales de las fuentes (decisión humana) |
| 2 — Servicio de inferencia | **Parcial — registro y segunda versión entregados** | El modelo entrenado real y su evaluación científica (frentes 12/13) | Modelo real y su política de aprobación |
| 3 — `prevention` | **Listo — servicio entregado y verificado** | Nada del alcance de demo; separar la base de `risk` es una decisión de producción | Decisión de producción sobre la base |
| 4 — Fallos terminales y replay | **Implementado y verificado** | El escenario end-to-end (interrumpir PostgreSQL, agotar reintentos, restaurar, reiniciar consumidores) y `-race` local | Entorno con PostgreSQL y broker |
| 5 — Observabilidad | Parcial — monitoreo local completo | Medir los umbrales con carga; `LOG-I3` (*ground truth*); Alertmanager; monitoreo en Kubernetes | Entorno y *ground truth* para medir |
| 6 — Autoscaling, carga y p95/SLO | Parcial | Ejecutar k6, verificar el HPA en clúster y medir el RTO | Entorno aislado y clúster de destino |
| 7 — Firma, registry y rollout | Pendiente | Firma, promoción y despliegue verificado (la CI ya produce SBOM/provenance) | Decisión de registry y llaves, y clúster (accesos) |
| 8 — Gobierno y seguridad (U1) | Pendiente | Controles y clasificación sin aprobar | Designar responsable y aprobar |
| 9 — Login browser/móvil/a11y | **Parcial — recorrido real medido** | Límites de prueba: sin certificación WCAG ni lector de pantalla; la aserción de foco a 390 px cierra sobre BODY | No es un bloqueo de producto; es el límite de lo medido |
| 10 — OpenAPI y contract tests | **Implementado y re-verificado** | Nada propio; la re-verificación fue en línea porque la admisión rechazó al verificador independiente | — |
| 11 — Hardening BH-01…BH-11 | Sesión A cerrada (detalle en §9) | BH-02 imágenes (inventario + `trivy`); BH-04/05/06 (entorno); BH-10 (DB y broker aislados); BH-11 (depende de BH-02/04/05/06); BH-12 (activación explícita) | Docker y `trivy`, entorno y accesos, y una decisión de activación para BH-12 |
| 12 — Evaluación científica | **Bloqueado** | Nada avanza sin datos reales | Datos representativos, modelo evaluable, *ground truth* y umbrales acordados |
| 13 — Go/No-Go | **Bloqueado** | Ídem | Evidencia de datos y modelo, y criterios firmados |
| 14 — Motor de datos y continuidad | Pendiente | Decisión de motor productivo (hoy solo PostGIS local aprobado) | Decisión humana; el despliegue redundante depende del frente 7 |
| 15 — Procedencia geoespacial (DIFROL) | Pendiente | Validación de la cartografía | Revisión DIFROL |

### 10.1 Entrega y revisión

- **PR #29** (`fix/backend-hardening-bh` → `main`, 83 archivos, +6 033/−188): quedó contenido en `main` y se cerró con un comentario (L24); no queda merge pendiente.
- Conviene revisarlo **por rebanada** (una unidad por commit); el reparto está en §9.5 de este documento.
- **Revisión nativa:** el candidato `b0f77f9..HEAD` se evaluó como `high` con `review_due`; la oferta se hizo una vez y el consentimiento quedó **declinado para este candidato** (`lineage_created: false`, sin autoridad consumida). Un cambio nuevo vuelve a tener su propia oferta.
- **Duplicado residual:** eliminado el 2026-10-17; la copia verificada vive en `odd/tasks/security/backend-hardening/`.

### 10.2 Registro

El §5 exige declarar **qué está entregado** y **qué falta** en [`../tasks/README.md`](../tasks/README.md). Al cerrar la sesión A se actualizó allí la fila del plan de hardening, las doce unidades y se agregó la tabla de los frentes de este plan, para que el índice deje de decir que BH-01…BH-11 están pendientes.

## 11. Cierre para la presentación con el cliente (2026-10-17)

La entrega de esta semana tiene su propio plan, con specs, tareas y reparto por worktrees: [`../tasks/presentation-client-ready.md`](../tasks/presentation-client-ready.md). Cubre los frentes **1, 2, 3 y 9** más el ítem de código abierto del frente 5 (latencia y errores en `gateway` y `jobs`), y deja declarado fuera lo que exige modelo entrenado, clúster o decisiones de gobierno.

| Ola | Rama | Tareas | Módulo |
| --- | --- | --- | --- |
| 1 | `feat/ingesta-ciclo` | T1–T6 | `risk` (ingesta, ciclo de 15 min, segunda versión de modelo) |
| 1 | `feat/prevention` | T7–T9 | `prevention` (servicio nuevo) |
| 1 | `feat/metrics-operacion` | T10–T11 | `gateway`, `jobs` |
| 2 | `feat/frontend-completo` | T12–T15 | `frontend` |
| cierre | `feat/presentacion-cliente` | T16–T19 | integración y verificación end-to-end |

**Estado al 2026-10-18:** la entrega quedó en `main` con `f509666` (PR #31, issue #30) en su momento; la ola de monitoreo siguió en los PRs #33–#35 y el baseline actual es `ef686c0`. S2 y S1 se cerraron con la corrida en frío (L22) y el monitoreo S7 con T25–T27 (L27). Sigue fuera de alcance lo que exige modelo entrenado, fuente real, carga/RTO, firma/registry/rollout, gobierno, DIFROL y monitoreo en manifiestos de Kubernetes.

Sigue en pie lo que este documento ya declara fuera: frentes 6, 7, 8, 12, 13, 14, 15 y las unidades BH que exigen entorno.

## 12. Plan de cierre de la semana (2026-10-20)

**Criterio de orden:** dependencias, no urgencia percibida. Cuatro de los seis frentes parciales no necesitan código nuevo: necesitan un entorno o una firma.

### 12.1 Fase 0 — Decisiones, día 1

| # | Decisión | Desbloquea | Quién decide |
| --- | --- | --- | --- |
| D1 | Motor de datos productivo, backups y SLA | Frente 14 y parte del 6 | Arquitectura + cliente |
| D2 | Registry, llaves de firma y acceso al clúster | Frente 7 y la ejecución del 6 | Infraestructura |
| D3 | Responsables de gobierno y aprobación de controles U1 | Frente 8 — **41 de los 132 IDs** | Cliente / jefe de proyecto |
| D4 | Qué subconjunto de los 132 es requisito real | Todo el marcado de IDs | Cliente |
| D5 | Alcance de accesibilidad: ¿WCAG AA? ¿lector de pantalla? | Cierre del frente 9 | Producto |

Salida de la fase: acta con nombre, fecha y criterio por decisión. D3 y D4 son las de mayor retorno: 41 IDs dependen de la primera y, sin la segunda, ningún cierre es defendible.

### 12.2 Fase 1 — Cerrar lo que ya está escrito, días 2–3

| Frente | Qué falta exactamente | Evidencia de cierre |
| --- | --- | --- |
| 5 Observabilidad | Medir RPS, p95, memoria y errores en entorno aislado | Medición registrada; OTel y *ground truth* quedan fuera del frente |
| 6 Autoscaling | Correr k6, aplicar HPA, comparar p95 contra <500 ms y medir RTO | Benchmark reproducible con pasa/falla |
| 11 Hardening | BH-02 (inventario + scan de imágenes con daemon Docker) y BH-04/05/06/10/11 según entorno | Scan y suites por unidad |
| 4 DLQ/replay | E2E: interrumpir PostgreSQL, agotar reintentos, restaurar, reiniciar consumidores; `-race` | Corrida registrada |
| 9 Login/a11y | Certificación WCAG y prueba con lector, si D5 lo incluye | Informe de medición |
| 1 y 2 | `T6` de cada frente: prueba de sustitución y fixture adversarial | Sustitución sin cambios fuera del adaptador (README §5.1) |
| 3, 4, 10 | Ya cerrados | No reabrir |

Cinco IDs pueden cerrarse sin datos porque son documentales: `GOB-I1`, `AME-I1`, `AME-I2`, `RED-I3` y `GNG-I1`. Ninguno está cerrado: faltan la decisión y la firma.

Nota transversal: §5.1 exige procedencia visible en el front. La procedencia ya se muestra en la vista General, incluida la marca de demostración (`frontend/src/views/OverviewView.tsx:171`, `:439`; `frontend/src/domain/provenance.ts:39`); lo que falta con una fuente real es que esa procedencia sea real, y el fixture adversarial sigue siendo trabajo de este frente, no del modelo.

### 12.3 Fase 2 — Bloqueado para acreditar, días 4–5

Solo preparación, sin ejecutar:

- **12 Evaluación científica:** congelar protocolo, métricas y umbrales. No se ejecuta sin *ground truth*.
- **13 Go/No-Go:** dejar el acta con criterios y firmas listas. No se firma.
- **15 DIFROL:** radicar la solicitud de validación y documentar la procedencia de la cartografía.
- **1 y 2:** mantener el contrato y el fixture; ninguna sustitución real.

### 12.4 Fase 3 — Sincronizar el estado

1. Marcar en `backend/docs/checklist-evidence-u*.md` solo IDs con evidencia reproducible.
2. Actualizar el §10 de este documento y [`../tasks/README.md`](../tasks/README.md).
3. Regenerar [`../presentacion-mandante.md`](../presentacion-mandante.md) con el corte final.
4. `python -B odd/tools/check_md_links.py` → debe dar `BROKEN: 0`.

### 12.5 Camino crítico

```text
D3 (gobierno) ──────────────► frente 8 ──► 41 IDs
D1 (motor) ──► frente 14 ──► frente 6 ──► frente 13 (acta)
D2 (registry) ──► frente 7 ──┘
D4 (alcance) ──────────────► marcado de cualquier ID
```

Si D1 y D2 no se resuelven el día 1, los frentes 6, 7, 13 y 14 no arrancan y la semana se consume en los parciales de la Fase 1. Ese es el único riesgo real del plan.

### 12.6 Lo que este plan no hace

- No cierra los 132: eso exige D4 más evidencia por ID, y varios IDs requieren datos reales que están explícitamente fuera.
- No convierte una suite en verde en una casilla marcada.
- No incluye OTel, firma de imágenes ni modelo entrenado: están fuera del alcance permitido y decirlo evita prometer lo que no se entrega.

