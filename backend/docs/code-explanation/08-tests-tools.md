# Tests y herramientas: expectativas leídas, no ejecución observada

**Re-derivado el 2026-10-20 contra el código de `267a4bb`.**

Los tests describen contratos de autorización, idempotencia y orden temporal; los scripts expresan una demostración end-to-end. Esta tarea solo los leyó: leer un archivo de test no es ejecutarlo. No presenta un resultado PASS ni usa evidencia histórica como si se hubiera reproducido.

[Índice e instantánea](README.md) · [Configuración](07-deployment.md)

## Alcance y cobertura estática completada

Se releyeron completos los ocho archivos que este documento ya citaba —seis Go de tests y dos scripts Python— y se recontó cada rango contra el árbol de hoy: `gateway_test.go` pasó de 54 a 1204 líneas, `jobs api_test.go` de 114 a 554, `risk api_test.go` de 70 a 527, las dos integraciones de 57/87 a 419/502 y `smoke.py` de 150 a 168. El [índice](README.md) todavía declara 1-272 y 1-145 para dos de esos archivos: esa fila es deuda de T3b, no de esta re-derivación. Las suites nuevas se listan junto a sus pares y solo se describen las que se leyeron completas; el resto queda como **no releído** en la tabla final, sin heredar la palabra «explicado». No se extiende esa cobertura a internals de bibliotecas, permisos efectivos ni resultados end-to-end. No se ejecutó ningún helper ni se abrió `.env`. Leer una llamada a credenciales no autoriza ejecutarla.

## Tests gateway y handlers

| Archivo / unidades y rango | Preparación / casos | Expectativa y límites |
| --- | --- | --- |
| [gateway_test.go](../../services/gateway/internal/handler/gateway_test.go):1-101 | Imports1-16; `allowedOrigin`20; `newTestGateway`22-28; fake `verifier`30-37; test39-70; tabla57; preflight74-101 | Cinco casos GET: `/v1/jobs` válido→202, token inválido→401, `/metrics`→**200** (serie local, ya no 404), `/health/live` y `/health/ready`→204. El upstream exige identidad no reenviada, `Bearer valid` conservado y request ID distinto de `spoofed` (41-49). Cada iteración crea una `New` nueva (65), así que no comparte cuota; `t.Errorf` sigue el loop y no hay subtests. El preflight del origen configurado responde204 con métodos, headers, Max-Age y Vary literales |
| [gateway_test.go](../../services/gateway/internal/handler/gateway_test.go):103-394 | `headerAllowed`103-114; Idempotency-Key116-135; allow-list explícita137-158; origen ajeno160-174; fail-closed176-202; CORS en401/404/429:204-250 y en502:252-271; preflight sin cuota273-304; página retirada306-325; bearer y health327-346; X-Request-ID expuesto348-394 | El preflight no refleja cabeceras ni usa comodín; sin origen configurado no emite CORS alguno; los errores y el 502 conservan origen y `Vary`. Doscientas preflights no gastan el cupo de la siguiente petición real. Las rutas de la página retirada responden404 con `not_found`. Sin token, `/v1/risk/latest` sigue401 y `/health/ready`204. `Access-Control-Expose-Headers` incluye X-Request-ID en preflight y en 401 |
| [gateway_test.go](../../services/gateway/internal/handler/gateway_test.go):396-694 | `subjectVerifier`396-404; `serve`406-412; cuotas por actor414-448; tope agregado450-481; bucket anónimo483-503; X-Forwarded-For505-531; health/preflight sin cuota533-575; UUID canónico577-599; un solo correlation id601-656; body igual a header658-694 | Agotar actor-a no consume el cupo de actor-b; el tope agregado responde429 con `Retry-After: 1` y `rate_limited`; los anónimos comparten un bucket y `X-Forwarded-For` no abre uno nuevo. `verify` no ve contexto y devuelve subject igual al token. El id de respuesta es uno solo, canónico y del gateway aunque el upstream lo reacuñe; el `request_id` del cuerpo es el mismo literal |
| [gateway_test.go](../../services/gateway/internal/handler/gateway_test.go):696-1204 | `/metrics`696-763; scrape sin cuota765-794; `statusRecorder`796-822 y flush824-839; rutas prevention841-948; rutas existentes950-1005; models hacia risk1007-1090; prefijo risk cubre cycle1092-1125; postura prevention en401/preflight/502:1127-1204 | Cinco peticiones pueblan las clases 2xx y 4xx, dejan 3xx en cero, fijan una 5xx y comprueban histograma acumulado y series de proceso; cien scrapes no consumen cuota. El wrapper reporta el código que net/http envía y deja alcanzable el writer para el flush. Siete rutas prevention llegan a su upstream con CORS y un solo id; jobs/risk conservan las suyas; `/v1/models` y la activación van a risk; `/v1/risk/cycle/run` ya estaba cubierta por el prefijo |

### Cómo interpretar los fixtures

