# Rediseño de la página: diseño del mockup con la funcionalidad real — FireWatch

Owner: sesión `01a13a74`. **Esta carpeta vive en `odd/urgent/` por pedido explícito del usuario.**
Objetivo: llevar el diseño y el estilo del **mockup retirado** (copia verificada byte a byte en `tmp/gateway-frontend-separation-1791858059/frontend/`) a la página real, **conservando la funcionalidad que ya funciona contra el backend**.
Restricciones: no perder ninguna capacidad actual; lo que no tenga backend se marca **"aún no implementado"** y nunca se simula como funcionando; sin cambios de contrato, backend ni compose; los 44 tests y el smoke deben seguir en verde; sin aprobación científica ni de producción.

## Specs

- **S1 — Petición del usuario, verbatim:** "Quiero conservar su diseño y el estilo de como se hizo, pero con la funcionalidad que tiene la de gateway, lo que falte de backend simplemente que se marque como aún no implementado, si hay cosas de la actual que el backup no lo mencionas y lo dejas en una carpeta aparte en @urgent/ porfa".
  - S1a — conservar el diseño y el estilo de cómo se hizo.
  - S1b — con la funcionalidad de la página real.
  - S1c — lo que falte de backend se marca **explícitamente** como aún no implementado.
  - S1d — lo que la página actual tenga y el backup no, se menciona (Tabla B).
  - S1e — la entrega de esta ruta vive en una carpeta aparte bajo `odd/urgent/`.
- **S2 — Origen, verbatim:** "estoy seguro de que habiamos hecho una nueva versión… y habíamos hecho una versión que estaba bien buena y creo que la borramos". Reconstruido en L2 de este Log: la versión existe y está intacta.
- **S3 — Límites invariables (reescrito en L4):** mensaje honesto de demostración sintética (sin calibración, sin CONAF/SENAPRED), y **la selección de rol del mockup no se porta**: sería un límite de autorización falso. La cláusula «sin push» queda **reemplazada**: el usuario autorizó publicar **solo** la rama `frontend/rediseno-pagina`, con `main` intacto, sin PR automático y sin despliegue.
- **S4 — Rama de frontend, verbatim (L4):** «i want to make the changes in a forntend branch on github please, after any commit to the main branch». Base elegida por el usuario: **desde `main`** (`0ed07e2`), no desde el trabajo previo de la rama de demo.
- **S5 — Alcance del porte, decisión del usuario (L4):** portar **todo lo que ya tenga dato real** —auditoría, métricas, búsqueda y filtro del historial y matriz de perfiles—; lo que no tenga backend se marca «aún no implementado» sin simularlo.
- **S6 — Recorrido y preservación (L4):** el recorrido con sesión real lo ejecuta **el usuario** al terminar el rediseño, con guion entregado por esta sesión; el mockup se preserva en una ruta versionada bajo `odd/urgent/rediseno-pagina/`.

## Inventario del mockup retirado (23 archivos, 608 líneas, verificado byte a byte)

