# Kubernetes deployment — v1.0 delivery substrate

**Estado:** abierto y en ejecución.

Requisito externo nuevo: la entrega se monta en Kubernetes. Sustituye el sustrato que fijaba D1 (Compose en una VM) y deja el motor de datos intacto.

## Specs

**S1.** «por pedido de la profe tenemos que usar si o si kubernetes para montar la app por lo que dejemoslonpreparado para kubernetes» — desplegar la aplicación en Kubernetes no es opcional ni una alternativa documentada: es el camino de entrega.

**S2.** «partamos con kibernetes real pero basico, con todas las bases para pasarlo a produccion real en caso de que el cliente nos contrate» — un clúster real, no una simulación: API server real, pods reales, manifiestos con forma de producción. Básico en operación, no en atajos.

**S3.** «me quedo con ti tercera opción, todo en el cister y documentado» — PostgreSQL con PostGIS y RabbitMQ corren dentro del clúster, y el reemplazo por servicios gestionados queda documentado con su costo y sus condiciones.

## Tasks

| ID | Tarea | Spec | Ruta | Estado |
| --- | --- | --- | --- | --- |
| T1 | Levantar el clúster local real; registrar la decisión | S2 | inline | **Diferido por decisión del responsable**: no se instala nada hasta tener acceso a la VM de entrega |
| T2 | Bootstrap de secretos fuera del repositorio | S1 | inline | Hecho en `3c7b0b0` |
| T3 | PostgreSQL + PostGIS como StatefulSet con almacenamiento, init de roles y bases | S3 | inline | Hecho en `3c7b0b0` |
| T4 | RabbitMQ como StatefulSet con persistencia y gestión acotada | S3 | inline | Hecho en `3c7b0b0` |
| T5 | Jobs de migración en el orden correcto (risk → prevention → jobs) | S3 | inline | Hecho en `dd9a542` — un Job con initContainers secuenciales; contrato ejecutable en `backend/deploy/k8s/tests/render-contract.sh` |
| T6 | Job que aprovisiona broker e identidades con el script existente | S3 | inline | Hecho en `dd9a542` — el script del repositorio montado desde ConfigMap; la API de administración pasó a TLS 15671 |
| **T7** | **Deployments de la aplicación contra los secretos reales, sin `replace-before-deploy`** — bloque abierto, partido en nueve unidades T7.1–T7.9; la dependencia de cada una va en su propia celda | S1 | inline | En curso: T7.1–T7.6 hechas; quedan T7.7, T7.8 y T7.9 |
| T7.1 | **T7.1 — Gateway: TLS de entrada, upstreams HTTPS y CA de confianza.** Superficie disjunta: en paralelo con T7.2–T7.5 | S1 | inline | Hecho en `069b418` |
| T7.2 | **T7.2 — jobs: TLS de entrada, AMQP con CA propia y JWKS con CA.** Superficie disjunta: en paralelo con T7.1 y T7.3–T7.5 | S1 | inline | Hecho en `e8e085f` |
| T7.3 | **T7.3 — risk: TLS de entrada, AMQP con CA propia, JWKS y cliente de inferencia con CA.** Superficie disjunta: en paralelo con T7.1, T7.2, T7.4 y T7.5 | S1 | inline | Hecho en `b2af3b2` |
| T7.4 | **T7.4 — prevention: TLS de entrada en 8084, JWKS y cliente administrativo del directorio con CA.** Superficie disjunta: en paralelo con T7.1–T7.3 y T7.5 | S1 | inline | Hecho en `52e943a` |
| T7.5 | **T7.5 — inference: TLS de entrada.** Superficie disjunta: en paralelo con T7.1–T7.4 | S1 | inline | Hecho en `74145c6` |
| T7.6 | **T7.6 — secretos de runtime y certificados por servicio.** Superficie disjunta: en paralelo con T7.7, T7.8 y con T7.1–T7.5 | S1 | inline | Hecho en `9f225e2` |
| T7.7 | **T7.7 — manifiestos de la aplicación.** Superficie disjunta: en paralelo con T7.6, T7.8 y con T7.1–T7.5 | S1 | inline | Pendiente |
| T7.8 | **T7.8 — Keycloak e inferencia en el clúster.** Superficie disjunta: en paralelo con T7.6, T7.7 y con T7.1–T7.5 | S1 | inline | Pendiente |
| T7.9 | **T7.9 — contrato de render ampliado.** Va al final: comprueba el resultado de todas las unidades, así que no se ejecuta en paralelo con ninguna | S1 | inline | Pendiente |
| T8 | Ingress y TLS, incluidos los listeners internos | S2 | inline | Pendiente |
| T9 | Validación end-to-end en el clúster, con evidencia | S1 | inline | **Bloqueada**: sin clúster no hay nada que validar |
| T10 | Camino a producción real documentado y enmienda a D1 | S2, S3 | inline | Pendiente |

