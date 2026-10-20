# Configuración y despliegue: arranque acotado, entorno aún por verificar

**Re-derivado el 2026-10-20 contra el código de `267a4bb`.**

El código define cuatro ejecutables Go independientes (`gateway`, `jobs`, `risk`, `prevention`); Compose conecta la demostración local y Kubernetes ofrece una base staging con placeholders. Los archivos describen cómo arrancar, no demuestran que un entorno esté desplegado o sea seguro en producción.

[Índice e instantánea](README.md) · [Persistencia](06-database.md) · [Siguiente: tests y herramientas](08-tests-tools.md)

## Alcance y fronteras

Todos los rangos completos están en el [registro](README.md): cuatro main gateway/jobs/risk/prevention:1-63/1-341/1-442/1-256; cuatro Dockerfile:1-16/17/24/18; cinco go.mod:1-23/23/34/19/3; go.work:1-9; Makefile:1-20; .dockerignore:1-8; Compose:1-273; siete YAML Kubernetes:1-6/19/10/22/234/105/99; Prometheus:1-32; reglas de alerta:1-104; plugins:1-1; provisioning de Grafana:1-16/17; panel:1-432. Lectura directa, sin builds, tests ni conexión a infraestructura; el render de la base se enumeró con `kubectl kustomize` y no se aplicó a ningún clúster. `.env` y variantes, `go.sum` y realm de cuentas excluidos; no se resuelven variables sensibles. Identidad por hashes de los bytes leídos, no revisión Git: [índice](README.md).

## Main jobs/risk: unidades y ramas compartidas

Los imports de ambos `cmd/server/main.go` conectan módulos propios, migraciones, OIDC, UUID, pgxpool, atomic y HTTP (`services/jobs/cmd/server/main.go:3-27`; `services/risk/cmd/server/main.go:3-32`). La diferencia funcional de los callbacks es `Handle: store.Complete` con `Fail: store.Fail` en jobs (`services/jobs/cmd/server/main.go:130`) frente a `Handle: publisher.Publish` con `ReportFailure` en risk (`services/risk/cmd/server/main.go:209`). Cada celda de líneas cita jobs primero y risk después para el mismo comportamiento. Prevention no comparte esa tabla: su arranque vive en `services/prevention/cmd/server/main.go:39-123` (pool, rama migrate, OIDC y HTTP en 8084) y su readiness responde 200/503 con cuerpo JSON desde `services/prevention/cmd/server/main.go:200-222`.

| Unidad / líneas (jobs / risk) | Secuencia y ramas | Efecto / error |
| --- | --- | --- |
| main, 29-35 / 34-40 | Logs JSON; run | Error -> service_stopped + exit 1 |
| run/pool, 37-51 / 42-56 | Contexto señales; parse DATABASE_URL; max 10/min1; connect 5 s; pool con cierre diferido | Errores configuración/setup genéricos; conexión efectiva no se demuestra al construir pool |
| migrate, 52-104 / 57-114 | Rama exacta de argumento; lock/versionado/commit; siembra de zonas después del commit | Explicada en [persistencia](06-database.md); siembra en cada corrida (`SeedZonesEmbedded` en jobs, `SeedEmbedded` en risk) y retorna antes de broker |
| configuración runtime, 105-117 / 115-127 | DB/AMQP no vacíos; issuer/JWKS con host y HTTPS salvo development HTTP; audiencia no vacía | Retorna errores antes de iniciar worker si inválido |
| verifier, 118-119 / 128-129 | Cliente HTTP 5 s; RemoteKeySet; NewVerifier con RS256 y audiencia | Validación delegada a OIDC; no discovery local |
| store/broker/worker, 120-133 / 130-212 | Store; umbrales DLQ; Broker con `Handle`/`Fail` o `ReportFailure`; goroutine Run; defer stop y espera | En risk el tramo añade registro de clasificadores, `MODEL_ID`, verificación del registro de modelos y publicador |
| ciclo periódico, — / 213-221 | Solo risk: `cycle.Start` en goroutine con intervalo de 15 min y zonas configurables | Publica a través del publicador; no salta guardas; jobs no tiene ciclo |
| mux/health, 134-139 / 222-226 | /v1 protegido; GET live 204; ready con ping acotado de 1 s | Ping falla ->503; éxito ->200; el cuerpo JSON de estado se envía en ambos códigos |
| contadores/metrics, 140-146 / 227-234 | atomic requests; MetricsText de entrega y dead-letter; bloques HTTP y de proceso | risk añade resultados de modelo y pool; errores Fprintf ignorados; métricas por proceso, no persistidas |
| outer, 147-167 / 235-255 | Incrementa contador para toda petición; parse ID o nuevo; ambas cabeceras; contexto 10 s; registra estado y latencia | ID válido incluye forma aceptada por uuid.Parse; no exige no Nil; no log HTTP por solicitud aquí |
| server/shutdown, 168-179 / 256-267 | 8080; límites 5/10/15/60 s y 16384 bytes; goroutine espera señales, Shutdown 10 s | Error shutdown ignorado; ErrServerClosed ->nil; otros errors devueltos |

