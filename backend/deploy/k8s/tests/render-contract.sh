#!/usr/bin/env sh
# Render contract for the Kubernetes base and the delivery overlay.
#
# What this checks and what it does not: it renders the base with kustomize and
# asserts the properties the bootstrap Jobs depend on — migration order, the DSN
# keys they read, the CA both of them need, the network ports that were opened
# for them, and the Pod Security fields admission will require — and then the
# properties the application workloads depend on, which is the crossing contract
# of odd/tasks/kubernetes-deployment.md (L10) as manifests: the three TLS
# variables and the `<service>-tls` Secret each Deployment mounts, the `https`
# name on every port and probe, the `https://` upstreams in config.yaml, the
# identity provider and the model service, and the ingress hop. It cannot check
# admission, scheduling, CNI enforcement or any actual handshake: that starts
# with T9, on a cluster. Everything here is a property of the manifests, so it
# fails when a manifest drifts and stays silent when a cluster would.
#
# It also renders overlays/delivery and asserts that every application image
# there is pinned by digest while the base still carries its placeholder tag:
# S1 of odd/tasks/k8s-delivery-closure.md is that sentence, and it is the clause
# that kept T7 open. Both renders are needed, so kubectl is required even when
# RENDER_FROM supplies the base render.
#
#   sh backend/deploy/k8s/tests/render-contract.sh
#
# KUBECTL overrides the binary. On Windows, kubectl ships inside Docker Desktop
# and is not on PATH by default, so the usual value is
#   KUBECTL="/c/Program Files/Docker/Docker/resources/bin/kubectl.exe"
#
# RENDER_FROM replaces the render: a file path, or `-` for stdin. It exists so
# this contract can be put through its own assertions against a deliberately
# broken copy without a manifest being edited:
#   kubectl kustomize base | sed 's/name: https/name: http/' |
#       RENDER_FROM=- sh backend/deploy/k8s/tests/render-contract.sh
# The greps that read base/*.yaml directly are unaffected by it; every check
# added for the application workloads reads the render.
set -eu

cd "$(dirname "$0")/.."

fail() {
	echo "FAIL: $1" >&2
	exit 1
}

# The client is resolved before the render because the delivery section below
# renders the overlay too, and no RENDER_FROM can stand in for that render.
K=${KUBECTL:-kubectl}
if ! command -v "$K" >/dev/null 2>&1; then
	if [ -x "/c/Program Files/Docker/Docker/resources/bin/kubectl.exe" ]; then
		K="/c/Program Files/Docker/Docker/resources/bin/kubectl.exe"
	else
		echo "FAIL: no kubectl. Set KUBECTL to its path." >&2
		exit 1
	fi
fi

if [ -n "${RENDER_FROM:-}" ]; then
	# A caller-supplied render. `cat -` reads stdin, which is the `-` case.
	render=$(cat "$RENDER_FROM") || fail "cannot read RENDER_FROM=$RENDER_FROM"
else
	render=$("$K" kustomize base) || fail "kustomize render"
fi

# One rendered document, by kind and name. The assertions below hold over a
# named object, because "somewhere in the render" is not that claim: the four
# services carry the same three variables and the same port name, so a check
# that scanned the whole render would pass with one of them gone.
doc() {
	printf '%s\n' "$render" | awk -v kind="$1" -v name="$2" '
		function emit() {
			if (dkind == kind && dname == name) printf "%s", buf
			buf = ""; dkind = ""; dname = ""; named = 0
		}
		/^---$/ { emit(); next }
		{ buf = buf $0 "\n" }
		/^kind: / { dkind = $2 }
		/^  name: / && !named { dname = $2; named = 1 }
		END { emit() }
	'
}

# The literal a container env var carries. Empty for a valueFrom reference,
# which is the other assertion's business.
value_of() {
	printf '%s\n' "$1" | awk -v want="$2" '
		/^ *- +name: / { if (found) exit; if ($3 == want) found = 1; next }
		found && /^ +value: / { print $2; exit }
	'
}

