# Gateway: filtrar y reenviar sin decidir el negocio

**Re-derived on 2026-10-20 against the code of commit `267a4bb`.**

El gateway genera correlación, aplica tres capas de cupo locales, verifica el token y envía rutas públicas a `jobs`, `risk` o `prevention`. No consulta bases de datos ni interpreta roles. La autorización final pertenece a cada servicio.

[Índice e instantánea](README.md) · [Siguiente: jobs](02-jobs.md)

## Alcance y conexiones

Lectura completa: [main.go](../../services/gateway/cmd/server/main.go), líneas 1-63, y [gateway.go](../../services/gateway/internal/handler/gateway.go), líneas 1-237. El test, leído como contrato, se describe en [tests](08-tests-tools.md). Fecha, ausencia de Git, hashes SHA-256 y dependencias no inspeccionadas: [registro compartido](README.md). No se ejecutó código de aplicación.

| Unidad | Entrada / responsabilidad | Dependencias / estado |
| --- | --- | --- |
| `main`, `run` — `services/gateway/cmd/server/main.go:18-63` | Proceso, configuración, servidor y apagado | `os.Getenv`, OIDC remoto, contexto de señales |
| `Verifier` — `services/gateway/internal/handler/gateway.go:39-41` | Contrato inyectable `Verify(ctx, token)` | Implementación OIDC en `main`, fake en tests |
| Constantes de cupo — `services/gateway/internal/handler/gateway.go:20-37` | Rate y burst de las tres capas; tope e inactividad de sujetos | Valores fijos en el binario, sin configuración externa |
| `New` — `services/gateway/internal/handler/gateway.go:43-198` | Construye mux, proxies, tres buckets y el handler | `url.URL`, transportes HTTP, mutex y buckets privados de cada instancia |
| `randomID`, `failure`, `rateLimited`, `rejectUnauthenticated` — `services/gateway/internal/handler/gateway.go:200-237` | ID aleatorio y salidas JSON de error | Entropía del sistema / `ResponseWriter` / bucket anónimo |

Los imports de cada archivo (líneas 1-18 de `gateway.go` y 1-16 de `main.go`) conectan utilidades estándar y OIDC; no introducen estado global de negocio. La función `New` devuelve una closure que conserva los proxies y los tres buckets entre solicitudes; el envoltorio `statusRecorder` (`services/gateway/internal/handler/recorder.go:1-29`) captura el status para `/metrics` y expone `Unwrap` para que el proxy siga pudiendo hacer flush.

## Arranque y terminación

`main` instala logs JSON a stdout y, si `run` devuelve error, registra `server_stopped` y termina con código 1 (`services/gateway/cmd/server/main.go:18-24`). Un `panic` no pasa por esa rama de retorno.

| Unidad / condición | Transformación | Resultado / error |
| --- | --- | --- |
| Contexto — `services/gateway/cmd/server/main.go:27-28` | Escucha interrupción y SIGTERM; `stop` diferido | Cancela servidor y contexto de claves |
| `required` — `services/gateway/cmd/server/main.go:29-35` | Obtiene variable; vacía implica `panic` con nombre de clave | No devuelve error recuperable |
| `parse` — `services/gateway/cmd/server/main.go:36-42` | Exige URL parseable, host y esquema HTTP/HTTPS | URL válida o `panic` |
| OIDC — `services/gateway/cmd/server/main.go:43-48` | Fuera de development exige HTTPS para issuer y JWKS; audiencia obligatoria; algoritmo RS256 | Error devuelto si HTTP no permitido; cliente remoto con timeout 5 s |
| `WEB_ORIGIN` — `services/gateway/cmd/server/main.go:49-50` | Lee el origen opcional y lo entrega a `handler.New` | Vacío desactiva CORS por completo |
| Servidor — `services/gateway/cmd/server/main.go:51-62` | Puerto 8080; límites cabeceras 5 s, lectura 10 s, escritura 15 s, idle 60 s, cabeceras 16384 bytes | `ErrServerClosed` se convierte a `nil`; otros errores se propagan |
| Apagado — `services/gateway/cmd/server/main.go:52-57` | Goroutine espera cancelación; `Shutdown` con 10 s | Error de apagado ignorado explícitamente |

No hay discovery automático en el código: issuer, JWKS y audiencia se entregan a `oidc.NewVerifier`. La validación interna de firma/expiración de esa biblioteca es una dependencia externa, no fuente inspeccionada aquí.

## Proxy y secuencia de filtros

