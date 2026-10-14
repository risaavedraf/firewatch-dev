# Configuración y despliegue: arranque acotado, entorno aún por verificar

El código define tres ejecutables Go independientes; Compose conecta la demostración local y Kubernetes ofrece una base staging con placeholders. Los archivos describen cómo arrancar, no demuestran que un entorno esté desplegado o sea seguro en producción.

[Índice e instantánea](README.md) · [Persistencia](06-database.md) · [Siguiente: tests y herramientas](08-tests-tools.md)

## Alcance y fronteras

Todos los rangos completos están en el [registro](README.md): ambos main jobs/risk:1-141; tres Dockerfile:1-16/17/17; cuatro go.mod:1-10/23/23/3; go.work:1-8; Makefile:1-19; .dockerignore:1-8; Compose:1-192; seis YAML Kubernetes:1-17/22/9/6/69/174; Prometheus:1-12; plugins:1-1. Lectura directa, sin renders, builds, tests ni conexión a infraestructura. `.env` y variantes, `go.sum` y realm de cuentas excluidos; no se resuelven variables sensibles. Identidad por hashes de los bytes leídos, no revisión Git: [índice](README.md).

## Main jobs/risk: unidades y ramas compartidas

Los imports de ambos `cmd/server/main.go:1-23` conectan módulos propios, migraciones, OIDC, UUID, pgxpool, atomic y HTTP. La diferencia funcional es `Handle: store.Complete` en jobs frente a `Handle: publisher.Publish` en risk. Las citas siguientes aplican a **ambos archivos**: `services/jobs/cmd/server/main.go` y `services/risk/cmd/server/main.go`.

| Unidad / líneas | Secuencia y ramas | Efecto / error |
| --- | --- | --- |
| main, 25-31 | Logs JSON; run | Error -> service_stopped + exit 1 |
| run/pool, 33-47 | Contexto señales; parse DATABASE_URL; max 10/min1; connect 5 s; pool con cierre diferido | Errores configuración/setup genéricos; conexión efectiva no se demuestra al construir pool |
| migrate, 48-75 | Rama exacta de argumento; lock/versionado/commit | Explicada en [persistencia](06-database.md); retorna antes de broker |
| configuración runtime, 76-88 | DB/AMQP no vacíos; issuer/JWKS con host y HTTPS salvo development HTTP; audiencia no vacía | Retorna errores antes de iniciar worker si inválido |
| verifier, 89-90 | Cliente HTTP 5 s; RemoteKeySet; NewVerifier con RS256 y audiencia | Validación delegada a OIDC; no discovery local |
| store/worker, 91-95 | Construye Store y Broker; goroutine Run; workerDone con cierre diferido; defer stop y espera | Espera worker al salir; no propaga error concreto de sesión a run |
| mux/health, 96-107 | /v1 handler protegido; GET live 204; ready Ping con 1 s | Ping falla ->503; éxito ->204; no consulta broker para readiness |
| contadores/metrics, 108-116 | atomic requests; ready=0 o1 según Broker.Ready; imprime cinco métricas | Errores Fprintf ignorados; métricas por proceso, no persistidas |
| outer, 117-128 | Incrementa contador para toda petición; parse ID o nuevo; actualiza ambas cabeceras; contexto10 s | ID válido incluye forma aceptada por uuid.Parse; no exige no Nil; no log HTTP por solicitud aquí |
| server/shutdown, 129-140 | 8080; límites 5/10/15/60 s y 16384 bytes; goroutine espera señales, Shutdown10 s | Error shutdown ignorado; ErrServerClosed ->nil; otros errors devueltos |

El contexto de 10 s del wrapper no mata mágicamente un handler que ignore cancelación; lo entrega a llamadas cooperativas, entre ellas SQL. La disponibilidad HTTP y la capacidad de procesar mensajes se separan: el ready solo comprueba DB y el broker tiene métrica independiente. Si ListenAndServe falla antes de recibir señal, el defer stop cancela worker; la goroutine de shutdown observa esa cancelación. No se prueba el comportamiento temporal real del cierre.

## Configuración esperada por el código

