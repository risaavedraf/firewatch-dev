# Mensajería: confirmación y deduplicación, no exactamente una vez

**Re-derivado el 2026-10-20 contra el código de `267a4bb`.**

Cada servicio tiene su propio `Broker`: reconecta RabbitMQ, consume una cola y publica su outbox en paralelo. Las transacciones protegen efectos locales; el cruce PostgreSQL/RabbitMQ permite duplicados. La entrega se diseña **al menos una vez**, no exactamente una vez.

[Índice e instantánea](README.md) · [Risk](03-risk.md) · [Siguiente: contratos](05-contracts.md)

## Alcance, dependencias y diferencias

Lecturas completas: [broker jobs](../../services/jobs/internal/messaging/broker.go):1-620 y [broker risk](../../services/risk/internal/messaging/broker.go):1-529. Fecha, hashes y límites de la instantánea: [índice](README.md). No se ejecutó broker ni base de datos; semántica interna de `amqp091-go`/pgx y orden de notificaciones son fronteras externas.

| Unidad | Jobs | Risk | Responsabilidad |
| --- | --- | --- | --- |
| Imports / `Broker` | `services/jobs/internal/messaging/broker.go:1-57` | `services/risk/internal/messaging/broker.go:1-52` | Pool/URL; contadores atómicos y snapshot de DLQ; callback `Handle`; jobs añade `Fail`, risk `ReportFailure` e importa `service` |
| `Run` | `services/jobs/internal/messaging/broker.go:102-124` | `services/risk/internal/messaging/broker.go:91-113` | Loop de sesiones, invalidación de salud y backoff |
| `session` | `services/jobs/internal/messaging/broker.go:126-179` | `services/risk/internal/messaging/broker.go:115-168` | Dial y consumo; canal monitor de la DLQ; delega en `runSession` |
| Validador | `services/jobs/internal/messaging/broker.go:285-307` | `services/risk/internal/service/demo.go:32-43` | Jobs valida resultados, BatchID y reportes de fallo; risk solicitudes con fecha futura acotada |
| `topology` | `services/jobs/internal/messaging/broker.go:504-544` | `services/risk/internal/messaging/broker.go:413-453` | `queueTopology` determinista; declara ambas colas, exchanges y DLQs |
| `relay` | `services/jobs/internal/messaging/broker.go:546-559` | `services/risk/internal/messaging/broker.go:455-468` | Consulta outbox continuamente |
| `publishOne` | `services/jobs/internal/messaging/broker.go:561-609` | `services/risk/internal/messaging/broker.go:470-518` | Lock SQL, publicación confirmada y commit |
| `pause` | `services/jobs/internal/messaging/broker.go:611-620` | `services/risk/internal/messaging/broker.go:520-529` | Espera cancelable con timer |

El wiring es explícito: jobs `Handle=store.Complete` y `Fail=store.Fail` (`services/jobs/cmd/server/main.go:130`); risk `Handle=publisher.Publish` y `ReportFailure=store.RecordJobFailure` (`services/risk/cmd/server/main.go:209`). `Ready`, `Published`, `Processed`, `Rejected` son memoria de proceso, se reinician con él y no son un ledger durable de mensajes. Ambos adapters publican además profundidad y edad de la DLQ (`DeadLetterSnapshot`, `Snapshot`, `invalidate`): `firewatch_dead_letter_depth` y `firewatch_dead_letter_depth_age_seconds` (`services/jobs/internal/messaging/broker.go:90-100`; `services/risk/internal/messaging/broker.go:79-89`), y jobs suma `firewatch_queued_job_age_seconds`. `ReadyReason` convierte una lectura desconocida o sobre umbral en razón de no-listo, pero el código HTTP de `/health/ready` depende solo del ping a la base (`services/jobs/cmd/server/main.go:219`; `services/risk/cmd/server/main.go:339`).

## Run: recuperación de sesión

