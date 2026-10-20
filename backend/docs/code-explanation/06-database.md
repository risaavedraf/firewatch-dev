# Persistencia: propiedad por servicio y transacciones locales

**Re-derivado el 2026-10-20 contra el código de `267a4bb`.**

`jobs` y `risk` tienen esquemas propios y cada servicio versiona los suyos sobre su propia base. El trabajo, su auditoría y su outbox se comprometen juntos; el lote risk, inbox y outbox también. No hay una transacción distribuida entre ambas bases ni claves foráneas que crucen servicios.

[Índice e instantánea](README.md) · [Contratos](05-contracts.md) · [Siguiente: despliegue](07-deployment.md)

## Alcance y conexiones

Lecturas completas: `jobs` trae **4** migraciones SQL ([001_initial.sql](../../services/jobs/migrations/001_initial.sql):1-30, `002_zones.sql`:1-50, `003_synthetic_scenario.sql`:1-23, `004_job_failure_state.sql`:1-19) y `risk` trae **9** ([001_initial.sql](../../services/risk/migrations/001_initial.sql):1-26, `002_models.sql`:1-87, `003_policy_ref.sql`:1-22, `004_zones.sql`:1-46, `005_zone_reference.sql`:1-26, `006_models_integrity.sql`:1-34, `007_synthetic_evaluation.sql`:1-25, `008_firewatch_rule_v2.sql`:1-19, `010_models_activation_grant.sql`:1-19); ambos `migrations/embed.go` completos ([jobs](../../services/jobs/migrations/embed.go):1-23, [risk](../../services/risk/migrations/embed.go):1-65); [postgres-init.sh](../../deploy/local/postgres-init.sh):1-31; y las ramas migrate de ambos `cmd/server/main.go` ([jobs](../../services/jobs/cmd/server/main.go):40-103, [risk](../../services/risk/cmd/server/main.go):45-113) dentro de archivos de 341 y 442 líneas. Snapshot y exclusiones sensibles: [índice](README.md). Ninguna migración fue ejecutada.

## Jobs: restricciones, índices y permisos

| Unidad / líneas de `services/jobs/migrations/001_initial.sql` | Declaración | Efecto esperado / frontera |
| --- | --- | --- |
| jobs, 1-10 | PK id UUID; actor/key requeridos; zona demo; state queued/published; batch opcional; requested_at default now; UNIQUE actor/key | Idempotencia por sujeto; no CHECK relaciona estado con batch_id |
| outbox, 11-17 | PK evento; routing string; payload jsonb; created_at now; published_at nullable | Pendiente si published_at NULL; payload no se valida como Event por SQL |
| índice, 18 | created_at parcial solo pendientes | Apoya selección relay; no asegura orden único si timestamps empatan |
| inbox, 19 | PK id y processed_at default now | Dedupe durable por ID, no contenido |
| audit, 20-27 | Identity bigint; actor/action text; job UUID FK local; trace UUID; fecha default | Trace inválido falla y puede revertir finalización; action no tiene enum SQL |
| grants, 28-30 | SELECT/INSERT/UPDATE jobs/outbox; SELECT/INSERT inbox/audit; USAGE/SELECT secuencia | No concede UPDATE/DELETE audit ni DDL; permisos efectivos dependen de roles externos |

`IF NOT EXISTS` evita recrear objetos presentes, pero no altera una tabla anterior con estructura distinta ni verifica su compatibilidad. Audit usa FK a jobs, no a usuarios OIDC. No hay políticas row-level ni limpieza/retención automatizada en este SQL. Las tres versiones siguientes son aditivas, guardadas y no editan `001_initial.sql`:

| Unidad / líneas | Declaración | Efecto esperado / frontera |
| --- | --- | --- |
| `002_zones.sql`, 23-28 | `zones(code PK, name, province, kind CHECK comuna/demo)` sin geometry | Catálogo local de jobs; la geometría queda solo en el catálogo de risk |
| `002_zones.sql`, 30-40 | Fila `valparaiso-demo` con `ON CONFLICT DO NOTHING` y `GRANT SELECT` a jobs_app | Única zona escrita en SQL; las 38 comunas llegan por el seed del migrate |
| `002_zones.sql`, 48-50 | DROP del CHECK `zone_id` y ADD FK `jobs.zone_id → zones(code)` | El conjunto de zonas válidas es dato, no literal compilado |
| `003_synthetic_scenario.sql`, 13-14 | `ADD COLUMN IF NOT EXISTS scenario_id text, evaluated_at timestamptz` | Metadatos opcionales; filas históricas quedan NULL y no se reinterpretan |
| `003_synthetic_scenario.sql`, 19-23 | CHECK `jobs_synthetic_metadata_pairing` | Escenario e instante van juntos o no van; ningún valor intermedio es representable |
| `004_job_failure_state.sql`, 14-16 | `attempts integer NOT NULL DEFAULT 0`, `failure_reason`, `failed_at` | El presupuesto de intentos deja de vivir solo en memoria del proceso |
| `004_job_failure_state.sql`, 18-19 | `state` reescrito a queued/published/failed | Estado terminal persistido; 004 no toca filas históricas |

## Risk: lote completo y localización

| Unidad / líneas de `services/risk/migrations/001_initial.sql` | Declaración | Efecto esperado |
| --- | --- | --- |
| 1 | CREATE EXTENSION postgis | Requiere permisos y extensión disponible |
| batches, 2-13 | ID PK; job_id UNIQUE sin FK externa; zona demo; fechas; modelo fijo; CHECK demo; geometry Point SRID4326; valid_until > requested_at | Fixture explicitamente sintético; una fila por trabajo; no CHECK de published_at respecto a solicitud |
| índices, 14-15 | zone/requested desc/id desc; GiST location | Apoya latest; GiST disponible aunque List no ejecuta filtro espacial |
| outbox, 16-23 | Mismo diseño e índice parcial que jobs | Publicación posterior a commit de lote |
| inbox, 24 | PK evento y fecha | Redelivery deduplicada |
| grants, 25-26 | SELECT/INSERT batches/inbox; SELECT/INSERT/UPDATE outbox | Aplicación no recibe UPDATE batches ni DELETE |

Las ocho versiones siguientes reemplazan los literales congelados por datos:

| Unidad / líneas | Declaración | Efecto esperado / frontera |
| --- | --- | --- |
| `002_models.sql`, 11-12 | DROP de `batches_model_version_check` y `batches_demo_check` | Modelo y flag demo dejan de ser literales del esquema |
| `002_models.sql`, 16-28 | `risk_level/factors/rationale`; `UPDATE` de las filas previas a `desconocido`; `SET NOT NULL`; CHECK de cuatro niveles | Único backfill de la colección: declara el nivel previo como desconocido, nunca «bajo» |
| `002_models.sql`, 41-49 | `models(version UNIQUE, kind, params_hash, approved_at, metrics, active)` | Una versión identifica un comportamiento inmutable, no una etiqueta |
| `002_models.sql`, 68-79 | Registro de `academic-rule-v1`, `demo-fixture-v1` y de las versiones históricas tomadas de `batches` | `ON CONFLICT DO NOTHING`; `params_hash` NULL cuando no es reconstruible |
| `002_models.sql`, 82-87 | FK `batches.model_version → models(version)` y `GRANT SELECT ON models` | Una versión sin fila en el registro no puede quedar referenciada |
| `003_policy_ref.sql`, 16-22 | `policy_ref jsonb` y CHECK de `policy_id`/`params_hash` | Sin referencia para filas previas: no se backfillea procedencia |
| `004_zones.sql`, 27-36 | `zones` con `geometry(Geometry,4326)` e índice GiST | Acepta Polygon, MultiPolygon y geometría NULL por diseño |
| `004_zones.sql`, 41-46 | Fila `valparaiso-demo` con geometría NULL y `GRANT SELECT` | Las 38 comunas llegan por el seed, no por SQL |
| `005_zone_reference.sql`, 24-26 | DROP del CHECK de zona y ADD FK `batches.zone_id → zones(code)` | Una zona ausente del catálogo hace fallar la migración |
| `006_models_integrity.sql`, 27-34 | CHECK `models_placeholder_not_active_check` e índice único parcial `models_single_active` | Un placeholder no puede estar activo y solo un modelo lo está |
| `007_synthetic_evaluation.sql`, 21-25 | `scenario_id`, `evaluated_at` y CHECK de emparejamiento | Metadatos sintéticos opcionales; filas históricas NULL |
| `008_firewatch_rule_v2.sql`, 16-19 | Registro de `firewatch-rule-v2` con `params_hash` congelado y `active=false` | No reemplaza al clasificador por defecto; la versión se elige por `MODEL_ID` |
| `010_models_activation_grant.sql`, 19 | `GRANT UPDATE ON models TO risk_app` | Habilita la activación; sin este permiso la API responde 503 |

