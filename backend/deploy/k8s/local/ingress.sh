#!/usr/bin/env sh
# Installs the ingress controller that base/ingress.yaml names and creates the
# edge certificate Secret that object serves, on a local cluster.
#
# S3 of odd/tasks/k8s-delivery-closure.md is a handshake, not an annotation. The
# only claim about the edge that had been checked was
# `backend-protocol: HTTPS` in base/ingress.yaml, and that is a statement about
# what NGINX would do once a controller exists — not a request that crossed the
# edge. This recipe supplies the two objects that make the crossing possible: a
# controller of the class the Ingress names, and the `firewatch-tls` Secret its
# TLS block references. The Ingress itself is applied with the rest of the
# overlay; this script does not touch it.
#
# The controller is pinned, never `latest`: an unpinned addon gives the
# handshake a different subject on every rebuild, and the version is what the
# observations in this directory were taken against. It is installed from that
# release's own kind-flavoured static manifest, whose controller pod claims the
# node ports the cluster maps — host 8080 to node 80, host 8443 to node 443 — so
# the edge is reachable from the operator's machine, not only from inside the
# cluster.
#
# Usage: ./ingress.sh
#        ./ingress.sh --help
set -eu

prog=ingress

# Pinned, never `latest`. `controller-v1.15.1` is the release the verification
# cluster ran; the image tag is the same version without the `controller-`
# prefix, which is why it is derived instead of written twice.
release=controller-v1.15.1
image_tag=${release#controller-}
manifest="https://raw.githubusercontent.com/kubernetes/ingress-nginx/$release/deploy/static/provider/kind/deploy.yaml"

namespace=ingress-nginx
deployment=ingress-nginx-controller
ingress_class=nginx
# The two Jobs in that manifest generate the admission webhook's certificate.
# They run once; a re-run of this script must not wait on objects it does not
# create, so they are waited on only while they exist.
admission_jobs="ingress-nginx-admission-create ingress-nginx-admission-patch"

# How long the rollout and the admission Jobs get before the recipe reports
# what stayed true instead of a bare exit code.
wait_seconds=180

# The edge certificate lives long enough that a rebuild does not fail the
# handshake on an expiry nobody watched for. Rotating it is deleting the Secret
# and re-running this script; the old one is never overwritten in place.
edge_days=825

usage() {
	cat <<EOF
Usage: ./ingress.sh

Install ingress-nginx $release on this cluster (namespace $namespace,
IngressClass $ingress_class) and create the edge certificate Secret that
base/ingress.yaml references.

The script:

  1. fails unless a node carries the label the pinned manifest selects on
     (ingress-ready=true), because that pod binds the node ports the cluster
     maps to the host and a node without the label leaves it Pending;
  2. applies $release's own kind static manifest, unless that exact controller
     image is already the one running;
  3. waits for the admission Jobs and the controller rollout, then fails unless
     IngressClass $ingress_class exists;
  4. creates the Secret base/ingress.yaml names, with a certificate for the host
     that file declares, generated here at run time.

The edge key pair is generated on this machine and never printed: only its path
inside a private temporary directory is written, and the directory is removed on
exit. The certificate is self-signed because the host it names is under the
reserved .invalid TLD (RFC 2606) and no CA can certify it, so nothing but the
Secret the Ingress serves depends on it.

No cluster is modified beyond $namespace and the objects that release declares,
plus the one Secret in the namespace base/namespace.yaml names. Re-running is
safe: an existing Secret is left untouched.
EOF
}

case "${1:-}" in
--help | -h)
	usage
	exit 0
	;;
esac

[ "$#" -eq 0 ] || {
	printf '%s: this script takes no arguments\n' "$prog" >&2
	usage >&2
	exit 1
}

command -v kubectl >/dev/null 2>&1 || {
	printf '%s: kubectl is not on PATH, and this recipe drives a cluster with\n' \
		"$prog" >&2
	printf '%s: it; put kubectl on PATH, point KUBECONFIG at the cluster and\n' \
		"$prog" >&2
	printf '%s: run it again\n' "$prog" >&2
	exit 1
}

command -v openssl >/dev/null 2>&1 || {
	printf '%s: openssl is not on PATH, and the edge certificate is generated\n' \
		"$prog" >&2
	printf '%s: here rather than committed; install it and run this again\n' \
		"$prog" >&2
	exit 1
}

