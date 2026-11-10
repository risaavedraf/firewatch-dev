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
| T7.1 | **T7.1 — Gateway: TLS de entrada, upstreams HTTPS y CA de confianza.** Superficie disjunta: en paralelo con T7.2–T7.5 | S1 | inline | Hecho en `300fa29` |
| T7.2 | **T7.2 — jobs: TLS de entrada, AMQP con CA propia y JWKS con CA.** Superficie disjunta: en paralelo con T7.1 y T7.3–T7.5 | S1 | inline | Hecho en `c42f254` |
| T7.3 | **T7.3 — risk: TLS de entrada, AMQP con CA propia, JWKS y cliente de inferencia con CA.** Superficie disjunta: en paralelo con T7.1, T7.2, T7.4 y T7.5 | S1 | inline | Hecho en `4e4cd7b` |
| T7.4 | **T7.4 — prevention: TLS de entrada en 8084, JWKS y cliente administrativo del directorio con CA.** Superficie disjunta: en paralelo con T7.1–T7.3 y T7.5 | S1 | inline | Hecho en `6662ac4` |
| T7.5 | **T7.5 — inference: TLS de entrada.** Superficie disjunta: en paralelo con T7.1–T7.4 | S1 | inline | Hecho en `e1f36a0` |
| T7.6 | **T7.6 — secretos de runtime y certificados por servicio.** Superficie disjunta: en paralelo con T7.7, T7.8 y con T7.1–T7.5 | S1 | inline | Hecho en `fcbabaf` |
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

**L14 (2026-11-10) — estrategia de entrega: `stacked-to-main`, decidida y registrada; los PRs no están publicados.**

**Decisión del responsable: `stacked-to-main`,** tras una sola pasada de troceado. Siete rebanadas contiguas, en orden, cada una en su propio PR contra `main` (`babf7d3`), revisables y reversibles por separado. Contiguas significa que no se reescribe el historial: cada commit pertenece a exactamente una rebanada y la frontera entre dos rebanadas es un SHA que ya existe. El plan completo —tabla de las siete, diagrama de dependencias y regla de mecanización— queda en «Plan de entrega (stacked-to-main)».

**Cuatro rebanadas superan la guía de ~400 líneas y no se trocean más: la 3, la 5, la 6 y la 7** —527, 874, 1.079 y 557 líneas—. Cada una es un comportamiento completo, un servicio que sirve TLS y confía en la CA interna, y partirla obligaría a partir `main.go` entre dos commits: el estado intermedio dejaría el listener ya cableado y los clientes todavía sin CA, un servicio que arranca y no puede hablar con nadie. El corte por comportamiento se sostiene; el corte por línea, en esas cuatro, no.

**Publicar los PRs queda pendiente de autorización del responsable.** Se registra el plan, no se abre nada: `push`, creación de PR y merge no son parte de la autorización de implementación, igual que quedó escrito en L11.

**L15 (2026-11-10) — el plan de L14 lo corrigió su propia ejecución tres veces; las ocho rebanadas se publicaron con `patch-id` idéntico.**

**Lo que se creía y lo que pasó.** L14 se escribió antes de publicar nada y quedó falso en tres puntos. El plan de «Plan de entrega (stacked-to-main)» es ya el corregido, y esto es lo que cambió:

1. **Eran ocho rebanadas, no siete.** El cierre documental del frente no cabía dentro del código de prevention: se separó en su propia rebanada, `bf6af99` + `73611a7`, 2 commits y 67 líneas autoradas (39 + 28), publicada como PR 8. La 7 queda con lo que de verdad entrega: prevention sirviendo TLS, `52e943a` y sus 557 líneas.
2. **La rebanada 4 no aplica sobre `main`.** Su parche, `9f225e2`, reescribe `backend/deploy/k8s/bootstrap-secrets.sh`, que `dd9a542` —rebanada 1— ya había modificado. Su base real es la rama del PR 1, `eede268`, y el PR 4 se abrió contra esa rama.
3. **La rebanada 8 tampoco aplica sobre `main` ni sobre la rama del PR 1 sola.** Sus dos commits son documentales sobre `odd/tasks/kubernetes-deployment.md`, y `9f225e2` también escribió ahí. Su base real es la rama del PR 4.

**Las rebanadas 2, 3, 5, 6 y 7 sí nacen de `main` limpiamente**, como decía L14: la corrección alcanza al número de rebanadas y a las bases de la 4 y de la 8, no a esas cinco.

