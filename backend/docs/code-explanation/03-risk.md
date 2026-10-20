# Risk: publicar una muestra fija sin inferir riesgo

**Re-derived on 2026-10-20 against the code of commit `267a4bb`.**

`risk` consume solicitudes de job, crea un lote con un punto fijo y expone latest/history y lecturas agregadas. El clasificador por defecto es la regla propia no oficial `academic-rule-v1`; una segunda regla, `firewatch-rule-v2`, se registra junto a ella y una activación del registro de modelos puede servirla. `risk_level` es uno de `bajo`, `medio`, `alto`, `desconocido`, y `desconocido` es el resultado honesto cuando falta un valor o la observación está vencida, nunca riesgo bajo. La entrada por defecto sigue siendo sintética: el proveedor HTTP/JSON existe (`FIREWATCH_SOURCE_URL`), pero no hay fuente de observaciones reales conectada.

[Índice e instantánea](README.md) · [Jobs](02-jobs.md) · [Siguiente: mensajería](04-messaging.md)

## Alcance y conexiones

Archivos completos: [api.go](../../services/risk/internal/handler/api.go):1-269; [security.go](../../services/risk/internal/handler/security.go):1-93; [store.go](../../services/risk/internal/repository/store.go):1-530; [demo.go](../../services/risk/internal/service/demo.go):1-43. Snapshot y dependencias: [índice](README.md). Arranque, SQL y broker se explican en sus partes respectivas. No se ejecutó código.

| Unidad / citas | Entrada | Salida / propiedad |
| --- | --- | --- |
| `Repository` — `services/risk/internal/handler/api.go:18-36` | Contexto, zona y límite; también lectura agregada, catálogo y registro de modelos | Slice de `Batch`, `Zone` o `ModelVersion`, o error |
| `Verifier`, `Authorize` — `services/risk/internal/handler/security.go:13-15,33-66` | Bearer | Acceso viewer/admin; guarda los roles verificados en el contexto, nunca el subject |
| `Store`, `Batch` — `services/risk/internal/repository/store.go:18-71` | Pool y filas de lotes | IDs/fechas/demo/modelo, GeoJSON crudo, policy/scenario y estados calculados |
| `ValidateRequest` — `services/risk/internal/service/demo.go:11-43` | Envelope evento y zona | Guard de forma; la pertenencia de la zona la decide la clave foránea; no modelo ni I/O |
| `Publish`, `PublishScheduled`, `List` — `services/risk/internal/repository/store.go:150-171,177-282,417-442` | Solicitud o consulta | Escritura atómica con o sin evento de finalización / lectura ordenada |

Los imports de los cuatro archivos (api:3-17; security:3-11; store:3-16; demo:3-9) separan HTTP/OIDC, repositorio pgx y contrato de evento. Main construye el `service.Publisher` y conecta el broker con `Handle: publisher.Publish` (`services/risk/cmd/server/main.go:196-209`); el consumidor llama primero `service.ValidateRequest` (`services/risk/internal/messaging/broker.go:180-186`). La función SQL no llama al validador: es una precondición de la ruta del broker, no garantía de invocaciones directas.

## Seguridad y consulta HTTP

`Authorize` exige dos partes Bearer, verifica con timeout 5 s y rechaza error/subject vacío; lee `typ`, `nbf`, roles y exige tipo Bearer y nbf no futuro. Acepta si cualquier rol es viewer o admin, de lo contrario 403, y guarda los roles verificados en el contexto (`services/risk/internal/handler/security.go:33-66`); `requireAdmin` reutiliza esos roles sin reverificar el token y exige admin (`services/risk/internal/handler/security.go:72-82`). Las ramas son equivalentes a jobs excepto que aquí no se guarda el `subjectKey` y sí los roles que consume `requireAdmin`. `respond` fija JSON, nosniff/no-store y status e ignora error de encode; `fail` incluye request ID (`services/risk/internal/handler/security.go:84-93`). La dependencia OIDC no se inspeccionó.

