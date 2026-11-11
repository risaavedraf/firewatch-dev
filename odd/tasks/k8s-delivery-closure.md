# Cierre de entrega de Kubernetes — overlay, métricas y borde

**Estado:** cerrado en su alcance el 2026-11-11 — T1, T2 y T3 fusionadas (#68, #69, #70). Sigue fuera el pendiente 5 de L21: el entorno objetivo.
**Origen:** los cinco pendientes que deja L21 de `odd/tasks/kubernetes-deployment.md`, autorizados en bloque en esta sesión. El pendiente 5 (entorno objetivo) no entra: requiere acceso que esta sesión no tiene.

## Specs

**S1.** «Overlay de entrega con digests. Es la cláusula de T7 que sigue abierta y la única que impide declarar T7 cerrada: hoy los Deployments llevan `replace-before-deploy`. Cierra cuando un overlay fija imágenes por digest y el `base` no necesita sustitución manual.»

**S2.** «metrics-server (o equivalente). Los HPA del `base` fallan con `unable to fetch metrics from resource metrics API` en un clúster sin métricas; hasta que exista, `V1-F-06` no puede observar escalado.»

**S3.** «Controlador de ingress y el Secret `firewatch-tls`. Sin eso el tramo borde→gateway no se ejercita; lo verificado es la anotación `backend-protocol: HTTPS`, no un handshake. Cierra cuando una petición atraviesa el ingress hasta el gateway por TLS.»

**S4.** «`proxy-ssl-verify` encendido. Hoy el tramo ingress→gateway va cifrado pero no autenticado. Necesita que `bootstrap-secrets.sh` emita un Secret con `tls.crt` (la CA interna) y `tls.key` no vacía, que es la forma que exige `nginx.ingress.kubernetes.io/proxy-ssl-secret`; `firewatch-ca` trae solo `ca.crt`, así que no sirve tal cual.»

**S5.** «Todo lo que afirmes acá necesita evidencia del tipo de L20.» Un render limpio no acredita un despliegue: el defecto de Keycloak pasó manifiesto válido, render limpio, contrato en verde y admisión `restricted`, y solo murió al arrancar el contenedor. Cada tarea cierra con evidencia observada, no con un `kubectl kustomize` exitoso.

## Tasks

| ID | Tarea | Spec | Rama | Estado |
| --- | --- | --- | --- | --- |
| T1 | Overlay de entrega con digests: las imágenes del `base` quedan sustituibles sin editar el `base`, y el contrato de render lo comprueba | S1, S5 | `feat/k8s-delivery-overlay` | Hecho en `1f1978a` — PR #68. Trece pods en `Running` y ambos Jobs en `Complete` sirviendo los cinco digests reales |
| T2 | metrics-server en el clúster local y evidencia observada de HPA, con receta reproducible | S2, S5 | `feat/k8s-metrics-hpa` | Hecho — PR #69. Escalado de `gateway` de 2 a 6 réplicas con evento `SuccessfulRescale` y vuelta a 2 al restaurar el objetivo |
| T3 | Controlador de ingress, Secret de borde y `proxy-ssl-verify` encendido, con handshake borde→gateway medido | S3, S4, S5 | `feat/k8s-edge-tls` | Hecho — PR #70. `401` del gateway a través del borde, `proxy_ssl_verify on` en el render, y `502` con ancla equivocada antes de restaurar |

**Dependencia de base.** Las tres ramas son una cadena: T2 nace de T1 y T3 de T2. Comparten `odd/tasks/k8s-delivery-closure.md` y, T2 y T3, el README de despliegue; ramificar las tres desde `main` garantiza conflicto en esos dos archivos. La cadena conserva un documento y un README con historia lineal y cada PR muestra solo su tramo.

**Fuera de esta cadena.** El pendiente de CI (acciones que todavía apuntan a Node 20) vive en `odd/tasks/ci-actions-refresh.md` y en la rama `chore/ci-actions-refresh`, desde `main`, para no compartir archivos con este frente.

**Ninguna rama se fusiona sin decisión humana.** El merge a `main` queda pendiente del responsable.

## Log

**L1 (2026-11-10) — autorización.** «puedes hacer todo en diferentes ramas porfa» — los pendientes 1 a 4 de L21, cada uno en su rama; el pendiente 5 queda fuera por falta de acceso al entorno objetivo.

**L2 (2026-11-10) — contexto recibido.** Handoff de la sesión anterior sobre `main` en `02d946d`, árbol limpio, PR #66 fusionado. Trampas heredadas que no hay que redescubrir: en Git Bash el bootstrap exige un shim de `mktemp` y `MSYS_NO_PATHCONV=1` solo para los certificados; el namespace nace con `pod-security.kubernetes.io/enforce: restricted`; `kubectl apply --dry-run=server` sobre todo el `base` necesita el Namespace ya creado. Verificación rápida antes de commitear: `bash backend/deploy/k8s/tests/render-contract.sh` y `kubectl kustomize backend/deploy/k8s/base` (48 objetos).

**L3 (2026-11-10) — T1 implementado; frontera de la rebanada.** `1f1978a` entrega el overlay, `pin-images.sh`, el contrato ampliado y el README: 460 líneas contadas, sobre el umbral de 400. No se parte: las cuatro piezas son una sola afirmación —el overlay pinea y el base sigue siendo plantilla— y el contrato falla si falta cualquiera de las dos mitades, así que un tramo suelto dejaría un verde que no se puede leer sin el siguiente. El documento y su plan viajan en el commit anterior, `2308bca`.

Los digests del overlay son los del índice OCI de las imágenes que el clúster de evidencia descarga, y no dependen del host que las sirve. Por eso el único valor que el árbol no puede derivar es el host, y queda como `REPLACE_WITH_REGISTRY_HOST` hasta que `pin-images.sh` lo reescribe desde una lista de imágenes.

**L4 (2026-11-10) — T1 validado en el clúster, no en el render.**

kind v1.35, contexto `kind-firewatch-verify`, bootstrap real (15 Secretos y 5 ConfigMaps), un registro local con las cinco imágenes. `pin-images.sh` reescribió host y digests desde una lista, y `kubectl apply -k backend/deploy/k8s/overlays/delivery` desplegó el resultado. Antes del apply se probó la descarga de los cinco digests con `crictl pull` sobre el nodo, para que un digest inválido fallara ahí y no dentro de un pod.

**Lo observado.** Trece pods en `1/1 Running` y ambos Jobs en `Complete`, con `SuccessCriteriaMet=True` y `Complete=True` (`reason=CompletionsReached`). Los seis Deployments en su réplica deseada: gateway 2/2, jobs 2/2, risk 2/2, prevention 2/2, inference 2/2, keycloak 1/1. Cero imágenes con `replace-before-deploy`: los cinco sirven `kind-registry:5000/firewatch/<servicio>@sha256:<digest>`. El overlay quedó idéntico a `HEAD` al terminar la corrida (`git diff --quiet` limpio), así que el pin real vive en la lista de imágenes que pasa el operador.

**Dos hallazgos que la corrida dejó, y que no se disimulan.** (1) `kubectl wait --for=condition=Ready pod --all` no sirve como criterio de aceptación en un namespace con Jobs: el pod de un Job terminado queda `Ready=False` para siempre y el comando sale 1 con el despliegue sano. La lectura correcta es por workload. (2) En un clúster frío los Jobs arrancan antes de que postgres y rabbitmq respondan: `broker-provision` falló una vez con `curl: (7) Failed to connect to rabbitmq port 15671` y `migrate-v14` tres veces con el initContainer `migrate-risk` en `exitCode=1`. Los dos Jobs terminaron `Complete` por reintento, pero ningún manifiesto garantiza ese orden.

**Lo que sigue abierto de S1.** El `base` ya no necesita sustitución manual; el overlay necesita una sola, el host del registro, y es mecánica y revisable. Eso es lo que S1 pedía y lo que queda dicho: no se declara sustitución cero, se declara una sustitución en un solo lugar.

**HPA, para T2.** Las cuatro leen métricas reales (`cpu: 1%/70%` y `4%/70%`), con `Available=True` en el APIService `v1beta1.metrics.k8s.io`. Ninguna llegó a escalar: las cuatro están en `ScalingLimited=True` con `reason=TooFewReplicas`, que es `V1-F-06` todavía sin observación de escalado.

**L5 (2026-11-10) — T2 cerrado con un escalado observado, no con métricas simplemente presentes.**

`backend/deploy/k8s/local/metrics-server.sh` instala la release pinneada (`v0.9.1`, nunca `latest`), agrega `--kubelet-insecure-tls` con la razón escrita al lado —el kubelet de kind sirve un certificado autofirmado que la CA del clúster no firmó, así que verificarlo es imposible y esa bandera es la concesión del clúster local, no un ajuste de producción—, espera el rollout y falla con un motivo nombrado si `v1beta1.metrics.k8s.io` no queda `Available=True` o si `kubectl top nodes` no responde. Es idempotente: una segunda corrida no cambia la generación del Deployment ni reinicia el pod.

**Lo observado.** Antes: `gateway cpu: 1%/70% 2 6 2`. Con el objetivo bajado a 1% —técnica de prueba, nunca un cambio en `base/autoscaling.yaml`— y carga real por `port-forward` en el puerto 8080 del pod (el 8443 no es puerto de contenedor) contra `/health/ready`: `gateway cpu: 18%/1% 2 6 6`, `gateway 6/6`, y el evento `SuccessfulRescale New size: 6; reason: cpu resource utilization (percentage of request) above target`. Las condiciones en ese momento: `ScalingActive=True ValidMetricFound` y `ScalingLimited=True TooManyReplicas`. El escalado llegó 60 s después de cruzar el umbral, que es la ventana de estabilización que declara el `base`. Restaurado el objetivo a 70, la lectura volvió a `2` con `New size: 2; reason: All metrics below target`.

**Alcance honesto.** Solo se ejercitó `gateway`; los otros tres leen la misma métrica por la misma API y quedan en `TooFewReplicas`, pero su escalado no se observó por separado. Y con el objetivo declarado de 70%, un lazo de `curl` por un `port-forward` no alcanza para seis réplicas: esto prueba la tubería de métricas y el mecanismo de escalado, no que 70% sea alcanzable con tráfico real. `V1-F-06` queda con observación de escalado; la prueba de carga sigue siendo trabajo aparte.

**Una trampa del overlay que conviene tener escrita.** `kubectl apply -k backend/deploy/k8s/overlays/delivery` sobre el árbol commiteado re-apunta los cinco Deployments a `REPLACE_WITH_REGISTRY_HOST`, que ningún nodo puede descargar. El overlay se aplica después de `pin-images.sh`, nunca antes; para tocar solo los HPA, el camino es `-f base/autoscaling.yaml`.

**L6 (2026-11-10) — T3 cerrado, y dos premisas de L21 que el clúster corrigió.**

L21 afirmaba dos cosas sobre el borde que la medición desmiente. Primera: que `firewatch-ca` no sirve tal cual porque solo trae `ca.crt`. Con `controller-v1.15.1` sí sirve: referenciarlo directo renderiza `ca-firewatch-firewatch-ca.pem` y la verificación pasa. Segunda, y más importante: que `proxy-ssl-secret` exige `tls.crt` con la CA y un `tls.key` no vacía. Esa forma se rechaza (`certificate and private key does not have a matching public key`) y, al fallar la anotación, **el bloque `ProxySSL` completo desaparece del render**: la configuración falla abierto, no cerrado, y el tramo queda cifrado pero sin verificar sin que nada lo diga. La forma que el controlador acepta es `ca.crt` sola.

**La decisión.** `bootstrap-secrets.sh` emite `gateway-upstream-ca` con `ca.crt` únicamente, en lugar de referenciar `firewatch-ca`. El ancla queda explícita, nombrada por el Ingress y verificable desde el contrato; el costo está escrito al lado: rotar la CA obliga a recrear ese Secret, y hasta entonces la verificación falla cerrado.

**Lo observado.** Petición desde el host, resolviendo el nombre al puerto mapeado, con el certificado de borde autofirmado (`CN=firewatch.example.invalid`, `SAN DNS:firewatch.example.invalid`, TLSv1.3): `http=401`, que es la respuesta JSON del gateway y no una página de NGINX, así que el tramo llega. En el controlador, línea 354 del render: `proxy_ssl_verify on`, `proxy_ssl_name gateway`, `proxy_ssl_trusted_certificate /etc/ingress-controller/ssl/ca-firewatch-gateway-upstream-ca.pem`. Con un ancla equivocada: `502` y `upstream SSL certificate verify error: (21:unable to verify the first certificate)`; restaurado el manifiesto y borrado el Secret de prueba, la petición volvió a `401`. Un ancla con nombre sin namespace da `503` con `AnnotationParsingFailed`, no un tramo sin verificar.

**Nota de método, porque me pasó a mí.** Mi primer `grep "proxy_ssl_verify on"` sobre el render dio `0` y por un momento pareció que la anotación no tenía efecto. El render alinea la directiva con espacios de columna: el valor estaba ahí. Medir con un patrón que admita alineación, o se lee un cero como un fallo que no existe.

**Fuera de lo observado.** Solo `/v1/jobs`, sin autenticar, un nodo, un controlador y un pool de gateway. No hay afirmación de disponibilidad, failover ni de petición autenticada de extremo a extremo.

**L7 (2026-11-10) — cierre del frente: las tres tareas quedan hechas y una sigue fuera.**

T1, T2 y T3 cierran con evidencia observada en el clúster, no con renders: el overlay sirve los cinco digests y los pods arrancan sobre ellos (PR #68), el HPA escala de 2 a 6 con un `SuccessfulRescale` medido (PR #69), y una petición cruza el borde hasta el gateway por TLS verificado, con el `502` de ancla equivocada como prueba de que la verificación se aplica (PR #70). La rama de CI sale aparte, en #67, con sus ocho workflows en verde.

**Lo que este frente no cierra, y no por falta de trabajo.** El pendiente 5 de L21 —aplicación al entorno objetivo— sigue abierto: todo lo observado es un clúster local. El realm de Keycloak se sigue aprovisionando fuera de banda. Y las cuatro ramas esperan decisión humana de merge: la cadena es #68 → #69 → #70, y #67 va suelta contra `main`.

**Dos premisas de L21 que quedaron corregidas para quien retome.** `firewatch-ca` sí sirve como ancla tal cual, y la forma `tls.crt` + `tls.key` que L21 suponía obligatoria la rechaza el controlador —peor, al rechazarla desaparece el bloque `ProxySSL` completo y el tramo queda sin verificar en silencio—. L6 lo deja medido; no lo vuelvas a deducir de la documentación.

**Un residuo que conviene saber.** El controlador conserva `ca-firewatch-edge-wrong-ca.pem` y `ca-firewatch-edge-probe-ca.pem` en su directorio de certificados, de los experimentos de ancla: los Secret de prueba se borraron y la anotación viva apunta a `firewatch/gateway-upstream-ca`, pero los archivos quedan hasta el siguiente recargue o hasta que se borre el clúster, que es desechable.

**L8 (2026-11-11) — fusionado, y qué efecto tiene sobre `v1-produccion.md`.**

Las cuatro ramas entraron a `main` en este orden: #67 `4457d38` (CI), #68 `0edf263` (overlay), #69 `2687ec0` (metrics-server y HPA) y #70 `638c55e` (borde TLS). La cadena se fusionó retargeteando cada PR a `main` después del anterior, así que cada diff publicado fue el de su propia unidad.

**Efecto sobre `odd/tasks/v1-produccion.md`.** `V1-F-06` avanza en la única cláusula que este frente podía tocar: «el escalado del HPA observado en clúster» queda observado, con `gateway` pasando de 2 a 6 réplicas y su evento `SuccessfulRescale`. Las otras dos cláusulas de esa tarea siguen abiertas y no se tocan: el reporte pasa/falla de k6 con `p(95)<500`, `max<2000` y `http_req_failed<0.01`, y el RTO medido en ≤ 15 min. `V1-H-06` y `V1-H-11` no se mueven: las dos piden el entorno autorizado, y lo observado aquí es un clúster local.

**Lo que este frente sigue sin cerrar.** El pendiente 5 de L21 (aplicación al entorno objetivo), el realm de Keycloak aprovisionado fuera de banda, `T9` en su cláusula de entorno y `T10` entero.
