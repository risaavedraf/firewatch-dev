# Local cluster: the metrics API the base's HPAs read

`deploy/k8s/base/autoscaling.yaml` scales on measured CPU utilisation, and that measurement is not the API server's. It comes from the aggregated `metrics.k8s.io` API, which exists only when something answers it — `metrics-server`, or a hosted equivalent. Without that API an HPA cannot compute a ratio: it reports `ScalingActive=False`, `reason=FailedGetResourceMetric`, message `failed to get cpu utilization: unable to fetch metrics from resource metrics API`, and it never leaves `minReplicas`. A cluster in that state renders the base, admits every pod, passes the render contract, and still cannot show a single scale-out, so the scaling of `V1-F-06` is not observable there. `metrics-server.sh` in this directory installs the API so that observation is possible, and this file records how to repeat it.

Nothing here is a deployment input. `base/` and `overlays/` do not mention the addon, do not carry its flag, and do not need it: a cluster that already serves `metrics.k8s.io` — every managed one — ignores this directory entirely.

## Install

```sh
sh backend/deploy/k8s/local/metrics-server.sh
```

The script needs `kubectl` and network access to the pinned release URL, nothing else: no `helm`, no `curl`, no `yq`, no temporary files. On Git Bash it therefore sidesteps the path-translation traps of `deploy/k8s/README.md`: those come from `mktemp` and `openssl`, and this script opens neither.

In order, it:

1. applies metrics-server **v0.9.1**, from that release's own `components.yaml`. The version is pinned in the script and never `latest`: an unpinned addon gives the observation a different subject on every rebuild.
2. adds `--kubelet-insecure-tls` to the `metrics-server` Deployment. This is the one flag kind needs and the one trade this recipe makes — see below.
3. waits for the rollout of that Deployment.
4. fails unless the APIService `v1beta1.metrics.k8s.io` reports `Available=True`, and again unless `kubectl top nodes` answers, each within 180 seconds. Each failure names what stayed true, so a broken install reports `ScalingActive`-shaped consequences instead of a bare exit code.

### The one flag, and why it is only for a local cluster

kind's kubelet serves its read-only port (10250) with a self-signed certificate that the cluster CA did not sign. metrics-server verifies that certificate against the cluster CA like any other client, so the scrape fails with `x509: certificate signed by unknown authority`, and the pod's own `/readyz` probe — the one that puts it in the Service's endpoints — never passes. Observed on this cluster while validating this script: with the flag stripped by a plain re-apply of the release, the replacement pod sat at `0/1 Running` for the whole time the roll-out waited on it. No ready endpoint means no metrics, and the HPAs stay exactly where they started. `--kubelet-insecure-tls` tells metrics-server to skip that verification.

That skip is a real trade: with the flag set, whatever answers on a node's 10250 is trusted to report that node's metrics, so a wrong or hostile endpoint could hand the autoscaler numbers that never describe the workload. A managed or production cluster does not need the trade — its kubelet certificate chains to the cluster CA, or a `--kubelet-client-certificate`/`--kubelet-certificate-authority` pair is configured — so the flag must not travel there. It lives in this script and in no manifest under `base/`, which is where a verification-off switch belongs: in a local recipe, not in a template every environment renders. The script warns on stderr when the current context is not `kind-*`.

### Re-running it

The script is idempotent, and idempotent here has a specific meaning: a second run against a healthy addon applies nothing and restarts no pod. That takes an explicit check, because the release's manifest carries the release's own args list and that list does not contain the flag — an unconditional `kubectl apply` would strip the flag, restart the addon, and the patch would add it back and restart it twice per run. So the release is applied only when the pinned Deployment, the flag, or the APIService is missing; otherwise the run only asserts. Measured on the verification cluster: generation stayed at 4, the pod kept its creation timestamp, and `RESTARTS` stayed 0 across the second run.

## Reproduce a scale-out

The four HPAs read live CPU and sit at 2 replicas because a `100m` request with a `1m` reading is `1%`, and 1% is the lowest number this metric can report. The floor is why a target alone never triggers anything: with `averageUtilization: 70` the HPA is `ScalingLimited=True` with `reason=TooFewReplicas`, and with a target of `1` it computes a ratio of exactly 1.0 and still does nothing. Something has to raise the average first.

That something can be real CPU, and a port-forward is the cheapest way to deliver it: it reaches the pod through the API server, so it needs no NetworkPolicy exception and no Service port exposed outside the cluster. The load lands on **one** pod; the HPA reads the average across the pool, which is why a single pressed replica is enough to move the ratio.

```sh
# 1. what the cluster looked like before
kubectl -n firewatch get hpa
kubectl -n firewatch get deploy

# 2. a test target, well below the 1m floor of a 100m request
kubectl -n firewatch patch hpa gateway --type=json \
  -p '[{"op":"replace","path":"/spec/metrics/0/resource/target/averageUtilization","value":1}]'

# 3. bounded load through a port-forward: four concurrent loops, 240s
kubectl -n firewatch port-forward deploy/gateway 18443:8080 &
pf=$!
end=$(( $(date +%s) + 240 ))
for n in 1 2 3 4; do
  ( while [ "$(date +%s)" -lt "$end" ]; do
      curl -sk -o /dev/null https://127.0.0.1:18443/health/ready
    done ) &
done
kill "$pf"   # the port-forward outlives the load; stop it, then the loops
wait

# 4. watch the ratio, the replica count and the reason
kubectl -n firewatch get hpa gateway
kubectl -n firewatch describe hpa gateway | tail -25

# 5. restore the value the base declares, then confirm it
kubectl -n firewatch apply -f backend/deploy/k8s/base/autoscaling.yaml
kubectl -n firewatch get hpa gateway
```

