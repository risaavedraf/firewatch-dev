# Handoff: recorrido con sesión en navegador

**Para:** la sesión de OpenCode que tiene el MCP `playwright` configurado en `opencode.json`.
**De:** la sesión que arregló el código. **Lo que falta es exactamente lo que esa herramienta puede hacer y esta no** (no usó credenciales del realm).

## Lo primero: tu skill `firewatch-page-review` está desactualizado

`.opencode/skills/firewatch-page-review/SKILL.md` describe la **página vanilla retirada** y afirma que la app React «no existe». Eso fue cierto hasta el PR #15. Hoy es falso, y si lo sigues vas a revisar código que ya no se sirve: la revisión anterior hizo exactamente eso y por eso su informe mezcló hallazgos reales con una página muerta. **Este documento manda sobre esa skill.**

| La skill dice | La realidad verificada |
| --- | --- |
| `frontend/` es una app vanilla sin build ni dependencias | React 19 + Vite + TypeScript; necesita `npm install` y `npm run build` |
| Los archivos son `index.html`, `proposal.css`, `proposal.js` | Los tres **no existen**; el código está en `frontend/src/**` |
| «Do not review a React app; it does not exist» | Es la única app que existe |
| El servidor sirve exactamente tres activos | Sirve `dist/` con la allowlist derivada del documento construido |
| Levántalo con `node frontend/scripts/serve.mjs` | `cd frontend && npm run build && npm run serve` |
| El login nunca se ejecutó de punta a punta | Estaba **roto** (el código que canjea el callback no llegaba al bundle) y **ya está arreglado**; verificar eso es tu tarea principal |
| Pregúntale al usuario las dos contraseñas | Sigue vigente: ver «Credenciales» |

## Estado del entorno, medido por esta sesión

- Pila arriba: `gateway identity jobs postgres rabbitmq risk`, todos en `running`.
- Gateway `http://localhost:8080/health/ready` → **204**.
- `http://localhost:4173` → **200**; el servidor del build **ya está corriendo**.
- Árbol de trabajo: rama `fix/frontend-review-findings`, commit **3187878**, con **todos los arreglos compilados** en `dist/`.
- **El realm sigue sin el URI de post-logout.** Sonda sin credenciales, ejecutada ahora:

| Petición | Resultado |
| --- | --- |
| `logout?client_id=firewatch-demo&post_logout_redirect_uri=http://localhost:4173/` | **400 "Invalid redirect uri"** |
| la misma con `http://localhost:8080/` | **302** |
| `auth?...&redirect_uri=http://localhost:4173/` (login) | **302** (ese sí está registrado) |

**Vuelve a correr esa sonda antes del recorrido.** Determina el resultado esperado del logout: si `:4173` devuelve **302**, el usuario ya aplicó el paso de la consola y el logout debe funcionar; si devuelve **400**, el logout **debe** fallar y eso es un defecto de entorno, no del cliente.

## Qué ejecutar, en orden de valor

1. **Inicio de sesión — el arreglo nuevo y lo más importante.** Hasta ahora nada en `src/` llamaba al canje del código OAuth, así que Vite lo eliminaba del bundle y el login **no podía completarse**: la página te mandaba a Keycloak y no canjeaba lo que volvía. Se arregló con `resumePendingLogin` al arrancar.
   **Criterio:** tras autenticarte en Keycloak vuelves a `http://localhost:4173/`, la tarjeta inferior dice «Sesión conectada», y `sessionStorage.getItem('firewatch.pkce.transaction')` es **null**. Si vuelves y sigues sin sesión, es un defecto real y es el hallazgo más importante que puedes reportar.