`New` crea un `ServeMux` y una fábrica de `ReverseProxy`. Cada proxy llama `SetURL(target)` y elimina `X-User-ID`/`X-User-Role` de la petición saliente (`services/gateway/internal/handler/gateway.go:45-52`). El código no llama `SetXForwarded`. La eliminación habitual de cabeceras de forwarding por `ReverseProxy` pertenece a la biblioteca estándar; el test expresa esa expectativa, no una implementación local adicional. El mismo proxy descarta en `ModifyResponse` el `X-Request-ID` que devuelve el upstream (`services/gateway/internal/handler/gateway.go:60-63`), de modo que la cabecera expuesta conserva un solo valor.

Las rutas `/v1/jobs`, `/v1/jobs/` y `/v1/audit` usan `jobs`; `/v1/risk/`, `/v1/models` y `/v1/models/` usan `risk`; `/v1/prevention/` usa `prevention` (`services/gateway/internal/handler/gateway.go:68-83`). El mux decide rutas, mientras el upstream decide métodos y endpoints. El transporte limita espera de cabeceras a 10 s y conexiones inactivas a 60 s; fallo de transporte produce `502 upstream_unavailable` (`services/gateway/internal/handler/gateway.go:64-65`).

| Orden / rama y líneas de `services/gateway/internal/handler/gateway.go` | Entrada / decisión | Efecto y propagación |
| --- | --- | --- |
| 96-127: inicialización, cabeceras y log | Envoltorio `statusRecorder`; ID nuevo por solicitud | Sobrescribe `X-Request-ID` entrante y lo escribe en petición y respuesta; `nosniff`, `no-store`, CSP; log diferido con método y duración, y métrica por clase de status |
| 128-140: CORS | `webOrigin` no vacío y `Origin` idéntico al configurado | `Allow-Origin` con ese origen, `Expose-Headers: X-Request-ID`, `Vary: Origin`; falla cerrado y nunca refleja un origen no validado |
| 141-152: preflight `OPTIONS` | Mismo origen y método `OPTIONS`, antes del cupo y del Bearer | 204 con `Allow-Methods GET, POST, OPTIONS`, `Allow-Headers Authorization, Content-Type, Idempotency-Key` y `Max-Age: 600`; no gasta token ni exige `Authorization` |
| 153-156: health | Path exactamente live o ready | 204 sin JWT, sin gasto de token; no comprueba upstream ni dependencias; no restringe método |
| 157-166: `/metrics` | Path exactamente `/metrics`, antes de cupo y Bearer | 200 `text/plain; version=0.0.4` con contadores HTTP y señales de proceso; el scrape no consume cupo |
| 167-172: cupo agregado | Bucket 20/s con burst 40, mutex | Si hay al menos 1, resta 1; si no, 429 `rate_limited` con `Retry-After: 1`; es pre-autenticación y por instancia |
| 173-176: fuera de `/v1/` | Prefijo incorrecto, después de gastar cuota agregada | 404 JSON |
| 177-181: Authorization | `strings.Fields`, dos piezas, esquema Bearer con `EqualFold` | 401 vía `rejectUnauthenticated`, que antes cobra el bucket anónimo |
| 182-188: verificación | Timeout 5 s; cancela tras `Verify`; error o subject vacío | 401; no lee `realm_access`, `typ` ni `nbf` de forma local |
| 189-194: cupo por subject | Bucket con clave = subject verificado: 5/s, burst 10, 4096 sujetos, evicción tras 5 min inactivo | 429 `rate_limited`; solo puede cobrarse después de autenticar |
| 195-196: envío | Token aceptado y cupos cobrados | Envuelve body con límite 8192 bytes y llama mux; el límite se hace efectivo al leer body, no es rechazo anticipado por longitud |
| 200-214: entropía | 16 bytes desde `rand.Read`, con los bits de versión 4 y de variante de RFC 9562 | Devuelve bytes; el emisor los formatea como UUID canónico con guiones; fallo provoca `panic` |
| 215-219: error | Status y código | JSON con ID de respuesta; error del encoder ignorado |
| 221-226: 429 | `rateLimited` | `Retry-After: 1` y cuerpo `rate_limited` |
| 228-237: anónimo | Bucket compartido 1/s, burst 5; sin IP, `RemoteAddr` ni proxy de confianza | 401 `unauthorized`; agotado, 429 `rate_limited` |

El log diferido no incluye status HTTP ni path: el status solo alimenta la métrica. El cupo agregado y el anónimo no son por usuario/IP ni distribuidos; dos réplicas tienen buckets distintos. El cupo por subject sí distingue un sujeto verificado, nunca una IP ni una cabecera de proxy. Una ruta `/v1/` sin registro puede llegar al comportamiento por defecto del mux después de autenticar, por lo que no todos los 404 son producidos por `failure`.