# A native Windows openssl under Git Bash cannot open the POSIX path `mktemp`
# prints (README, "Running the bootstrap on Windows"), so paths handed to it are
# converted when cygpath is there. On a POSIX host this is the identity.
host_path() {
	if command -v cygpath >/dev/null 2>&1; then
		cygpath -m "$1"
	else
		printf '%s\n' "$1"
	fi
}

# --- what the manifests say this recipe has to provide ------------------------
#
# The host and the Secret name are read from base/ingress.yaml instead of
# repeated here: the certificate has to cover the name the Ingress routes, and a
# second copy of that name is exactly the drift this recipe would hide. The
# namespace comes from base/namespace.yaml for the same reason — it is the
# namespace the Ingress is admitted into, and the one its proxy-ssl-secret
# annotation has to name.
repo_root="$(cd "$(dirname "$0")/../../.." && pwd)"
ingress_file="$repo_root/deploy/k8s/base/ingress.yaml"
namespace_file="$repo_root/deploy/k8s/base/namespace.yaml"
for f in "$ingress_file" "$namespace_file"; do
	[ -f "$f" ] || {
		printf '%s: missing %s\n' "$prog" "$f" >&2
		exit 1
	}
done

edge_secret="$(awk '/^ *secretName: /{print $2; exit}' "$ingress_file")"
[ -n "$edge_secret" ] || {
	printf '%s: %s declares no tls secretName, so there is no Secret to\n' \
		"$prog" "$ingress_file" >&2
	printf '%s: create: the Ingress would serve the controller default\n' "$prog" >&2
	exit 1
}

edge_host="$(awk '/^ *- hosts: \[/{sub(/.*\[/, ""); sub(/\].*/, ""); print; exit}' "$ingress_file")"
[ -n "$edge_host" ] || {
	printf '%s: %s declares no tls hosts entry, so the certificate to issue\n' \
		"$prog" "$ingress_file" >&2
	printf '%s: is not named there and this recipe would guess\n' "$prog" >&2
	exit 1
}

app_namespace="$(awk '/^metadata:/{m=1;next} m && /^  name:/{print $2; exit}' "$namespace_file")"
[ -n "$app_namespace" ] || {
	printf '%s: %s declares no metadata.name\n' "$prog" "$namespace_file" >&2
	exit 1
}

# --- the context this recipe is for ------------------------------------------
#
# The controller claims the node's ports 80 and 443. On a shared cluster that is
# someone else's traffic, so the context is named before anything changes when
# it does not look like a local one.
context=$(kubectl config current-context 2>/dev/null || true)
case "$context" in
kind-*) ;;
*)
	printf '%s: warning: context "%s" is not kind-*\n' \
		"$prog" "${context:-<none>}" >&2
	printf '%s: the pinned manifest is the kind flavour: it binds the node\n' \
		"$prog" >&2
	printf '%s: ports 80 and 443 through hostPort and selects on\n' "$prog" >&2
	printf '%s: ingress-ready=true; stop here if this cluster is shared\n' \
		"$prog" >&2
	;;
esac

# --- 1. the node label the pinned manifest selects on ------------------------
#
# The kind flavour of the release is a pod with hostPort 80 and 443 and
# `nodeSelector: ingress-ready: "true"`. A cluster created without that label
# renders, applies, and leaves the controller Pending with no error of its own —
# the port mapping is there and nothing claims it. So the label is asserted
# before the apply, and the message names the fix instead of the symptom.
node=$(kubectl get nodes -l ingress-ready=true -o name 2>/dev/null | head -1) || node=
if [ -z "$node" ]; then
	printf '%s: no node carries ingress-ready=true, which is the label the\n' \
		"$prog" >&2
	printf '%s: kind manifest selects on, so the controller would stay\n' "$prog" >&2
	printf '%s: Pending and the host ports would stay unanswered\n' "$prog" >&2
	printf '%s: label the node and re-run:\n' "$prog" >&2
	printf '  kubectl label node <node> ingress-ready=true\n' >&2
	exit 1
fi