El contexto de 10 s del wrapper no mata mágicamente un handler que ignore cancelación; lo entrega a llamadas cooperativas, entre ellas SQL. La disponibilidad HTTP y la capacidad de procesar mensajes se separan: el código de `/health/ready` depende solo del ping a la base (`services/jobs/cmd/server/main.go:256-265`; `services/risk/cmd/server/main.go:429-441`), mientras el cuerpo informa broker y dead-letter y esas señales quedan en métricas. `204` es la respuesta de `/health/live` y la que el gateway devuelve estáticamente para sus dos rutas de salud (`services/gateway/internal/handler/gateway.go:153-155`). Si ListenAndServe falla antes de recibir señal, el defer stop cancela worker; la goroutine de shutdown observa esa cancelación. No se prueba el comportamiento temporal real del cierre.

## Configuración esperada por el código

| Variables / consumidor | Validación local | Alcance |
| --- | --- | --- |
| OIDC_ISSUER, OIDC_JWKS_URL, OIDC_AUDIENCE / todos | URL/host/esquema; HTTPS fuera development; audiencia requerida | Biblioteca comprueba token con RS256 configurado; realm real no inspeccionado |
| JOBS_URL, RISK_URL, PREVENTION_URL / gateway | `required` y URL HTTP/HTTPS, panic si inválida | HTTP interno permitido incluso fuera development |
| DATABASE_URL / jobs/risk/prevention | pgxpool.ParseConfig; no vacío en runtime | TLS DB no exigido explícitamente por run; depende de URL suministrada |
| AMQP_URL / jobs/risk | No vacío antes de worker | No valida amqps en run; Dial y reconexión gestionan errores; prevention no lo usa |
| APP_ENV / todos | Solo string development permite OIDC HTTP | No es selección automática de infraestructura ni garantía de producción |
| DLQ_READY_MAX_DEPTH, DLQ_READY_MAX_AGE / jobs/risk | Entero no negativo y duración positiva; si no, error de arranque | Defaults 0 y 6 s; no cambian el código de readiness, solo el estado del cuerpo y las métricas |

Citas: `services/gateway/cmd/server/main.go:29-51`; `services/jobs/cmd/server/main.go:105-117`; `services/risk/cmd/server/main.go:115-127`; umbrales DLQ en `services/jobs/cmd/server/main.go:182-210` y `services/risk/cmd/server/main.go:293-321`. No se reproduce ningún valor de credencial.

## Módulos, workspace e imágenes

| Archivo / rango | Declaraciones y efecto |
| --- | --- |
| [go.work](../../go.work):1-9 | Go 1.27.2 y cinco módulos: contratos, gateway, jobs, prevention y risk. No módulo raíz ejecutable |
| [gateway/go.mod](../../services/gateway/go.mod):1-23 | firewatch.local/gateway, Go1.27.2, OIDC3.17.0 y kin-openapi0.133.0; jose4.1.4 y once indirectos de soporte |
| [jobs/go.mod](../../services/jobs/go.mod):1-23; [risk/go.mod](../../services/risk/go.mod):1-34 | Módulos propios; contracts0.0.0, OIDC3.17.0, jose4.1.4, uuid1.6.0, pgx5.9.2, amqp1.13.0; risk añade kin-openapi0.133.0; replace local ../../contracts/gen |
| [prevention/go.mod](../../services/prevention/go.mod):1-19 | firewatch.local/prevention, Go1.27.2, OIDC3.17.0, jose4.1.4, uuid1.6.0, pgx5.9.2; sin amqp ni contratos |
| [contracts/gen/go.mod](../../contracts/gen/go.mod):1-3 | firewatch.local/contracts y Go1.27.2, sin dependencias declaradas |
| [.dockerignore](../../.dockerignore):1-8 | Excluye .env, anidados, bin, coverage.out, go.work*, .git, deploy y docs del contexto; no es escáner universal de secretos ni excluye todo archivo llamado .env.example |

