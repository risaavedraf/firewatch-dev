# FireWatch database schema inventory

This document is the catalog of the migrated FireWatch databases, read from the
live server through `information_schema` and `pg_catalog` — not transcribed from
`backend/services/{risk,jobs,prevention}/migrations`. It was queried on 2026-11-09
against an isolated compose project (`fw-schema-inv`) migrated from zero, whose
`postgres` container ran `postgis/postgis:17-3.5`: PostgreSQL 17.5 (Debian
17.5-1.pgdg110+1) with PostGIS 3.5.2. Service objects live in two databases.
`jobs` holds schema `public` (6 relations, 30 columns) and is independent. `risk`
holds schema `public` (7 service relations, 43 columns, plus 3 PostGIS extension
relations with 19 columns) and schema `prevention` (6 relations, 46 columns).
`risk` and `jobs` are separate databases; `prevention` is a separate schema of
the `risk` database, not a separate database. Every relation below is owned by
`postgres`. Service and extension objects together are 22 relations and 138
columns.

Types are quoted as the catalog renders them: `data_type` verbatim, with
`character_maximum_length` appended for `character varying` and `udt_schema` /
`udt_name` in parentheses for `USER-DEFINED` and `ARRAY`. `identity (ALWAYS)` in
the Default column means `is_identity = YES` with `identity_generation = ALWAYS`;
`information_schema.columns.column_default` is null for those columns. A dash
means the catalog records no default.

The live ledger is `public.schema_migrations`. `jobs` holds versions 1–4; `risk`
holds 1–14, of which 9, 11 and 14 are the prevention service's rows, because the
two services share that ledger. Every migration in this document's basis is
applied: HEAD `54d7ea1` plus `014_action_assignee.sql`, which is described under
"Migration basis" and is why `prevention.directory_users` and
`prevention.actions.assignee_id` appear below. The inventory lists
`public.readings`, created by `012_readings.sql`, and the `ml-fixture-v1` row that
`013_ml_fixture_v1.sql` inserts into `public.models`.
That row is named here but not enumerated: row data is out of scope for this
inventory. The catalog is authoritative here; where the long-lived local stack
differed is recorded in "Migration basis".

## Service `jobs` (database `jobs`, schema `public`)

Six relations, 30 columns. `jobs` owns its database and its own
`schema_migrations`; no relation in this section is shared with `risk` or
`prevention`.

### `public.audit` — 6 columns

| Column | Type | Nullable | Default |
| --- | --- | --- | --- |
| `id` | `bigint` | no | identity (ALWAYS) |
| `actor_id` | `text` | no | — |
| `action` | `text` | no | — |
| `job_id` | `uuid` | no | — |
| `trace_id` | `uuid` | no | — |
| `occurred_at` | `timestamp with time zone` | no | now() |

**Keys.**

- PK `audit_pkey`: id
- FK `audit_job_id_fkey`: `job_id -> public.jobs(id)`, ON DELETE NO ACTION, ON UPDATE NO ACTION

### `public.inbox` — 2 columns

| Column | Type | Nullable | Default |
| --- | --- | --- | --- |
| `id` | `uuid` | no | — |
| `processed_at` | `timestamp with time zone` | no | now() |

**Keys.**

- PK `inbox_pkey`: id

### `public.jobs` — 12 columns

| Column | Type | Nullable | Default |
| --- | --- | --- | --- |
| `id` | `uuid` | no | — |
| `actor_id` | `text` | no | — |
| `idempotency_key` | `uuid` | no | — |
| `zone_id` | `text` | no | — |
| `state` | `text` | no | — |
| `batch_id` | `uuid` | yes | — |
| `requested_at` | `timestamp with time zone` | no | now() |
| `scenario_id` | `text` | yes | — |
| `evaluated_at` | `timestamp with time zone` | yes | — |
| `attempts` | `integer` | no | 0 |
| `failure_reason` | `text` | yes | — |
| `failed_at` | `timestamp with time zone` | yes | — |

**Keys.**

- PK `jobs_pkey`: id
- UNIQUE `jobs_actor_id_idempotency_key_key`: actor_id, idempotency_key
- FK `jobs_zone_id_fkey`: `zone_id -> public.zones(code)`, ON DELETE NO ACTION, ON UPDATE NO ACTION

### `public.outbox` — 5 columns

| Column | Type | Nullable | Default |
| --- | --- | --- | --- |
| `id` | `uuid` | no | — |
| `routing_key` | `text` | no | — |
| `payload` | `jsonb` | no | — |
| `created_at` | `timestamp with time zone` | no | now() |
| `published_at` | `timestamp with time zone` | yes | — |

