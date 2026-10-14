# Tests y herramientas: expectativas leídas, no ejecución observada

Los tests describen contratos de autorización, idempotencia y orden temporal; los scripts expresan una demostración end-to-end. Esta tarea solo los leyó. No presenta un resultado PASS ni usa evidencia histórica como si se hubiera reproducido.

[Índice e instantánea](README.md) · [Configuración](07-deployment.md)

## Alcance y cobertura estática completada

Se reinspeccionaron completos seis archivos Go de tests y dos scripts Python, con rangos del [registro](README.md). Las declaraciones, inicializadores, casos, assertions y salidas locales se explican abajo. Los ocho archivos pasan de **parcial** a **explicado estáticamente**; no se extiende esa cobertura a internals de bibliotecas, permisos efectivos ni resultados end-to-end. No se ejecutó ningún helper ni se abrió `.env`. Leer una llamada a credenciales no autoriza ejecutarla.

## Tests gateway y handlers

| Archivo / unidades y rango | Preparación / casos | Expectativa y límites |
| --- | --- | --- |
| [gateway_test.go](../../services/gateway/internal/handler/gateway_test.go):1-54 | Imports1-12; fake verifier14-21 devuelve subject admin solo para string valid; test23-54 crea upstream y loop de cuatro solicitudes | Comprueba headers, Bearer, ID sustituido y códigos202/401/404/204; New se reconstruye por caso, no agota bucket compartido |
| [jobs api_test.go](../../services/jobs/internal/handler/api_test.go):1-114 | Imports1-19; fakeStore21-30 incrementa Create; test32-41 genera RSA y verifier estático; tabla42-62; loop63-114 | Admin202; viewer/operator403; sin token/aud/iss/exp/nbf/tipo inválido401; JSON/clave/zona/body400; rechazo no debe llamar Create |
| [risk api_test.go](../../services/risk/internal/handler/api_test.go):1-70 | Imports1-16; fakeStore18-26; RSA/verifier28-37; tabla38-49; loop50-70 | Viewer/admin200; sin lotes404; rol distinto403; zona/param duplicado400; 403 no consulta repositorio; no hay caso sin token |

### Cómo interpretar los fixtures

Jobs construye claims con defaults de audiencia/issuer/expiración/tipo si el caso no los cambia (`services/jobs/internal/handler/api_test.go:65-94`). Firma JWT y asigna headers; genera clave de idempotencia salvo `badKey`; compara status y `store.calls` (líneas 95-111). Fallos de generación de clave, construcción del signer, firma y serialización compacta manejados con `t.Fatal` interrumpen el test o subtest, no simulan una respuesta de aplicación. El error de `json.Marshal` se ignora en línea 82.

Risk firma claims por caso y, si `data=true`, entrega un lote fake con demo/desconocido/current (`services/risk/internal/handler/api_test.go:50-68`). Ese fake no prueba cálculo SQL de frescura, límites/order o GeoJSON; solo habilita la rama HTTP. Algunos errores del signer/marshal se ignoran en el fixture, por lo que no se infiere cobertura de errores criptográficos.

Gateway fake no verifica criptografía: comprueba la integración con interfaz `Verifier`. El upstream afirma headers eliminados y Bearer conservado (`services/gateway/internal/handler/gateway_test.go:24-35`), pero no demuestra por lectura que la versión de la biblioteca se comporte así al ejecutar.

### Gateway: todas las declaraciones y verificaciones

