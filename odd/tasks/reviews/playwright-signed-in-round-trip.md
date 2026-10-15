# ✅ Informe del recorrido — Playwright con sesión (2026-10-15)

> **Procedencia de esta publicación.** El informe de abajo lo produjo la sesión de OpenCode con el MCP `playwright` sobre Chrome headless, **no** la sesión que lo publica: cada medición de navegador es **reportada**, con su atribución intacta. Lo que esta publicación verificó es dónde vive y que sus dos afirmaciones documentales citadas (L7 del ledger y el paso 16 del guion) siguen ahí. Su brief de origen es `odd/urgent/rediseno-pagina/handoff-playwright-signed-in.md`, que llega al repositorio con **PR #18**; mientras ese PR siga abierto, las referencias «este handoff» y «§4» de abajo apuntan a ese documento. Su hallazgo H1 y su arreglo son **#19** / **PR #20**.

**Ejecutado por:** la sesión de OpenCode con el MCP `playwright` (Chrome headless, contexto aislado).
**Base:** rama `fix/frontend-review-findings`, HEAD `7dec907`.
**Entorno medido:** pila arriba (`gateway identity jobs postgres rabbitmq risk`, en `running`); gateway `http://localhost:8080/health/ready` → **204**; página `http://localhost:4173` → **200**.
**Reglas respetadas:** siempre `http://localhost:4173`, nunca `127.0.0.1`; no se reinició ni reimportó el realm; no se modificó ningún archivo versionado **durante** la revisión; evidencia en `tmp/review/` (ignorado por git). `git status --short` al terminar: **sin cambios** salvo `?? opencode.json`, que ya existía antes de empezar.

> Nota de alcance: esta anotación se añadió al documento **después** del recorrido, a pedido explícito del usuario. Es la única edición de un archivo versionado que produjo esta sesión; no se hizo ningún commit.

## 1. Sonda del realm (ejecutada antes del recorrido y re-ejecutada al final)

Misma respuesta en ambas corridas; salida guardada en `tmp/review/realm-probe.txt`.

| Petición (sin credenciales) | Resultado |
| --- | --- |
| `logout?client_id=firewatch-demo&post_logout_redirect_uri=http://localhost:4173/` | **400 "Invalid redirect uri"** |
| la misma con `post_logout_redirect_uri=http://localhost:8080/` | **302** |
| `auth?...&redirect_uri=http://localhost:4173/` (login) | **302** |

**Lectura:** el 400 confirma que al cliente `firewatch-demo` del realm vivo le sigue faltando el URI de post-logout de `:4173`. Por lo tanto el logout **debe** fallar; es un defecto de **estado del realm**, no del cliente.

## 2. Resultados por paso

| # | Paso | Veredicto | Detalle |
| --- | --- | --- | --- |
| 1 | Inicio de sesión (el arreglo nuevo) | **PASA** | Tras autenticar en Keycloak se volvió a `http://localhost:4173/`; la tarjeta del sidebar dice **«Sesión conectada»**; `sessionStorage.getItem('firewatch.pkce.transaction')` = **null**; `sessionStorage` y `localStorage` vacíos. |
| 2 | Logout con sesión real | **Observado (defecto de entorno)** | Clic real en `#logout` → `:8081/.../logout?...&post_logout_redirect_uri=http://localhost:4173/` → **HTTP 400 "Invalid redirect uri"**. Idéntico a la sonda. Evidencia: `02-logout-400-realm.png`. |
| 3 | Reintento del SSO tras el logout | **Observado (consecuencia del entorno)** | «Iniciar sesión con Keycloak» volvió a `:4173` **sin pedir contraseña**: la sesión SSO sobrevivió porque el logout RP-initiated falló con 400. |
| 4 | Alcance del sidebar a 4 alturas, con sesión activa | **PASA (el clic que faltaba)** | `#logout` es alcanzable y **realmente clicable** en 1280×720, 1366×768, 1440×900 y 1024×768. Ver detalle abajo. |
| 5 | Estado del control de inicio de sesión | **PASA** | Con sesión: `#login` `disabled=true`, `#logout` visible. Sin sesión: `#login` habilitado, `#logout` `hidden=true`. |
| 6 | Icono | **PASA** | 0 peticiones a `/favicon.ico`, 0 404 por eso, 0 errores de consola en la carga limpia. |
| 7 | `demo-viewer` y `#audit` | **Alcanzable PASA / `request_id` FALLA** | La vista es alcanzable y muestra el 403 explícito; **no** muestra su `request_id` (hallazgo nuevo, ver §4). |