| Pieza | Contenido |
| --- | --- |
| `styles/tokens.css` (20) | Paleta forest (#102c2d/#173c3b/#285e56/#638579), `--canvas #f5f5ef`, `--paper #fffefa`, ink/muted/line/soft, terracota (#a74d34 + soft), oro (#85691e + soft), radio 12px, sombra suave, Inter |
| `styles/app.css` (263) | Sidebar fija 234px, topbar 75px, `update-bar`, métricas, paneles, `overview-grid`, mapa esquemático, chips, tablas, filtros+búsqueda, pipeline, footer, toast, avisos; responsive 1650/1200/1000/760 y `prefers-reduced-motion` |
| `src/components/shell.js` (28) | Sidebar con marca, ficha de ámbito, navegación agrupada con iconos y marcador activo; topbar con breadcrumb, control de perfil y avatar; `#content`; footer con "Restablecer filtros" |
| `src/core/router.js` (11) | Grupos: Prevención (Territorio, Historial), Información (Fuentes y calidad, Actualización de información), Coordinación (Registro de actividad, Perfiles y permisos) |
| `src/main.js` (117) | Reducer propio, router por hash, 6 vistas, toast, restauración de foco, skip-link, Escape para el menú móvil |
| `src/views/*.js` (80) | overview, history, sources, jobs, audit, access |
| Nota del propio mockup | Las vistas de comparación de modelos y acciones preventivas ya se habían retirado allí: no se portan |

## Tabla A — Qué se puede hacer real y qué debe quedar "aún no implementado"

| Elemento del mockup | Respuesta real disponible hoy | Decisión |
| --- | --- | --- |
| Paleta, tipografía, radios, sombras | No aplica (presentación) | **Portar completo** |
| Sidebar agrupada, topbar, breadcrumb, footer, toast | No aplica (presentación) | **Portar**, mapeando a las secciones reales |
| Tarjetas de métricas | Parcial: hay zonas del catálogo, lotes del historial y estado de sesión | Portar con datos reales; lo que no tenga fuente se marca |
| Mapa territorial sectorial | **Real**: `/v1/risk/zones` con 39 comunas y geometría SUBDERE/IGM/INE | Portar el **estilo** sobre la geometría real (proyección propia ya existente) |
| Chips de estado | **Real** para nivel (bajo/medio/alto/desconocido) y vigencia | Portar mapeando a valores reales; `candidate`, `in_progress`, `reviewed`, `closed` **no implementados** |
| Tabla de sectores (prioridad, cobertura, entorno) | Parcial: comuna, nivel, vigencia y lote son reales | "Cobertura %" y "Entorno" **no implementados** (no existen en el backend) |
| Ficha de detalle (Registro, Vigencia, Estado, Referencia, Fuente) | **Real** | Portar; "Cobertura" **no implementada** |
| Factores con barra de proporción | Parcial: el lote real trae `name`, `value`, `known`, `contribution` | Usar `contribution` real; sin dato, sin barra |
| Bloque de incertidumbre | **Real** (equivale a los límites de demostración) | Portar con el texto honesto actual |
| Historial con búsqueda y filtro de vigencia | Parcial: `/v1/risk/history?zone_id=` es **por zona** | Portar búsqueda/filtro sobre los datos reales de la zona; historial de **todas** las zonas **no implementado** |
| Fuentes y calidad con "Revisar"/"Incorporar registros" y hallazgos | Parcial: `/v1/risk/sources` entrega procedencia real **embebida**, sin catálogo de acciones | Portar la procedencia real; acciones y hallazgos **no implementados** |
| "Solicitar actualización" | **Real**: `POST /v1/jobs` (con `scenario_id` opcional) | Portar |
| "Publicar lote", avance manual y reevaluación | **No existe endpoint** | **No implementado** (el servidor decide la publicación) |
| Registro de actividad | **Real**: `/v1/audit` (admin, últimos 100) | Portar con los campos reales |
| Matriz de perfiles y permisos | Parcial: los roles reales vienen de Keycloak (viewer/admin) | Portar la matriz como documentación y mostrar el rol **real** de la sesión; el **selector de rol no se porta** |
| "Restablecer filtros" | Parcial: los filtros reales son de cliente | Portar si no inventa datos |

## Tabla B — Capacidades de la página actual que el backup NO tiene (se conservan)

| # | Capacidad actual | Por qué se conserva |
| --- | --- | --- |
| 1 | Login/logout OIDC PKCE; token en memoria y `sessionStorage` solo para la transacción | Sin esto no hay página: es la única puerta a los datos |
| 2 | Llamadas autenticadas al gateway; 401 → reset de sesión; 403 → mensaje de permiso | Comportamiento de seguridad ya probado |
| 3 | Catálogo real de 39 comunas con geometría real, recuadros insulares, atribución y restricción DIFROL | El mockup usaba sectores ficticios; aquí hay geografía real declarada |
| 4 | Panel de último lote: factores con `value`/`contribution`/`known`, `rationale`, `policy` con hash, `model_version`, `data_status`, `valid_until`, id de lote, aviso de demo | Es la evidencia real que sustenta la demo |
| 5 | Re-evaluación de vigencia con temporizador acotado y guarda de sesgo de reloj | Evita afirmar vigencia vencida |
| 6 | Historial por zona | Trazabilidad real |
| 7 | Panel de procedencia (`/v1/risk/sources`) con integridad y semántica de `source_cutoff_at` | Honestidad sobre el corte de fuente |
| 8 | Estado comprobable del pipeline con su propia cota y vencimiento | Distingue "no verificado" de "verde" |
| 9 | Tarjeta de disponibilidad del gateway (`/health/ready`) | No acredita el pipeline por sí sola, y así se declara |
| 10 | Selector de escenario sintético + creación de trabajo con `scenario_id`, `evaluated_at` del servidor e `Idempotency-Key` | Es la funcionalidad pedida en la ruta anterior |
| 11 | Colores persistentes por comuna con límite declarado | Pedido explícito del usuario (T6 de la ruta anterior) |
| 12 | Franja y avisos honestos de demostración sintética | Requisito de honestidad de todo el proyecto |
| 13 | CSP con allowlist explícita de orígenes + servidor estático con allowlist de archivos y confinamiento de rutas | Superficie de seguridad ya verificada |
| 14 | 44 pruebas frontend en arnés de VM + smoke estático | Es la red que impide que el rediseño rompa comportamiento |
| 15 | Sondeo de trabajo con cota y `MAX_POLL_ATTEMPTS`; correlación zona/trabajo/lote | Evita mostrar datos de otra zona |
| 16 | Descarte de respuestas obsoletas por secuencia (fences anti-carrera) | Evita mezclar respuestas de zonas distintas |

## Tasks

Orden de ejecución. Cada unidad cierra con un commit de unidad de trabajo en `frontend/rediseno-pagina`.

- **T1 — S1/S2 — exploración de solo lectura** — **hecho**: inventario del mockup (23 archivos, 608 líneas) y Tablas A y B de este documento. `commit: ninguno`.
- **T2 — S1a — inline** — fundación visual: tokens (paleta forest, canvas/paper, terracota/oro, radio 12, sombra suave, Inter) y primitivas (panel, chip, tabla, métrica, botón, toast, footer) aplicadas a la página real **sin cambiar comportamiento**; 44 tests + smoke verdes. `commit: unitario`.
- **T3 — S1a — inline** — shell: sidebar agrupada con ficha de ámbito e iconos, topbar con breadcrumb, `update-bar` y pie; el perfil mostrado es **el de la sesión real**, nunca un selector de rol. Todo id existente se conserva. `commit: unitario`.
- **T4 — S1b — inline, test-first** — tarjetas de métricas con dato real (zonas del catálogo, lotes del historial, estado de sesión); sin fuente, se marca. `commit: unitario`.
- **T5 — S1b — inline, test-first** — búsqueda y filtro de vigencia sobre el historial **por zona** ya consultado; el historial de todas las zonas queda «aún no implementado». `commit: unitario`.
- **T6 — S1b — inline, test-first** — panel de auditoría contra `/v1/audit` real (solo admin, campos reales, 403 con mensaje explícito). `commit: unitario`.
- **T7 — S1b/S1c — inline, test-first** — matriz de perfiles y permisos como documentación + **rol real de la sesión** cuando una fuente real lo exponga; si no la expone, se marca «aún no implementado». `commit: unitario`.
- **T8 — S1c — inline** — estados «aún no implementado» para cada elemento de la Tabla A sin backend (cobertura %, contexto por sector, prioridad regional, acciones de revisión/incorporación de fuentes y hallazgos, «Publicar lote»/avance manual/reevaluación, historial de todas las zonas, estados `candidate`/`in_progress`/`reviewed`/`closed`), visibles y no simulados. `commit: unitario`.
- **T9 — S1d — inline** — mantener este documento (Tablas A y B) y `odd/tasks/README.md` al día con cada cambio real. `commit: junto con T2–T8`.
- **T10 — S1b — verificación** — 44 + nuevas pruebas frontend, `npm run smoke`, y guion de recorrido entregado al usuario. `commit: ninguno`.
- **T11 — S4 — entrega** — rama `frontend/rediseno-pagina` publicada en GitHub; `main` intacto; sin PR ni despliegue. `commit: push`.

## Log

- **L1** — Petición recibida, verbatim: "Quiero conservar su diseño y el estilo de como se hizo, pero con la funcionalidad que tiene la de gateway, lo que falte de backend simplemente que se marque como aún no implementado, si hay cosas de la actual que el backup no lo mencionas y lo dejas en una carpeta aparte en @urgent/ porfa".
- **L2** — Origen reconstruido: el usuario recordaba "una versión que estaba bien buena" y creía que se había borrado. Es el **mockup retirado**, y no se perdió: sobrevive en `tmp/gateway-frontend-separation-1791858059/frontend/` con `MANIFEST.md` (23 archivos, huellas MD5 previas al retiro, spec S11 de `odd/tasks/architecture/gateway-frontend-separation.md`). Verificado aquí por comparación `md5sum -b`: **23/23 byte a byte idénticos**. El retiro fue una decisión documentada — no llamaba al backend ni lo servía el gateway — y fue reemplazado por la página real.
- **L3** — Decisión de forma, declarada para que el usuario pueda corregirla: el rediseño se aplica **sobre la misma página** (`frontend/index.html`, `proposal.css`, `proposal.js`) conservando identificadores y comportamiento, porque el servidor estático sirve exactamente tres activos con allowlist y una página paralela duplicaría la lógica de la API. El aspecto anterior queda recuperable en el historial de la rama (commits `6c53a52` y `4216694`). Si el usuario quería dos páginas conviviendo, se mueve a una carpeta propia.
- **L4** — Cambio de instrucción del usuario, verbatim: «i want to make the changes in a forntend branch on github please, after any commit to the main branch». Cuatro decisiones tomadas con él en la misma interacción:
  - **Rama:** crear `frontend/rediseno-pagina` **desde `main`** (`0ed07e2`) y publicarla en GitHub. Reconciliación declarada: partir estrictamente de `0ed07e2` dejaría fuera los dos commits de frontend ya hechos (selector de escenario y colores persistentes), que la Tabla B exige conservar; por eso la rama se creó desde `main` y sobre ella se **cherry-pickearon** `6c53a52` y `4216694`, quedando como «`main` + trabajo de frontend» con árbol idéntico al previo (`git diff 4216694 --stat` vacío). `main` no se toca.
  - **Alcance (reescribe el alcance de T2–T4):** portar **todo lo que ya tenga dato real**, no solo lo visual. Añade panel de auditoría real, métricas reales, búsqueda y filtro del historial y matriz de perfiles. El resto se marca «aún no implementado».
  - **Recorrido:** lo ejecuta el usuario al terminar; esta sesión entrega el guion exacto y qué capturar.
  - **Preservación:** el mockup se copió a `odd/urgent/rediseno-pagina/referencia-mockup/` (30 archivos, incluida la instantánea completa con `gateway-web/` y los dos prototipos HTML); `md5sum -b` de `frontend/` **idéntico 23/23** contra `tmp/`. Deja de existir una única copia ignorada por Git.