Jobs construye claims con defaults de audiencia/issuer/expiración/tipo si el caso no los cambia (`services/jobs/internal/handler/api_test.go:286-302`). Firma JWT y asigna headers; genera clave de idempotencia salvo `badKey`; compara status y `store.calls` (líneas 303-335). Fallos de generación de clave, construcción del signer, firma y serialización compacta manejados con `t.Fatal` interrumpen el test o subtest, no simulan una respuesta de aplicación. El error de `json.Marshal` se ignora en línea 303.

Risk firma claims por caso y, si `data=true`, entrega un lote fake con demo/desconocido/current (`services/risk/internal/handler/api_test.go:206-228`). Ese fake no prueba cálculo SQL de frescura, límites/order o GeoJSON; solo habilita la rama HTTP. Los errores de signer/compactación se ignoran en el fixture (207-209), por lo que no se infiere cobertura de errores criptográficos.

Gateway no verifica criptografía: comprueba la integración con la interfaz `Verifier` (30-37). El upstream afirma headers eliminados y Bearer conservado (`services/gateway/internal/handler/gateway_test.go:40-51`), pero no demuestra por lectura que la versión de la biblioteca se comporte así al ejecutar.

### Gateway: todas las declaraciones y verificaciones

- Paquete/imports conectan contexto, errores, HTTP/httptest, URL, regexp, strings, JSON y el tipo OIDC; `verifier struct{}` no guarda estado. `Verify` ignora contexto: cualquier string distinto de `valid` devuelve `(nil,error)`; `valid` devuelve un IDToken con subject `admin`, sin claims ni firma (`services/gateway/internal/handler/gateway_test.go:1-37`).
- El callback upstream registra errores **no fatales** si llega cualquier cabecera de identidad/X-Forwarded-For, si Authorization no es exactamente `Bearer valid`, o si request ID sigue siendo `spoofed`; luego responde 202 incluso tras esos errores. La última comparación no exige un ID no vacío ni su formato. `defer upstream.Close()` limpia el servidor de fixture; el error de `url.Parse` se descarta (`services/gateway/internal/handler/gateway_test.go:39-53`).
- La tabla tiene ahora **cinco** casos, todos mediante GET: `/v1/jobs` válido→202, token inválido→401, `/metrics`→200, `/health/live`→204 y `/health/ready`→204. Cada iteración fija cuatro cabeceras no confiables y Authorization (para health queda `Bearer `), crea recorder y una instancia `New` nueva. Por ello no comparte cuota entre casos. Un status distinto usa `t.Errorf` y sigue el loop; no hay subtests, comprobación de body, ID de respuesta ni afirmación explícita del número de llamadas upstream (`services/gateway/internal/handler/gateway_test.go:54-70`).
- El preflight y el CORS no existían en la lectura anterior; hoy son el bloque mayor del archivo. `newTestGateway` fija `http://localhost:4173` y cada prueba decide si lo pasa o lo deja vacío (`services/gateway/internal/handler/gateway_test.go:20-28,72-101,160-202`). Ninguna de esas pruebas cubre un origen múltiple, credenciales CORS ni un preflight con token vencido.

### Jobs: fakes, preparación y dieciséis casos

`fakeStore` acumula campos para cinco métodos. `Create` incrementa `calls` y el slice `scenarios`, y devuelve ID nuevo, zona recibida y estado queued, sin error; no guarda actor/key/trace ni prueba que sean correctos (`services/jobs/internal/handler/api_test.go:49-57`). `Get` devuelve el Job y el error configurados (58-60), `Audit` nil/nil (61), `Replay` registra id/actor/trace/confirmación (66-72) y `ZoneExists` cuenta llamadas y responde el mapa o el error del caso (74-80).

La clave RSA de 2048 bits y el signer RS256 se construyen por test. Sus dos errores posibles terminan con `t.Fatal`. `StaticKeySet` usa la clave pública local, issuer `https://issuer.test`, audiencia `firewatch-api` y allowlist RS256: el fixture no usa JWKS remoto (`services/jobs/internal/handler/api_test.go:241-250`).

| Caso declarado — `services/jobs/internal/handler/api_test.go:251-283` | Cambio sobre defaults | Código esperado |
| --- | --- | --- |
| admin accepted | role admin | 202 |
| viewer forbidden / unknown role forbidden | viewer / operator | 403 |
| missing token | noToken=true; se firma token pero no se adjunta | 401 |
| wrong audience / wrong issuer | other / otro issuer | 401 |
| expired token / future token | exp hace un minuto / nbf dentro de una hora | 401 |
| id token rejected | typ ID en lugar de Bearer | 401 |
| unknown field / multiple documents | propiedad admin / segundo objeto JSON | 400 |
| unknown zone | zone_id other, ausente del catálogo fake | 400 |
| catalogue commune code accepted | zone_id `05101` presente en el catálogo | 202 |
| catalogue lookup fails | zoneErr; el handler responde `storage_unavailable` | 503 |
| missing idempotency | badKey=true **omite** cabecera; no prueba UUID mal formado o Nil | 400 |
| oversized body | 9000 letras dentro de zone_id | 400 |

