# Reestructuración del frontend: del archivo único a la estructura del mockup

**Estado: planificación únicamente.** Este documento es el entregable: no cambia código (S9).
**Ejecuta:** una sesión aparte, en una rama aparte, sin tocar `main` (S9). Base: `f4d99b7`.
**Lee además:** `./README.md` — Specs S1–S6, Tablas A y B, Tareas T1–T12, Log L1–L7 de la ruta de rediseño. **Este documento reemplaza la decisión de forma de su L3** («el rediseño se aplica sobre la misma página»).

## Por qué existe

La ruta anterior portó los estilos y el shell del mockup **sobre la página de un solo archivo** (`frontend/index.html` 158 líneas + `proposal.js` 1960 + `proposal.css` 415), conservando la estructura del gateway: tres activos, sin grafo de módulos. El usuario corrigió esa dirección: quería la inversa (S7). El síntoma visible que lo motivó: las nueve áreas conviven en un mismo documento y se recorren con scroll (S8).

Línea base medida hoy sobre `f4d99b7`: `npm test` **60 pass / 0 fail**, `npm run smoke` **PASS**.

## Specs

- **S7 — Estructura de destino, verbatim:** «La idea era que usaras la misma estructura de carpeta que había en el frontend en tmp, usaste los mismos codigos del de gateway, la idea era reescribir gateway en el formato delde tmp/frontend no al revés... deja un archivo odd que arme toda la estructuración de basicamente copiar el tmp/frontend en frontend original y adaptarlo con el index.html y proposal.js y .css».
  Destino: `frontend/` adopta la organización del mockup (`index.html` liviano + `src/**` + `styles/**` + `scripts/**` + `tests/**`), y lo que se adapta a esa organización es el contenido real de `index.html`, `proposal.js` y `proposal.css`.
- **S8 — Áreas por separado, verbatim:** «puedo scrolleando ver toda la info, no por separado».
  Cada área es su propia vista: **una visible a la vez**, elegida por la ruta del hash, como ya lo hacen `routes` y `allowedRoute` del mockup. No se pierde ninguna área: las que hoy existen reciben entrada propia.
- **S9 — Alcance y rama, verbatim:** «genera solo lo que hay que hacer, y luego te mando el id de una sesion que trabajará en otra rama para poder hacer estas modificaciones sin afectar main».
  Este documento es la planificación. La ejecución ocurre en otra rama; `main` no se toca; ningún commit de este documento es un commit de código.
- **S10 — Límites invariables (no negociables, ya declarados en `./README.md`):**
  Tabla B completa (toda capacidad real de la página actual sobrevive); avisos de honestidad visibles (atribución, restricción DIFROL, notas de leyenda, inventario «Aún no implementado»); contratos de endpoints sin cambios; 401/403 reales; **sin gating por rol en el cliente** (S3 de la ruta); la página se abre como `http://localhost:4173` y nunca como `127.0.0.1:4173`; y ninguna aserción de la red de pruebas actual se pierde en silencio.
- **S11 — Invariante de honestidad (razón por la que esto no es copiar y pegar):**
  El mockup es una maqueta sin backend: usa fixtures, reloj congelado, sectores ficticios y **prohíbe en su propio verificador** el vocabulario de demostración y todo `fetch`. La página real hace exactamente lo contrario. Se porta la **estructura de carpetas**, nunca sus datos, sus reglas de copy ni su gating de rol.

## Estructura de destino