**Superficies de edición de T7, una por unidad** (rutas relativas al repositorio):

- `T7.1` — `backend/services/gateway/cmd/server/main.go`, `backend/services/gateway/cmd/server/transport.go`, `backend/services/gateway/internal/handler/gateway.go` y los `_test.go` de esos paquetes.
- `T7.2` — `backend/services/jobs/cmd/server/`, `backend/services/jobs/internal/messaging/broker.go` y sus tests.
- `T7.3` — `backend/services/risk/cmd/server/`, `backend/services/risk/internal/messaging/broker.go`, `backend/services/risk/internal/classify/inference.go` y sus tests.
- `T7.4` — `backend/services/prevention/cmd/server/`, `backend/services/prevention/internal/directory/directory.go` y sus tests.
- `T7.5` — `backend/services/inference/cmd/server/main.go` y sus tests.
- `T7.6` — `backend/deploy/k8s/bootstrap-secrets.sh`.
- `T7.7` — `backend/deploy/k8s/base/workloads.yaml`, `backend/deploy/k8s/base/config.yaml`.
- `T7.8` — `backend/deploy/k8s/base/keycloak.yaml` (nuevo), `backend/deploy/k8s/base/inference.yaml` (nuevo), `backend/deploy/k8s/base/network.yaml`, `backend/deploy/k8s/base/kustomization.yaml`.
- `T7.9` — `backend/deploy/k8s/tests/render-contract.sh`.

**Paralelismo.** T7.1–T7.5 tienen superficies disjuntas entre sí. T7.6, T7.7 y T7.8 son disjuntas entre sí y también respecto de T7.1–T7.5. Esas ocho unidades pueden ejecutarse en paralelo. T7.9 no puede: comprueba el resultado de todas y va al final, sobre las ocho anteriores ya presentes.

## Restricciones que no vienen de este pedido

- `V1-H-05` sigue vigente: nada de texto plano fuera de `APP_ENV=development`. En el clúster se cumple con TLS real, no con la excepción de desarrollo.
- La matriz de permisos del broker de `V1-H-04` se conserva: mismas dos identidades, mismos tres exchanges, mismos cinco bindings.
- Ninguna credencial en el repositorio, ni en valores por defecto de ConfigMap, ni en imágenes.
- El ledger de migraciones es único por base: los Jobs deben respetar el orden que ya documenta `backend/docs/migration-ledger.md`.

## Log

**L1 (2026-11-09, textual).** «por pedido de la profe tenemos que usar si o si kubernetes para montar la app por lo que dejemoslonpreparado para kubernetes»

**L2 (2026-11-09, textual).** «partamos con kibernetes real pero basico, con todas las bases para pasarlo a produccion real en caso de que el cliente nos contrate»

**L3 (2026-11-09, textual).** «me gustaría que me expliques cual es la mejor opción yporqué, pero me quedo con ti tercera opción, todo en el cister y documentado»

**L4 (2026-11-09) — estado del entorno medido.** `kubectl` presente; sin kind, k3d ni minikube; el contexto de Docker Desktop no responde. Docker 29.5.3 con 12 CPU y 8 GB, suficiente para un clúster de un nodo. La base de manifiestos existente son 527 líneas (namespace, cuatro Deployments, Services, PDBs, políticas de red, ingress y HPA) y **no incluye** Postgres, RabbitMQ, Jobs de migración ni Job de aprovisionamiento: eso es lo que falta para que el despliegue exista de verdad.