## Contrato implementado e intención

**Implementado:** forwarding del Bearer original, no creación de cabeceras de identidad confiables; subject no vacío requerido; autorización por rol delegada. El cupo agregado se aplica antes de JWT y puede rechazar incluso solicitudes inválidas con 429; después de autenticar, el cupo por subject y el anónimo pueden hacer lo mismo (`services/gateway/internal/handler/gateway.go:167-194`).

**Intención leída:** `TestGatewayFiltersIdentityHeadersAndInternalRoutes` espera 202 para token fake válido, 401 para inválido, 200 para `/metrics` y 204 para health; comprueba eliminación de identidad/forwarding y sustitución del ID (`services/gateway/internal/handler/gateway_test.go:39-70`). No se ejecutó. El archivo creció a 1204 líneas y hoy cubre además preflight, CORS, cupos, `/metrics` y rutas (`services/gateway/internal/handler/gateway_test.go:1-1204`); sigue sin cubrir errores de entropía ni el arranque.

**Frontera no resuelta:** `Verifier` debe respetar el contrato de devolver un token no nulo cuando `err==nil`; el handler accede a `token.Subject` sin guard independiente. No se afirma fallo de la implementación real ni se probó ese fake adverso.

## Traza razonada normal y alternativa

Supuestos: cupo agregado con 40 tokens, subject nuevo con 10, Bearer aceptado por OIDC, `jobs` responde 202. Entrada `POST /v1/jobs` con JSON demo e Idempotency-Key UUID.

| Paso | Cita | Valores / decisión | Resultado |
| --- | --- | --- | --- |
| 1 | `services/gateway/internal/handler/gateway.go:109-115` | ID entrante arbitrario | ID aleatorio `G` reemplaza entrada y respuesta |
| 2 | `services/gateway/internal/handler/gateway.go:167-172` | 40 disponibles | 39 después de aceptar, sin contar refill ilustrativo |
| 3 | `services/gateway/internal/handler/gateway.go:177-188` | Dos partes; token con subject | Verificación aceptada |
| 4 | `services/gateway/internal/handler/gateway.go:45-52,68-70,191-196` | Ruta jobs; 10 disponibles para el subject | Proxy elimina identidad declarada, cobra 1 del subject, conserva token y envía al upstream |
| 5 | `services/jobs/internal/handler/api.go:156-168` | Upstream recibe correlación | Normaliza UUID y devuelve 202/Location; [continuación](02-jobs.md) |

El ID `G` es un UUID canónico con guiones (versión 4, variante de RFC 9562) y es el **mismo literal** en las tres capas: jobs y risk lo reciben, lo pasan por `uuid.Parse` y lo reescriben con `String()`, y el gateway descarta el valor del upstream para que la cabecera expuesta tenga **un solo** valor. La conservación byte a byte **sí** se garantiza desde ese arreglo: `encodeHex` de `google/uuid` v1.6.0 formatea los 16 bytes sin re-enmascarar versión ni variante (leído en el fuente del módulo, no ejecutado contra los servicios en marcha), así que el eco del servicio es idéntico. Antes del arreglo la conservación no se garantizaba y la cabecera llegaba al navegador con los dos formatos unidos por `", "`.

**Alternativa:** con el cupo agregado agotado, el paso 2 termina en 429 sin upstream; con token inválido termina en 401, o en 429 si el bucket anónimo ya está gastado; con el cupo por subject agotado, el paso 4 termina en 429; con upstream inaccesible tras verificación, `ErrorHandler` devuelve 502 (`services/gateway/internal/handler/gateway.go:65,167-194`). El log diferido corre en esas salidas normales; un fallo de entropía ocurre antes de instalarlo.

## Cobertura y límites

| Archivo / unidades | Bloques inventariados | Estado |
| --- | --- | --- |
| `main.go:1-63` | Imports; main; señales; closures required/parse; OIDC y HTTPS; `WEB_ORIGIN`; servidor; goroutine; retorno | Explicado |
| `gateway.go:1-237` | Imports; constantes de cupo; interfaz; fábrica/proxy/rewrite/`ModifyResponse`; rutas; tres buckets; handler (CORS, preflight, health, `/metrics`, cupos, guards); ID; serialización y salidas de error | Explicado |

No se observaron respuestas reales, claves remotas, distribución de carga ni semántica completa de `ReverseProxy`. La suite leída, no ejecutada, ya ejercita cupos, rutas desconocidas, preflight y fallos de upstream (`services/gateway/internal/handler/gateway_test.go:1-1204`); el envoltorio `statusRecorder` se cita por su efecto sobre la métrica y no se explica línea por línea.