En `services/jobs/internal/messaging/broker.go:102-124` y `services/risk/internal/messaging/broker.go:91-113`, `invalidate` deja Ready falso y la profundidad en UNKNOWN; `delay` comienza en un segundo; mientras ctx no esté cancelado llama `session`, descarta su error concreto (`_ = b.session(ctx)`) e invalida otra vez. Si se canceló retorna; si no, registra reconexión con `retry_seconds` y espera el delay. **Después de esperar**, compara `time.Since(started)>1 minuto`: incluye duración de sesión **más la pausa**, no solo la sesión. Si supera ese umbral reinicia delay a 1 s; si no, lo duplica hasta 30 s. Con sesiones y pausas acumuladas por iteración bajo el umbral, las esperas pueden ser 1, 2, 4, 8, 16, 30 s; la primera es 1 s. No es una medición de estabilidad de sesión aislada.

`pause` crea timer, difiere Stop y selecciona cancelación (false) o vencimiento (true): jobs:611-620 y risk:520-529. No hay jitter. Al no registrar el error concreto de `session`, el log de reconexión no explica por sí solo si falló DB, conexión, topología, publisher, consumidor o la lectura pasiva de la DLQ.

## Topología durable y cuarentena

Los dos adapters declaran lo mismo: cada uno mantiene su copia de `queueTopology`, con contenido idéntico y sin import compartido. Cada llamada fallida retorna inmediatamente; el orden del slice es fijo, así que la declaración es determinista.

| Declaración / citas | Parámetros | Significado local y límite |
| --- | --- | --- |
| Exchange principal — `services/jobs/internal/messaging/broker.go:521-523`; `services/risk/internal/messaging/broker.go:430-432` | `firewatch.events`, topic, durable | Routing por clave v1 |
| Exchange muerto — `services/jobs/internal/messaging/broker.go:524-526`; `services/risk/internal/messaging/broker.go:433-435` | `firewatch.dead`, direct, durable | Separado del flujo normal |
| DLQs — `services/jobs/internal/messaging/broker.go:527-533`; `services/risk/internal/messaging/broker.go:436-442` | `<queue>.dead`, quorum; bind con clave `<queue>` | Rechazos pueden dirigirse a cuarentena |
| Colas normales — `services/jobs/internal/messaging/broker.go:534-536`; `services/risk/internal/messaging/broker.go:443-445` | Quorum; delivery-limit=5; DLX/routing; overflow reject-publish; dead-letter-strategy at-least-once | Configuración solicitada al broker, no validación observada de infraestructura |

Bindings de `queueTopology`, aplicados en el mismo loop (`services/jobs/internal/messaging/broker.go:512-518,537-541`; `services/risk/internal/messaging/broker.go:421-427,446-450`): `risk.jobs.v1 <- jobs.requested.v1`; `jobs.results.v1 <- risk.batch.published.v1` **y** `risk.job.failed.v1` ([contratos](05-contracts.md)). Una cola quorum en un nodo local no demuestra alta disponibilidad. El límite de delivery del broker es distinto de los cinco intentos del callback dentro de una sola entrega.

## Session: abrir, trabajar y cerrar

| Bloque y citas jobs / risk | Condición / operación | Error / estado |
| --- | --- | --- |
| 126-146 / 115-135 | Dial heartbeat 10 s y conexión 5 s; Channel; topology; Qos(1); Consume manual ack de `jobs.results.v1` (jobs) o `risk.jobs.v1` (risk) | Cualquier error retorna; cierres diferidos |
| 147-179, 433-502 / 136-168, 342-411 | Canal monitor con `NotifyClose` de conexión, consumidor y monitor; ticker de 3 s; lectura pasiva de la DLQ; `runSession` con contexto hijo, `errorsCh` buffer 2, gate de apagado, goroutine relay y Ready=true tras la primera lectura | Si la lectura pasiva o el relay fallan, `shutdown` invalida, cancela y cierra la conexión |
| 453-461, 183-189 / 362-370, 172-178 | Select de `ctx.Done`, `failures` (cierre AMQP) o error del relay; select de `ctx.Done` o delivery cerrado | Devuelve error/cancelación; Run reconecta si corresponde |
| 223-242, 190-197 / 179-186 | Body >16384, Unmarshal fallido, tipo no reconocido o evento inválido | Rejected++, Reject(false); no requeue; si Reject falla termina sesión; si no continúa |
| 366-390, 198-201 / 272-299, 187-190 | Hasta cinco llamadas Handle, cada una con timeout 5 s; nil o error permanente corta el loop; el quinto fallo **no** espera | Si el error es transitorio, espera 1, 2, 4 y 8 s entre intentos; cancelación devuelve ctx.Err |
| 342-364, 203-208 / 191-195, 212-232 | Fallaron todos los intentos | Rejected++, log con `attempts` y `permanent`; jobs escribe `jobs.failure_reason` con `Fail`, risk encola `risk.job.failed.v1` con `ReportFailure`; después Reject(false) |
| 209-211, 343-346 / 196-202 | Callback exitoso | Processed++ y log **antes** de Ack(false); Ack fallido termina sesión |