- Paquete/imports conectan contexto, errores, HTTP/httptest, URL, testing y el tipo OIDC; `verifier struct{}` no guarda estado. `Verify` ignora contexto: cualquier string distinto de `valid` devuelve `(nil,error)`; `valid` devuelve un IDToken con subject `admin`, sin claims ni firma (`services/gateway/internal/handler/gateway_test.go:1-21`).
- El callback upstream registra errores **no fatales** si llega cualquier cabecera de identidad/X-Forwarded-For, si Authorization no es exactamente `Bearer valid`, o si request ID sigue siendo `spoofed`; luego responde 202 incluso tras esos errores. La última comparación no exige un ID no vacío ni su formato. `defer upstream.Close()` limpia el servidor de fixture; el error de `url.Parse` se descarta (`services/gateway/internal/handler/gateway_test.go:23-37`).
- La tabla tiene cuatro casos, todos mediante GET: `/v1/jobs` válido → 202, token inválido → 401, `/metrics` → 404 y `/health/live` → 204. Cada iteración fija cuatro cabeceras no confiables y Authorization (para health queda `Bearer `), crea recorder y una instancia `New` nueva. Por ello no comparte cuota entre casos. Un status distinto usa `t.Errorf` y sigue el loop; no hay subtests, comprobación de body, ID de respuesta ni afirmación explícita del número de llamadas upstream (`services/gateway/internal/handler/gateway_test.go:38-54`).

### Jobs: fakes, preparación y catorce casos

`fakeStore.calls` empieza en cero. `Create` lo incrementa y devuelve ID nuevo, zona recibida y estado queued, sin error; no guarda actor/key/trace ni prueba que sean correctos. `Get` devuelve Job cero y `Audit` devuelve nil/nil: satisfacen la interfaz, pero este test solo usa POST. Imports conectan esas estructuras con criptografía, JWT, HTTP, strings y reloj (`services/jobs/internal/handler/api_test.go:1-30`).

La clave RSA de 2048 bits y el signer RS256 se construyen una sola vez. Sus dos errores posibles terminan el test con `t.Fatal`. `StaticKeySet` usa la clave pública local, issuer `https://issuer.test`, audiencia `firewatch-api` y allowlist RS256: el fixture no usa JWKS remoto (`services/jobs/internal/handler/api_test.go:32-41`).

| Caso declarado — `services/jobs/internal/handler/api_test.go:42-62` | Cambio sobre defaults | Código esperado |
| --- | --- | --- |
| admin accepted | role admin | 202 |
| viewer forbidden / unknown role forbidden | viewer / operator | 403 |
| missing token | noToken=true; se firma token pero no se adjunta | 401 |
| wrong audience / wrong issuer | other / otro issuer | 401 |
| expired token / future token | exp hace un minuto / nbf dentro de una hora | 401 |
| id token rejected | typ ID en lugar de Bearer | 401 |
| unknown field / multiple documents | propiedad admin / segundo objeto JSON | 400 |
| unknown zone | zone_id other | 400 |
| missing idempotency | badKey=true **omite** cabecera; no prueba UUID mal formado o Nil | 400 |
| oversized body | 9000 letras dentro de zone_id | 400 |

Cada `t.Run` rellena solo valores vacíos/cero: aud/iss, exp=now+1 minuto, typ=Bearer y body demo. Subject es siempre `admin-subject`, aunque role cambie; nbf queda cero salvo caso futuro. Marshal se ignora, pero Sign/CompactSerialize fallidos terminan el subtest. Content-Type es siempre JSON; se omiten headers según noToken/badKey. Después de invocar handler se exige status; fallo fatal evita la assertion siguiente. Para cualquier caso distinto de 202 se exige calls=0. Para 202 **no** se exige calls=1 ni body/Location correcto. No hay casos 415, error del store, GET/Audit, subject vacío o claims mal tipados (`services/jobs/internal/handler/api_test.go:63-114`).

### Risk: seis casos, no matriz criptográfica completa

`fakeStore` tiene slice de batches y contador; `List` ignora zona/límite/contexto, incrementa calls y retorna la slice sin error. Imports/clave/signer son análogos a jobs; los errores de RSA/NewSigner son fatales. El verifier fija issuer/audiencia mediante clave estática, pero no declara `SupportedSigningAlgs` en este fixture (`services/risk/internal/handler/api_test.go:1-37`).

La tabla cubre viewer/admin con datos → 200, viewer sin datos → 404, role other → 403, zona other y zone_id repetido → 400. Cada subtest firma claims con exp=now+1 minuto y typ Bearer; ignora errores de Marshal, Sign y CompactSerialize (no los convierte en assertions de negocio). Todas las solicitudes son GET latest con token. Si data=true, el fake ofrece un único Batch con tres campos, sin IDs, geometría ni fechas; si false, slice nil. Status inesperado termina el subtest; solo el caso 403 comprueba calls=0. Los 400 no tienen assertion auxiliar sobre persistencia y los 200/404 no verifican cantidad de llamadas, body ni valores de límite. No se prueba history, ausencia de token, 503 o frescura real (`services/risk/internal/handler/api_test.go:38-70`).