Cada `t.Run` rellena solo valores vacíos/cero: aud/iss, exp=now+1 minuto, typ=Bearer y body demo. Subject es siempre `admin-subject`, aunque role cambie; nbf queda cero salvo caso futuro. Marshal se ignora, pero Sign/CompactSerialize fallidos terminan el subtest. Content-Type es siempre JSON; se omiten headers según noToken/badKey. El catálogo fake se puebla con `valparaiso-demo` cuando el caso no trae uno propio (325-327). Después de invocar el handler se exige status; fallo fatal evita la assertion siguiente. Para cualquier caso distinto de 202 se exige calls=0. Para 202 **no** se exige calls=1 ni body/Location correcto. No hay casos 415, error del store, subject vacío o claims mal tipados (`services/jobs/internal/handler/api_test.go:284-338`).

El archivo cubre además el estado de salud y el replay, que la lectura anterior no mencionaba. `TestStatusAuthorization` (82-150) prueba cinco roles contra `/v1/jobs/status`: sin token e inválido→401, operator y viewer→403, admin→200 con `Cache-Control: no-store`; luego degrada a 503 con `"ready":false` y a 503 `status_unavailable` con campos nullable en null. `TestGetJobExposesStateAndPersistedFailure` (152-239) exige que un job failed exponga `failure_reason`/`attempts`/`failed_at` y que queued/published los omitan (tres casos). `TestReplayRequiresAdmin` (408-442) cubre 401/403; `TestReplayRejections` (444-490) fija 404 `job_not_found`, 409 `job_not_replayable`, 409 `replay_confirmation_required`, 503 `storage_unavailable` y 400 `invalid_job_id` con `request_id` igual al trace; `TestReplayRejectsMalformedConfirmation` (516-554) rechaza siete cuerpos sin tocar el store.

### Risk: nueve casos, no matriz criptográfica completa

`fakeStore` acumula batches, catálogo de zonas y seis métodos: `ListModels`45, `ActivateModel`54, `List`70 —ignora zona/límite/contexto, incrementa `calls` y retorna la slice sin error—, `LatestPerZone`77, `ZoneExists`87 y `ListZones`95, que registra el bbox recibido. Imports/clave/signer son análogos a jobs; los errores de RSA/NewSigner son fatales. El verifier de `TestReadAccessAndInsufficientData` fija issuer/audiencia mediante clave estática y **no** declara `SupportedSigningAlgs` (179).

La tabla de `/v1/risk/latest` tiene ahora **nueve** entradas (189-204): viewer/admin con datos→200, viewer sin datos→404, role other→403, zona other→400, código de comuna presente en catálogo→200, parámetro duplicado→400, parámetro ausente→400 y fallo del catálogo→503 `storage_unavailable`. Cada subtest firma claims con exp=now+1 minuto y typ Bearer; ignora errores de Marshal, Sign y CompactSerialize (207-209). Todas las solicitudes son GET latest con token. Si data=true, el fake ofrece un único Batch con tres campos, sin IDs, geometría ni fechas (217-219); si false, slice nil. Status inesperado termina el subtest; solo el caso 403 comprueba calls=0 y zoneCalls=0 (225-227). Los 400 no tienen assertion auxiliar sobre persistencia y los 200/404 no verifican cantidad de llamadas, body ni valores de límite. No se prueba history ni frescura real (`services/risk/internal/handler/api_test.go:170-230`).

El resto del archivo es nuevo respecto de la lectura anterior: `TestStatusAuthorization` (104-168) con cinco roles —viewer sí puede leer el estado— más 503 degradado y 503 sin chequeo; `TestBatchesEndpoint` (270-363) prueba el agregado por zona con orden, arreglo vacío y `storage_unavailable`, y que sea una sola consulta y no una por zona; `TestBatchesEndpointKeepsTheReadVerificationPath` (365-393) repite 401/403/200/200 sin tocar almacenamiento; `TestParseBBox` (407-446) fija quince entradas de validación pura, sin recorte silencioso; `TestZonesEndpoint` (448-527) cubre catálogo completo, bbox válido, bbox sin coincidencias, bbox inválido, parámetro desconocido o repetido, catálogo vacío y 503, y exige geometría como objeto, no string.

## Tests de repositorio y validador

