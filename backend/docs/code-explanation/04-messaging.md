# Mensajería: confirmación y deduplicación, no exactamente una vez

Cada servicio tiene su propio `Broker`: reconecta RabbitMQ, consume una cola y publica su outbox en paralelo. Las transacciones protegen efectos locales; el cruce PostgreSQL/RabbitMQ permite duplicados. La entrega se diseña **al menos una vez**, no exactamente una vez.

[Índice e instantánea](README.md) · [Risk](03-risk.md) · [Siguiente: contratos](05-contracts.md)

## Alcance, dependencias y diferencias

Lecturas completas: [broker jobs](../../services/jobs/internal/messaging/broker.go):1-236 y [broker risk](../../services/risk/internal/messaging/broker.go):1-222. Fecha, hashes y límites de la instantánea: [índice](README.md). No se ejecutó broker ni base de datos; semántica interna de `amqp091-go`/pgx y orden de notificaciones son fronteras externas.

| Unidad | Jobs | Risk | Responsabilidad |
| --- | --- | --- | --- |
| Imports / `Broker` | `services/jobs/internal/messaging/broker.go:1-26` | `services/risk/internal/messaging/broker.go:1-26` | Pool/URL; contadores atómicos; callback Handle; risk importa service, jobs UUID |
| `Run` | `services/jobs/internal/messaging/broker.go:28-47` | `services/risk/internal/messaging/broker.go:28-47` | Loop de sesiones y backoff |
| `session` | `services/jobs/internal/messaging/broker.go:49-123` | `services/risk/internal/messaging/broker.go:49-122` | Consume results en jobs, requests en risk; relay simultáneo |
| Validador | `services/jobs/internal/messaging/broker.go:125-136` | `services/risk/internal/service/demo.go:13-24` | Jobs valida resultados/BatchID; risk solicitudes/fecha futura |
| `topology` | `services/jobs/internal/messaging/broker.go:138-160` | `services/risk/internal/messaging/broker.go:124-146` | Declara ambas colas, exchanges y DLQs |
| `relay` | `services/jobs/internal/messaging/broker.go:162-175` | `services/risk/internal/messaging/broker.go:148-161` | Consulta outbox continuamente |
| `publishOne` | `services/jobs/internal/messaging/broker.go:177-225` | `services/risk/internal/messaging/broker.go:163-211` | Lock SQL, publicación confirmada y commit |
| `pause` | `services/jobs/internal/messaging/broker.go:227-236` | `services/risk/internal/messaging/broker.go:213-222` | Espera cancelable con timer |

El wiring es explícito: jobs `Handle=store.Complete`, risk `Handle=store.Publish` (ambos `cmd/server/main.go:91-94`). `Ready`, `Published`, `Processed`, `Rejected` son memoria de proceso, se reinician con él y no son un ledger durable de mensajes.

## Run: recuperación de sesión

En `services/jobs/internal/messaging/broker.go:28-47` y `services/risk/internal/messaging/broker.go:28-47`, `delay` comienza en un segundo; mientras ctx no esté cancelado llama `session`, descarta su error concreto y baja Ready. Si se canceló retorna; si no, registra reconexión y espera el delay. **Después de esperar**, compara `time.Since(started)>1 minuto`: incluye duración de sesión **más la pausa**, no solo la sesión. Si supera ese umbral reinicia delay a 1 s; si no, lo duplica hasta 30 s. Con sesiones y pausas acumuladas por iteración bajo el umbral, las esperas pueden ser 1, 2, 4, 8, 16, 30 s; la primera es 1 s. No es una medición de estabilidad de sesión aislada.

`pause` crea timer, difiere Stop y selecciona cancelación (false) o vencimiento (true): jobs:227-236 y risk:213-222. No hay jitter. Al no registrar el error concreto de `session`, el log de reconexión no explica por sí solo si falló DB, conexión, topología, publisher o consumidor.

## Topología durable y cuarentena

Los dos adapters declaran lo mismo. Cada llamada fallida retorna inmediatamente; el orden del loop sobre map de colas no es garantizado.

