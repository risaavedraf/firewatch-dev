# Presentación con el cliente — aplicación completa funcionando antes del modelo

**Objetivo:** dejar la aplicación completa en funcionamiento (backend y frontend) con implementación sintética determinista, con todo lo relacionado con datos y modelos implementado, de modo que lo único pendiente sea el **modelo entrenado real**.
**Alcance:** frentes 1 (ingesta/ciclo), 2 (servicio de inferencia), 3 (`prevention`), 9 (login/móvil/accesibilidad) y el ítem de código abierto del frente 5 (latencia y errores en `gateway` y `jobs`), más la verificación end-to-end local. Todo lo demás queda declarado fuera con su motivo en «Fuera de alcance».
**Cómo leerlo:** este documento es la especificación que leen los agentes. Las tareas se reparten por **superficies disjuntas** para que dos escritores no toquen el mismo archivo; el contrato de `prevention` está congelado en §«Contrato congelado» para que backend y frontend avancen en paralelo. Los criterios por ID del checklist siguen viviendo en [`../../Referencias/tareas_verificacion_implementacion_IA.md`](../../Referencias/tareas_verificacion_implementacion_IA.md).

## Specs

- **S1** — «en 3/4 dias tenemos presentación con el cliente así que tenemos que tener el proyecto listo hasta antes del modelo».
- **S2** — «todo lo que esté relacionado a datos y modelos tiene que estar implementado, aunque sean datos falsos o así».
- **S3** — «necesitamos en funcionamiento la app completa, tanto front y bkac no puede faltar nada».
- **S4** — «si es posible generar más de un worktree en orca para tener agentes trabajando en distintas ramas en paralelo estaría mil veces mejor porfa».
- **S5** — «resuelve el trabajo documental pendiente, borra todos los archivos temp y handoff».
- **S6** — **Fuera de alcance explícito:** el modelo entrenado real y su evaluación científica (frentes 12 y 13), la firma de imágenes/promoción de registry/rollout (frente 7), el autoscaling, la carga y el RTO medido (frente 6), el gobierno U1 (frente 8), la procedencia DIFROL (frente 15), la decisión de motor de datos productivo (frente 14) y las unidades BH que exigen Docker/clúster/trivy. S2 autoriza implementar los frentes 1 y 2 **completos con implementación sintética**; no autoriza inventar requisitos nuevos ni declarar aprobado un modelo.

## Estado verificado al abrir el trabajo (2026-10-17)

| Área | Lo que ya existe | Lo que falta |
| --- | --- | --- |
| Ingesta | Contrato `Provider`/`Registry`/`ProvenanceFrom` con validación estricta de descriptores (`backend/services/risk/internal/source/provider.go:38-78`); proveedores `FixtureProvider` y `SyntheticProvider` (`source/fixture.go:24,38`; `source/synthetic_scenarios.go:38,68`); fusión que excluye recuperaciones incompletas (`source/provider.go:151-163`) | Ningún proveedor real; **ningún scheduler ni ciclo de 15 min**; validación por registro (formato/fecha/coordenadas/duplicados/faltantes) no implementada; frescura por fuente no expuesta |
| Ciclo | `valid_until` se fija 15 min después de pedir un job (`backend/services/risk/internal/repository/store.go:174-175`) | El productor del ciclo; `GET /v1/risk/sources` expone descriptor y procedencia geoespacial, **no** última ejecución ni frescura (`handler/api.go:88`; `handler/sources.go:15-25,82-103`) |
| Modelo | Interfaz `Classifier` con `ID()`, `Kind()`, `Classify()` (`classify/classifier.go:202-212`); `academic-rule-v1` determinista (`classify/academic_rule.go:50`); selección por `MODEL_ID` (`cmd/server/main.go:135-144`); tabla `models` con `version`, `params_hash`, `approved_at`, `metrics`, `active` (`migrations/002_models.sql:41-48,82-84`); `batches.model_version` y `policy_ref` | Ninguna segunda implementación; `approved_at` sembrado NULL (`002_models.sql:56-58,68-71`); sin endpoint de versiones ni de activación/reversión; sin panel que muestre la versión activa |
| `prevention` | Nada: no hay servicio, migración, ruta ni vista | Todo. `frontend/src/views/PendingView.tsx:50` declara que no existe catálogo de acciones |
| Observabilidad | `risk` expone latencia, errores por clase, proceso y pool (`internal/metrics/metrics.go:54-60`, `http.go:61-64`, `process.go:36-58`); `jobs` expone `/metrics` con contadores de broker y DLQ (`cmd/server/main.go:183-188`; `messaging/broker.go:89-99`) | `gateway` **no expone `/metrics`** (su prueba espera 404: `gateway/internal/handler/gateway_test.go:55`); `jobs` sin latencia, errores por clase ni señales de proceso |
| Frontend | 7 rutas (`frontend/src/app/routes.ts:22-29`) con vistas de territorio, historial, fuentes, trabajos, pendientes, sesión y auditoría; estilos responsivos y `prefers-reduced-motion` (`frontend/src/styles/app.css:336-391`) | Sin vista de acciones preventivas; sin panel de frescura del ciclo ni de versión de modelo; sin prueba de accesibilidad ni de ancho móvil |