| Archivo / rango | Secuencia y branches | Intención, no resultado |
| --- | --- | --- |
| [jobs integración](../../services/jobs/internal/repository/store_integration_test.go):1-419 | Imports1-16; seis tests en18, 63, 124, 226, 247 y 362; skip en la guarda de cada uno; ctx20s/pool; usa `stringPtr` de `synthetic_scenarios_test.go:18` | Idempotencia transaccional, relación de zona en `Complete`, metadata sintética asignada una vez, y tres pruebas de replay. Depende de DB migrada desechable; crea datos y no limpia filas |
| [risk integración](../../services/risk/internal/repository/store_integration_test.go):1-502 | Imports1-16; nueve tests en18, 124, 185, 211, 244, 338, 370, 422 y 462; skip; ctx20s o60s | Deduplicación y orden, referencias ausentes declaradas permanentes, reporte de fallo al outbox, catálogo y bbox, hash de parámetros, territorio y publicación programada. Invoca `Store.Publish`/`PublishScheduled` directo, sin ValidateRequest |
| [demo_test.go](../../services/risk/internal/service/demo_test.go):1-69 | Imports1-9; metadata opcional11-34; tabla de nueve casos38-58; copia/muta/valida60-68 | Dos válidos (fixture demo y no-demo), zona `other` **aceptada** por el gate de forma, y rechaza tipo, zona vacía, ID inválido/Nil, actor vacío y solicitud futura |
| [retry_schedule_regression_test.go](../../services/jobs/internal/messaging/retry_schedule_regression_test.go):1-44 | Imports1-11; test17-44; dos iteraciones (permanente y temporal) | El presupuesto es 5 intentos con esperas literales 1, 2, 4 y 8 s; un error permanente gasta 1 intento y no espera. Escribe el cronograma en vez de compararlo con `retryBackoff` |
| [recovery_boundary_test.go](../../services/jobs/internal/messaging/recovery_boundary_test.go):1-116 | Fakes de acknowledger16-29; frontera de despacho35-102; prefijo de métricas104-116 | Siete casos: un reporte válido llega al writer de fallo y un completion al de éxito; tipo desconocido, razón vacía, attempts 0/negativo o razón de 256 caracteres se rechazan sin ack. La serie `queued_job_age_seconds` se añade tras el prefijo heredado |
| Migraciones y prevention: `services/risk/migrations/migrations_test.go:1-276`, `services/prevention/migrations/migrations_test.go:1-170`, `services/jobs/cmd/server/migration_registration_test.go:1-100`, `services/prevention/internal/state/machine_test.go:1-167`, `services/prevention/internal/handler/api_test.go:1-671` | Solo funciones y rangos verificados (10, 5, 2, 7 y 13 declaraciones `func Test`) | **No releído** en esta tarea: se listan como hermanos existentes, sin describir sus casos ni afirmar cobertura |

El test risk de integración con `OccurredAt` cero refuerza la distinción entre precondición del consumidor y método SQL: no prueba que un envelope así atravesaría el broker. Ninguno de estos tests muestra scheduler, modelos reales, tolerancia temporal exacta, reconexión o implementación de DLX. Todas las integraciones hacen `Skip` si `testing.Short()` o `TEST_DATABASE_URL` está vacía (`services/jobs/internal/repository/store_integration_test.go:19-21`, `services/risk/internal/repository/store_integration_test.go:19-21`), de modo que leerlas tampoco equivale a ejecutarlas.

### Jobs integración: secuencia y salidas completas

Imports habilitan contexto, lectura de **una variable** de entorno, JSON, strings, reloj, contrato, UUID y pool; no leen archivos `.env`. La guarda short/URL vacía hace Skip antes de crear pool. Contexto de 20 s y pool tienen limpieza diferida; error de New es fatal (`services/jobs/internal/repository/store_integration_test.go:1-25`).

En el primer test, actor/key/trace son UUIDs nuevos. Primer Create debe retornar nil error; segundo exige nil error **y** mismo ID (OR de fallo). Query de outbox filtrada por job_id exige Scan sin error y count=1. El resultado directo tiene UUIDs evento/batch, actor/zona coincidentes y trace inválido; omite Type, Demo y timestamps. Complete debe fallar y Get debe devolver queued. Se cambia solo trace y se reutiliza **el mismo ID evento**: Complete debe funcionar dos veces. La query final exige exactamente dos audit. Esto protege indirectamente rollback de inbox (`services/jobs/internal/repository/store_integration_test.go:18-61`). No comprueba estado published ni número de inbox.

El segundo test (63-121) prueba que un batch de **otra** zona no completa el job aunque `Demo=false`, que el error se declara permanente, que el job sigue queued, que no hay audit `batch.published`, y que el batch de la zona propia sí completa y deja una sola audit. El tercero (124-218) fija la metadata sintética: instante asignado en UTC por el servidor, evento del outbox con ese mismo instante, reintento con otro escenario que devuelve el original sin segunda fila de outbox, reintento legacy, job legacy sin metadata ni campos en el evento, `Complete` que no borra la metadata, y una inserción directa rechazada por la restricción de emparejamiento. El cuarto (226-245) es una costura pura: el evento de replay usa el actor y la zona **almacenados** y acuña ID nuevo.

