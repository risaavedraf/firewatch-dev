#!/bin/sh
set -eu
psql --username "$POSTGRES_USER" --dbname postgres -v ON_ERROR_STOP=1 \
  -v jobs_password="$JOBS_DB_PASSWORD" -v risk_password="$RISK_DB_PASSWORD" \
  -v identity_password="$KEYCLOAK_DB_PASSWORD" <<'SQL'
CREATE ROLE jobs_app LOGIN PASSWORD :'jobs_password';
CREATE ROLE risk_app LOGIN PASSWORD :'risk_password';
CREATE ROLE identity_app LOGIN PASSWORD :'identity_password';
CREATE DATABASE jobs;
CREATE DATABASE risk;
CREATE DATABASE jobs_test;
CREATE DATABASE risk_test;
CREATE DATABASE identity OWNER identity_app;
REVOKE CONNECT ON DATABASE jobs FROM PUBLIC;
REVOKE CONNECT ON DATABASE risk FROM PUBLIC;
REVOKE CONNECT ON DATABASE jobs_test FROM PUBLIC;
REVOKE CONNECT ON DATABASE risk_test FROM PUBLIC;
REVOKE CONNECT ON DATABASE identity FROM PUBLIC;
GRANT CONNECT ON DATABASE jobs TO jobs_app;
GRANT CONNECT ON DATABASE risk TO risk_app;
GRANT CONNECT ON DATABASE jobs_test TO jobs_app;
GRANT CONNECT ON DATABASE risk_test TO risk_app;
SQL
psql --username "$POSTGRES_USER" --dbname jobs -v ON_ERROR_STOP=1 <<'SQL'
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
GRANT USAGE ON SCHEMA public TO jobs_app;
SQL
psql --username "$POSTGRES_USER" --dbname risk -v ON_ERROR_STOP=1 <<'SQL'
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
GRANT USAGE ON SCHEMA public TO risk_app;
SQL
