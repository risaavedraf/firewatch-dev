# Urgente — preparación MVP FireWatch

**Estado:** foto de planificación del **2026-10-14**, corregida el 2026-10-20. **No es el estado vigente.** El corte actual es [`../estado-requerimientos-cliente.md`](../estado-requerimientos-cliente.md) y el trabajo por frente vive en [`../plan-produccion/README.md`](../plan-produccion/README.md).

**Propósito:** carpeta temporal para preparar la reunión con el jefe de proyecto y organizar el trabajo derivado del checklist IA. Es una cola de planificación, no una declaración de que sus tareas ya estén implementadas.

## Lectura rápida para mañana

La evidencia revisada describe un prototipo/demo local con clasificación académica y entradas sintéticas. Hay componentes backend y geoespaciales descritos como implementados y pruebas registradas en ODD, pero esta revisión no levantó el stack ni ejecutó esas pruebas. No se debe presentar como predicción basada en observaciones reales, producto desplegado ni sistema aprobado para producción.

| Afirmación | Estado que se puede comunicar | Evidencia / límite |
|---|---|---|
| Demo geoespacial y flujo backend | Implementación documentada; validar la demo en el entorno de presentación | `../tasks/backend/firewatch-app-funcional.md`; `../tasks/backend/firewatch-geospatial-map.md`; código de riesgo en `backend/services/risk/internal/handler/api.go` |
| Escenarios sintéticos: catálogo de seis perfiles y selección opcional por API | Implementado y probado dentro de su alcance sintético; la ruta autenticada completa se ejecutó una vez en un stack desechable aislado | `../tasks/backend/urgent-synthetic-mvp.md`; `../../backend/docs/synthetic-scenarios.md`; `../../backend/contracts/openapi/firewatch-v1.yaml` |
| Naturaleza de la clasificación | Regla académica; datos sintéticos, no riesgo observado | `../tasks/backend/firewatch-data-contract.md` |
| Pruebas frontend | Hay una ronda registrada; no acredita login/logout real en Keycloak ni revisión móvil | `../tasks/reviews/judgment-day-frontend-round2.md`; pendientes indicados en `../tasks/frontend/client-proposal-page.md` |
| Hardening | Pendiente/no demostrado por la documentación revisada | `../tasks/security/backend-hardening.md`; unidades `../tasks/security/backend-hardening/` |
| Preparación operativa y validación científica | No hay evidencia suficiente de carga/SLA, drift con ground truth, benchmark del modelo ni Go/No-Go firmado | `../../Referencias/tareas_verificacion_implementacion_IA.md` |

### Guion recomendado

1. Mostrar el flujo que se pueda ejecutar y confirmar antes de la reunión; si no se ejecutó, decirlo.
2. Describirlo como prototipo académico de demostración, con clasificación sintética.
3. Mostrar brechas como trabajo planificado, no como capacidades existentes.
4. Pedir al jefe de proyecto que confirme el alcance mínimo para clientes, los datos permitidos, los criterios de aceptación y quién aprueba riesgos/modelo.

**No afirmar sin nueva evidencia:** exactitud del modelo, riesgo real por comuna, rendimiento/SLA, disponibilidad, seguridad aprobada, monitoreo productivo, cumplimiento regulatorio o aptitud para decisiones de emergencia.

### PASS en una prueba no es una casilla marcada

Son dos objetos distintos y uno no se convierte en el otro:

- **Prueba en PASS** observa un comportamiento acotado y ejecutable hoy (`go test`, `go vet`). No cierra un requisito.
- **Casilla del checklist marcada** exige el criterio de cierre del ID: evidencia reproducible, umbrales acordados con el equipo, responsable y, cuando aplica, datos reales. Una prueba sintética no puede aportar eso.

