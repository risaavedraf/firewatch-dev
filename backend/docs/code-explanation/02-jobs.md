# Jobs: aceptar una solicitud y cerrar su ciclo con auditoría

`jobs` mantiene trabajos `queued`/`published`, reutiliza una solicitud repetida del mismo actor y registra comando y resultado. El 202 depende de una transacción PostgreSQL, no de que RabbitMQ esté disponible ni de que el lote ya exista.

[Índice e instantánea](README.md) · [Gateway](01-gateway.md) · [Siguiente: risk](03-risk.md)

## Alcance y mapa

Lecturas completas: [api.go](../../services/jobs/internal/handler/api.go):1-85, [security.go](../../services/jobs/internal/handler/security.go):1-62 y [store.go](../../services/jobs/internal/repository/store.go):1-122. Arranque: [despliegue](07-deployment.md); tablas: [persistencia](06-database.md); callback de resultados: [mensajería](04-messaging.md). No se ejecutó código; la identidad y límites comunes constan en el [índice](README.md).

| Declaración | Interfaz / responsabilidad | Llamador / dependencia |
| --- | --- | --- |
| `Repository` — `services/jobs/internal/handler/api.go:17-21` | `Create`, `Get`, `Audit`, con contexto | Handler independiente del pool; `Store` inyectado por main |
| `Verifier`, `subjectKey` — `services/jobs/internal/handler/security.go:13-16` | Token verificado y clave de contexto privada | `Authorize`; impide colisión con claves string |
| `Store`, `Job`, `Audit` — `services/jobs/internal/repository/store.go:15-29` | Pool, respuesta de trabajo y registro auditable | UUIDs como string; `BatchID` puntero permite JSON null; fechas `time.Time` |
| `New` — `services/jobs/internal/handler/api.go:23-85` | Construye rutas y las envuelve con autorización | SQL solo detrás de interfaz; datos JSON no incluyen actor del trabajo |

Los imports (api:1-15, security:1-11, store:1-13) conectan JSON/HTTP, OIDC, UUID, pgx y contratos v1. No comparten repositorios con `risk`.

## Autorización antes de las rutas

`Authorize` procesa cada ruta protegida. La autenticación tiene prioridad sobre validación de contenido; por eso un viewer enviando un body inválido obtiene 403, no necesariamente 400.

| Rama — `services/jobs/internal/handler/security.go` | Entrada / transformación | Salida / efectos |
| --- | --- | --- |
| 18-24 | Authorization debe tener exactamente esquema y token; comparación Bearer case-insensitive | 401 sin llamar siguiente |
| 25-31 | `Verify` con contexto de 5 s y cancel diferido | Error o subject vacío: 401 |
| 32-42 | Extrae `typ`, `nbf`, `realm_access.roles` | Claims ilegibles, `typ != Bearer` o nbf futuro: 401 |
| 43-49 | Itera roles; solo `admin` autoriza | Coloca subject verificado en contexto original y llama siguiente; sin admin: 403 |
| 53-62 | `respond` fija JSON/nosniff/no-store y status; `fail` añade código e ID | Ignora error al codificar; ID puede venir de wrapper main |

El timeout de verificación no se convierte en contexto de negocio: `WithValue` parte de `r.Context()`. El wrapper de main ya aporta un timeout de 10 s ([despliegue](07-deployment.md)). El tipo esperado del subject se afirma mediante type assertion sin comprobación en el POST; es seguro por la conexión local `New -> Authorize` cuando se cumple el contrato del verifier, no por aceptar una cabecera de identidad.

## API: guardas y errores HTTP

| Unidad / líneas de `services/jobs/internal/handler/api.go` | Condición / operación | Resultado |
| --- | --- | --- |
| POST, 25-30 | `mime.ParseMediaType` acepta `application/json`, incluso parámetros válidos | Tipo distinto o error: 415 `json_required` |
| POST, 31-35 | Parsea Idempotency-Key UUID y rechaza Nil | 400 `invalid_idempotency_key` |
| POST, 36-44 | Struct con `zone_id`; body limitado a 8192; campos desconocidos prohibidos; segunda lectura debe ser EOF; zona fija | Error de JSON, múltiples documentos o zona distinta: 400 `invalid_demo_request` |
| POST, 45-50 | Parsea request ID o genera otro; subject desde contexto; pasa actor, key normalizada, zona y trace | ID en respuesta antes de almacenamiento |
| POST, 51-58 | `Create` retorna trabajo y error de commit incluido | Error: log sin datos del body y 503; éxito: Location y 202, incluso trabajo previamente publicado |
| GET job, 59-75 | Parsea ID; llama `Get` | 400 si formato inválido; `pgx.ErrNoRows` -> 404; otro error -> 503; éxito 200 |
| GET audit, 76-83 | Llama `Audit` sin parámetros de usuario | Error 503; éxito 200, lista hasta 100 |
| Construcción, 23-24,84-85 | Mux por método y ruta envuelto con `Authorize` | Métodos no registrados quedan al mux, no a errores JSON uniformes |

La guarda GET no rechaza explícitamente UUID Nil como el POST. No hay filtro por propietario en `Get` ni `Audit`: el permiso es `admin`, no “solo mis trabajos”. El POST restringe zona; no compara payload de una repetición con el almacenado porque la forma permitida es única.

## Repositorio: aceptación atómica

`Create(ctx, actor, key, zone, trace)` abre transacción, instala rollback diferido y genera ID candidato (`services/jobs/internal/repository/store.go:31-38`). El rollback es limpieza de salida fallida; tras commit ya no revierte la transacción comprometida según el contrato esperado de pgx, no inspeccionado aquí.

