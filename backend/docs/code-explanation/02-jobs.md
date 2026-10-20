# Jobs: aceptar una solicitud y cerrar su ciclo con auditoría

**Re-derived on 2026-10-20 against the code of commit `267a4bb`.**

`jobs` mantiene trabajos `queued`/`published`/`failed`, reutiliza una solicitud repetida del mismo actor, registra comando y resultado y devuelve a `queued` un trabajo fallido. El 202 depende de una transacción PostgreSQL, no de que RabbitMQ esté disponible ni de que el lote ya exista.

[Índice e instantánea](README.md) · [Gateway](01-gateway.md) · [Siguiente: risk](03-risk.md)

## Alcance y mapa

Lecturas completas: [api.go](../../services/jobs/internal/handler/api.go):1-237, [security.go](../../services/jobs/internal/handler/security.go):1-62, [status.go](../../services/jobs/internal/handler/status.go):1-52 y [store.go](../../services/jobs/internal/repository/store.go):1-315. Arranque: [despliegue](07-deployment.md); tablas: [persistencia](06-database.md); callback de resultados: [mensajería](04-messaging.md). No se ejecutó código; la identidad y límites comunes constan en el [índice](README.md).

| Declaración | Interfaz / responsabilidad | Llamador / dependencia |
| --- | --- | --- |
| `Repository` — `services/jobs/internal/handler/api.go:19-35` | `Create` (variádico: escenario opcional), `Get`, `Audit`, `ZoneExists`, `Replay`, todos con contexto | Handler independiente del pool; `Store` inyectado por main |
| `Verifier`, `subjectKey` — `services/jobs/internal/handler/security.go:13-16` | Token verificado y clave de contexto privada | `Authorize`; impide colisión con claves string |
| `Store`, `Job`, `Audit` — `services/jobs/internal/repository/store.go:15-57` | Pool, proyección compartida, trabajo y registro auditable | UUIDs como string; `BatchID`, `ScenarioID`, `EvaluatedAt`, `FailureReason` y `FailedAt` punteros permiten JSON null; `Attempts` siempre presente |
| `New` — `services/jobs/internal/handler/api.go:80-237` | Construye el mux de cinco rutas y lo envuelve con autorización | SQL solo detrás de interfaz; datos JSON no incluyen actor del trabajo |
| `replayConfirmToken`, `replayConfirmation`, `knownScenarioID` — `services/jobs/internal/handler/api.go:41-78` | Body de replay estricto y pertenencia al vocabulario de escenarios | Rechazan en el transporte, antes de tocar el store |

Los imports (api:3-17, security:3-11, status:3-7, store:3-13) conectan JSON/HTTP, OIDC, UUID, pgx y contratos v1. No comparten repositorios con `risk`.

## Autorización antes de las rutas

`Authorize` procesa cada ruta protegida, incluida `POST /v1/jobs/{id}/replay`: el rol se comprueba antes de leer el body de confirmación. La autenticación tiene prioridad sobre validación de contenido; por eso un viewer enviando un body inválido obtiene 403, no necesariamente 400.

| Rama — `services/jobs/internal/handler/security.go` | Entrada / transformación | Salida / efectos |
| --- | --- | --- |
| 20-24 | Authorization debe tener exactamente esquema y token; comparación Bearer case-insensitive | 401 sin llamar siguiente |
| 25-31 | `Verify` con contexto de 5 s y cancel diferido | Error o subject vacío: 401 |
| 32-42 | Extrae `typ`, `nbf`, `realm_access.roles` | Claims ilegibles, `typ != Bearer` o nbf futuro: 401 |
| 43-49 | Itera roles; solo `admin` autoriza | Coloca subject verificado en contexto original y llama siguiente; sin admin: 403 |
| 53-62 | `respond` fija JSON/nosniff/no-store y status; `fail` añade código e ID | Ignora error al codificar; ID puede venir de wrapper main |

El timeout de verificación no se convierte en contexto de negocio: `WithValue` parte de `r.Context()`. El wrapper de main ya aporta un timeout de 10 s (`services/jobs/cmd/server/main.go:147-167`). El tipo esperado del subject se afirma mediante type assertion sin comprobación ni en el POST ni en el replay (`api.go:161,208`); es seguro por la conexión local `New -> Authorize` cuando se cumple el contrato del verifier, no por aceptar una cabecera de identidad.

## API: guardas y errores HTTP

