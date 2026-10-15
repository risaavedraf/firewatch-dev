# Arreglos de la revisión de navegador — FireWatch

**Objetivo:** cerrar los defectos que una revisión de navegador con Playwright encontró en la página, sobre la **base real** (`origin/main` `a6b1cb2`, app React + Vite + TypeScript).
**Rama:** `fix/frontend-review-findings`, creada desde `origin/main` `a6b1cb2`. `main` no se toca.
**Estado:** implementación en curso. La página vanilla que el informe describió (`frontend/proposal.js`, `proposal.css`) está **retirada**: sus defectos sobreviven en el código React y se arreglan allí.

## Specs

### S1 — Alcance: los cinco hallazgos del informe, verbatim

El informe completo está en **L1**. Sus hallazgos son la especificación y se citan textualmente en S2–S6. Nada fuera de ellos entra en el alcance salvo lo declarado en S11.

### S2 — F1, logout no cierra sesión (verbatim)

> **F1 — Logout does not log out; Keycloak returns 400 (HIGH)**
> Did: logged in as demo-admin, clicked "Cerrar sesión".
> Expected: return to http://localhost:4173/, signed out.
> Actual: browser goes to `http://localhost:8081/realms/firewatch/protocol/openid-connect/logout?id_token_hint=…&client_id=firewatch-demo&post_logout_redirect_uri=http%3A%2F%2Flocalhost%3A4173%2F` → HTTP 400, page says "We are sorry… Invalid redirect uri". It never returns to :4173.
> Root cause: the live realm's firewatch-demo client has no http://localhost:4173/ post-logout URI. The shipped backend/deploy/local/firewatch-realm.json:18 does declare `http://localhost:8080/##http://localhost:4173/`, but the Keycloak volume is 8 days old and --import-realm never updates an existing realm.
> Second consequence (also confirmed): because RP-initiated logout failed, the Keycloak SSO session survived. Clicking "Iniciar sesión con Keycloak" again returned to :4173 fully authenticated with no password prompt — logout is effectively a no-op.

**El código no es el defecto.** Verificado por esta sesión, sin credenciales y contra el realm vivo:

| Petición | Resultado |
| --- | --- |
| `logout?client_id=firewatch-demo&post_logout_redirect_uri=http://localhost:4173/` | **400 "Invalid redirect uri"** |
| la misma, con `http://localhost:8080/` | **302** |
| `auth?...&redirect_uri=http://localhost:4173/` | **302** (el de login **sí** está registrado) |

Misma petición, mismo cliente, distinto URI: falta exactamente el de post-logout. `frontend/src/config.ts` mantiene `redirectURI = 'http://localhost:4173/'` y `src/core/session.ts` arma el logout con `post_logout_redirect_uri: redirectURI`, así que el defecto es de estado del realm y **no cambia con la base**.

### S3 — F2, el fondo del sidebar es inalcanzable (verbatim)

> **F2 — Sidebar bottom is unreachable at laptop heights (HIGH)**
> Did: measured #logout / Gateway card at several viewports.
> Actual: the whole .sidebar-bottom sits below the viewport and cannot be scrolled to: viewport 1280×720, 1366×768, 1440×900, 1024×768, 1920×1080.
> A real Playwright click on #logout timed out: "element is outside of the viewport".
> Root cause: frontend/proposal.css:70 — `.sidebar { position: sticky; top: 0; height: calc(100vh - 34px); overflow: visible }` with `.sidebar-bottom { margin-top: auto }`. Content is 925 px tall vs clientHeight 686 (1280×720) → 239 px overflow, no internal scroll, sticky element pinned → permanently below the fold.
> Impact: at common heights, mouse users cannot sign out, and the Gateway status card / "Actualizar estado" are also unreachable. (Sign-in still works via the main "Conectar sesión" card.)

**Sobrevive al port, idéntico.** `frontend/src/styles/app.css:41` repite la misma declaración, **sin `overflow`**:

```css
.sidebar { position: sticky; top: 0; width: 234px; height: calc(100vh - 34px); min-height: 560px; display: flex; flex-direction: column; padding: 30px 19px 22px; color: #d5e1db; background: var(--forest-900); }
```