Por eso puede ocurrir —y ocurre— que una prueba esté en PASS y su ID siga sin marcar. Estado remedido el **2026-10-14**: 132 IDs asignados, **0 marcados**, 55 casillas de verificación abiertas y, en el módulo `risk`, la suite completa reporta **293 PASS / 9 omitidos / 0 fallos** con `go vet` sin diagnósticos. Los 9 omitidos dependen de una base de datos y se ejecutan aparte en bases desechables aisladas; «suite en 0» no significa que esos 9 se hayan ejecutado. Las corridas históricas de bases desechables y del arnés E2E autenticado quedan como evidencia documental registrada, no reproducida en esta remedición.

## Qué tenemos y qué falta (estado consolidado)

Síntesis por área, para separar lo construido de lo planificado. La evidencia por ID vive en [tareas_verificacion_implementacion_IA.md](../../Referencias/tareas_verificacion_implementacion_IA.md) y en los registros `backend/docs/checklist-evidence-u*.md`; esta tabla no los reemplaza.

**El corte vigente es [estado-requerimientos-cliente.md](../estado-requerimientos-cliente.md).** Lo que sigue es la foto del 2026-10-14 con las filas que el código contradecía ya corregidas y anotadas; no debe leerse como un segundo corte paralelo.

| Área | Qué tenemos (con evidencia) | Qué falta o está solo parcial |
|---|---|---|
| **Backend / flujo asíncrono** | `gateway` (JWT RS256, CORS exacto, límite local), `jobs` (idempotencia, transacción job+audit+outbox) y `risk` (consumidor, clasificación, publicación de lote) cableados de extremo a extremo sobre RabbitMQ; relay con publisher confirms, DLQ, migraciones embebidas y PostGIS/GeoJSON; servicio `prevention` con su vista de navegador; ciclo de 15 min con frescura por fuente; replay de DLQ autenticado | Ingesta real: fuente de observación real y sus rangos productivos |
| **Clasificación** | Regla académica `academic-rule-v1` sobre entrada **sintética** (6 perfiles), con pruebas | Modelo real, validación científica, drift con ground truth, umbrales aprobados |
| **Geoespacial / frontend** | Mapa de comunas de Valparaíso; app React 19 + TS + Vite (7 vistas, OIDC PKCE, servidor estático con CSP) | Login/logout real en navegador (ronda E2E no ejecutada); revisión móvil/accesibilidad |
| **Contratos** | Eventos versionados con compuerta real de round-trip (`contracts/gen/events/v1`) | OpenAPI validado **solo sintácticamente**; pendiente decisión de contrato-como-verdad (hallazgo M6) |
| **Observabilidad / operación** | `/metrics` por proceso, readiness y profundidad/edad de DLQ; Prometheus con siete reglas de alerta; Grafana con datasource y dashboard versionados; contador `firewatch_high_risk_predictions_total` con panel propio; cuatro HPA; escenario de carga k6 escrito | OpenTelemetry (trazas distribuidas); p95/SLO medidos: la herramienta de carga existe, la corrida no |
| **Seguridad / entrega** | Contenedores non-root, FS read-only, sin capabilities; secretos fuera del build; bodies/timeouts acotados; CI corriendo en verde en `main` (Jobs, Risk, Gateway y Backend integration) | Firma de imágenes, promoción de registry y rollout automatizado |
| **Checklist IA (132 IDs)** | Suites en verde a nivel sintético (`risk`: 293 PASS / 9 omitidos / 0 fallos; `go vet` limpio) | **0 de 132 IDs marcados**, 55 casillas abiertas; U4: 23 parciales (solo sintético), 19 bloqueados; umbrales, responsables, datos reales y Go/No-Go sin firmar |