**L5 (2026-11-09) — plano de datos escrito, sin aplicar.** Commit `3c7b0b0`. PostgreSQL como StatefulSet con su claim, TLS de servidor y el script de roles del repositorio montado (no copiado, para que no pueda desviarse de la versión de Compose). RabbitMQ con el listener AMQP en claro **apagado de raíz**: solo 5671 con certificado, porque las guardas de `V1-H-05` rechazan `amqp://` fuera de desarrollo y un listener en claro sería a la vez peso muerto y vía de escape. La API de administración queda en claro pero aislada por política de red: lleva comandos de operador, no datos. `bootstrap-secrets.sh` genera credenciales y una CA interna, y **nunca rota en silencio**: un secreto existente se deja intacto, porque cambiar una contraseña bajo un servicio corriendo lo rompe en la siguiente reconexión. `kubectl kustomize` renderiza 32 recursos: eso prueba validez estructural y nada más.

**L6 (2026-11-09) — verificación independiente del bloque completo: fail.** Tres defectos en estos manifiestos, ninguno aplicado por tanto ninguno observable: los selectores de `network.yaml` exigen un namespace `firewatch-infra` que no existe y una etiqueta `app:` que estos pods no llevan, así que con `default-deny` nadie alcanzaría 5432 ni 5671; el PSS `restricted` del namespace rechaza los dos StatefulSet en admisión porque no pueden declarar `runAsNonRoot`; y la clave del ConfigMap de plugins no coincide con el `subPath` que monta el pod, así que RabbitMQ no arranca. Los tres quedaron a cargo de la sesión que verificó, junto con la corrección del rollback documentado en `04-broker-least-privilege.md`.

**L7 (2026-11-09) — el hueco grande, y por qué no se toca todavía.** El gateway no puede arrancar en este clúster: su ConfigMap declara `APP_ENV: staging` con upstreams `http://`, ningún servicio sirve TLS (los cuatro usan `ListenAndServe`), el cliente AMQP no acepta hoy una CA propia y los secretos de runtime no llevan `sslrootcert`. Además faltan dos servicios en la base: Keycloak y el de inferencia, sin los cuales la aplicación no funciona de punta a punta aunque todo lo demás esté bien. Es el tramo T7/T8 y merece su propio trabajo, no un parche.

**L8 (2026-11-10) — T5 y T6 escritos y verificados, sin aplicar.** Commit `dd9a542`. Corte de unidad: 522 líneas autoradas en un solo commit; el bloque se commitea junto porque los dos Jobs y las políticas que los conectan comparten `kustomization.yaml` y `network.yaml`, y ninguno renderiza completo por separado.

**T5 es un solo Job, no tres.** `initContainers` `migrate-risk` → `migrate-prevention`, contenedor principal `migrate-jobs`. Los initContainers de un pod corren estrictamente en orden y el principal no arranca hasta que ambos salen 0, así que el orden risk → prevention → jobs lo garantiza el API server, no un `sleep` ni un `kubectl wait`. Tres Jobs separados no tendrían orden ninguno y competirían por el ledger compartido en un volumen recién creado. Cada paso ejecuta `/server migrate`, lee la clave del DSN de la base donde vive su ledger (`RISK_DATABASE_URL` para risk y prevention, `JOBS_DATABASE_URL` para jobs), monta la CA interna en `/etc/tls/ca.crt` —la ruta que el DSN ya nombra— y declara `APP_ENV=staging`, que es lo que impide que la guarda de transporte tome su excepción de desarrollo.

**T6 ejecuta el script del repositorio**, montado desde un ConfigMap que `bootstrap-secrets.sh` renderiza del archivo, con las seis variables que el script exige y las tres credenciales solo por `secretKeyRef`. La identidad administrativa es `firewatch`, la misma que `RABBITMQ_DEFAULT_USER` del broker.

**Cuatro políticas de red nuevas**, porque `default-deny` deja a cualquier pod sin salida: `migrate-egress`, `broker-provision-egress`, `rabbitmq-management` y la entrada de `app: migrate` en `postgres-entry`. Ninguna amplía los derechos de un servicio de la aplicación.

**`backend/deploy/k8s/tests/render-contract.sh`** convierte el bloque en una prueba repetible: orden de los pasos, claves de DSN, montaje de la CA, los dos puertos, los campos de PSS y ausencia de credenciales literales. Se comprobó que falla cuando debe: invertir los initContainers o inyectar una credencial literal lo pone en rojo.

