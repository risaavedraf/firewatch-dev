# BH-04 — Separate broker identities and enforce route permissions

**Status:** pending | **Priority:** P1 | **Dependencies:** authorized target environment and service-to-route permission inventory; coordinate BH-05.

## Evidence and scope

`docker-compose.yml:108,119` shows a shared broker identity. Abuse of publication permissions requires broker publisher access; the review does not establish external access or deployed credential exposure.

Define per-service identities and least-privilege configure/write/read permissions for required exchanges, queues, bindings, and routes. Validate the effective policy in the intended broker environment.

## Acceptance checklist

- [ ] Document each service's required operations and deny unrelated publications/consumption.
- [ ] Separate identities and provision/rotate credentials through an approved secret mechanism; never commit credentials.
- [ ] Validate route-level permission behavior and any broker-specific enforcement limitations.
- [ ] Record sanitized allow/deny evidence and a safe rollout/rollback procedure.

## Verification — future, not run

**Checks status:** pending. With explicitly authorized broker access, exercise required operations and forbidden cross-service routes using each identity. Keep credentials and payload secrets out of evidence.

## Execution route and next action

**Proposed route:** delegated for permission-matrix preparation and coordinated deployment configuration work.

**Next action:** approve the service permission matrix and target-environment operations before provisioning identities.