| Variables / consumidor | Validación local | Alcance |
| --- | --- | --- |
| OIDC_ISSUER, OIDC_JWKS_URL, OIDC_AUDIENCE / todos | URL/host/esquema; HTTPS fuera development; audiencia requerida | Biblioteca comprueba token con RS256 configurado; realm real no inspeccionado |
| JOBS_URL, RISK_URL / gateway | `required` y URL HTTP/HTTPS, panic si inválida | HTTP interno permitido incluso fuera development |
| DATABASE_URL / jobs/risk | pgxpool.ParseConfig; no vacío en runtime | TLS DB no exigido explícitamente por run; depende de URL suministrada |
| AMQP_URL / jobs/risk | No vacío antes de worker | No valida amqps en run; Dial y reconexión gestionan errores |
| APP_ENV / todos | Solo string development permite OIDC HTTP | No es selección automática de infraestructura ni garantía de producción |

Citas: `services/gateway/cmd/server/main.go:29-49` y ambos main jobs/risk:36-90. No se reproduce ningún valor de credencial.

## Módulos, workspace e imágenes

| Archivo / rango | Declaraciones y efecto |
| --- | --- |
| [go.work](../../go.work):1-8 | Go 1.26.6 y cuatro módulos; contratos + gateway + jobs + risk. No módulo raíz ejecutable |
| [gateway/go.mod](../../services/gateway/go.mod):1-10 | firewatch.local/gateway, Go1.26.6, OIDC3.17.0; jose4.1.4/oauth2 indirectos |
| [jobs/go.mod](../../services/jobs/go.mod):1-23; [risk/go.mod](../../services/risk/go.mod):1-23 | Módulos propios; contracts0.0.0, OIDC3.17.0, jose4.1.4, uuid1.6.0, pgx5.9.2, amqp1.13.0; bloque indirecto de soporte; replace local ../../contracts/gen |
| [contracts/gen/go.mod](../../contracts/gen/go.mod):1-3 | firewatch.local/contracts y Go1.26.6, sin dependencias declaradas |
| [.dockerignore](../../.dockerignore):1-8 | Excluye .env, anidados, bin, coverage.out, go.work*, .git, deploy y docs del contexto; no es escáner universal de secretos ni excluye todo archivo llamado .env.example |

Los go.sum inventariados no se analizaron: no se acredita integridad ni disponibilidad de módulos. La selección automática de toolchain está fuera de la ejecución autorizada.

Los [Dockerfile gateway](../../services/gateway/Dockerfile):1-16, [jobs](../../services/jobs/Dockerfile):1-17 y [risk](../../services/risk/Dockerfile):1-17 usan el mismo patrón:

1. Build desde Go1.26.6-bookworm con digest; WORKDIR de módulo; `GOWORK=off`, `CGO_ENABLED=0` (gateway:1-3; jobs/risk:1-3).
2. Jobs/risk copian contracts/gen antes de go.mod/go.sum para resolver replace (jobs/risk:4-6). Gateway copia solo su módulo (gateway:4-5). `go mod download && go mod verify` es instrucción de build leída, no ejecutada aquí.
3. Copian código y compilan `/server` con trimpath y `-s -w` (gateway:6-7; jobs/risk:7-8).
4. Target `test` hereda build y usa ENTRYPOINT `go test -count=1 ./...` (gateway:9-10; jobs/risk:10-11). No implica tests en la imagen runtime ni ejecución automática al leer Dockerfile.
5. Runtime distroless static-debian12 nonroot con digest; copia únicamente binario, UID/GID65532, expone8080, entrypoint/server (gateway:12-16; jobs/risk:13-17). No incluye toolchain ni scripts.

## Compose: grafo de servicios y alternativas

[docker-compose.yml](../../docker-compose.yml):1-192 declara el proyecto `firewatch` y dos anchors. Su interpretación real depende de Compose y variables externas, no leídas.