## Tests de repositorio y validador

| Archivo / rango | Secuencia y branches | Intención, no resultado |
| --- | --- | --- |
| [jobs integración](../../services/jobs/internal/repository/store_integration_test.go):1-57 | Imports1-12; test14-57; skip15-17 si short o TEST_DATABASE_URL vacía; ctx20s/pool18-25; dos Create26-38; Complete con trace malo39-46; trace corregido y duplicado47-56 | Misma ID y un outbox; rollback deja queued; dos audit al final. Depende de DB migrada desechable; crea datos y no limpia filas |
| [risk integración](../../services/risk/internal/repository/store_integration_test.go):1-87 | Imports1-13; test15-87; skip16-18; ctx/pool19-26; assessment real28-37; Publish y duplicados39-53; List54-57; asserts de nivel/factors/rationale/demo/policy58-86 | Evento duplicado y nuevo ID para mismo job no duplican lote/outbox; solicitud antigua no suplanta nueva; el nivel, los factors y el rationale sobreviven el round trip. Invoca `Store.Publish` directo, sin ValidateRequest |
| [demo_test.go](../../services/risk/internal/service/demo_test.go):1-44 | Imports1-10; TestValidateRequest12-39; base válida13; tabla14-28; copia/muta/valida29-39; allowlist41-56 | Dos válidos (fixture demo y no-demo), y rechaza tipo, zona `other`/vacía, ID inválido/Nil, actor vacío y solicitud futura |

El test risk de integración con OccurredAt cero refuerza la distinción entre precondición del consumidor y método SQL: no prueba que un envelope así atravesaría el broker. Ninguno de estos tests muestra scheduler, modelos reales, tolerancia temporal exacta, reconexión o implementación de DLX.

### Jobs integración: secuencia y salidas completas

Imports habilitan contexto, lectura de **una variable** de entorno, reloj, contrato, UUID y pool; no leen archivos `.env`. La guarda short/URL vacía hace Skip antes de crear pool. Contexto de 20 s y pool tienen limpieza diferida; error de New es fatal (`services/jobs/internal/repository/store_integration_test.go:1-25`).

Actor, key y trace son UUIDs nuevos. Primer Create debe retornar nil error; segundo exige nil error **y** mismo ID (OR de fallo). Query de outbox filtrada por job_id exige Scan sin error y count=1. Cada desviación termina el test, sin seguir la secuencia (`services/jobs/internal/repository/store_integration_test.go:26-38`).

El resultado directo tiene UUIDs evento/batch, actor/zona coincidentes y trace inválido; omite Type, Demo y timestamps. Complete debe fallar y Get debe devolver queued. Se cambia solo trace y se reutiliza **el mismo ID evento**: Complete debe funcionar dos veces. Query final exige exactamente dos audit para J. Esto protege indirectamente rollback de inbox: si el ID fallido quedara deduplicado, la corrección no crearía la segunda audit. No comprueba explícitamente estado published, batch asignado ni número de inbox; tampoco prueba validador AMQP (`services/jobs/internal/repository/store_integration_test.go:39-57`). No hay cleanup SQL: Close no borra filas.

### Risk integración: dos capas de duplicación y orden

Misma guarda, timeout y cleanup de pool que jobs; cualquier error de pool o `Store.Publish` termina el test. `newer` tiene IDs/trace nuevos, actor test, tipo pedido/demo/zona y RequestedAt ahora UTC; OccurredAt queda cero. Publica una vez y repite mismo evento (inbox), luego copia con ID evento nuevo pero mismo JobID (UNIQUE job). Copia `older` cambia ID/JobID y resta una hora a RequestedAt, pero se publica después (`services/risk/internal/repository/store_integration_test.go:1-87`).