Los dos últimos ya necesitan base (247-355 y 362-419). El primero falla un job (`Fail` con `permanent_error`/5), cuenta una sola fila de outbox previa, ejecuta `Replay` y exige job queued con attempts0 y campos terminales limpios, una segunda fila de outbox construida desde la fila guardada, exactamente una audit del operador, un segundo replay rechazado con `ErrReplayConfirmationRequired` sin filas nuevas, y un id desconocido distinguible (`pgx.ErrNoRows`). El segundo exige que el replay **no** confirmado no escriba nada, que el confirmado reinicie attempts sin mover `RequestedAt` ni perder el escenario, y que un job ya published siga rechazado con `ErrJobNotReplayable`. No hay cleanup SQL: Close no borra filas.

### Risk integración: dos capas de duplicación y orden

Misma guarda, timeout y cleanup de pool que jobs; cualquier error de pool o `Store.Publish` termina el test. El primer test (18-115) usa un assessment real —nivel `alto`, un factor `temperatura`, rationale y `Demo=false`—, publica el mismo evento dos veces (inbox) y comprueba que el outbox queda en 1; luego publica una copia con ID evento nuevo pero mismo JobID y una solicitud más antigua de otro job. `List` con límite 1 debe retornar exactamente un elemento con el JobID más nuevo. Exige dos completions que nombran **el mismo** batch guardado, una sola fila en batches, y que nivel, factor, rationale, `Demo=false` y policy nil sobrevivan el round trip. No cuenta el inbox completo ni aísla zona frente a datos concurrentes; presupone base desechable sin un trabajo ajeno más reciente.

Los ocho restantes son nuevos: referencias ausentes (zona fuera del catálogo y versión de modelo no registrada) deben fallar como permanentes sin dejar batches (117-178); el evento de fallo se construye como costura pura y su forma de wire se compara literal (185-205); `RecordJobFailure` inserta el reporte en el outbox propio con `routing_key` correcto (211-241); el catálogo completo viene ordenado por código y sin la fila demo sin geometría, el bbox continental es subconjunto estricto que excluye 05201 y un bbox lejano devuelve cero sin error (244-336); el hash de parámetros coincide con el congelado y una versión ausente se reporta como no encontrada (338-368); cada zona del catálogo publica y `List` devuelve la geometría del catálogo con límites 1 y 100 (370-420); una tabla temporal sin geometría hace que `List` falle con «no catalogue geometry» en vez de inventar territorio (422-460); y una publicación programada persiste el batch sin escribir completion, mientras el job explícito conserva exactamente una (462-502).

### Demo: mutaciones independientes y guardas ausentes

Paquete/imports usan testing, reloj, contrato y UUID. Base tiene IDs válidos, actor admin, tipo pedido, demo true, zona fija y ambas fechas no cero. La tabla declara nombre, callback de mutación y bool esperado. Nueve entradas: dos válidas (fixture demo y demo=false), tipo publicado, zona `other` —hoy **aceptada** a propósito: la pertenencia la decide el catálogo y la foreign key, no una allowlist Go—, zona vacía, ID invalid, ID Nil, actor vacío y RequestedAt ahora+1 h inválidos (`services/risk/internal/service/demo_test.go:37-58`).

Cada subtest **copia** base antes de mutarla, llama ValidateRequest y compara `(err==nil)` con valid; desigualdad es fatal. No compara texto de error ni modifica el fixture del siguiente caso. El reloj no está inyectado: no prueba umbral exacto de un minuto, actor >255 bytes, fechas cero, JobID/TraceID inválidos ni relación temporal entre fechas. La prueba de `SupportedZone` y de la allowlist exacta **ya no existe**: se retiró junto con la lista compilada. En su lugar, el primer test (11-34) fija que la metadata de escenario es opcional en el gate de forma y que un escenario sin instante pasa aquí y se rechaza después en el dominio.

## init_env.py: creación exclusiva, no rotación

[init_env.py](../../scripts/init_env.py):1-18 es un script de top-level, no función importable sin efectos. Docstring e imports1-4 explican propósito; root6 apunta backend; names7-12 enumera **nueve** variables locales (no valores). path12 selecciona destino; `os.open` con WRONLY|CREAT|EXCL y modo0600 (13) crea archivo solo si no existe. `fdopen`14-17 escribe comentario y por cada nombre genera `secrets.token_hex(24)`; print18 anuncia creación.

| Condición | Resultado razonado | Límite |
| --- | --- | --- |
| Archivo ausente y permisos válidos | Nueve secretos aleatorios escritos, 48 hex por valor | No se generaron ni leyeron aquí |
| Archivo existente | O_EXCL hace fallar open, sin sobrescribir | Excepción no capturada por script |
| Error después de creación | Context manager cierra stream si ya abierto; posible archivo parcial | No hay rollback de filesystem ni recuperación implementada |

El modo0600 expresa intención POSIX; permisos efectivos de Windows/ACL no se inspeccionaron. El script no rota credenciales ya guardadas en volúmenes ni sincroniza identidades existentes. Nada de esta sección reproduce datos personales o passwords.

