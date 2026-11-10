# Mensajería: confirmación y deduplicación, no exactamente una vez

**Re-derivado el 2026-10-20 contra el código de `267a4bb`.** **Corregido el 2026-11-09 para el cambio de mínimo privilegio del broker (BH-04):** dos exchanges de publicación, identidades por servicio y topología propiedad del aprovisionador. Los rangos de `broker.go` se releyeron contra el árbol actual (602 líneas en jobs, 511 en risk); las demás citas conservan la re-derivación del 2026-10-20.

Cada servicio tiene su propio `Broker`: reconecta RabbitMQ, consume una cola y publica su outbox en paralelo. Las transacciones protegen efectos locales; el cruce PostgreSQL/RabbitMQ permite duplicados. La entrega se diseña **al menos una vez**, no exactamente una vez.

[Índice e instantánea](README.md) · [Risk](03-risk.md) · [Siguiente: contratos](05-contracts.md)

## Alcance, dependencias y diferencias

Lecturas completas: [broker jobs](../../services/jobs/internal/messaging/broker.go):1-602 y [broker risk](../../services/risk/internal/messaging/broker.go):1-511. Fecha, hashes y límites de la instantánea: [índice](README.md). No se ejecutó broker ni base de datos; semántica interna de `amqp091-go`/pgx y orden de notificaciones son fronteras externas.

| Unidad | Jobs | Risk | Responsabilidad |
| --- | --- | --- | --- |
| Imports / `Broker` | `services/jobs/internal/messaging/broker.go:1-62` | `services/risk/internal/messaging/broker.go:1-57` | Pool/URL; contadores atómicos y snapshot de DLQ; callback `Handle`; jobs añade `Fail`, risk `ReportFailure` e importa `service` |
| `Run` | `services/jobs/internal/messaging/broker.go:107-129` | `services/risk/internal/messaging/broker.go:96-118` | Loop de sesiones, invalidación de salud y backoff |
| `session` | `services/jobs/internal/messaging/broker.go:131-184` | `services/risk/internal/messaging/broker.go:120-173` | Dial y consumo; canal monitor de la DLQ; delega en `runSession` |
| Validador | `services/jobs/internal/messaging/broker.go:290-312` | `services/risk/internal/service/demo.go:32-43` | Jobs valida resultados, BatchID y reportes de fallo; risk solicitudes con fecha futura acotada |
| `topology` | `services/jobs/internal/messaging/broker.go:509-526` | `services/risk/internal/messaging/broker.go:418-435` | Verificación pasiva del exchange propio y de las dos colas propias; no declara nada |
| `relay` | `services/jobs/internal/messaging/broker.go:528-541` | `services/risk/internal/messaging/broker.go:437-450` | Consulta outbox continuamente |
| `publishOne` | `services/jobs/internal/messaging/broker.go:543-591` | `services/risk/internal/messaging/broker.go:452-500` | Lock SQL, publicación confirmada y commit |
| `pause` | `services/jobs/internal/messaging/broker.go:593-602` | `services/risk/internal/messaging/broker.go:502-511` | Espera cancelable con timer |

El wiring es explícito: jobs `Handle=store.Complete` y `Fail=store.Fail` (`services/jobs/cmd/server/main.go:130`); risk `Handle=publisher.Publish` y `ReportFailure=store.RecordJobFailure` (`services/risk/cmd/server/main.go:215`). `Ready`, `Published`, `Processed`, `Rejected` son memoria de proceso, se reinician con él y no son un ledger durable de mensajes. Ambos adapters publican además profundidad y edad de la DLQ (`DeadLetterSnapshot`, `Snapshot`, `invalidate`): `firewatch_dead_letter_depth` y `firewatch_dead_letter_depth_age_seconds` (`services/jobs/internal/messaging/broker.go:95-105`; `services/risk/internal/messaging/broker.go:84-94`), y jobs suma `firewatch_queued_job_age_seconds`. `ReadyReason` convierte una lectura desconocida o sobre umbral en razón de no-listo, pero el código HTTP de `/health/ready` depende solo del ping a la base (`services/jobs/cmd/server/main.go:256-265`; `services/risk/cmd/server/main.go:523-536`).

