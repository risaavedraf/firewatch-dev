# App real FireWatch: React + Vite + TypeScript sobre el contrato vigente

**Estado: ejecución autorizada.** Rama de trabajo: `frontend/react-vite`, creada desde `f4d99b7` (= `origin/main`, merge del PR #1). `main` no se toca.
**Reemplaza:** `./reestructuracion-frontend.md` (sus tareas R2–R10 y su dirección de forma) y la decisión L3 de `./README.md`. Ambas quedan como historia.
**Base de evidencia del port:** `frontend/docs/react-migration-plan.md` §1 (inventario de capacidades con `ruta:línea`) y §6 (matriz de paridad C1–C21).

## Por qué existe

El usuario corrigió la dirección dos veces. La segunda corrección manda: no se porta la estructura de carpetas del mockup al archivo único vanilla; se **construye la aplicación real en React + Vite + TypeScript**, con las mejores prácticas de la herramienta, y se apunta al backend que ya existe.

## Specs

- **S12 — Cambio de dirección, verbatim (tres mensajes, en orden):**
  1. «puedes trabajar en esta rama» (respuesta a D5: el nombre de la rama queda a criterio de la sesión ejecutora).
  2. «estarías de pana si nos dejamos de mamadas y ya empiezas a trabajar con el react original que necesitamos, ya saca todo lo que hable de cosas academicas y datos no sinteticos, hagamos la app real ya».
  3. «con que hagamos la app real me refiero a que ya empecemos con el vite y React y usemos todas las mejores practicas de desarrollo con react, optimización seguridad y todo, ya sabes que funciona porque tenemos 2 mockups distintos, ahora hagamos el sistema firewatch real, lo más cercano a lo que va a llegar a producción de acuerdo?».
- **S13 — Stack y alcance del backend, decidido por el usuario:** React + TypeScript + Vite con npm. El backend objetivo es **el actual, sin cambios de contrato**: los 8 endpoints del gateway (`http://localhost:8080`), Keycloak (`http://localhost:8081/realms/firewatch`, cliente público PKCE) y `/health/ready`. Además: «crea todo lo real que necesitaría el front de la app» → la arquitectura completa (router, transporte, dominio puro, sesión, diseño), no un prototipo.
- **S14 — Vistas, decidido por el usuario:** 7 vistas, agrupando `#session`+`#permissions` y `#sources`+`#pipeline`; `#pending` («Aún no implementado») con **vista propia y entrada en la navegación**.
- **S15 — Honestidad, decidido por el usuario:** los avisos de demostración sintética / regla académica dejan de ser el titular de la página (franja y banner dominantes) y pasan a **aviso técnico discreto, siempre visible y accesible**, concentrado en la vista de estado de datos. No se borra la declaración: el backend sirve hoy lotes con `demo: true` y `policy_id` académica, y negarlo sería afirmar un riesgo real observado que no existe.
- **S16 — Selector de escenario, decidido por el usuario:** se conserva (`GET /v1/risk/synthetic-scenarios` + `scenario_id` en `POST /v1/jobs`); puede renombrarse el copy.
- **S17 — Invariantes que siguen vigentes (de S10/`./README.md` Tabla B):** toda capacidad real sobrevive; sin gating por rol en el cliente (un `viewer` debe llegar a `#audit` y ver su 403 explícito); 401 → reset de sesión; sin fixtures ni reloj congelado; tokens solo en memoria (la transacción PKCE es lo único en `sessionStorage`, y se declara); la página se abre como `http://localhost:4173`, nunca `127.0.0.1:4173`; CSP con `connect-src http://localhost:8080 http://localhost:8081`; ningún secreto en variables `VITE_*`.
- **S18 — Paridad y red de pruebas:** ninguna aserción de `tests/proposal-readback.test.cjs` se pierde en silencio. Cada aserción se porta a su nuevo hogar (dominio, componente, transporte o smoke) y se demuestra 0 perdidas **antes** de retirar el arnés de VM y la página vanilla.

## Tareas

Cada unidad cierra con un commit de unidad de trabajo en `frontend/react-vite`. Test-first donde hay comportamiento nuevo.

- **N1 — S12 — inline — docs** — este documento, la entrada de supersesión en `./reestructuracion-frontend.md` y la línea base registrada (`npm test` 60/60, `npm run smoke` PASS). `commit: docs`.
- **N2 — S13 — inline** — andamiaje: `package.json` con dependencias fijadas, `vite.config.ts`, `tsconfig.json` (strict), `index.html` de entrada, `src/main.tsx`; `npm run build` y `npm run typecheck` en verde. `commit: unitario`.
- **N3 — S13 — inline** — sistema de diseño: `src/styles/tokens.css` (paleta forest ya aprobada en `1ec9c8c`) y `app.css`; primitivas `ui/`. `commit: unitario`.
- **N4 — S14 — inline, test-first** — router de hash con las 7 vistas, navegación agrupada, normalización de hash inválido y estado activo. `commit: unitario`.
- **N5 — S13/S17 — inline, test-first** — dominio puro: vigencia (expiración 1000 ms antes, tope 3600000 ms), mapeo de niveles, validez de lote (fail-closed, 404 ≠ bajo), procedencia, readiness, filtros del historial. `commit: unitario`.
- **N6 — S13 — inline, test-first** — capa de datos: constantes de origen, cliente HTTP con `Authorization`, `credentials: omit`, `cache: no-store`, descarte de respuestas obsoletas y deadlines. `commit: unitario`.
- **N7 — S13/S17 — inline, test-first** — sesión: PKCE S256 con WebCrypto, `state`/`verifier` en `sessionStorage` bajo `firewatch.pkce.transaction`, token en memoria, rol decodificado **sin verificar firma** y solo para mostrar, logout RP-Initiated. `commit: unitario`.
- **N8 — S14/S15/S16 — delegable, test-first** — las 7 vistas: overview (métricas, mapa real con comunas y recuadros insulares, último lote, factores, justificación, política), historial, fuentes+servicios, procesamiento (selector de escenario conservado), sesión+permisos (matriz citada por `ruta:línea`, sin selector de rol), «Aún no implementado» y auditoría (403 explícito). `commit: unitario` por grupo de vistas.
- **N9 — S18 — inline, obligatorio antes de borrar nada** — portar **cada** aserción del arnés de VM a Vitest y demostrar 0 perdidas; recién entonces retirar `proposal.js`, `proposal.css`, el `index.html` vanilla y `tests/proposal-readback.test.cjs`. `commit: unitario`.
- **N10 — S17 — inline** — servidor estático sobre `dist/` en `:4173` con la CSP real, `no-store`, `nosniff`, `no-referrer`, sin listado de directorios y con confinamiento de rutas; `scripts/check.mjs` reescrito con las reglas invertidas; `smoke.mjs` adaptado al build. `commit: unitario`.
- **N11 — S12 — verificación** — suite completa, build, smoke y guion de recorrido en navegador para el usuario (comuna `05109` con lote, `05101` sin lote, historial `05501`, `demo-viewer` con 403). `commit: ninguno`.

## Log

- **L1** — Cambio de dirección del usuario, verbatim (los tres mensajes de S12), en respuesta a las decisiones D1–D5 del documento anterior. Decisiones derivadas: D1 = 7 vistas agrupadas; D2 = `#pending` con vista propia; D5 = nombre de rama a criterio de la sesión ejecutora; D4 (reparto de `proposal.css`) queda **obsoleta** al reescribirse la página en React.
- **L2** — Línea base reproducida por esta sesión sobre `f4d99b7` antes de cualquier cambio: `cd frontend && npm test` → `tests 60, pass 60, fail 0, skipped 0`, exit 0; `npm run smoke` → `PASS: loopback HTTP serves the three page assets…`, exit 0. Árbol de `f4d99b7` verificado idéntico al del worktree de partida (`git diff --stat f4d99b7 HEAD` vacío).
- **L3** — Respuestas del usuario a las decisiones de bloqueo (verbatim): avisos → «Convertirlos en aviso técnico discreto»; selector de escenario → «Conservar el selector, cambiar de nombre si es necesario»; backend → «Dejalo activo para el backend actual, pero crea todo lo real que necesitaría el front de la app».