| Mockup (referencia-mockup/frontend/) | Destino `frontend/` | Origen del contenido |
|---|---|---|
| `index.html` (20 líneas: `<div id="app">`, `#feedback`, `type="module"`) | `index.html` | Estructura del mockup; los avisos de honestidad y los contenedores reales salen de `index.html` actual |
| `src/main.js` — composición, delegación de eventos, `hashchange` | `src/main.js` | Mockup, adaptado al flujo PKCE y a los cargadores reales |
| `src/core/router.js` — tabla de rutas y grupos | `src/core/router.js` | **9 áreas reales**, sin campo `admin` (S10) |
| `src/core/state.js` — reducer puro | `src/core/state.js` | Transiciones reales; **sin** `SET_ROLE`, `REQUEST_JOB`, `REVIEW_SOURCE` del mockup |
| `src/core/selectors.js` — vigencia, filtros, elegibilidad | `src/core/selectors.js` | Los filtros y conteos de `proposal.js` hoy inline |
| `src/components/shell.js` — sidebar, topbar, update-bar, footer | `src/components/shell.js` | Sidebar del mockup; tarjeta de salud, tarjeta de sesión, `demo-strip` y banner de advertencia salen de la página real |
| `src/components/ui.js` — `escape`, `icon`, `chip`, `metric`, `table` | `src/components/ui.js` | Mockup, **menos** `presentationName` (trampa 6) |
| `src/components/terrain-map.js` — 4 sectores ficticios en SVG | `src/components/map.js` | **Página real**: 38 comunas desde GeoJSON, colores por comuna e insets |
| `src/views/*.js` (6) | `src/views/*.js` (**9**) | Ver mapeo de áreas |
| `src/data/fixtures.js` — reloj congelado y datos inventados | **no se crea** | La página real no inventa datos (trampa 3) |
| — | `src/core/api.js` **(nuevo)** | `gatewayOrigin`, `apiRequest`, `apiRequestStatus`, los 8 endpoints reales |
| — | `src/core/session.js` **(nuevo)** | PKCE, token en memoria, decodificación de `realm_access.roles`, logout |
| `styles/tokens.css` + `styles/app.css` | `styles/tokens.css` + `styles/app.css` | Tokens ya portados en S1a; el resto de `proposal.css` se reparte |
| `scripts/serve.mjs` (allowlist de 23 módulos, `connect-src 'none'`) | `scripts/serve.mjs` | Se conserva el real y **se amplía la allowlist al grafo explícito** |
| `scripts/check.mjs`, `scripts/smoke.mjs` | `scripts/check.mjs`, `scripts/smoke.mjs` | Reescritos con las reglas **invertidas** (trampa 1, 2) |
| `tests/state.test.mjs` (163 líneas, dominio puro) | `tests/domain.test.mjs` | Patrón del mockup aplicado al dominio real |

**Mapeo de áreas a vistas** (las 9 secciones reales de `frontend/index.html`, por línea del archivo actual):

| Vista | Sección actual | Contenido que debe sobrevivir |
|---|---|---|
| `#overview` | `index.html:43` | Métricas reales, mapa, último lote, factores, justificación, política, notas «NO IMPLEMENTADO» |
| `#history` | `:80` | Búsqueda, filtro de vigencia, contador, tabla, nota de historial global no implementado |
| `#session` | `:81` | Botón de inicio de sesión Keycloak, estado de autenticación |
| `#sources` | `:87` | Procedencia real, atribución, restricción, hash, integridad, corte |
| `#pipeline` | `:102` | Estado comprobable de riesgo, jobs y gateway |
| `#jobs` | `:113` | Selector de escenario, solicitud de lote, estado del trabajo |
| `#pending` | `:117` | Inventario «Aún no implementado» — **hoy no tiene entrada en la barra lateral: debe tenerla** |
| `#audit` | `:123` | Registro real, 5 columnas, 403 explícito para no administradores |
| `#permissions` | `:136` | Matriz citada por `ruta:línea` y rol declarado por el token |

## Trampas: lo que NO se copia tal cual

1. **El verificador del mockup prohíbe el vocabulario que la página real debe mostrar.** `referencia-mockup/frontend/scripts/check.mjs:30,34` falla ante `demo|demostración|sintétic*|escenario|fixtures`. La página real dice «ENTORNO DE DEMOSTRACIÓN», «DEMO SINTÉTICA» y «Escenario sintético». Copiarlo tal cual falla; "arreglarlo" borrando los avisos rompe S10. Se reescribe invertido: **exige** los avisos y **prohíbe** los fixtures.
2. **`check.mjs:49` exige que NO haya red:** `assert.doesNotMatch(source, /\b(?:fetch|XMLHttpRequest|WebSocket)\s*\(/)`. La página real llama a 8 endpoints. Esa regla debe invertirse.
3. **`src/data/fixtures.js` no se porta.** Reloj congelado `2026-01-15T15:00:00Z`, sectores `s1`–`s4`, historial y evento de auditoría inventados. La página real tiene 38 comunas y lotes reales; inventar es exactamente lo que S10 prohíbe.
4. **El gating por rol no se porta.** `src/core/router.js` marca `admin: true` en rutas, `shell.js` filtra la navegación por rol y `state.js` implementa `SET_ROLE`. La ruta ya lo declaró: sería un límite de autorización falso. Un `viewer` debe **poder** llegar a `#audit` para ver su 403.
5. **El mapa del mockup no sirve.** `terrain-map.js` dibuja 4 sectores ficticios con trazados fijos. El real dibuja comunas desde `/v1/risk/zones` con colores persistentes por comuna e insets insulares fuera de posición geográfica.
6. **`ui.js: presentationName` inventa nombres.** Traduce `scenario-v1` → «Bosque · versión 01» y el actor sintético. La página real muestra identificadores opacos (`actor_id`, `job_id`, `trace_id`) y declara que no los adorna.
7. **Cuidado con el nombre `tmp/frontend`.** El original estaba en `tmp/gateway-frontend-separation-1791858059/frontend/` (ignorado por git). La copia versionada y verificada por MD5 es `referencia-mockup/frontend/` (23 archivos). `tmp/rediseno/` es **otra cosa**: un worktree de la rama de rediseño, ya superado por el merge.
8. **La red de 60 pruebas no sobrevive sola.** `tests/proposal-readback.test.cjs:40` ejecuta `proposal.js` **como un solo archivo de texto** en una VM con un DOM de juguete (`:20`) que solo entiende `getElementById` y `querySelector('.history-panel'|'.health-dot')`. Un grafo de módulos no carga así. Sin R8, Tabla B queda sin red.
9. **La allowlist de 3 activos es deliberada, no un accidente.** `frontend/scripts/serve.mjs:7-9` dice «three assets, no bundler and no module graph». Se amplía a un conjunto explícito de rutas de módulo; nunca a un directorio.
10. **La CSP real difiere.** El mockup sirve `connect-src 'none'`; la página real necesita `connect-src http://localhost:8080 http://localhost:8081`. Y se abre como `localhost`, no `127.0.0.1`: el reino y el gateway registran solo esa forma.