## Run: recuperación de sesión

En `services/jobs/internal/messaging/broker.go:107-129` y `services/risk/internal/messaging/broker.go:96-118`, `invalidate` deja Ready falso y la profundidad en UNKNOWN; `delay` comienza en un segundo; mientras ctx no esté cancelado llama `session`, descarta su error concreto (`_ = b.session(ctx)`) e invalida otra vez. Si se canceló retorna; si no, registra reconexión con `retry_seconds` y espera el delay. **Después de esperar**, compara `time.Since(started)>1 minuto`: incluye duración de sesión **más la pausa**, no solo la sesión. Si supera ese umbral reinicia delay a 1 s; si no, lo duplica hasta 30 s. Con sesiones y pausas acumuladas por iteración bajo el umbral, las esperas pueden ser 1, 2, 4, 8, 16, 30 s; la primera es 1 s. No es una medición de estabilidad de sesión aislada.

`pause` crea timer, difiere Stop y selecciona cancelación (false) o vencimiento (true): jobs:593-602 y risk:502-511. No hay jitter. Al no registrar el error concreto de `session`, el log de reconexión no explica por sí solo si falló DB, conexión, topología, publisher, consumidor o la lectura pasiva de la DLQ.

## Topología: el aprovisionador declara, los servicios verifican

Ninguno de los dos adapters declara recursos. `topology()` — `services/jobs/internal/messaging/broker.go:509-526`; `services/risk/internal/messaging/broker.go:418-435` — hace solo comprobaciones pasivas: `ExchangeDeclarePassive` sobre su propio exchange de publicación y `QueueDeclarePassive` sobre su propia cola y su cola de muertos, y nombra en el error el recurso que falta. Declarar dejó de ser posible bajo mínimo privilegio: enlazar una cola al exchange del par exige permiso de escritura sobre ese exchange, y tenerlo permitiría a un servicio publicar como su par, que es lo que la separación de exchanges niega.

La topología la crea `deploy/local/rabbitmq-topology.sh`, que Compose ejecuta como servicio de un solo uso `broker-topology`; `jobs` y `risk` esperan a que termine con `service_completed_successfully` (`docker-compose.yml:76-96,165,218`).

| Recurso | Configuración | Quién lo crea |
| --- | --- | --- |
| `firewatch.jobs.events` | topic, durable | Aprovisionador; solo `jobs` escribe en él |
| `firewatch.risk.events` | topic, durable | Aprovisionador; solo `risk` escribe en él |
| `firewatch.dead` | direct, durable | Aprovisionador; ningún servicio escribe en él |
| `risk.jobs.v1`, `jobs.results.v1` | quorum; delivery-limit=5; DLX/routing; overflow reject-publish; dead-letter-strategy at-least-once | Aprovisionador, con los argumentos que los servicios usaban al declarar |
| `risk.jobs.v1.dead`, `jobs.results.v1.dead` | quorum | Aprovisionador |

Los cinco bindings también son del aprovisionador: `risk.jobs.v1` ← `firewatch.jobs.events` con `jobs.requested.v1`; `jobs.results.v1` ← `firewatch.risk.events` con `risk.batch.published.v1` **y** `risk.job.failed.v1` ([contratos](05-contracts.md)); `risk.jobs.v1.dead` ← `firewatch.dead` con `risk.jobs.v1`; `jobs.results.v1.dead` ← `firewatch.dead` con `jobs.results.v1`. El broker enruta el dead letter con la clave igual al nombre de la cola. Una cola quorum en un nodo local no demuestra alta disponibilidad. El límite de delivery del broker es distinto de los cinco intentos del callback dentro de una sola entrega.

