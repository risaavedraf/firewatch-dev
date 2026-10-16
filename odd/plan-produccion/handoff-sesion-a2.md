# Handoff — Sesión A2: cerrar la sesión A

Continuación de la **Sesión A** del reparto sugerido en [`README.md`](README.md) §7. Cubre los frentes **4, 10 y 11** del [plan de producción](README.md). Este documento es autocontenido: no necesitas leer la conversación anterior.

## Objetivo

Terminar los frentes **4** (recuperación de fallos terminales y replay de DLQ), **10** (OpenAPI como fuente de verdad y contract tests) y **11** (hardening BH-01…BH-11). **BH-12 permanece diferida** (activación explícita).

## Estado al cierre de la sesión A (2026-10-16)

- Rama de trabajo: **`fix/session-copy-and-correlation-id`** (los commits de la sesión A se hicieron aquí; no se creó rama nueva).
- **Commits de la sesión A, en orden:**

| Commit | Unidad | Contenido |
|---|---|---|
| `ee215b4` | BH-01 | Baseline de toolchain: `odd/tasks/security/backend-hardening/01-toolchain-baseline.md` |
| `b91714b` | BH-08 / frente 10 | Contrato OpenAPI alineado + `gateway/.../contract_test.go`, `risk/.../contract_test.go`; `kin-openapi v0.133.0` en `gateway/go.{mod,sum}` y `risk/go.{mod,sum}`; `08-openapi-errors.md` |
| `aed5e09` | BH-02 | Evidencia de scans: `02-vulnerability-scans.md` |
| `678a13a` | BH-09 | `jobs/internal/handler/auth_negative_test.go` + `09-authentication-tests.md` |

- **Sin commitear y AJENO a la sesión A** (otra sesión, B/observabilidad, y WIP del plan): `backend/services/risk/cmd/server/{main.go,main_test.go}`, `risk/internal/service/{publisher.go,publisher_test.go}`, `risk/internal/metrics/`, `backend/deploy/k8s/base/{kustomization.yaml,autoscaling.yaml}`, `backend/deploy/load/`, `backend/deploy/local/{prometheus.yml,alerts.yml}`, `backend/docker-compose.yml`, `backend/docs/operations.md`, y los docs sin commitear `README.md`, `backend/docs/architecture.md`, `backend/docs/code-explanation/README.md`, `odd/urgent/README.md`, `odd/plan-produccion/`. **No tocar, no commitear, no formatear sin decisión explícita del usuario.**

## Cómo retomar

1. `mem_context` y `mem_search` en el proyecto `firewatch`; topics útiles: `firewatch/backend-hardening/bh-01-baseline`, `firewatch/backend-hardening/bh-08-openapi-errors`, `firewatch/parallel-sessions/shared-worktree`.
2. Leer el documento de la unidad que toque antes de escribir: `odd/tasks/security/backend-hardening/<NN>-*.md` (criterios de aceptación y checklist) y la fila correspondiente de este handoff.
3. Confirmar el árbol con `git status` **antes** de commitear: si aparece un archivo que no tocaste, es de otra sesión → no lo incluyas.

## Tabla de estado por unidad