| Rama / líneas de `services/jobs/internal/repository/store.go` | Estado previo / operación | Resultado / error |
| --- | --- | --- |
| 32-36 | `Begin` | Error devuelve `Job{}`; si abre, rollback diferido |
| 39-48 | INSERT queued con conflicto `(actor_id,idempotency_key)` ignorado; conflicto da `ErrNoRows` en RETURNING | Busca trabajo existente del mismo par y hace commit; no añade outbox ni audit de repetición |
| 49-56 | Inserción nueva; construye `Event` con ID nuevo, tipo pedido, fechas del DB, trace/actor/zona, `Demo=true`; marshal | Error SQL o JSON aborta antes de commit |
| 57-63 | Inserta outbox y audit `job.requested` en esa misma tx | Cualquier error revierte; devuelve trabajo junto al error de commit |
| 66-70 | `Get`: SELECT por ID y scan | Error conservado para que handler distinga no encontrado |
| 72-87 | `Audit`: SELECT orden ID descendente LIMIT 100; slice vacía no nil; loop Scan y append | Error inicial/scan/final rows.Err se propaga; cierre diferido |

El código no envía AMQP durante el POST. La fila outbox se publica posteriormente; con broker caído puede existir un trabajo aceptado durable y todavía `queued`. Un error de commit puede dejar al cliente sin certeza del resultado; la repetición con la misma clave sirve para recuperar esa incertidumbre, no para prometer que todo 503 equivale a “ningún cambio ocurrió”.

## Repositorio: resultado y deduplicación

`Complete` es el callback configurado por main, no un endpoint HTTP. El broker valida el envelope antes de llamarlo; el método no repite esa validación (`services/jobs/cmd/server/main.go:91-94`; [mensajería](04-messaging.md)).

| Rama / líneas de `services/jobs/internal/repository/store.go` | Operación / condición | Postcondición |
| --- | --- | --- |
| 89-98 | Begin + rollback; INSERT inbox con ID evento, conflicto ignorado | Fallos se devuelven al broker |
| 99-101 | Inbox ya contiene ID | Commit sin modificar trabajo ni audit; dedupe por evento |
| 102-110 | UPDATE solo si ID, actor y zona coinciden y estado queued | Si una fila cambia, asigna batch y añade `batch.published`; error audit revierte estado e inbox |
| 111-120 | UPDATE no cambió fila | Consulta existencia de trabajo ya published con mismo batch/actor/zona; si coincide acepta resultado repetido con otro ID; si no, error `result does not match a known job` |
| 121-122 | Commit | Error vuelve al broker; solo éxito permite Ack |

No se consulta la base de `risk` para verificar lote: la identidad del resultado se comprueba contra el trabajo local y el envelope. No se actualiza a estado `failed` al rechazar resultados; un mensaje en DLQ puede dejar trabajo queued.

## Traza razonada de idempotencia y finalización

Supuestos: actor `A` autenticado admin, UUIDs válidos `K`/`T`, DB y commits exitosos. Letras representan UUIDs ilustrativos, no valores enviados literalmente.

| Paso | Cita | Estado antes -> después |
| --- | --- | --- |
| POST inicial | `services/jobs/internal/handler/api.go:25-58` | JSON demo + K -> llamada Create(A,K,zona,T) |
| Transacción | `services/jobs/internal/repository/store.go:39-63` | Sin par A/K -> job J queued, outbox E solicitado, audit solicitado; commit -> 202 |
| Repetición | `services/jobs/internal/repository/store.go:39-48` | Mismo A/K -> J existente; sin nueva audit/outbox |
| Resultado de risk | `services/jobs/internal/repository/store.go:95-110` | E2 nuevo, J queued -> inbox E2, J published con B, audit publicado |
| Redelivery | `services/jobs/internal/repository/store.go:99-101` | E2 ya en inbox -> commit sin repetir efectos |

**Alternativa de error:** si el trace del resultado no cumple el tipo UUID SQL, falla el INSERT de audit después del UPDATE. La función retorna error y rollback revierte UPDATE e inbox (`services/jobs/internal/repository/store.go:94,107-110`). El broker reintenta y finalmente puede rechazar a DLQ, no devolver un HTTP 503 a la petición original. El test de integración expresa este escenario (`services/jobs/internal/repository/store_integration_test.go:39-55`), pero no se ejecutó.

## Implementado, pretendido y cobertura

**Implementado:** idempotencia por actor/clave; consultas globales de admin; transacciones de aceptación y finalización; dedupe por ID evento y comprobación de resultado repetido. **Intención:** OpenAPI describe 202 durable e idempotencia (`contracts/openapi/firewatch-v1.yaml:11-41`) y tests rechazan entradas antes de persistencia (`services/jobs/internal/handler/api_test.go:48-61,103-111`). No son resultados observados.

| Archivo / unidades | Inventario explicado | Estado |
| --- | --- | --- |
| `api.go:1-85` | Imports/interfaz; constructor; tres handlers; guards; respuesta; wrapper | Explicado |
| `security.go:1-62` | Imports/interfaz/clave; claims; timeout; loop roles; respond/fail | Explicado |
| `store.go:1-122` | Imports/structs; Create/conflicto/evento; Get; Audit/loop; Complete/dedup/match | Explicado |

Límites: semántica de pgx y aislamiento/concurrencia real no ejecutados; callbacks dependen de broker y esquema. Verificación posterior propuesta: conflictos concurrentes, commit incierto y resultado con actor/batch incorrectos.