Barrido sobre 13 refs (todas las ramas de frontend y `origin/main`): **13/13 sin `overflow`**, y **cero** archivos `.tsx` contienen `overflow`, así que tampoco hay un arreglo a nivel de componente. La estructura que desborda sigue igual: `.sidebar-bottom { margin-top: auto }` (`:56`) y el override de 720 px (`:361`).

### S4 — F3, el botón de login no refleja la sesión (verbatim)

> **F3 — Login button stays visible while authenticated (MEDIUM)**
> After a successful login the "Conectar sesión" card still shows an enabled, visible "Iniciar sesión con Keycloak" button, while the sidebar says "Sesión conectada / Cerrar sesión". Clicking it restarts the OAuth flow.

**Sobrevive, con otra forma.** En React la asimetría es explícita: `src/components/Shell.tsx:153` usa `hidden={!signedIn}` en `#logout`, mientras `src/views/SessionView.tsx:238` renderiza `#login` sin ninguna condición de deshabilitado. Barrido sobre 13 refs: **0 versiones** tienen `disabled` en `#login`. El estado disponible es `session.status` con los valores `signed-in`, `signing-in` y el resto (`SessionView.tsx:212-219`).

### S5 — F4, favicon 404 en cada carga (verbatim)

> **F4 — favicon.ico 404 on every load (LOW)**
> GET /favicon.ico → 404 (no favicon declared; server serves 3 assets) logs a console error on each load.

**Sobrevive.** `frontend/index.html` no declara ningún `<link rel="icon">`, y **0 refs** de 13 lo declaran. Pesa más que antes: `frontend/scripts/serve.mjs` ya no usa una lista fija de tres archivos; construye la allowlist leyendo los `src`/`href` del `dist/index.html` construido. Un ícono en `data:` no necesita allowlist; un archivo externo sí, y obligaría a tocar el build o la allowlist.

Restricción de la CSP vigente (`serve.mjs`): `img-src 'self' data:`. Un `data:` URI es admisible sin cambiar la política.

### S6 — F5, los 503 son una cola muerta (verbatim, INFO)

> **F5 — 503s are a DLQ backlog, and the UI degrades correctly (INFO)**
> /v1/risk/status and /v1/jobs/status → 503 {"ready":false,"reason":"dead_letter_backlog", dead_letter depth 3 > max_depth 0}. Data endpoints return 200 normally. The page shows it honestly ("Desconocido: … HTTP 503 · dead_letter_backlog"). Environment issue, not frontend. Polled every 30 s, so the console gains 2 errors/min while degraded.

No es un defecto del frontend y la página degrada correctamente. El alcance autorizado (S11) es un **diagnóstico read-only**: no se descarta ni se recarga nada sin decisión explícita del usuario.

### S7 — Lo verificado correcto no se toca

El informe declara correctos el round trip de login con PKCE S256, el almacenamiento del token solo en memoria, los roles, el cross-origin, el mapa, el teclado, la degradación y el flujo de trabajos. Esta ruta no los modifica y no puede romperlos: el arreglo de S3 es CSS, el de S4 es estado de un control y el de S5 es un `data:` URI en el documento.

### S8 — Base correcta y qué NO se arregla

La base es `origin/main` `a6b1cb2`. La página que el informe describió (`frontend/proposal.js`, `frontend/proposal.css`, `tests/proposal-readback.test.cjs`) **está retirada** y no existe en esa base. **No se escribe ni una línea sobre ella.** El defecto se arregla donde vive hoy: `frontend/src/styles/app.css`, `frontend/src/views/SessionView.tsx`, `frontend/index.html`.

### S9 — Entrega: rama + issue + PR con el flujo completo de la skill

Decisión del usuario, verbatim: «plan rama aparte con issue y pr, para que calce con las skills de gentle para los pr y agregalo al que falta en plan reestructuración», y ante el conflicto con `odd/PR-CONVENTIONS.md`: «Flujo completo de la skill».

