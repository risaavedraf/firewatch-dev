#!/usr/bin/env sh
# Installs the resource-metrics API the HPAs in base/ read, on a local cluster.
#
# `base/autoscaling.yaml` scales on measured CPU utilisation, and that number is
# not the API server's: it comes from the aggregated `metrics.k8s.io` API that
# metrics-server serves. Without it an HPA reports `ScalingActive=False` with
# `unable to fetch metrics from resource metrics API` and never leaves its
# floor, which is why the scale-out of V1-F-06 cannot be observed on a cluster
# with no metrics API. This script installs that API, pinned, so the environment
# behind the observation can be rebuilt instead of remembered.
#
# It mutates nothing but its own addon: the objects the pinned release declares,
# in `kube-system`, and the one flag that release needs on kind. It is
# idempotent in the sense that matters: a second run against a healthy addon
# applies nothing, restarts no pod and only asserts. That needs an explicit
# check, because the release's own manifest lists its args without the flag, so
# an unconditional apply would strip the flag and restart the addon twice per
# run; the check is at the top of the body.
#
# Usage: ./metrics-server.sh
#        ./metrics-server.sh --help
set -eu

prog=metrics-server

# Pinned, never `latest`. An unpinned addon gives the scale-out observation a
# different subject on every rebuild: a later release can change what the
# APIService serves and how fast its first scrape lands. v0.9.1 is the release
# the verification cluster runs.
release=v0.9.1
release_base=https://github.com/kubernetes-sigs/metrics-server/releases/download
components="$release_base/$release/components.yaml"

namespace=kube-system
deployment=metrics-server
apiservice=v1beta1.metrics.k8s.io
insecure_flag=--kubelet-insecure-tls

# The APIService needs a few seconds after the rollout before the aggregation
# layer marks it Available, so each assertion below waits instead of reading
# once and failing on a healthy install. Both waits are bounded.
wait_seconds=180
poll_seconds=5

usage() {
	cat <<EOF
Usage: ./metrics-server.sh

Install metrics-server $release into this cluster's $namespace namespace, so the
aggregated resource-metrics API that the HPAs in base/ read exists.

The script:

  1. applies the pinned release's components.yaml, whose URL names $release
     and never latest, unless the pinned Deployment, the flag and the
     APIService are all already there;
  2. adds $insecure_flag to the $deployment Deployment,
     unless that flag is already there;
  3. waits for that Deployment to roll out;
  4. fails unless $apiservice is Available=True, and again
     unless kubectl top nodes answers, each within ${wait_seconds}s.

$insecure_flag is a local-cluster trade, not a production setting:
kind's kubelet certificate is self-signed and the cluster CA did not sign it, so
metrics-server cannot verify it. The comment above the patch says exactly what
the flag trusts and why it must not travel to a shared cluster.

No cluster is modified beyond $namespace/$deployment and the objects that
release declares. Re-running is safe.
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

# --- the context this recipe is for ------------------------------------------
#
# The flag below is a local-cluster trade, so the context is named before
# anything changes when it does not look like one. An apply that silently turns
# off kubelet certificate verification on a shared cluster is worse than a
# recipe that says which cluster it expects.
context=$(kubectl config current-context 2>/dev/null || true)
case "$context" in
kind-*) ;;
*)
	printf '%s: warning: context "%s" is not kind-*\n' \
		"$prog" "${context:-<none>}" >&2
	printf '%s: %s is a local-cluster trade; stop here if this cluster is\n' \
		"$prog" "$insecure_flag" >&2
	printf '%s: shared (the comment above the patch says why)\n' \
		"$prog" >&2
	;;
esac

# --- 1. is this addon already the one the recipe installs? -------------------
#
# The release's manifest carries the release's own args list, and that list does
# not include the flag added below. Applying it is therefore not a no-op on an
# addon that already has the flag: the apply strips the flag, restarts the
# addon, and the patch re-adds it and restarts it a second time. A second run of
# an install script must not churn a healthy addon, so the release is applied
# only when the pinned Deployment, the flag, or the APIService is missing. The
# assertions after this block are what verify the result either way.
existing_image=
existing_args=
if kubectl -n "$namespace" get deploy "$deployment" >/dev/null 2>&1; then
	existing_image=$(kubectl -n "$namespace" get deploy "$deployment" \
		-o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null || true)
	existing_args=$(kubectl -n "$namespace" get deploy "$deployment" \
		-o jsonpath='{.spec.template.spec.containers[0].args}' 2>/dev/null || true)
