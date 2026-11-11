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
#
# KUBECTL overrides the client binary, the same way tests/render-contract.sh does.
# Pointing it at a recording stub shows what this script would create without a
# cluster; it takes one executable path, never extra arguments.
set -eu

# One variable for the client, so the script can be exercised against a stub.
kubectl_bin="${KUBECTL:-kubectl}"

# The namespace is declared in one place, the base manifests, and the default is
# read from there instead of repeated here: every certificate below carries
# <service>.<namespace>.svc SANs, and a namespace written by hand would leave them
# naming a Service that nothing answers for. An explicit argument still wins.
repo_root="$(cd "$(dirname "$0")/../.." && pwd)"
namespace_file="$repo_root/deploy/k8s/base/namespace.yaml"
if [ ! -f "$namespace_file" ]; then
	echo "missing $namespace_file: it declares the namespace these SANs name" >&2
	exit 1
fi
base_namespace="$(awk '/^metadata:/{m=1;next} m && /^  name:/{print $2; exit}' "$namespace_file")"
if [ -z "$base_namespace" ]; then
	echo "$namespace_file declares no metadata.name" >&2
	exit 1
fi
namespace="${1:-$base_namespace}"
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
# The CA signing key is deliberately not cluster material: it lives on the
# operator's machine, and FIREWATCH_CA_KEY points somewhere else when it does not
# live in the default location.
ca_key_file="${FIREWATCH_CA_KEY:-$HOME/.firewatch-internal-ca/ca.key}"

exists() {
	"$kubectl_bin" --namespace "$namespace" get secret "$1" >/dev/null 2>&1
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
	"$kubectl_bin" --namespace "$namespace" create secret generic "$name" $literal_args
}

echo "namespace/$namespace"
"$kubectl_bin" get namespace "$namespace" >/dev/null 2>&1 || "$kubectl_bin" create namespace "$namespace"

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
	"$kubectl_bin" --namespace "$namespace" get secret "$1" --output "jsonpath={.data.$2}" | base64 --decode
}
postgres_password="$(read_secret postgres-credentials POSTGRES_PASSWORD)"
jobs_db_password="$(read_secret postgres-credentials JOBS_DB_PASSWORD)"
risk_db_password="$(read_secret postgres-credentials RISK_DB_PASSWORD)"

apply_generic broker-credentials \
	"RABBITMQ_PASSWORD=$(random_hex)" \
	"RABBITMQ_JOBS_PASSWORD=$(random_hex)" \
	"RABBITMQ_RISK_PASSWORD=$(random_hex)" \
	"RABBITMQ_ERLANG_COOKIE=$(random_hex)"

# Read back for the same reason as the database passwords: an existing Secret is
# never rewritten, so the AMQP_URL below has to carry the password the broker was
# provisioned with, not the one this run generated.
jobs_broker_password="$(read_secret broker-credentials RABBITMQ_JOBS_PASSWORD)"
risk_broker_password="$(read_secret broker-credentials RABBITMQ_RISK_PASSWORD)"

# --- internal CA and server certificates -----------------------------------
# One CA for the cluster's internal services. The certificates name the Service
# DNS entries as SANs, because a client that verifies the hostname would reject
# a certificate issued for a bare pod name.
#
# issue_certificate SECRET_BASE [SERVICE]: the Secret is <SECRET_BASE>-tls, and the
# certificate is valid for the Service name clients dial, which is not always the
# same word (Keycloak's Service is `identity`, its Secret `keycloak-tls`).
issue_certificate() {
	secret_base="$1"
	service="${2:-$1}"
	if exists "$secret_base-tls"; then
		echo "secret/$secret_base-tls already exists, left untouched"
		return 0
	fi
	openssl req -new -newkey rsa:2048 -nodes -keyout "$workdir/$secret_base.key" \
		-out "$workdir/$secret_base.csr" -subj "/CN=$service" >/dev/null 2>&1
	# localhost and 127.0.0.1 are SANs because a port-forward is how a service is
	# exercised before its Ingress exists, and a certificate that named only the
	# in-cluster DNS would fail verification there.
	cat > "$workdir/$secret_base.ext" <<EXT
subjectAltName = DNS:$service, DNS:$service.$namespace.svc, DNS:$service.$namespace.svc.cluster.local, DNS:localhost, IP:127.0.0.1
extendedKeyUsage = serverAuth
EXT
	openssl x509 -req -in "$workdir/$secret_base.csr" -CA "$workdir/ca.crt" -CAkey "$workdir/ca.key" \
		-CAcreateserial -days 825 -extfile "$workdir/$secret_base.ext" \
		-out "$workdir/$secret_base.crt" >/dev/null 2>&1
	"$kubectl_bin" --namespace "$namespace" create secret generic "$secret_base-tls" \
		--from-file=tls.crt="$workdir/$secret_base.crt" \
		--from-file=tls.key="$workdir/$secret_base.key" \
		--from-file=ca.crt="$workdir/ca.crt"
}