**Keys.**

- PK `outbox_pkey`: id

### `public.schema_migrations` — 1 column

| Column | Type | Nullable | Default |
| --- | --- | --- | --- |
| `version` | `integer` | no | — |

**Keys.**

- PK `schema_migrations_pkey`: version

### `public.zones` — 4 columns

| Column | Type | Nullable | Default |
| --- | --- | --- | --- |
| `code` | `text` | no | — |
| `name` | `text` | no | — |
| `province` | `text` | yes | — |
| `kind` | `text` | no | — |

**Keys.**

- PK `zones_pkey`: code

## Service `risk` (database `risk`, schema `public`)

Seven service relations, 43 columns. Two columns are `USER-DEFINED
public.geometry`: `batches.location` and `zones.geometry`; the catalog's
`geometry_columns` view renders them as `POINT`/4326 and `GEOMETRY`/4326. The
`models_single_active` unique partial index is a uniqueness key and is listed with
`models`; other indexes are out of scope.

### `public.batches` — 15 columns

| Column | Type | Nullable | Default |
| --- | --- | --- | --- |
| `id` | `uuid` | no | — |
| `job_id` | `uuid` | no | — |
| `zone_id` | `text` | no | — |
| `requested_at` | `timestamp with time zone` | no | — |
| `published_at` | `timestamp with time zone` | no | now() |
| `valid_until` | `timestamp with time zone` | no | — |
| `model_version` | `text` | no | — |
| `demo` | `boolean` | no | — |
| `location` | `USER-DEFINED` (`public.geometry`) | no | — |
| `risk_level` | `text` | no | — |
| `factors` | `jsonb` | yes | — |
| `rationale` | `text` | yes | — |
| `policy_ref` | `jsonb` | yes | — |
| `scenario_id` | `text` | yes | — |
| `evaluated_at` | `timestamp with time zone` | yes | — |

**Keys.**

- PK `batches_pkey`: id
- UNIQUE `batches_job_id_key`: job_id
- FK `batches_model_version_fkey`: `model_version -> public.models(version)`, ON DELETE NO ACTION, ON UPDATE NO ACTION
- FK `batches_zone_id_fkey`: `zone_id -> public.zones(code)`, ON DELETE NO ACTION, ON UPDATE NO ACTION

### `public.inbox` — 2 columns

| Column | Type | Nullable | Default |
| --- | --- | --- | --- |
| `id` | `uuid` | no | — |
| `processed_at` | `timestamp with time zone` | no | now() |

**Keys.**

- PK `inbox_pkey`: id

### `public.models` — 7 columns

| Column | Type | Nullable | Default |
| --- | --- | --- | --- |
| `id` | `uuid` | no | gen_random_uuid() |
| `version` | `text` | no | — |
| `kind` | `text` | no | — |
| `params_hash` | `text` | yes | — |
| `approved_at` | `timestamp with time zone` | yes | — |
| `metrics` | `jsonb` | no | '{}'::jsonb |
| `active` | `boolean` | no | false |

**Keys.**

- PK `models_pkey`: id
- UNIQUE `models_version_key`: version
- UNIQUE INDEX `models_single_active`: `(active) WHERE active`

### `public.outbox` — 5 columns

| Column | Type | Nullable | Default |
| --- | --- | --- | --- |
| `id` | `uuid` | no | — |
| `routing_key` | `text` | no | — |
| `payload` | `jsonb` | no | — |
| `created_at` | `timestamp with time zone` | no | now() |
| `published_at` | `timestamp with time zone` | yes | — |

**Keys.**

- PK `outbox_pkey`: id

### `public.readings` — 8 columns

| Column | Type | Nullable | Default |
| --- | --- | --- | --- |
| `zone_id` | `text` | no | — |
| `variable` | `text` | no | — |
| `observed_at` | `timestamp with time zone` | no | — |
| `value` | `double precision` | no | — |
| `unit` | `text` | no | — |
| `source_id` | `text` | no | — |
| `quality` | `text` | no | — |
| `ingested_at` | `timestamp with time zone` | no | now() |

**Keys.**

- UNIQUE `readings_identity`: zone_id, variable, observed_at

No primary key: the identity the migration asserts is the unique constraint
above. `readings_variable_window` is a non-unique index on `(zone_id, variable,
observed_at DESC)` and is out of scope, like every other non-unique index here.

### `public.schema_migrations` — 1 column

