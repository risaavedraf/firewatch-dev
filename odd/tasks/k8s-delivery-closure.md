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
