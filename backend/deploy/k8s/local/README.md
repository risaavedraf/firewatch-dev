# Local cluster: the addons the base depends on

Two things `base/` assumes and does not install are built here, one section each:
the aggregated metrics API the HPAs read, below, and the ingress controller plus
the edge certificate the Ingress serves, under `## The edge`. Both are local
recipes: nothing under `base/` or `overlays/` mentions either, and a cluster that
already runs a controller and serves `metrics.k8s.io` ignores this directory.

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

## The edge: the controller and the certificate the Ingress needs

`base/ingress.yaml` is one Ingress object: it routes `firewatch.example.invalid` to the `gateway` Service and declares the two things that make the hop behind it TLS — `backend-protocol: HTTPS` for the encryption and `proxy-ssl-verify` / `proxy-ssl-name` / `proxy-ssl-secret` for the authentication. Neither dependency is in the base: a controller has to exist for the class the Ingress names, and the certificate Secret has to exist for its `tls` block. `ingress.sh` supplies both on a local cluster.

### Install

```sh
sh backend/deploy/k8s/local/ingress.sh
```

It applies ingress-nginx **controller-v1.15.1**, from that release's own kind-flavoured static manifest (`deploy/static/provider/kind/deploy.yaml`), pinned in the script and never `latest`: an unpinned addon gives the handshake a different subject on every rebuild. It then waits for the two admission Jobs and the controller rollout, fails unless `IngressClass nginx` exists, and creates the Secret `base/ingress.yaml` names — `firewatch-tls` — for the host that file declares. Host and Secret name are read from the manifest rather than repeated in the script, so a change there is a change here.

The kind flavour is why the recipe is this one: its controller pod binds `hostPort` 80 and 443 and selects on the node label `ingress-ready=true`. Those are the node ports the cluster maps to the host (`8080` to `80`, `8443` to `443`), which is what makes the edge reachable from the operator's machine instead of only from inside. A cluster created without that label is not something the script fixes quietly: it fails naming the label, because otherwise the controller sits `Pending` and nothing but an unanswered host port would say why.

The edge key pair is generated on this machine at run time and never committed, because a private key in the repository is a key every clone shares. It is self-signed: the host is under the reserved `.invalid` TLD (RFC 2606), no CA will certify it, and `curl --resolve` points the name at the cluster without a DNS entry anywhere. The key is written only inside a `mktemp -d` directory that is removed on exit, and the script prints the Secret's name and the host it covers, never the material. Re-running is safe in the same sense as `bootstrap-secrets.sh`: an existing Secret is left untouched, so rotating the edge certificate means deleting the Secret and running the script again. It also asserts that whatever certificate the Secret holds covers the host the Ingress routes — an operator-supplied certificate for a different name renders, applies, and then fails in the browser.

Two Git Bash details apply here as in `bootstrap-secrets.sh`, and the script handles both instead of documenting them: a native `openssl` cannot open the POSIX path `mktemp` prints, so paths handed to it go through `cygpath -m` when it is there, and MSYS path translation would mangle the `-subj "/CN=..."` argument, so the `openssl` call runs with `MSYS_NO_PATHCONV=1`.

### Reproduce the handshake

The Ingress is applied with the rest of the render, not by this script — the controller has to exist first, so this is the order:

```sh
sh backend/deploy/k8s/local/ingress.sh
kubectl -n firewatch apply -f backend/deploy/k8s/base/ingress.yaml

# the edge, with TLS details
curl -vk --resolve firewatch.example.invalid:8443:127.0.0.1 \
  https://firewatch.example.invalid:8443/v1/jobs
echo | openssl s_client -connect 127.0.0.1:8443 \
  -servername firewatch.example.invalid 2>/dev/null |
  openssl x509 -noout -subject -issuer -ext subjectAltName
```

A `401` is the passing outcome, and the body says whose it is:

```text
HTTP/1.1 401 Unauthorized
Content-Type: application/json
{"code":"unauthorized","request_id":"f9d1c4cc-dc91-4b24-a115-51f0916670e8"}
```

Nothing answered that before the gateway's auth middleware ran, so the request crossed both hops; a failed edge, a refused upstream or a plaintext proxy would each have produced something else (no response at the edge, a `502` from NGINX, a reset). The controller's access log names the upstream that answered, and that is the second half of the same claim:

```text
172.24.0.1 - - [11/Nov/2026:01:21:19 +0000] "GET /v1/jobs HTTP/1.1" 401 76 "-" "curl/8.7.1" 100 0.003 [firewatch-gateway-8080] [] 10.244.0.12:8080 76 0.003 401 e275dcf1eaff681fb739f01862b382b2
```

### What each hop does, and what the first observation was missing

The edge terminates TLS with the `firewatch-tls` Secret: `openssl s_client` reports `subject=CN=firewatch.example.invalid`, `issuer=CN=firewatch.example.invalid`, `X509v3 Subject Alternative Name: DNS:firewatch.example.invalid`, `TLSv1.3`, and `Verify return code: 18 (self-signed certificate)`, which is the expected result for a self-signed certificate nothing has been told to trust.

The hop NGINX makes to the gateway is the one S3 left unobserved. Before these annotations the rendered configuration had `proxy_pass https://upstream_balancer` and nothing else about that hop except `proxy_ssl_session_reuse on`: encrypted, and accepting whatever certificate answered on the Service address. With them it reads:

```text
# PEM sha: 2cef5f1b1d144ecd5ffeac28cfa1639477fe01cd
proxy_ssl_trusted_certificate           /etc/ingress-controller/ssl/ca-firewatch-gateway-upstream-ca.pem;
proxy_ssl_verify                        on;
proxy_ssl_verify_depth                  1;
proxy_ssl_name                          gateway;
proxy_ssl_server_name                   off;
```

`proxy_ssl_server_name off` is the template's default and not a defect: `proxy_ssl_name` is what verification compares against the certificate's SANs, and no SNI is needed for a Service address that serves one certificate.

### The Secret key shape `proxy-ssl-secret` reads, as observed

The annotation's documentation describes a Secret with `tls.crt`, `tls.key` and `ca.crt`, and a first reading of S4 expected the internal CA in `tls.crt` with any non-empty `tls.key`. The controller does not work that way, and this was measured by pointing the annotation at candidate Secrets instead of trusting the description. Cluster `kind-firewatch-verify`, controller v1.15.1:

- A Secret with `tls.crt` = the CA and `tls.key` = an unrelated generated key is **rejected**:

  ```text
  W... backend_ssl.go:47] Error obtaining X.509 certificate: unexpected error creating SSL Cert: certificate and private key does not have a matching public key: tls: private key does not match public key
  E... annotations.go:216] "error reading Ingress annotation" err="error obtaining certificate: local SSL certificate firewatch/edge-probe-tls was not found" name="ProxySSL" ingress="firewatch/firewatch"
  E... Warning reason: 'AnnotationParsingFailed' Error parsing annotations: error obtaining certificate: local SSL certificate firewatch/edge-probe-tls was not found
  ```

  The whole `ProxySSL` block is then dropped from the render, so the hop silently goes back to unverified — a misconfigured anchor fails open, not closed. With `tls.crt` set to the CA and a *matching* `tls.key` it would instead be presented as a client certificate, which this hop does not use and the gateway does not ask for.

- A Secret with **`ca.crt` only** is accepted, and the controller writes that file at `/etc/ingress-controller/ssl/ca-<namespace>-<name>.pem` and renders `proxy_ssl_trusted_certificate` from it (the fingerprint of the written file equals the Secret's `ca.crt`). `tls.crt` and `tls.key` are optional and are read as the client pair, not as the anchor.

- A **bare Secret name** in the annotation is not accepted at all: the annotation fails to parse — `AnnotationParsingFailed Error parsing annotations: location denied, reason: invalid format (namespace/name) found in 'gateway-upstream-ca'` — no `proxy_ssl_trusted_certificate` is rendered, and the location answers `503` instead of reaching the gateway. `namespace/name` is the only form that works, which is why the annotation in `base/ingress.yaml` spells the namespace out even though the Ingress lives in it.

So the shape is one key, `ca.crt`, and `bootstrap-secrets.sh` creates the Secret that way. It repackages the CA into a dedicated `gateway-upstream-ca` instead of pointing the annotation at `firewatch-ca`. Recorded honestly: this controller version would also accept `firewatch-ca` as-is — it was tried, and the rendered line was `proxy_ssl_trusted_certificate /etc/ingress-controller/ssl/ca-firewatch-firewatch-ca.pem` — so the copy is a pinning decision, not a workaround for a shape the controller refuses. It keeps the edge's trust anchor separate from the CA every workload mounts, so a later change to `firewatch-ca` (a rotation bundle, a second CA) needs a deliberate decision about the edge rather than silently widening what the Ingress accepts. The cost is in `bootstrap-secrets.sh`'s own rotation rule: replacing the internal CA means deleting and re-creating this Secret too, or verification fails closed when the gateway's certificate changes.

### Enforcement, seen failing

A configured `proxy-ssl-verify: 'on'` that has never rejected anything is not evidence. The anchor was pointed at a CA that did not sign the gateway's certificate (a scratch Secret created for the experiment, deleted afterwards), the annotation patched, and the same request repeated:

```text
http_code=502
2026/11/11 01:21:47 [error] 2087#2087: *30902 upstream SSL certificate verify error: (21:unable to verify the first certificate) while SSL handshaking to upstream, client: 172.24.0.1, server: firewatch.example.invalid, request: "GET /v1/jobs HTTP/1.1", upstream: "https://10.244.0.12:8080/v1/jobs", host: "firewatch.example.invalid:8443"
```

Then the Ingress was re-applied from `base/ingress.yaml`, the scratch Secret deleted, and the same request answered `401` again with `{"code":"unauthorized","request_id":"de6a4226-d593-4573-b265-3b21e3a5abb8"}`, the annotation read back as `firewatch/gateway-upstream-ca`, and the rendered line back at `ca-firewatch-gateway-upstream-ca.pem`. The 502 in the middle is the difference between encryption and authentication: with the wrong anchor the handshake completes and verification refuses it, which is precisely the state the earlier configuration could not reach.

### What these observations do not cover

- Only the pair `gateway` to ingress is exercised, on `/v1/jobs`: the one path an unauthenticated request reaches the gateway's own auth middleware through. No authenticated request, no other path and no browser client was tested.
- Nothing here measures availability or failover: one node, one controller replica, one gateway pool. The controller's own high-availability configuration is not part of this recipe.
- The gateway presents no client certificate on this hop, because its listener requires none (no `ClientAuth` in the service's TLS configuration). If that changes, the Secret needs the matching `tls.crt` / `tls.key` pair the controller reads, and the shape note above is where the next person should start.
- The controller's admission webhook means an Ingress applied while the webhook is unavailable is rejected; that is the addon's behaviour, not something these manifests control.