| Column | Type | Nullable | Default |
| --- | --- | --- | --- |
| `version` | `integer` | no | — |

**Keys.**

- PK `schema_migrations_pkey`: version

### `public.zones` — 5 columns

| Column | Type | Nullable | Default |
| --- | --- | --- | --- |
| `code` | `text` | no | — |
| `name` | `text` | no | — |
| `province` | `text` | yes | — |
| `kind` | `text` | no | — |
| `geometry` | `USER-DEFINED` (`public.geometry`) | yes | — |

**Keys.**

- PK `zones_pkey`: code

## Service `prevention` (database `risk`, schema `prevention`)

Six relations, 46 columns. The prevention service owns this schema only and
records its versions into `risk.public.schema_migrations`; it creates no table in
`public`.

### `prevention.actions` — 12 columns

| Column | Type | Nullable | Default |
| --- | --- | --- | --- |
| `id` | `uuid` | no | — |
| `community_code` | `text` | no | — |
| `title` | `text` | no | — |
| `description` | `text` | no | ''::text |
| `priority` | `text` | no | ''::text |
| `status` | `text` | no | — |
| `assignee` | `text` | yes | — |
| `due_date` | `date` | yes | — |
| `created_by` | `text` | no | — |
| `created_at` | `timestamp with time zone` | no | now() |
| `updated_at` | `timestamp with time zone` | no | now() |
| `assignee_id` | `uuid` | yes | — |

**Keys.**

- PK `actions_pkey`: id
- FK `actions_assignee_id_fkey`: `assignee_id -> prevention.directory_users(id)`, ON DELETE RESTRICT, ON UPDATE NO ACTION

`assignee_id` is the last column because the migration adds it with
`ALTER TABLE ... ADD COLUMN`, which appends. `assignee` is the legacy free-text
assignment kept for rows created before the reference existed; it remains
nullable and unconstrained.

### `prevention.action_evidence` — 6 columns

| Column | Type | Nullable | Default |
| --- | --- | --- | --- |
| `id` | `bigint` | no | identity (ALWAYS) |
| `action_id` | `uuid` | no | — |
| `criterion` | `text` | no | — |
| `evidence_references` | `ARRAY` (`pg_catalog._text`) | no | — |
| `created_by` | `text` | no | — |
| `created_at` | `timestamp with time zone` | no | now() |

**Keys.**

- PK `action_evidence_pkey`: id
- FK `action_evidence_action_id_fkey`: `action_id -> prevention.actions(id)`, ON DELETE NO ACTION, ON UPDATE NO ACTION

### `prevention.action_evidence_review` — 7 columns

| Column | Type | Nullable | Default |
| --- | --- | --- | --- |
| `id` | `bigint` | no | identity (ALWAYS) |
| `action_id` | `uuid` | no | — |
| `evidence_id` | `bigint` | no | — |
| `decision` | `text` | no | — |
| `reviewer_sub` | `text` | no | — |
| `note` | `text` | no | ''::text |
| `reviewed_at` | `timestamp with time zone` | no | now() |

**Keys.**

- PK `action_evidence_review_pkey`: id
- FK `action_evidence_review_action_id_fkey`: `action_id -> prevention.actions(id)`, ON DELETE NO ACTION, ON UPDATE NO ACTION
- FK `action_evidence_review_evidence_id_fkey`: `evidence_id -> prevention.action_evidence(id)`, ON DELETE NO ACTION, ON UPDATE NO ACTION

### `prevention.action_history` — 7 columns

| Column | Type | Nullable | Default |
| --- | --- | --- | --- |
| `id` | `bigint` | no | identity (ALWAYS) |
| `action_id` | `uuid` | no | — |
| `from_status` | `text` | no | — |
| `to_status` | `text` | no | — |
| `actor_sub` | `text` | no | — |
| `note` | `text` | no | ''::text |
| `created_at` | `timestamp with time zone` | no | now() |

**Keys.**

- PK `action_history_pkey`: id
- FK `action_history_action_id_fkey`: `action_id -> prevention.actions(id)`, ON DELETE NO ACTION, ON UPDATE NO ACTION

### `prevention.action_reassessment` — 11 columns

| Column | Type | Nullable | Default |
| --- | --- | --- | --- |
| `id` | `bigint` | no | identity (ALWAYS) |
| `action_id` | `uuid` | no | — |
| `job_id` | `uuid` | no | — |
| `batch_id` | `uuid` | no | — |
| `version` | `text` | no | — |
| `risk_level` | `text` | no | — |
| `zone_id` | `text` | no | — |
| `synthetic` | `boolean` | no | — |
| `batch_published_at` | `timestamp with time zone` | no | — |
| `linked_by` | `text` | no | — |
| `linked_at` | `timestamp with time zone` | no | now() |