No hay función, guarda de entrada, try/except ni rollback. La lista es finita (nueve iteraciones); cada token tiene 24 bytes aleatorios representados por 48 caracteres hex, seguido de newline. `fdopen` usa UTF-8/newline LF y al salir del with cierra/flushea el stream; si fdopen mismo falla, todavía no existe context manager que cierre fd. Resolución de ruta, creación, entropía, write, flush/close o print pueden propagar sus excepciones sin capturar; un fallo tardío puede dejar archivo completo o parcial y la siguiente ejecución no lo sobrescribe por O_EXCL. Print solo se alcanza tras cerrar correctamente, pero no verifica persistencia física con fsync. Son ramas inferidas desde instrucciones locales, no pruebas del OS (`scripts/init_env.py:6-18`).

## smoke.py: helpers y orquestación explícita

[smoke.py](../../scripts/smoke.py):1-168 importa argparse/base64/json/pathlib/subprocess/time/urllib/uuid (1-11), define ROOT y endpoints locales API/IDENTITY (13-15), el conjunto `CANONICAL_RISK_LEVELS` con los cuatro niveles del contrato (17-21) y solo llama main si `__name__=="__main__"` (167-168). Esa guarda no ejecutó nada durante lectura.

| Unidad / líneas de `scripts/smoke.py` | Entradas / transformación | Salidas / errores |
| --- | --- | --- |
| credentials,24-26 | Lee .env y separa líneas no vacías/no comentario por primer `=` | Dict; no parse robusto de quotes/export; no se invocó |
| request,29-42 | Copia headers; token agrega Bearer; dict body ->JSON/Content-Type; Request; urlopen timeout15 | Status+JSON o None si vacío; HTTPError ->code,None; otros errores no capturados aquí |
| wait_for,45-55 | Deadline monotonic, loop callback, retorno truthy, sleep1 | Captura URLError/TimeoutError/ConnectionError y reintenta; deadline ->RuntimeError; otras excepciones salen |
| token_for,58-65 | Form password grant/client demo; closure login devuelve access_token solo200 | wait_for; depende de realm excluido, no adecuado como flujo browser de producción |
| create_job,68-72 | POST demo con UUID nuevo o key dada | Assert202; devuelve job; error assertion termina |
| await_job,75-79 | GET por id; closure acepta200 y state published | wait_for; no reconoce failed como terminal |
| compose,82-83 | subprocess docker compose con cwd ROOT/checkTrue | Mutación real si ejecutado; no se invocó |

Las assertions de main incluyen 401 sin token, 403 para viewer en POST/audit, 400 zona unknown, mismo ID al reintentar, lote coincidente con batch_id, demo true/risk_level en el conjunto canónico, rationale string no vacío, Point y acciones de audit (`scripts/smoke.py:94-115`). `request` descarta cuerpo HTTPError, por lo que el script verifica principalmente códigos en caminos negativos, no schema completo de errores.

La prueba DLQ compara `messages_ready` anterior/posterior (`scripts/smoke.py:146-160`): en un entorno concurrente, un aumento podría incluir otro mensaje. Es intención razonable para demo aislada, no correlación por ID del mensaje malformed. Tampoco se afirma que assertions sean activas con cualquier modo Python: el archivo usa `assert`, cuyo funcionamiento depende de opciones de ejecución externas.

### Smoke: branches y excepciones de helpers

- Los imports no invocan main. ROOT se deriva del archivo; API/IDENTITY son constantes locales; el conjunto de niveles canónicos se declara sin efectos (17-21); la guarda final llama main solo cuando el nombre es `__main__`. No existe manejador global de excepciones (`scripts/smoke.py:1-21,167-168`).
- `credentials` incluye solo líneas no vacías y que no comienzan por `#`, divide por primer `=` y forma dict (claves repetidas conservan última entrada). Espacios no se recortan; un comentario con espacios iniciales no se descarta; una línea sin `=` provoca ValueError. Lectura/decodificación fallidas se propagan. Esa operación **no se invocó** aquí (`scripts/smoke.py:24-26`).
- `request` crea una copia de headers (o dict vacío), añade Authorization solo si token es truthy y convierte body a bytes JSON únicamente si es dict; eso sustituye Content-Type. Los bytes de formulario pasan sin conversión. Dentro del try, respuesta no vacía se parsea JSON y vacía retorna None; with cierra respuesta. HTTPError retorna solo code/None. Construcción de Request está antes del try; errores de URL, body, JSON, red o decodificación que no sean HTTPError no se capturan por este helper (`scripts/smoke.py:29-42`).
- `wait_for` evalúa check mientras reloj sea menor que deadline; valor truthy retorna intacto, falsy sigue. Captura únicamente URLError/TimeoutError/ConnectionError y duerme un segundo después de falsy o excepción capturada. Al agotar lanza RuntimeError. El timeout no es un límite duro: callback (por ejemplo HTTP de hasta 15 s) y sleep pueden sobrepasarlo; no hay cancelación de callback. JSONDecodeError, KeyError, TypeError y AssertionError salen inmediatamente (`scripts/smoke.py:45-55`).
- `token_for` codifica formulario con password grant/client fijo. `login` usa 200 para acceder a `data["access_token"]`; otro status devuelve None y reintenta. Un 200 sin objeto/token causa excepción de estructura no capturada por wait_for; access_token vacío sigue esperando. Realm y credenciales siguen siendo fronteras excluidas (`scripts/smoke.py:58-65`).
- `create_job` usa key recibida si truthy, de lo contrario UUID nuevo, POST demo y assert202; retorna job sin validar esquema. `await_job.completed` consulta id del job; status distinto de 200 cortocircuita y reintenta; 200 exige state published. Body ausente/mal formado con 200 genera error local en lugar de espera; no reconoce failed como terminal (`scripts/smoke.py:68-79`).
- `compose` ejecutaría docker compose con argumentos, cwd backend y check=True. Executable ausente o salida no cero se propagan; no captura stdout/stderr ni añade timeout. No se ejecutó (`scripts/smoke.py:82-83`).