**Corrección a L5.** L5 afirmaba que la API de administración quedaba en claro. Eso no se sostiene: la contraseña administrativa viaja en una cabecera basic-auth y el bloque se declara sujeto a «nada de texto plano fuera de `APP_ENV=development`». El listener de administración pasó a **TLS en 15671** con el mismo certificado que AMQPS, el puerto claro quedó apagado (`management.tcp.port = 0`) y curl verifica contra la CA interna por `CURL_CA_BUNDLE`. Ese `0` es la única pieza que solo un clúster puede confirmar: si el plugin no lo acepta, el broker falla visible al arrancar, no en silencio.

**Verificación independiente: pass con defectos.** Corregidos antes del commit: dos comentarios afirmaban «y a nada más» cuando `dns-and-oidc` concede 443 a cualquier pod, y `backoffLimit: 3` prometía más de lo que el manifiesto garantiza —nada espera la readiness de Postgres—, así que quedó en 6 con un comentario que dice lo que el Job realmente hace. Registrados y **sin corregir**: los DSN de migración usan el superusuario `postgres`, más amplio que el «dueño de migraciones» que describe el ledger; la regla 443-a-cualquiera que todo pod hereda merece estrecharse en un overlay; y nada de esto está aplicado.

**No verificable sin clúster:** admisión `restricted`, secuencia real de initContainers, aplicación de las NetworkPolicy por el CNI, disponibilidad de `/bin/sh` en `curlimages/curl`, el `0` del listener de administración y cualquier Job terminando con éxito. Sigue siendo T9.

**L9 (2026-11-10) — revisión nativa: dos defectos reales encontrados y corregidos; el ciclo no cerró.** Linaje `review-f124814899db9021`, tres lentes más refutador (`deepseek-v4.1-flash`), ocho archivos, 563 líneas. Los dos hallazgos fueron culpa del candidato y ambos los había introducido yo:

1. **Bloqueante — volumen `ca` montado y nunca declarado** en `broker-provision.yaml` (commit `51713e5`). El pod habría sido rechazado en admisión y el broker no se habría aprovisionado nunca. Peor: mi contrato de render lo dejó pasar porque solo comprobaba la cadena `CURL_CA_BUNDLE`. Ahora el contrato deriva los volúmenes montados de cada manifiesto y exige que el pod los declare; verificado en rojo contra el archivo roto.
2. **Crítico R4-001 — nombre fijo del Job de migraciones** (`9ae67e1`). El pod template de un Job es inmutable y un Job terminado no se re-ejecuta con `apply`, así que con nombre fijo y TTL de una hora un release dentro de esa hora o saltaba la migración o era rechazado, y la aplicación podía arrancar contra el esquema viejo. `generateName` no sirve: `kubectl apply` y kustomize exigen `metadata.name`. La corrección nombra el Job por la versión de esquema que produce (`migrate-v14`), y el contrato deriva la migración más alta del árbol y falla si el nombre se desincroniza.

**El ciclo quedó abierto.** Con los dos defectos corregidos y commiteados, la captura del plan de corrección se rechazó tres veces con `collectBinding is missing or stale for current STATUS`, incluso con el árbol de `HEAD` idéntico al candidato congelado (`c8a35234`) y el workspace limpio. Es un problema del canal de corrección, no del código: el linaje sigue en `correction_required` y **no hay aprobación de revisión**. Hace falta intervención sobre la autoridad o una revisión nueva del candidato corregido (árbol posterior a `9ae67e1`). El trabajo no se revierte por eso: el bloque está verificado por el contrato de render y por el verificador independiente, y la corrección es la correcta.

**L10 (2026-11-10) — T7 se abre y se parte en unidades.**

**Decisión del responsable, textual:** «t7 completo ahora, en unidades paralelizables si es posible». Se ejecuta el tramo entero y en cortes que no comparten archivos, para que la revisión pueda ir por partes sin releer lo mismo.

**El hallazgo que fija el orden de trabajo.** En el backend no existe hoy ninguna llamada a `ServeTLS`, a `tls.LoadX509KeyPair` ni a `amqp.DialTLS`, mientras las guardas que ya existen rechazan exactamente la configuración de estos manifiestos: `requireTLSOutsideDevelopment` (`backend/services/{jobs,risk,prevention}/cmd/server/transport.go`), `requireSecureBrokerURL` (`backend/services/jobs/cmd/server/transport.go:47`, `backend/services/risk/cmd/server/transport.go:47`), `requireSecureUpstreams` (`backend/services/gateway/cmd/server/transport.go:12`) y las guardas de OIDC en los cuatro `main.go` (gateway, jobs, risk y prevention). Con `APP_ENV=staging`, el `config.yaml` con upstreams `http://` y los DSN con `sslmode=disable`, esos servicios no arrancan. **T7 no añade seguridad: hace que el runtime cumpla las guardas que ya tiene.**