**Keys.**

- PK `action_reassessment_pkey`: id
- UNIQUE `action_reassessment_job_id_key`: job_id
- FK `action_reassessment_action_id_fkey`: `action_id -> prevention.actions(id)`, ON DELETE NO ACTION, ON UPDATE NO ACTION

### `prevention.directory_users` — 3 columns

| Column | Type | Nullable | Default |
| --- | --- | --- | --- |
| `id` | `uuid` | no | — |
| `display_name` | `text` | no | — |
| `synced_at` | `timestamp with time zone` | no | now() |

**Keys.**

- PK `directory_users_pkey`: id

The primary key is the uniqueness key the service's upsert by `id` uses; no
other index is declared on the table.

## Extension-owned relations (database `risk`, schema `public`)

PostGIS 3.5.2 creates three relations in `public` that no service migration
creates: one table and two views. They are listed because they are part of the
queried schema and are included in the verification below, but they are not
service objects and this document does not interpret them.

### `public.geography_columns` — 7 columns

| Column | Type | Nullable | Default |
| --- | --- | --- | --- |
| `f_table_catalog` | `name` | yes | — |
| `f_table_schema` | `name` | yes | — |
| `f_table_name` | `name` | yes | — |
| `f_geography_column` | `name` | yes | — |
| `coord_dimension` | `integer` | yes | — |
| `srid` | `integer` | yes | — |
| `type` | `text` | yes | — |

**Keys.**

- none (view)

### `public.geometry_columns` — 7 columns

| Column | Type | Nullable | Default |
| --- | --- | --- | --- |
| `f_table_catalog` | `character varying(256)` | yes | — |
| `f_table_schema` | `name` | yes | — |
| `f_table_name` | `name` | yes | — |
| `f_geometry_column` | `name` | yes | — |
| `coord_dimension` | `integer` | yes | — |
| `srid` | `integer` | yes | — |
| `type` | `character varying(30)` | yes | — |

**Keys.**

- none (view)

### `public.spatial_ref_sys` — 5 columns

| Column | Type | Nullable | Default |
| --- | --- | --- | --- |
| `srid` | `integer` | no | — |
| `auth_name` | `character varying(256)` | yes | — |
| `auth_srid` | `integer` | yes | — |
| `srtext` | `character varying(2048)` | yes | — |
| `proj4text` | `character varying(2048)` | yes | — |

**Keys.**

- PK `spatial_ref_sys_pkey`: srid

## Foreign keys

Ten foreign keys exist in the catalog. Every one is single-column,
`NOT DEFERRABLE` and `ON UPDATE NO ACTION` (`confupdtype` is `a`). Nine are
`ON DELETE NO ACTION`; the tenth, `prevention.actions.assignee_id`, is
`ON DELETE RESTRICT` (`confdeltype` is `r`), because deleting a directory row
under a live assignment would silently unassign work in progress. Child and
parent are `schema.table.column`; the database is stated per group.

**Database `jobs`**

| Child | Parent | ON DELETE | ON UPDATE | Constraint |
| --- | --- | --- | --- | --- |
| `public.audit.job_id` | `public.jobs.id` | NO ACTION | NO ACTION | `audit_job_id_fkey` |
| `public.jobs.zone_id` | `public.zones.code` | NO ACTION | NO ACTION | `jobs_zone_id_fkey` |

**Database `risk`**

| Child | Parent | ON DELETE | ON UPDATE | Constraint |
| --- | --- | --- | --- | --- |
| `public.batches.model_version` | `public.models.version` | NO ACTION | NO ACTION | `batches_model_version_fkey` |
| `public.batches.zone_id` | `public.zones.code` | NO ACTION | NO ACTION | `batches_zone_id_fkey` |
| `prevention.action_evidence.action_id` | `prevention.actions.id` | NO ACTION | NO ACTION | `action_evidence_action_id_fkey` |
| `prevention.action_evidence_review.action_id` | `prevention.actions.id` | NO ACTION | NO ACTION | `action_evidence_review_action_id_fkey` |
| `prevention.action_evidence_review.evidence_id` | `prevention.action_evidence.id` | NO ACTION | NO ACTION | `action_evidence_review_evidence_id_fkey` |
| `prevention.action_history.action_id` | `prevention.actions.id` | NO ACTION | NO ACTION | `action_history_action_id_fkey` |
| `prevention.actions.assignee_id` | `prevention.directory_users.id` | RESTRICT | NO ACTION | `actions_assignee_id_fkey` |
| `prevention.action_reassessment.action_id` | `prevention.actions.id` | NO ACTION | NO ACTION | `action_reassessment_action_id_fkey` |