List con límite 1 debe retornar nil error, exactamente un elemento y JobID newer; los OR cortocircuitan antes de indexar si hay error o longitud distinta. Query de outbox exige una fila para ese job. No cuenta todos los batches/inbox ni aísla zona frente a datos concurrentes; presupone base desechable sin un trabajo ajeno más reciente. No limpia datos ni usa consumidor/validación de request (`services/risk/internal/repository/store_integration_test.go:45-53`).

### Demo: mutaciones independientes y guardas ausentes

Paquete/imports usan testing/reflect/reloj/contrato/UUID. Base tiene IDs válidos, actor admin, tipo pedido, demo true, zona fija y ambas fechas no cero. La tabla declara nombre, callback de mutación y bool esperado. Nueve entradas: dos válidas (fixture demo y demo=false, «non-demo origin is still well formed»); tipo publicado, zona `other`, zona vacía, ID invalid, ID Nil, actor vacío y RequestedAt ahora+1 h inválidos (`services/risk/internal/service/demo_test.go:12-28`).

Cada subtest **copia** base antes de mutarla, llama ValidateRequest y compara `(err==nil)` con valid; desigualdad es fatal. No compara texto de error ni modifica el fixture del siguiente caso. El reloj no está inyectado: no prueba umbral exacto de un minuto, actor >255 bytes, fechas cero, JobID/TraceID inválidos ni relación temporal entre fechas. Aparte se cubren `SupportedZone` (positivo y negativos) y la allowlist exacta (`services/risk/internal/service/demo_test.go:29-44`).

## init_env.py: creación exclusiva, no rotación

[init_env.py](../../scripts/init_env.py):1-18 es un script de top-level, no función importable sin efectos. Docstring e imports1-4 explican propósito; root6 apunta backend; names7-11 enumera ocho variables locales (no valores). path12 selecciona destino; `os.open` con WRONLY|CREAT|EXCL y modo0600 (13) crea archivo solo si no existe. `fdopen`14-17 escribe comentario y por cada nombre genera `secrets.token_hex(24)`; print18 anuncia creación.

| Condición | Resultado razonado | Límite |
| --- | --- | --- |
| Archivo ausente y permisos válidos | Ocho secretos aleatorios escritos, 48 hex por valor | No se generaron ni leyeron aquí |
| Archivo existente | O_EXCL hace fallar open, sin sobrescribir | Excepción no capturada por script |
| Error después de creación | Context manager cierra stream si ya abierto; posible archivo parcial | No hay rollback de filesystem ni recuperación implementada |

El modo0600 expresa intención POSIX; permisos efectivos de Windows/ACL no se inspeccionaron. El script no rota credenciales ya guardadas en volúmenes ni sincroniza identidades existentes. Nada de esta sección reproduce datos personales o passwords.

No hay función, guarda de entrada, try/except ni rollback. La lista es finita (ocho iteraciones); cada token tiene 24 bytes aleatorios representados por 48 caracteres hex, seguido de newline. `fdopen` usa UTF-8/newline LF y al salir del with cierra/flushea el stream; si fdopen mismo falla, todavía no existe context manager que cierre fd. Resolución de ruta, creación, entropía, write, flush/close o print pueden propagar sus excepciones sin capturar; un fallo tardío puede dejar archivo completo o parcial y la siguiente ejecución no lo sobrescribe por O_EXCL. Print solo se alcanza tras cerrar correctamente, pero no verifica persistencia física con fsync. Son ramas inferidas desde instrucciones locales, no pruebas del OS (`scripts/init_env.py:6-18`).

## smoke.py: helpers y orquestación explícita

[smoke.py](../../scripts/smoke.py):1-150 importa argparse/base64/json/pathlib/subprocess/time/urllib/uuid (1-11), define ROOT y endpoints locales API/IDENTITY (13-15) y solo llama main si `__name__=="__main__"` (149-150). Esa guarda no ejecutó nada durante lectura.