Los go.sum inventariados no se analizaron: no se acredita integridad ni disponibilidad de módulos. La selección automática de toolchain está fuera de la ejecución autorizada.

Los [Dockerfile gateway](../../services/gateway/Dockerfile):1-16, [jobs](../../services/jobs/Dockerfile):1-17, [risk](../../services/risk/Dockerfile):1-24 y [prevention](../../services/prevention/Dockerfile):1-18 usan el mismo patrón:

1. Build desde Go1.27.2-bookworm con digest; WORKDIR de módulo; `GOWORK=off`, `CGO_ENABLED=0` (en los cuatro:1-3).
2. Jobs/risk copian contracts/gen antes de go.mod/go.sum para resolver replace (jobs:4-6; risk:4-6). Gateway y prevention copian solo su módulo (gateway:4-5; prevention:4-5). `go mod download && go mod verify` es instrucción de build leída, no ejecutada aquí.
3. Copian código y compilan `/server` con trimpath y `-s -w` (gateway:6-7; jobs:7-8; risk:7-8; prevention:6-7).
4. Target `test` hereda build y usa ENTRYPOINT `go test -count=1 ./...` (gateway:9-10; jobs:10-11; risk:10-18; prevention:9-10). Risk añade la copia de `contracts/openapi/` porque sus tests la leen por ruta relativa (risk:11-17). No implica tests en la imagen runtime ni ejecución automática al leer Dockerfile.
5. Runtime distroless static-debian12 nonroot con digest; copia únicamente binario, UID/GID65532, entrypoint/server (gateway:12-16; jobs:13-17; risk:20-24; prevention:12-18). Gateway/jobs/risk exponen 8080 y prevention 8084 (prevention:15-17). No incluye toolchain ni scripts.

## Compose: grafo de servicios y alternativas

[docker-compose.yml](../../docker-compose.yml):1-273 declara el proyecto `firewatch` y dos anchors. Su interpretación real depende de Compose y variables externas, no leídas.

| Bloque / líneas | Responsabilidad / conexiones | Condición o límite |
| --- | --- | --- |
| OIDC, 3-7 | development; issuer localhost8081; JWKS por hostname interno identity; audiencia firewatch-api | Issuer identifica emisor público; JWKS es dirección de acceso interna; no se exige que URLs sean iguales |
| hardened, 9-19 | read-only, cap_drop ALL, no-new-privileges, usuario65532, init, pids100, mem256m, cpu1, backend, restart | Anchor aplicado a aplicaciones y migradores; migradores sobrescriben restart |
| postgres, 22-50 | PostGIS17-3.5; variables de passwords; volumen datos/init; health `pg_isready -h 127.0.0.1` | Sin host port; el health exige TCP real, no solo el socket de inicialización; no prueba permisos app |
| rabbitmq, 52-69 | 4.1.8-management; vhost; volumen y plugins; health; management loopback15672 | Nodo único; contraseña variable; backend+edge |
| identity, 71-91 | Keycloak26.5.2 start-dev/import-realm, base identity, hostname localhost, mount realm | Espera postgres healthy; puerto loopback8081; realm excluido, claims/cuentas no comprobados |
| migradores, 93-121 | Tres imágenes app con argumento migrate; owner DB; postgres healthy; restart no; prevention migra la base risk | No AMQP/OIDC necesarios por rama migrate |
| jobs/risk, 123-147 | Roles DB separados, URL AMQP local; anchor OIDC; risk fija `MODEL_ID` por defecto; esperan migrate completed y rabbit healthy | No host ports; protocolos DB/AMQP locales sin TLS en variables declaradas |
| prevention, 149-157 | Base risk con esquema propio, anchor OIDC, espera prevention-migrate | Sin AMQP ni broker: no depende de rabbitmq |
| gateway, 159-170 | JOBS_URL/RISK_URL/PREVENTION_URL internos, WEB_ORIGIN, puerto loopback8080, backend+edge | depends_on simple no comprueba readiness identity/jobs/risk/prevention |
| prometheus, 172-180 | Perfil observability; mount config, alertas y datos; loopback9090 | Opcional, no parte de startup por defecto |
| grafana, 182-200 | Perfil observability; datasource y panel por archivo; loopback3000; password admin requerido | Sin Alertmanager: las alertas se ven en Prometheus, no se enrutan |
| tests, 202-229 | Perfil test; target Docker test; bases separadas; esperan test migrations | No ejecución autorizada aquí |
| test migradores, 231-262 | migrate en jobs_test/risk_test con owner | Ejecutables app, no lógica distinta de migración |
| redes/volúmenes, 264-273 | edge, backend internal true; postgres/rabbit/prometheus/grafana data | Declaración de persistencia, no verificación de backups ni credenciales ya inicializadas |