| Rama / líneas de `services/risk/internal/handler/api.go` | Decisión | Resultado |
| --- | --- | --- |
| 59-62 | `get(latest)` produce closure; lee query | Dos rutas comparten la misma lógica |
| 64-76 | Zona debe existir en el catálogo; exactamente una clave y un valor | Zona vacía, extra, duplicada o ausente del catálogo: 400 `invalid_zone`; fallo del catálogo: 503 `storage_unavailable` |
| 77-81 | History usa 100; latest usa 1 | Consulta repositorio con límite interno, no configurable por cliente |
| 82-85 | Error de `List` | 503 `storage_unavailable` |
| 86-93 | Latest vacío / no vacío | 404 con `no_published_batch` y `data_status=insufficient`; si hay lote: 200 primer elemento |
| 94-112 | History, rutas agregada/catálogo/modelos/ciclo y registro de rutas | 200 slice (vacía permitida); GET salvo activación y ciclo (POST admin); `Authorize` envuelve mux |

`insufficient` es respuesta de ausencia, no campo de un lote almacenado. No hay paginación ni eliminación de lotes vencidos. Las rutas no filtran por actor.

## Validación del evento

| Rama / líneas de `services/risk/internal/service/demo.go` | Entrada rechazada / permitida | Propagación |
| --- | --- | --- |
| 11-30 | Comentario: la zona se valida solo por forma y la pertenencia la decide la clave foránea hacia `zones(code)` | Una zona bien formada fuera del catálogo no se rechaza aquí; la clave foránea la rechaza al persistir |
| 32-35 | Forma: tipo `JobRequestedType`, `ZoneID` no vacío, actor no vacío y <=255 bytes, ambas fechas no cero, solicitud no posterior a now+1 minuto | Error `invalid event`; broker rechaza sin ejecutar callback |
| 36-41 | Itera ID, JobID, TraceID; parse UUID y no Nil | Error `invalid event identifier` |
| 42-43 | Todas guardas superadas | nil; no mutación |

El límite `len(ActorID)` mide bytes de string Go, no caracteres Unicode. No se exige BatchID vacío ni se compara OccurredAt con RequestedAt. La tolerancia de un minuto se aplica a la solicitud, no al timestamp de ocurrencia. La regla académica ya es un clasificador implementado y declarado como regla propia no oficial; sustituirla por un modelo entrenado sigue exigiendo datos y aprobación.

## Publicación transaccional

| Rama / líneas de `services/risk/internal/repository/store.go` | Operación | Salida / efectos |
| --- | --- | --- |
| 178-190 | Begin, rollback diferido, inbox por request.ID | Error inicial/SQL se propaga |
| 187-189 | Mismo ID evento ya registrado | Commit sin otro lote ni evento |
| 224-232 | ID lote nuevo; INSERT con `job_id` único; punto (-71.6127,-33.0472), SRID 4326; `model_version` del clasificador; vigencia RequestedAt+15 min | Error revierte inbox; conflicto job ignorado; zona o modelo ausente se clasifica como fallo permanente |
| 233-262 | Ya existe lote para JobID, aunque evento nuevo | Commit de inbox sin otro lote; con `settleJob` emite la finalización que nombra el lote almacenado |
| 269-277 | Copia envelope; reemplaza ID, tipo, BatchID y OccurredAt=now UTC; marshal | Conserva actor/trace/RequestedAt/zona/demo; error JSON aborta |
| 278-281 | INSERT outbox y commit | Lote e intención de evento visibles juntos; commit fallido devuelto al broker |

`published_at` proviene del default SQL, no de `event.OccurredAt`; pueden diferir. La vigencia empieza en el momento de solicitud, no de finalización: un lote tardío puede nacer ya stale. Dos deduplicaciones tienen objetivos distintos: inbox protege redelivery de evento; UNIQUE(job_id) protege una nueva identidad de evento para el mismo trabajo, que reemite la finalización en vez de perderse. El ciclo automático usa la misma transacción con `persistOnly`: guarda el lote y confirma sin evento de finalización (`services/risk/internal/repository/store.go:263-268`). No se confirma a RabbitMQ hasta retorno exitoso del callback.

## Listado y frescura

`List` ejecuta SELECT parametrizado por zona y límite, ordena `requested_at DESC,id DESC`, resuelve el GeoJSON con PostGIS y exige que el catálogo tenga geometría (`services/risk/internal/repository/store.go:418`). No ordena por published_at ni filtra fecha de vigencia.