| Unidad / líneas de `scripts/smoke.py` | Entradas / transformación | Salidas / errores |
| --- | --- | --- |
| credentials,18-20 | Lee .env y separa líneas no vacías/no comentario por primer `=` | Dict; no parse robusto de quotes/export; no se invocó |
| request,23-36 | Copia headers; token agrega Bearer; dict body ->JSON/Content-Type; Request; urlopen timeout15 | Status+JSON o None si vacío; HTTPError ->code,None; otros errores no capturados aquí |
| wait_for,39-49 | Deadline monotonic, loop callback, retorno truthy, sleep1 | Captura URLError/TimeoutError/ConnectionError y reintenta; deadline ->RuntimeError; otras excepciones salen |
| token_for,52-59 | Form password grant/client demo; closure login devuelve access_token solo200 | wait_for; depende de realm excluido, no adecuado como flujo browser de producción |
| create_job,62-66 | POST demo con UUID nuevo o key dada | Assert202; devuelve job; error assertion termina |
| await_job,69-73 | GET por id; closure acepta200 y state published | wait_for, no detecta estado failed implementado |
| compose,76-77 | subprocess docker compose con cwd ROOT/checkTrue | Mutación real si ejecutado; no se invocó |
| main,80-106 | Parse flag resilience; credenciales; login; health; auth/zona; idempotencia; espera; latest/history/audit | Assertions sobre stack real; print PASS solo después de ellas, no prueba para esta tarea |
| resilience,108-123 | stop broker, crea queued y lee lote previo; finally start; espera recuperación; restart jobs/risk; verifica persistencia | Provoca interrupciones deliberadas; finally intenta iniciar aun si assertion falla; error externo puede abortar |
| cuarentena,125-146 | Basic auth construida en memoria; consulta DLQ; publica JSON malformed; espera aumento; verifica lote unchanged | Requiere management RabbitMQ; imprime job ID, no tokens/passwords; no observado |

Las assertions de main incluyen 401 sin token, 403 para viewer en POST/audit, 400 zona unknown, mismo ID al reintentar, lote coincidente con batch_id, demo true/risk_level en el conjunto canónico/Point, una ocurrencia de J en history y acciones de audit (`scripts/smoke.py:87-105`). `request` descarta cuerpo HTTPError, por lo que el script verifica principalmente códigos en caminos negativos, no schema completo de errores.

La prueba DLQ compara `messages_ready` anterior/posterior (`scripts/smoke.py:127-144`): en un entorno concurrente, un aumento podría incluir otro mensaje. Es intención razonable para demo aislada, no correlación por ID del mensaje malformed. Tampoco se afirma que assertions sean activas con cualquier modo Python: el archivo usa `assert`, cuyo funcionamiento depende de opciones de ejecución externas.

### Smoke: branches y excepciones de helpers

- Los imports no invocan main. ROOT se deriva del archivo; API/IDENTITY son constantes locales; la guarda final llama main solo cuando el nombre es `__main__`. No existe manejador global de excepciones (`scripts/smoke.py:1-15,149-150`).
- `credentials` incluye solo líneas no vacías y que no comienzan por `#`, divide por primer `=` y forma dict (claves repetidas conservan última entrada). Espacios no se recortan; un comentario con espacios iniciales no se descarta; una línea sin `=` provoca ValueError. Lectura/decodificación fallidas se propagan. Esa operación **no se invocó** aquí (`scripts/smoke.py:18-20`).
- `request` crea una copia de headers (o dict vacío), añade Authorization solo si token es truthy y convierte body a bytes JSON únicamente si es dict; eso sustituye Content-Type. Los bytes de formulario pasan sin conversión. Dentro del try, respuesta no vacía se parsea JSON y vacía retorna None; with cierra respuesta. HTTPError retorna solo code/None. Construcción de Request está antes del try; errores de URL, body, JSON, red o decodificación que no sean HTTPError no se capturan por este helper (`scripts/smoke.py:23-36`).
- `wait_for` evalúa check mientras reloj sea menor que deadline; valor truthy retorna intacto, falsy sigue. Captura únicamente URLError/TimeoutError/ConnectionError y duerme un segundo después de falsy o excepción capturada. Al agotar lanza RuntimeError. El timeout no es un límite duro: callback (por ejemplo HTTP de hasta 15 s) y sleep pueden sobrepasarlo; no hay cancelación de callback. JSONDecodeError, KeyError, TypeError y AssertionError salen inmediatamente (`scripts/smoke.py:39-49`).
- `token_for` codifica formulario con password grant/client fijo. `login` usa 200 para acceder a `data["access_token"]`; otro status devuelve None y reintenta. Un 200 sin objeto/token causa excepción de estructura no capturada por wait_for; access_token vacío sigue esperando. Realm y credenciales siguen siendo fronteras excluidas (`scripts/smoke.py:52-59`).
- `create_job` usa key recibida si truthy, de lo contrario UUID nuevo, POST demo y assert202; retorna job sin validar esquema. `await_job.completed` consulta id del job; status distinto de 200 cortocircuita y reintenta; 200 exige state published, otro estado espera. Body ausente/mal formado con 200 genera error local en lugar de espera; no reconoce failed como terminal (`scripts/smoke.py:62-73`).
- `compose` ejecutaría docker compose con argumentos, cwd backend y check=True. Executable ausente o salida no cero se propagan; no captura stdout/stderr ni añade timeout. No se ejecutó (`scripts/smoke.py:76-77`).

