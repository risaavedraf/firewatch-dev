# Risk: publicar una muestra fija sin inferir riesgo

`risk` consume solicitudes de job, crea un lote con un punto fijo y expone latest/history. El servicio clasifica el lote con la regla propia no oficial `academic-rule-v1`: `risk_level` es uno de `bajo`, `medio`, `alto`, `desconocido`, y `desconocido` es el resultado honesto cuando falta un valor o la observación está vencida, nunca riesgo bajo. La entrada sigue siendo sintética y no hay ingestión real.

[Índice e instantánea](README.md) · [Jobs](02-jobs.md) · [Siguiente: mensajería](04-messaging.md)

## Alcance y conexiones

Archivos completos: [api.go](../../services/risk/internal/handler/api.go):1-46; [security.go](../../services/risk/internal/handler/security.go):1-61; [store.go](../../services/risk/internal/repository/store.go):1-163; [demo.go](../../services/risk/internal/service/demo.go):1-43. Snapshot y dependencias: [índice](README.md). Arranque, SQL y broker se explican en sus partes respectivas. No se ejecutó código.

| Unidad / citas | Entrada | Salida / propiedad |
| --- | --- | --- |
| `Repository` — `services/risk/internal/handler/api.go:10-12` | Contexto, zona, límite | Slice de `Batch` o error |
| `Verifier`, `Authorize` — `services/risk/internal/handler/security.go:13-50` | Bearer | Acceso viewer/admin, sin almacenar subject en contexto |
| `Store`, `Batch` — `services/risk/internal/repository/store.go:15-36` | Pool y filas de lotes | IDs/fechas/demo/modelo, GeoJSON crudo y estados calculados |
| `ValidateRequest` — `services/risk/internal/service/demo.go:11-43` | Envelope evento y zona | Guard de forma; la pertenencia de la zona la decide la clave foránea; no modelo ni I/O |
| `Publish`, `List` — `services/risk/internal/repository/store.go:63-163` | Solicitud o consulta | Escritura atómica / lectura ordenada |

Los imports de los cuatro archivos (api:1-8; security:1-11; store:1-11; demo:1-9) separan HTTP/OIDC, repositorio pgx y contrato de evento. Main conecta `Handle: store.Publish` (`services/risk/cmd/server/main.go:91-94`); el consumidor llama primero `service.ValidateRequest` (`services/risk/internal/messaging/broker.go:87-98`). La función SQL no llama al validador: es una precondición de la ruta del broker, no garantía de invocaciones directas.

## Seguridad y consulta HTTP

`Authorize` exige dos partes Bearer, verifica con timeout 5 s y rechaza error/subject vacío; lee `typ`, `nbf`, roles y exige tipo Bearer y nbf no futuro. Acepta si cualquier rol es viewer o admin, de lo contrario 403 (`services/risk/internal/handler/security.go:17-50`). Las ramas son equivalentes a jobs excepto permisos y ausencia de `subjectKey`. `respond` fija JSON, nosniff/no-store y status e ignora error de encode; `fail` incluye request ID (`services/risk/internal/handler/security.go:52-61`). La dependencia OIDC no se inspeccionó.

| Rama / líneas de `services/risk/internal/handler/api.go` | Decisión | Resultado |
| --- | --- | --- |
| 14-18 | `get(latest)` produce closure; lee query | Dos rutas comparten la misma lógica |
| 19-22 | Zona debe ser demo; exactamente una clave y un valor | Zona vacía/distinta, extra o duplicada: 400 `invalid_zone` |
| 23-27 | History usa 100; latest usa 1 | Consulta repositorio con límite interno, no configurable por cliente |
| 28-31 | Error de `List` | 503 `storage_unavailable` |
| 32-38 | Latest vacío / no vacío | 404 con `no_published_batch` y `data_status=insufficient`; si hay lote: 200 primer elemento |
| 40-46 | History y registro de rutas | 200 slice (vacía permitida); solo GET; `Authorize` envuelve mux |

`insufficient` es respuesta de ausencia, no campo de un lote almacenado. No hay paginación ni eliminación de lotes vencidos. Las rutas no filtran por actor.

## Validación del evento

| Rama / líneas de `services/risk/internal/service/demo.go` | Entrada rechazada / permitida | Propagación |
| --- | --- | --- |
| 11-31 | Comentario: la zona se valida solo por forma y la pertenencia la decide la clave foránea hacia `zones(code)` | Una zona bien formada fuera del catálogo no se rechaza aquí; la clave foránea la rechaza al persistir |
| 33-35 | Forma: tipo `JobRequestedType`, `ZoneID` no vacío, actor no vacío y <=255 bytes, ambas fechas no cero, solicitud no posterior a now+1 minuto | Error `invalid event`; broker rechaza sin ejecutar callback |
| 36-41 | Itera ID, JobID, TraceID; parse UUID y no Nil | Error `invalid event identifier` |
| 42 | Todas guardas superadas | nil; no mutación |

El límite `len(ActorID)` mide bytes de string Go, no caracteres Unicode. No se exige BatchID vacío ni se compara OccurredAt con RequestedAt. La tolerancia de un minuto se aplica a la solicitud, no al timestamp de ocurrencia. La regla académica ya es un clasificador implementado y declarado como regla propia no oficial; sustituirla por un modelo entrenado sigue exigiendo datos y aprobación.

## Publicación transaccional