2. **Logout con sesión real.** Con sesión activa, `#logout` **ya es visible** (antes estaba oculto sin sesión, y por eso la medición previa se hizo sobre otro control del mismo bloque). Haz clic y compara con la sonda.
3. **Reintento del SSO.** Después del logout, «Iniciar sesión con Keycloak» debe **volver a pedir contraseña**. Si entra sin pedirla, la sesión SSO sobrevivió porque el logout RP-initiated falló.
4. **Alcance del sidebar a 1280×720, 1366×768, 1440×900 y 1024×768, con sesión activa.** `#logout` debe ser alcanzable y **realmente clicable**. El arreglo es `overflow-y` en `.sidebar`; sin él, a 1280×720, 1366×768 y 1024×768 el clic falla por tiempo de espera. Ya hay un A/B que lo demuestra, pero **con el control que siempre está visible**, no con `#logout`: ese clic es el que falta.
5. **Estado del control de inicio de sesión.** Deshabilitado con sesión activa, habilitado otra vez tras cerrarla.
6. **Icono.** Ninguna petición a `/favicon.ico` con 404 ni error de consola por eso; el documento declara un `data:` URI.
7. **`demo-viewer` — sigue sin verificar.** Inicia sesión como `demo-viewer` y abre `#audit`: la vista debe ser **alcanzable** y mostrar el 403 explícito con su `request_id`. Ninguna vista puede quedar oculta por rol.

## Credenciales

Los usuarios `demo-admin` (roles `admin`, `viewer`) y `demo-viewer` (rol `viewer`) existen en el realm. **Sus contraseñas no están en el repositorio**: `backend/scripts/init_env.py` las genera aleatoriamente en `backend/.env` (`DEMO_ADMIN_PASSWORD`, `DEMO_VIEWER_PASSWORD`), que está ignorado por Git.

**Pídeselas al usuario** antes de intentar el recorrido, y **nunca** las imprimas en evidencia, capturas, hallazgos ni mensajes. Sin ellas los pasos 1–5 y 7 quedan bloqueados: márcalos como **no verificados**, nunca como defectos de login.

## Reglas duras

1. Abre **`http://localhost:4173`**, nunca `http://127.0.0.1:4173`. Para el navegador son dos orígenes distintos y solo la forma `localhost` está registrada en Keycloak y en el gateway.
2. **No reinicies ni reimportes el realm.** Nada de `docker compose down -v`: borra el volumen de identidad con sus usuarios, sesiones y credenciales. Es estado persistente y el reimport es *create-only*, no aplica cambios sobre un realm existente.
3. **No modifiques archivos versionados y no hagas commits.** `git status --short` debe quedar igual antes y después.
4. La evidencia va a **`tmp/review/`**, que está ignorado por Git.
5. Reporta **solo** lo que ejercitaste. Lo no verificado se declara no verificado.
6. Si la pila no está arriba, dilo y detente.

## Cómo levantarlo, si hace falta

```sh
docker compose -f backend/docker-compose.yml up -d          # 6 servicios
cd frontend && npm install && npm run build && npm run serve # dist/ en :4173
```

El servidor de `:4173` **ya está corriendo**. Si responde 200, no lo levantes de nuevo.

## Evidencia esperada

- Una captura por altura para el paso 4, con `#logout` visible y el resultado del clic.
- La URL final tras el logout y el contenido de `sessionStorage` antes y después.
- La captura del 403 de `demo-viewer` en `#audit`.
- La salida de la sonda del realm que ejecutaste.
- `git status --short` al final, sin cambios.

## Cómo se relaciona con el trabajo ya hecho

- Rama `fix/frontend-review-findings` (PR #18, issue #17); `main` intacto en `a6b1cb2`.
- **No repitas** lo que ya está verificado: 232 pruebas y los seis gates en exit 0, el bundle contiene el canje del token (antes no), y el A/B del sidebar a cinco viewports. Todo con detalle en [`../../tasks/frontend/browser-review-fixes.md`](../../tasks/frontend/browser-review-fixes.md), sobre todo L10 y L11.
- Si encuentras un defecto nuevo: repórtalo con `ruta:línea` y el paso exacto que lo reproduce. **No lo arregles**: esta entrega está en revisión.
