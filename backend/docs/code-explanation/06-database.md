# Persistencia: propiedad por servicio y transacciones locales

`jobs` y `risk` tienen esquemas propios. El trabajo, su auditoría y su outbox se comprometen juntos; el lote risk, inbox y outbox también. No hay una transacción distribuida entre ambas bases ni claves foráneas que crucen servicios.

[Índice e instantánea](README.md) · [Contratos](05-contracts.md) · [Siguiente: despliegue](07-deployment.md)

## Alcance y conexiones

Lecturas completas: [SQL jobs](../../services/jobs/migrations/001_initial.sql):1-30; [SQL risk](../../services/risk/migrations/001_initial.sql):1-26; ambos `migrations/embed.go`:1-6 ([jobs](../../services/jobs/migrations/embed.go), [risk](../../services/risk/migrations/embed.go)); [postgres-init.sh](../../deploy/local/postgres-init.sh):1-31. Rama migrate de ambos `cmd/server/main.go:48-75` leída completa dentro de sus archivos de 141 líneas. Snapshot y exclusiones sensibles: [índice](README.md). Ninguna migración fue ejecutada.

## Jobs: restricciones, índices y permisos

| Unidad / líneas de `services/jobs/migrations/001_initial.sql` | Declaración | Efecto esperado / frontera |
| --- | --- | --- |
| jobs, 1-10 | PK id UUID; actor/key requeridos; zona demo; state queued/published; batch opcional; requested_at default now; UNIQUE actor/key | Idempotencia por sujeto; no CHECK relaciona estado con batch_id |
| outbox, 11-17 | PK evento; routing string; payload jsonb; created_at now; published_at nullable | Pendiente si published_at NULL; payload no se valida como Event por SQL |
| índice, 18 | created_at parcial solo pendientes | Apoya selección relay; no asegura orden único si timestamps empatan |
| inbox, 19 | PK id y processed_at default now | Dedupe durable por ID, no contenido |
| audit, 20-27 | Identity bigint; actor/action text; job UUID FK local; trace UUID; fecha default | Trace inválido falla y puede revertir finalización; action no tiene enum SQL |
| grants, 28-30 | SELECT/INSERT/UPDATE jobs/outbox; SELECT/INSERT inbox/audit; USAGE/SELECT secuencia | No concede UPDATE/DELETE audit ni DDL; permisos efectivos dependen de roles externos |

`IF NOT EXISTS` evita recrear objetos presentes, pero no altera una tabla anterior con estructura distinta ni verifica su compatibilidad. Audit usa FK a jobs, no a usuarios OIDC. No hay políticas row-level ni limpieza/retención automatizada en este SQL.

## Risk: lote completo y localización

| Unidad / líneas de `services/risk/migrations/001_initial.sql` | Declaración | Efecto esperado |
| --- | --- | --- |
| 1 | CREATE EXTENSION postgis | Requiere permisos y extensión disponible |
| batches, 2-13 | ID PK; job_id UNIQUE sin FK externa; zona demo; fechas; modelo fijo; CHECK demo; geometry Point SRID4326; valid_until > requested_at | Fixture explicitamente sintético; una fila por trabajo; no CHECK de published_at respecto a solicitud |
| índices, 14-15 | zone/requested desc/id desc; GiST location | Apoya latest; GiST disponible aunque List no ejecuta filtro espacial |
| outbox, 16-23 | Mismo diseño e índice parcial que jobs | Publicación posterior a commit de lote |
| inbox, 24 | PK evento y fecha | Redelivery deduplicada |
| grants, 25-26 | SELECT/INSERT batches/inbox; SELECT/INSERT/UPDATE outbox | Aplicación no recibe UPDATE batches ni DELETE |

No se almacenan `DataStatus`, `RiskLevel` o `Notice`: el repositorio los calcula al leer (`services/risk/internal/repository/store.go:79-86`). `ValidUntil` es valor persistido. El punto fijo lo inserta el repositorio, no una tabla de observaciones.

## Embed y rama migrate

Los dos `embed.go:1-6` importan `_ "embed"`, usan `//go:embed 001_initial.sql` y declaran `Initial string`: el SQL se incorpora en el binario al compilar. No existe función de migración en ese paquete ni lectura SQL dinámica del filesystem.

La rama `migrate` es igual en ambos servicios. Las referencias siguientes aplican a `services/jobs/cmd/server/main.go` y `services/risk/cmd/server/main.go`.

