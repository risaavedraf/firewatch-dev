# Recorrido con sesión: hallazgos de Playwright y sus arreglos — FireWatch

**Objetivo:** cerrar lo **accionable** del recorrido con sesión real que otra sesión ejecutó con Playwright (informe completo en L2), sobre `main` `41b9514`.
**Entrega:** dos PRs — `fix/gateway-expose-request-id` (`type:bug`) y `docs/playwright-round-trip-report` (`type:docs`). `main` se tocó **una sola vez**, para fusionar PR #16 y habilitar el flujo issue-first.
**Fuera de alcance, declarado:** el URI de post-logout en la consola de Keycloak (acción del usuario), la UX del logout con realm mal configurado y el 404 de zona sin lote (decisiones de producto y de contrato), las carreras de respuestas supersedidas y la cola muerta.

## Specs

### S1 — Alcance y decisiones, verbatim del usuario

> prepara los fixes de todo porfa, si hay que hacer algun push a main hazlo y luego continua con estos fixes en una nueva rama, mismo sistema para pushear, issue y pr de la skill

Tres decisiones del usuario que fijan el alcance:

1. **Push a main:** solo **PR #16** (adopción del flujo issue-first), porque `main` no tenía `.github/PULL_REQUEST_TEMPLATE.md`. **PR #18 queda intacto**, en revisión.
2. **Skill obsoleta:** corregirla **solo localmente, sin commit** (ver S5).
3. **Issues:** crearlos y aplicarles `status:approved`, cerrando con `Closes #N`.

### S2 — H1, el 403 de `#audit` no muestra su `request_id` (verbatim)

> ### H1 — El 403 de `#audit` no muestra su `request_id` (MEDIUM)
>
> - **Qué:** el mensaje de 403 no incluye `· request_id …`, pese a que el código lo concatena.
> - **Dónde:**
>   - `frontend/src/views/AuditView.tsx:75` — `const request = failure.requestId !== undefined ? ` · request_id ${failure.requestId}` : '';`
>   - `frontend/src/core/api.ts:174` — `const requestId = response.headers.get('X-Request-ID') ?? '';`
>   - `backend/services/gateway/internal/handler/gateway.go:59-69` — CORS fija `Access-Control-Allow-Origin`, `Allow-Methods`, `Allow-Headers`, `Max-Age` y `Vary`; **no** fija `Access-Control-Expose-Headers`.
> - **Causa raíz:** la respuesta es **cross-origin** (`:8080` desde `:4173`). El navegador solo expone al JS las cabeceras *safelisted* salvo que el servidor declare `Access-Control-Expose-Headers`; el gateway no lo hace (0 coincidencias de «Expose» en `backend/`). Por eso `get('X-Request-ID')` devuelve `null` y el `request_id` nunca se pinta.
> - **Prueba observada:** la respuesta cruda trae `x-request-id: 3a2165512429ed5575faae44bc7b944c, 3a216551-2429-ed55-75fa-ae44bc7b944c`; pero desde el JS de la página `fetch('http://localhost:8080/v1/audit').headers` expone solo `cache-control`, `content-length`, `content-type` — `x-request-id` = **null**. En el DOM no aparece la palabra «request» ni ningún UUID.
> - **Contradice:** el paso 7 de este handoff y `odd/urgent/rediseno-pagina/guion-recorrido.md:38` / `README.md:104`.
> - **Naturaleza:** el cliente lee bien; el arreglo es **del servidor (gateway)**. No se tocó.

**El cliente no se toca:** `api.ts:174` es el **único** consumo de una cabecera de respuesta no safelisted en `frontend/src/` (barrido: `grep -rn "headers.get"` → una sola coincidencia). No hay nada que corregir del lado de la página.

### S3 — Recomendación 1 del informe: arreglar el gateway (verbatim)

> 1. **Gateway (prioridad, arreglo de H1):** añadir `Access-Control-Expose-Headers: X-Request-ID` en el bloque CORS de `backend/services/gateway/internal/handler/gateway.go:59-69`, con su prueba (recordar S10: los errores 401/403/404 ya deben llevar CORS; sumar aquí el exposé).

El resultado exigido es el contrato observable por el navegador: **una respuesta cross-origin desde el origen configurado expone `X-Request-ID` al JS de la página**, y **un origen no configurado no recibe ninguna cabecera CORS, tampoco esta**.

### S4 — Recomendación 3: no dejar la afirmación sin sustento (verbatim)

> 3. **Documentación:** corregir `guion-recorrido.md:38` / `README.md:104` o arreglar el gateway; no dejar la afirmación sin sustento.

