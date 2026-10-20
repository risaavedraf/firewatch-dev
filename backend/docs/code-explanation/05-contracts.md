# Contratos: separar el formato compartido del comportamiento
**Re-derivado el 2026-10-20 contra el código de `267a4bb`.**

Los servicios comparten un envelope JSON, un contrato de observaciones y una especificación HTTP, no repositorios ni reglas de negocio. Aunque vive en `contracts/gen`, el tipo Go está **escrito a mano**; no se presupone un generador ni correspondencia automática entre OpenAPI y handlers. El envelope declara hoy seis constantes —tres routing keys— y catorce campos.

[Índice e instantánea](README.md) · [Mensajería](04-messaging.md) · [Siguiente: persistencia](06-database.md)

## Alcance y evidencia

Lecturas completas: [events.go](../../contracts/gen/events/v1/events.go):1-58 y contracts/gen/events/v1/events_test.go:1-138; [contrato de eventos](../../contracts/events/v1/README.md):1-26; [OpenAPI](../../contracts/openapi/firewatch-v1.yaml):1-2125; [observaciones](../../contracts/gen/observations/v1/observations.go):1-176. Snapshot: [índice](README.md). El módulo de tres líneas (`contracts/gen/go.mod`) se explica en [despliegue](07-deployment.md). No se ejecutó validación OpenAPI, generación ni petición a un servidor: **nada de lo que sigue está verificado contra un proceso en ejecución**. Las referencias `$ref` son contratos declarativos, no llamadas Go.

## Envelope y direcciones

| Declaración / líneas de `contracts/gen/events/v1/events.go` | Valor o interfaz | Uso comprobado |
| --- | --- | --- |
| 1-5 | Comentario handwritten; paquete v1; import time | No lógica de dominio compartida |
| 7-21 | Exchange `firewatch.events`; tipos `jobs.requested.v1`, `risk.batch.published.v1`, `risk.job.failed.v1`; colas `risk.jobs.v1` / `jobs.results.v1` | Bindings en ambos `topology`; `jobs.results.v1` recibe dos claves (`services/jobs/internal/messaging/broker.go:516-517`); los relays toman routing_key del outbox |
| 23-33 | ID, Type, OccurredAt, TraceID, JobID, ActorID, ZoneID, BatchID omitempty, RequestedAt, Demo | Correlación, validación y match contra trabajo |
| 34-37 | `scenario_id` omitempty y `evaluated_at *time.Time` omitempty | Par opcional pero acoplado: el consumidor rechaza medio par (`services/jobs/internal/messaging/broker.go:289-299`) |
| 38-50 | `failure_reason` y `attempts` omitempty, presentes sólo en `risk.job.failed.v1` | Reporte terminal entre servicios; los vocabularios se duplican en cada adaptador local, no hay SDK |
| 53-58 | `SyntheticScenarioIDs()`: seis perfiles, devuelve copia | Vocabulario de escenarios; el transporte de jobs compara igualdad exacta (`services/jobs/internal/handler/api.go:71-78`) |

Los strings Go no imponen UUID ni tamaño por sí solos; las restricciones vienen de handlers/consumidores/SQL. `omitempty` sólo afecta a la serialización: `batch_id`, `scenario_id`, `evaluated_at`, `failure_reason` y `attempts` pueden faltar sin que la deserialización los exija. `Demo` no es puerta de validez —describe el origen de la solicitud—, mientras que fechas cero y UUID nulos sí hacen fallar `valid` (`services/jobs/internal/messaging/broker.go:285-307`).

`contracts/events/v1/README.md:3-9` documenta transporte y una tabla de dos routing keys: **no incluye `risk.job.failed.v1`**, que existe sólo en el código. Líneas 11-13 especifican UUIDs, fechas UTC, deduplicación transaccional, Ack después de commit y versionado de cambios incompatibles; línea 15 prohíbe datasets/modelos en bodies. Líneas 17-24 añaden la metadata de escenario opcional y la línea 26 limita la extensión a servicios locales actualizados juntos. Es **intención**, no enforcement. La producción de solicitudes usa fecha DB y `Demo=true` (`services/jobs/internal/repository/store.go:62-77`); risk copia y transforma el envelope (`services/risk/internal/repository/store.go:269-281`).

## OpenAPI: mapa completo de declaraciones