| Rama / líneas | Operación | Error / limpieza |
| --- | --- | --- |
| 36-47 | Parsea DATABASE_URL, configura pool 10 max/1 min y conexión 5 s | Error parse/setup se transforma a mensaje genérico; defer pool.Close |
| 48-55 | Solo `len(os.Args)==2 && os.Args[1]=="migrate"`; contexto 30 s; Begin | Error conexión genérico; rollback diferido |
| 56-61 | `pg_advisory_xact_lock(1)` y tabla schema_migrations(version PK) | Error SQL devuelto; serializa migradores de esa base |
| 62-65 | SELECT EXISTS version=1 | Error Scan devuelto |
| 66-73 | Si no existe: ejecuta Initial e inserta versión 1 | Cualquier error revierte esquema/versionado transaccional esperado |
| 74-75 | Commit tanto si versión ya existía como si nueva | Devuelve error commit; no arranca HTTP ni broker |

Con argumentos adicionales, incluso `migrate extra`, la condición no se cumple y se intenta el servidor normal. La rama migrate ocurre antes de exigir AMQP/OIDC, pero usa el mismo setup del pool. No hay colección de versiones futuras, down migrations ni comparación de checksum de SQL con versión guardada.

## Inicialización local: roles antes de grants

`deploy/local/postgres-init.sh:1-5` usa `set -eu`, psql con ON_ERROR_STOP y parámetros de contraseñas **desde variables**, sin valores literales en el archivo leído. No se leyeron valores de esas variables.

| Bloque / líneas de `deploy/local/postgres-init.sh` | Responsabilidad / error |
| --- | --- |
| 6-13 | Crea roles jobs_app/risk_app/identity_app y bases jobs/risk/jobs_test/risk_test/identity; owner identity_app para identity |
| 14-23 | Revoca CONNECT de PUBLIC en las cinco bases; concede acceso específico a roles de aplicación/test |
| 24-31 | En jobs/risk, revoca CREATE de PUBLIC en schema public y concede USAGE al rol correspondiente |

Cada psql falla ante error y `set -e` corta el script; `set -u` corta ante variables ausentes. No contiene `IF NOT EXISTS` para roles/bases ni BEGIN global: un fallo intermedio puede dejar inicialización parcial; no se promete rerun idempotente. Las bases test no reciben aquí el mismo bloque de revocación CREATE que jobs/risk. El mount de init depende del comportamiento de imagen PostgreSQL y de que el volumen requiera inicialización; no se ejecutó esa dependencia.

## Traza razonada y error

Supuestos: base jobs nueva, rol jobs_app existente, migrador con permisos, sin versión 1.

| Paso | Cita | Estado antes -> después |
| --- | --- | --- |
| Entrada | `services/jobs/cmd/server/main.go:48-55` | Argumento migrate exacto -> tx con timeout 30 s |
| Serialización | `services/jobs/cmd/server/main.go:56-65` | Adquiere lock y determina exists=false |
| DDL | `services/jobs/cmd/server/main.go:66-73`; `services/jobs/migrations/001_initial.sql:1-30` | Crea tablas/índice/permisos, inserta versión |
| Commit | `services/jobs/cmd/server/main.go:74` | Esquema y versión durables; proceso retorna sin HTTP |

**Alternativa:** rol jobs_app inexistente produce error en GRANT; la rama retorna error y rollback impide registrar versión 1. En risk, falta de permisos para PostGIS puede fallar al inicio. Un migrador concurrente espera lock hasta obtenerlo o agotar contexto; no se simuló esa espera.

## Cobertura, intención y límites

Cinco archivos completos de esta parte están explicados por todas sus declaraciones/bloques; ramas migrate de ambos main se cubren aquí y el resto en [despliegue](07-deployment.md). La intención documentada exige owner de migración distinto de credenciales runtime (`deploy/k8s/README.md:14-19`); Compose usa usuario postgres para migradores y roles app para servicios (`docker-compose.yml:82-122`). No se comprobaron permisos efectivos, defaults PostgreSQL, aislamiento, instalación PostGIS ni integridad de datos existentes.

Próxima verificación propuesta: migración repetida, roles ausentes, lock concurrente y permisos audit/DDL con base desechable autorizada. No ejecutar contra datos operativos ni inferir compatibilidad Oracle: SQL usa PostgreSQL/PostGIS explícitamente.