| Unidad / líneas de `services/jobs/internal/handler/api.go` | Condición / operación | Resultado |
| --- | --- | --- |
| status, 86 | `GET /v1/jobs/status` con `statusHandler` (`status.go:38-52`) | 200 si `Ready`, 503 si no; sin guardas de body |
| POST, 88-92 | `mime.ParseMediaType` acepta `application/json`, incluso parámetros válidos | Tipo distinto o error: 415 `json_required` |
| POST, 93-97 | Parsea Idempotency-Key UUID y rechaza Nil | 400 `invalid_idempotency_key` |
| POST, 98-117 | Body limitado a 8192; struct con `zone_id` y `scenario_id` opcional; campos desconocidos prohibidos; segunda lectura debe ser EOF | Error de JSON, múltiples documentos o body sobredimensionado: 400 `invalid_demo_request` |
| POST, 122-134 | Presencia cruda de `scenario_id`: `null` o `""` explícitos rechazados | 400 `invalid_demo_request` sin escritura |
| POST, 138-141 | Escenario debe pertenecer a `v1.SyntheticScenarioIDs()` | 400 `invalid_demo_request` sin escritura |
| POST, 146-155 | `ZoneExists` contra el catálogo `zones` | Error: 503 `storage_unavailable`; zona ausente: 400 `invalid_demo_request` |
| POST, 156-161 | Parsea request ID o genera otro; subject desde contexto; pasa actor, key normalizada, zona, trace y escenario | ID en respuesta antes de almacenamiento |
| POST, 162-168 | `Create` retorna trabajo y error de commit incluido | Error: log sin datos del body y 503; éxito: Location y 202, incluso trabajo previamente publicado |
| GET job, 170-186 | Parsea ID; llama `Get` | 400 si formato inválido; `pgx.ErrNoRows` -> 404; otro error -> 503; éxito 200 |
| replay, 187-227 | Echo de request ID, parseo de ID, confirmación estricta, `Replay` | 400 `invalid_replay_request`; 404 `job_not_found`; 409 `replay_confirmation_required`; 409 `job_not_replayable`; 503; éxito 202 |
| GET audit, 228-235 | Llama `Audit` sin parámetros de usuario | Error 503; éxito 200, lista hasta 100 |
| Construcción, 80-86,236 | Mux por método y ruta envuelto con `Authorize` | Métodos no registrados quedan al mux, no a errores JSON uniformes |

La guarda GET no rechaza explícitamente UUID Nil como el POST ni el replay. No hay filtro por propietario en `Get` ni `Audit`: el permiso es `admin`, no “solo mis trabajos”. El POST valida zona y escenario contra el catálogo, pero no compara el payload de una repetición con el almacenado: en conflicto devuelve la fila guardada con su escenario original, así que un reintento con otro escenario no la reemplaza.

## Repositorio: aceptación atómica

`Create(ctx, actor, key, zone, trace, scenario…)` abre transacción, instala rollback diferido y genera ID candidato (`services/jobs/internal/repository/store.go:79-89`). El rollback es limpieza de salida fallida; tras commit ya no revierte la transacción comprometida según el contrato esperado de pgx, no inspeccionado aquí.

| Rama / líneas de `services/jobs/internal/repository/store.go` | Estado previo / operación | Resultado / error |
| --- | --- | --- |
| 84-88 | `Begin` | Error devuelve `Job{}`; si abre, rollback diferido |
| 89-98 | ID candidato, punteros tipados y, con escenario, un instante UTC asignado por el servidor | Sin escenario ambos punteros quedan nil y el INSERT recibe NULL explícito |
| 99-109 | INSERT queued con conflicto `(actor_id,idempotency_key)` ignorado; conflicto da `ErrNoRows` en RETURNING | Busca trabajo existente del mismo par y hace commit; no añade outbox ni audit de repetición |
| 110-118 | Error SQL en el INSERT aborta; `requestEvent` se construye desde la fila guardada y se serializa | Error SQL o JSON aborta antes de commit |
| 119-125 | Inserta outbox y audit `job.requested` en esa misma tx, luego commit | Cualquier error revierte; devuelve trabajo junto al error de commit |
| 128-136 | `Get`: SELECT por ID con la proyección compartida y scan | Error conservado para que handler distinga no encontrado |
| 138-153 | `Audit`: SELECT orden ID descendente LIMIT 100; slice vacía no nil; loop Scan y append | Error inicial/scan/final `rows.Err` se propaga; cierre diferido |

El código no envía AMQP durante el POST. La fila outbox se publica posteriormente; con broker caído puede existir un trabajo aceptado durable y todavía `queued`. Un error de commit puede dejar al cliente sin certeza del resultado; la repetición con la misma clave sirve para recuperar esa incertidumbre, no para prometer que todo 503 equivale a “ningún cambio ocurrió”.

## Repositorio: resultado y deduplicación

`Complete` es el callback configurado por main, no un endpoint HTTP; `Fail` comparte ese registro y `Replay` es la ruta de recuperación del operador. El broker valida el envelope antes de llamar a `Complete`; el método no repite esa validación (`services/jobs/cmd/server/main.go:130`; [mensajería](04-messaging.md)).

