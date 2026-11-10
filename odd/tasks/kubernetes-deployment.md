# Kubernetes deployment — v1.0 delivery substrate

**Estado:** abierto y en ejecución.

Requisito externo nuevo: la entrega se monta en Kubernetes. Sustituye el sustrato que fijaba D1 (Compose en una VM) y deja el motor de datos intacto.

## Specs

**S1.** «por pedido de la profe tenemos que usar si o si kubernetes para montar la app por lo que dejemoslonpreparado para kubernetes» — desplegar la aplicación en Kubernetes no es opcional ni una alternativa documentada: es el camino de entrega.

**S2.** «partamos con kibernetes real pero basico, con todas las bases para pasarlo a produccion real en caso de que el cliente nos contrate» — un clúster real, no una simulación: API server real, pods reales, manifiestos con forma de producción. Básico en operación, no en atajos.

**S3.** «me quedo con ti tercera opción, todo en el cister y documentado» — PostgreSQL con PostGIS y RabbitMQ corren dentro del clúster, y el reemplazo por servicios gestionados queda documentado con su costo y sus condiciones.

## Tasks

| ID | Tarea | Spec | Ruta | Commit |
| --- | --- | --- | --- | --- |
| T1 | Elegir y levantar el clúster local real; registrar la decisión | S2 | inline | pendiente |
| T2 | Bootstrap de secretos fuera del repositorio | S1 | inline | pendiente |
| T3 | PostgreSQL + PostGIS como StatefulSet con almacenamiento, init de roles y bases | S3 | worker | pendiente |
| T4 | RabbitMQ como StatefulSet con persistencia y gestión acotada | S3 | worker | pendiente |
| T5 | Jobs de migración en el orden correcto (risk → prevention → jobs) | S3 | worker | pendiente |
| T6 | Job que aprovisiona broker e identidades con el script existente | S3 | worker | pendiente |
| T7 | Deployments de la aplicación contra los secretos reales, sin `replace-before-deploy` | S1 | worker | pendiente |
| T8 | Ingress y TLS del borde exterior | S2 | worker | pendiente |
| T9 | Validación end-to-end en el clúster, con evidencia | S1 | inline | pendiente |
| T10 | Camino a producción real documentado y enmienda a D1 | S2, S3 | inline | pendiente |

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