### Smoke: inventario completo de main y callbacks

| Bloque / líneas de `scripts/smoke.py` | Decisiones, assertions y efectos razonados |
| --- | --- |
| 80-87 | Parser con único flag resilience booleano (false por defecto); argumentos inválidos terminan vía argparse. Lee env, accede dos claves y obtiene admin/viewer; espera health204. No verifica readiness SQL de servicios directamente: health es gateway. |
| 88-92 | Assertions: GET jobs sin token401; POST viewer403 (sin idempotencia); audit viewer403; POST admin con zona unknown/UUID400. Cortocircuita y aborta al primer resultado inesperado. |
| 93-100 | UUID K; crea J; assertion hace segundo POST con K y compara ID; espera published; latest viewer exige status200 y batch_id de completion, luego demo true/risk_level en {bajo, medio, alto, desconocido} y geometry.type Point. No comprueba coordenadas, vigencia o model_version. |
| 101-106 | History200 y sum de coincidencias job_id exactamente1; audit200 y conjunto de acciones del job exactamente requested/published. El conjunto no detecta acciones duplicadas. Primer print PASS solo cubre esta fase; puede emitirse antes de un fallo de resiliencia/cuarentena posterior. |
| 108-118 | Si resilience false omite todo bloque; true ejecuta stop broker **antes del try**. Dentro crea queued, exige conservar batch previo y lee state queued sin assertion de status del GET. finally intenta start broker incluso si falló cuerpo del try; si stop falla no se entra a finally, y si start falla puede sustituir excepción previa. Luego await_job y restart jobs/risk. |
| 119-123 | Closure persisted exige status200 y batch_id recovered; otro status retorna false, 200 mal formado propaga error. wait_for espera recuperación tras restart; print de esta fase no implica que cuarentena posterior funcione. |
| 125-132 | Cuarentena ocurre con o sin flag. Basic auth se forma en memoria, GET management de DLQ exige200, messages_ready default0; latest exige200 antes de enviar mensaje. |
| 133-142 | POST management publica payload inválido `{invalid`, delivery_mode2 y clave pedido; exige200 y routed truthy. Closure quarantined consulta DLQ: status no200 →false; 200 exige conteo mayor al inicial, default0; wait_for usa30 s nominales. |
| 143-146 | Latest debe seguir200 y mismo ID que last_valid capturado inmediatamente antes de envío (también funciona tras fase resilience). Imprime PASS cuarentena y ID del **job inicial**, no del recovered; no imprime tokens/passwords. |

Todos los accesos a dict/list, comprensiones, comparaciones y calls fallidas sin catch se propagan hasta proceso principal. No hay cleanup global de trabajos/lotes ni garantía de restaurar servicios si una interrupción ocurre fuera del try/finally. Con Python optimizado se omiten assertions **y sus expresiones**: también desaparecen las llamadas HTTP de líneas 88-92, el segundo Create de95 y el GET de114; no solo se pierden comprobaciones. Esta observación estática explica por qué leer prints PASS no certifica un recorrido uniforme (`scripts/smoke.py:88-146`).