**Verificación antes de publicar, rebanada por rebanada.** Cada diff publicado se comparó con el parche original mediante `git patch-id --stable`: las ocho dan el mismo id, así que el diff publicado es idéntico al escrito en el frente y ninguna rebanada perdió ni ganó cambios al cambiar de base.

**PRs publicados.** 1 = #57, 2 = #58, 3 = #59, 4 = #60 (base: rama del PR 1), 5 = #61, 6 = #62, 7 = #63 y 8 = esta rama. Por eso la 📍 desapareció de la tabla: no queda ninguna rebanada pendiente de publicar.

**L16 (2026-11-10) — las ocho rebanadas entraron en `main`; `main` queda en rojo por tres defectos que no vienen de ellas.**

**Fusión, en este orden.** `f369b43` (#57) → `e077ef4` (#60) → `f4f850c` (#64) → `1fc564f` (#58) → `4afdba6` (#59) → `0bf7413` (#61) → `14b65f7` (#62) → `c0eedd4` (#63). Cabeza de `main` tras el frente: `c0eedd4`.

**Las rebanadas 4 y 8 se reapuntaron antes de fusionarlas.** Las dos tenían como base ramas de otros PRs —la 4 la del PR 1 y la 8 la del PR 4—, así que se reapuntaron a `main` con `gh pr edit --base main` antes del merge. Por eso el orden de fusión no es el de publicación: entra primero la 4 y solo después la 8, y de ahí 57 → 60 → 64 → 58, 59, 61, 62, 63.

**Se usaron merge commits, no squash.** Este documento cita SHAs concretos rebanada por rebanada, y aplastar los commits los dejaría huérfanos: ningún corte de la tabla de «Plan de entrega (stacked-to-main)» se podría comprobar contra la historia. El merge commit conserva el límite de cada rebanada.

**Verificación de que `main` tiene todo el contenido.** `git diff --stat origin/main feat/k8s-bootstrap-jobs` nombra un solo archivo, `odd/tasks/kubernetes-deployment.md`, con 19 inserciones y 48 borrados al ir de `main` a la rama; la diferencia va a favor de `main`, que tiene la versión corregida del documento. No queda ningún archivo de código en la rama sin entrar en `main`.

**Tres defectos dejan `main` en rojo, y ninguno lo introdujeron estas rebanadas.**

1. `backend/services/risk/internal/messaging/broker_test.go` no pasa `gofmt`: una línea en blanco doble antes de `TestPublishesOnItsOwnExchange`. Es el único archivo de todo el backend que lo incumple, y ya fallaba en `babf7d3`, antes del frente.
2. El workflow `Backend integration` falla en `main` desde antes del frente. Está sin diagnosticar: el log solo dice `exit code 1`.
3. El job `compose` falla los contract tests del directorio de prevention —`TestUsersContractSuccessResponse`, `TestUsersContractUnauthorizedResponse`, `TestUsersContractForbiddenResponse`, `TestUsersContractDirectoryUnavailableResponse`, `TestUsersContractDirectoryUpstreamFailedResponse`, `TestDirectoryUserSchemaRejectsAnUndocumentedField`, `TestCreateActionContractAssignmentResponses` y `TestPreventiveActionSchemaDocumentsBothAssignments`— con `directory_not_configured` y `directory_read_failed`: el harness del stack no le pasa al servicio el endpoint del directorio de Keycloak. Vienen del trabajo del directorio (`ac2ce05`), no de TLS.

**Consecuencia operativa.** Con `main` rojo, un fallo nuevo no se distingue de uno heredado sin mirar si ese workflow ya fallaba en un commit anterior. Cualquier lectura de CI sobre este frente empieza por la historia del workflow, no por el color del check.

**L17 (2026-11-10) — los SHA que cita este documento no son los que viven en `main`: mapa de traducción.**

**Qué pasó.** Las rebanadas 2, 3, 5, 6 y 7 se publicaron con `git cherry-pick` sobre `main` —así lo fija la mecanización de «Plan de entrega (stacked-to-main)»—, así que el commit que quedó en `main` es el mismo parche con SHA distinto del original que este documento cita. Las rebanadas 4 y 8 tampoco conservaron el suyo: cada una se repobló sobre su base real, la 4 sobre `eede268` y la 8 sobre `fcbabaf`, porque sus parches no aplicaban sobre `main`. La rebanada 1 se fusionó sin cherry-pick, así que sus SHA sí son los originales: `dd9a542`, `63a5124`, `51713e5`, `9ae67e1`, `7c94e02`, `54734c9`, `eede268`.

**Consecuencia.** Quien busque en `main` un SHA citado en L13 —`b2af3b2`, `52e943a`—, en L15 —`9f225e2`, `bf6af99`, `73611a7`— o en la columna «Commits incluidos» de la tabla de entrega, no lo encuentra. Esas entradas y esa tabla se quedan como están: son registro histórico de lo que se publicó, no una descripción de `main`. La columna **Estado** de la tabla de tareas sí cita ya el SHA de `main` en T7.1–T7.6.

**El mapa.** SHA citado en el documento → SHA que sí existe en `main`:

- `74145c6` → `e1f36a0` (inference)
- `069b418` → `300fa29` (gateway)
- `9f225e2` → `fcbabaf` (certificado por servicio y entorno runtime)
- `e8e085f` → `c42f254` (jobs)
- `b2af3b2` → `4e4cd7b` (risk)
- `52e943a` → `6662ac4` (prevention)
- `bf6af99` → `d64e667` (cierre documental)
- `73611a7` → `f2f695e` (plan de entrega)

**Verificación.** `git merge-base --is-ancestor` sobre los dieciséis SHA del mapa: los ocho citados **no** son ancestros de `main` y los ocho reemplazos **sí**; los siete de la rebanada 1 también, que es lo que la mantiene fuera del mapa. Cada par da el mismo `git patch-id --stable`, así que el commit de `main` es el mismo parche y no un commit vecino.

## Plan de entrega (stacked-to-main)

Ocho rebanadas **contiguas**, en este orden, cada una en su propio PR. **Las ocho están ya fusionadas en `main`, con la cabeza `c0eedd4`**, así que este plan describe historia consumada y no trabajo pendiente; el orden de fusión y lo que quedó abierto está en L16. No se reescribe historial: cada commit pertenece a una sola rebanada y la frontera entre dos rebanadas es un SHA existente. **Las ocho ya están publicadas** —la 1 es #57, la 2 #58, la 3 #59, la 4 #60, la 5 #61, la 6 #62, la 7 #63 y la 8 esta rama—, así que la 📍 que marcaba «la siguiente a publicar» desaparece: no hay siguiente. La columna **Base** dice la rama sobre la que cada rebanada aplica de verdad, que no es `main` en las dos que comparten archivos con una anterior.

| PR | Base | Rango de commits | Commits incluidos | Líneas cambiadas | Unidad de comportamiento que entrega |
| --- | --- | --- | --- | --- | --- |
| 1 | `main` | `babf7d3..eede268` | 7 — `dd9a542`, `63a5124`, `51713e5`, `9ae67e1`, `7c94e02`, `54734c9`, `eede268` | 653 | Jobs de migración y aprovisionamiento del broker, con sus dos correcciones y la documentación del sustrato |
| 2 | `main` | `74145c6` | 1 — `74145c6` | 453 | inference sirve TLS desde el listener dirigido por entorno |
| 3 | `main` | `069b418` | 1 — `069b418` | 527 | gateway sirve TLS y confía en la CA interna |
| 4 | rama del PR 1 (`eede268`) | `9f225e2` | 1 — `9f225e2` | 186 | un certificado por servicio y el entorno runtime |
| 5 | `main` | `e8e085f` | 1 — `e8e085f` | 874 | jobs sirve TLS y confía en la CA para broker y OIDC |
| 6 | `main` | `b2af3b2` | 1 — `b2af3b2` | 1.079 | risk sirve TLS y confía en la CA para broker, OIDC e inferencia |
| 7 | `main` | `52e943a` | 1 — `52e943a` | 557 | prevention sirve TLS y confía en la CA para OIDC y el directorio |
| 8 | rama del PR 4 (`9f225e2`) | `bf6af99` + `73611a7` | 2 — `bf6af99`, `73611a7` | 67 (39 + 28) | cierre documental del frente |

**Dependencias de base.** Sólo cinco rebanadas se pueblan desde `main`: la 2, la 3, la 5, la 6 y la 7. Las otras dos nacen de la rama de una anterior, porque comparten un archivo con ella:

- **4 → rama del PR 1 (`eede268`).** `9f225e2` reescribe `backend/deploy/k8s/bootstrap-secrets.sh`, el mismo script que `dd9a542` modificó en la rebanada 1. Aplicado sobre `main`, ese es el parche que conflictúa.
- **8 → rama del PR 4 (`9f225e2`).** `bf6af99` y `73611a7` son documentales sobre `odd/tasks/kubernetes-deployment.md`, el mismo archivo que `9f225e2` también toca al dejar escrito el contrato de L10. Ni sobre `main` ni sobre la rama del PR 1 sola aplica limpia: le falta ese contexto.

**Y una arista lógica, que no es de base:** la 5 depende de la 4 porque los servicios necesitan el certificado por servicio que emite el aprovisionador.

```text
main  babf7d3  (merge-base con el frente)
 │
 ├── PR 1  babf7d3..eede268   sustrato: Jobs de migración y aprovisionamiento del broker
 │    └──(base)──▶ PR 4  9f225e2   certificado por servicio + entorno runtime
 │         └──(base)──▶ PR 8  bf6af99 + 73611a7   cierre documental del frente
 │
 ├── PR 2  74145c6            inference
 ├── PR 3  069b418            gateway
 ├── PR 5  e8e085f            jobs (depende de la 4 por comportamiento, no por base)
 ├── PR 6  b2af3b2            risk
 └── PR 7  52e943a            prevention
```

**Mecanización.** Una rama por rebanada, y la base de cada una no la elige el orden de publicación sino el archivo que comparte con una anterior:

- La rebanada 1 es la rama original recortada en su SHA final, `eede268`: se abre tal cual, sin reescribir historial.
- Las rebanadas 2, 3, 5, 6 y 7 nacen de `main` y se pueblan con `git cherry-pick` de su commit.
- La rebanada 4 nace de la rama del PR 1: `9f225e2` reescribe `backend/deploy/k8s/bootstrap-secrets.sh`, que `dd9a542` ya había modificado, así que ese parche es el que conflictúa sobre `main`.
- La rebanada 8 nace de la rama del PR 4: sus dos commits tocan `odd/tasks/kubernetes-deployment.md`, que `9f225e2` también modificó al dejar escrito el contrato de L10, así que ese es el parche que conflictúa sobre `main` y sobre la rama del PR 1 sola.
- Cada rama encadenada se rebasa sobre `main` cuando la anterior se fusiona: la 4 queda en `main` tras el merge del PR 1 y la 8 tras el del PR 4.

Nada se aplana ni se reordena: la frontera de cada PR sigue siendo un SHA existente. El diff publicado se comparó con el original rebanada por rebanada con `git patch-id --stable`, y las ocho dan el mismo id.

**L18 (2026-11-10) — el defecto 2 de L16 queda diagnosticado y cerrado: no faltaban certificados, faltaba `APP_ENV` en un servicio.**

**El diagnóstico que faltaba.** Los runs `42895529340` (push) y `42895560420` (PR) sobre `46e8ff1` dejan `Frontend`, `Risk` y `Prevention` en verde y solo `Backend integration` en rojo, y dentro de él un único job, `compose`. El paso que falla es `docker compose up -d --build`, y el mensaje es de Compose, no del servicio:

```text
dependency failed to start: container firewatch-inference-1 is unhealthy
```

La causa no está en ese mensaje sino en el log del contenedor, que solo imprime el paso `Migration diagnostics on failure`:

```text
{"level":"ERROR","msg":"service_stopped","error":"TLS_CERT_FILE is required outside development: set TLS_CERT_FILE and TLS_KEY_FILE, or run with APP_ENV=development for plaintext"}
```

**Por qué solo inference.** Cinco servicios tienen el guard de transporte —`gateway`, `jobs`, `prevention`, `risk` e `inference`— y los cinco exigen el literal `development` para aceptar texto plano. Cuatro de ellos fusionan el ancla `*oidc`, que declara `APP_ENV: development`; `inference` no toma OIDC, no fusiona el ancla, y era el único que nunca recibía la variable. El guard estaba escrito esperando que el stack local la declarara —su propio comentario dice "the literal value the local stack sets"—, así que el defecto fue una omisión de cableado en `docker-compose.yml`, no un certificado ausente.

**La corrección.** `APP_ENV: development` en el bloque `environment` de `inference`, con el comentario que explica por qué lo declara sin heredar el ancla, para que nadie lo borre al limpiar. No se generan certificados en CI: el stack de desarrollo sirve texto plano a propósito, igual que los otros cuatro servicios, y el stack de producción es un archivo aparte que no incluye `inference`.

**Por qué no lo cazó nada.** El defecto llegó a `main` porque ninguna prueba miraba el cableado de entorno de Compose: el guard tiene pruebas propias y el arranque tiene su smoke, pero ninguno nota que un servicio nunca recibe la variable. `backend/scripts/test_ci_migration.py` gana `TlsGuardEnvironmentTest`, que deriva del código la lista de servicios con el guard y exige que cada uno reciba `development`, sea en su propio bloque o por el ancla. Corre en el workflow antes de `docker compose up`, sin daemon. Se escribió primero y falló nombrando exactamente un servicio, `inference`.

**Verificación.** `python3 -m unittest discover -s scripts -p test_ci_migration.py`: 18 pruebas, todas en verde. `docker compose config --quiet`: válido, y el bloque de `inference` muestra `APP_ENV: development`. El daemon de Docker no estaba corriendo en la máquina donde se hizo el arreglo, así que no hubo prueba con contenedores: la prueba de extremo a extremo es el run de CI sobre esta rama.

**L19 (2026-11-10) — el cuarto rojo no era el mismo defecto: `broker-topology` pierde una carrera contra la API de administración.**

**Lo que reveló el run.** `60befd0` corrige `inference` —el contenedor pasa a `Healthy`— y el mismo commit produce dos resultados: el run de `pull_request` (`42899423939`) en verde y el de `push` (`42899415104`) en rojo. No es una diferencia de código: los dos prueban el mismo árbol. El fallo es otro, y con la prueba de `inference` ya cerrada queda a la vista:

```text
Container firewatch-broker-topology-1 Error service "broker-topology" didn't complete successfully: exit 1
```

Su log, otra vez solo visible en `Migration diagnostics on failure`, dice:

```text
provisioning: exchanges
provisioning: PUT /exchanges/firewatch/firewatch.jobs.events: request failed
```

**La carrera.** `request failed` es la rama de transporte, no un código HTTP: la primera petición murió sin respuesta. El healthcheck del broker contesta por el lado AMQP, así que `depends_on: rabbitmq: service_healthy` libera el aprovisionador mientras el plugin de administración todavía está abriendo el 15672. En el run que pasó, el aprovisionador arrancó con el broker ya asentado; en el que falló, arrancó 0,2 s después del verde y chocó. `set -eu` convertía ese tropiezo de arranque en un stack caído.

**La corrección.** `request` reintenta solo el fallo de transporte, acotado a 30 intentos de un segundo, y escribe el motivo de curl una sola vez al agotarlos: un broker lento ya no tumba el `up`, y un broker que nunca responde falla con su razón en vez de con un contador. Un código HTTP no se reintenta nunca —un 409 que significa que el objeto existente discrepa, o un 401 de credenciales, deben fallar al primer intento—. El `DELETE` del exchange legado pasa por el mismo camino, porque corría al final sin red y un broker reiniciado a mitad lo habría tumbado sin decir nada.

**Verificación.** Tres pruebas nuevas en `BrokerTopologyRetryTest`, con un `curl` falso que es el único del `PATH` del hijo y puede rechazar las primeras N conexiones: el reintento llega a buen término, el reintento se detiene en el límite y conserva el motivo, y un 409 falla al primer intento. Contra el script anterior las dos primeras fallan y la tercera pasa, que es lo que debe hacer: fija el comportamiento que ya existía. Las 21 pruebas del archivo quedan en verde.

**Nota de plataforma.** MSYS/git-bash reescribe un valor de entorno que empieza por `/` como ruta de Windows antes de que lo vea el hijo, así que el patrón del 409 se nombra sin barra inicial. Es una trampa del harness, no del script.

**Frontera de la rebanada.** Los cuatro commits de esta rama —`46e8ff1`, `cd10c4f`, `9f51b4f` y este— son una sola rebanada de revisión: la rama `fix/ci-green` y su PR #65. Supera el umbral de 400 líneas contando pruebas y documento, y no se parte porque los tres rojos y su causa comparten una sola verificación: el run de `Backend integration` sobre el mismo workflow. Partirla dejaría tramos cuyo verde no se puede leer sin el anterior.