| Bloque / líneas de `services/risk/internal/repository/store.go` | Transformación / decisión | Resultado |
| --- | --- | --- |
| 422-431 | Slice vacía no nil; loop rows.Next; Scan columnas más geometry string | Error scan termina; defer cierra rows |
| 432-441 | Geometry -> RawMessage; `readAssessment` compartido; append; `rows.Err()` | Geometría de catálogo ausente se devuelve; geometry como objeto, no string escapado; error iteración se devuelve |
| 494-528 | `readAssessment`: instante UTC, `Factors`, `Policy`; estado inicial current; `!now.Before(valid_until)`; aviso derivado (demo o `LevelDesconocido`) | Igualdad o posterior al límite: stale; estado y aviso se calculan al leer, nunca se persisten |

La exactitud del GeoJSON y funciones espaciales depende de PostGIS. El aviso es una cadena original en inglés de la implementación; esta documentación no modifica copy de la API. `DataStatus`, `RiskLevel` y `Notice` se calculan al leer y no son columnas persistidas. La lectura agregada comparte `readAssessment` y omite la geometría (`services/risk/internal/repository/store.go:462-482`).

## Traza razonada

Supuestos: evento válido E para J, RequestedAt=12:00 UTC, procesamiento a 12:02, commits exitosos; DB inicialmente sin J. UUIDs simbólicos representan valores válidos.

| Paso | Cita | Valores antes / después |
| --- | --- | --- |
| Validación | `services/risk/internal/service/demo.go:32-43` | Zona con forma válida y UUIDs válidos; fecha no futura -> nil |
| Persistencia | `services/risk/internal/repository/store.go:224-262` | Sin E/J -> inbox E y lote B; valid_until=12:15; punto fijo |
| Envelope | `services/risk/internal/repository/store.go:269-281` | E solicitado -> E2 publicado, BatchID B, OccurredAt cercano a 12:02; commit |
| Lectura 12:10 | `services/risk/internal/repository/store.go:417-441,517-520` | now <12:15 -> B current con nivel canónico (bajo/medio/alto/desconocido) |
| Lectura 12:15 | `services/risk/internal/repository/store.go:517-520` | Igualdad con vigencia -> B stale; el nivel canónico no cambia y el lote no desaparece |

**Alternativa:** Un tipo distinto, una zona vacía o un UUID no válido falla en el validador y el broker llama Reject(false), sin abrir transacción (`services/risk/internal/messaging/broker.go:180-186`). Una zona bien formada fuera del catálogo pasa el validador pero la clave foránea la rechaza al persistir. Si la solicitud es válida pero SQL falla, rollback elimina inbox/lote no comprometidos y callback retorna error; broker reintenta. Si latest no encuentra lotes retorna 404 insufficient; history vacía retorna 200, no 404.

## Contrato y cobertura

**Implementado:** fixture fijo, dedupe doble, outbox atómica, orden por solicitud, frescura en lectura. **Intención leída:** tests requieren viewer/admin, ausencia explícita y rechazo de duplicación de query (`services/risk/internal/handler/api_test.go:189-201`); integración exige que un trabajo antiguo terminado después no suplante al nuevo (`services/risk/internal/repository/store_integration_test.go:67-77`). No se ejecutaron.

| Archivo / unidades inventariadas | Estado |
| --- | --- |
| `api.go:1-269`: imports/interfaz, constructores, fábrica get, guard de zona, límites, latest/history, rutas agregada/catálogo/modelos/ciclo, wrapper | Explicado |
| `security.go:1-93`: imports/interfaz, roles en contexto, `Authorize`, `requireAdmin`, respond/fail | Explicado |
| `store.go:1-530`: imports/structs, tx/inbox/insert/conflicto/evento, `Publish`/`PublishScheduled`, `List`/`readAssessment` | Explicado |
| `demo.go:1-43`: imports, comentario de forma, guard de forma y loop UUID | Explicado |

No hay predictor propio, ingestión meteorológica ni modelo entrenado en estas unidades: `api.go` solo expone el registro de modelos y el disparo manual del ciclo, y el clasificador es una regla en `internal/classify`. Revisión propuesta: bordes temporales con reloj controlado, lotes tardíos y validación directa fuera del broker; no realizada.