| Bloque / líneas | Responsabilidad / conexiones | Condición o límite |
| --- | --- | --- |
| OIDC, 3-7 | development; issuer localhost8081; JWKS por hostname interno identity; audiencia firewatch-api | Issuer identifica emisor público; JWKS es dirección de acceso interna; no se exige que URLs sean iguales |
| hardened, 9-19 | read-only, cap_drop ALL, no-new-privileges, usuario65532, init, pids100, mem256m, cpu1, backend, restart | Anchor aplicado a aplicaciones y migradores; migradores sobrescriben restart |
| postgres, 22-39 | PostGIS17-3.5; variables de passwords; volumen datos/init; health pg_isready | Sin host port; healthy necesario para migraciones/identity; no prueba permisos app |
| rabbitmq, 41-58 | 4.1.8-management; vhost; volumen y plugins; health; management loopback15672 | Nodo único; contraseña variable; backend+edge |
| identity, 60-80 | Keycloak26.5.2 start-dev/import-realm, base identity, hostname localhost, mount realm | Espera postgres healthy; puerto loopback8081; realm excluido, claims/cuentas no comprobados |
| migradores, 82-100 | Imágenes app con argumento migrate; owner DB; postgres healthy; restart no | No AMQP/OIDC necesarios por rama migrate |
| jobs/risk, 102-122 | Roles DB separados, URL AMQP local; anchor OIDC; esperan migrate completed y rabbit healthy | No host ports; protocolos DB/AMQP locales sin TLS en variables declaradas |
| gateway, 124-133 | JOBS_URL/RISK_URL internos, puerto loopback8080, backend+edge | depends_on simple no comprueba readiness identity/jobs/risk |
| prometheus, 135-142 | Perfil observability; mount config/datos; loopback9090 | Opcional, no parte de startup por defecto |
| tests, 144-160 | Perfil test; target Docker test; bases separadas; esperan test migrations | No ejecución autorizada aquí |
| test migradores, 162-182 | migrate en jobs_test/risk_test con owner | Ejecutables app, no lógica distinta de migración |
| redes/volúmenes, 184-192 | edge, backend internal true; postgres/rabbit/prometheus data | Declaración de persistencia, no verificación de backups ni credenciales ya inicializadas |

Interpolaciones `${...:?required}` requieren variables; los mensajes indican prerequisitos, no contienen passwords resueltos. El grafo inicial espera RabbitMQ healthy, pero después de iniciar, `Broker.Run` gestiona caída y read API puede seguir disponible. No se equipara restart a disponibilidad medible.

## Kubernetes: cada recurso y sus fronteras

| Archivo / rango | Recursos / responsabilidad | Límites implementados en el manifiesto |
| --- | --- | --- |
| [namespace.yaml](../../deploy/k8s/base/namespace.yaml):1-6 | Namespace firewatch, label enforce restricted | Necesita políticas admitidas por clúster |
| [config.yaml](../../deploy/k8s/base/config.yaml):1-17 | ConfigMap staging, issuer/JWKS HTTPS example.invalid, audiencia/URLs internos; ServiceAccount sin automount | Valores placeholder; no incluye Secrets ni infraestructura |
| [kustomization.yaml](../../deploy/k8s/base/kustomization.yaml):1-9 | Namespace y cinco recursos YAML | No se renderizó; overlay no leído/no inventariado |
| [ingress.yaml](../../deploy/k8s/base/ingress.yaml):1-22 | Ingress nginx TLS secretRef; redirect; body8k; host example.invalid; solo /v1 ->gateway8080 | No publica health/metrics; secret/certificado externo |
| [workloads.yaml](../../deploy/k8s/base/workloads.yaml):1-42 | Deployment gateway2; SA; nonroot65532; seccomp; spread ScheduleAnyway; configMap; readonly/dropALL/no escalation; recursos/probes | Imagen replace-before-deploy; spread no es garantía dura de nodos separados |
| `deploy/k8s/base/workloads.yaml:43-86` | Deployment jobs2, mismo hardening/probes, config y secret jobs-runtime | Secret no definido aquí; ready consulta DB, no broker |
| `deploy/k8s/base/workloads.yaml:87-130` | Deployment risk2, secret risk-runtime | Mismas fronteras; sin migración Job incluida |
| `deploy/k8s/base/workloads.yaml:131-150` | Tres Services selector app, puerto8080->http | Routing interno por labels; no LB externo declarado |
| `deploy/k8s/base/workloads.yaml:152-174` | Tres PDB minAvailable1 | Interrupción voluntaria, no garantía contra fallos arbitrarios |