# The name bound to a container port, as the Service and the probes spell it.
port_name() {
	printf '%s\n' "$1" | awk -v want="$2" '
		/^ *- +containerPort: / { p = $3 }
		p == want && /^ +name: / { print $2; exit }
	'
}

# The port each probe addresses, one per line.
probe_ports() {
	printf '%s\n' "$1" | awk '
		/^ *(readinessProbe|livenessProbe|startupProbe):$/ { p = 1; next }
		p && /^ *port: / { print $2; p = 0 }
	'
}

# A Service's `name port targetPort`, which is one claim and not three.
service_port() {
	printf '%s\n' "$1" | awk '
		/^ *- +name: / { n = $3 }
		/^ +port: / { p = $2 }
		/^ +targetPort: / { t = $2 }
		END { print n, p, t }
	'
}

# The shape all six TLS pods share: the container port is named `https`, no
# probe addresses another port in clear, and the certificate directory is
# mounted where the variables point. Six call sites, one failure to name.
tls_pod() {
	[ "$(port_name "$2" "$3")" = https ] ||
		fail "$1's container port $3 is not named https: the Service targetPort and the probes address it by that name"
	probes=$(probe_ports "$2")
	[ -n "$probes" ] ||
		fail "$1 declares no probe that addresses a port: nothing would take a replica out of the Service"
	offport=$(printf '%s\n' "$probes" | grep -vx https | tr '\n' ' ')
	[ -z "$offport" ] ||
		fail "$1 probes ports '$offport', want 'https': a plaintext probe never gets a 200 from a TLS listener"
	if printf '%s\n' "$2" | grep -q '^ *scheme: HTTP$'; then
		fail "$1 probes its TLS listener with scheme: HTTP"
	fi
	printf '%s\n' "$2" | grep -q '^ *- mountPath: /etc/tls$' ||
		fail "$1 does not mount its certificate directory at /etc/tls: the paths its variables name are relative to it"
}

# The application images a render carries, one per line, read from stdin. It
# selects by repository rather than by tag, so a placeholder tag and a pinned
# digest both come through and the two checks below can tell them apart. The
# registry host may carry a port, which is why the tag is stripped after the
# digest and not the other way round.
app_images() {
	awk '
		/^ *image: / {
			repo = $2
			sub(/@.*/, "", repo)
			sub(/:[^\/]*$/, "", repo)
			if (repo ~ /(^|\/)firewatch\/(gateway|jobs|risk|prevention|inference)$/)
				print $2
		}
	'
}

# The application images still carrying the base placeholder tag.
placeholder_images() {
	app_images | grep -F ':replace-before-deploy' || true
}

# The application images that are not pinned by digest.
unpinned_images() {
	app_images | grep -vE '^[^ ]+@sha256:[0-9a-f]{64}$' || true
}

# How many objects a render contains, counted by the one top-level key every
# document has. A digest pin adds no object and removes none, so an overlay that
# changes the count is doing something this contract does not describe.
object_count() {
	awk '/^kind: / { n++ } END { print n + 0 }'
}

# --- the two Jobs exist, by the names the documentation uses ------------------
printf '%s\n' "$render" | grep -q 'name: broker-provision' ||
	fail "broker-provision Job missing from the render"