### Matriz de permisos efectiva

Leída de los permisos efectivos del broker, no deducida del código.

| Identidad | configure | write | read |
| --- | --- | --- | --- |
| `firewatch-jobs` | `firewatch.jobs.events`, `jobs.results.v1`, `jobs.results.v1.dead` | `firewatch.jobs.events` | `jobs.results.v1` |
| `firewatch-risk` | `firewatch.risk.events`, `risk.jobs.v1`, `risk.jobs.v1.dead` | `firewatch.risk.events` | `risk.jobs.v1` |

`configure` cubre solo lo propio, `write` un único exchange, `read` una única cola. Nadie tiene `write` sobre `firewatch.dead`: el dead-lettering lo hace el broker, y ese permiso permitiría a un servicio inyectar un dead letter falsificado en la cola de su par. Ninguna identidad lleva tag de management: la API de management responde `401 {"error":"not_authorised","reason":"Not management user"}` antes de comprobar permisos, así que la operación sigue usando la identidad admin `firewatch`.

En local, Compose deriva `RABBITMQ_PASSWORD-jobs` y `RABBITMQ_PASSWORD-risk` del único secreto que ya exige, así que un clone nuevo no necesita variables nuevas; el overlay de producción exige `RABBITMQ_JOBS_PASSWORD` y `RABBITMQ_RISK_PASSWORD`, una por identidad (`docker-compose.prod.yml:114-115`). El exchange compartido anterior, `firewatch.events`, se borró después de la migración.

## Session: abrir, trabajar y cerrar

| Bloque / cita (jobs / risk) | Condición / operación | Error / estado |
| --- | --- | --- |
| `session` 131-184 / 120-173 | Dial heartbeat 10 s y conexión 5 s; Channel; verificación pasiva de topología; Qos(1); Consume manual ack de `jobs.results.v1` (jobs) o `risk.jobs.v1` (risk) | Cualquier error retorna; cierres diferidos |
| `session` 131-184, `runSession` 438-507 / `session` 120-173, `runSession` 347-416 | Canal monitor con `NotifyClose` de conexión, consumidor y monitor; ticker de 3 s; lectura pasiva de la DLQ; `runSession` con contexto hijo, `errorsCh` buffer 2, gate de apagado, goroutine relay y Ready=true tras la primera lectura | Si la lectura pasiva o el relay fallan, `shutdown` invalida, cancela y cierra la conexión |
| `runSession` 438-507, `consume` 186-219 / `runSession` 347-416, `consume` 175-210 | Select de `ctx.Done`, `failures` (cierre AMQP) o error del relay; select de `ctx.Done` o delivery cerrado | Devuelve error/cancelación; Run reconecta si corresponde |
| `decode` 228-247, `consume` 186-219 / `consume` 175-210 | Body >16384, Unmarshal fallido, tipo no reconocido o evento inválido | Rejected++, Reject(false); no requeue; si Reject falla termina sesión; si no continúa |
| `handleWithRetries` 379-395, `consume` 186-219 / `handleWithRetries` 288-304, `consume` 175-210 | Hasta cinco llamadas Handle, cada una con timeout 5 s; nil o error permanente corta el loop; el quinto fallo **no** espera | Si el error es transitorio, espera 1, 2, 4 y 8 s entre intentos; cancelación devuelve ctx.Err |
| `settleDelivery` 347-356, `failJob` 362-369, `consume` 186-219 / `settleDeadLetter` 217-222, `reportFailure` 230-237, `consume` 175-210 | Fallaron todos los intentos | Rejected++, log con `attempts` y `permanent`; jobs escribe `jobs.failure_reason` con `Fail`, risk encola `risk.job.failed.v1` con `ReportFailure`; después Reject(false) |
| `consume` 186-219, `settleDelivery` 347-356 / `consume` 175-210, `settleDeadLetter` 217-222 | Callback exitoso | Processed++ y log **antes** de Ack(false); Ack fallido termina sesión |

