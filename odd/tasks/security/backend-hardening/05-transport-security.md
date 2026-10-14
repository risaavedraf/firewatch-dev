# BH-05 — Enforce secure internal and data-service transport

**Status:** pending | **Priority:** P1 | **Dependencies:** BH-01; transport/environment decision; coordinate BH-04 and BH-06.

## Evidence and scope

Kubernetes `config.yaml:10-11` declares HTTP upstreams. The reviewed jobs `main.go` validates OIDC settings but does not fully enforce data-service TLS at startup. These static observations do not establish that deployed secrets or transport are insecure.

Treat gateway-to-service transport, RabbitMQ TLS, and PostgreSQL TLS as one coherent enforcement task. Select HTTPS or verified mTLS for internal traffic and verified TLS for data services, with explicit trust and service-identity checks.

## Acceptance checklist

- [ ] Document transport paths, termination points, trust sources, and certificate/identity verification ownership.
- [ ] Reject insecure non-development configurations at startup or the applicable deployment enforcement boundary.
- [ ] Define an explicit development-only exception that cannot silently enable production plaintext.
- [ ] Align application settings and deployment configuration without logging secrets or weakening certificate checks.

## Verification — future, not run

**Checks status:** pending. Test secure connections, plaintext rejection, untrusted/expired certificates, identity mismatch, and development exception boundaries for each transport. Validate any mesh-enforced path rather than assuming it is protected.

## Execution route and next action

**Proposed route:** delegated for transport design preparation and coordinated application/deployment tests and changes.

**Next action:** choose the transport enforcement design and authorized trust-provisioning workflow.
