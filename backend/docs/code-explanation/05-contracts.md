# Contratos: separar el formato compartido del comportamiento

Los servicios comparten un envelope JSON y una especificación HTTP, no repositorios ni reglas de negocio. Aunque vive en `contracts/gen`, el tipo Go está **escrito a mano**; no se presupone un generador ni correspondencia automática entre OpenAPI y handlers.

[Índice e instantánea](README.md) · [Mensajería](04-messaging.md) · [Siguiente: persistencia](06-database.md)

## Alcance y evidencia

Lecturas completas: [events.go](../../contracts/gen/events/v1/events.go):1-26, [contrato de eventos](../../contracts/events/v1/README.md):1-15 y [OpenAPI](../../contracts/openapi/firewatch-v1.yaml):1-203. Snapshot: [índice](README.md). El módulo de tres líneas se explica en [despliegue](07-deployment.md). No se ejecutó validación OpenAPI, generación ni código. Las referencias `$ref` son contratos declarativos, no llamadas Go.

## Envelope y direcciones

| Declaración / líneas de `contracts/gen/events/v1/events.go` | Valor o interfaz | Uso comprobado |
| --- | --- | --- |
| 1-5 | Comentario handwritten; paquete v1; import time | No lógica de dominio compartida |
| 7-13 | Exchange `firewatch.events`; tipos `jobs.requested.v1` / `risk.batch.published.v1`; colas `risk.jobs.v1` / `jobs.results.v1` | Declaración y bindings en ambos `topology`; relays toman routing_key de outbox |
| 15-22 | ID, Type, OccurredAt, TraceID, JobID, ActorID, ZoneID | Correlación, validación y match contra trabajo |
| 23-26 | BatchID omitempty; RequestedAt; Demo | BatchID ausente para request; obligatorio por validador de resultado; fechas como time.Time |

Los strings Go no imponen UUID ni tamaño por sí solos; las restricciones vienen de handlers/consumidores/SQL. `omitempty` solo afecta BatchID vacío al serializar, no obliga un campo en deserialización. `Demo` bool con valor cero false y fechas cero se rechazan por consumidores, no por la declaración.

`contracts/events/v1/README.md:3-9` documenta productor y suscriptor; líneas 11-13 especifican UUIDs, fechas UTC, deduplicación transaccional, Ack después de commit y versionado de cambios incompatibles. Líneas 15 prohíben llevar datasets/modelos en bodies y aclaran ausencia de predicciones. Es **intención**. La producción de solicitudes usa fecha DB y `Demo=true` (`services/jobs/internal/repository/store.go:52`); risk copia y transforma envelope (`services/risk/internal/repository/store.go:51-55`). Ningún generador valida automáticamente esa coherencia.

## OpenAPI: mapa completo de declaraciones

| Bloque / líneas de `contracts/openapi/firewatch-v1.yaml` | Responsabilidad / entradas | Resultado declarado |
| --- | --- | --- |
| 1-9 | OpenAPI 3.0.3, versión 0.1.0, servidor localhost, seguridad global bearer | Clasificación con regla propia no oficial |
| 11-41 | POST jobs admin; Idempotency-Key UUID requerido; body objeto solo zone_id demo | 202 Job; 400/401/403/415/429/503 |
| 42-60 | GET job, ID path UUID, admin | 200 Job; 400/401/403/404; estados queued/published |
| 61-75 | GET latest, parámetro Zone, viewer/admin, más reciente por solicitud | 200 Batch; 400/401/403/404 insufficient |
| 76-91 | GET history, parámetro Zone, hasta 100 por solicitud descendente | Array de Batch; 400/401/403 |
| 92-113 | GET audit admin, últimas 100 | Objetos actor/action/job/trace/fecha; 401/403 |
| 114-126 | bearerAuth HTTP JWT; Zone required enum; responses reutilizables | Refs centralizan descripciones, no ejecución |
| 127-136 | Schema Job | IDs y fechas; enum state; batch_id nullable; required no incluye batch_id |
| 137-158 | Schema Batch y propiedades básicas required | IDs/fechas; modelo y demo documentados sin enum; required incluye factors/rationale/policy |
| 159-168 | geometry Point con dos coordenadas numéricas | No limita rangos lon/lat ni impone SRID en OpenAPI |
| 169-203 | data_status current/stale; risk_level canónico (bajo/medio/alto/desconocido); notice; factors, rationale y policy nullable | Clasificador propio no oficial; `desconocido` nunca se presenta como riesgo bajo |

