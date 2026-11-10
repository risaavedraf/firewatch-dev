#!/usr/bin/env sh
# Creates the namespace-level material the Kubernetes base consumes: random
# credentials and an internal CA with server certificates.
#
# It never prints a secret, never writes one into the repository, and never
# rotates silently: a Secret that already exists is left untouched, because
# replacing a password under a running service breaks it at its next reconnect.
# To rotate deliberately, delete the Secret first and re-run; the dependent
# workloads then need a restart, which is the operator's decision, not this
# script's.
#
# The CA signing key is the one piece of material this script deliberately keeps
# out of the cluster: the Secret carries only ca.crt, and ca.key stays on the
# operator's machine (FIREWATCH_CA_KEY, default ~/.firewatch-internal-ca/ca.key).
#
# Usage: ./bootstrap-secrets.sh [namespace]
set -eu

namespace="${1:-firewatch}"
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
# The CA signing key is deliberately not cluster material: it lives on the
# operator's machine, and FIREWATCH_CA_KEY points somewhere else when it does not
# live in the default location.
ca_key_file="${FIREWATCH_CA_KEY:-$HOME/.firewatch-internal-ca/ca.key}"

exists() {
	kubectl --namespace "$namespace" get secret "$1" >/dev/null 2>&1
}

random_hex() {
	# 24 bytes of hex: long enough that a password is never guessed, short enough
	# to stay readable in a connection string.
	openssl rand -hex 24
}

apply_generic() {
	# apply_generic NAME KEY=VALUE...
	name="$1"
	shift
	if exists "$name"; then
		echo "secret/$name already exists, left untouched"
		return 0
	fi
	literal_args=""
	for pair in "$@"; do
		literal_args="$literal_args --from-literal=$pair"
	done
	# shellcheck disable=SC2086 # word splitting is how kubectl receives each --from-literal
	kubectl --namespace "$namespace" create secret generic "$name" $literal_args
}

echo "namespace/$namespace"
kubectl get namespace "$namespace" >/dev/null 2>&1 || kubectl create namespace "$namespace"

# --- credentials -----------------------------------------------------------
# Generated once and then reused on every run: the StatefulSet reads them, so a
# changed value would be applied to a running database that has the old one.
postgres_password="$(random_hex)"
jobs_db_password="$(random_hex)"
risk_db_password="$(random_hex)"
keycloak_db_password="$(random_hex)"
apply_generic postgres-credentials \
	"POSTGRES_PASSWORD=$postgres_password" \
	"JOBS_DB_PASSWORD=$jobs_db_password" \
	"RISK_DB_PASSWORD=$risk_db_password" \
	"KEYCLOAK_DB_PASSWORD=$keycloak_db_password"

# If the secret already existed, the generated values above are not the ones in
# the cluster. Read them back so the derived URLs below match what the database
# will actually accept.
read_secret() {
	kubectl --namespace "$namespace" get secret "$1" --output "jsonpath={.data.$2}" | base64 --decode
}
postgres_password="$(read_secret postgres-credentials POSTGRES_PASSWORD)"
jobs_db_password="$(read_secret postgres-credentials JOBS_DB_PASSWORD)"
risk_db_password="$(read_secret postgres-credentials RISK_DB_PASSWORD)"

apply_generic broker-credentials \
	"RABBITMQ_PASSWORD=$(random_hex)" \
	"RABBITMQ_JOBS_PASSWORD=$(random_hex)" \
	"RABBITMQ_RISK_PASSWORD=$(random_hex)" \
	"RABBITMQ_ERLANG_COOKIE=$(random_hex)"

# --- internal CA and server certificates -----------------------------------
# One CA for the cluster's internal services. The certificates name the Service
# DNS entries as SANs, because a client that verifies the hostname would reject
# a certificate issued for a bare pod name.
issue_certificate() {
	common_name="$1"
	shift
	if exists "$common_name-tls"; then
		echo "secret/$common_name-tls already exists, left untouched"
		return 0
	fi
	openssl req -new -newkey rsa:2048 -nodes -keyout "$workdir/$common_name.key" \
		-out "$workdir/$common_name.csr" -subj "/CN=$common_name" >/dev/null 2>&1
	cat > "$workdir/$common_name.ext" <<EXT
subjectAltName = DNS:$common_name, DNS:$common_name.$namespace.svc, DNS:$common_name.$namespace.svc.cluster.local
extendedKeyUsage = serverAuth
EXT
	openssl x509 -req -in "$workdir/$common_name.csr" -CA "$workdir/ca.crt" -CAkey "$workdir/ca.key" \
		-CAcreateserial -days 825 -extfile "$workdir/$common_name.ext" \
		-out "$workdir/$common_name.crt" >/dev/null 2>&1
	kubectl --namespace "$namespace" create secret generic "$common_name-tls" \
		--from-file=tls.crt="$workdir/$common_name.crt" \
		--from-file=tls.key="$workdir/$common_name.key" \
		--from-file=ca.crt="$workdir/ca.crt"
}