No other column is constrained to a parent. In particular,
`public.batches.job_id` is unique but has no FK because `jobs` lives in another
database; `prevention.action_reassessment.job_id` is unique but has no FK, and its
`batch_id` has neither; `prevention.actions.community_code` has no FK to
`public.zones.code`; `public.readings.zone_id` has no FK to `public.zones.code`
either, even though it is the first column of `readings_identity`.

## What this inventory is not

- It is not an ERD. Cardinality, optionality and the relationship diagram are
  V1-M-02; this document names keys only.
- It does not document indexes beyond uniqueness keys. Non-unique indexes exist
  and are out of scope; `models_single_active` appears because it is a unique
  partial index, which is a uniqueness key.
- It does not restate business meaning, validation rules, CHECK constraints or
  privileges. A type and a default are here; why the column exists is not.
- It is not a data inventory: it says nothing about row counts or contents.

## Migration basis

This inventory describes a database migrated from zero in an isolated compose
project, not the long-lived local stack. The project was created from
`backend/docker-compose.yml` under its own project name, so it had its own
volumes and network; `postgres` was started alone, the three migrate services
were each run once to completion, and every query in this document was run
against that container (`fw-schema-inv-postgres-1`).

| Service | Exit code | Ledger rows it wrote |
| --- | --- | --- |
| `jobs-migrate` | 0 | `jobs`: 1, 2, 3, 4 |
| `risk-migrate` | 0 | `risk`: 1–8, 10, 12, 13 |
| `prevention-migrate` | 0 | `risk`: 9, 11, 14 |

The basis is HEAD as of this change: `54d7ea1`
(`54d7ea11cdd8da5607187c50e6da1b8c5b333924`) plus
`services/prevention/migrations/014_action_assignee.sql`, the prevention
migration added by this change. That migration is now part of the basis, not an
outlier: a database migrated from the bare `54d7ea1` commit stops at `risk` 13 and
has neither `prevention.directory_users` nor `prevention.actions.assignee_id`, so
it predates this document. The migrate images were built from the working tree
that contains 14, and no `.sql` file under `services/{risk,jobs}/migrations/`
differs from the commit. The result was `jobs` at 1–4 and `risk` at 1–14, and on
that catalog `to_regclass('public.readings')` returned `readings` and `SELECT
version FROM models` returned `academic-rule-v1`, `demo-fixture-v1`,
`firewatch-rule-v2` and `ml-fixture-v1`.

The local `firewatch` stack was stale and used to disagree with this document.
Its `risk-migrate` image predates `012_readings.sql` and `013_ml_fixture_v1.sql`,
so that database's ledger stopped at 11, `public.readings` did not exist, and
`public.models` held three rows. The migration tree was not short; the running
image was. An earlier version of this document recorded that gap as a discrepancy
against the SQL tree, which is what it was — a discrepancy against a database
that had never applied HEAD. Migrating from the stated basis applies every
migration through 14, and the catalog in this document describes that database. The isolated project was
removed with `docker compose -p fw-schema-inv down -v` once the queries were
captured; the `firewatch` project was not touched.

## How to reproduce and verify

All commands run from `backend/`. They read `.env` through compose — no password
is printed, passed or stored — and `psql` connects over the container's local
socket as the container's `postgres` superuser. The project is disposable: bring
it up, migrate, query, then `docker compose -p fw-schema-inv down -v`.

```sh
docker compose -p fw-schema-inv build jobs-migrate risk-migrate prevention-migrate
docker compose -p fw-schema-inv up -d postgres
# wait for the postgres healthcheck, then apply every migration from zero
docker compose -p fw-schema-inv run --rm --no-deps jobs-migrate
docker compose -p fw-schema-inv run --rm --no-deps risk-migrate
docker compose -p fw-schema-inv run --rm --no-deps prevention-migrate
docker compose -p fw-schema-inv exec -T postgres \
  psql -U postgres -d risk -X -A -t -c "SELECT version FROM schema_migrations ORDER BY version"
docker compose -p fw-schema-inv exec -T postgres \
  psql -U postgres -d jobs -X -A -t -c "SELECT version FROM schema_migrations ORDER BY version"
```