Se elige **arreglar el gateway**, así que las dos afirmaciones pasan a ser **ciertas** y **no se editan**. La obligación que queda es la otra mitad de la frase: darles sustento documental (S3 en el doc de contrato, T2).

### S5 — Recomendación 8: la skill obsoleta (verbatim) y la decisión tomada

> 8. **Skill desactualizado:** `.opencode/skills/firewatch-page-review/SKILL.md` sigue describiendo la página vanilla retirada; conviene actualizarlo o retirarlo para que nadie vuelva a revisar código muerto.

**Decisión del usuario: corregirla localmente, sin commit.** Precisión medida, porque la primera afirmación de esta sesión fue imprecisa: la regla que ignora `.opencode/` vive en el `.gitignore` de **PR #18**, que sigue abierto. Sobre `main` `41b9514` —la base de este trabajo— `.opencode/` está **sin trackear pero no ignorado** (`git check-ignore` → exit 1), así que técnicamente *podría* commitearse. La política, sin embargo, ya está adoptada en el `.gitignore` entrante: «agent-harness scratch que no es fuente del repo. […] ninguno es un entregable». Esta tarea respeta esa política y **no produce artefacto versionado**.

### S6 — El flujo de entrega, verbatim del usuario

> mismo sistema para pushear, issue y pr de la skill

