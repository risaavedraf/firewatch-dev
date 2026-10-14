# BH-06 — Restrict Kubernetes egress to required destinations

**Status:** pending | **Priority:** P1 | **Dependencies:** target network plugin and IdP routing decision; coordinate BH-05.

## Evidence and scope

`network.yaml:12-23` permits all pods outbound TCP 443 to any IPv4 address. This is broad egress, not proof of SSRF or a successful attack.

Select an environment-specific overlay or controlled egress gateway compatible with the actual network plugin and IdP routing. Do not assume portable hostname filtering in basic NetworkPolicy.

## Acceptance checklist

- [ ] Inventory required destinations and flows, including DNS, IdP/JWKS, broker, database, and internal service traffic as applicable.
- [ ] Choose supported destination controls and document dynamic IdP address handling.
- [ ] Restrict unrelated destinations without breaking required flows or secure transport.
- [ ] Document environment-specific rollout, observability, and rollback.

## Verification — future, not run

**Checks status:** pending. In an explicitly authorized environment, demonstrate required flows succeed and unrelated outbound destinations are denied. Confirm the installed plugin actually enforces the selected controls.

## Execution route and next action

**Proposed route:** delegated for network-policy preparation and coordinated environment overlay checks.

**Next action:** approve the target plugin/routing design and bounded connectivity-test destinations.
