# Gateway: filtrar y reenviar sin decidir el negocio

El gateway genera correlación, aplica un límite agregado local, verifica el token y envía rutas públicas a `jobs` o `risk`. No consulta bases de datos ni interpreta roles. La autorización final pertenece a cada servicio.

[Índice e instantánea](README.md) · [Siguiente: jobs](02-jobs.md)

## Alcance y conexiones

Lectura completa: [main.go](../../services/gateway/cmd/server/main.go), líneas 1-61, y [gateway.go](../../services/gateway/internal/handler/gateway.go), líneas 1-105. El test, leído como contrato, se describe en [tests](08-tests-tools.md). Fecha, ausencia de Git, hashes SHA-256 y dependencias no inspeccionadas: [registro compartido](README.md). No se ejecutó código de aplicación.

| Unidad | Entrada / responsabilidad | Dependencias / estado |
| --- | --- | --- |
| `main`, `run` — `services/gateway/cmd/server/main.go:18-61` | Proceso, configuración, servidor y apagado | `os.Getenv`, OIDC remoto, contexto de señales |
| `Verifier` — `services/gateway/internal/handler/gateway.go:19-21` | Contrato inyectable `Verify(ctx, token)` | Implementación OIDC en `main`, fake en tests |
| `New` — `services/gateway/internal/handler/gateway.go:23-92` | Construye handler y proxies | `url.URL`, transportes HTTP, mutex y bucket privados de cada instancia |
| `randomID`, `failure` — `services/gateway/internal/handler/gateway.go:94-105` | ID aleatorio y errores JSON | Entropía del sistema / `ResponseWriter` |

Los imports de cada archivo (líneas 1-17 de `gateway.go` y 1-16 de `main.go`) conectan utilidades estándar y OIDC; no introducen estado global de negocio. La función `New` devuelve una closure que conserva los proxies y el bucket entre solicitudes.

## Arranque y terminación

`main` instala logs JSON a stdout y, si `run` devuelve error, registra `server_stopped` y termina con código 1 (`services/gateway/cmd/server/main.go:18-24`). Un `panic` no pasa por esa rama de retorno.

| Unidad / condición | Transformación | Resultado / error |
| --- | --- | --- |
| Contexto — `services/gateway/cmd/server/main.go:27-28` | Escucha interrupción y SIGTERM; `stop` diferido | Cancela servidor y contexto de claves |
| `required` — `services/gateway/cmd/server/main.go:29-35` | Obtiene variable; vacía implica `panic` con nombre de clave | No devuelve error recuperable |
| `parse` — `services/gateway/cmd/server/main.go:36-42` | Exige URL parseable, host y esquema HTTP/HTTPS | URL válida o `panic` |
| OIDC — `services/gateway/cmd/server/main.go:43-48` | Fuera de development exige HTTPS para issuer y JWKS; audiencia obligatoria; algoritmo RS256 | Error devuelto si HTTP no permitido; cliente remoto con timeout 5 s |
| Servidor — `services/gateway/cmd/server/main.go:49-60` | Puerto 8080; límites cabeceras 5 s, lectura 10 s, escritura 15 s, idle 60 s, cabeceras 16384 bytes | `ErrServerClosed` se convierte a `nil`; otros errores se propagan |
| Apagado — `services/gateway/cmd/server/main.go:50-55` | Goroutine espera cancelación; `Shutdown` con 10 s | Error de apagado ignorado explícitamente |

No hay discovery automático en el código: issuer, JWKS y audiencia se entregan a `oidc.NewVerifier`. La validación interna de firma/expiración de esa biblioteca es una dependencia externa, no fuente inspeccionada aquí.

## Proxy y secuencia de filtros

`New` crea un `ServeMux` y una fábrica de `ReverseProxy`. Cada proxy llama `SetURL(target)` y elimina `X-User-ID`/`X-User-Role` de la petición saliente (`services/gateway/internal/handler/gateway.go:23-36`). El código no llama `SetXForwarded`. La eliminación habitual de cabeceras de forwarding por `ReverseProxy` pertenece a la biblioteca estándar; el test expresa esa expectativa, no una implementación local adicional.

Las rutas `/v1/jobs`, `/v1/jobs/` y `/v1/audit` usan `jobs`; `/v1/risk/` usa `risk` (`services/gateway/internal/handler/gateway.go:37-40`). El mux decide rutas, mientras el upstream decide métodos y endpoints. El transporte limita espera de cabeceras a 10 s y conexiones inactivas a 60 s; fallo de transporte produce `502 upstream_unavailable` (líneas 33-34).

