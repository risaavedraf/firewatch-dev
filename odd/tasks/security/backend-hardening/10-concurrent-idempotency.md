# BH-10 — Prove idempotency under concurrent and reordered work

**Status:** pending | **Priority:** P2 | **Dependencies:** BH-01; isolated test databases/broker resources; coordinate BH-07.

## Evidence and scope

The reviewed repository tests are sequential. They do not prove concurrency safety. Earlier application-database tests emitted events, so this task must not reuse the application database or production-connected event resources.

Test simultaneous requests and event processing at the persistence boundary, including unique constraints and atomic rollback. Do not assume an existing defect from the coverage gap.

## Acceptance checklist

- [ ] Require isolated test databases and event infrastructure with explicit safeguards against application targets.
- [ ] Verify simultaneous same-actor/same-key requests produce the agreed single logical result/effect.
- [ ] Verify distinct actors and keys remain independent under concurrency.
- [ ] Cover duplicate/reordered events, uniqueness conflicts, and rollback with no partial state or duplicate effects.

## Verification — future, not run

**Checks status:** pending. Run synchronized concurrency scenarios and controlled transaction failures against isolated infrastructure. Assert persisted records and emitted effects, not only response codes; capture repeatability and cleanup evidence.

## Execution route and next action

**Proposed route:** delegated for isolated fixture preparation and coordinated repository/event tests.

**Next action:** approve isolated resource provisioning and expected idempotency semantics before any database test.