## Tareas

Cada unidad cierra con un commit de unidad de trabajo en la rama nueva. Test-first donde hay comportamiento nuevo (RED observado antes de tocar fuente).

- **R1 — S7/S8 — inline** — **hecho por este documento**: árbol de destino, mapeo de las 9 áreas, trampas y decisiones abiertas. `commit: ninguno`.
- **R2 — S10 — inline** — **línea base antes de tocar nada**: `npm test` (60/60) y `npm run smoke` (PASS) en la rama nueva, y extraer la lista de aserciones e identificadores que deben sobrevivir (0 perdidos). Sin esto, R8 no tiene contra qué comparar. `commit: docs`.
- **R3 — S7 — inline** — crear el árbol de carpetas y mover contenido **sin cambiar comportamiento**: `index.html` queda como entrada liviana, `proposal.css` se reparte en `styles/tokens.css` + `styles/app.css`, `proposal.js` se mueve sin partirse y `serve.mjs` amplía su allowlist a las rutas nuevas. Criterio de salida: `smoke` PASS y la página carga igual. `commit: unitario`.
- **R4 — S8 — inline, test-first** — `src/core/router.js` con las 9 vistas, `src/main.js` con `hashchange` y normalización de hash inválido, y **una sola vista visible a la vez**; se añade la entrada de navegación que hoy le falta a `#pending`. `commit: unitario`.
- **R5 — S8 — inline, test-first** — extraer el dominio puro a `src/core/state.js` y `src/core/selectors.js` (filtros del historial, vigencia, conteos, estado de sesión) y cubrirlo con `tests/domain.test.mjs` al modo del `tests/state.test.mjs` del mockup. `commit: unitario`.
- **R6 — S8 — inline, test-first** — extraer la E/S a `src/core/api.js` (los 8 endpoints, `apiRequest` vs `apiRequestStatus`) y `src/core/session.js` (PKCE, token, rol decodificado **sin** verificar firma, logout). `commit: unitario`.
- **R7 — S8 — delegable, test-first** — partir `src/views/*.js` por área sobre `components/ui.js`, conservando cada aviso de honestidad dentro de su vista y las citas `ruta:línea` de la matriz de permisos. `commit: unitario`.
- **R8 — S10 — inline, obligatorio antes de borrar nada** — reconstruir la red de pruebas: portar **cada** aserción del arnés VM a su nuevo hogar (dominio, marcado o smoke), demostrar el conteo 0 perdidas y recién entonces retirar el arnés viejo. `commit: junto con R5–R7 o el suyo`.
- **R9 — S11 — inline** — reescribir `scripts/check.mjs` con las reglas invertidas (exigir avisos de honestidad, prohibir fixtures y reloj fijo, exigir los endpoints reales) y adaptar `scripts/smoke.mjs` a la nueva allowlist. `commit: unitario`.
- **R10 — S10 — verificación + recorrido del usuario** — suite completa, `smoke`, y el recorrido en navegador sobre la estructura nueva con los valores reales ya verificados (comuna `05109` con lote, `05101` sin lote, historial `05501`, `demo-viewer` con 403). `commit: ninguno`.

## Decisiones abiertas para la sesión ejecutora