### Smoke: inventario completo de main y callbacks

| Bloque / líneas de `scripts/smoke.py` | Decisiones, assertions y efectos razonados |
| --- | --- |
| 86-93 | Parser con único flag resilience booleano (false por defecto); argumentos inválidos terminan vía argparse. Lee env, accede dos claves y obtiene admin/viewer; espera **204 de `/health/ready`**, no de `/health/live`. |
| 94-98 | Assertions: GET jobs sin token401; POST viewer403 (sin idempotencia); audit viewer403; POST admin con zona unknown/UUID400. Cortocircuita y aborta al primer resultado inesperado. |
| 99-109 | UUID K; crea J; assertion hace segundo POST con K y compara ID; espera published; latest viewer exige status200 y batch_id de completion, luego demo true, risk_level en el conjunto canónico y geometry.type Point. Además exige rationale string no vacío, que la lectura anterior no comprobaba. No comprueba coordenadas, vigencia o model_version. |
| 110-115 | History200 y sum de coincidencias job_id exactamente1; audit200 y conjunto de acciones del job exactamente requested/published. El conjunto no detecta acciones duplicadas. Primer print PASS solo cubre esta fase; puede emitirse antes de un fallo de resiliencia/cuarentena posterior. |
| 117-127 | Si resilience false omite todo bloque; true ejecuta stop broker **antes del try**. Dentro crea queued, exige conservar batch previo y lee state queued sin assertion de status del GET. finally intenta start broker incluso si falló cuerpo del try; si stop falla no se entra a finally, y si start falla puede sustituir excepción previa. Luego await_job y restart jobs/risk. |
| 128-141 | El comentario 128-133 explica por qué latest no puede exigir el id recuperado: el ciclo de arranque publica otro batch. Closure persisted consulta history y luego latest: exige que el batch recuperado esté en history y que `data_status` sea `current`; wait_for espera esa convergencia. El print de esta fase no implica que cuarentena posterior funcione. |
| 143-155 | Cuarentena ocurre con o sin flag. Basic auth se forma en memoria, GET management de DLQ exige200 y messages_ready default0; latest exige200 antes de enviar. POST management publica el payload inválido `{invalid`, delivery_mode2 y clave pedido; exige200 y routed truthy. |
| 156-164 | Closure quarantined consulta DLQ: status no200 →false; 200 exige conteo mayor al inicial, default0; wait_for usa30 s nominales. Latest debe seguir200 y mismo ID que last_valid capturado antes del envío (también funciona tras fase resilience). Imprime PASS cuarentena y el ID del **job inicial**, no del recovered; no imprime tokens/passwords. |

Todos los accesos a dict/list, comprensiones, comparaciones y calls fallidas sin catch se propagan hasta proceso principal. No hay cleanup global de trabajos/lotes ni garantía de restaurar servicios si una interrupción ocurre fuera del try/finally. Con Python optimizado se omiten assertions **y sus expresiones**: también desaparecen las llamadas HTTP de 94-98, el segundo Create de 101 y el GET de 123; no solo se pierden comprobaciones. Esta observación estática explica por qué leer prints PASS no certifica un recorrido uniforme (`scripts/smoke.py:94-164`).

## Traza razonada y alternativa

Supuestos: script ejecutado hipotéticamente sin optimización, stack local sano, cuentas configuradas y DB limpia; esta tarea **no lo ejecutó**.

| Paso | Cita | Valores antes -> después |
| --- | --- | --- |
| Login | `scripts/smoke.py:58-65,90-93` | Respuesta200 con access_token -> tokens locales en memoria |
| Comando | `scripts/smoke.py:99-102` | Clave K -> J; repetición K ->mismo J; wait_for espera published |
| Lectura | `scripts/smoke.py:103-115` | B coincide con finalización, history una fila J, audit dos acciones |
| Cuarentena | `scripts/smoke.py:151-162` | Payload `{invalid` -> conteo DLQ aumenta, lote latest permanece |