if exists firewatch-ca; then
	kubectl --namespace "$namespace" get secret firewatch-ca --output jsonpath='{.data.ca\.crt}' | base64 --decode > "$workdir/ca.crt"
	if [ -n "$(kubectl --namespace "$namespace" get secret firewatch-ca --output jsonpath='{.data.ca\.key}' 2>/dev/null)" ]; then
		echo "secret/firewatch-ca still carries ca.key: take it out of the cluster with" >&2
		echo "  kubectl --namespace $namespace patch secret firewatch-ca --type=json -p '[{\"op\":\"remove\",\"path\":\"/data/ca.key\"}]'" >&2
	fi
else
	openssl req -x509 -newkey rsa:2048 -nodes -days 3650 -keyout "$workdir/ca.key" \
		-out "$workdir/ca.crt" -subj "/CN=firewatch-internal-ca" >/dev/null 2>&1
	kubectl --namespace "$namespace" create secret generic firewatch-ca \
		--from-file=ca.crt="$workdir/ca.crt"
	# The key never leaves this machine: anything able to read Secrets in the
	# namespace could otherwise mint a certificate for postgres or rabbitmq and
	# answer as either of them.
	mkdir -p "$(dirname "$ca_key_file")"
	chmod 700 "$(dirname "$ca_key_file")"
	cp "$workdir/ca.key" "$ca_key_file"
	chmod 600 "$ca_key_file"
	echo "CA private key written to $ca_key_file (0600, outside the cluster)"
fi

# New certificates need the signing key, which the cluster no longer holds. When
# the local copy is missing the operator supplies it through FIREWATCH_CA_KEY; the
# existing certificates and the secret are never rewritten here.
if [ -s "$ca_key_file" ]; then
	cp "$ca_key_file" "$workdir/ca.key"
else
	echo "no CA private key at $ca_key_file: existing certificates are kept, new ones cannot be issued" >&2
fi

if [ -s "$workdir/ca.key" ]; then
	issue_certificate postgres
	issue_certificate rabbitmq
else
	echo "skipping certificate issuance: the CA private key is not available" >&2
fi

# --- migration-owner connection strings ------------------------------------
# Separate from the application credentials on purpose: a migration owner can
# reshape the schema, and no application pod may hold that authority.
# sslmode=verify-full with an explicit root certificate, because verify-ca would
# accept any certificate signed by the CA for any hostname.
migration_suffix="?sslmode=verify-full&sslrootcert=/etc/tls/ca.crt"
apply_generic migration-credentials \
	"JOBS_DATABASE_URL=postgres://postgres:$postgres_password@postgres:5432/jobs$migration_suffix" \
	"RISK_DATABASE_URL=postgres://postgres:$postgres_password@postgres:5432/risk$migration_suffix"

# --- shared scripts, as ConfigMaps -----------------------------------------
# The PostgreSQL role/database script, the broker plugin list and the broker
# topology script already exist in deploy/local and are the single description of
# what those things are. They are loaded from there rather than copied into the
# Kubernetes tree, so the base cannot drift from the Compose stack. apply -f -
# makes a re-run update them, which is safe: none of them is mounted by a running
# container's decision logic, they are read at container start.
repo_root="$(cd "$(dirname "$0")/../.." && pwd)"
# The ConfigMap key is the file name the container reads, not the repository file
# name: rabbitmq.yaml mounts subPath `enabled_plugins`, which is also what
# RABBITMQ_ENABLED_PLUGINS_FILE names. Declaring the key here, per entry, keeps the
# two sides of that contract visible in one place instead of deriving it from a
# basename.
# `rabbitmq-topology` is mounted by the broker-provision Job as /topology and run
# as that Job's entrypoint, so the Job declares no topology of its own.
for entry in "postgres-init:deploy/local/postgres-init.sh:postgres-init.sh" "rabbitmq-plugins:deploy/local/rabbitmq-plugins:enabled_plugins" "rabbitmq-topology:deploy/local/rabbitmq-topology.sh:rabbitmq-topology.sh"; do
	configmap_name="${entry%%:*}"
	rest="${entry#*:}"
	source_file="$repo_root/${rest%%:*}"
	configmap_key="${rest#*:}"
	kubectl --namespace "$namespace" create configmap "$configmap_name" \
		--from-file="$configmap_key=$source_file" \
		--dry-run=client --output yaml | kubectl --namespace "$namespace" apply -f -
done

echo
echo "done. Application runtime secrets are created when the workloads are wired (T7),"
echo "because they carry the broker trust settings that the workload mounts must match."