Two details of that recipe are easy to get wrong. The port is **8080**: the pod's listener is the contract's 8080 and the port *name* `https` is what makes it TLS, so `port-forward ... 18443:8443` reaches nothing. And the health path is `/health/ready` (204) or `/health/live` (204): the gateway registers those two and answers a bare `/health` with `404`.

**Lowering an HPA target is a test technique, not a change to the policy.** It belongs in a `kubectl patch` against a live object for the length of the experiment, never in `base/autoscaling.yaml`, whose `70` is the number this project's scaling decision is written against. Step 5 is not optional: it re-asserts the declared value from the file that declares it, and the HPA then behaves as `base/` says it does.

Restore with `-f base/autoscaling.yaml` and not with `-k overlays/delivery`, even though the overlay is what was deployed. The overlay also re-points all five Deployment images, and the committed overlay names no registry host, so applying it rolls every application pod onto an unpullable image and turns an HPA experiment into an outage. When only the HPA changed, apply only the file that declares it.

## What the run observed

Cluster `kind-firewatch-verify`, 2026-11-10, metrics-server v0.9.1 with `Available=True`. Before the experiment every HPA read `cpu: 1%/70%` at 2 replicas. After the target was lowered to 1 and the load started:

| t | `get hpa gateway` | pods (`kubectl top pods -l app=gateway`) |
|---|---|---|
| +0s | `cpu: 1%/1%`, 2 replicas | `1m`, `1m` |
| +21s | `cpu: 1%/1%`, 2 replicas | `86m` on the forwarded pod, `1m` on the other |
| +32s | `cpu: 43%/1%`, 2 replicas | unchanged |
| +63s | `cpu: 53%/1%`, 2 replicas | `110m`, `1m` |
| +74s | `cpu: 55%/1%`, HPA still reporting 2 | six pods listed by +95s (`9m`, `10m`, `9m` on the new ones) |

The HPA wrote `spec.replicas: 6` at +69s — the `SuccessfulRescale` below — which is 60 seconds after the ratio crossed the target: that is the base's `scaleUp.stabilizationWindowSeconds: 60` doing its job, not a slow controller. `kubectl get deploy gateway` already read `6/6` at +74s, while the `REPLICAS` column of `kubectl get hpa` kept showing `2` until +95s. That column is `status.currentReplicas`, refreshed by the controller on its own 15-second cycle, so the object that made the decision lags the Deployment it has already resized; read the Deployment when the question is whether a scale-out happened. Six is `maxReplicas`, so the HPA then reported `ScalingLimited=True` with `reason=TooManyReplicas` instead of settling on a computed number, and utilisation fell from `55%` to `18%` as the load stayed on the one forwarded pod. The event that explains the jump, from `kubectl describe hpa gateway` and `kubectl get events`:

```text
Normal  SuccessfulRescale  New size: 6; reason: cpu resource utilization (percentage of request) above target
Normal  ScalingReplicaSet deployment/gateway Scaled up replica set gateway-8654f475d9 from 2 to 6
```

and the conditions at that moment:

```text
AbleToScale    True  ScaleUpStabilized
ScalingActive  True  ValidMetricFound   the HPA was able to successfully calculate a replica count from cpu resource utilization
ScalingLimited True  TooManyReplicas
```

`ScalingActive=True` with `reason=ValidMetricFound` is the half `V1-F-06` was missing: the HPA computed a replica count from a real measurement instead of staying at its floor. `ScalingLimited=True` is not a failure here — it is what a pool pinned at `maxReplicas` reports, the same way the earlier `TooFewReplicas` was a pool pinned at `minReplicas`.

The return trip is evidence too, because step 5 is what makes the observation temporary: at `00:45:42Z`, six minutes and fifteen seconds after the scale-up, the same object logged `Normal SuccessfulRescale New size: 2; reason: All metrics below target`, and `kubectl get deploy gateway` read `2/2` again. That gap is the base's `scaleDown.stabilizationWindowSeconds: 300` counted from the moment the recommendation fell below the pool's size — the load was still running for the first minute of it — plus the controller's own 15-second cycle. Neither window was touched: only the target was, and it read `70` again at the end.

One caveat about reading the events of a young HPA. The same object still carries `FailedGetResourceMetric` warnings from its first two minutes, whose messages are `did not receive metrics for targeted pods (pods might be unready)` and `no metrics returned from resource metrics API`. Those are warm-up, not a missing addon: the APIService answered those calls and simply had no sample for pods that had just started. A cluster with no metrics API at all answers with `unable to fetch metrics from resource metrics API` instead, which is a different sentence and a different fault. The two are not interchangeable evidence.