| Rama / líneas de `services/jobs/internal/repository/store.go` | Operación / condición | Postcondición |
| --- | --- | --- |
| 166-171 | `Complete`: Begin + rollback diferido | Fallos se devuelven al broker |
| 172-178 | INSERT inbox con ID evento, conflicto ignorado | Inbox ya contiene el ID: commit sin modificar trabajo ni audit; dedupe por evento |
| 179-187 | UPDATE solo si ID, actor y zona coinciden y estado queued | Si una fila cambia, asigna batch y añade `batch.published`; error audit revierte estado e inbox |
| 188-197 | UPDATE no cambió fila | Consulta existencia de trabajo ya published con mismo batch/actor/zona; si coincide acepta resultado repetido con otro ID; si no, `errNoMatchingJob` |
| 161-164 | `errNoMatchingJob` se declara `Permanent()` | El consumidor manda el mensaje a DLQ sin reintentar |
| 198 | Commit | Error vuelve al broker; solo éxito permite Ack |
| 206-209 | `Fail`: UPDATE a `failed` con razón y `attempts` | Solo transiciona desde queued; ID desconocido no es error |
| 242-254 | `replayGate` | Fallido siempre reejecutable; queued solo con confirmación; published nunca |
| 275-298 | `Replay`: UPDATE guardado de la fila y RETURNING; sin fila lee el estado y aplica `replayGate` | 404 `pgx.ErrNoRows`; 409 `ErrReplayConfirmationRequired` o `ErrJobNotReplayable`; sin efectos |
| 299-314 | Reconstruye el evento desde la fila con actor y zona almacenados e ID nuevo; outbox y audit con el operador; commit | Error revierte todo; `requested_at` no se reescribe |

No se consulta la base de `risk` para verificar lote: la identidad del resultado se comprueba contra el trabajo local y el envelope. Un resultado que no coincide se declara permanente y no cambia el estado: la fila sigue `queued`, porque `Fail` solo escribe cuando el fallo viene de la entrega del propio trabajo.

## Traza razonada de idempotencia y finalización

Supuestos: actor `A` autenticado admin, UUIDs válidos `K`/`T`, DB y commits exitosos. Letras representan UUIDs ilustrativos, no valores enviados literalmente.

| Paso | Cita | Estado antes -> después |
| --- | --- | --- |
| POST inicial | `services/jobs/internal/handler/api.go:88-161` | JSON demo + K (+ escenario) -> llamada Create(A,K,zona,T,escenario) |
| Transacción | `services/jobs/internal/repository/store.go:99-125` | Sin par A/K -> job J queued, outbox E solicitado, audit solicitado; commit -> 202 |
| Repetición | `services/jobs/internal/repository/store.go:99-109` | Mismo A/K -> J existente; sin nueva audit/outbox |
| Resultado de risk | `services/jobs/internal/repository/store.go:172-187` | E2 nuevo, J queued -> inbox E2, J published con B, audit publicado |
| Redelivery | `services/jobs/internal/repository/store.go:172-178` | E2 ya en inbox -> commit sin repetir efectos |
| Replay de un fallido | `services/jobs/internal/repository/store.go:275-314` | J failed -> J queued, attempts 0, E3 nuevo, audit del operador |

**Alternativa de error:** si el trace del resultado no cumple el tipo UUID SQL, falla el INSERT de audit después del UPDATE. La función retorna error y rollback revierte UPDATE e inbox (`services/jobs/internal/repository/store.go:179-187`). El broker reintenta y finalmente puede rechazar a DLQ, no devolver un HTTP 503 a la petición original. El test de integración expresa este escenario (`services/jobs/internal/repository/store_integration_test.go:43-50`), pero no se ejecutó.

## Implementado, pretendido y cobertura

**Implementado:** idempotencia por actor/clave; consultas globales de admin; transacciones de aceptación y finalización; dedupe por ID evento y comprobación de resultado repetido; estado `failed` durable con `attempts`; validación de escenario contra el contrato y de zona contra el catálogo; replay guardado con confirmación explícita. **Intención:** OpenAPI describe 202 durable, catálogo de zonas y escenario opcional (`contracts/openapi/firewatch-v1.yaml:47-100`) y tests rechazan entradas antes de persistencia (`services/jobs/internal/handler/api_test.go:268-282,333-335`). No son resultados observados.

| Archivo / unidades | Inventario explicado | Estado |
| --- | --- | --- |
| `api.go:1-237` | Imports/interfaz/claves; constructor; guardas POST; status/GET/replay/audit; wrapper | Explicado |
| `security.go:1-62` | Imports/interfaz/clave; claims; timeout; loop roles; respond/fail | Explicado |
| `status.go:1-52` | `StatusCheck`; tipos de estado; handler 200/503 | Explicado |
| `store.go:1-315` | Imports/structs/proyección; Create/conflicto/evento; Get; Audit; Complete/dedup/match; Fail; Replay/gate | Explicado |

Límites: semántica de pgx y aislamiento/concurrencia real no ejecutados; callbacks dependen de broker y esquema; el replay no se probó contra un broker real. Verificación posterior propuesta: conflictos concurrentes, commit incierto, replays concurrentes y resultado con actor/batch incorrectos.