Interpolaciones `${...:?required}` requieren variables; los mensajes indican prerequisitos, no contienen passwords resueltos. El grafo inicial espera RabbitMQ healthy, pero después de iniciar, `Broker.Run` gestiona caída y read API puede seguir disponible. No se equipara restart a disponibilidad medible.

## Kubernetes: cada recurso y sus fronteras

| Archivo / rango | Recursos / responsabilidad | Límites implementados en el manifiesto |
| --- | --- | --- |
| [namespace.yaml](../../deploy/k8s/base/namespace.yaml):1-6 | Namespace firewatch, label enforce restricted | Necesita políticas admitidas por clúster |
| [config.yaml](../../deploy/k8s/base/config.yaml):1-19 | ConfigMap staging, issuer/JWKS HTTPS example.invalid, audiencia/URLs internos con prevention8084; ServiceAccount sin automount | Valores placeholder; no incluye Secrets ni infraestructura |
| [kustomization.yaml](../../deploy/k8s/base/kustomization.yaml):1-10 | Namespace y seis archivos de recursos; el render enumera 25 objetos | No se aplicó a ningún clúster; overlay no leído/no inventariado |
| [ingress.yaml](../../deploy/k8s/base/ingress.yaml):1-22 | Ingress nginx TLS secretRef; redirect; body8k; host example.invalid; solo /v1 ->gateway8080 | No publica health/metrics; secret/certificado externo |
| [workloads.yaml](../../deploy/k8s/base/workloads.yaml):1-41 | Deployment gateway2; SA; nonroot65532; seccomp; spread ScheduleAnyway; configMap; readonly/dropALL/no escalation; recursos/probes | Imagen replace-before-deploy; spread no es garantía dura de nodos separados |
| `deploy/k8s/base/workloads.yaml:43-85` | Deployment jobs2, mismo hardening/probes, config y secret jobs-runtime | Secret no definido aquí; ready consulta DB, no broker |
| `deploy/k8s/base/workloads.yaml:87-129` | Deployment risk2, secret risk-runtime | Mismas fronteras; sin migración Job incluida |
| `deploy/k8s/base/workloads.yaml:131-174` | Deployment prevention2 en 8084, secret prevention-runtime | Puerto propio del contrato; mismas fronteras |
| `deploy/k8s/base/workloads.yaml:176-202` | Cuatro Services selector app: 8080->http y prevention8084->http | Routing interno por labels; no LB externo declarado |
| `deploy/k8s/base/workloads.yaml:204-234` | Cuatro PDB minAvailable1 | Interrupción voluntaria, no garantía contra fallos arbitrarios |
| [autoscaling.yaml](../../deploy/k8s/base/autoscaling.yaml):1-105 | Un HorizontalPodAutoscaler por servicio: min2, max6, CPU70% de la request, scaleUp60 s, scaleDown300 s | Exige metrics-server y medición real; no demuestra capacidad probada |

### Políticas de red: uniones de permisos, no cifrado

[network.yaml](../../deploy/k8s/base/network.yaml):1-99 contiene cinco políticas:

- `default-deny` (1-7): todos pods, ingress/egress sin reglas.
- `dns-and-oidc` (8-24): todos pods pueden DNS TCP/UDP53 a kube-system y TCP443 a cualquier IPv4. El comentario pide estrechar CIDR; actualmente es amplio, no solo identidad.
- `gateway-entry` (25-49): ingress desde namespace ingress-nginx a8080; egress a jobs/risk de namespace local a8080 y a prevention a8084.
- `service-entry-and-data` (50-77): jobs/risk reciben gateway local o namespace monitoring a8080; salen a postgres en firewatch-infra5432 y rabbitmq5671 del mismo namespace de infraestructura.
- `prevention-entry-and-data` (78-99): prevention recibe gateway o monitoring a8084 y sale solo a postgres5432; sin egress a broker.