| Rama / líneas de `services/risk/internal/repository/store.go` | Operación | Salida / efectos |
| --- | --- | --- |
| 64-72 | Begin, rollback diferido, inbox por request.ID | Error inicial/SQL se propaga |
| 73-75 | Mismo ID evento ya registrado | Commit sin otro lote ni evento |
| 99-107 | ID lote nuevo; INSERT con `job_id` único; punto (-71.6127,-33.0472), SRID 4326; `model_version` del clasificador; vigencia RequestedAt+15 min | Error revierte inbox; conflicto job ignorado |
| 105-107 | Ya existe lote para JobID, aunque evento nuevo | Commit de inbox sin crear outbox adicional |
| 108-116 | Copia envelope; reemplaza ID, tipo, BatchID y OccurredAt=now UTC; marshal | Conserva actor/trace/RequestedAt/zona/demo; error JSON aborta |
| 117-121 | INSERT outbox y commit | Lote e intención de evento visibles juntos; commit fallido devuelto al broker |

`published_at` proviene del default SQL, no de `event.OccurredAt`; pueden diferir. La vigencia empieza en el momento de solicitud, no de finalización: un lote tardío puede nacer ya stale. Dos deduplicaciones tienen objetivos distintos: inbox protege redelivery de evento; UNIQUE(job_id) protege una nueva identidad de evento para el mismo trabajo. No se confirma a RabbitMQ hasta retorno exitoso del callback.

## Listado y frescura

`List` ejecuta SELECT parametrizado por zona y límite, ordena `requested_at DESC,id DESC` y obtiene GeoJSON de PostGIS (`services/risk/internal/repository/store.go:124-124`). No ordena por published_at ni filtra fecha de vigencia.

| Bloque / líneas de `services/risk/internal/repository/store.go` | Transformación / decisión | Resultado |
| --- | --- | --- |
| 129-137 | Slice vacía no nil; loop rows.Next; Scan columnas más geometry string | Error scan termina; defer cierra rows |
| 136-151 | Geometry -> RawMessage; estado inicial current; `!now.Before(valid_until)` | En igualdad o después del límite: stale |
| 148-162 | `DataStatus` current/stale y aviso derivado (demo o `LevelDesconocido`); `Factors`/`Policy`; append; `rows.Err()` | JSON geometry como objeto, no string escapado; error iteración se devuelve |

La exactitud del GeoJSON y funciones espaciales depende de PostGIS. El aviso es una cadena original en inglés de la implementación; esta documentación no modifica copy de la API. `DataStatus`, `RiskLevel` y `Notice` se calculan al leer y no son columnas persistidas.

## Traza razonada

Supuestos: evento válido E para J, RequestedAt=12:00 UTC, procesamiento a 12:02, commits exitosos; DB inicialmente sin J. UUIDs simbólicos representan valores válidos.

| Paso | Cita | Valores antes / después |
| --- | --- | --- |
| Validación | `services/risk/internal/service/demo.go:33-43` | Zona con forma válida y UUIDs válidos; fecha no futura -> nil |
| Persistencia | `services/risk/internal/repository/store.go:64-107` | Sin E/J -> inbox E y lote B; valid_until=12:15; punto fijo |
| Envelope | `services/risk/internal/repository/store.go:108-121` | E solicitado -> E2 publicado, BatchID B, OccurredAt cercano a 12:02; commit |
| Lectura 12:10 | `services/risk/internal/repository/store.go:123-162` | now <12:15 -> B current con nivel canónico (bajo/medio/alto/desconocido) |
| Lectura 12:15 | `services/risk/internal/repository/store.go:148-151` | Igualdad con vigencia -> B stale; el nivel canónico no cambia y el lote no desaparece |

**Alternativa:** Un tipo distinto, una zona vacía o un UUID no válido falla en el validador y el broker llama Reject(false), sin abrir transacción (`services/risk/internal/messaging/broker.go:87-94`). Una zona bien formada fuera del catálogo pasa el validador pero la clave foránea la rechaza al persistir. Si la solicitud es válida pero SQL falla, rollback elimina inbox/lote no comprometidos y callback retorna error; broker reintenta. Si latest no encuentra lotes retorna 404 insufficient; history vacía retorna 200, no 404.

## Contrato y cobertura

**Implementado:** fixture fijo, dedupe doble, outbox atómica, orden por solicitud, frescura en lectura. **Intención leída:** tests requieren viewer/admin, ausencia explícita y rechazo de duplicación de query (`services/risk/internal/handler/api_test.go:43-48`); integración exige que un trabajo antiguo terminado después no suplante al nuevo (`services/risk/internal/repository/store_integration_test.go:26-51`). No se ejecutaron.

| Archivo / unidades inventariadas | Estado |
| --- | --- |
| `api.go:1-46`: imports/interfaz, New, fábrica get, guard query, límites, latest vacío, history, wrapper | Explicado |
| `security.go:1-61`: imports/interfaz, headers, timeout/claims, loop roles, respond/fail | Explicado |
| `store.go:1-163`: imports/structs, tx/inbox/job conflict/evento, List/scan/frescura/retorno | Explicado |
| `demo.go:1-43`: imports, comentario de forma, guard de forma y loop UUID | Explicado |

No hay predictor, scheduler, ingestión meteorológica ni modelo entrenado en estas unidades. Revisión propuesta: bordes temporales con reloj controlado, lotes tardíos y validación directa fuera del broker; no realizada.