| Declaración / citas | Parámetros | Significado local y límite |
| --- | --- | --- |
| Exchange principal — `services/jobs/internal/messaging/broker.go:139-141`; `services/risk/internal/messaging/broker.go:125-127` | `firewatch.events`, topic, durable | Routing por clave v1 |
| Exchange muerto — `services/jobs/internal/messaging/broker.go:142-144`; `services/risk/internal/messaging/broker.go:128-130` | `firewatch.dead`, direct, durable | Separado del flujo normal |
| DLQs — `services/jobs/internal/messaging/broker.go:145-151`; `services/risk/internal/messaging/broker.go:131-137` | `<queue>.dead`, quorum; bind con clave `<queue>` | Rechazos pueden dirigirse a cuarentena |
| Colas normales — `services/jobs/internal/messaging/broker.go:152-158`; `services/risk/internal/messaging/broker.go:138-144` | Quorum; delivery-limit=5; DLX/routing; overflow reject-publish; dead-letter-strategy at-least-once | Configuración solicitada al broker, no validación observada de infraestructura |

Bindings: `risk.jobs.v1 <- jobs.requested.v1`; `jobs.results.v1 <- risk.batch.published.v1` ([contratos](05-contracts.md)). Una cola quorum en un nodo local no demuestra alta disponibilidad. El límite de delivery del broker es distinto de los cinco intentos del callback dentro de una sola entrega.

## Session: abrir, trabajar y cerrar

| Bloque y citas jobs / risk | Condición / operación | Error / estado |
| --- | --- | --- |
| 50-69 / 50-69 | Dial heartbeat 10 s y conexión 5 s; Channel; topology; Qos(1); Consume manual ack | Cualquier error retorna; cierres diferidos |
| 70-77 / 70-76 | Contexto hijo, errorsCh buffer 1, publisherDone; goroutine relay; Ready=true | Cleanup cancela, cierra conexión y espera publisher antes de salir |
| 78-87 / 77-86 | Select ctx.Done, error publisher o delivery cerrado | Devuelve error/cancelación; Run reconecta si corresponde |
| 88-95 / 87-94 | Body >16384, Unmarshal fallido o evento inválido | Rejected++, Reject(false); no requeue; si Reject falla termina sesión; si no continúa |
| 96-107 / 95-106 | Hasta cinco llamadas Handle, cada una con timeout 5 s; cancel inmediato; break si nil | Si error, espera 1,2,4,8,16 segundos incluso tras quinto fallo; cancelación retorna ctx.Err |
| 108-114 / 107-113 | Fallaron todos intentos | Rejected++; log de IDs; Reject(false), sin estado failed en DB |
| 114-120 / 113-119 | Callback exitoso | Processed++ y log **antes** de Ack(false); Ack fallido termina sesión |

Las citas de la tabla pertenecen respectivamente a `services/jobs/internal/messaging/broker.go` y `services/risk/internal/messaging/broker.go`. `Qos(1)` limita entregas sin ack según la dependencia AMQP, pero no es un límite global del número de workers entre réplicas. El publisher usa canales propios sobre la misma conexión. Un callback nil implica commit según los repositorios conectados; no significa que Ack haya llegado al broker. Por eso Processed puede aumentar y aun así haber redelivery.

### Validación de resultados jobs

`valid` exige tipo publicado, actor no vacío de hasta 255 bytes, zona no vacía y ambas fechas no cero; luego recorre ID, JobID, TraceID y BatchID y exige UUID no Nil (`services/jobs/internal/messaging/broker.go:126-137`). `Demo` no es una guarda de forma: describe el origen de la solicitud, y la relación entre la zona del lote y la del job se verifica contra la fila almacenada del job en `Store.Complete` (`services/jobs/internal/repository/store.go`), no aquí. Devuelve booleano, sin motivo detallado. A diferencia del validador risk, no limita RequestedAt futuro. JSON no usa `DisallowUnknownFields` en ninguno de los consumidores; campos adicionales no se rechazan por esa razón. La validación compara el tipo del envelope, no `delivery.Type` ni `delivery.MessageId`.

## Relay y publishOne: la ventana de duplicación

`relay` da 5 s a cada `publishOne`. Error termina el publisher y la sesión; sin fila espera 250 ms; con fila exitosa vuelve inmediatamente. Al cancelar retorna ctx.Err (jobs:162-175, risk:148-161).