Como el repositorio registró que **no** tiene etiquetas `type:*` / `status:approved` ni plantilla de PR, y su propio documento dice que adoptar el flujo «se hace una sola vez, en un PR de configuración», esta ruta entrega **dos PRs**:

1. **PR de configuración** — crea las etiquetas, agrega `.github/PULL_REQUEST_TEMPLATE.md` y actualiza `odd/PR-CONVENTIONS.md` para registrar el cambio de política.
2. **PR de los arreglos** — con issue aprobado enlazado y exactamente una etiqueta `type:*`.

Ningún merge lo ejecuta esta ruta: mergear es decisión del usuario.

### S10 — Realm: el usuario aplica, esta ruta verifica

Verbatim: «yo tengo acceso al realm, como lo arreglo». Esta ruta entrega los pasos exactos sobre el realm vivo, el usuario los aplica y esta ruta **re-verifica sin credenciales**: la sonda de S2 debe pasar de **400 a 302**. Restricción vigente: no reiniciar el volumen de identidad ni reimportar el realm (`backend/docs/keycloak-realm-setup.md`), y no saltar de Keycloak 26.5.2 (arriba de 26.6.0 los URI `http` fallan por `secure-client-uris`).

### S11 — Alcance adicional autorizado

Verbatim: «Todo». Incluye: (a) F4, (b) corregir la deriva de documentación, (c) `.gitignore` para los artefactos sin versionar, (d) el diagnóstico **read-only** de la cola muerta.

### S12 — Invariantes de la app React que sobreviven

De `odd/urgent/rediseno-pagina/app-react-vite.md` S17: sin gating por rol en el cliente; 401 → reset de sesión; tokens solo en memoria (la transacción PKCE es lo único en `sessionStorage`); sin fixtures ni reloj congelado; la página se abre como `http://localhost:4173` y nunca como `127.0.0.1:4173`; CSP con `connect-src http://localhost:8080 http://localhost:8081`; ningún secreto en variables `VITE_*`; sin cambios de contrato de endpoints; y **ninguna aserción existente se pierde en silencio**.

### S13 — Coordinación entre sesiones

La sesión vecina de `tmp/rediseno` (`frontend/react-vite`) confirmó, con evidencia de `ls-remote`: (1) los tres defectos **no son suyos**; (2) su PR #3 está **CLOSED**, `mergedAt: null`, y su contenido llegó a `main` por la cadena rediseno-01..09, con `readiness.ts` idéntico byte a byte — **no hay merge pendiente** que condicione la base; (3) no toma ninguno de los tres, así que esta ruta los toma todos.

Matiz que se registra como decisión **nueva y separada**, no como merge: `origin/main` tolera un lote con `notice` ausente o no-string y renderiza el fallback fijo, mientras `frontend/react-vite` lo rechaza en el borde (fail-closed, `api.ts:291`). Esta ruta no cambia ese comportamiento en ninguna de las dos direcciones.

### S14 — Procedencia de la evidencia

En este documento, «observado» significa que **esta sesión ejecutó la comprobación**; «reportado» significa que proviene del informe o de otra sesión. Son observados: la sonda 400/302 del realm, el barrido de 13 refs, el 404 del favicon, y la detención de la pila. Son reportados: las mediciones de viewport y los clics reales de Playwright, y el `503` de la cola muerta (esta sesión no pudo autorizar sus sondas: sin token los endpoints devuelven `401`).

## Tasks