## Contrato congelado de `prevention` (§S3)

Backend y frontend trabajan en paralelo contra esta frontera; **no se cambia sin avisar al orquestador**, porque un cambio de contrato rompe a la otra rama.

- **Módulo:** `backend/services/prevention` (`firewatch.local/prevention`), servicio propio en el puerto `8084`, esquema PostgreSQL `prevention`, migración `008_preventive_actions.sql`.
- **Tablas:** `prevention.actions` (id, community_code, title, description, priority, status, assignee, due_date, created_by, created_at, updated_at) y `prevention.action_history` (id, action_id, from_status, to_status, actor_sub, note, created_at).
- **Estados y transiciones válidas:** `propuesta` → `asignada` → `en_curso` → `bloqueada` → `en_curso` → `enviada` → `aceptada` | `devuelta`; `devuelta` → `en_curso` (reapertura); `cancelada` desde cualquier estado no terminal. `aceptada` y `cancelada` son terminales. Toda otra transición responde `409 invalid_transition` y no escribe nada.
- **Rutas** (mismo formato de error `{"code": ..., "request_id": ...}` que los demás servicios):

| Método | Ruta | Rol | Respuesta |
| --- | --- | --- | --- |
| GET | `/v1/prevention/actions?community_code=&status=` | viewer/admin | 200 lista |
| POST | `/v1/prevention/actions` | admin | 201 creada (`propuesta`) |
| GET | `/v1/prevention/actions/{id}` | viewer/admin | 200 detalle + historial |
| POST | `/v1/prevention/actions/{id}/transitions` (`{to, note}`) | admin | 200 nueva versión; 409 transición inválida; 422 entrada inválida |

- **Auditoría:** cada transición escribe `action_history` y emite el evento de auditoría ya existente en `jobs`/`risk`.

## Tasks

Una línea por tarea. «Ruta» nombra quién escribe: `worker` (agente en worktree propio) o `inline` (el orquestador). «Commit» se completa al cerrar la tarea.

### Ola 1 — tres superficies disjuntas en paralelo (S4)