Las citas de la tabla son rangos de `services/jobs/internal/messaging/broker.go` a la izquierda de la barra y de `services/risk/internal/messaging/broker.go` a la derecha; el nombre de la función acompaña al rango para localizar el bloque. `Qos(1)` limita entregas sin ack según la dependencia AMQP, pero no es un límite global del número de workers entre réplicas. El publisher usa canales propios sobre la misma conexión. Un callback nil implica commit según los repositorios conectados; no significa que Ack haya llegado al broker. Por eso Processed puede aumentar y aun así haber redelivery. `Fail` y `ReportFailure` nil son seguros: el rechazo y su dead letter quedan como única evidencia.

### Validación de resultados jobs

`valid` exige tipo publicado, actor no vacío de hasta 255 bytes, zona no vacía, ambas fechas no cero y metadatos sintéticos emparejados (`scenario_id` con `evaluated_at` no cero, o ninguno); luego recorre ID, JobID, TraceID y BatchID y exige UUID no Nil (`services/jobs/internal/messaging/broker.go:290-312`). El consumidor jobs además despacha `risk.job.failed.v1` a `validFailure` y `failHandler`, que exigen motivo no vacío de hasta 255 bytes y `attempts>0` (`services/jobs/internal/messaging/broker.go:267-278,254-259`), y descarta cualquier otro tipo (`services/jobs/internal/messaging/broker.go:228-247`). `Demo` no es una guarda de forma: describe el origen de la solicitud, y la relación entre la zona del lote y la del job se verifica contra la fila almacenada del job en `Store.Complete` (`services/jobs/internal/repository/store.go`), no aquí. Devuelve booleano, sin motivo detallado. A diferencia del validador jobs, risk acota `RequestedAt` a un minuto futuro (`services/risk/internal/service/demo.go:33`). JSON no usa `DisallowUnknownFields` en ninguno de los consumidores; campos adicionales no se rechazan por esa razón. La validación compara el tipo del envelope, no `delivery.Type` ni `delivery.MessageId`.

## Relay y publishOne: la ventana de duplicación

`relay` da 5 s a cada `publishOne`. Error termina el publisher y la sesión; sin fila espera 250 ms; con fila exitosa vuelve inmediatamente. Al cancelar retorna ctx.Err (`services/jobs/internal/messaging/broker.go:528-541`; `services/risk/internal/messaging/broker.go:437-450`).

| Fase / citas jobs y risk | Decisión / efecto | Propagación |
| --- | --- | --- |
| Begin + SELECT — `services/jobs/internal/messaging/broker.go:544-557`; `services/risk/internal/messaging/broker.go:453-466` | Transacción; rollback diferido; primera pendiente por created_at, `FOR UPDATE SKIP LOCKED` | NoRows -> false,nil; otros errores -> false,error; otras réplicas evitan fila bloqueada |
| Canal/confirms — `services/jobs/internal/messaging/broker.go:558-566`; `services/risk/internal/messaging/broker.go:467-475` | Abre canal nuevo por mensaje; Confirm(false); return channel buffer 1 | Error aborta; canal se cierra diferido |
| Publish — `services/jobs/internal/messaging/broker.go:567-573`; `services/risk/internal/messaging/broker.go:476-483` | JSON persistent, mandatory=true; MessageId=id, Type=key, timestamp now UTC; espera deferred confirm con contexto | Error publicación/espera devuelve sin marcar SQL |
| Confirm/return — `services/jobs/internal/messaging/broker.go:575-582`; `services/risk/internal/messaging/broker.go:484-491` | Ack falso -> `publish rejected`; notificación disponible -> `message unroutable`; default si no hay return disponible | No marca published en esas ramas de error |
| SQL/commit — `services/jobs/internal/messaging/broker.go:583-590`; `services/risk/internal/messaging/broker.go:492-499` | UPDATE published_at=now; commit; Published++ solo después | Error SQL/commit puede dejar mensaje ya entregado y fila aún pendiente |