- **B1 — S3 — F2 sidebar inalcanzable** — ruta: inline (CSS de una línea) — verificación: medición en navegador a 1280×720, 1366×768, 1440×900 y 1024×768 con clic real en `#logout` — commit: `b5c689c`
- **B2 — S4 — F3 estado del control de login** — ruta: inline, test-first con vitest (RED → GREEN) — commit: `dc1cbe4`
- **B3 — S5 — F4 favicon con `data:` URI** — ruta: inline — commit: `08bdf65`
- **B4 — S11 — deriva de documentación y `.gitignore`** — ruta: inline — commits: `5f23f54` (higiene), `400ea42` (documentación)
- **B5 — S6 — diagnóstico read-only de la cola muerta** — ruta: delegada a `gentle-ai-explore` (fuente) + lectura propia de la pila — bloqueada: la pila está detenida
- **B6 — S2, S10 — pasos del realm y re-verificación** — ruta: acción del usuario + sonda sin credenciales de esta sesión
- **B7 — S9 — PR de configuración (etiquetas, plantilla, convenciones)** — ruta: inline + `gh` — commit: `chore(repo)`
- **B8 — S9 — issue aprobado y PR de los arreglos** — ruta: `gh` — requiere B7
- **B9 — S12 — verificación independiente del candidato** — ruta: `gentle-ai-verify` (solo lectura)

## Log

- **L1 — Pedido original, verbatim.** «dejé a ún agente ejecutando tareas en playwright y respondió esto» + «puedes hecharle un ojo y le preparamos un plan para arreglarlo porfa». El informe íntegro, que es la fuente de S2–S6:

> FireWatch E2E browser review — results
> Stack was already up (6 containers, page 200). Login round trip executed in a real browser for the first time. Evidence in tmp/review/ (git-ignored).
> CONFIRMED findings
> F1 — Logout does not log out; Keycloak returns 400 (HIGH)
> Did: logged in as demo-admin, clicked "Cerrar sesión".
> Expected: return to http://localhost:4173/, signed out.
> Actual: browser goes to http://localhost:8081/realms/firewatch/protocol/openid-connect/logout?id_token_hint=…&client_id=firewatch-demo&post_logout_redirect_uri=http%3A%2F%2Flocalhost%3A4173%2F → HTTP 400, page says "We are sorry… Invalid redirect uri". It never returns to :4173.
> Root cause: the live realm's firewatch-demo client has no http://localhost:4173/ post-logout URI. The shipped backend/deploy/local/firewatch-realm.json:18 does declare http://localhost:8080/##http://localhost:4173/, but the Keycloak volume is 8 days old and --import-realm never updates an existing realm. This is the already-documented pending step (backend/docs/keycloak-realm-setup.md:43, odd/tasks/frontend/client-proposal-page.md:41-45) — so the frontend code is correct; the realm is stale.
> Second consequence (also confirmed): because RP-initiated logout failed, the Keycloak SSO session survived. Clicking "Iniciar sesión con Keycloak" again returned to :4173 fully authenticated with no password prompt — logout is effectively a no-op.
> F2 — Sidebar bottom is unreachable at laptop heights (HIGH)
> Did: measured #logout / Gateway card at several viewports.
> Actual: the whole .sidebar-bottom sits below the viewport and cannot be scrolled to: viewport 1280×720, 1366×768, 1440×900, 1024×768, 1920×1080.
> A real Playwright click on #logout timed out: "element is outside of the viewport".
> Root cause: frontend/proposal.css:70 — .sidebar { position: sticky; top: 0; height: calc(100vh - 34px); overflow: visible } with .sidebar-bottom { margin-top: auto }. Content is 925 px tall vs clientHeight 686 (1280×720) → 239 px overflow, no internal scroll, sticky element pinned → permanently below the fold.
> Impact: at common heights, mouse users cannot sign out, and the Gateway status card / "Actualizar estado" are also unreachable. (Sign-in still works via the main "Conectar sesión" card.)
> F3 — Login button stays visible while authenticated (MEDIUM)
> After a successful login the "Conectar sesión" card still shows an enabled, visible "Iniciar sesión con Keycloak" button, while the sidebar says "Sesión conectada / Cerrar sesión". Clicking it restarts the OAuth flow.
> F4 — favicon.ico 404 on every load (LOW)
> GET /favicon.ico → 404 (no favicon declared; server serves 3 assets) logs a console error on each load.
> F5 — 503s are a DLQ backlog, and the UI degrades correctly (INFO)
> /v1/risk/status and /v1/jobs/status → 503 {"ready":false,"reason":"dead_letter_backlog", dead_letter depth 3 > max_depth 0}. Data endpoints return 200 normally. The page shows it honestly ("Desconocido: … HTTP 503 · dead_letter_backlog"). Environment issue, not frontend. Polled every 30 s, so the console gains 2 errors/min while degraded.
> VERIFIED CORRECT
> - Login round trip (admin): authorize → PKCE S256 → POST /token 200 → callback → URL cleaned → sessionStorage PKCE transaction cleaned.
> - PKCE: firewatch.pkce.transaction = {state, verifier}, verifier 43 chars; redirect uses localhost on both :8081 and the :4173 redirect URI.
> - Token storage: never in cookies or localStorage; memory only; storage empty after logout.
> - Roles: demo-admin → viewer, admin.
> - Cross-origin: all API traffic only to localhost:8080 / :8081.
> - Map: 38 commune paths; 05109 → zone-shape level-alto zone-selected, fill rgb(199,116,84) matching legend ALTO; 37 neutral; selected stroke 3 px; counter updated to 1.
> - Keyboard: map buttons focusable; Enter and Space both select; aria-pressed follows; group labelled.
> - Degradation: empty history, no-batch, and degraded services all render explicit visible statuses — nothing silent.
> - Job flow: POST /v1/jobs → published → GET /v1/jobs/{id} → latest + history refreshed.
> UNVERIFIED
> - demo-viewer role/permissions — no viewer password provided.
> - Superseded-response races, fail-closed provenance path, expired/401 token handling.
> Disclosures
> - Environment mutation: I published one synthetic batch for zone 05109 (job fdef3c56-…, batch c3b46c1e-…) via the app's own admin "Solicitar lote de la zona". 05109 now shows history 2 and ALTO. Evidence: tmp/review/04-map-after-batch-05109.png.
> - .playwright-mcp/ is untracked and not git-ignored — MCP logs/snapshots land there. Real evidence is in tmp/.
> - Pre-existing drift still stands: frontend/README.md says 40 tests (runs 60); react-migration-plan.md describes the pre-redesign page. Not touched.