| Bloque / líneas de `contracts/openapi/firewatch-v1.yaml` | Responsabilidad / entradas | Resultado declarado |
| --- | --- | --- |
| 1-9 | OpenAPI 3.0.3, versión 0.1.0, servidor localhost, seguridad global bearer | Descripción declara regla propia no oficial sobre entrada sintética |
| 11-46 | `GET /v1/risk/status` y `GET /v1/jobs/status`; 200/503 más 401/403/429/502 | Estado de dependencias acotado, no prueba de entrega; el probe `/health/ready` decide sólo por base |
| 47-100 | `POST /v1/jobs` admin; Idempotency-Key UUID; body `additionalProperties: false` con `zone_id` del catálogo y `scenario_id` opcional (70-80) | 202 Job; 400/401/403/415/429/502/503 |
| 101-237 | `GET /v1/jobs/{id}` y `POST /v1/jobs/{id}/replay`; estados queued/published/failed con `failure_reason` y `attempts`; replay confirmado con `queued_may_be_in_flight` | 200/202 Job; 400/404/409 `replay_confirmation_required` o `job_not_replayable` |
| 238-317 | Lecturas de batches: latest (404 `no_published_batch` en 254-269), history (hasta 100) y batches (último publicado por zona, orden `zone_id`) | Batch y BatchCollection; 400/401/403/404; la colección vacía responde 200 |
| 318-444 | synthetic-scenarios y sources (geografía, `risk_input` y `sources`) | Catálogo de perfiles y procedencia geográfica; sin base, broker ni fetch de fuente |
| 445-507 | `POST /v1/risk/cycle/run` admin y `GET /v1/models` | Ciclo sincrónico por zona con 503 `cycle_unavailable`; registro de modelos con la fila activa primero |
| 508-563 | `POST /v1/models/{version}/activate` admin | 200 ModelVersion; 404 `model_not_found`; 409 `model_not_activatable` |
| 564-595 | `GET /v1/risk/zones` GeoJSON con `bbox` opcional de cuatro números | FeatureCollection, vacía válida; 400 por bbox malformado o repetido |
| 596-624 | `GET /v1/audit` admin, últimas 100 | actor/action/job/trace/fecha; 401/403 |
| 625-1256 | Seis rutas de prevention (`actions`, `{id}`, `transitions`, `evidence`, `reviews`, `reassessments`) con siete operaciones | 200/201/400/404/409/415/422 con códigos propios por ruta |
| 1258-1279 | `bearerAuth` HTTP JWT; parámetro `Zone` (query `zone_id`, requerido, sin enum); header `X-Request-ID` | Los refs centralizan descripciones; el conjunto válido de zonas lo sirve el catálogo |
| 1280-1367 | Nueve responses reutilizables: ready/unavailable (200/503) y 400/401/403/404/429/502/503 | `RateLimited` trae `Retry-After` y aplica a toda operación; 502 y 503 declaran efecto nulo |
| 1368-1571 | Schemas operativos y de dominio: `CycleRunResult`, `ModelVersion`, `ErrorResponse` (1436-1455), `OperationalStatus`, `DeadLetterStatus`, `GeographyProvenance` | Estado acotado y procedencia; `ErrorResponse` exige `code` y `request_id` |
| 1572-1769 | `SyntheticScenarioID` (seis valores), `Job` (1585-1637), `BatchCore` (1638-1730), `Batch` con geometría, `ZoneBatch` y `BatchCollection` | `BatchCore` impide que la lectura por zona y la agregada diverjan; `Batch` añade `geometry` |
| 1770-1813 | `Geometry` RFC 7946 (Point/Polygon/MultiPolygon), `ZoneProperties`, `ZoneFeature`, `ZoneFeatureCollection` | No limita rangos lon/lat ni impone SRID en OpenAPI |
| 1814-2125 | Vocabulario congelado de prevention: ocho estados con sus transiciones, acción e historial, evidencia y revisiones, reassessment con `causal_proof` fijo en false | Un reassessment sintético se declara no causal; `desconocido` es un valor propio del enum de `risk_level`, no un sinónimo de `bajo` |

Las branches HTTP reales están en [jobs](02-jobs.md) y [risk](03-risk.md). Aquí los “casos” son alternativas de respuesta declarada, no código ejecutable. Los refs internos apuntan a Job, Batch, Zone, responses y schemas definidos en el mismo archivo; no requieren un schema remoto en el texto leído.

## Diferencias y fronteras observadas