# --- 2. is this addon already the one the recipe installs? -------------------
#
# Applying the manifest a second time is not free: the Deployment spec is
# re-asserted and any pod that differs rolls. A recipe that restarts a healthy
# controller on every run cannot be used to reproduce an observation twice. The
# release is therefore applied only when the pinned image is not what is
# running or the IngressClass is missing; the assertions after the block are the
# check either way.
existing_image=
if kubectl -n "$namespace" get deploy "$deployment" >/dev/null 2>&1; then
	existing_image=$(kubectl -n "$namespace" get deploy "$deployment" \
		-o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null || true)
fi

at_pin=0
case "$existing_image" in
*"/controller:$image_tag@"*) at_pin=1 ;;
esac

if [ "$at_pin" -eq 1 ] && kubectl get ingressclass "$ingress_class" >/dev/null 2>&1; then
	printf '%s: %s/%s is already ingress-nginx %s\n' \
		"$prog" "$namespace" "$deployment" "$image_tag"
	printf '%s: nothing to apply; the assertions below are the check\n' "$prog"
else
	printf '%s: applying ingress-nginx %s from the pinned kind manifest\n' \
		"$prog" "$image_tag"
	printf '%s: %s\n' "$prog" "$manifest"
	kubectl apply -f "$manifest" || {
		printf '%s: could not apply %s\n' "$prog" "$manifest" >&2
		printf '%s: the cluster is unchanged beyond what that apply reported\n' \
			"$prog" >&2
		exit 1
	}
fi

# The pin is asserted, not assumed: if another installation owns this addon, the
# behaviour this recipe documents is not the behaviour of the controller that is
# about to be waited on.
image=$(kubectl -n "$namespace" get deploy "$deployment" \
	-o jsonpath='{.spec.template.spec.containers[0].image}') || {
	printf '%s: could not read %s/%s\n' "$prog" "$namespace" "$deployment" >&2
	exit 1
}
case "$image" in
*"/controller:$image_tag@"*) ;;
*)
	printf '%s: %s/%s runs "%s" but this recipe pins\n' \
		"$prog" "$namespace" "$deployment" "$image" >&2
	printf '%s: controller:%s, so another installation owns the addon\n' \
		"$prog" "$image_tag" >&2
	printf '%s: and the version the edge observations were taken against is\n' \
		"$prog" >&2
	printf '%s: not the one running\n' "$prog" >&2
	exit 1
	;;
esac

# --- 3. the admission Jobs and the rollout -----------------------------------
#
# The manifest's ValidatingWebhookConfiguration has failurePolicy Fail, so an
# Ingress applied before the webhook's certificate exists is rejected. The two
# Jobs are waited on first, and only while they are there: a re-run against a
# cluster where they have already completed must not fail on a missing Job.
for job in $admission_jobs; do
	kubectl -n "$namespace" get job "$job" >/dev/null 2>&1 || continue
	printf '%s: waiting for %s/%s to complete\n' "$prog" "$namespace" "$job"
	kubectl -n "$namespace" wait --for=condition=complete "job/$job" \
		--timeout="${wait_seconds}s" || {
		printf '%s: %s/%s did not complete within %ss; the admission\n' \
			"$prog" "$namespace" "$job" "$wait_seconds" >&2
		printf '%s: webhook has no certificate, and the Ingress object cannot\n' \
			"$prog" >&2
		printf '%s: be applied while failurePolicy is Fail\n' "$prog" >&2
		exit 1
	}
done

printf '%s: waiting for %s/%s to roll out\n' "$prog" "$namespace" "$deployment"
kubectl -n "$namespace" rollout status deploy/"$deployment" \
	--timeout="${wait_seconds}s" || {
	printf '%s: the %s rollout did not complete within %ss, so nothing\n' \
		"$prog" "$deployment" "$wait_seconds" >&2
	printf '%s: answers on the node ports the edge is reached through\n' \
		"$prog" >&2
	printf '%s: inspect it with: kubectl -n %s logs deploy/%s\n' \
		"$prog" "$namespace" "$deployment" >&2
	exit 1
}

# The Ingress in base/ names its class by name, not by a controller-class
# annotation: without this object the Ingress is admitted, is assigned no
# controller, and never serves a byte.
kubectl get ingressclass "$ingress_class" >/dev/null 2>&1 || {
	printf '%s: IngressClass %s does not exist, so the Ingress in base/\n' \
		"$prog" "$ingress_class" >&2
	printf '%s: (ingressClassName: %s) would be served by nothing\n' \
		"$prog" "$ingress_class" >&2
	exit 1
}
printf '%s: IngressClass %s is present\n' "$prog" "$ingress_class"