Las citas de la tabla pertenecen respectivamente a `services/jobs/internal/messaging/broker.go` y `services/risk/internal/messaging/broker.go`. `Qos(1)` limita entregas sin ack según la dependencia AMQP, pero no es un límite global del número de workers entre réplicas. El publisher usa canales propios sobre la misma conexión. Un callback nil implica commit según los repositorios conectados; no significa que Ack haya llegado al broker. Por eso Processed puede aumentar y aun así haber redelivery. `Fail` y `ReportFailure` nil son seguros: el rechazo y su dead letter quedan como única evidencia.

### Validación de resultados jobs

`valid` exige tipo publicado, actor no vacío de hasta 255 bytes, zona no vacía, ambas fechas no cero y metadatos sintéticos emparejados (`scenario_id` con `evaluated_at` no cero, o ninguno); luego recorre ID, JobID, TraceID y BatchID y exige UUID no Nil (`services/jobs/internal/messaging/broker.go:285-307`). El consumidor jobs además despacha `risk.job.failed.v1` a `validFailure` y `failHandler`, que exigen motivo no vacío de hasta 255 bytes y `attempts>0` (`services/jobs/internal/messaging/broker.go:262-273,249-254`), y descarta cualquier otro tipo (`services/jobs/internal/messaging/broker.go:228-241`). `Demo` no es una guarda de forma: describe el origen de la solicitud, y la relación entre la zona del lote y la del job se verifica contra la fila almacenada del job en `Store.Complete` (`services/jobs/internal/repository/store.go`), no aquí. Devuelve booleano, sin motivo detallado. A diferencia del validador jobs, risk acota `RequestedAt` a un minuto futuro (`services/risk/internal/service/demo.go:33`). JSON no usa `DisallowUnknownFields` en ninguno de los consumidores; campos adicionales no se rechazan por esa razón. La validación compara el tipo del envelope, no `delivery.Type` ni `delivery.MessageId`.

## Relay y publishOne: la ventana de duplicación

`relay` da 5 s a cada `publishOne`. Error termina el publisher y la sesión; sin fila espera 250 ms; con fila exitosa vuelve inmediatamente. Al cancelar retorna ctx.Err (`services/jobs/internal/messaging/broker.go:546-559`; `services/risk/internal/messaging/broker.go:455-468`).

| Fase / citas jobs y risk | Decisión / efecto | Propagación |
| --- | --- | --- |
| Begin + SELECT — `services/jobs/internal/messaging/broker.go:562-575`; `services/risk/internal/messaging/broker.go:471-484` | Transacción; rollback diferido; primera pendiente por created_at, `FOR UPDATE SKIP LOCKED` | NoRows -> false,nil; otros errores -> false,error; otras réplicas evitan fila bloqueada |
| Canal/confirms — `services/jobs/internal/messaging/broker.go:576-584`; `services/risk/internal/messaging/broker.go:485-493` | Abre canal nuevo por mensaje; Confirm(false); return channel buffer 1 | Error aborta; canal se cierra diferido |
| Publish — `services/jobs/internal/messaging/broker.go:585-591`; `services/risk/internal/messaging/broker.go:494-501` | JSON persistent, mandatory=true; MessageId=id, Type=key, timestamp now UTC; espera deferred confirm con contexto | Error publicación/espera devuelve sin marcar SQL |
| Confirm/return — `services/jobs/internal/messaging/broker.go:593-600`; `services/risk/internal/messaging/broker.go:502-509` | Ack falso -> `publish rejected`; notificación disponible -> `message unroutable`; default si no hay return disponible | No marca published en esas ramas de error |
| SQL/commit — `services/jobs/internal/messaging/broker.go:601-608`; `services/risk/internal/messaging/broker.go:510-517` | UPDATE published_at=now; commit; Published++ solo después | Error SQL/commit puede dejar mensaje ya entregado y fila aún pendiente |