El lock SQL se mantiene mientras se espera a RabbitMQ. La lectura de returns es no bloqueante después de confirm; la garantía de orden temporal entre Return y confirm depende de la biblioteca/broker y no fue examinada. Se describe lo que hace el código, no se certifica esa garantía.

## Traza razonada normal

Supuestos: el aprovisionador ya creó la topología, una outbox jobs E comprometida, publish routable confirmado, callbacks/acks exitosos.

| Paso | Cita | Estado anterior -> posterior |
| --- | --- | --- |
| Selección jobs | `services/jobs/internal/messaging/broker.go:544-557` | E pending -> lock y publicación a `jobs.requested.v1` |
| Confirmación | `services/jobs/internal/messaging/broker.go:571-590` | ack true, sin return disponible -> fila published y contador +1 |
| Consumo risk | `services/risk/internal/messaging/broker.go:175-210` | JSON válido -> `publisher.Publish` con 5 s por intento |
| Efectos | `services/risk/internal/repository/store.go:177-281` | Inbox E + lote B + outbox E2 -> commit |
| Ack | `services/risk/internal/messaging/broker.go:201-207` | Callback nil -> Processed++, Ack |
| Retorno jobs | `services/jobs/internal/messaging/broker.go:191-218` | Evento E2 publicado por relay risk -> Complete -> Ack; J published |

**Alternativa de crash:** RabbitMQ confirma E, pero falla el commit SQL de jobs (`services/jobs/internal/messaging/broker.go:583-588`). La sesión retorna error y luego reconecta; outbox puede conservar E pendiente y publicarlo otra vez. Risk reconoce E en inbox y retorna commit sin lote nuevo (`services/risk/internal/repository/store.go:183-188`). Un crash entre commit de consumidor y Ack también puede producir redelivery. Esta ventana justifica dedupe, no una promesa exactly-once.

**Alternativa de agotamiento:** DB del consumidor falla cinco veces: cinco callbacks y esperas de 1, 2, 4 y 8 s, sin espera tras el quinto fallo; luego Reject(false) y DLQ si el broker/configuración cumple esa ruta, y el estado terminal queda escrito por `Fail` (jobs) o encolado por `ReportFailure` (risk). Si ctx se cancela durante espera, retorna sin ejecutar el rechazo final; cierre de conexión deja al broker resolver la entrega no confirmada.

## Cobertura y contrato

El mapa cubre las ocho unidades que siguen existiendo en ambos archivos. El resto del adapter se relee aquí sin fila propia: `decode`/`failHandler`/`validFailure` (solo jobs), `handleWithRetries`, `settleDelivery`/`failJob` y `settleDeadLetter`/`reportFailure`, `DeadLetterSnapshot`/`Snapshot`/`invalidate`, `runSession`, `topology` y `ReadyReason`/`MetricsText`. Estado: **explicado estáticamente**. Sí hay tests del adapter, inventariados como `Parcial`: `services/jobs/internal/messaging/broker_test.go`, `retry_schedule_regression_test.go` y `recovery_boundary_test.go`; `services/risk/internal/messaging/broker_test.go`. Fijan el cronograma y la frontera de recuperación, no el broker ejecutándose. El smoke expresa validación end-to-end de cuarentena y resiliencia ([tests y herramientas](08-tests-tools.md)). El contrato leído afirma entrega al menos una vez y Ack tras commit, y desde esta corrección nombra los tres eventos y los dos exchanges de publicación (`contracts/events/v1/README.md:3-16`), coherente con el wiring, no comprobado por ejecución en esta tarea.

Pendiente: semántica externa de Return/confirm, cancelación real, carreras, comportamiento de quorum/DLX, latencias y fallos de commit incierto. Ningún contador ni configuración reemplaza esas verificaciones.