`RiskLevel` sí es columna persistida (`services/risk/migrations/002_models.sql`:16-28). `DataStatus` y `Notice` no se almacenan: se derivan al leer en `services/risk/internal/repository/store.go:517-528`, junto con el nivel persistido. `ValidUntil` y `Policy` son valores persistidos. El punto fijo lo inserta el repositorio, no una tabla de observaciones (`store.go:226`).

## Embed y rama migrate

Los dos `migrations/embed.go` embeben todo su SQL con `//go:embed`: el de jobs ocupa 23 líneas y exporta `Initial`, `Zones`, `SyntheticScenario` y `JobFailureState` ([jobs](../../services/jobs/migrations/embed.go):1-23); el de risk ocupa 65 y guarda nueve archivos en variables privadas, más el tipo `Migration{Version,Name,SQL}` y `All()` ([risk](../../services/risk/migrations/embed.go):1-65), que devuelve la lista ascendente 1-8 y 10 (53-65). Añadir una versión es añadir un archivo y una entrada en esa lista; no existe lectura SQL dinámica del filesystem.

La rama `migrate` ya **no es idéntica** en los dos servicios: comparte el esqueleto, pero jobs lleva la lista de versiones literal dentro de `main.go` y risk la importa de `migrations.All()` y además lee de una vez el conjunto ya aplicado.

| Rama / líneas | Operación | Error / limpieza |
| --- | --- | --- |
| jobs `main.go:40-51` · risk `main.go:45-56` | Parsea DATABASE_URL, configura pool 10 max/1 min y conexión 5 s | Error parse/setup se transforma a mensaje genérico; defer pool.Close |
| jobs `main.go:52-59` · risk `main.go:57-64` | Solo `len(os.Args)==2 && os.Args[1]=="migrate"`; contexto 30 s; Begin y rollback diferido | Error conexión genérico; el rollback diferido cubre toda la rama |
| jobs `main.go:60-65` · risk `main.go:65-70` | `pg_advisory_xact_lock(1)` y tabla schema_migrations(version PK) | Error SQL devuelto; serializa migradores de esa base |
| jobs `main.go:66-88` · risk `main.go:71-99` | jobs recorre cuatro entradas literales (1 Initial, 2 Zones, 3 SyntheticScenario, 4 JobFailureState) con EXISTS por versión; risk lee el set aplicado (71-88) y recorre `migrations.All()` (89-99); si la versión existe, continue | Error envuelto como `migration N (nombre)` en risk y `migration N` en jobs; el rollback descarta lo aplicado en la transacción |
| jobs `main.go:95-97` · risk `main.go:105-107` | Commit del esquema, antes del seed | Error commit devuelto; no arranca HTTP ni broker |
| jobs `main.go:98-103` · risk `main.go:108-113` | Seed de zonas desde el asset embebido, en su propia transacción, en cada migrate | Error `seed zones: %w`; log `zones_seeded` con el conteo |