### Detalle del paso 4 (la evidencia que faltaba)

Con `.sidebar { overflow-y: auto }`, `#logout` arranca fuera del viewport y queda dentro tras hacer scroll. Se usó un clic de accionabilidad real de Playwright (`click({ trial: true })`), que ejecuta todas las comprobaciones (visible, estable, recibe eventos, dentro del viewport tras auto-scroll) **sin** disparar el logout; más un hit-test `elementFromPoint` en su centro.

| Viewport | `#logout` antes | trial click | `scrollTop` del sidebar | en viewport | hit-test |
| --- | --- | --- | --- | --- | --- |
| 1280×720 | fuera (`top` 832) | **ok** | 208 | sí | `BUTTON#logout` |
| 1366×768 | fuera (`top` 832) | **ok** | 160 | sí | `BUTTON#logout` |
| 1440×900 | dentro | **ok** | 5 | sí | `BUTTON#logout` |
| 1024×768 | fuera (`top` 844) | **ok** | 172 | sí | `BUTTON#logout` |

Capturas: `tmp/review/step4-logout-1280x720.png`, `step4-logout-1366x768.png`, `step4-logout-1440x900.png`, `step4-logout-1024x768.png`.
**Límite declarado:** el A/B previo se hizo con el control siempre visible (`#readiness-refresh`); aquí se completó con el clic real sobre `#logout`.

### Detalle del paso 7

- `#audit` mostró: **«SIN PERMISO — La API denegó el permiso para consultar el registro de actividad (HTTP 403 · forbidden). Es un endpoint de solo administradores…»**.
- Red: `GET http://localhost:8080/v1/audit` → **403**.
- El usuario `demo-viewer` declara rol **`viewer`** (solo).
- Los **7 enlaces** del nav están presentes (incluido «Registro de actividad»): **ninguna vista oculta por rol**.
- Evidencia: `tmp/review/03-audit-viewer-403.png`.

## 3. Verificado como correcto (aprobado)

- **Round trip de login completo ahora funciona de punta a punta** (antes el código de canje no llegaba al bundle): autorización → PKCE S256 → callback → vuelta a `:4173` → transacción PKCE eliminada.
- **Token solo en memoria:** `sessionStorage`/`localStorage` vacíos; un reload real deja la sesión sin conexión (confirma el invariante).
- **De gradación honesta:** el 404 de `/v1/risk/latest?zone_id=05101` (zona sin lote) se renderiza como **«SIN LOTE — La API no informa lotes publicados para esta zona»**, no como error silencioso ni inventado.
- **Fix del sidebar (`overflow-y`) vigente** y suficiente para que `#logout` sea alcanzable y clicable a las cuatro alturas.
- **Estado de controles correcto** (login deshabilitado con sesión, logout oculto sin sesión).
- **Favicon con `data:` URI** sin peticiones externas ni error de consola.
- **Sin gating por rol en el cliente:** `#audit` es alcanzable para `viewer` (decide el servidor).

## 4. Hallazgos nuevos

### H1 — El 403 de `#audit` no muestra su `request_id` (MEDIUM)

- **Qué:** el mensaje de 403 no incluye `· request_id …`, pese a que el código lo concatena.
- **Dónde:**
  - `frontend/src/views/AuditView.tsx:75` — `const request = failure.requestId !== undefined ? ` · request_id ${failure.requestId}` : '';`
  - `frontend/src/core/api.ts:174` — `const requestId = response.headers.get('X-Request-ID') ?? '';`
  - `backend/services/gateway/internal/handler/gateway.go:59-69` — CORS fija `Access-Control-Allow-Origin`, `Allow-Methods`, `Allow-Headers`, `Max-Age` y `Vary`; **no** fija `Access-Control-Expose-Headers`.
- **Causa raíz:** la respuesta es **cross-origin** (`:8080` desde `:4173`). El navegador solo expone al JS las cabeceras *safelisted* salvo que el servidor declare `Access-Control-Expose-Headers`; el gateway no lo hace (0 coincidencias de «Expose» en `backend/`). Por eso `get('X-Request-ID')` devuelve `null` y el `request_id` nunca se pinta.
- **Prueba observada:** la respuesta cruda trae `x-request-id: 3a2165512429ed5575faae44bc7b944c, 3a216551-2429-ed55-75fa-ae44bc7b944c`; pero desde el JS de la página `fetch('http://localhost:8080/v1/audit').headers` expone solo `cache-control`, `content-length`, `content-type` — `x-request-id` = **null**. En el DOM no aparece la palabra «request» ni ningún UUID.
- **Contradice:** el paso 7 de este handoff y `odd/urgent/rediseno-pagina/guion-recorrido.md:38` / `README.md:104`.
- **Naturaleza:** el cliente lee bien; el arreglo es **del servidor (gateway)**. No se tocó.

