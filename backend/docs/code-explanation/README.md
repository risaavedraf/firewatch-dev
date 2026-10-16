# Backend FireWatch: explicación del código y sus límites

El backend implementa una **demostración sintética asíncrona**: un administrador solicita un `job`, `risk` clasifica el lote con una regla propia no oficial sobre entrada sintética y `jobs` registra la finalización. Calcula un nivel canónico (`bajo`/`medio`/`alto`/`desconocido`) con la regla `academic-rule-v1`; no predice fecha ni lugar y no sustituye a CONAF ni SENAPRED. Esta colección explica el código leído, sus decisiones y sus errores; no constituye una ejecución ni una certificación de producción.

**Cierre documental:** los archivos seleccionados se explican estáticamente, mientras que el resto del árbol se clasifica como parcial, contextual o excluido. La verificación estructural reproducible con hashes cubre los archivos no excluidos y el verificador imprime, al ejecutarse, los conteos vigentes de cada estado y de los archivos con hash. Los límites de ejecución y dependencias siguen explícitos.

> **Instantánea desactualizada — declarado el 2026-10-14.** El [verificador](verify_structure.py) termina en **exit 1** contra el árbol actual: hay rangos completos que ya no coinciden con el número de líneas del archivo, archivos fuente que no están en el inventario (por ejemplo `services/risk/internal/source/synthetic_scenarios.go` y `services/risk/migrations/007_synthetic_evaluation.sql`) y `source-snapshot.json` ya no reproduce los hashes del árbol. La tabla de inventario y las citas de las partes describen, por tanto, el árbol de su fecha de re-baseline, **no el actual**. **No se re-baseliniza en esta entrega:** exige autorización explícita y está declarado como deuda abierta en [firewatch-data-contract.md](../../../odd/tasks/backend/firewatch-data-contract.md) y [gateway-frontend-separation.md](../../../odd/tasks/architecture/gateway-frontend-separation.md). Regenerar el manifiesto con `--snapshot` sin releer las citas certificaría como vigentes rangos que no se volvieron a leer. Los conteos no se copian en prosa, por la razón que da la sección «Reconciliación»: los imprime el verificador.
>
> Nota añadida por la entrega de preparación de publicación ([github-repo-prep.md](../../../odd/tasks/publication/github-repo-prep.md)). No modifica los informes históricos ni sus observaciones fechadas.
>
> **Deriva sustantiva — no solo rangos (2026-10-14).** Además de la deriva estructural (rangos, hashes, inventario), la revisión contrastó las afirmaciones con el código y encontró citas desactualizadas en el **contenido**. Ejemplos verificados, no exhaustivos:
> - [Persistencia](06-database.md): describe un modelo de migración única (`Initial`, versión 1) en ambos servicios; el código embebe **3** migraciones en `jobs` y **7** en `risk`, con un runner multi-versión (`services/jobs/cmd/server/main.go:64-85`).
> - [Configuración y despliegue](07-deployment.md): afirma que `/health/ready` responde `204` al tener éxito el ping; en `jobs`/`risk` el éxito es **`200`** y el fallo **`503`** (`live` es `204`; el `204` de readiness es del gateway).
> - [Mensajería](04-messaging.md): lista una espera de `1,2,4,8,16` s «incluso tras quinto fallo»; el cronograma real es `{1,2,4,8}` s, cinco intentos y **sin** espera tras el quinto (`retryBackoff`, `broker.go:245-266`).
> - [Jobs](02-jobs.md): la interfaz `Repository` añade `ZoneExists` y un `Create` variádico (`internal/handler/api.go:19-30`).
> - [Risk](03-risk.md) y [Mensajería](04-messaging.md): el manejador del broker es `publisher.Publish`, no `store.Publish` (`risk/cmd/server/main.go:178`).
> - [Gateway](01-gateway.md): falta la rama de preflight `OPTIONS` que responde `204` (`gateway.go:88-96`).
>
> Estas citas **no se corrigen aquí** sin re-baseline autorizado; se listan para que el lector no las tome por vigentes. Corregir solo los rangos sin releer el texto certificaría como válidas afirmaciones que siguen siendo falsas.

## Ruta de lectura