Con argumentos adicionales, incluso `migrate extra`, la condición no se cumple y se intenta el servidor normal. La rama migrate ocurre antes de exigir AMQP/OIDC, pero usa el mismo setup del pool. No hay down migrations ni comparación de checksum del SQL con la versión guardada: `schema_migrations` es la única fuente de lo aplicado. risk no usa las versiones 9 y 11 porque prevention las reserva en esa misma base (`services/prevention/migrations/embed.go`:38-39), y un test fija el orden sin huecos obligatorios (`services/risk/migrations/migrations_test.go`:27-33).

## Inicialización local: roles antes de grants

`deploy/local/postgres-init.sh:1-5` usa `set -eu`, psql con ON_ERROR_STOP y parámetros de contraseñas **desde variables**, sin valores literales en el archivo leído. No se leyeron valores de esas variables.

| Bloque / líneas de `deploy/local/postgres-init.sh` | Responsabilidad / error |
| --- | --- |
| 6-13 | Crea roles jobs_app/risk_app/identity_app y bases jobs/risk/jobs_test/risk_test/identity; owner identity_app para identity |
| 14-23 | Revoca CONNECT de PUBLIC en las cinco bases; concede acceso específico a roles de aplicación/test |
| 24-31 | En jobs/risk, revoca CREATE de PUBLIC en schema public y concede USAGE al rol correspondiente |

Cada psql falla ante error y `set -e` corta el script; `set -u` corta ante variables ausentes. No contiene `IF NOT EXISTS` para roles/bases ni BEGIN global: un fallo intermedio puede dejar inicialización parcial; no se promete rerun idempotente. Las bases test no reciben aquí el mismo bloque de revocación CREATE que jobs/risk. El mount de init depende del comportamiento de imagen PostgreSQL y de que el volumen requiera inicialización; no se ejecutó esa dependencia.

## Traza razonada y error

Supuestos: base jobs nueva, rol jobs_app existente, migrador con permisos, sin ninguna versión en `schema_migrations`.

| Paso | Cita | Estado antes -> después |
| --- | --- | --- |
| Entrada | `main.go:52-59` | Argumento migrate exacto -> tx con timeout 30 s |
| Serialización | `main.go:60-65` | Adquiere lock, crea el ledger y determina el set aplicado |
| DDL | `main.go:66-88`; `services/jobs/migrations/001_initial.sql:1-30` | Aplica 1, 2, 3 y 4 en orden, registrando cada versión |
| Commit | `main.go:95-97` | Esquema y versiones durables; el proceso aún no retorna |
| Seed | `main.go:98-103` | Zonas del asset upsertadas; retorna sin HTTP ni broker |

**Alternativa:** rol jobs_app inexistente produce error en GRANT (`services/jobs/migrations/001_initial.sql`:28-30); la rama retorna error y el rollback impide registrar versión alguna. En risk, falta de permisos para PostGIS puede fallar al inicio. Un migrador concurrente espera el lock hasta obtenerlo o agotar el contexto; no se simuló esa espera.

## Cobertura, intención y límites

Las 13 migraciones SQL de los dos servicios están explicadas por sus declaraciones y bloques; las ramas migrate de ambos main se cubren aquí y el resto en [despliegue](07-deployment.md). Los tests de estructura fijan las garantías citadas y ninguno ejecuta SQL contra una base real: `services/jobs/migrations/synthetic_scenarios_test.go`:1-55, `services/risk/migrations/migrations_test.go`:1-276 y `services/risk/migrations/models_activation_grant_test.go`:1-81. La intención documentada exige owner de migración distinto de credenciales runtime (`deploy/k8s/README.md`:19); Compose usa usuario postgres para los tres migradores y roles app para los servicios (`docker-compose.yml`:93-157). No se comprobaron permisos efectivos, defaults PostgreSQL, aislamiento, instalación PostGIS ni integridad de datos existentes.

Próxima verificación propuesta: migración repetida, roles ausentes, lock concurrente y permisos audit/DDL con base desechable autorizada. No ejecutar contra datos operativos ni inferir compatibilidad Oracle: SQL usa PostgreSQL/PostGIS explícitamente.