The ledgers must read 1–14 for `risk` and 1–4 for `jobs`. Columns, types,
nullability and defaults:

```sql
SELECT table_schema, table_name, ordinal_position, column_name,
       data_type, udt_schema, udt_name, character_maximum_length,
       is_nullable, column_default, is_identity, identity_generation
FROM information_schema.columns
WHERE table_schema IN ('public', 'prevention')
ORDER BY table_schema, table_name, ordinal_position;
```

Keys (primary, foreign and unique constraints, with the delete/update actions):

```sql
SELECT n.nspname AS schema, c.relname AS table_name, con.conname, con.contype,
       pg_get_constraintdef(con.oid) AS definition,
       con.confdeltype, con.confupdtype, con.condeferrable
FROM pg_constraint con
JOIN pg_class c ON c.oid = con.conrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname IN ('public', 'prevention')
  AND c.relkind IN ('r', 'p')
  AND con.contype IN ('p', 'f', 'u')
ORDER BY 1, 2, con.contype, con.conname;
```

Unique indexes that back no constraint:

```sql
SELECT n.nspname, c.relname, i.relname, pg_get_indexdef(ix.indexrelid)
FROM pg_index ix
JOIN pg_class i ON i.oid = ix.indexrelid
JOIN pg_class c ON c.oid = ix.indrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname IN ('public', 'prevention') AND ix.indisunique
  AND NOT EXISTS (SELECT 1 FROM pg_constraint k WHERE k.conindid = ix.indexrelid)
ORDER BY 1, 2, 3;
```

Both databases use the same queries; `jobs` has only `public`, and `risk` has
`public` and `prevention`. Run each with `docker compose -p fw-schema-inv exec -T
postgres psql -U postgres -d <jobs|risk> -X -A -F '|' -c "<query>"`.

The catalog holds 19 primary keys, 10 foreign keys, 5 unique constraints and 1
unique index that backs no constraint. The document states the same numbers:
19 PK, 10 FK, 5 UNIQUE and 1 UNIQUE INDEX bullets, and the foreign-key table has
10 rows. A `- UNIQUE INDEX` bullet is a unique partial index, not a constraint,
which is why it is counted apart from the 5.

The document-versus-catalog diff extracts `(database, schema, relation, column)`
from the relation headings and the first column of each column table, then
compares it with `information_schema.columns`:

```sh
TMP=$(mktemp -d)

awk '
/^## Service `jobs`/       { db="jobs"; sch="public"; t=0; next }
/^## Service `risk`/       { db="risk"; sch="public"; t=0; next }
/^## Service `prevention`/ { db="risk"; sch="prevention"; t=0; next }
/^## Extension-owned/      { db="risk"; sch="public"; t=0; next }
/^## /                     { t=0; next }
/^### `/ {
  h=$0; sub(/^### `/,"",h); sub(/`.*/,"",h); split(h,p,"."); tbl=p[2]; t=1; next
}
t && /^\| `/ {
  c=$0; sub(/^\| `/,"",c); sub(/`.*/,"",c); print db "|" sch "|" tbl "|" c
}
' docs/schema-inventory.md | sort > "$TMP/doc.txt"

docker compose -p fw-schema-inv exec -T postgres psql -U postgres -d jobs -X -A -t -F '|' \
  -c "SELECT 'jobs|'||table_schema||'|'||table_name||'|'||column_name
      FROM information_schema.columns WHERE table_schema = 'public'" \
  | sort > "$TMP/jobs.txt"
docker compose -p fw-schema-inv exec -T postgres psql -U postgres -d risk -X -A -t -F '|' \
  -c "SELECT 'risk|'||table_schema||'|'||table_name||'|'||column_name
      FROM information_schema.columns
      WHERE table_schema IN ('public', 'prevention')" \
  | sort > "$TMP/risk.txt"

cat "$TMP/jobs.txt" "$TMP/risk.txt" | sort > "$TMP/catalog.txt"
comm -3 "$TMP/doc.txt" "$TMP/catalog.txt" | wc -l
```

The result on the isolated project was `0`. Both sides hold 138 lines: 22
relations and 138 columns — 6 relations in `jobs.public`, 7 service relations
plus 3 PostGIS relations (10 relations, 62 columns) in `risk.public`, and 6
relations (46 columns) in `risk.prevention`. Run against the previous text, which
had been read from the stale local stack, the same diff reported 8 differences,
all of them the columns of `public.readings`.