1. [Gateway y entrada HTTP](01-gateway.md): qué acepta la frontera pública.
2. [Jobs y auditoría](02-jobs.md): aceptación durable e idempotencia.
3. [Risk y demostración](03-risk.md): publicación y consultas de lotes.
4. [Mensajería](04-messaging.md): entrega, confirmaciones, reintentos y duplicados.
5. [Contratos](05-contracts.md): eventos y API declarada.
6. [Persistencia](06-database.md): esquema, permisos y migraciones.
7. [Configuración y despliegue](07-deployment.md): arranque, imágenes y fronteras operativas.
8. [Tests y herramientas](08-tests-tools.md): expectativas leídas, no resultados ejecutados.

## Instantánea analizada

- Raíz: `E:\Duoc\Arquitectura Sistemas IA\FireWatch\backend`; fecha de análisis: **2026-10-06**; re-baseline de la instantánea: **2026-10-11**.
- `git rev-parse --show-toplevel`, `git status --short` y `git rev-parse HEAD`, desde `backend`, devolvieron `fatal: not a git repository (or any of the parent directories): .git`. No hay SHA, rama ni clasificación Git de archivos modificados/no seguidos disponibles.
- CodeGraph se consultó primero con la ruta de `backend`: informó que no existe `.codegraph/`. Se utilizó lectura directa; no se inicializó un índice.
- Identidad de contenido registrada en [source-snapshot.json](source-snapshot.json): SHA-256 de bytes originales, tamaño y líneas de los archivos no excluidos. Se re-baselinó el 2026-10-11 tras corregir inventario y rangos completos. No hay hash de secretos, realm ni go.sum; tampoco diff, historial o atomicidad de captura multiarchivo. Cambios posteriores pueden invalidar citas y serán detectados para estos archivos al volver a verificar.
- El manifiesto `source-snapshot.json` se re-baselinó en este hito: se incorporaron al inventario los archivos que faltaban (paquete `classify/`, `source/`, `service/publisher*`, contratos de observaciones, migraciones 002/003, pruebas asociadas y activos web) y se corrigieron los rangos completos; después `verify_structure.py --snapshot` reescribió hashes, tamaños y líneas. La instantánea **quedó** alineada con el árbol **de ese momento (2026-10-11)**; la nota de cabecera del **2026-10-14** documenta la deriva posterior.
- Se preservaron los informes existentes. El recorrido estructural enumera todos los archivos fuente fuera de esta carpeta; los informes, el manifiesto y su verificador son salida documental y no sustituyen documentos previos.
- No se ejecutó código de aplicación, tests, Docker, migraciones, scripts operativos, instalaciones ni validadores de aplicaciones. Sí se ejecutó el **verificador estructural documental** autorizado; inspecciona archivos, no importa ni llama la aplicación. Las consultas Git son de solo lectura.
- Los paquetes externos Go, la implementación de Keycloak, PostgreSQL/PostGIS, RabbitMQ, Prometheus, Docker y Kubernetes no se inspeccionaron ni ejecutaron. Las llamadas a esas dependencias son fronteras, no prueba de su funcionamiento.

## Mapa de responsabilidades

```text
Authorization: Bearer ...
  -> gateway: verificación, límite local, proxy
  -> jobs: rol admin, validación, transacción job + audit + outbox
  -> relay jobs -> firewatch.events / jobs.requested.v1
  -> consumidor risk -> inbox + batch sintético + outbox
  -> relay risk -> firewatch.events / risk.batch.published.v1
  -> consumidor jobs -> inbox + estado published + audit

viewer/admin -> gateway -> risk latest/history -> batches + frescura calculada
```

Este recorrido se resuelve en `services/jobs/internal/repository/store.go:31-64`, `services/risk/internal/repository/store.go:29-64` y `services/jobs/internal/repository/store.go:89-122`. Los callbacks `Handle` se conectan explícitamente en ambos `cmd/server/main.go:91-94`; no se deducen solo de imports.

## Registro compartido de cobertura