# The Job is named after the schema version it produces. A fixed name survives
# `apply`, and a completed Job is never re-run, so a release with new migrations
# would silently skip them. This keeps the name honest with the tree.
schema_version=$(ls ../../services/risk/migrations/*.sql ../../services/prevention/migrations/*.sql |
	sed 's|.*/||' | grep -oE '^[0-9]+' | sort -n | tail -1 | sed 's/^0*//')
[ -n "$schema_version" ] || fail "could not read the migration versions from the tree"
printf '%s\n' "$render" | grep -q "^  name: migrate-v$schema_version$" ||
	fail "the migrate Job is not named migrate-v$schema_version: a stale name is not re-run by apply and would skip the new migrations"

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

# --- no container mounts a volume the pod does not define ---------------------
# The pod spec is rejected at admission when a volumeMount names a volume that
# does not exist, and kustomize renders that happily. This check exists because
# exactly that got through once: the CA mount was added without its volume, and
# the broker Job would never have started. It reads the render and every pod in
# it, not the two Job files: T7.7 added mounts to five more workloads, and the
# version that only read the Job files could not see them. A name in
# `volumeClaimTemplates` counts as declared, because that is where a StatefulSet
# pod's claim volume comes from.
unpinned=$(printf '%s\n' "$render" | awk '
	function indent(s) { match(s, /^ */); return RLENGTH }
	function clear(a,   k) { for (k in a) delete a[k] }
	function reset() { vm = -1; vo = -1; vt = -1; clear(mounted); clear(declared) }
	function report(   m) {
		for (m in mounted)
			if (!(m in declared)) {
				bad = bad (bad == "" ? "" : "; ") who ": mounts volume \"" m "\" but never declares it"
			}
	}
	/^---$/ { report(); reset(); who = ""; named = 0; next }
	/^  name: / && !named { who = $2; named = 1 }
	/^ *volumeMounts:[ ]*$/ { vm = indent($0); next }
	/^ *volumes:[ ]*$/ { vo = indent($0); next }
	/^ *volumeClaimTemplates:[ ]*$/ { vt = indent($0); next }
	{
		i = indent($0)
		if (vm >= 0 && i < vm) vm = -1
		if (vo >= 0 && i < vo) vo = -1
		if (vt >= 0 && i < vt) vt = -1
		if (vm >= 0) {
			if (i == vm && $0 !~ /^ *- /) vm = -1
			else if (i == vm && $0 ~ /- name: /) { split($0, f, "- name: "); mounted[f[2]] = 1 }
			else if (i == vm + 2 && $0 ~ /^ *name: /) mounted[$2] = 1
		}
		if (vo >= 0) {
			if (i == vo && $0 !~ /^ *- /) vo = -1
			else if (i == vo && $0 ~ /- name: /) { split($0, f, "- name: "); declared[f[2]] = 1 }
			else if (i == vo + 2 && $0 ~ /^ *name: /) declared[$2] = 1
		}
		if (vt >= 0 && i > vt && $0 ~ /^ *name: /) declared[$2] = 1
	}
	END { report(); if (bad != "") print bad }')
[ -z "$unpinned" ] || fail "$unpinned"

# --- the two volumes that carry trust and topology are sourced, not empty ----
grep -q 'secretName: firewatch-ca' base/broker-provision.yaml ||
	fail "broker-provision must source its CA from the firewatch-ca Secret"

# --- every application Deployment serves TLS on a port named `https` ---------
# The crossing contract fixes one shape for all four services: the key pair at
# the two paths the runtime reads, the internal CA at the third for everything
# they call outbound, the pod's own `<service>-tls` Secret at /etc/tls, and the
# port named `https` in the container, the Service and both probes.
# bootstrap-secrets.sh issues exactly `<service>-tls`, so a rename on either
# side leaves a pod unable to read its certificate; with APP_ENV=staging that
# is a crash-loop at rollout, not a test that failed earlier.
for pair in gateway:8080 jobs:8080 risk:8080 prevention:8084; do
	svc=${pair%%:*}
	port=${pair#*:}
	d=$(doc Deployment "$svc")
	[ -n "$d" ] || fail "Deployment $svc is missing from the render"

	for tls in TLS_CERT_FILE=/etc/tls/tls.crt \
		TLS_KEY_FILE=/etc/tls/tls.key \
		TLS_CA_FILE=/etc/tls/ca.crt; do
		var=${tls%%=*}
		want=${tls#*=}
		got=$(value_of "$d" "$var")
		[ "$got" = "$want" ] ||
			fail "$svc sets $var to '$got', want $want: a listener or a client that reads a path nothing is mounted at stops at startup"
	done
	printf '%s\n' "$d" | grep -q "^ *secretName: $svc-tls\$" ||
		fail "$svc does not mount the $svc-tls Secret: that is the name bootstrap-secrets.sh issues, and no other one is rotated with it"

	tls_pod "$svc" "$d" "$port"

	s=$(doc Service "$svc")
	[ -n "$s" ] || fail "Service $svc is missing from the render"
	[ "$(service_port "$s")" = "https $port https" ] ||
		fail "Service $svc publishes '$(service_port "$s")', want 'https $port https': a plaintext targetPort is the hop the contract forbids"
done
# The other half of that name, because a Secret nothing issues renders exactly
# like one the bootstrap script creates. inference is on this list too: its pod
# mounts the same shape (inference.yaml).
for svc in gateway jobs risk prevention inference; do
	grep -q "issue_certificate $svc\$" bootstrap-secrets.sh ||
		fail "bootstrap-secrets.sh does not issue the $svc-tls Secret the $svc pod mounts"
done

# --- the shared config names no plaintext hop --------------------------------
# APP_ENV=staging is what keeps the guards armed: it is the reason a missing
# certificate variable is a refusal at startup and not a plaintext fallback.
# Every URL below is the contract's, and `requireSecureUpstreams` refuses a
# plaintext one, so a scheme that drifts to http:// is a gateway that will not
# start and a host or port that drifts is one that starts and reaches nothing.
cfg=$(doc ConfigMap firewatch-config)
[ -n "$cfg" ] || fail "the firewatch-config ConfigMap is missing from the render"
printf '%s\n' "$cfg" | grep -q '^  APP_ENV: staging$' ||
	fail "APP_ENV is not staging: outside development the TLS guards apply, and this contract assumes they do"
for url in \
	JOBS_URL=https://jobs:8080 \
	RISK_URL=https://risk:8080 \
	PREVENTION_URL=https://prevention:8084 \
	INFERENCE_URL=https://inference:8080 \
	OIDC_ISSUER=https://identity:8443/realms/firewatch \
	OIDC_JWKS_URL=https://identity:8443/realms/firewatch/protocol/openid-connect/certs; do
	key=${url%%=*}
	want=${url#*=}
	printf '%s\n' "$cfg" | grep -q "^  $key: $want\$" ||
		fail "$key is not $want: the client that reads it either refuses the hop or cannot resolve it"
done
plain=$(printf '%s\n' "$cfg" | grep -E '^  [A-Z_]+: http://' | tr '\n' ' ')
[ -z "$plain" ] || fail "a plaintext URL is back in the shared config: $plain"

# --- the identity provider and the model service are objects in the base ----
# config.yaml names both by Service. Without these two objects, OIDC_ISSUER and
# INFERENCE_URL name hosts nothing answers for: every key-set fetch fails in
# DNS, and risk stops before its first prediction because registering the model
# classifier reads /v1/model at startup.
kc=$(doc Deployment keycloak)
[ -n "$kc" ] || fail "the keycloak Deployment is missing: every OIDC URL in firewatch-config would name a host nothing answers for"
printf '%s\n' "$kc" | grep -q '^ *secretName: keycloak-tls$' ||
	fail "keycloak does not mount the keycloak-tls Secret: it would start with no certificate to serve"
grep -q 'issue_certificate keycloak identity' bootstrap-secrets.sh ||
	fail "bootstrap-secrets.sh issues keycloak-tls without the identity SAN: clients dial https://identity:8443 and the name would not be covered"
grep -q 'apply_generic keycloak-bootstrap-admin' bootstrap-secrets.sh ||
	fail "bootstrap-secrets.sh no longer creates the keycloak-bootstrap-admin Secret the Deployment reads"
for tls in KC_HTTPS_CERTIFICATE_FILE=/etc/tls/tls.crt \
	KC_HTTPS_CERTIFICATE_KEY_FILE=/etc/tls/tls.key; do
	var=${tls%%=*}
	want=${tls#*=}
	got=$(value_of "$kc" "$var")
	[ "$got" = "$want" ] || fail "keycloak sets $var to '$got', want $want"
done
# Both administrator keys, by reference and never baked in: on a first start
# against an empty `identity` database, a provider with no administrator can
# never have a realm created in it.
[ "$(printf '%s\n' "$kc" | grep -c '^ *name: keycloak-bootstrap-admin$')" = 2 ] ||
	fail "keycloak must take both administrator keys by secretKeyRef from keycloak-bootstrap-admin: one reference is half an account"
for var in KC_BOOTSTRAP_ADMIN_USERNAME KC_BOOTSTRAP_ADMIN_PASSWORD; do
	printf '%s\n' "$kc" | grep -q "^ *- name: $var\$" ||
		fail "keycloak no longer reads $var: the Secret can be mounted and the variable gone, and a first start then has no administrator"
done
tls_pod keycloak "$kc" 8443
ident=$(doc Service identity)
[ -n "$ident" ] || fail "Service identity is missing: every issuer and key-set URL in firewatch-config names it"
[ "$(service_port "$ident")" = "https 8443 https" ] ||
	fail "Service identity publishes '$(service_port "$ident")', want 'https 8443 https': the contract fixes 8443 and every client dials it by name"

inf=$(doc Deployment inference)
[ -n "$inf" ] || fail "the inference Deployment is missing: INFERENCE_URL would name a host nothing answers for"
model_path=$(value_of "$inf" FIREWATCH_MODEL_PATH)
[ "$model_path" = /models/logistic.json ] ||
	fail "inference serves the model at '$model_path', want /models/logistic.json: the artifact has no default and a pod without it stops at startup"
for tls in TLS_CERT_FILE=/etc/tls/tls.crt TLS_KEY_FILE=/etc/tls/tls.key; do
	var=${tls%%=*}
	want=${tls#*=}
	got=$(value_of "$inf" "$var")
	[ "$got" = "$want" ] || fail "inference sets $var to '$got', want $want: it terminates TLS on 8080 like the other four"
done
printf '%s\n' "$inf" | grep -q '^ *secretName: inference-tls$' ||
	fail "inference does not mount the inference-tls Secret"
printf '%s\n' "$inf" | grep -q '^ *- mountPath: /models$' ||
	fail "inference does not mount the artifact directory at /models: the path FIREWATCH_MODEL_PATH names would be absent from the pod"
printf '%s\n' "$inf" | grep -q '^ *name: inference-model$' ||
	fail "inference no longer mounts the inference-model ConfigMap: the pod would wait for an artifact nothing supplies"
tls_pod inference "$inf" 8080
isvc=$(doc Service inference)
[ -n "$isvc" ] || fail "Service inference is missing: INFERENCE_URL names it"
[ "$(service_port "$isvc")" = "https 8080 https" ] ||
	fail "Service inference publishes '$(service_port "$isvc")', want 'https 8080 https'"
grep -q 'inference-model:services/inference/testdata/logistic.json:logistic.json' bootstrap-secrets.sh ||
	fail "bootstrap-secrets.sh no longer renders inference-model from services/inference/testdata/logistic.json"

# --- the edge tells NGINX the gateway speaks TLS -----------------------------
# NGINX proxies every backend in plaintext unless the annotation says otherwise,
# and this is the hop where the mistake hides: plaintext into a TLS listener
# fails each request with a 502, not the rollout.
ing=$(doc Ingress firewatch)
[ -n "$ing" ] || fail "the Ingress is missing from the render"
printf '%s\n' "$ing" | grep -q '^ *nginx.ingress.kubernetes.io/backend-protocol: HTTPS$' ||
	fail "the Ingress does not declare backend-protocol HTTPS: NGINX would proxy the gateway's TLS port in plaintext"

# --- the two new flows are open on both halves -------------------------------
# default-deny closes every pod, and a policy that opens only egress leaves the
# destination shut, which is the defect L6 found in the data plane. The model
# and the identity provider each need an entry rule as well as their clients'
# egress rule; drop either half and the flow renders and never passes a byte.
for policy in risk-to-inference inference-entry oidc-egress identity-entry-and-data; do
	[ -n "$(doc NetworkPolicy "$policy")" ] ||
		fail "NetworkPolicy $policy is missing from the render: one half of the model or the identity flow is closed"
done

# --- the delivery overlay pins every application image by digest -------------
# S1 of odd/tasks/k8s-delivery-closure.md is this sentence and nothing more: an
# overlay pins the images, and the base needs no manual substitution. A digest
# belongs in the overlay because it names one built artifact rather than a
# service: pinned in the base, that file would be valid for a single registry and
# a single build, and every environment would edit the same tracked file to point
# at its own copy of the same commit.
delivery=$("$K" kustomize overlays/delivery) ||
	fail "kustomize render of overlays/delivery: the delivery overlay does not render"

stale=$(printf '%s\n' "$delivery" | placeholder_images | tr '\n' ' ')
[ -z "$stale" ] ||
	fail "the delivery render still carries the base placeholder tag: $stale"

# The other end of the same claim: a `newTag` renders an image some registry may
# serve from a mutable tag tomorrow, and only a digest makes a rollout the same
# bytes the review approved.
untagged=$(printf '%s\n' "$delivery" | unpinned_images | tr '\n' ' ')
[ -z "$untagged" ] ||
	fail "an application image in the delivery render is not pinned by digest: $untagged"

# The five services are named here rather than derived from the list, because a
# service that leaves the base and the overlay together would leave the two
# checks above with nothing to read.
for svc in gateway jobs risk prevention inference; do
	printf '%s\n' "$delivery" | app_images | grep -q "/firewatch/$svc@" ||
		fail "the delivery render carries no $svc image pinned by digest: the overlay does not pin every service the cluster runs"
done

# The base is the other half of S1: it keeps the placeholder so it stays a
# template, and the digests live in the overlay. A digest pinned here would
# satisfy the checks above and destroy the property they exist for.
base_render=$("$K" kustomize base) || fail "kustomize render of base"
for svc in gateway jobs risk prevention inference; do
	printf '%s\n' "$base_render" |
		grep -q "image: firewatch/$svc:replace-before-deploy" ||
		fail "the base no longer renders firewatch/$svc:replace-before-deploy: the placeholder is what keeps it a template, and a digest here would name one registry for every environment"
done

# An overlay pins images; it does not add or drop objects. The base is rendered
# again above rather than reused: RENDER_FROM may have replaced the first render
# with a caller's copy, and an object count compared against mutated input proves
# nothing.
base_objects=$(printf '%s\n' "$base_render" | object_count)
delivery_objects=$(printf '%s\n' "$delivery" | object_count)
[ "$delivery_objects" = "$base_objects" ] ||
	fail "the delivery render has $delivery_objects objects and the base $base_objects: an image pin must not add or drop one"

# --- those two failures are proven, not assumed ------------------------------
# An assertion nobody has seen fail is a guess, and the checks above would pass
# just as happily if they read nothing. So the defects they exist to catch are
# injected into a scratch copy of the overlay and the same two predicates are
# asked again. This is the intent of the RENDER_FROM note at the top, for a
# section whose input has to be re-rendered: the mutation here is a manifest
# mistake, not a text edit on its output. Each defect is caught by one check and
# misses the other on purpose — a tag leaves no placeholder to find, and a
# removed entry leaves the object count intact.
scratch=$(mktemp -d) || fail "mktemp -d: cannot create the scratch tree"
trap 'rm -rf "$scratch"' EXIT

# kubectl is a native Windows binary under Git Bash, where mktemp answers with a
# POSIX path it cannot open (README, "Running the bootstrap on Windows").
kube_path() {
	if command -v cygpath >/dev/null 2>&1; then
		cygpath -m "$1"
	else
		printf '%s\n' "$1"
	fi
}

# One copy per mutation, with the base at the depth the overlay names:
# overlays/delivery resolves ../../base.
scratch_copy() {
	mkdir -p "$scratch/$1/overlays/delivery" || fail "scratch tree"
	cp -R base "$scratch/$1/base" || fail "scratch base copy"
	cp overlays/delivery/kustomization.yaml "$scratch/$1/overlays/delivery/" ||
		fail "scratch overlay copy"
}

# Defect one: the gateway entry pins a tag instead of a digest. `newTag` is what
# an operator reaches for when pasting a release tag, and the folded value below
# the replaced key has to go with it or the copy would not parse.
scratch_copy tagged
awk '
	/^  - name: firewatch\/gateway$/ { entry = 1; print; next }
	entry == 1 && /^    digest: / { print "    newTag: latest"; entry = 2; next }
	entry == 2 && /^      / { entry = 0; next }
	{ print }
' overlays/delivery/kustomization.yaml \
	>"$scratch/tagged/overlays/delivery/kustomization.yaml" ||
	fail "could not write the tagged-image mutation"

tagged_render=$("$K" kustomize "$(kube_path "$scratch/tagged/overlays/delivery")") ||
	fail "the tagged-image mutation does not render, so it proves nothing"
tagged_stale=$(printf '%s\n' "$tagged_render" | placeholder_images | tr '\n' ' ')
[ -z "$tagged_stale" ] ||
	fail "the tagged-image mutation also produced a placeholder ('$tagged_stale'), so it no longer isolates the digest check"
tagged_untagged=$(printf '%s\n' "$tagged_render" | unpinned_images | tr '\n' ' ')
printf '%s\n' "$tagged_untagged" | grep -q 'firewatch/gateway:latest' ||
	fail "the digest check did not notice an overlay entry turned back into a tag: it reported '$tagged_untagged', want firewatch/gateway:latest"

# Defect two: one service leaves the overlay's `images` list. Nothing is removed
# from the render, so the object count is unchanged and only the placeholder says
# so.
scratch_copy dropped
awk '
	/^  - name: firewatch\/inference$/ { drop = 1; next }
	drop && /^  - name: / { drop = 0 }
	drop { next }
	{ print }
' overlays/delivery/kustomization.yaml \
	>"$scratch/dropped/overlays/delivery/kustomization.yaml" ||
	fail "could not write the dropped-entry mutation"

dropped_render=$("$K" kustomize "$(kube_path "$scratch/dropped/overlays/delivery")") ||
	fail "the dropped-entry mutation does not render, so it proves nothing"
dropped_objects=$(printf '%s\n' "$dropped_render" | object_count)
[ "$dropped_objects" = "$base_objects" ] ||
	fail "the dropped-entry mutation changed the object count ($dropped_objects, base $base_objects), so it no longer isolates the placeholder check"
dropped_stale=$(printf '%s\n' "$dropped_render" | placeholder_images | tr '\n' ' ')
case $dropped_stale in
*"firewatch/inference:replace-before-deploy"*) ;;
*)
	fail "the placeholder check did not notice a service removed from the overlay's images list: it reported '$dropped_stale'"
	;;
esac

# --- no credential lives in a manifest ---------------------------------------
if grep -nEi 'pass(word)?[:=] *[a-z0-9]{6,}|BEGIN ([A-Z]+ )?PRIVATE KEY' \
	base/jobs-migrate.yaml base/broker-provision.yaml >/dev/null 2>&1; then
	fail "a literal credential appears in a manifest"
fi

echo "PASS: render contract holds for the bootstrap Jobs, the application workloads and the delivery overlay"