if exists firewatch-ca; then
	"$kubectl_bin" --namespace "$namespace" get secret firewatch-ca --output jsonpath='{.data.ca\.crt}' | base64 --decode > "$workdir/ca.crt"
	if [ -n "$("$kubectl_bin" --namespace "$namespace" get secret firewatch-ca --output jsonpath='{.data.ca\.key}' 2>/dev/null)" ]; then
		echo "secret/firewatch-ca still carries ca.key: take it out of the cluster with" >&2
		echo "  kubectl --namespace $namespace patch secret firewatch-ca --type=json -p '[{\"op\":\"remove\",\"path\":\"/data/ca.key\"}]'" >&2
	fi
else
	openssl req -x509 -newkey rsa:2048 -nodes -days 3650 -keyout "$workdir/ca.key" \
		-out "$workdir/ca.crt" -subj "/CN=firewatch-internal-ca" >/dev/null 2>&1
	"$kubectl_bin" --namespace "$namespace" create secret generic firewatch-ca \
		--from-file=ca.crt="$workdir/ca.crt"
	# The key never leaves this machine: anything able to read Secrets in the
	# namespace could otherwise mint a certificate for any service here and answer
	# as it.
	mkdir -p "$(dirname "$ca_key_file")"
	chmod 700 "$(dirname "$ca_key_file")"
	cp "$workdir/ca.key" "$ca_key_file"
	chmod 600 "$ca_key_file"
	echo "CA private key written to $ca_key_file (0600, outside the cluster)"
fi

# --- the trust anchor the Ingress verifies the gateway against -------------
# base/ingress.yaml turns upstream verification on with
# `proxy-ssl-verify: 'on'`, names the SAN the gateway certificate carries, and
# gives the controller this Secret by name. Repackaging the CA is what makes
# that hop authenticated and not only encrypted: without an anchor the
# controller verifies the gateway against its own trust store, which has never
# heard of this CA, and every request through the edge fails.
#
# The shape below is the one ingress-nginx v1.15.1 actually reads, determined by
# installing it and pointing the annotation at candidate Secrets rather than by
# reading the annotation's documentation. The controller renders
# `proxy_ssl_trusted_certificate` from the Secret's `ca.crt` and writes it at
# /etc/ingress-controller/ssl/ca-<namespace>-<name>.pem. It treats `tls.crt`
# and `tls.key` in the same Secret as a *client* key pair it presents to the
# upstream: the CA in `tls.crt` with an unrelated `tls.key` was rejected with
# "certificate and private key does not have a matching public key", and with no
# `ca.crt` the annotation renders no trusted certificate at all. This hop asks
# for no client certificate and the gateway requires none, so the Secret carries
# one key and no key pair.
#
# The CA is copied rather than referenced. firewatch-ca would be accepted as-is
# by this controller version — it carries exactly `ca.crt` — but it is the
# cluster's distributable trust anchor, read by every workload that verifies an
# outbound peer, and this is one hop's pin: the single CA the ingress accepts
# when it authenticates the gateway. Keeping them separate means a later change
# to firewatch-ca (a rotation bundle, a second CA) needs a deliberate decision
# about the edge instead of silently widening what the Ingress accepts. The
# cost is the same rotation rule as every other Secret here: replacing the CA
# means deleting this Secret too, and until it is re-created verification fails
# closed when the gateway's certificate changes.
if exists gateway-upstream-ca; then
	echo "secret/gateway-upstream-ca already exists, left untouched"