# --- 4. the edge certificate Secret ------------------------------------------
#
# base/ingress.yaml serves TLS from a Secret the operator supplies, and this is
# that Secret for a local cluster. The certificate is generated at run time and
# never committed: a private key in the repository is a key every clone shares,
# and it would still not be trusted by anything outside this machine.
#
# The host is `firewatch.example.invalid`, so the certificate is self-signed.
# `.invalid` is reserved by RFC 2606 and no CA will certify it, which is the
# point: the name exists for a local replica of the edge, and `curl --resolve`
# points it at the cluster without a DNS entry anywhere.
if kubectl -n "$app_namespace" get secret "$edge_secret" >/dev/null 2>&1; then
	printf '%s: secret/%s already exists in %s, left untouched\n' \
		"$prog" "$edge_secret" "$app_namespace"
else
	workdir=$(mktemp -d) || {
		printf '%s: mktemp -d failed, so the key pair has no private home\n' \
			"$prog" >&2
		exit 1
	}
	# The temporary directory holds the only copy of the new private key, and
	# trailing output is the last place it should appear.
	trap 'rm -rf "$workdir"' EXIT

	# `-addext` needs OpenSSL 1.1.1 or newer; the alternative, an extension file
	# plus `-extensions`, is what bootstrap-secrets.sh uses for its CSR because
	# it signs with `openssl x509 -req` and not `req -x509`.
	MSYS_NO_PATHCONV=1 openssl req -x509 -newkey rsa:2048 -nodes -days "$edge_days" \
		-keyout "$(host_path "$workdir/tls.key")" \
		-out "$(host_path "$workdir/tls.crt")" \
		-subj "/CN=$edge_host" \
		-addext "subjectAltName=DNS:$edge_host" \
		-addext "extendedKeyUsage=serverAuth" >/dev/null 2>&1 || {
		printf '%s: could not generate a certificate for %s (openssl 1.1.1\n' \
			"$prog" "$edge_host" >&2
		printf '%s: or newer is required for -addext)\n' "$prog" >&2
		exit 1
	}

	kubectl -n "$app_namespace" create secret tls "$edge_secret" \
		--cert="$(host_path "$workdir/tls.crt")" \
		--key="$(host_path "$workdir/tls.key")" || {
		printf '%s: could not create secret/%s in %s; the Ingress would\n' \
			"$prog" "$edge_secret" "$app_namespace" >&2
		printf '%s: serve the controller default certificate instead\n' "$prog" >&2
		exit 1
	}
	printf '%s: created secret/%s in %s for %s\n' \
		"$prog" "$edge_secret" "$app_namespace" "$edge_host"
fi

# The Secret that exists is not necessarily the one this recipe would create:
# an operator-supplied certificate for the public name is a valid Secret here.
# What is not valid is a certificate that does not cover the host the Ingress
# routes, and that fails as a browser warning rather than as an error anywhere.
# Only `tls.crt` is read — it is public — and its subject alternative names are
# matched without printing the certificate.
kubectl -n "$app_namespace" get secret "$edge_secret" -o jsonpath='{.data.tls\.crt}' |
	base64 --decode |
	openssl x509 -noout -text 2>/dev/null | grep -q "DNS:$edge_host" || {
	printf '%s: secret/%s in %s does not carry a certificate for %s, so\n' \
		"$prog" "$edge_secret" "$app_namespace" "$edge_host" >&2
	printf '%s: the edge would answer with a name mismatch\n' "$prog" >&2
	exit 1
}
printf '%s: secret/%s covers %s\n' "$prog" "$edge_secret" "$edge_host"

printf '%s\n' "$prog: the edge is installed. The Ingress is applied with the rest" \
	"$prog: of the render, not by this script; deploy/k8s/local/README.md has the" \
	"$prog: order and the expected output."
kubectl -n "$namespace" get pods -l app.kubernetes.io/name=ingress-nginx \
	--no-headers 2>/dev/null || true
printf '%s: reproduce the handshake with (host port 8443 maps to node 443):\n' "$prog"
printf '  curl -vk --resolve %s:8443:127.0.0.1 https://%s:8443/v1/jobs\n' \
	"$edge_host" "$edge_host"