**Alternativa:** token endpoint responde401; request retorna(401,None), login retornaNone, wait_for reintenta hasta deadline y lanzaRuntimeError (58-65,45-55). Si la solicitud de job retorna503, create_job falla assert y no llega a print PASS (68-72). En modo resilience, `finally` intenta arrancar RabbitMQ incluso si falla la comprobación de queued (117-127); no garantiza recuperación si Compose mismo falla.

## Evidencia histórica separada

`docs/verification.md:3-21` declara tests, scans, builds y checks previos exitosos; `docs/verification.md:37-44` registra el refresco de toolchain del 2026-10-14 y `docs/verification.md:62-70` enumera lo que esos checks **no** establecen (clúster, inferencia real, CSRF, imágenes de infraestructura, firma de imágenes). Se leyeron como contexto, sin repetirlos ni verificar que correspondan exactamente a estos bytes: el propio archivo se declara baseline histórico en la línea 74. No hay log original ni SHA disponible en esta tarea para convertir afirmaciones históricas en evidencia propia.

## Cobertura y siguientes comprobaciones

| Archivo seleccionado | Declaraciones / branches documentados | Estado |
| --- | --- | --- |
| `services/gateway/internal/handler/gateway_test.go:1-1204` | Imports/constantes, dos fakes, un helper de origen y otro de request, 28 tests de cabeceras, CORS, cuotas, correlation ids, métricas y ruteo | Explicado |
| `services/jobs/internal/handler/api_test.go:1-554` | Imports, cinco métodos fake, clave/signer/verifier, 5 roles de status, 3 formas de GET, 16 casos de create y 5 pruebas de replay | Explicado |
| `services/risk/internal/handler/api_test.go:1-527` | Imports, seis métodos fake, harness de tokens, 5 roles de status, 9 casos de latest, 3 de batches, 4 de gate, 15 de bbox y 8 de zonas | Explicado |
| `services/jobs/internal/repository/store_integration_test.go:1-419` | Imports, guardas, timeout/pool/defer, idempotencia, zona almacenada, metadata sintética y tres pruebas de replay | Explicado |
| `services/risk/internal/repository/store_integration_test.go:1-502` | Imports, guardas, deduplicación/orden, referencias permanentes, outbox de fallo, catálogo, bbox, hash, territorio y publicación programada | Explicado |
| `services/risk/internal/service/demo_test.go:1-69` | Imports/base, metadata opcional, nueve callbacks, copia por subtest y assertion booleana | Explicado |
| `services/jobs/internal/messaging/retry_schedule_regression_test.go:1-44` | Cronograma literal de 5 intentos y caso permanente | Explicado |
| `services/jobs/internal/messaging/recovery_boundary_test.go:1-116` | Acknowledger fake, siete casos de despacho y prefijo de métricas | Explicado |
| `scripts/init_env.py:1-18` | Imports/root/list/path/fd, apertura exclusiva, with/loop/write/print, errores locales sin catch | Explicado |
| `scripts/smoke.py:1-168` | Imports/constantes, siete helpers, main, dos callbacks internos, flag/finally, guarda | Explicado |
| Migraciones y prevention (`migrations_test.go` ×2, `migration_registration_test.go`, `state/machine_test.go`, `prevention/internal/handler/api_test.go`) | Solo nombres de test y rangos verificados | Identificado, no releído |
| `services/risk/internal/service/publisher_test.go:1-948` y los `synthetic_scenarios_test.go` de cada servicio | Existen y crecieron; ningún caso descrito aquí | Identificado, no releído |
| Verificación sintética: `scripts/verify_synthetic.py:1-1043`, `scripts/test_verify_synthetic.py:1-412`, `scripts/verify_ci_migration.py:1-514`, `scripts/test_ci_migration.py:1-743`, `scripts/fetch_comunas.py:1-391`, `scripts/test_fetch_comunas.py:1-228` | Herramientas nuevas que el documento nunca listó; solo se verificaron nombres y rangos | Identificado, no releído |

**Conclusión:** los ocho archivos antes parciales siguen explicados por sus unidades y decisiones locales, con rangos contados de nuevo; cero huecos de lectura de esos archivos. Las suites nuevas se listan pero **no** se explican: leer un test no es ejecutarlo, y ninguna fila de esta tabla afirma un resultado observado. No se reclaman internals criptográficos, excepciones internas exhaustivas de urllib/OS, permisos efectivos ni ejecución end-to-end. La prosa no adjudica RED/GREEN: documentación pasiva y ejecución de aplicación no autorizada. Hashes, límites de líneas y enlaces se comprueban como estructura en el [índice](README.md#comprobación-documental-y-límites).

Verificación posterior propuesta, no ejecutada: revisar cobertura negativa de broker/commit, controlar reloj, validar schema HTTP de errores y correlacionar DLQ por mensaje. Requiere autorización aparte; no es invitación a usar credenciales o bases operativas.