### H2 — El logout falla por estado del realm (INFO, ya conocido)

- El 400 del post-logout y la SSO sobreviviente son **F1 / S2** del informe original, confirmados aquí con sesión real. Es defecto de **entorno**, no del cliente: `frontend/src/config.ts` mantiene `redirectURI = 'http://localhost:4173/'` y el cliente arma el logout correcto.

## 5. Discrepancia de expectativa documental

`guion-recorrido.md:38` y `README.md:104` afirman que la auditoría muestra el 403 «con su `request_id`». Hoy eso es **falso** mientras el gateway no exponga `X-Request-ID`. Hay que **arreglar el gateway o corregir la afirmación**; no dejarla como está.

## 6. Evidencia producida (todo en `tmp/review/`, ignorado por git)

- `findings-playwright-signed-in.md` — este informe en crudo.
- `realm-probe.txt` — salida de la sonda.
- `01-signed-in-overview-404.png` — sesión iniciada + «SIN LOTE» (zona sin lote).
- `02-logout-400-realm.png` — el 400 del logout en Keycloak.
- `03-audit-viewer-403.png` — el 403 de `demo-viewer` en `#audit`.
- `step4-logout-<WxH>.png` — 4 capturas, una por altura, con `#logout` visible.

## 7. Manejo de credenciales

Las contraseñas se leyeron de `backend/.env` (ignorado por git) con autorización explícita del usuario y **no** se escribieron en evidencia, capturas, hallazgos ni mensajes. Advertencia honesta: la herramienta de Playwright **imprime el código que ejecuta**, así que los valores sí quedaron visibles en su salida de herramienta (no en artefactos). Los archivos temporales de tecleo se borraron al terminar.

## 8. Recomendaciones para revisar / echar un ojo

1. **Gateway (prioridad, arreglo de H1):** añadir `Access-Control-Expose-Headers: X-Request-ID` en el bloque CORS de `backend/services/gateway/internal/handler/gateway.go:59-69`, con su prueba (recordar S10: los errores 401/403/404 ya deben llevar CORS; sumar aquí el exposé).
2. **Cobertura que hoy falta:** no existe ninguna aserción de navegador que compruebe que el 403 de `#audit` pinta el `request_id` (el `check.mjs` es estático). Vale la pena una prueba dirigida tras el arreglo del gateway.
3. **Documentación:** corregir `guion-recorrido.md:38` / `README.md:104` o arreglar el gateway; no dejar la afirmación sin sustento.
4. **Realm (pendiente del usuario):** aplicar el URI de post-logout `http://localhost:4173/` en el cliente `firewatch-demo`. Hasta entonces, F1 y su consecuencia (SSO que sobrevive al logout) persisten.
5. **UX del logout en realm mal configurado (diseño, no defecto):** cuando el post-logout devuelve 400, el usuario queda en la página de error de Keycloak sin volver a la app. El cliente es correcto, pero conviene decidir si se quiere un mensaje más claro; es decisión de producto.
6. **Consola limpia (opcional):** el 404 de `/v1/risk/latest` para zona sin lote es correcto pero ensucia la consola cada sondeo; si se quiere consola sin errores, el backend podría responder 200 con vacío —decisión de contrato, no urgente.
7. **Automatización futura del login:** `run_code_unsafe` imprime el código ejecutado; si se automatiza un recorrido con credenciales, no incrustarlas en el código (usar almacén de secretos y evitar el eco).
8. **Skill desactualizado:** `.opencode/skills/firewatch-page-review/SKILL.md` sigue describiendo la página vanilla retirada; conviene actualizarlo o retirarlo para que nadie vuelva a revisar código muerto.

## 9. No verificado (fuera del alcance de este handoff)

- Carreras de respuestas supersedidas y manejo de token expirado/401 (arrastrado del informe original).
- La cola muerta (F5): diagnóstico previo; no se purgó ni recargó nada.
- El logout «feliz» (volver a `:4173` cerrado) **no es verificable** hasta que el realm tenga el URI de post-logout; hoy el resultado correcto es el 400.