fi

at_pin=0
case "$existing_image" in
*"/metrics-server:$release") at_pin=1 ;;
esac

flagged=0
case "$existing_args" in
*"$insecure_flag"*) flagged=1 ;;
esac

if [ "$at_pin" -eq 1 ] && [ "$flagged" -eq 1 ] &&
	kubectl get apiservice "$apiservice" >/dev/null 2>&1; then
	printf '%s: %s/%s is already metrics-server %s with %s\n' \
		"$prog" "$namespace" "$deployment" "$release" "$insecure_flag"
	printf '%s: nothing to apply; the assertions below are the check\n' \
		"$prog"
else
	# --- 1a. the pinned release ----------------------------------------------
	#
	# Upstream's quick start reads `latest/download/components.yaml`. That is
	# the one part of it this recipe replaces: the URL above names v0.9.1, so
	# the objects applied here are the ones this recipe was written against.
	printf '%s: applying metrics-server %s from the pinned release\n' \
		"$prog" "$release"
	kubectl apply -f "$components" || {
		printf '%s: could not apply %s\n' "$prog" "$components" >&2
		printf '%s: the cluster is unchanged beyond what that apply reported\n' \
			"$prog" >&2
		exit 1
	}
fi

# --- 2. the one flag kind needs ----------------------------------------------
#
# kind's kubelet serves its read-only port (10250) with a self-signed
# certificate that the cluster CA did not sign. metrics-server verifies that
# certificate against the cluster CA like any client, so the scrape fails with
# `x509: certificate signed by unknown authority`, the addon's own pods keep
# failing their readiness or the APIService never becomes Available, and every
# HPA keeps reporting `unable to fetch metrics from resource metrics API`.
# `--kubelet-insecure-tls` tells metrics-server to skip that verification.
#
# That skip is the trade, and it is a local one: with the flag set, whatever
# answers on a node's 10250 is trusted to report metrics for that node, and a
# wrong or hostile endpoint could feed the autoscaler numbers that never
# describe the workload. A managed or production cluster does not need that
# trade — the kubelet's certificate chains to the cluster CA, or the
# `--kubelet-client-certificate`/`--kubelet-certificate-authority` pair is
# configured — so the flag must not travel there. It lives in this script and in
# no manifest under `base/`, which is where a verification-off switch belongs:
# in a local recipe, not in a template that every environment renders.
#
# The patch appends to the live args list rather than rewriting it, so the
# release's own flags stay exactly as its manifest declares them.
case "$flagged" in
1)
	# Already there, and appending it again would pass the flag to
	# metrics-server twice. The line above is what reported this state.
	;;
*)
	printf '%s: adding %s to %s/%s\n' \
		"$prog" "$insecure_flag" "$namespace" "$deployment"
	args_path=/spec/template/spec/containers/0/args
	patch=$(printf '[{"op":"add","path":"%s/-","value":"%s"}]' \
		"$args_path" "$insecure_flag")
	kubectl -n "$namespace" patch deploy "$deployment" --type=json \
		-p "$patch" || {
		printf '%s: could not add %s: the addon is installed but cannot\n' \
			"$prog" "$insecure_flag" >&2
		printf '%s: scrape this kubelet, so %s will not\n' \
			"$prog" "$apiservice" >&2
		printf '%s: become Available\n' "$prog" >&2
		exit 1
	}
	;;
esac

# The pin is asserted, not assumed: if something else already owns this addon,
# the behaviour this recipe describes is not the behaviour of the deployment
# that is about to be polled.
image=$(kubectl -n "$namespace" get deploy "$deployment" \
	-o jsonpath='{.spec.template.spec.containers[0].image}') || {
	printf '%s: could not read %s/%s\n' \
		"$prog" "$namespace" "$deployment" >&2
	exit 1
}
case "$image" in
*"/metrics-server:$release") ;;
*)
	printf '%s: %s/%s runs "%s" but this recipe pins\n' \
		"$prog" "$namespace" "$deployment" "$image" >&2
	printf '%s: metrics-server:%s, so another installation owns the addon\n' \
		"$prog" "$release" >&2
	printf '%s: and the version this recipe was checked against is not the\n' \
		"$prog" >&2
	printf '%s: one running\n' "$prog" >&2
	exit 1
	;;