| Código implementado | Contrato declarado / consecuencia |
| --- | --- |
| Gateway responde 502 `upstream_unavailable` al fallar el upstream (`services/gateway/internal/handler/gateway.go:65`) | Hoy figura como `UpstreamUnavailable` (1347-1357) en las 22 operaciones: ese hueco de inventario se cerró |
| GET job, replay, audit y las lecturas de risk devuelven 503 `storage_unavailable` (`services/jobs/internal/handler/api.go:181-184,221-225,230-233`; `services/risk/internal/handler/api.go:68-71,81-84`) | `StorageUnavailable` (1358-1367) se declara en toda operación que toca almacenamiento; `/v1/risk/synthetic-scenarios` (318-338) no lo declara porque no lee base ni broker |
| Tres capas de cuota, la agregada cobrada antes de rutas (`services/gateway/internal/handler/gateway.go:88-95,167-172,189-194`) | `RateLimited` (1329-1346) aplica a toda operación y ya no es exclusivo del POST |
| POST jobs rechaza campos desconocidos y un segundo documento JSON (`services/jobs/internal/handler/api.go:112-117`) | `additionalProperties: false` y `required: [zone_id]` (70-71) expresan parte del chequeo; el segundo documento y el `scenario_id: null` explícito no se describen |
| Query de risk exige exactamente un `zone_id` (`services/risk/internal/handler/api.go:61-66`) | El parámetro `Zone` (1260-1271) fija obligatoriedad y tipo; el rechazo de claves repetidas queda en la prosa de cada operación |
| Latest sin batch devuelve 404 con `code` y `data_status` (`services/risk/internal/handler/api.go:86-90`) | Declarado en 254-269, incluido que ese body no lleva `request_id` |
| Un método o ruta no registrados los responde el `ServeMux`, sin body JSON (`services/jobs/internal/handler/api.go:86-87`) | `ErrorResponse` (1436-1455) exige `code` y `request_id`, pero ninguna operación documenta el 405 ni la forma de esos cuerpos |

Los roles se describen en prosa, no como scopes del esquema bearer; un cliente no debe interpretar ausencia de scopes como acceso universal. No se lee el realm excluido ni se verifica que la configuración de identidad emita esos claims.

## Traza razonada de transformación

Supuestos: aceptación transaccional con IDs válidos y fixture demo.

| Paso | Cita | Transformación |
| --- | --- | --- |
| HTTP | `contracts/openapi/firewatch-v1.yaml:47-100`; `services/jobs/internal/handler/api.go:87-168` | zone_id del catálogo, Idempotency-Key UUID y scenario_id opcional -> `Create` idempotente por (actor, key), sujeto desde JWT y traza desde X-Request-ID |
| Request event | `services/jobs/internal/repository/store.go:62-77,110-121` | Construye E con tipo solicitado, JobID J, `Demo: true` y sin metadata ausente -> JSON sin `batch_id`, `scenario_id` ni `evaluated_at` |
| Publication event | `services/risk/internal/repository/store.go:269-281` | Copia E; nuevo ID E2, tipo publicado, BatchID B, OccurredAt now -> JSON de resultado con la metadata de escenario intacta |
| Completion | `services/jobs/internal/messaging/broker.go:229-233,285-307`; `services/jobs/internal/repository/store.go:172-198` | `valid` exige cuatro UUID y par de escenario coherente; `Complete` compara actor y zona contra la fila y escribe inbox/audit -> J published |

**Alternativa:** un envelope con Type publicado pero BatchID vacío pasa el decode Go, pero `valid` devuelve false y el consumidor lo rechaza sin publicar (`services/jobs/internal/messaging/broker.go:229-233,300-305`); un `risk.job.failed.v1` mal formado sigue el mismo camino (`services/jobs/internal/messaging/broker.go:262-273`). Un schema Go compartido no sustituye validación semántica. Cambiar una routing key sin migrar bindings produce `message unroutable` en el relay —publica con mandatory y escucha los returns (`services/jobs/internal/messaging/broker.go:584-600`)—, no un error de compilación.

## Cobertura y límites

| Archivo / unidades | Estado / hueco |
| --- | --- |
| `events.go:1-58`: comment/paquete/import, seis constantes, catorce campos, `SyntheticScenarioIDs` | Explicado; sin branches ejecutables propias |
| `contracts/events/v1/README.md:1-26`: transporte, tabla, envelope, garantías, metadata de escenario, límite de versiones | Explicado como intención; su tabla de routing keys omite `risk.job.failed.v1` |
| `firewatch-v1.yaml:1-2125`: metadata, 21 rutas y 22 operaciones, componentes, refs y propiedades | Explicado por bloques declarativos; no se validó parser ni generación |
| contracts/gen/observations/v1/observations.go:122-142 frente a `risk_input` de `contracts/openapi/firewatch-v1.yaml`:392-423 | Parcial: `SourceDescriptor` y el schema coinciden en catorce campos, pero el YAML no fija los enums `kind`/`origin` ni valida el resto del paquete |

No se afirma que contrato y código sean idénticos: este documento declara lo que el texto dice, no lo que un servidor hace, y la instantánea de la colección no se regeneró en esta re-derivación. Próxima revisión propuesta: validar el YAML con un parser autorizado, comprobar que el cliente cubre los estados y códigos nuevos (`failed`, 409/422 de prevention, `queued_may_be_in_flight`) y añadir `risk.job.failed.v1` a la tabla de `contracts/events/v1/README.md`.