| ID | Spec | Superficie | Ruta | Descripción | Commit |
| --- | --- | --- | --- | --- | --- |
| T1 | S2 | `backend/services/risk/internal/source/**` | worker `feat/ingesta-ciclo` | Proveedor HTTP/JSON configurable por entorno (`FIREWATCH_SOURCE_URL`) que normaliza a `ObservationSet`, con `fixture` como valor por defecto determinista y sin red. | — |
| T2 | S2 | `source/**`, `repository/**` | worker `feat/ingesta-ciclo` | Validación por registro (formato, fecha, coordenadas, duplicados, faltantes) que rechaza el registro inválido con causa registrada, y no publica lotes incompletos. | — |
| T3 | S2 | `cmd/server/**`, `source/**` | worker `feat/ingesta-ciclo` | Scheduler de 15 min que dispara ingesta → validación → clasificación → publicación, más `POST /v1/risk/cycle/run` (admin) para disparo manual en la demo. | — |
| T4 | S2 | `internal/handler/**` | worker `feat/ingesta-ciclo` | Exponer estado y frescura por fuente (última ejecución, próxima, completa/incompleta, antigüedad) en `GET /v1/risk/sources`. | — |
| T5 | S2 | `internal/classify/**`, `migrations/002_models.sql` (datos) | worker `feat/ingesta-ciclo` | Segunda implementación real y determinista de `Classifier` (`firewatch-rule-v2`) con parámetros versionados, registrada junto a `academic-rule-v1`; ninguna reemplaza a la otra por defecto. | — |
| T6 | S2 | `internal/handler/**`, `internal/repository/**` | worker `feat/ingesta-ciclo` | Endpoint de versiones de modelo con activación y reversión (`GET /v1/models`, `POST /v1/models/{version}/activate`), que fija `approved_at`, `metrics` y `active` de forma transaccional. | — |
| T7 | S3 | `backend/services/prevention/**`, `migrations/008_*` | worker `feat/prevention` | Máquina de estados de §«Contrato congelado» con **pruebas de transiciones válidas e inválidas escritas antes** de la implementación (test-first). | — |
| T8 | S3 | `backend/services/prevention/**` | worker `feat/prevention` | Servicio completo: migración, repositorio, handlers y las cuatro rutas del contrato, reutilizando el middleware de autenticación y el formato de error existentes. | — |
| T9 | S3 | `backend/go.work`, `backend/docker-compose.yml`, `backend/deploy/k8s/base/**`, `.github/workflows/_go-service.yml` | worker `feat/prevention` | Registrar el módulo nuevo en el workspace, la pila local, el manifiesto de Kubernetes y la matriz de CI. **Archivos compartidos: solo añadir la línea propia, nunca reformatear el resto.** | — |
| T10 | S3 | `backend/services/gateway/**` | worker `feat/metrics-operacion` | `/metrics` en `gateway` con latencia (histograma), errores por clase y señales de proceso, con el mismo patrón que `risk/internal/metrics` (hoy su prueba espera 404 y debe cambiar a 200). | — |
| T11 | S3 | `backend/services/jobs/**` | worker `feat/metrics-operacion` | Añadir a `jobs` latencia, errores por clase y señales de proceso **sin** perder los contadores de broker/DLQ que ya expone. | — |

### Ola 2 — frontend e integración

| ID | Spec | Superficie | Ruta | Descripción | Commit |
| --- | --- | --- | --- | --- | --- |
| T12 | S3 | `frontend/src/**` | worker `feat/frontend-completo` | Vista `#actions` contra el contrato congelado: lista, detalle con historial, alta y transiciones habilitadas por rol; retirar de `PendingView` la afirmación de que no existe catálogo de acciones. | — |
| T13 | S3 | `frontend/src/views/SourcesView.tsx`, `OverviewView.tsx` | worker `feat/frontend-completo` | Mostrar frescura y última ejecución del ciclo, y la versión de modelo activa con su marca `synthetic`. | — |
| T14 | S3 | `frontend/src/**` | worker `feat/frontend-completo` | Recorrido de login/logout real con Keycloak y llamada autenticada; el 401/403 debe mostrar el `request_id` recibido. | — |
| T15 | S3 | `frontend/src/styles/**`, `frontend/tests/**` | worker `feat/frontend-completo` | Revisión móvil a 390 px y accesibilidad: foco visible, etiquetas de control, contraste AA y navegación por teclado, con prueba automatizada que falle si se pierde. | — |
| T16 | S1,S3 | — | inline | Integración en la rama de presentación por rebanadas revisables; resolver los archivos compartidos (§T9) una sola vez y verificar que la pila local levanta. | — |
| T17 | S1 | — | inline | Verificación end-to-end sobre la pila local: ingesta → clasificación → publicación → historial → acción preventiva, con evidencia por spec y las suite de los cuatro módulos. | — |
| T18 | S1 | `README.md`, `odd/tasks/README.md`, `odd/plan-produccion/**` | inline | Actualizar el estado entregado/faltante del índice y del plan de producción con la evidencia observada. | — |
| T19 | S1 | — | inline | PRs por rebanada. **Merge y push siguen siendo decisión del usuario**, incluidos el PR #29 y el PR #28 ya abiertos. | — |