### Políticas de red: uniones de permisos, no cifrado

[network.yaml](../../deploy/k8s/base/network.yaml):1-69 contiene cuatro políticas:

- `default-deny` (1-6): todos pods, ingress/egress sin reglas.
- `dns-and-oidc` (8-23): todos pods pueden DNS TCP/UDP53 a kube-system y TCP443 a cualquier IPv4. El comentario pide estrechar CIDR; actualmente es amplio, no solo identidad.
- `gateway-entry` (25-41): ingress desde namespace ingress-nginx a8080; egress a jobs/risk de namespace local a8080.
- `service-entry-and-data` (43-69): jobs/risk reciben gateway local o namespace monitoring a8080; salen a postgres en firewatch-infra5432 y rabbitmq5671 del mismo namespace de infraestructura.

Estas reglas requieren CNI que aplique NetworkPolicy. La combinación de reglas permitidas es una dependencia de Kubernetes; no se comprobó comportamiento de red. Puerto5671 expresa intención TLS AMQP, no fuerza TLS por código. No se provisionan DB/broker, roles, Secrets ni certificados en estos recursos. HTTP gateway->servicios sigue siendo HTTP en ConfigMap; no hay mTLS implementado aquí.

## Observabilidad y Makefile

[prometheus.yml](../../deploy/local/prometheus.yml):1-12 fija scrape_interval15s y targets jobs8080, risk8080, rabbitmq15692. [rabbitmq-plugins](../../deploy/local/rabbitmq-plugins):1 habilita management y prometheus. No hay scrape gateway ni alert rules en estos archivos; no se consultó Prometheus.

[Makefile](../../Makefile):1-19 declara SERVICE?=jobs, SERVICES gateway/jobs/risk y targets phony. `test` recorre servicios y encadena test/vet, abortando ante error; build usa SERVICE; up/down/logs delegan Compose; integration ejecuta dos contenedores test secuencialmente; demo llama smoke. Son instrucciones operativas **leídas**, no órdenes ejecutadas por esta tarea. El loop necesita shell POSIX; no se sustituyó ni probó en PowerShell.

## Traza razonada normal y alternativa

Supuestos: variables externas presentes, volumen local inicializable, builds correctos y servicios externos disponibles.

| Paso | Cita | Cambio razonado |
| --- | --- | --- |
| Base | `docker-compose.yml:22-39,82-100` | postgres healthy -> migradores pueden ejecutar migrate |
| App | `docker-compose.yml:102-122`; ambos main:76-95 | Migrate completed + rabbit healthy -> pools, verifier y workers |
| HTTP | ambos main:96-129 | Handlers y probes disponibles; wrapper asigna request ID/timeout |
| Ready | ambos main:99-107 | Ping DB nil ->204, independientemente de Broker.Ready |
| Caída broker | ambos broker.go:28-47 | Ready broker false, reconexión; DB/read handlers no se eliminan |

**Alternativa:** OIDC HTTP en `APP_ENV=staging` falla en main:80-84 antes de iniciar worker. DB Ping fallido después del arranque devuelve503 en readiness. Kubernetes sin Secrets o con imágenes placeholder no tiene un despliegue funcional demostrado; esta colección no predice el error exacto de un clúster no inspeccionado.

## Cobertura y límites

Los 21 archivos explicados de esta parte cubren main runtime, declaraciones de módulos, tres etapas de cada imagen, anchors/servicios/redes/volúmenes Compose, todos los recursos YAML y targets/observabilidad. Las ramas migrate se remiten a [persistencia](06-database.md); los rangos completos reconciliados están en el índice. `deploy/k8s/README.md:3-29` aporta intención y prerequisitos, no cobertura de código adicional ni ejecución.

No se acredita compatibilidad de imágenes, render de 17 recursos, TLS real, backups, SLA, CI remoto, seguridad de dependencias o roles del realm. Las verificaciones históricas se separan en [tests y herramientas](08-tests-tools.md). Antes de desplegar se necesitarían decisiones de DNS, registry, infraestructura y secrets por su propietario, no cambios implícitos de este análisis.