Se sigue `odd/PR-CONVENTIONS.md` **vigente** (adoptado por PR #16): un issue con `status:approved` por PR, **exactamente una** etiqueta `type:*` con su casilla marcada, la plantilla `.github/PULL_REQUEST_TEMPLATE.md`, rama `tipo/descripcion`, Conventional Commits **sin** trailer `Co-Authored-By`.

### S7 — Lo que NO se toca, y por qué

- **`backend/docs/code-explanation/**`:** es una **instantánea fechada** (`backend/docs/code-explanation/README.md:7`) y su re-baseline «exige autorización explícita»; está declarada como deuda abierta. `01-gateway.md` documenta rangos de `gateway.go` que **ya** estaban obsoletos antes de este cambio. Este PR **agranda esa deuda declarada en 6 líneas medidas** —`gateway.go` pasa de **124 a 130** líneas— y lo dice en su Test Plan en vez de regenerar el manifiesto sin releer las citas.

**Regla que este error fija, y que es el motivo de esta enmienda:** un conteo se **mide**, nunca se estima ni se recuerda. La primera versión de S7 y de `L39` afirmaba «2 líneas» y **era falsa**: se declaró sin medirla, teniendo el `6 ++` del `git diff --stat` delante. El verificador independiente la falsificó y el coordinador la reprodujo. Es la misma regla que S14 ya enunciaba para su propio caso —«un conteo de errores nunca se declara desde una salida truncada»—, extendida a un conteo que no se declaró desde ninguna salida. Mismo tipo de fallo, misma corrección: declarar el número con su medición y su herramienta.
- **`frontend/**`:** sin cambios. El defecto es del servidor (S2).
- **La skill de S5:** no versionable, y la precisión importa — en `main` `41b9514` está **sin trackear pero no ignorada**; la regla que la excluye llega con el PR #18, y es la política que ese PR declara la que la vuelve no entregable. Las dos afirmaciones están **ancladas a `41b9514`**: al fusionarse el PR #18 describen un estado pasado, no el vigente, y se dejan escritas así a propósito.
- **Keycloak / realm:** nada. El URI de post-logout es acción del usuario en la consola; no se reinicia ni se reimporta el realm.

## Tasks

| ID | Spec | Qué | Rama | Cierra |
| --- | --- | --- | --- | --- |
| T1 | S2, S3 | `Access-Control-Expose-Headers: X-Request-ID` en `gateway.go`, con prueba RED→GREEN que además prueba el fail-closed | `fix/gateway-expose-request-id` | Issue A |
| T2 | S3, S4 | Sustento documental: `backend/README.md` (contrato de CORS) y spec S15 en `odd/tasks/architecture/gateway-frontend-separation.md` | `fix/gateway-expose-request-id` | Issue A |
| T3 | S2, S4 | Publicar el informe del recorrido como documento propio y registrar la corrección en el ledger | `docs/playwright-round-trip-report` | Issue B |
| T4 | S5 | Corregir `.opencode/skills/firewatch-page-review/SKILL.md` **sin commit** | — | — |

## Log

**L1** (usuario, verbatim — origen de todo este trabajo):

> prepara los fixes de todo porfa, si hay que hacer algun push a main hazlo y luego continua con estos fixes en una nueva rama, mismo sistema para pushear, issue y pr de la skill

**L2** — El informe que especifica este trabajo está en **`odd/tasks/reviews/playwright-signed-in-round-trip.md`** (T3). Citado verbatim en S2, S3, S4 y S5. Su brief de origen es `odd/urgent/rediseno-pagina/handoff-playwright-signed-in.md`, que vive en **PR #18** (abierto) y que este trabajo **no toca**.

**L7** — Orden de entrega, declarado porque un verificador lo midió y su medición es correcta: en la rama **`fix/gateway-expose-request-id`** el archivo del informe de L2 **todavía no existe** y el ledger —que sí existe— **todavía no tiene la entrada L8**. Ambos llegan con el **PR #22**. Quien verifique solo la rama de A reportará —con razón— que no los encuentra; esta nota existe para que eso se lea como el orden declarado y no como un enlace roto. Es un acoplamiento real entre dos PRs y se declara en vez de esconderse: si #22 no se fusionara, el informe no estaría en el repositorio.

**L8** — Atribución de las afirmaciones de este documento, para que ninguna se lea como medida cuando no lo es: los conteos de RED/GREEN, el crecimiento de líneas del snapshot y el estado de las ramas y los PRs **los midió el coordinador** con la herramienta que cita cada caso; las mediciones de navegador son **del informe** (L2) y no se reprodujeron aquí; y el **estado remoto histórico** (PR #18 abierto y `MERGEABLE`, un solo merge a `main`) es lo que el coordinador observó en su momento, no algo que una re-verificación documental pueda reconfirmar. Declararlo es preferible a que una de esas tres clases se lea como si fuera de otra.

**L3** — Estado medido antes de escribir: `main` = `41b9514` tras fusionar PR #16 (MERGED `2026-10-15T20:20:02Z`); PR #18 abierto, `MERGEABLE`, intacto. Suite base del gateway **en verde** antes del cambio: `go test ./...` → `ok firewatch.local/gateway/internal/handler 0.977s`, exit 0.

**L4** — El informe llegó como **modificación sin commitear** de un archivo que `main` no tiene (el handoff nace en PR #18). Se preservó su contenido en `tmp/handoff/handoff-full.md` (ignorado por Git) y se restauró el árbol antes de cambiar de rama, para no arrastrar un cambio ajeno a ningún commit.

**L6** — Corrección de una afirmación propia, hecha al medirla: dije que `.opencode/` estaba ignorado por `.gitignore:55`, y eso es cierto **solo en la rama de PR #18**. Sobre `main` `41b9514` —el commit en que se midió— esa línea no existía, así que `.opencode/` y `.playwright-mcp/` aparecían como **sin trackear** en `git status`; el PR #18 cambia eso al fusionarse. No cambia la decisión (S5), pero cambia la razón: lo que la sostiene es la política declarada en el `.gitignore` entrante, no una regla ya vigente. Queda dicho en lugar de dejar la justificación inexacta.

**Anomalía de la herramienta, declarada para que nadie la lea como un defecto del repositorio.** Un verificador independiente midió `git check-ignore -v .opencode/` y obtuvo una coincidencia (`.gitignore:38: .opencode/`, exit 0), lo que contradice lo de arriba. Medido a fondo **sobre `41b9514`**, y **es espuria**: el `.gitignore` del árbol de trabajo tenía 43 líneas, 1759 bytes, la **línea 38 vacía** y **cero** ocurrencias de la cadena `opencode`; la misma versión del archivo con finales **LF** (1716 bytes, o sea el blob versionado) **no producía coincidencia**, y ambas variantes reproducen el mismo resultado en un repositorio temporal aislado. `git version 2.45.1.windows.1`, con `core.autocrlf=true`: la coincidencia depende de los finales de línea del árbol de trabajo, no de ninguna regla escrita. **Lo que sí está medido sobre `41b9514`, y es lo que sostenía la decisión entonces:** el archivo de la skill no estaba ignorado (`git check-ignore -v .opencode/skills/firewatch-page-review/SKILL.md` → sin coincidencia, exit 1) y `git ls-files --others --exclude-standard .opencode/` lo listaba como sin trackear y no ignorado. Al fusionarse el PR #18, `.opencode/` **sí** queda ignorado: la decisión de no versionarlo se mantiene, ahora por la regla vigente en lugar de por ausencia de regla. Queda escrito porque un verificador que pruebe **solo la forma del directorio** llegará a la conclusión contraria, y merece saber por qué antes de reportarla.

**L5** — Elección de ruta para el informe (T3): se publica como **documento propio** en `odd/tasks/reviews/` en vez de como apéndice del handoff. Motivo: eso último obligaría a escribir en la rama de **PR #18**, que está **en revisión** y que el propio handoff pide no modificar («No lo arregles: esta entrega está en revisión»).