## Olas y worktrees (S4)

Superficies disjuntas por rama; los archivos compartidos se tocan una sola vez y solo para añadir la línea propia.

| Ola | Rama / worktree | Tareas | Módulo |
| --- | --- | --- | --- |
| 1 | `feat/ingesta-ciclo` | T1–T6 | `risk` |
| 1 | `feat/prevention` | T7–T9 | `prevention` (nuevo) |
| 1 | `feat/metrics-operacion` | T10–T11 | `gateway`, `jobs` |
| 2 | `feat/frontend-completo` | T12–T15 | `frontend` |
| cierre | `feat/presentacion-cliente` (integración) | T16–T19 | — |

## Fuera de alcance

- Modelo entrenado real, aprobación formal y evaluación científica (frentes 12 y 13): no se declara aprobado ningún modelo; la implementación sintética se rotula como tal en API y UI.
- Firma de imágenes, promoción de registry y rollout (frente 7); autoscaling, carga y RTO medido (frente 6); gobierno U1 (frente 8); DIFROL (frente 15); motor de datos productivo (frente 14).
- BH-02 (inventario y scan de imágenes), BH-04/05/06, BH-10, BH-11 y BH-12.

## Log

- **L1 (2026-10-17)** — Pedido del usuario, verbatim: «resuelve el trabajo documental pendiente, borra todos los archivos temp y handoff / luego que me generes un plan para terminar el codigo pronto ya que en 3/4 dias tenemos presentación con el cliente así que tenemos que tener el proyecto listo hasta antes del modelo / todo lo que esté relacionado a datos y modelos tiene que estar implementado, aunque sean datos falsos o así, necesitamos en funcionamiento la app completa, tanto front y bkac no puede faltar nada / si es posible generar más de un worktree en orca para tener agentes trabajando en distintas ramas en paralelo estaría mil veces mejor porfa».
- **L2 (2026-10-17)** — Trabajo documental de S5 ejecutado y verificado en esta sesión: `odd/tasks/backend-hardening/` (12 archivos) eliminado tras comprobar que eran las versiones previas de la copia verificada en `security/backend-hardening/`; las tres referencias backtick obsoletas de `odd/urgent/README.md` corregidas; los tres handoffs retirados (`handoff-sesion-a2.md`, `odd/temp/handoff-ruta-completa-demo.md`, `handoff-playwright-signed-in.md`) con su contenido autoritativo trasladado a `odd/plan-produccion/README.md` §4 (D1 y D7) y §9.5 (rebanadas del PR #29) y sus ocho referencias repuntadas; `tmp/` limpiado. `python -B odd/tools/check_md_links.py` → 95 archivos, `BROKEN: 0`. **Una excepción declarada:** `tmp/gateway-frontend-separation-1791858059/` (23 archivos, copia byte a byte del mockup retirado) se conservó porque es la única copia que existe y su borrado es irreversible; queda a decisión del usuario.
- **L3 (2026-10-17)** — Mapa de estado por área producido por un explorador de solo lectura (`gentle-ai-explore`), resumido en §«Estado verificado». De ese mapa salen las seis correcciones de alcance de este plan: el *seam* `Provider` ya estaba congelado (el frente 1 no empieza de cero), la tabla `models` ya existe con `approved_at`/`active`, `prevention` no tiene absolutamente nada implementado, `gateway` no expone `/metrics`, `jobs` expone contadores pero no latencia, y el frontend tiene siete vistas sin ninguna de acciones preventivas.