elif [ -s "$workdir/ca.crt" ]; then
	"$kubectl_bin" --namespace "$namespace" create secret generic gateway-upstream-ca \
		--from-file=ca.crt="$workdir/ca.crt"
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
	# The two data services, then one certificate per application service: the
	# cross-contract in odd/tasks/kubernetes-deployment.md (L10) puts TLS on every
	# service port, and a per-service certificate means one pod's key cannot answer
	# as its peer.
	issue_certificate postgres
	issue_certificate rabbitmq
	issue_certificate gateway
	issue_certificate jobs
	issue_certificate risk
	issue_certificate prevention
	issue_certificate inference
	# Keycloak is the product; `identity` is the Service clients dial, so that is the
	# name the certificate has to carry.
	issue_certificate keycloak identity
else
	echo "skipping certificate issuance: the CA private key is not available" >&2
fi

# --- connection strings ----------------------------------------------------
# sslmode=verify-full with an explicit root certificate, because verify-ca would
# accept any certificate signed by the CA for any hostname. One suffix for every
# DSN, so the trust settings cannot drift between the migration owner and the
# application: the CA is mounted at the path this names.
tls_suffix="?sslmode=verify-full&sslrootcert=/etc/tls/ca.crt"

# Migration owners are separate from the application credentials on purpose: a
# migration owner can reshape the schema, and no application pod may hold that
# authority.
apply_generic migration-credentials \
	"JOBS_DATABASE_URL=postgres://postgres:$postgres_password@postgres:5432/jobs$tls_suffix" \
	"RISK_DATABASE_URL=postgres://postgres:$postgres_password@postgres:5432/risk$tls_suffix"

# What the Deployments mount: base/workloads.yaml names these three Secrets as
# `secretRef`, so without them a pod waits in CreateContainerConfigError, and a
# plaintext DSN is refused outside development. The passwords are the roles
# postgres-init.sh creates, read back from postgres-credentials above; the trust
# settings are the same suffix the migration owner gets.
#
# Two keys, and each one has an owner that reads it at startup: DATABASE_URL, and
# AMQP_URL — jobs and risk read it before building their worker
# (services/{jobs,risk}/cmd/server/transport.go), so a pod without it starts and
# never serves. prevention owns no queue and holds no broker client, so it never
# gets one.
#
# Every part of that URL comes from the base, not from a guess: host `rabbitmq` and
# port 5671 are the Service and its only AMQP listener in base/rabbitmq.yaml
# (listeners.ssl.default), vhost `firewatch` is that StatefulSet's
# RABBITMQ_DEFAULT_VHOST, and `firewatch-jobs` / `firewatch-risk` are the two users
# deploy/local/rabbitmq-topology.sh provisions, whose passwords this script keeps in
# broker-credentials. amqps because requireSecureBrokerURL refuses any other scheme
# outside development; no verification parameter because the CA is not part of the
# URL, the pod mounts it and names it in TLS_CA_FILE, and that same guard refuses
# `verify=verify_none`.
apply_generic jobs-runtime \
	"DATABASE_URL=postgres://jobs_app:$jobs_db_password@postgres:5432/jobs$tls_suffix" \
	"AMQP_URL=amqps://firewatch-jobs:$jobs_broker_password@rabbitmq:5671/firewatch"
apply_generic risk-runtime \
	"DATABASE_URL=postgres://risk_app:$risk_db_password@postgres:5432/risk$tls_suffix" \
	"AMQP_URL=amqps://firewatch-risk:$risk_broker_password@rabbitmq:5671/firewatch"
# prevention owns the `prevention` schema inside the risk database (L8), so its DSN
# names that same database: a database of its own is never created, and a DSN
# naming one would point at nothing.
apply_generic prevention-runtime \
	"DATABASE_URL=postgres://risk_app:$risk_db_password@postgres:5432/risk$tls_suffix"