El lock SQL se mantiene mientras se espera a RabbitMQ. La lectura de returns es no bloqueante después de confirm; la garantía de orden temporal entre Return y confirm depende de la biblioteca/broker y no fue examinada. Se describe lo que hace el código, no se certifica esa garantía.

## Traza razonada normal

Supuestos: una outbox jobs E comprometida, infraestructura acepta declaraciones, publish routable confirmado, callbacks/acks exitosos.

| Paso | Cita | Estado anterior -> posterior |
| --- | --- | --- |
| Selección jobs | `services/jobs/internal/messaging/broker.go:562-575` | E pending -> lock y publicación a `jobs.requested.v1` |
| Confirmación | `services/jobs/internal/messaging/broker.go:589-608` | ack true, sin return disponible -> fila published y contador +1 |
| Consumo risk | `services/risk/internal/messaging/broker.go:180-190` | JSON válido -> `publisher.Publish` con 5 s por intento |
| Efectos | `services/risk/internal/repository/store.go:177-281` | Inbox E + lote B + outbox E2 -> commit |
| Ack | `services/risk/internal/messaging/broker.go:196-202` | Callback nil -> Processed++, Ack |
| Retorno jobs | `services/jobs/internal/messaging/broker.go:186-213` | Evento E2 publicado por relay risk -> Complete -> Ack; J published |

**Alternativa de crash:** RabbitMQ confirma E, pero falla el commit SQL de jobs (`services/jobs/internal/messaging/broker.go:601-606`). La sesión retorna error y luego reconecta; outbox puede conservar E pendiente y publicarlo otra vez. Risk reconoce E en inbox y retorna commit sin lote nuevo (`services/risk/internal/repository/store.go:183-188`). Un crash entre commit de consumidor y Ack también puede producir redelivery. Esta ventana justifica dedupe, no una promesa exactly-once.

**Alternativa de agotamiento:** DB del consumidor falla cinco veces: cinco callbacks y esperas de 1, 2, 4 y 8 s, sin espera tras el quinto fallo; luego Reject(false) y DLQ si el broker/configuración cumple esa ruta, y el estado terminal queda escrito por `Fail` (jobs) o encolado por `ReportFailure` (risk). Si ctx se cancela durante espera, retorna sin ejecutar el rechazo final; cierre de conexión deja al broker resolver la entrega no confirmada.

## Cobertura y contrato

El mapa cubre las ocho unidades que siguen existiendo en ambos archivos. El resto del adapter se relee aquí sin fila propia: `decode`/`failHandler`/`validFailure` (solo jobs), `handleWithRetries`, `settleDelivery`/`failJob` y `settleDeadLetter`/`reportFailure`, `DeadLetterSnapshot`/`Snapshot`/`invalidate`, `runSession`, `queueTopology` y `ReadyReason`/`MetricsText`. Estado: **explicado estáticamente**. Sí hay tests del adapter, inventariados como `Parcial`: `services/jobs/internal/messaging/broker_test.go`, `retry_schedule_regression_test.go` y `recovery_boundary_test.go`; `services/risk/internal/messaging/broker_test.go`. Fijan el cronograma y la frontera de recuperación, no el broker ejecutándose. El smoke expresa validación end-to-end de cuarentena y resiliencia ([tests y herramientas](08-tests-tools.md)). El contrato leído afirma entrega al menos una vez y Ack tras commit (`contracts/events/v1/README.md:11-15`), coherente con el wiring, no comprobado por ejecución en esta tarea.

Pendiente: semántica externa de Return/confirm, cancelación real, carreras, comportamiento de quorum/DLX, latencias y fallos de commit incierto. Ningún contador ni configuración reemplaza esas verificaciones.