| Unidad | Estado | Evidencia | Puerta / siguiente paso |
|---|---|---|---|
| **BH-01** Toolchain y baseline | **Cerrada** | `ee215b4`, `01-toolchain-baseline.md` | — |
| **BH-02** Scans de dependencias e imágenes | **Parcial** | `aed5e09`, `02-vulnerability-scans.md` | `govulncheck@v1.9.0` limpio en los 4 módulos. **Falta**: inventario de imágenes + scan con `trivy 0.69.3` → requiere un daemon de Docker levantado. El mínimo de la checklist de imágenes queda abierto. |
| **BH-03** Cuotas del gateway | Pendiente | `03-gateway-rate-limits.md` | **Decisión humana**: límites agregado/autenticado/anon, fuentes de identidad confiables (no confiar en IP reenviada) y comportamiento de rutas de salud. Luego, limiter + tests. Debe mantener el `429` consistente con BH-08. |
| **BH-04** Mínimo privilegio del broker | Pendiente | `04-broker-least-privilege.md` | **Decisión humana/accesos** al entorno objetivo. |
| **BH-05** Seguridad de transporte | Pendiente | `05-transport-security.md` | **Decisión de entorno**; coordina con BH-04. |
| **BH-06** Egress de Kubernetes | Pendiente | `06-kubernetes-egress.md` | **Decisión de entorno** (routing/plugin). |
| **BH-07 / frente 4** Recuperación y replay DLQ | Pendiente | `07-job-recovery.md`, `04-dlq-replay-terminal.md` | **Decisión humana**: política de reintentos/backoff, transitorio vs permanente, estados terminales observables y replay idempotente. Coordina con **BH-10**. Implementar SOLO después de acordar la política. |
| **BH-08 / frente 10** OpenAPI y contract tests | **Implementada; verificación pendiente** | `b91714b`, `08-openapi-errors.md` | Los contract tests pasan y el RED quedó probado contra el contrato previo. **Correr la verificación independiente**: ver el comando abajo. Si su veredicto no se capturó, hay que repetirla (se había lanzado como tarea en segundo plano, que no sobrevive al reinicio de sesión). |
| **BH-09** Tests negativos de autenticación | **Implementada; ratificación pendiente** | `678a13a`, `09-authentication-tests.md` | Cobertura añadida sobre el path real de `Authorize`. **Pendiente humano**: ratificar por escrito la política de tokens (hoy solo está en código: RS256 + JWKS remoto). |
| **BH-10** Idempotencia concurrente | Pendiente | `10-concurrent-idempotency.md` | **Infraestructura**: DB y broker **aislados** (`docker compose --profile test`) y salvaguardas contra los recursos de aplicación. Roto por Docker caído. |
| **BH-11** Seguridad de despliegue | Pendiente | `11-deployment-security-verification.md` | Depende de BH-02/04/05/06; se valida al final. |
| **BH-12** Errores centralizados | Diferida | `12-centralized-error-handling.md` | No implementar sin activación explícita. |

## Decisiones humanas que desbloquean la sesión A2

1. **Docker**: levantarlo desbloquea **BH-02** (scan de imágenes) y **BH-10** (infra aislada).
2. **Política de recuperación** (frente 4 / BH-07): reintentos, backoff, transitorio vs permanente, estados terminales.
3. **Cuotas del gateway** (BH-03).
4. **Entorno/accesos** para BH-04/05/06/11.

## Entorno y gotchas (Windows / PowerShell 5.1)

- **Go**: local `1.27.1` con `GOTOOLCHAIN=auto` → resuelve a `go1.27.2`, que es lo que exigen `backend/go.work` y todos los `go.mod`, y lo que fija CI (`.github/workflows/_go-service.yml`). No hace falta instalar nada.
- **Tests de módulo** (replican CI; CI además corre `-race`):
  ```powershell
  # desde backend/services/<svc>  (gateway, risk, jobs)  o  backend/contracts/gen
  $env:GOWORK='off'; go test -count=1 ./...
  ```
- **Contrato OpenAPI** (mismo validador que CI; sale 0 sin salida cuando es válido):
  ```powershell
  # desde backend/
  go run github.com/getkin/kin-openapi/cmd/validate@v0.133.0 contracts/openapi/firewatch-v1.yaml
  ```
- **Vulnerabilidades** (desde cada módulo): `go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...`
- **Verificación enfocada de la sesión A**:
  ```powershell
  # backend/services/gateway  y  backend/services/risk
  $env:GOWORK='off'; go test -run Contract -count=1 ./internal/handler/
  # backend/services/jobs
  $env:GOWORK='off'; go test -run TestAuthorize -count=1 -v ./internal/handler/
  ```
- **Docker**: el daemon **no estaba disponible** en la sesión A. Verificar con `docker version` antes de BH-02/BH-10.
- **`tmp/`** contiene copias viejas del backend (`tmp/backend-urgent`, `tmp/ci-gates`, `tmp/rediseno`) — **ignorarlas**.
- **CodeGraph** está indexado en la raíz; conviene sincronizar antes de consultas estructurales.
- La suite completa (todos los módulos) tarda ~30 s; los tests de handler del gateway/risk son rápidos.