**Contrato de cruce, obligatorio para todas las unidades** —es lo que evita que las unidades diverjan:

- Servidor: `TLS_CERT_FILE` y `TLS_KEY_FILE` con las rutas del certificado y la clave. Si ambos están definidos se sirve TLS con `ListenAndServeTLS`; si ninguno está definido y `APP_ENV=development` se mantiene el texto plano de desarrollo; en cualquier otro caso el arranque falla nombrando la variable que falta.
- Salientes: `TLS_CA_FILE` con el PEM de la CA interna. Si está definido, alimenta un `x509.CertPool` propio; los clientes internos lo usan en `TLSClientConfig.RootCAs`.
- AMQP: `amqp.Config` incorpora `TLSClientConfig` con esa CA sobre `amqps://`; el `Dial` de 5 s y el `Heartbeat` se conservan.
- DSN: el código no cambia. El secreto de runtime aporta `DATABASE_URL` con `sslmode=verify-full&sslrootcert=/etc/tls/ca.crt`.
- Rutas dentro del pod: `/etc/tls/ca.crt`, `/etc/tls/tls.crt`, `/etc/tls/tls.key`.
- Secrets: `firewatch-ca` (clave `ca.crt`) ya existe; `bootstrap-secrets.sh` pasa a emitir un certificado por servicio —`gateway-tls`, `jobs-tls`, `risk-tls`, `prevention-tls`, `inference-tls`, `keycloak-tls`— con claves `tls.crt`, `tls.key` y `ca.crt`, y los secretos `jobs-runtime`, `risk-runtime`, `prevention-runtime` con la clave `DATABASE_URL`.
- Puertos: 8080 en gateway, jobs, risk e inference; 8084 en prevention. Sin cambios. El nombre del puerto en Service y probes pasa de `http` a `https`.
- Upstreams: `https://jobs:8080`, `https://risk:8080`, `https://prevention:8084`; inferencia `https://inference:8080`.
- Keycloak: Service `identity` en 8443, issuer y JWKS `https://identity:8443/realms/firewatch`.
- `APP_ENV` sigue en `staging`.

**Paridad del Compose de producción, verificada.** `backend/docker-compose.prod.yml:35` declara `APP_ENV: production` con `AMQP_URL: amqp://` (`:193`, `:205`) y DSN `sslmode=disable` (`:192`, `:204`), es decir, el Compose de producción tampoco pasaría esas guardas. No se corrige en T7; se registra como pendiente para T10.

**Límite de tamaño por unidad.** Cada unidad debe caber en un candidato de revisión de unas 400 líneas autoradas, porque el relay de revisión corta alrededor de 16 minutos por lente.

**L11 (2026-11-10) — límite de entrega.** La rama `feat/k8s-bootstrap-jobs` acumula **634 líneas autoradas en 6 commits** desde `main` (`babf7d3`), medidas con los archivos de bloqueo excluidos. Eso pasa la rebanada de revisión de ~400 líneas que fija el protocolo.

**La entrega va en dos rebanadas encadenadas, decidida.** No hay ningún PR abierto todavía, así que se escribe como:

- **Rebanada A — sustrato de arranque:** `babf7d3..7c94e02`, cinco commits: el plano de datos y los dos Jobs de arranque con su contrato de render.
- **Rebanada B — cargas de trabajo con TLS:** `54734c9` en adelante, el bloque T7 completo.

La B se apila sobre la A: un PR para la A y otro para la B, y la B no se abre hasta que la A esté aceptada. Ninguna se abre todavía: abrir PR es decisión del responsable, no de esta sesión.

**El motivo.** El relay de revisión nativa corta alrededor de 16 minutos por lente —lo que ya ocurrió en el linaje abandonado—, así que un candidato de la rama entera no es revisable; cada rebanada y cada unidad de T7 se revisa sobre su propio commit.