| Fase / citas jobs y risk | Decisión / efecto | Propagación |
| --- | --- | --- |
| Begin + SELECT — `services/jobs/internal/messaging/broker.go:177-191`; `services/risk/internal/messaging/broker.go:163-177` | Transacción; rollback diferido; primera pendiente por created_at, `FOR UPDATE SKIP LOCKED` | NoRows -> false,nil; otros errores -> false,error; otras réplicas evitan fila bloqueada |
| Canal/confirms — `services/jobs/internal/messaging/broker.go:192-200`; `services/risk/internal/messaging/broker.go:178-186` | Abre canal nuevo por mensaje; Confirm(false); return channel buffer 1 | Error aborta; canal se cierra diferido |
| Publish — `services/jobs/internal/messaging/broker.go:201-208`; `services/risk/internal/messaging/broker.go:187-194` | JSON persistent, mandatory=true; MessageId=id, Type=key, timestamp now UTC; espera deferred confirm con contexto | Error publicación/espera devuelve sin marcar SQL |
| Confirm/return — `services/jobs/internal/messaging/broker.go:209-216`; `services/risk/internal/messaging/broker.go:195-202` | Ack falso -> `publish rejected`; notificación disponible -> `message unroutable`; default si no hay return disponible | No marca published en esas ramas de error |
| SQL/commit — `services/jobs/internal/messaging/broker.go:217-225`; `services/risk/internal/messaging/broker.go:203-211` | UPDATE published_at=now; commit; Published++ solo después | Error SQL/commit puede dejar mensaje ya entregado y fila aún pendiente |

El lock SQL se mantiene mientras se espera a RabbitMQ. La lectura de returns es no bloqueante después de confirm; la garantía de orden temporal entre Return y confirm depende de la biblioteca/broker y no fue examinada. Se describe lo que hace el código, no se certifica esa garantía.

## Traza razonada normal

Supuestos: una outbox jobs E comprometida, infraestructura acepta declaraciones, publish routable confirmado, callbacks/acks exitosos.

| Paso | Cita | Estado anterior -> posterior |
| --- | --- | --- |
| Selección jobs | `services/jobs/internal/messaging/broker.go:185-201` | E pending -> lock y publicación a `jobs.requested.v1` |
| Confirmación | `services/jobs/internal/messaging/broker.go:205-224` | ack true, sin return disponible -> fila published y contador +1 |
| Consumo risk | `services/risk/internal/messaging/broker.go:87-101` | JSON válido -> `Publish` con 5 s |
| Efectos | `services/risk/internal/repository/store.go:35-63` | Inbox E + lote B + outbox E2 -> commit |
| Ack | `services/risk/internal/messaging/broker.go:113-118` | Callback nil -> Processed++, Ack |
| Retorno jobs | `services/jobs/internal/messaging/broker.go:88-119` | Evento E2 publicado por relay risk -> Complete -> Ack; J published |

**Alternativa de crash:** RabbitMQ confirma E, pero falla el commit SQL de jobs (líneas 217-222). La sesión retorna error y luego reconecta; outbox puede conservar E pendiente y publicarlo otra vez. Risk reconoce E en inbox y retorna commit sin lote nuevo (`services/risk/internal/repository/store.go:35-41`). Un crash entre commit de consumidor y Ack también puede producir redelivery. Esta ventana justifica dedupe, no una promesa exactly-once.

**Alternativa de agotamiento:** DB del consumidor falla cinco veces: cinco callbacks y esperas hasta 16 s, luego Reject(false) y DLQ si el broker/configuración cumple esa ruta. Si ctx se cancela durante espera, retorna sin ejecutar el rechazo final; cierre de conexión deja al broker resolver la entrega no confirmada.

## Cobertura y contrato

Las ocho unidades del mapa, incluidos campos atómicos, inicializadores, goroutines, loops, select y salidas de error, se explican para ambos archivos completos. Estado: **explicado estáticamente**. No hay tests específicos de `messaging` en el inventario; el smoke expresa validación end-to-end de cuarentena y resiliencia ([tests y herramientas](08-tests-tools.md)). El contrato leído afirma entrega al menos una vez y Ack tras commit (`contracts/events/v1/README.md:11-15`), coherente con el wiring, no comprobado por ejecución en esta tarea.

Pendiente: semántica externa de Return/confirm, cancelación real, carreras, comportamiento de quorum/DLX, latencias y fallos de commit incierto. Ningún contador ni configuración reemplaza esas verificaciones.