- **D1 — ¿9 vistas o menos?** El mockup tenía 6 y las áreas reales son 9. Se pueden agrupar (`#session` con `#permissions`, `#pipeline` con `#sources`). Mi recomendación: mantener las 9, porque agrupar esconde avisos de honestidad que hoy están en paneles distintos.
- **D2 — ¿`#pending` con vista propia o al pie de `#overview`?** Recomendación: vista propia con su entrada; es el inventario que la transparencia exige visible y hoy no tiene acceso directo.
- **D3 — ¿Se conserva el contrato de marcado del arnés VM?** Recomendación: conservar los identificadores que la tabla del paso 3 de S8 enumera y migrar las aserciones, no el mecanismo.
- **D4 — ¿`styles/app.css` se fusiona o se rescribe?** Recomendación: repartir `proposal.css` entre tokens y layout, sin rediseñar lo ya aprobado en S1a.
- **D5 — Rama.** Propuesta: `frontend/reestructura-modular`, creada desde `f4d99b7`. La decisión del nombre es del usuario.

## Log

- **L1** — Petición del usuario, verbatim (dos mensajes, en orden): «si pero mira C:\Users\rikar\AppData\Local\Temp\orca-paste-1791995562923-3c849d29-b7eb-4b04-9be5-922149542943.png ahi puedes ver que puedo scrolleando ver toda la info, no por separado» y «La idea era que usaras la misma estructura de carpeta que había en el frontend en tmp, usaste los mismos codigos del de gateway, la idea era reescribir gateway en el formato delde tmp/frontend no al revés... deja un archivo odd que arme toda la estructuración de basicamente copiar el tmp/frontend en frontend original y adaptarlo con el index.html y proposal.js y .css genera solo lo que hay que hacer, y luego te mando el id de una sesion que trabajará en otra rama para poder hacer estas modificaciones sin afectar main».
- **L2** — Corrección de la decisión de forma de la ruta anterior. `./README.md` L3 declaró aplicar el rediseño sobre la misma página «conservando identificadores y comportamiento, porque el servidor estático sirve exactamente tres activos con allowlist y una página paralela duplicaría la lógica de la API», y dejó escrito que el usuario podía corregirlo. El usuario lo corrigió: la restricción se resuelve ampliando la allowlist al grafo explícito, no conservando el archivo único.
- **L3** — Contexto de la verificación previa a este documento: el PR #1 de la ruta anterior está **mergeado** (`f4d99b7`, 2026-10-14T16:03:30Z) y `main` local quedó sincronizado. Hay tres archivos con cambios sin commitear en `odd/urgent/` (`README.md`, `02-pruebas-datos.md`, `03-pruebas-modelo.md`) que pertenecen a la auditoría de esa carpeta y **no** a esta reestructuración: la sesión ejecutora no debe mezclarlos en sus commits.
- **L4** — Línea base medida sobre `f4d99b7`: `frontend/` = 4 archivos runtime y de prueba (158 + 415 + 1960 + 1176 líneas), `npm test` 60 pass / 0 fail, `npm run smoke` PASS. Mockup de referencia: 23 archivos, verificados por MD5 contra el respaldo previo a la separación (ver `./referencia-mockup/MANIFEST.md`).
- **L5 — SUPERSEDIDO.** El usuario corrigió la dirección antes de que R2–R10 empezaran: en vez de adaptar el contenido real a la organización del mockup dentro del archivo único, se construye la **aplicación real en React + Vite + TypeScript** sobre el contrato del backend vigente. Verbatim: «estarías de pana si nos dejamos de mamadas y ya empiezas a trabajar con el react original que necesitamos, ya saca todo lo que hable de cosas academicas y datos no sinteticos, hagamos la app real ya» y «con que hagamos la app real me refiero a que ya empecemos con el vite y React y usemos todas las mejores practicas de desarrollo con react, optimización seguridad y todo, ya sabes que funciona porque tenemos 2 mockups distintos, ahora hagamos el sistema firewatch real, lo más cercano a lo que va a llegar a producción de acuerdo?».
  Las tareas **R2–R10 quedan obsoletas** y su plan completo se reemplaza por `./app-react-vite.md` (tareas N1–N11), que conserva de este documento: la Tabla B, el mapeo de las 9 áreas reales, el inventario de trampas (`./README.md` Tabla A/B y §Trampas) y la obligación de portar cada aserción del arnés antes de retirarlo. R1 (este documento) sigue siendo el entregable de planificación de la ruta anterior; ninguna de sus restricciones de forma —servir tres activos, conservar el archivo único— sobrevive al cambio de dirección. `commit: ninguno`.