> **Corrección del 2026-10-20 (base `267a4bb`).** Tres filas de esta tabla se escribieron contra un árbol anterior y contradecían el código. Se corrigieron en el lugar y se conserva aquí lo que decían, porque este archivo es memoria de planificación y no estado vigente.
>
> 1. «Ingesta real y ciclo de 15 min; servicio `prevention`; replay de DLQ» — solo la ingesta real sigue faltando. El servicio existe en `backend/services/prevention/` (puerto 8084, esquema `prevention`, migración `009`; `backend/deploy/k8s/base/workloads.yaml:134`), el ciclo corre al arranque y cada 15 minutos (`backend/README.md:38`, `backend/docs/architecture.md:63`) y el replay es la ruta autenticada `POST /v1/jobs/{id}/replay` (`backend/services/jobs/internal/handler/api.go:187`).
> 2. «Grafana (no existe) […] contador `high_risk_predictions_total` (no existe); HPA/autoscaler, herramienta de carga» — los cuatro artefactos existen: Grafana en `backend/docker-compose.yml:187` con provisioning y dashboard versionados en `backend/deploy/local/grafana/`; el contador se emite como `firewatch_high_risk_predictions_total` en `backend/services/risk/internal/metrics/metrics.go:56` y lo consume el panel `backend/deploy/local/grafana/dashboards/firewatch-overview.json:190`; hay cuatro `HorizontalPodAutoscaler` en `backend/deploy/k8s/base/autoscaling.yaml`; y el escenario de carga está en `backend/deploy/load/k6-risk.js`. Lo que falta es la **medición** (p95/SLO y lectura bajo carga), no el artefacto. OpenTelemetry sí sigue ausente en todo el árbol.
> 3. «U4: 10 parciales, 32 bloqueados» — el registro de U4 declara 42 IDs: 23 con evidencia parcial y 19 bloqueados (`backend/docs/checklist-evidence-u4.md`).
>
> Los agregados completos, el reparto por unidad y la lista de lo que falta por dominio están en [estado-requerimientos-cliente.md](../estado-requerimientos-cliente.md).

Lectura clave: lo que funciona conserva la etiqueta **sintética/académica**; lo que falta no es un error, sino alcance no construido o no demostrado. La deriva de citas de la explicación del código está anotada en [code-explanation/README.md](../../backend/docs/code-explanation/README.md).

## Unidades ODD

Cada ID del checklist fuente está asignado una sola vez. “Pendiente de verificación” significa que esta organización no comprobó ese requisito de forma individual; no equivale a afirmar que el código relacionado no exista. La evidencia debe registrarse y el estado debe actualizarse al ejecutar cada unidad.

| Unidad | Dominio | IDs asignados | Archivo |
|---|---|---:|---|
| U1 | Gobierno, seguridad y protección de datos/modelo | 41 | `01-seguridad-gobierno.md` |
| U2 | Calidad, sesgo y fugas de datos | 18 | `02-pruebas-datos.md` |
| U3 | Evaluación y robustez del modelo | 11 | `03-pruebas-modelo.md` |
| U4 | MLOps, rendimiento, optimización y monitoreo | 42 | `04-operacion-monitoreo.md` |
| U5 | Stack, pruebas continuas, Go/No-Go y práctica | 20 | `05-ct-aprobacion-demo.md` |
| **Total** | | **132** | |

## Prioridad de trabajo

- **Antes de presentar:** confirmar alcance, preparar una demo reproducible y decir con claridad qué es sintético y qué no se ha medido. No intentar representar las 132 tareas como completadas.
- **Después de acordar el mínimo con el jefe de proyecto:** ejecutar las unidades U1–U5 conforme a los criterios de la fuente. No dar por supuesto que todo el checklist es requisito de la demo de mañana.
- **Antes de clientes:** acordar por escrito qué se puede mostrar, datos disponibles, criterios de aceptación, validaciones de autenticación y la lista de afirmaciones permitidas.

## Fuente de verdad y limpieza

El detalle normativo y los criterios íntegros están en `../../Referencias/tareas_verificacion_implementacion_IA.md`; este backlog los referencia por ID, no los reemplaza. El documento de seguimiento ODD es `../tasks/presentation/presentacion-mvp.md`. Esta carpeta es temporal y se puede retirar cuando el equipo lo decida; no existe una eliminación automática.
