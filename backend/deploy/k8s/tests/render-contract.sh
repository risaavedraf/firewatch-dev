#!/usr/bin/env sh
# Render contract for the Kubernetes base.
#
# What this checks and what it does not: it renders the base with kustomize and
# asserts the properties the bootstrap Jobs depend on — migration order, the DSN
# keys they read, the CA both of them need, the network ports that were opened
# for them, and the Pod Security fields admission will require. It cannot check
# admission, scheduling, CNI enforcement or any actual migration: that starts with
# T9, on a cluster. Everything here is a property of the manifests, so it fails
# when a manifest drifts and stays silent when a cluster would.
#
#   sh backend/deploy/k8s/tests/render-contract.sh
#
# KUBECTL overrides the binary. On Windows, kubectl ships inside Docker Desktop
# and is not on PATH by default, so the usual value is
#   KUBECTL="/c/Program Files/Docker/Docker/resources/bin/kubectl.exe"
set -eu

cd "$(dirname "$0")/.."

K=${KUBECTL:-kubectl}
if ! command -v "$K" >/dev/null 2>&1; then
	if [ -x "/c/Program Files/Docker/Docker/resources/bin/kubectl.exe" ]; then
		K="/c/Program Files/Docker/Docker/resources/bin/kubectl.exe"
	else
		echo "FAIL: no kubectl. Set KUBECTL to its path." >&2
		exit 1
	fi
fi

fail() {
	echo "FAIL: $1" >&2
	exit 1
}

render=$("$K" kustomize base) || fail "kustomize render"

# --- the two Jobs exist, by the names the documentation uses ------------------
printf '%s\n' "$render" | grep -q 'name: broker-provision' ||
	fail "broker-provision Job missing from the render"
printf '%s\n' "$render" | grep -q '^  name: migrate$' ||
	fail "migrate Job missing from the render"

# --- migration order is a pod-level guarantee of the API server ----------------
init_order=$(awk '/^      initContainers:/{f=1;next} /^      containers:/{f=0} f&&/^        - name:/{print $3}' base/jobs-migrate.yaml | tr '\n' ' ')
[ "$init_order" = "migrate-risk migrate-prevention " ] ||
	fail "initContainer order is '$init_order', want 'migrate-risk migrate-prevention '"
main_order=$(awk '/^      containers:/{f=1;next} /^      volumes:/{f=0} f&&/^        - name:/{print $3}' base/jobs-migrate.yaml)
[ "$main_order" = "migrate-jobs" ] ||
	fail "main container is '$main_order', want 'migrate-jobs' after the two init steps"

# --- every step reads the database its ledger actually lives in ---------------
[ "$(grep -c 'key: RISK_DATABASE_URL' base/jobs-migrate.yaml)" = 2 ] ||
	fail "risk and prevention must both read RISK_DATABASE_URL: the prevention schema lives in the risk database"
[ "$(grep -c 'key: JOBS_DATABASE_URL' base/jobs-migrate.yaml)" = 1 ] ||
	fail "the jobs step must read JOBS_DATABASE_URL"
[ "$(grep -c 'mountPath: /etc/tls' base/jobs-migrate.yaml)" = 3 ] ||
	fail "all three migration steps need the CA mounted where sslrootcert points"
grep -q 'secretName: firewatch-ca' base/jobs-migrate.yaml ||
	fail "the migration Job does not mount the internal CA"
[ "$(grep -c 'sslmode=verify-full&sslrootcert=/etc/tls/ca.crt' bootstrap-secrets.sh)" = 1 ] ||
	fail "the migration DSN no longer demands verify-full against /etc/tls/ca.crt"

# --- broker provisioning runs the repository script, over TLS -----------------
grep -q 'value: https://rabbitmq:15671' base/broker-provision.yaml ||
	fail "broker-provision must reach the management API over TLS on 15671"
grep -q 'value: /etc/tls/ca.crt' base/broker-provision.yaml ||
	fail "broker-provision must verify the management certificate against the internal CA"
grep -q 'name: rabbitmq-topology' base/broker-provision.yaml ||
	fail "broker-provision must mount the topology script, not carry a copy"
grep -q 'rabbitmq-topology:deploy/local/rabbitmq-topology.sh:rabbitmq-topology.sh' bootstrap-secrets.sh ||
	fail "bootstrap-secrets.sh no longer renders the topology ConfigMap from the repository file"
[ "$(grep -c 'command: \[sh, /topology/rabbitmq-topology.sh\]' base/broker-provision.yaml)" = 1 ] ||
	fail "broker-provision must run the mounted script"

# --- the network reaches exactly these two destinations ----------------------
[ "$(grep -c 'port: 15671' base/network.yaml)" = 2 ] ||
	fail "the management port must appear once as ingress and once as egress, in no third place"
[ "$(awk '/name: rabbitmq-management/,/^---/' base/network.yaml | grep -c 'app: broker-provision')" = 1 ] ||
	fail "only broker-provision may reach the management port"
[ "$(awk '/name: postgres-entry/,/^---/' base/network.yaml | grep -c 'migrate')" = 1 ] ||
	fail "postgres-entry must admit the migrate Job's label"
[ "$(awk '/name: migrate-egress/,/^---/' base/network.yaml | grep -c 'port: 5432')" = 1 ] ||
	fail "migrate-egress must reach postgres:5432"
[ "$(awk '/name: broker-provision-egress/,/^---/' base/network.yaml | grep -c 'port: 15671')" = 1 ] ||
	fail "broker-provision-egress must reach the management port"

# --- Pod Security "restricted" is declared, not assumed ----------------------
for f in base/jobs-migrate.yaml base/broker-provision.yaml; do
	grep -q 'runAsNonRoot: true' "$f" || fail "$f does not declare runAsNonRoot"
	grep -q 'seccompProfile' "$f" || fail "$f does not declare a seccomp profile"
	grep -q 'allowPrivilegeEscalation: false' "$f" || fail "$f allows privilege escalation"
	grep -q 'readOnlyRootFilesystem: true' "$f" || fail "$f has a writable root filesystem"
done
[ "$(grep -c 'drop: \[ALL\]' base/jobs-migrate.yaml)" = 3 ] ||
	fail "every migration container must drop ALL capabilities"

# --- no credential lives in a manifest ---------------------------------------
if grep -nEi 'pass(word)?[:=] *[a-z0-9]{6,}|BEGIN ([A-Z]+ )?PRIVATE KEY' \
	base/jobs-migrate.yaml base/broker-provision.yaml >/dev/null 2>&1; then
	fail "a literal credential appears in a manifest"
fi

echo "PASS: render contract holds for the bootstrap Jobs"