## Archivos clave de la sesión A

- `backend/contracts/openapi/firewatch-v1.yaml` — contrato; `servers:` apunta al **gateway**, por eso `429`/`502` aplican a toda operación.
- `backend/services/gateway/internal/handler/{gateway.go,contract_test.go}` — limitador global, proxy y su contrato.
- `backend/services/risk/internal/handler/{api,security,sources,status,contract_test.go}.go`
- `backend/services/jobs/internal/handler/{api,security,status,auth_negative_test.go}.go`
- `odd/tasks/security/backend-hardening/` — criterios y evidencia por unidad.
- `odd/plan-produccion/{README.md,04-dlq-replay-terminal.md,10-openapi-contract-tests.md,11-hardening-bh.md}` — el plan.

## Recordatorio de evidencia

No marcar una casilla sin resultado observado. `pasa` cubre solo los comandos y paquetes ejecutados; distinguir siempre *limpio* de *no ejecutado*. Verificación independiente obligatoria para cambios que toquen un ítem de riesgo alto (el contrato, BH-08, lo es: ítem 3).

## Decisiones resueltas (2026-10-16, sesión grill)

Las cuatro decisiones humanas de arriba quedaron resueltas. La política acordada vive en el documento de cada unidad: [`03-gateway-rate-limits.md`](../tasks/security/backend-hardening/03-gateway-rate-limits.md), [`07-job-recovery.md`](../tasks/security/backend-hardening/07-job-recovery.md) y [`09-authentication-tests.md`](../tasks/security/backend-hardening/09-authentication-tests.md).

- **D1 — Alcance de A2 en dos niveles.** Nivel 1, cerrable sin entorno: BH-03, BH-07 (política + implementación + tests unitarios), BH-08 (re-verificación independiente) y BH-09 (ratificación escrita). Nivel 2, **diferido con razón registrada y sin marcar hecho**: BH-02 (scan de imágenes), verificación end-to-end de BH-07, BH-10, BH-04/05/06/11 y BH-12. Docker sigue caído: `docker version` falla contra `npipe:////./pipe/dockerDesktopLinuxEngine`.
- **D2/D3 — Recuperación.** Estado terminal explícito `failed` con `failure_reason` y `attempts` persistidos, más replay idempotente de operador. `failed` **solo con evidencia real** (fallo permanente, o entrega en la DLQ ligada al `job_id`), nunca por vencimiento de plazo. Reintentos sin cambios: 5 intentos, `{1,2,4,8}` s.
- **D4 — Cuotas del gateway.** Agregado sin cambios (refill 20/s, burst 40 por réplica); autenticado por `sub` 5/s con burst 10; anónimo compartido 1/s con burst 5; estado acotado con evicción. Sin `X-Forwarded-For` ni proxy confiable declarado.
- **D5 — Tokens.** Se ratifica la asimetría tal cual: `jobs` solo `admin` (incluidas las lecturas), `risk` `viewer` o `admin`.
- **D6 — Rama.** El trabajo vive en **`fix/backend-hardening-bh`**, nacida en `main`. `fix/session-copy-and-correlation-id` volvió a `origin` (`cbb92ec`), así que PR #28 conserva su alcance de navegador.
- **D7 — Archivos.** `.agents/` quedó en `.gitignore`; la doc se commiteó completa; los artefactos de los frentes 5/6 (sesión B) se commitearon junto a `backend/docs/operations.md` para que la documentación y el código que describe lleguen en el mismo commit.

**Los commits de la sesión A cambiaron de SHA** al reaplicarse sobre `main`: `ee215b4`→`02911d5`, `b91714b`→`750cd80`, `aed5e09`→`702aff6`, `678a13a`→`03baa09`. La tabla de la sección «Estado al cierre de la sesión A» apunta a los SHA viejos, que ya no existen en la rama de trabajo.
