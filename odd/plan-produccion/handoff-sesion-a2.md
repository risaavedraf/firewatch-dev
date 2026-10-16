# Handoff — Sesión A2: cerrar la sesión A

Continuación de la **Sesión A** del reparto sugerido en [`README.md`](README.md) §7. Cubre los frentes **4, 10 y 11** del [plan de producción](README.md). Este documento es autocontenido: no necesitas leer la conversación anterior.

## Objetivo

Terminar los frentes **4** (recuperación de fallos terminales y replay de DLQ), **10** (OpenAPI como fuente de verdad y contract tests) y **11** (hardening BH-01…BH-11). **BH-12 permanece diferida** (activación explícita).

## Estado que la sesión A dejó al abrirse la A2 (2026-10-16)

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

## Estado al cierre de la A2 (2026-10-16)

Rama **`fix/backend-hardening-bh`**, nacida en `main` (`b0f77f9`). Árbol limpio. **15 commits, 77 archivos, +5383/−152. Sin pushear, sin issue, sin PR.**

| Commit | Unidad | Contenido |
|---|---|---|
| `02911d5` | BH-01 | Baseline de toolchain *(cherry-pick de `ee215b4`)* |
| `750cd80` | BH-08 | Contract tests contra rutas reales *(cherry-pick de `b91714b`)* |
| `702aff6` | BH-02 | Evidencia de scans de dependencias *(cherry-pick de `aed5e09`)* |
| `03baa09` | BH-09 | Tests negativos de la política de tokens *(cherry-pick de `678a13a`)* |
| `2ab9cc4` | — | `.agents/` fuera de control de versiones |
| `9326daa` | frentes 5/6 | Métricas de resultado y artefactos operativos de la sesión B, con `operations.md` |
| `784f589` | — | El plan de producción, un archivo por frente |
| `e025643` | — | Arranque del frontend, semántica de health y citas obsoletas |
| `29811ef` | A2 | Decisiones D1–D7 y las políticas acordadas |
| `2981dc5` | **BH-03** | Tres capas de cuota del gateway |
| `54c1ce8` | **BH-07 u1** | Estado terminal `failed` en la fila del job |
| `1436dae` | BH-08 | Corrección de dos descripciones falsas del contrato, más las sondas del verificador |
| `0b0f784` | **BH-07 u2** | Reporte de fallo `risk`→`jobs` y señal de job atascado |
| `537f83a` | **BH-07 u3** | Replay de operador |
| `a00b483` | BH-07 | Corrección del bloqueador del runbook, más las sondas del verificador |

### Qué está entregado y verificado

- **BH-03** — el agregado pre-auth queda sin cambios (refill 20/s, burst 40 por réplica) y sigue siendo la única cota previa a la autenticación; se añaden la cuota por `sub` tras el JWT (5/s, burst 10) y una cuota anónima **compartida** (1/s, burst 5), con estado acotado por evicción y tope duro. Sin `X-Forwarded-For` ni proxy confiable. El `429` conserva su forma.
- **BH-07** — `failed` con `failure_reason`, `attempts` y `failed_at` persistidos; el consumidor de `jobs` marca su propia fila al agotar; `risk` reporta su fallo por su **outbox** (`risk.job.failed.v1`) y `jobs` lo aplica; replay de operador admin-only con transición guardada; `firewatch_queued_job_age_seconds`, alerta de job atascado y runbook.
- **Evidencia observada**: suites verde en `gateway`, `jobs` (5 paquetes), `risk` (10) y `contracts/gen`; `gofmt` y `go vet` limpios; el contrato valida con el mismo validador que CI. Verificación independiente en **dos rondas**, que encontró y cerró **3 bloqueadores**: dos descripciones del contrato que afirmaban comportamiento inexistente y una instrucción del runbook que prescribía un replay imposible.

### Huecos declarados — leer esto antes de retomar