Las branches HTTP reales están en [jobs](02-jobs.md) y [risk](03-risk.md). Aquí los “casos” son alternativas de respuesta declarada, no código ejecutable. Los refs internos apuntan a Job, Batch, Zone y responses definidos en el mismo archivo; no requieren un schema remoto en el texto leído.

## Diferencias y fronteras observadas

| Código implementado | Contrato declarado / consecuencia |
| --- | --- |
| Gateway puede devolver 502 al fallar upstream (`services/gateway/internal/handler/gateway.go:34`) | No figura 502 en respuestas OpenAPI: inventario de errores no exhaustivo |
| GET job/audit/risk pueden producir 503 (`services/jobs/internal/handler/api.go:70-80`; `services/risk/internal/handler/api.go:28-30`) | Algunas operaciones no declaran 503; no implica que nunca ocurra |
| Gateway rate limit es global local antes de rutas (`services/gateway/internal/handler/gateway.go:59-72`) | Solo POST enumera 429; otras rutas /v1 también pueden recibirlo |
| JSON POST no admite desconocidos ni segundo documento (`services/jobs/internal/handler/api.go:39-43`) | additionalProperties=false expresa una parte, no el chequeo de documento adicional |
| Query risk rechaza claves adicionales/duplicadas (`services/risk/internal/handler/api.go:18-21`) | Zone enum/required no describe por sí solo toda esa restricción |
| Latest vacío devuelve un objeto con código/data_status (`services/risk/internal/handler/api.go:33-35`) | 404 solo se describe, no schema de ese body |
| Errores mux de método/ruta pueden no usar helpers JSON | OpenAPI no define forma uniforme de error ni request_id obligatorio |

Los roles se describen en prose, no como scopes del esquema bearer; un cliente no debe interpretar ausencia de scopes como acceso universal. No se lee el realm excluido ni se verifica que la configuración de identidad emita esos claims.

## Traza razonada de transformación

Supuestos: aceptación transaccional con IDs válidos y fixture demo.

| Paso | Cita | Transformación |
| --- | --- | --- |
| HTTP | `contracts/openapi/firewatch-v1.yaml:15-35`; `services/jobs/internal/handler/api.go:31-50` | zone_id e Idempotency-Key -> parámetros del repositorio, sujeto desde JWT |
| Request event | `services/jobs/internal/repository/store.go:52-57` | Crea E con tipo solicitado, JobID J, Demo true, BatchID vacío -> JSON sin batch_id |
| Publication event | `services/risk/internal/repository/store.go:51-60` | Copia E; nuevo ID E2, tipo publicado, BatchID B, OccurredAt now -> JSON de resultado |
| Completion | `services/jobs/internal/messaging/broker.go:126-137`; `services/jobs/internal/repository/store.go:100-133` | `valid` exige BatchID UUID; `Complete` compara actor y zona contra la fila del job -> J published |

**Alternativa:** envelope con Type publicado pero BatchID vacío pasa el decode Go, pero `valid` devuelve false y broker rechaza (`services/jobs/internal/messaging/broker.go:88-94,129-133`). Un schema Go compartido no sustituye validación semántica. Cambiar routing key sin migrar bindings puede originar `message unroutable` en relay, no un error de compilación.

## Cobertura y límites

| Archivo / unidades | Estado / hueco |
| --- | --- |
| `events.go:1-26`: comment/paquete/import, cinco constantes, diez campos | Explicado; sin branches ejecutables propias |
| `contracts/events/v1/README.md:1-15`: transporte, tabla, envelope, garantías, límites | Explicado como intención |
| `firewatch-v1.yaml:1-203`: metadata, cinco rutas, componentes, refs y propiedades | Explicado por bloques declarativos; no se validó parser ni generación |

No se afirma que contrato y código sean idénticos. Próxima revisión propuesta: completar responses y schemas de error, comprobar cliente real y validar specs con herramienta autorizada; en este slice sí se modificó el schema `Batch` de la OpenAPI para declarar `factors`, `rationale`, `policy` y los enums reales.
