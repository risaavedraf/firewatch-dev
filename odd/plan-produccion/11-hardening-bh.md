# Frente 11 — Hardening backend BH-01…BH-11

- **Grupo de entrega:** G3 — Robustez de backend.
- **Estado:** Sesión A cerrada (2026-10-16, rama `fix/backend-hardening-bh`). BH-01 cerrada; BH-03 y BH-07 implementadas, incluido el replay de un job `queued`; BH-08 re-verificada en línea y BH-09 ratificada por escrito; BH-02 parcial (faltan el inventario y el scan de imágenes, que exigen daemon de Docker); BH-04/05/06/10/11 diferidas por entorno; BH-12 diferida. Detalle, decisiones y rebanadas de revisión en [`README.md`](README.md) §9 y §9.5.
- **Riesgo (Task Size):** alto al implementar — ítems 2 (seguridad), 4 (idempotencia concurrente) y 5 (despliegue/release).
- **Depende de:** BH-01 abre la secuencia; BH-11 depende de BH-02/04/05/06; BH-12 requiere activación explícita y baseline BH-08.
- **Seam / superficie:** toolchain, gateway, broker, transporte, red Kubernetes, consumidores, OpenAPI, autenticación, persistencia e infraestructura de despliegue.

**Objetivo.** Establecer baseline y validar controles de seguridad backend/despliegue con evidencia repetible.

**Documentación real (redirección).** Cada unidad tiene su documento con criterios propios; no se reescriben aquí.
- [`../tasks/security/backend-hardening.md:11-28`](../tasks/security/backend-hardening.md) — alcance y estado de las 12 unidades.
- [`../tasks/security/backend-hardening.md:32-41`](../tasks/security/backend-hardening.md) — secuencia y cierre.
- Unidades: [`01-toolchain-baseline.md`](../tasks/security/backend-hardening/01-toolchain-baseline.md) … [`12-centralized-error-handling.md`](../tasks/security/backend-hardening/12-centralized-error-handling.md).

## Specs

- **S1** — Cada unidad BH-NN se ejecuta y verifica según **su** documento, con evidencia repetible.
- **S2** — El baseline (BH-01) es el punto de entrada; no se marcan unidades dependientes sin él.
- **S3** — BH-12 permanece **diferida** (activación explícita); no se implementa por defecto.

## Tasks (una unidad = una tarea)

| ID | Unidad | Estado | Documento |
| --- | --- | --- | --- |
| T1 | BH-01 — Toolchain y baseline | **Cerrada** (`02911d5`) | [`01-toolchain-baseline.md`](../tasks/security/backend-hardening/01-toolchain-baseline.md) |
| T2 | BH-02 — Vulnerabilidades e imágenes | **Parcial** (`702aff6`) — `govulncheck` limpio en los 4 módulos; falta el inventario y el scan de imágenes con `trivy`, que exige daemon de Docker | [`02-vulnerability-scans.md`](../tasks/security/backend-hardening/02-vulnerability-scans.md) |
| T3 | BH-03 — Cuotas gateway | **Implementada** (`2981dc5`); `-race` pendiente en CI | [`03-gateway-rate-limits.md`](../tasks/security/backend-hardening/03-gateway-rate-limits.md) |
| T4 | BH-04 — Mínimo privilegio del broker | Pendiente — decisión de entorno/accesos | [`04-broker-least-privilege.md`](../tasks/security/backend-hardening/04-broker-least-privilege.md) |
| T5 | BH-05 — Seguridad de transporte | Pendiente — decisión de entorno | [`05-transport-security.md`](../tasks/security/backend-hardening/05-transport-security.md) |
| T6 | BH-06 — Egress Kubernetes | Pendiente — decisión de entorno | [`06-kubernetes-egress.md`](../tasks/security/backend-hardening/06-kubernetes-egress.md) |
| T7 | BH-07 — Recuperación de jobs *(coordina con frente 4)* | **Implementada** (`54c1ce8`, `0b0f784`, `537f83a`, `90f48a6`); verificación end-to-end pendiente por Docker | [`07-job-recovery.md`](../tasks/security/backend-hardening/07-job-recovery.md) |
| T8 | BH-08 — Errores OpenAPI *(frente 10)* | **Implementada** (`750cd80`, `1436dae`); re-verificada en línea el 2026-10-16 — la ruta de verificador independiente fue rechazada por admisión en tiempo de ejecución | [`08-openapi-errors.md`](../tasks/security/backend-hardening/08-openapi-errors.md) |
| T9 | BH-09 — Autenticación negativa | **Implementada** (`03baa09`) y **ratificada por escrito** el 2026-10-16 (D5) | [`09-authentication-tests.md`](../tasks/security/backend-hardening/09-authentication-tests.md) |
| T10 | BH-10 — Idempotencia concurrente | **Diferida** — exige DB y broker aislados con Docker levantado | [`10-concurrent-idempotency.md`](../tasks/security/backend-hardening/10-concurrent-idempotency.md) |
| T11 | BH-11 — Seguridad de despliegue | **Diferida** — depende de BH-02/04/05/06 | [`11-deployment-security-verification.md`](../tasks/security/backend-hardening/11-deployment-security-verification.md) |
| T12 | BH-12 — Errores centralizados | **Diferida** | [`12-centralized-error-handling.md`](../tasks/security/backend-hardening/12-centralized-error-handling.md) |

Ruta por defecto: **una unidad por sesión/escritor** (superficies disjuntas); gate de verificación por unidad que nombre ítem de riesgo alto.

## Dependencias y bloqueos humanos

- **Bloqueo humano:** autorizar alcance/comandos BH-01; resolver decisiones de entorno, políticas y accesos indicadas por cada unidad.
- T7 coordina con el frente 4; T8 con el frente 10.

## Criterios de cierre

- `../tasks/security/backend-hardening.md:32-41`; aceptación/verificación en cada `../tasks/security/backend-hardening/NN-*.md`.

## Log

- **L1 (2026-10-16)** — Creación del desglose por spec a partir de [`README.md`](README.md) §1 (frente 11). Documento de planificación; sin implementación.
- **L2 (2026-10-16)** — Tabla de estado puesta al día contra la realidad de la rama `fix/backend-hardening-bh`: BH-01 cerrada, BH-02 parcial, BH-08 y BH-09 implementadas, BH-03 y BH-07 con política acordada, y las dependientes de entorno diferidas con razón. Se dejó de marcar como «Pendiente» lo que ya tiene commit y evidencia.
- **L3 (2026-10-16, cierre)** — Sesión A cerrada: T8 re-verificada en línea (RED reproducido en un árbol aislado, matriz contra los handlers, validador en 0, y una promesa de más del contrato corregida en su propio commit `cf23d20`), T9 ratificada por escrito, y T7 ampliada con el replay de un job `queued` más su compuerta en `risk` (`90f48a6`). Lo que queda abierto de este frente lo está por **entorno** (Docker para BH-02/10/11 y las decisiones de BH-04/05/06), no por decisiones pendientes. El frente se publica como un PR único sobre `fix/backend-hardening-bh`, revisable por rebanadas: una unidad por commit (BH-01, BH-08, BH-02, BH-09, frentes 5/6, plan, decisiones A2, BH-03, BH-07 u1–u3, BH-07 u4, cierre documental).
