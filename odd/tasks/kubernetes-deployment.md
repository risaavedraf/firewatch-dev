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
| T7 | Deployments de la aplicación contra los secretos reales, sin `replace-before-deploy` | S1 | inline | Pendiente: hoy el gateway no arrancaría (ver L6) |
| T8 | Ingress y TLS, incluidos los listeners internos | S2 | inline | Pendiente |
| T9 | Validación end-to-end en el clúster, con evidencia | S1 | inline | **Bloqueada**: sin clúster no hay nada que validar |
| T10 | Camino a producción real documentado y enmienda a D1 | S2, S3 | inline | Pendiente |

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