Las rutas siguientes son relativas a `backend/`. Los intervalos son inclusivos y abarcan el archivo completo salvo exclusión. **Explicado** significa explicación estática de las unidades y decisiones locales agrupadas en la parte enlazada. **Parcial** aquí significa que el archivo está inventariado y registrado con hash —se leyó para verificar su rango completo—, pero esta colección **no** lo analiza en detalle: agrupa el código posterior a la redacción original (por ejemplo `classify/`, `source/`, `service/publisher*`, contratos de observaciones, pruebas nuevas y activos web); no es un hueco de explicación pendiente ni una afirmación de cobertura de tests. **Contexto** no pertenece a la explicación ejecutable; **Excluido** no fue leído. Ningún estado significa cobertura de tests ni dominio de bibliotecas externas.

| Archivo inventariado | Rango leído | Estado | Parte / motivo |
| --- | --- | --- | --- |
| `services/gateway/cmd/server/main.go` | 1-63 | Explicado | [Gateway](01-gateway.md) |
| `services/gateway/internal/handler/gateway.go` | 1-121 | Explicado | [Gateway](01-gateway.md) |
| `services/gateway/internal/handler/gateway_test.go` | 1-272 | Explicado | [Tests](08-tests-tools.md#gateway-todas-las-declaraciones-y-verificaciones) |
| `services/gateway/Dockerfile` | 1-16 | Explicado | [Despliegue](07-deployment.md) |
| `services/gateway/go.mod` | 1-10 | Explicado | [Despliegue](07-deployment.md) |
| `services/gateway/go.sum` | — | Excluido | Lock/checksums de dependencias; no verifica integridad esta lectura |
| `services/gateway/.env.example` | — | Excluido | Política de no lectura de archivos `.env` y variantes |
| `services/jobs/cmd/server/main.go` | 1-198 | Explicado | [Despliegue](07-deployment.md) y [Persistencia](06-database.md) |
| `services/jobs/internal/handler/api.go` | 1-103 | Explicado | [Jobs](02-jobs.md) |
| `services/jobs/internal/handler/security.go` | 1-62 | Explicado | [Jobs](02-jobs.md) |
| `services/jobs/internal/handler/api_test.go` | 1-145 | Explicado | [Tests](08-tests-tools.md#jobs-fakes-preparación-y-catorce-casos) |
| `services/jobs/internal/messaging/broker.go` | 1-352 | Explicado | [Mensajería](04-messaging.md) |
| `services/jobs/internal/messaging/broker_test.go` | 1-197 | Parcial | Inventariado y con hash; no analizado en detalle por esta colección |
| `services/jobs/internal/repository/store.go` | 1-133 | Explicado | [Jobs](02-jobs.md) |
| `services/jobs/internal/repository/store_integration_test.go` | 1-115 | Explicado | [Tests](08-tests-tools.md#jobs-integración-secuencia-y-salidas-completas) |
| `services/jobs/internal/repository/zones.go` | 1-141 | Parcial | Inventariado y con hash; no analizado en detalle por esta colección |
| `services/jobs/internal/repository/zones_crossdatabase_integration_test.go` | 1-114 | Parcial | Inventariado y con hash; no analizado en detalle por esta colección |
| `services/jobs/internal/repository/zones_test.go` | 1-235 | Parcial | Inventariado y con hash; no analizado en detalle por esta colección |
| `services/jobs/migrations/001_initial.sql` | 1-30 | Explicado | [Persistencia](06-database.md) |
| `services/jobs/migrations/002_zones.sql` | 1-50 | Parcial | Inventariado y con hash; no analizado en detalle por esta colección |
| `services/jobs/migrations/embed.go` | 1-9 | Explicado | [Persistencia](06-database.md) |
| `services/jobs/Dockerfile` | 1-17 | Explicado | [Despliegue](07-deployment.md) |
| `services/jobs/go.mod` | 1-23 | Explicado | [Despliegue](07-deployment.md) |
| `services/jobs/go.sum` | — | Excluido | Lock/checksums de dependencias |
| `services/jobs/.env.example` | — | Excluido | Política de no lectura de archivos `.env` y variantes |
| `services/risk/cmd/server/main.go` | 1-250 | Explicado | [Despliegue](07-deployment.md) y [Persistencia](06-database.md) |
| `services/risk/internal/classify/academic_rule.go` | 1-296 | Parcial | Inventariado y con hash; no analizado en detalle por esta colección |
| `services/risk/internal/classify/academic_rule_test.go` | 1-404 | Parcial | Inventariado y con hash; no analizado en detalle por esta colección |
| `services/risk/internal/classify/behavior_table_test.go` | 1-539 | Parcial | Inventariado y con hash; no analizado en detalle por esta colección |
| `services/risk/internal/classify/classifier.go` | 1-361 | Parcial | Inventariado y con hash; no analizado en detalle por esta colección |
| `services/risk/internal/classify/classifier_test.go` | 1-264 | Parcial | Inventariado y con hash; no analizado en detalle por esta colección |
| `services/risk/internal/classify/params.go` | 1-213 | Parcial | Inventariado y con hash; no analizado en detalle por esta colección |
| `services/risk/internal/classify/params_test.go` | 1-874 | Parcial | Inventariado y con hash; no analizado en detalle por esta colección |
| `services/risk/internal/handler/api.go` | 1-187 | Explicado | [Risk](03-risk.md) |
| `services/risk/internal/handler/security.go` | 1-61 | Explicado | [Risk](03-risk.md) |
| `services/risk/internal/handler/api_test.go` | 1-279 | Explicado | [Tests](08-tests-tools.md#risk-seis-casos-no-matriz-criptográfica-completa) |
| `services/risk/internal/messaging/broker.go` | 1-338 | Explicado | [Mensajería](04-messaging.md) |
| `services/risk/internal/messaging/broker_test.go` | 1-195 | Parcial | Inventariado y con hash; no analizado en detalle por esta colección |
| `services/risk/internal/repository/store.go` | 1-299 | Explicado | [Risk](03-risk.md) |
| `services/risk/internal/repository/store_integration_test.go` | 1-278 | Explicado | [Tests](08-tests-tools.md#risk-integración-dos-capas-de-duplicación-y-orden) |
| `services/risk/internal/service/demo.go` | 1-43 | Explicado | [Risk](03-risk.md) |
| `services/risk/internal/service/demo_test.go` | 1-44 | Explicado | [Tests](08-tests-tools.md#demo-mutaciones-independientes-y-guardas-ausentes) |
| `services/risk/internal/service/publisher.go` | 1-247 | Parcial | Inventariado y con hash; no analizado en detalle por esta colección |
| `services/risk/internal/service/publisher_integration_test.go` | 1-133 | Parcial | Inventariado y con hash; no analizado en detalle por esta colección |
| `services/risk/internal/service/publisher_test.go` | 1-543 | Parcial | Inventariado y con hash; no analizado en detalle por esta colección |
| `services/risk/internal/source/fixture.go` | 1-130 | Parcial | Inventariado y con hash; no analizado en detalle por esta colección |
| `services/risk/internal/source/provider.go` | 1-315 | Parcial | Inventariado y con hash; no analizado en detalle por esta colección |
| `services/risk/internal/source/provider_test.go` | 1-365 | Parcial | Inventariado y con hash; no analizado en detalle por esta colección |
| `services/risk/internal/zones/seed.go` | 1-150 | Parcial | Inventariado y con hash; no analizado en detalle por esta colección |
| `services/risk/internal/zones/seed_integration_test.go` | 1-131 | Parcial | Inventariado y con hash; no analizado en detalle por esta colección |
| `services/risk/internal/zones/seed_test.go` | 1-114 | Parcial | Inventariado y con hash; no analizado en detalle por esta colección |
| `services/risk/migrations/001_initial.sql` | 1-26 | Explicado | [Persistencia](06-database.md) |
| `services/risk/migrations/002_models.sql` | 1-87 | Parcial | Inventariado y con hash; no analizado en detalle por esta colección |
| `services/risk/migrations/003_policy_ref.sql` | 1-22 | Parcial | Inventariado y con hash; no analizado en detalle por esta colección |
| `services/risk/migrations/004_zones.sql` | 1-46 | Parcial | Inventariado y con hash; no analizado en detalle por esta colección |
| `services/risk/migrations/005_zone_reference.sql` | 1-26 | Parcial | Inventariado y con hash; no analizado en detalle por esta colección |
| `services/risk/migrations/006_models_integrity.sql` | 1-34 | Parcial | Inventariado y con hash; no analizado en detalle por esta colección |
| `services/risk/migrations/embed.go` | 1-53 | Explicado | [Persistencia](06-database.md) |
| `services/risk/migrations/migrations_test.go` | 1-205 | Parcial | Inventariado y con hash; no analizado en detalle por esta colección |
| `services/risk/Dockerfile` | 1-17 | Explicado | [Despliegue](07-deployment.md) |
| `services/risk/go.mod` | 1-23 | Explicado | [Despliegue](07-deployment.md) |
| `services/risk/go.sum` | — | Excluido | Lock/checksums de dependencias |
| `services/risk/.env.example` | — | Excluido | Política de no lectura de archivos `.env` y variantes |
| `contracts/gen/events/v1/events.go` | 1-26 | Explicado | [Contratos](05-contracts.md); escrito a mano, no excluido por su carpeta |
| `contracts/gen/geo/geo.go` | 1-25 | Parcial | Contrato generado inventariado y con hash; no analizado en detalle por esta colección |
| `contracts/gen/geo/valparaiso-comunas.geojson` | 1-1 | Parcial | Activo geográfico inventariado y con hash; no analizado en detalle |
| `contracts/gen/go.mod` | 1-3 | Explicado | [Despliegue](07-deployment.md) |
| `contracts/gen/observations/v1/observations.go` | 1-176 | Parcial | Inventariado y con hash; no analizado en detalle por esta colección |
| `contracts/gen/observations/v1/observations_test.go` | 1-118 | Parcial | Inventariado y con hash; no analizado en detalle por esta colección |
| `contracts/events/v1/README.md` | 1-15 | Explicado | [Contratos](05-contracts.md), intención de transporte |
| `contracts/openapi/firewatch-v1.yaml` | 1-568 | Explicado | [Contratos](05-contracts.md), declaraciones agrupadas |
| `deploy/local/postgres-init.sh` | 1-31 | Explicado | [Persistencia](06-database.md) |
| `deploy/local/prometheus.yml` | 1-12 | Explicado | [Despliegue](07-deployment.md) |
| `deploy/local/rabbitmq-plugins` | 1-1 | Explicado | [Despliegue](07-deployment.md) |
| `deploy/local/firewatch-realm.json` | — | Excluido | Configuración de cuentas/credenciales de identidad; no se establecen sus clientes, roles ni TTL |
| `deploy/k8s/base/config.yaml` | 1-17 | Explicado | [Despliegue](07-deployment.md) |
| `deploy/k8s/base/ingress.yaml` | 1-22 | Explicado | [Despliegue](07-deployment.md) |
| `deploy/k8s/base/kustomization.yaml` | 1-9 | Explicado | [Despliegue](07-deployment.md) |
| `deploy/k8s/base/namespace.yaml` | 1-6 | Explicado | [Despliegue](07-deployment.md) |
| `deploy/k8s/base/network.yaml` | 1-69 | Explicado | [Despliegue](07-deployment.md) |
| `deploy/k8s/base/workloads.yaml` | 1-174 | Explicado | [Despliegue](07-deployment.md) |
| `deploy/k8s/README.md` | 1-29 | Contexto | Prerrequisitos declarados, no infraestructura provisionada |
| `scripts/fetch_comunas.py` | 1-391 | Parcial | Script inventariado y con hash; no analizado en detalle por esta colección |
| `scripts/init_env.py` | 1-18 | Explicado | [Herramientas](08-tests-tools.md#init_envpy-creación-exclusiva-no-rotación); permisos efectivos no observados |
| `scripts/smoke.py` | 1-159 | Explicado | [Herramientas](08-tests-tools.md#smoke-branches-y-excepciones-de-helpers) y [main](08-tests-tools.md#smoke-inventario-completo-de-main-y-callbacks); sin ejecución |
| `scripts/test_fetch_comunas.py` | 1-228 | Parcial | Script de prueba inventariado y con hash; no analizado en detalle por esta colección |
| `.dockerignore` | 1-8 | Explicado | [Despliegue](07-deployment.md) |
| `.env` | — | Excluido | Configuración sensible; no lectura ni reproducción |
| `.env.example` | — | Excluido | Política de no lectura de archivos `.env` y variantes |
| `docker-compose.yml` | 1-195 | Explicado | [Despliegue](07-deployment.md), referencias de variables, sin resolver valores |
| `go.work` | 1-8 | Explicado | [Despliegue](07-deployment.md) |
| `go.work.sum` | — | Excluido | Lock/checksums del workspace; no se lee ni verifica integridad |
| `Makefile` | 1-19 | Explicado | [Despliegue](07-deployment.md), comandos leídos, no ejecutados |
| `README.md` | 1-91 | Contexto | Instrucciones existentes y futuros trabajos |
| `docs/architecture.md` | 1-63 | Contexto | Decisiones existentes contrastadas con fuente |
| `docs/keycloak-realm-setup.md` | 1-58 | Contexto | Configuración de cliente OIDC declarada; no verificada contra un Keycloak vivo |
| `docs/verification.md` | 1-45 | Contexto | Resultados históricos declarados, no repetidos ni acreditados por esta tarea |

**Reconciliación:** `verify_structure.py` imprime los conteos vigentes de archivos inventariados y encontrados, de cada estado y de los archivos con hash; este README no los duplica a mano porque cualquier edición los invalida. La cobertura explicativa corresponde solo al alcance seleccionado y no es cobertura de tests ni de las exclusiones o dependencias externas. Los Markdown de esta colección, `source-snapshot.json` y `verify_structure.py` son salida, fuera del denominador. No se encontraron archivos fuente adicionales, vendor, binarios o coberturas dentro del árbol backend inventariado; no se inspeccionaron cachés o volúmenes externos. Los `go.sum`, los archivos `.env`/variantes y el realm siguen excluidos.

## Re-baseline de la instantánea (2026-10-11)

El verificador ancla cada entrada del registro en un rango de líneas (`1-<total>`) y guarda el hash de bytes de cada archivo. Por eso **cualquier** edición de un archivo fuente cambia su total de líneas o su hash, invalida la fila y hace que el verificador reporte la diferencia. Con el árbol en movimiento, esos errores se acumulan como un ruido de fondo permanente que oculta regresiones reales: un archivo eliminado, una cita fuera de rango o un enlace roto quedan indistinguibles del movimiento normal. Se decidió entonces **re-baselinar una sola vez y de forma deliberada** en este hito: incorporar al inventario los archivos que faltaban, corregir los rangos completos y reescribir `source-snapshot.json` con `--snapshot`.

**Segundo re-baseline (cierre del hito de mapa geoespacial):** con el mismo motivo y la misma causa raíz —anclaje en rangos de líneas y hashes de archivo completo—, se repitió el re-baseline una sola vez al cierre de las slices M1..M5 del mapa geoespacial. Se incorporaron al inventario los archivos nuevos (`contracts/gen/geo/`, `services/jobs/internal/repository/zones*`, `services/jobs/migrations/002_zones.sql`, `services/risk/internal/zones/`, `services/risk/migrations/004_zones.sql` y `005_zone_reference.sql`, `scripts/fetch_comunas.py` y `scripts/test_fetch_comunas.py`), se corrigieron los rangos completos y se reescribió `source-snapshot.json` con `--snapshot`. El verificador ignora por diseño los cachés generados (`__pycache__`, `.pytest_cache`, `*.pyc`), de modo que el inventario describe código fuente y no artefactos: en una máquina limpia esos cachés no reaparecen como faltantes. Los conteos se leen del resumen del propio verificador, no de esta prosa.

**Re-baseline al cierre de Unit A.2:** se incorporaron los archivos nuevos al inventario, se actualizaron los rangos completos y se reescribió `source-snapshot.json` con `--snapshot`; los conteos provienen del resumen del verificador.

**Causa raíz, registrada como candidata y explícitamente NO corregida ahora:** una herramienta que quiere detectar deriva **estructural** debería anclarse en la identidad del archivo y en sus símbolos, no en números de línea. Anclar en líneas convierte cualquier reformateo en un falso positivo. El comportamiento de `verify_structure.py` no se modifica en este re-baseline; queda anotado para una tarea futura.

## Distinciones importantes

| Implementación leída | Intención/documentación | No establecido |
| --- | --- | --- |
| Dos estados SQL: `queued`/`published` | Eventos al menos una vez | Estado terminal de fallo o replay automático |
| Fecha límite de lote = solicitud + 15 minutos | Clasificación por regla propia no oficial | Scheduler de 15 minutos o calidad científica |
| Verificación JWT en gateway y servicios | Roles `viewer`/`admin` | Configuración real del realm, revocación inmediata |
| Readiness de servicios consulta PostgreSQL | Lecturas disponibles ante caída de broker | Disponibilidad medida, funcionamiento del clúster |
| Bases de despliegue con placeholders | Entorno staging configurable | Despliegue Kubernetes realizado |

Las afirmaciones históricas de `docs/verification.md:3-21` se conservan como **contexto ajeno**, no como resultados observados de este análisis. La configuración CI mencionada en `docs/architecture.md:54` está fuera de `backend` y no fue inspeccionada.

## Comprobación documental y límites

Desde `backend/`, con Python 3.9 o posterior, el [verificador](verify_structure.py) utiliza solo biblioteca estándar y no importa código fuente. `-B` evita crear `__pycache__`. Las comprobaciones son estructurales, **no resultados PASS de la aplicación**:

```sh
# Captura autorizada inicial; escribe únicamente el manifiesto en esta carpeta.
python -B docs/code-explanation/verify_structure.py --snapshot
# Repetible y de solo lectura: verifica la instantánea, inventario, enlaces y citas.
python -B docs/code-explanation/verify_structure.py --list-citations
# Mismas comprobaciones, solo resumen JSON (sin enumeración de citas).
python -B docs/code-explanation/verify_structure.py
```

- La captura inicial no convirtió explicación parcial en completa automáticamente y sirvió para detectar la deriva estructural; sus conteos concretos no se fijan en prosa.
- Resultado de cierre observado: `verify_structure.py` imprime el resumen vigente (Markdown, enlaces locales, anchors, rangos de cita resueltos, archivos inventariados y encontrados, estados y hashes) junto con la lista de errores. En el cierre de este re-baseline la lista quedó vacía y la salida fue 0. Los rangos cuentan ocurrencias, incluidas repeticiones y abreviaturas aplicadas a ambos servicios; no son unidades de código distintas. Los conteos exactos se leen del resumen del propio verificador, no de este párrafo.
- El cierre contrasta todos los enlaces Markdown locales (incluidos anchors de títulos), existencia de archivos citados y límites inclusivos de rangos. Enumera citas con rutas completas, rangos junto a enlaces y abreviaturas con contexto explícito por informe. Las tablas con rangos numéricos sin ruta se contrastaron manualmente con sus cabeceras y fuentes completas; el script no certifica pertinencia semántica de una cita ni interpreta Markdown arbitrario, YAML/OpenAPI o Go.
- El inventario se reconcilia mediante nombres de archivos reales, estados de la tabla y rangos completos contra número de líneas; los hashes se comparan sin regenerar el manifiesto. Nombres excluidos se enumeran, **no se abren**. No se contactan endpoints remotos.
- Se reinspeccionaron los archivos no excluidos y los informes; se corrigió el backoff que mide sesión más pausa, la omisión de errores Marshal en fixtures y las limitaciones reales de assertions, cleanup y deadlines del smoke. Ninguna corrección requiere modificar fuente.
- Todas las escrituras de esta reanudación se limitaron a esta carpeta: Markdown modificados y archivos de soporte nuevos. La comparación de hashes y la lista de nombres no detectó cambios de las fuentes no excluidas. Sin Git no se puede probar ausencia de cambios concurrentes externos, cambios en secretos no leídos ni cambios fuera de backend; no se afirma esa garantía.
- Pendientes **fuera del cierre documental**: comportamiento real de dependencias, permisos Windows/ACL, validación con parsers de despliegue/contratos, concurrencia, ejecución de tests/smoke y producción. Necesitan autorización separada. No quedan huecos de explicación del alcance seleccionado; las filas con estado **Parcial** son archivos inventariados y verificados que quedan fuera de ese alcance, no explicaciones incompletas.