Estas reglas requieren CNI que aplique NetworkPolicy. La combinación de reglas permitidas es una dependencia de Kubernetes; no se comprobó comportamiento de red. Puerto5671 expresa intención TLS AMQP, no fuerza TLS por código. No se provisionan DB/broker, roles, Secrets ni certificados en estos recursos. HTTP gateway->servicios sigue siendo HTTP en ConfigMap; no hay mTLS implementado aquí.

## Observabilidad y Makefile

[prometheus.yml](../../deploy/local/prometheus.yml):1-32 fija scrape_interval15s, carga `/etc/prometheus/alerts.yml` (6-7) y apunta a jobs8080, risk8080, gateway8080, rabbitmq15692 y prevention8084. [alerts.yml](../../deploy/local/alerts.yml):1-104 declara siete reglas en dos grupos (latencia, errores, dead-letter, jobs atascados, broker, pool y target caído); los umbrales se declaran objetivos acordados, no mediciones. [rabbitmq-plugins](../../deploy/local/rabbitmq-plugins):1 habilita management y prometheus. [Grafana](../../deploy/local/grafana/provisioning/datasources/prometheus.yml):1-16 provisiona el datasource por archivo y [el proveedor de paneles](../../deploy/local/grafana/provisioning/dashboards/firewatch.yml):1-17 carga un panel versionado; no hay enrutamiento de alertas ni se consultó ninguna instancia.

[Makefile](../../Makefile):1-20 declara SERVICE?=jobs, SERVICES gateway/jobs/risk/prevention y targets phony. `test` recorre los cuatro servicios y encadena test/vet, abortando ante error; build usa SERVICE; up/down/logs delegan Compose; integration ejecuta tres contenedores test secuencialmente; demo llama smoke. Son instrucciones operativas **leídas**, no órdenes ejecutadas por esta tarea. El loop necesita shell POSIX; no se sustituyó ni probó en PowerShell.

## Traza razonada normal y alternativa

Supuestos: variables externas presentes, volumen local inicializable, builds correctos y servicios externos disponibles.

| Paso | Cita | Cambio razonado |
| --- | --- | --- |
| Base | `docker-compose.yml:22-50,93-121` | postgres healthy -> los tres migradores aplican su esquema; jobs y risk además siembran zonas |
| App | `docker-compose.yml:123-147`; `services/jobs/cmd/server/main.go:105-133`; `services/risk/cmd/server/main.go:115-220` | Migrate completed + rabbit healthy -> pools, verifier, workers y, en risk, ciclo |
| HTTP | `services/jobs/cmd/server/main.go:134-167`; `services/risk/cmd/server/main.go:222-255` | Handlers y probes disponibles; wrapper asigna request ID/timeout |
| Ready | `services/jobs/cmd/server/main.go:250-265`; `services/risk/cmd/server/main.go:429-441` | Ping DB nil ->200 con cuerpo JSON, independientemente de Broker.Ready |
| Caída broker | `services/jobs/internal/messaging/broker.go:102-124`; `services/risk/internal/messaging/broker.go:91-113` | Ready broker false, reconexión; DB/read handlers no se eliminan |

**Alternativa:** OIDC HTTP en `APP_ENV=staging` falla en `services/jobs/cmd/server/main.go:109-114` y `services/risk/cmd/server/main.go:119-124` antes de iniciar worker. DB Ping fallido después del arranque devuelve503 en readiness con el mismo cuerpo. Kubernetes sin Secrets o con imágenes placeholder no tiene un despliegue funcional demostrado; esta colección no predice el error exacto de un clúster no inspeccionado.

## Cobertura y límites

Los 33 archivos explicados de esta parte cubren main runtime de los cuatro servicios, declaraciones de módulos, tres etapas de cada imagen, anchors/servicios/redes/volúmenes Compose, los siete YAML de Kubernetes con sus HPAs, scrape y alertas, provisioning de Grafana y targets del Makefile. Las ramas migrate se remiten a [persistencia](06-database.md); los rangos completos reconciliados están en el índice. `deploy/k8s/README.md:3-30` aporta intención y prerequisitos, no cobertura de código adicional ni ejecución.

No se acredita compatibilidad de imágenes, render aplicado de 25 objetos, TLS real, backups, SLA, CI remoto, seguridad de dependencias o roles del realm. Las verificaciones históricas se separan en [tests y herramientas](08-tests-tools.md). Antes de desplegar se necesitarían decisiones de DNS, registry, infraestructura y secrets por su propietario, no cambios implícitos de este análisis.