| Orden / rama y líneas de `services/gateway/internal/handler/gateway.go` | Entrada / decisión | Efecto y propagación |
| --- | --- | --- |
| 42-54: inicialización y cabeceras | Bucket inicial 40; reloj actual; ID nuevo por solicitud | Sobrescribe `X-Request-ID` entrante; devuelve ID, `nosniff`, `no-store`, CSP; log diferido de método y duración |
| 55-58: health | Path exactamente live o ready | 204 sin JWT, sin gasto de token; no comprueba upstream ni dependencias; no restringe método |
| 59-67: bucket | Mutex; `tokens=min(40,tokens+segundos*20)`; `last=now` | Si hay al menos 1, resta 1; unlock antes de responder; compartido solo por instancia |
| 68-72: agotado | `allowed=false` | 429 `rate_limited`, `Retry-After: 1`; no verificación JWT |
| 73-76: fuera de `/v1/` | Prefijo incorrecto, después de gastar cuota | 404 JSON; `/metrics` no es público |
| 77-81: Authorization | `strings.Fields`, dos piezas, esquema Bearer sin distinguir mayúsculas | 401 si falta o está mal formado |
| 82-88: verificación | Timeout 5 s; cancela tras `Verify`; error o subject vacío | 401; no lee `realm_access`, `typ` ni `nbf` de forma local |
| 89-91: envío | Token aceptado | Envuelve body con límite 8192 bytes y llama mux; el límite se hace efectivo al leer body, no es rechazo anticipado por longitud |
| 94-100: entropía | 16 bytes desde `rand.Read` | Devuelve bytes, luego formato hexadecimal de 32 caracteres; fallo provoca `panic` |
| 101-105: error | Status y código | JSON con ID de respuesta; error del encoder ignorado |

El log diferido no incluye status HTTP ni path. El límite no es por usuario/IP ni distribuido; dos réplicas tienen buckets distintos. Una ruta `/v1/` sin registro puede llegar al comportamiento por defecto del mux después de autenticar, por lo que no todos los 404 son producidos por `failure`.

## Contrato implementado e intención

**Implementado:** forwarding del Bearer original, no creación de cabeceras de identidad confiables; subject no vacío requerido; autorización por rol delegada. El bucket se aplica antes de JWT y puede rechazar incluso solicitudes inválidas con 429 (`services/gateway/internal/handler/gateway.go:59-90`).

**Intención leída:** `TestGatewayFiltersIdentityHeadersAndInternalRoutes` espera 202 para token fake válido, 401 para inválido, 404 para métricas y 204 para health; comprueba eliminación de identidad/forwarding y sustitución del ID (`services/gateway/internal/handler/gateway_test.go:23-54`). No se ejecutó. No cubre saturación, errores de entropía, arranque ni todas las propiedades de la biblioteca OIDC.

**Frontera no resuelta:** `Verifier` debe respetar el contrato de devolver un token no nulo cuando `err==nil`; el handler accede a `token.Subject` sin guard independiente. No se afirma fallo de la implementación real ni se probó ese fake adverso.

## Traza razonada normal y alternativa

Supuestos: bucket con 40 tokens, Bearer aceptado por OIDC, `jobs` responde 202. Entrada `POST /v1/jobs` con JSON demo e Idempotency-Key UUID.

| Paso | Cita | Valores / decisión | Resultado |
| --- | --- | --- | --- |
| 1 | `services/gateway/internal/handler/gateway.go:45-54` | ID entrante arbitrario | ID aleatorio `G` reemplaza entrada y respuesta |
| 2 | `services/gateway/internal/handler/gateway.go:59-67` | 40 disponibles | 39 después de aceptar, sin contar refill ilustrativo |
| 3 | `services/gateway/internal/handler/gateway.go:77-88` | Dos partes; token con subject | Verificación aceptada |
| 4 | `services/gateway/internal/handler/gateway.go:25-40,89-91` | Ruta jobs | Proxy elimina identidad declarada, conserva token y envía al upstream |
| 5 | `services/jobs/internal/handler/api.go:45-57` | Upstream recibe correlación | Normaliza UUID y devuelve 202/Location; [continuación](02-jobs.md) |

El ID `G` de 32 dígitos hexadecimales es tratado como entrada a `uuid.Parse` por el servicio; el comportamiento preciso del parseador externo no se ejecutó aquí. Si no lo acepta, el servicio genera un UUID nuevo. No se garantiza conservación byte a byte del formato entre capas.

**Alternativa:** con `tokens<1`, el paso 2 termina en 429 sin upstream; con token inválido termina en 401; con upstream inaccesible tras verificación, `ErrorHandler` devuelve 502 (`services/gateway/internal/handler/gateway.go:34,68-88`). El log diferido corre en esas salidas normales; un fallo de entropía ocurre antes de instalarlo.

## Cobertura y límites

| Archivo / unidades | Bloques inventariados | Estado |
| --- | --- | --- |
| `main.go:1-61` | Imports; main; señales; closures required/parse; HTTPS; verifier; servidor; goroutine; retorno | Explicado |
| `gateway.go:1-105` | Imports; interfaz; fábrica/proxy/rewrite/error handler; rutas; bucket; handler y guards; ID; serialización | Explicado |

No se observaron respuestas reales, claves remotas, distribución de carga ni semántica completa de `ReverseProxy`. Una futura comprobación podría ejercitar quota, rutas desconocidas y fallos de upstream; está propuesta, no ejecutada.
