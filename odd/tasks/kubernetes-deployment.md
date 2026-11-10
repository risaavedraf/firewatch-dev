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
| T5 | Jobs de migración en el orden correcto (risk → prevention → jobs) | S3 | inline | Siguiente |
| T6 | Job que aprovisiona broker e identidades con el script existente | S3 | inline | Siguiente |
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
