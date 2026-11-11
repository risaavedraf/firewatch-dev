# Cierre de entrega de Kubernetes — overlay, métricas y borde

**Estado:** abierto y en ejecución.
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
| T1 | Overlay de entrega con digests: las imágenes del `base` quedan sustituibles sin editar el `base`, y el contrato de render lo comprueba | S1, S5 | `feat/k8s-delivery-overlay` | pendiente |
| T2 | metrics-server en el clúster local y evidencia observada de HPA, con receta reproducible | S2, S5 | `feat/k8s-metrics-hpa` | pendiente |
| T3 | Controlador de ingress, Secret de borde y `proxy-ssl-verify` encendido, con handshake borde→gateway medido | S3, S4, S5 | `feat/k8s-edge-tls` | pendiente |

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