# The identity provider's first administrator. Keycloak reads these two variables
# only on a first start against an empty database; from then on the account lives
# in the `identity` database and the Secret is never read again. Without it no
# administrator exists, so no realm can be created at all and the provider serves
# an empty server - the state base/keycloak.yaml would otherwise start into.
#
# Both values are generated, username included, for the same two reasons as every
# other credential here: neither is written into the repository, and apply_generic
# leaves an existing Secret untouched, so a re-run cannot replace the account an
# operator is already using. Read them back with
#   kubectl --namespace <ns> get secret keycloak-bootstrap-admin -o yaml
# and sign in as that user to create the realm.
apply_generic keycloak-bootstrap-admin \
	"KC_BOOTSTRAP_ADMIN_USERNAME=admin_$(random_hex)" \
	"KC_BOOTSTRAP_ADMIN_PASSWORD=$(random_hex)"

# --- shared scripts and the model artifact, as ConfigMaps -------------------
# The PostgreSQL role/database script, the broker plugin list, the broker
# topology script, the development realm and the inference model already exist in
# the repository and are the single description of what those things are. They are
# loaded from there rather than copied into the Kubernetes tree, so the base cannot
# drift from the Compose stack. apply -f - makes a re-run update them, which is
# safe: none of them is mounted by a running container's decision logic, they are
# read at container start.
# The ConfigMap key is the file name the container reads, not the repository file
# name: rabbitmq.yaml mounts subPath `enabled_plugins`, which is also what
# RABBITMQ_ENABLED_PLUGINS_FILE names. Declaring the key here, per entry, keeps the
# two sides of that contract visible in one place instead of deriving it from a
# basename.
# `rabbitmq-topology` is mounted by the broker-provision Job as /topology and run
# as that Job's entrypoint, so the Job declares no topology of its own.
# `keycloak-realm` is the development realm. The file carries ${...} placeholders
# that Keycloak expands from the container's environment
# (docs/keycloak-realm-setup.md): what lands in the cluster is the shape of the
# realm, never a password. Nothing mounts it - base/keycloak.yaml imports no realm
# (see its header and deploy/k8s/README.md), because the placeholders
# DEMO_ADMIN_PASSWORD, DEMO_VIEWER_PASSWORD and DIRECTORY_CLIENT_SECRET are supplied
# by no Secret here. It stays a ConfigMap so the shape of the realm an operator
# provisions out of band is readable from the cluster, not imported.
# `inference-model` is the model artifact: services/inference/testdata/logistic.json,
# byte for byte the file the Compose stack mounts. base/inference.yaml mounts this
# ConfigMap at /models and names /models/logistic.json in FIREWATCH_MODEL_PATH, so a
# pod that starts without it stops at startup naming that variable instead of serving
# a zero model. It cannot be a kustomize configMapGenerator: the base's load
# restrictor refuses a generator file outside the kustomization root.
for entry in \
	"postgres-init:deploy/local/postgres-init.sh:postgres-init.sh" \
	"rabbitmq-plugins:deploy/local/rabbitmq-plugins:enabled_plugins" \
	"rabbitmq-topology:deploy/local/rabbitmq-topology.sh:rabbitmq-topology.sh" \
	"keycloak-realm:deploy/local/firewatch-realm.json:firewatch-realm.json" \
	"inference-model:services/inference/testdata/logistic.json:logistic.json"; do
	configmap_name="${entry%%:*}"
	rest="${entry#*:}"
	source_file="$repo_root/${rest%%:*}"
	configmap_key="${rest#*:}"
	"$kubectl_bin" --namespace "$namespace" create configmap "$configmap_name" \
		--from-file="$configmap_key=$source_file" \
		--dry-run=client --output yaml | "$kubectl_bin" --namespace "$namespace" apply -f -
done

echo
echo "done. jobs-runtime and risk-runtime carry DATABASE_URL and AMQP_URL;"
echo "prevention-runtime carries DATABASE_URL only. The TLS file mounts belong to the"
echo "workload wiring (T7.7), and its mounts must keep the paths these URLs name:"
echo "/etc/tls/ca.crt."