- **L2 — Pregunta del usuario sobre la base, verbatim.** «oye esto que estamos trabajando está en main? recuerdo que estuvimos trabajando haciendo cosas ern otra rama no?». Diagnóstico: la rama estaba bien y `main` intacto; lo que estaba desactualizado era **`main` local**, 51 commits detrás de `origin/main`.
- **L3 — Decisión de la dirección del trabajo, verbatim.** «plan rama aparte con issue y pr, para que calce con las skills de gentle para los pr y agregalo al que falta en plan reestructuración»; «yo tengo acceso al realm, como lo arreglo»; «Todo».
- **L4 — Conflicto de política y su resolución.** `odd/PR-CONVENTIONS.md` (commit `e14a62e`) registra que el repositorio **no** adopta el flujo issue-first, no tiene etiquetas `type:*` / `status:approved` ni plantilla, y que adoptarlo «se hace una sola vez, en un PR de configuración». El usuario eligió el **flujo completo de la skill**, que exige ese PR previo: de ahí los dos PRs de S9.
- **L5 — Hallazgo que invalidó el primer plan.** `origin/main` está 51 commits adelante e incluye `24d615c refactor(frontend): complete React migration and retire legacy harness` y los merges hasta el PR #15. La página revisada está retirada: `frontend/proposal.css`, `proposal.js` y `tests/proposal-readback.test.cjs` **no existen** en esa base, y el servidor de `:4173` estaba sirviendo el árbol viejo (el CSS que devolvía era byte a byte el `proposal.css` obsoleto). Los defectos F1–F4, verificados uno por uno contra el árbol React, **sobreviven**. Decisión del usuario tras el hallazgo: «habla con las otras sesiones y vemos que pasa».
- **L6 — Respuesta de la sesión vecina** (`01a140d9-6395`, `tmp/rediseno`, rama `frontend/react-vite`): los tres defectos no son suyos; su PR #3 está **CLOSED**, `mergedAt: null`, `headRefOid: 64193de`, y la rama no es ancestro de `main`; su contenido sustantivo (`readiness.ts`) ya está en `main` por la cadena rediseno-01..09, así que **no hay merge pendiente**; y no toma ninguno de los tres. Aporte adicional: `frontend/react-vite` difiere de `main` en un comportamiento de negocio (rechaza en el borde un `notice` ausente que `main` tolera), registrado en S13 como decisión nueva y separada. También confirmó que el fast-forward era seguro y que reposicionar la rama era lo limpio, que es lo que se hizo.
- **L7 — Re-baseline ejecutado.** `main` avanzado por fast-forward a `a6b1cb2`; la rama anterior (0 commits) borrada y recreada desde esa base. Preservado antes el único archivo que habría bloqueado el fast-forward: la copia local sin versionar de `odd/urgent/rediseno-pagina/reestructuracion-frontend.md`, que resultó ser la **duplicada obsoleta** — `origin/main` ya trae el documento marcado «SUPERSEDIDO — no ejecutar» apuntando a `./app-react-vite.md`. Copia guardada en `tmp/rebaseline-backup/reestructuracion-frontend.local-102.md`. Esto deja **sin efecto** la instrucción de «agregarlo al plan de reestructuración pendiente»: no hay plan pendiente que completar, y los arreglos se registran en la ruta viva, `app-react-vite.md` (task B8 de este documento).
- **L8 — Incidente de entorno.** A mitad del diagnóstico la **pila entera se detuvo** (~3 s, los 11 servicios, eventos `kill`/`stop`/`die` sin `destroy`): no fue un fallo ni un OOM (`OOMKilled=false`), fue una detención deliberada por otro actor. Los volúmenes `firewatch_postgres-data` y `firewatch_rabbitmq-data` siguen ahí, así que **no hubo pérdida de datos**. Ninguno de los comandos de esta sesión muta estado (solo `ps`, `exec rabbitmqctl list_queues`, `events`, `volume ls`, `inspect`, `curl`). Consecuencia: T5 y la re-verificación en navegador quedan bloqueadas hasta que la pila vuelva a estar arriba.
- **L9 — Implementación de B1–B4, con evidencia observada.**
  - **Línea base antes de escribir:** `npm run verify` sobre `a6b1cb2` → exit 0 (lint → typecheck → tests → source gate → build → smoke). `npm install`: 240 paquetes, 0 vulnerabilidades.
  - **B2 (F3), test-first de verdad:** el test se escribió **antes** del arreglo y se observó el rojo: `AssertionError: expected false to be true` en la aserción del botón deshabilitado, con 9 de 10 pruebas del archivo pasando. Tras el arreglo, `tests/views/session.test.tsx` → **10/10 PASS**.
  - **B1 (F2) y B3 (F4) sin prueba automática, y se declara por qué:** F2 es una propiedad de **layout** y jsdom no tiene motor de layout; F4 queda cubierta de forma estructural, porque el *source gate* prohíbe referencias externas y el smoke verifica que el build siga sirviendo el mismo conjunto de activos. Se eligió `data:` precisamente para no tocar la allowlist derivada de `dist/`.
  - **Verificación después de los tres arreglos:** `npm run verify` → exit 0. Detalles observados: el *source gate* informa **36 archivos** revisados y las 9 declaraciones de honestidad presentes; el build informa **2 activos referenciados**, sin cambio respecto de la línea base, que es la prueba de que el ícono no entró a la allowlist; el smoke pasa.
  - **La afirmación sobre el gate se sostiene en su regla, no solo en su salida:** `scripts/check.mjs` §6 prohíbe `(?:src|href)` seguido de comilla y `https?:` o `//` en `.html`/`.css`. Un `data:` URI no coincide, y la corrida lo confirma.
  - **Commits:** `b5c689c` (F2), `dc1cbe4` (F3 con su prueba), `08bdf65` (F4), `5f23f54` (`.gitignore`), `400ea42` (`frontend/README.md`). Ninguno toca `main`.
  - **Pendiente y no declarado como hecho:** la medición en navegador de B1 (la pila está detenida), la aplicación del URI de post-logout en el realm (B6, acción del usuario), el diagnóstico en vivo de la cola muerta (B5) y la verificación independiente (B9).