esac

# --- 3. the rollout ----------------------------------------------------------
printf '%s: waiting for %s/%s to roll out\n' "$prog" "$namespace" "$deployment"
kubectl -n "$namespace" rollout status deploy/"$deployment" \
	--timeout="${wait_seconds}s" || {
	printf '%s: the %s rollout did not complete within %ss, so the addon\n' \
		"$prog" "$deployment" "$wait_seconds" >&2
	printf '%s: is not serving and nothing downstream can be Available\n' \
		"$prog" >&2
	printf '%s: inspect it with: kubectl -n %s logs deploy/%s\n' \
		"$prog" "$namespace" "$deployment" >&2
	exit 1
}

# --- 4. the APIService must answer -------------------------------------------
#
# Applying the release also registers the APIService, but the aggregation layer
# marks it Available only once metrics-server answers its discovery calls, which
# is seconds after its pods are Ready. Reading once here fails a healthy
# install, so this waits, and the message it leaves on timeout names the state
# that makes the HPAs unable to scale.
available=
waited=0
while [ "$waited" -lt "$wait_seconds" ]; do
	available=$(kubectl get apiservice "$apiservice" \
		-o jsonpath='{.status.conditions[?(@.type=="Available")].status}' \
		2>/dev/null || true)
	if [ "$available" = True ]; then
		break
	fi
	sleep "$poll_seconds"
	waited=$((waited + poll_seconds))
done

if [ "$available" != True ]; then
	pods=$(kubectl -n "$namespace" get pods -l k8s-app="$deployment" \
		--no-headers 2>/dev/null | tr '\n' ' ' || true)
	printf '%s: %s is not Available=True after %ss (last status "%s")\n' \
		"$prog" "$apiservice" "$wait_seconds" "${available:-<unset>}" >&2
	printf '%s: the resource-metrics API does not answer, so every HPA\n' \
		"$prog" >&2
	printf '%s: still reports "unable to fetch metrics from resource metrics\n' \
		"$prog" >&2
	printf '%s: API" and the scale-out cannot be observed. %s pods: %s\n' \
		"$prog" "$deployment" "${pods:-none found}" >&2
	printf '%s: inspect it with: kubectl -n %s logs deploy/%s\n' \
		"$prog" "$namespace" "$deployment" >&2
	exit 1
fi

printf '%s: %s is Available=True\n' "$prog" "$apiservice"

# --- 5. kubectl top must answer ----------------------------------------------
#
# An Available APIService and arriving numbers are two different states: the pod
# can be Ready while its first scrape has not landed, and `kubectl top` fails
# until it does. That read is the one the HPA controller makes, so it is the one
# this recipe waits on. `kubectl top` can also answer with an `error:` line and
# a zero exit status, so the text is checked as well.
top_ok=0
top_output=
waited=0
while [ "$waited" -lt "$wait_seconds" ]; do
	if top_output=$(kubectl top nodes 2>&1); then
		case "$top_output" in
		error:* | Error*) ;;
		"") ;;
		*) top_ok=1 ;;
		esac
	fi
	if [ "$top_ok" -eq 1 ]; then
		break
	fi
	sleep "$poll_seconds"
	waited=$((waited + poll_seconds))
done

if [ "$top_ok" -ne 1 ]; then
	printf '%s: kubectl top nodes did not answer within %ss\n' \
		"$prog" "$wait_seconds" >&2
	printf '%s: the metrics endpoint is registered but returns no numbers,\n' \
		"$prog" >&2
	printf '%s: so the HPAs cannot compute utilisation and will not scale.\n' \
		"$prog" >&2
	printf '%s: last output: %s\n' "$prog" "${top_output:-<none>}" >&2
	printf '%s: inspect it with: kubectl -n %s logs deploy/%s\n' \
		"$prog" "$namespace" "$deployment" >&2
	exit 1
fi

printf '%s\n' "$top_output"
printf '%s: the resource-metrics API is serving; the HPAs in base/ can read\n' \
	"$prog"
printf '%s: CPU. Watch them with: kubectl get hpa --all-namespaces\n' "$prog"