## Traza razonada y alternativa

Supuestos: script ejecutado hipotéticamente sin optimización, stack local sano, cuentas configuradas y DB limpia; esta tarea **no lo ejecutó**.

| Paso | Cita | Valores antes -> después |
| --- | --- | --- |
| Login | `scripts/smoke.py:52-59,84-86` | Respuesta200 con access_token -> tokens locales en memoria |
| Comando | `scripts/smoke.py:93-96` | Clave K -> J; repetición K ->mismo J; wait_for espera published |
| Lectura | `scripts/smoke.py:97-105` | B coincide con finalización, history una fila J, audit dos acciones |
| Cuarentena | `scripts/smoke.py:133-144` | Payload `{invalid` -> conteo DLQ aumenta, lote latest permanece |

**Alternativa:** token endpoint responde401; request retorna(401,None), login retornaNone, wait_for reintenta hasta deadline y lanzaRuntimeError (52-59,39-49). Si la solicitud de job retorna503, create_job falla assert y no llega a print PASS (62-66). En modo resilience, `finally` intenta arrancar RabbitMQ incluso si falla la comprobación de queued (108-116); no garantiza recuperación si Compose mismo falla.

## Evidencia histórica separada

`docs/verification.md:3-21` declara tests, scans, builds y checks previos exitosos. Se leyó como contexto, sin repetirlos ni verificar que correspondan exactamente a estos bytes. `docs/verification.md:37-45` declara límites de producción, seguridad e inferencia; esta colección conserva esa separación. No hay log original ni SHA disponible en esta tarea para convertir afirmaciones históricas en evidencia propia.

## Cobertura y siguientes comprobaciones

| Archivo seleccionado | Declaraciones / branches documentados | Estado |
| --- | --- | --- |
| `services/gateway/internal/handler/gateway_test.go:1-54` | Imports, fake/verificación, upstream/assertions, defer, tabla de cuatro entradas, loop/status | Explicado |
| `services/jobs/internal/handler/api_test.go:1-114` | Imports, tres métodos fake, clave/signer/verifier, catorce casos, defaults, firma, headers y assertions | Explicado |
| `services/risk/internal/handler/api_test.go:1-70` | Imports/struct/List, clave/signer/verifier, seis casos, claims, datos opcionales y assertions | Explicado |
| `services/jobs/internal/repository/store_integration_test.go:1-57` | Imports, Skip, timeout/pool/defer, Create repetido, counts, error trace, corrección/redelivery | Explicado |
| `services/risk/internal/repository/store_integration_test.go:1-53` | Imports, Skip, timeout/pool/defer, duplicados ID/job, solicitud antigua, List/count | Explicado |
| `services/risk/internal/service/demo_test.go:1-44` | Imports/base, nueve callbacks, copia por subtest, validación y assertion booleana; pruebas de zona y allowlist | Explicado |
| `scripts/init_env.py:1-18` | Imports/root/list/path/fd, apertura exclusiva, with/loop/write/print, errores locales sin catch | Explicado |
| `scripts/smoke.py:1-150` | Imports/constantes, siete helpers, main, cuatro callbacks internos, assertions, flag/finally, guarda | Explicado |

**Conclusión:** 8/8 archivos antes parciales ahora explicados por sus unidades y decisiones locales; cero huecos de lectura de esos archivos. No se reclaman internals criptográficos, excepciones internas exhaustivas de urllib/OS, permisos efectivos ni ejecución end-to-end. La prosa no adjudica RED/GREEN: documentación pasiva y ejecución de aplicación no autorizada. Hashes, límites de líneas y enlaces se comprueban como estructura en el [índice](README.md#comprobación-documental-y-límites).

Verificación posterior propuesta, no ejecutada: revisar cobertura negativa de broker/commit, controlar reloj, validar schema HTTP de errores y correlacionar DLQ por mensaje. Requiere autorización aparte; no es invitación a usar credenciales o bases operativas.