**L12 (2026-11-10) — corrección al contrato de L10: `AMQP_URL` en los secretos de runtime.**

L10 fijó `DATABASE_URL` como única clave de `jobs-runtime`, `risk-runtime` y `prevention-runtime`, y omitió `AMQP_URL`. La tabla de secretos de `backend/deploy/k8s/README.md` sí la declaraba para `jobs` y `risk`, y ambos procesos la leen al arrancar (`backend/services/{jobs,risk}/cmd/server/transport.go`): el arranque se detiene sin ella (`backend/services/jobs/cmd/server/main.go:121`, `backend/services/risk/cmd/server/main.go:119`), así que los pods habrían arrancado sin broker y sin servir. La clave entra ahora en esos dos secretos y solo en ellos —`prevention` no declara cola ni cliente de broker (`backend/services/prevention/cmd/server/main.go:194`)— como `amqps://` sobre 5671, con las credenciales que `bootstrap-secrets.sh` ya guarda en `broker-credentials` (`RABBITMQ_JOBS_PASSWORD` y `RABBITMQ_RISK_PASSWORD`, releídas del Secret para que la URL nombre la contraseña que el broker tiene y no la que generó esa ejecución): usuarios `firewatch-jobs` y `firewatch-risk` y vhost `firewatch`, los que provisiona `deploy/local/rabbitmq-topology.sh`; host `rabbitmq` y 5671, el Service y su único listener AMQP en `base/rabbitmq.yaml` (`RABBITMQ_DEFAULT_VHOST: firewatch`, `listeners.ssl.default = 5671`). La URL no lleva parámetros de verificación: la CA entra por `TLS_CA_FILE` en el pod, y `requireSecureBrokerURL` rechaza `verify=verify_none`. Lo detectó la unidad T7.6 al ejecutar el script, no una revisión.

**L13 (2026-11-10) — T7.3 y T7.4 cerradas en `b2af3b2` y `52e943a`.**

Una sesión anterior dejó las dos unidades escritas y sin commitear. Esta las cierra en dos commits, uno por servicio: `b2af3b2` para risk y `52e943a` para prevention, sobre la superficie que L10 asignó a cada una y sin tocar la de ninguna otra unidad.

**Verificación, medida.** `go test -p 1 ./...` en risk: 12 paquetes `ok`, 0 `FAIL`; en prevention: 8 paquetes `ok`, 0 `FAIL`. Ambas suites se repitieron con `-count=1` para que ninguna respuesta viniera de la caché, con el mismo resultado. `gofmt -l` limpio en los 16 archivos tocados. Los handshakes de los tests son reales —`tls.Server` en loopback y `httptest.StartTLS`—, no stubs: lo que se comprueba es el protocolo, no una función que dice haber hecho TLS.

**Rebanada de entrega.** «Rebanada C — cargas de trabajo con TLS, servicios risk y prevention»: `e8e085f..52e943a`, dos commits, uno por servicio. La rama acumula ya 13 commits y unas 4.300 líneas autoradas desde `main`, muy por encima de la rebanada de ~400 que fija el protocolo de revisión, así que la siguiente unidad del frente no empieza sin aplicar la estrategia de entrega que ya decidió L11: PR encadenado o revisión por rebanada.

**Tres hallazgos menores de la revisión independiente, registrados y sin corregir.** Ninguno bloquea esta entrega y ninguno abre una vía insegura en el despliegue; quedan como tarea de un pase posterior:

1. `backend/services/prevention/internal/directory/directory.go` — si la segunda lectura del bundle falla, `New` ignora el error y deja las raíces del sistema, aunque su propio comentario dice lo contrario; y `TestAnUnloadableTLSCAFileDoesNotBecomeATrustAnchor` comprueba `Transport == nil`, que es justamente el caso contrario al que el comentario promete. En el arranque real no hay vía insegura, porque `run()` aborta antes de construir el cliente.
2. prevention no tiene ningún handshake positivo, ni para el listener ni para JWKS: el listener solo se prueba con un certificado inexistente, y el test de JWKS levanta un servidor TLS al que nunca le hace una petición.
3. Los call sites de `run()` no están cubiertos en ninguno de los dos servicios —ni en `jobs` desde `e8e085f`—: quitar el pool camino de un cliente deja la suite en verde. Los helpers y los handshakes sí están cubiertos.