1. **Un job `queued` cuya entrega no se puede rastrear no tiene camino de replay.** El guard `failed`-solo es deliberado, porque un job encolado puede seguir en vuelo. **Decisión pendiente:** extender el replay a `queued` con un guard explícito contra reencolar trabajo en vuelo, o aceptar el hueco. Está escrito en `backend/docs/operations.md` §10.
2. **No observado, exige PostgreSQL:** un reenvío tardío del fallo viejo podría volver a marcar `failed` un job ya replayado (`Fail` no compara identidad de evento); y si `risk` ya persistió el lote pero `jobs` no consumió el resultado, la rama de conflicto de lote de `risk` no emite otro completion, así que el replay podría dejar el job `queued`.
3. **Advisory:** con `Broker.Fail == nil` un `risk.job.failed.v1` se ACKea sin escribir. Producción lo cablea, pero a diferencia del give-up esa rama no deja dead letter.
4. **Sin probar en este host:** `-race` (no hay compilador de C y `CGO_ENABLED=0`), la migración `004` y las transacciones contra PostgreSQL, y la topología viva contra RabbitMQ. Docker sigue caído. CI es la comprobación autoritativa de esas partes.

### Revisión nativa

El candidato se evaluó dos veces (`assess` → `high`, `review_due` por riesgo) y el usuario **saltó ambas**. Cada declive es por candidato: no se creó registro, nada se consumió y las revisiones siguen activas. El límite revisado **no avanzó**, así que el próximo corte sigue siendo `b0f77f9..HEAD`.

### Qué falta de A2

- **BH-08** — la re-verificación independiente, que se perdió con el reinicio de sesión; hoy sería sobre `750cd80` y `1436dae`.
- **BH-09** — la ratificación escrita de la política de tokens, incluida la asimetría `admin`/`viewer` de D5.
- **Push, issue y PR** — la rama es local. **Decisión humana**, no un pendiente técnico.

### Cómo retomar

1. `mem_context` y `mem_search` en el proyecto `firewatch`; topics útiles: `firewatch/backend-hardening/a2-decisions` y `firewatch/backend-hardening/a2-execution`.
2. `git status` **antes** de escribir nada: si aparece un archivo que no tocaste, es de otra sesión → no lo incluyas.
3. El estado real por unidad vive en `odd/tasks/security/backend-hardening/<NN>-*.md` y en el `## Log` de `odd/plan-produccion/`. No marcar una casilla sin resultado observado.

## Estado al cierre de la sesión A (2026-10-16)

**La sesión A queda cerrada.** Los tres pendientes que A2 dejó abiertos se resolvieron, y hay un cuarto entregado por decisión humana:

| Pendiente de A2 | Resolución |
| --- | --- |
| **BH-09** — ratificación escrita de la política de tokens | **Ratificada por el usuario tal cual está en el código** (D5): RS256 + JWKS remoto, `typ`/`sub`/`exp` verificados, cierre en falso (401) si el JWKS no responde, `jobs` solo `admin` incluidas las lecturas, `risk` `viewer` o `admin`. |
| **BH-08** — re-verificación independiente | **Re-verificada en línea**: RED reproducido en un export aislado con el contrato previo, GREEN en el árbol actual, matriz operación-error contrastada contra los handlers y validador de CI en 0. La ruta de verificador independiente fue **rechazada por admisión en tiempo de ejecución** (el candidato no declara ítem de riesgo alto ante la compuerta), así que queda registrado como auto-verificación con sus límites escritos, no como verificación independiente. Una promesa de más del contrato se corrigió en `cf23d20`. |
| **Hueco declarado #1** — un job `queued` sin camino de replay | **Resuelto por decisión humana: extender el replay**, no aceptar el hueco. Entregado en `90f48a6`: confirmación explícita (`{"confirm":"queued_may_be_in_flight"}`; sin ella `409 replay_confirmation_required` y nada escrito) y, en `risk`, completion re-emitido para el lote ya almacenado. Límite residual declarado: la confirmación es una atestación del operador y el **trabajo** duplicado se acota, no se evita. |
| **Entrega** | Autorizada por el usuario y **ejecutada**: rama publicada en `origin` y **[PR #29](https://github.com/risaavedraf/firewatch-dev/pull/29)** abierto contra `main` (80 archivos, +6 000/−166). |

**Commits nuevos desde el cierre de A2:** `90f48a6` (BH-07 u4: replay de `queued` + compuerta de conflicto en `risk`), `cf23d20` (corrección de la descripción de `ErrorResponse.code`), `c2f5044` (cierre documental) y las dos correcciones que CI encontró al publicar la rama: `8ac654e` (el contrato en la imagen de pruebas de `risk`) y `761c16c` (las citas de guards del backend en el frontend).

**Rebanadas de revisión del PR — una unidad por commit:** BH-01, BH-08/frente 10, BH-02, BH-09, frentes 5/6 de la sesión B, plan de producción, decisiones A2, BH-03, BH-07 u1, u2, u3, BH-07 u4 + contrato (`90f48a6`), corrección del contrato (`cf23d20`), cierre documental. La rama acumula el frente completo (≈6 000 líneas autoradas), así que la revisión conviene hacerla **por rebanada**, no como un diff único.

**CI del PR — lo que solo podía correr allí.** Publicar la rama destapó **dos defectos previos de la rama** que nunca se habían ejecutado (ninguno del cierre): la etapa `test` de `risk` no copiaba `contracts/openapi/` al contenedor, así que sus contract tests fallaban con «no such file or directory», y el test de frontend que cita guards del backend por `archivo:línea` había quedado obsoleto porque BH-03/BH-07 movieron esas líneas. Ambos se corrigieron (`8ac654e`, `761c16c`) y el re-run quedó **verde**: `checks` (frontend, 24 archivos / 235 pruebas), los cuatro `checks / test` de Go y **`compose`**, que corre los perfiles `test` con PostgreSQL, el smoke de resiliencia, `kubeconform` y el validador del contrato. Como la imagen de pruebas recibe `TEST_DATABASE_URL` y el `ENTRYPOINT` no usa `-short`, las pruebas de integración **no pueden saltar**: las reglas de base de datos nuevas **corrieron y pasaron en CI** (`TestReplayQueuedJobRequiresConfirmation` en `jobs`, y las dos pruebas de deduplicación/escenario de `risk`). **Sigue diferido:** el escenario end-to-end del documento de BH-07 (interrumpir PostgreSQL, agotar reintentos, restaurar y reiniciar consumidores) y `-race` en este host.

**Evidencia observada en esta sesión:** suites verdes en `gateway` (2 paquetes), `risk` (10), `jobs` (5) y `contracts/gen`; `gofmt -l` sin salida; `go vet` limpio en los cuatro módulos; `kin-openapi v0.133.0 validate` en 0; pruebas de contrato de `gateway`/`risk` y de replay de `jobs` en PASS; RED del hueco de contrato reproducido en un árbol aislado. **No ejecutado aquí:** las pruebas de integración con PostgreSQL (saltan sin `TEST_DATABASE_URL`; CI las corre con `docker compose --profile test run --rm jobs-test` y `risk-test`) y `-race` (sin compilador de C en este host).

**Sigue diferido, con razón y sin decisión de producto pendiente:** BH-02 (inventario y scan de imágenes: sin daemon de Docker y sin `trivy` en este host), BH-10 (DB y broker aislados), BH-04/05/06/11 (decisiones de entorno y accesos), BH-12 (activación explícita), la verificación end-to-end de BH-07 (PostgreSQL + broker) y el `-race` local.

**Revisión nativa:** el candidato `b0f77f9..HEAD` se evalúa de nuevo como `high` con `review_due` (80 rutas, ≈6 081 líneas). Los dos declives anteriores fueron por candidato, y este incluye los commits de hoy, así que la oferta se relanzó **una vez**.

**Resultado de la oferta:** el consentimiento se **declinó para este candidato** el 2026-10-16 — `consent: declined_this_candidate`, `lineage_created: false`, `mutation_performed: none`. No se creó linaje ni se consumió autoridad, y el declive es por candidato: un cambio nuevo vuelve a ofrecer su propia revisión. El push y el PR no dependieron de ella.
