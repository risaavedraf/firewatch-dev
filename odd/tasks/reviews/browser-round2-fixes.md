# Arreglos de la segunda ronda de revisión de navegador — FireWatch

**Objetivo:** cerrar los tres defectos que la segunda revisión con Playwright confirmó sobre la base real: una sesión que no puede decir que su token fue rechazado, un id de correlación duplicado y en dos formatos, y una barra que afirma lo contrario de la tarjeta de disponibilidad.
**Rama:** `fix/session-copy-and-correlation-id`, creada desde `main` `b0f77f9`. `main` no se toca.
**Estado:** implementación en curso. Dos escritores en superficies disjuntas (`frontend/**` y `backend/services/gateway/**`); cada tarea cierra con su commit de unidad de trabajo.

## Specs

### S1 — Alcance: los tres hallazgos del informe de L1, verbatim

Los hallazgos de L1 son la especificación y se citan textualmente en S2, S4 y S5. Nada fuera de ellos entra en alcance. **Fuera de alcance, explícitamente:** el URI de post-logout del realm (F1/S2 de `frontend/browser-review-fixes.md`, acción del usuario sobre el realm vivo), la cola muerta (F5), la deriva documental de `odd/tasks/README.md` registrada en L5, y cualquier decisión sobre `decodeJWTPayload` (L12 de la ruta anterior).

### S2 — Hallazgo 1: un 401 no puede explicarse; el usuario lee el copy de primera visita (verbatim)

> Un solo 401 en cualquier llamada tira el token, vacía el selector de zonas (38 → 1) y deja el copy genérico de primera visita: «Conecta un token autorizado…». Es decir, el mensaje le dice al usuario que nunca estuvo conectado.

> before: session="Sesión conectada"  opts=38
> after : session="Sin conexión"      opts=1
> alerts: "Conecta un token autorizado para cargar las zonas analizadas."

**Diagnóstico verificado en esta sesión (observado):** `frontend/src/core/api.ts:174-177` llama a `resetSession()` en 401 y `frontend/src/core/session.ts:320-323` publica `{ status: 'signed-out', roles: null }`. `SessionState` (`session.ts:19-24`) no tiene campo de motivo, así que ningún consumidor puede distinguir «se rechazó tu token» de «no hay sesión». Los textos son literales por vista: `MapPanel.tsx:79`, `OverviewView.tsx:52`, `AuditView.tsx:35`, `SourcesView.tsx:51,138`, `JobsView.tsx:42` y `Shell.tsx:145-147`. El comentario de `JobsView.tsx:66-69` («the session callback owns the copy») documenta un canal que el modelo no puede transportar.

**Requisito:** el modelo de sesión expresa el rechazo y las vistas lo dicen. Ningún texto puede afirmar ni insinuar que el usuario nunca estuvo conectado cuando había un token y el servidor lo rechazó.

### S3 — El modelo expresa el rechazo (decisión del usuario, verbatim)

> Campo reason en SessionState (mi recomendación)

Forma acordada: `reason: 'rejected' | null`. Reglas del modelo, que son la especificación de T1:

1. `reason` vale `'rejected'` **solo** cuando el transporte envió un token no vacío y el servidor respondió 401. Un 401 sin token enviado no es «tu token fue rechazado»: baja la sesión sin motivo.
2. `reason` vuelve a `null` en `startLogin()`, en un login completado y en `signOut()` (salida pedida por el usuario: el copy anónimo es el correcto).
3. `publish()` compara también `reason`: un cambio de motivo con el mismo `status` debe notificar a los suscriptores.
4. **El 401 sigue bajando el token.** Es el invariante «401 → reset de sesión» de `odd/urgent/rediseno-pagina/app-react-vite.md` S17; esta ruta cambia la expresión del motivo, nunca la baja del token.

### S4 — Hallazgo 2: id de correlación duplicado y en dos formatos (verbatim)

> simulé el 403 con access-control-expose-headers y el header duplicado que enviará el backend ya corregido, y la página renderiza literalmente:
> SIN PERMISO La API denegó el permiso para consultar el registro de actividad
> (HTTP 403 · forbidden · request_id aabbccddeeff00112233445566778899, aabbccdd-eeff-0011-2233-445566778899).

> O sea: al reconstruir el gateway, el hallazgo B se vuelve visible para el usuario. Los dos arreglos deben ir juntos, no uno.

**Diagnóstico verificado en esta sesión (observado):**

- `backend/services/gateway/internal/handler/gateway.go:60` fija `X-Request-ID` con `fmt.Sprintf("%x", randomID())` → 32 hex, sin guiones.
- `backend/services/jobs/cmd/server/main.go:145-150` y `backend/services/risk/cmd/server/main.go:192-197` vuelven a fijarla **en el upstream** como UUID con guiones (`id.String()`).
- El `ReverseProxy` copia cabeceras de respuesta con `Add`: la respuesta del gateway sale con **dos** valores. El navegador los une con `", "`, `api.ts:174` guarda la cadena unida y `AuditView.tsx:75` la pinta.
- PR #20 (`7adc9b1`, ya en `main`) agregó `Access-Control-Expose-Headers: X-Request-ID`; eso es lo que convierte el duplicado latente en visible. El acoplamiento que el informe describe es real.

**Requisito, y es un solo id con un solo formato:**

1. El gateway genera el id como el **UUID canónico con guiones** que los servicios ya emiten, para que el log del gateway, el log de jobs/risk, el `request_id` del cuerpo JSON y la cabecera expuesta sean el **mismo literal**.
2. La respuesta del gateway expone **exactamente un** valor de `X-Request-ID`, el suyo. El valor del upstream no se agrega: se descarta en `ModifyResponse` para que el `Set` del gateway quede como único valor.
3. El navegador nunca recibe una lista separada por comas.

### S5 — Hallazgo 3: la barra contradice a la tarjeta (verbatim)

> - #readiness-status → role=status, aria-live=polite: «Gateway disponible…» ✅ correcto
> - #update-checked → role=status: «sin verificar, conecta un token para comprobar los servicios» ❌ hardcodeado

> Dos estados contiguos en la misma barra, uno verdadero y otro falso, y el falso está en una región live, así que se anuncia al lector de pantalla. Ya no es cosmético.

**Diagnóstico verificado en esta sesión (observado):** `Shell.tsx:180` es la **única** aparición de `update-checked` en `frontend/src` y es un literal que ningún estado actualiza, mientras `#readiness-status` (`Shell.tsx:128`, `role="status"` + `aria-live="polite"`) informa el estado real del sondeo.

**Corrección a la severidad del informe (observado):** en esta base `#update-checked` es un `<strong>` dentro de `.update-bar` **sin `role` y sin `aria-live`** (`Shell.tsx:174-181`). La única región viva de la barra es `#readiness-status`, que es la verdadera. Así que es texto contradictorio visible, no un anuncio falso al lector de pantalla. Queda pendiente que el informe relea `role` sobre el build servido: si devuelve `status`, lo servido no es este árbol.

**Requisito:** `#update-checked` deja de ser un literal fijo. Dice el estado real que la página conoce (última revisión hecha, o que no se ha podido revisar) y nunca afirma lo contrario de la tarjeta de disponibilidad.

### S6 — Invariantes que sobreviven

De `odd/urgent/rediseno-pagina/app-react-vite.md` S17 y de las convenciones del repositorio: sin gating por rol en el cliente; 401 → reset de sesión; tokens solo en memoria; sin fixtures ni reloj congelado; la página se abre como `http://localhost:4173`; CSP con `connect-src http://localhost:8080 http://localhost:8081`; ningún secreto en variables `VITE_*`; **ninguna aserción existente se pierde en silencio**; un cambio de comportamiento existente conserva su prueba.

### S7 — Entrega: rama, issue aprobado y PR

Decisión vigente del repositorio (`odd/PR-CONVENTIONS.md`, 2026-10-15): issue con `status:approved` primero, exactamente una etiqueta `type:*` en el PR, plantilla `.github/PULL_REQUEST_TEMPLATE.md`, rama `tipo/descripcion`, commits en Conventional Commits sin `Co-Authored-By`.

**Límite de autorización de esta ruta:** entrega la rama y sus commits de unidad de trabajo. Abrir el issue y el PR publica en el remoto y **no** está autorizado por la opción elegida en L2 («Merge y PR siguen siendo tuyos»): queda bloqueado hasta una autorización explícita del usuario, y el merge es decisión suya en cualquier caso. Abrir el issue localmente no es posible porque el flujo adoptado vive en GitHub.

### S8 — Procedencia de la evidencia

«Observado» significa que **esta sesión** ejecutó o inspeccionó la comprobación; «reportado» significa que proviene del informe de L1 o de otra sesión. Está **observado por esta sesión**: la lectura de las seis rutas de código que causan los tres defectos, que el `main` local no tiene deriva respecto de `origin/main`, y el sondeo en vivo que muestra que el gateway **desplegado** responde 401 con CORS y **sin** `Access-Control-Expose-Headers` (contenedor anterior al merge de PR #20). Está **reportado**: las mediciones de Playwright del informe (38 → 1, el render del `request_id` unido, el `role` de `#update-checked`).

### S9 — Límite declarado de la verificación

El id duplicado se prueba con una prueba del gateway contra un upstream de prueba que emite su propio `X-Request-ID` (determinista, en proceso). **No** se verifica en el navegador contra la pila viva en esta ruta, por dos razones: el contenedor del gateway está construido con código anterior a PR #20 y no expone la cabecera, y jobs/risk no publican puertos al host; reproducirlo exigiría reconstruir la pila y usar credenciales del realm. Ambas cosas quedan como acción posterior explícita, no como algo declarado hecho.

## Tasks

- **T1 — S2, S3 — la sesión expresa el rechazo y las vistas lo dicen** — ruta: delegada a `gentle-ai-worker` en `frontend/**`, test-first, con la revisión del padre y una corrección de copy — commit: `f9eb60e`
- **T2 — S4 — un solo id de correlación, un solo formato** — ruta: delegada a `gentle-ai-worker` en `backend/services/gateway/**`, test-first, con el ROJO re-observado por el padre — commit: `69791c1`
- **T2b — S4 — corregir el documento que describía el formato de 32 caracteres y la conservación byte a byte** — ruta: inline — commit: `69791c1`
- **T2c — S4 — re-apuntar las cuatro citas de la matriz de permisos que T2 desplazó** — ruta: inline, con el guard de citas como prueba — commit: `ba95f7b`
- **T3 — S5 — `#update-checked` deja de afirmar lo contrario de la tarjeta** — ruta: delegada con T1 (mismo archivo `Shell.tsx`), test-first — commit: `f9eb60e`
- **T4 — S6 — verificación agregada del padre** — ruta: inline (`npm run verify`, `go test ./...`, `go vet`, sonda viva read-only) — hecho, ver L10
- **T5 — S6 — verificación independiente y decisión de riesgo** — ruta: `gentle-ai-verify` (solo lectura) sobre `main..f9eb60e`, obligada por el plan de `assess`, que devolvió `unassessable` y por tanto trata el candidato como riesgo alto (L11) — en curso
- **T6 — S7 — fila del índice `odd/tasks/README.md`** — ruta: inline — hecho, ver L6
- **T7 — S7 — issue aprobado y PR** — ruta: `gh` — **bloqueada**: publicar en el remoto requiere autorización explícita del usuario (S7)

## Log

- **L1 — Pedido original, verbatim.** El informe completo de la segunda ronda de Playwright, que es la fuente de S2, S4 y S5:

> hicimos un analisis con playwright de la pagina y estos fueron los allasgos
>
> Hallazgo nuevo y serio: un 401 te desloguea en silencio, con el mensaje equivocado
> Probado con login limpio antes de cada caso, inyectando la respuesta en /v1/risk/zones:
> before: session="Sesión conectada"  opts=38
> after : session="Sin conexión"      opts=1
> alerts: "Conecta un token autorizado para cargar las zonas analizadas."
> Un solo 401 en cualquier llamada tira el token, vacía el selector de zonas (38 → 1) y deja el copy genérico de primera visita: «Conecta un token autorizado…». Es decir, el mensaje le dice al usuario que nunca estuvo conectado.
> Por qué pasa — y por qué no es un descuido trivial. Miré el código:
> export interface SessionState {
>   status: 'signed-out' | 'signing-in' | 'signed-in';
>   roles: string[] | null;
> }
> SessionState no tiene campo de motivo. Y JobsView.tsx:68-71 documenta explícitamente el diseño:
> // `null` means "leave the panel as it is": an unauthorized answer already reset
> // the session, and the session callback owns the copy in that case.
> if (failure.kind === 'unauthorized') return null;
> El comentario dice que «el callback de sesión es dueño del copy»… pero el modelo de sesión no puede cargar razón alguna, así que ese copy es por fuerza el anónimo. Borrar el token ante un 401 es correcto y seguro; lo que está roto es que el modelo no sabe expresar «tu token fue rechazado».
> Y el escenario real más probable de 401 es el token expirado: el usuario trabaja, expira, y la página le dice que conecte un token como si fuera un visitante nuevo. El arreglo es chico: un reason en SessionState (p. ej. 'rejected'), o un aviso de un solo uso.
> El contraste que lo confirma como defecto
> Los otros tres caminos están bien:
> Caso    Sesión
> 500    ✅ se conserva
> JSON malformado    ✅ se conserva
> Red abortada    ✅ se conserva
> 401    ❌ se destruye
> La app sabe reportar fallas explícitamente. Simplemente no puede reportar esta.
> Confirmado empíricamente: el id duplicado se pinta sucio
> Antes te lo reporté como hipótesis. Ahora está probado: simulé el 403 con access-control-expose-headers y el header duplicado que enviará el backend ya corregido, y la página renderiza literalmente:
> SIN PERMISO La API denegó el permiso para consultar el registro de actividad
> (HTTP 403 · forbidden · request_id aabbccddeeff00112233445566778899, aabbccdd-eeff-0011-2233-445566778899).
> O sea: al reconstruir el gateway, el hallazgo B se vuelve visible para el usuario. Los dos arreglos deben ir juntos, no uno.
> Hallazgo C, ahora más nítido
> Tenía razón a medias. Con el gateway sano (verifiqué /health/ready = 204 desde la propia página), los dos textos conviven:
> - #readiness-status → role=status, aria-live=polite: «Gateway disponible…» ✅ correcto
> - #update-checked → role=status: «sin verificar, conecta un token para comprobar los servicios» ❌ hardcodeado
> Dos estados contiguos en la misma barra, uno verdadero y otro falso, y el falso está en una región live, así que se anuncia al lector de pantalla. Ya no es cosmético.

- **L2 — Decisión de alcance y de forma, verbatim.** Respuestas del usuario a las dos preguntas de esta sesión:

> Las tres en una ruta ODD con rama aparte
> — Rama + doc de feature + commits por unidad de trabajo. Merge y PR siguen siendo tuyos.

> Campo reason en SessionState (mi recomendación)
> — Estado real, una sola fuente de verdad. Toca ~15 aserciones que comparan el snapshot completo.

  **Corrección de la propia pregunta, medida y no estimada:** no son ~15 aserciones las que comparan el snapshot completo, son **5** (`frontend/tests/core/session.test.ts:104,132,220,235` y `frontend/tests/views/session.test.tsx:172`). El resto de las apariciones de `signed-out` lee `status`, que no cambia. La estimación de la pregunta estuvo alta; el trabajo real es menor.
- **L3 — Base, rama y comprobación de deriva.** `git fetch` y comparación: `main` local = `origin/main` = `b0f77f9`, sin commits en ninguno de los dos sentidos (el incidente de L2 de la ruta anterior no se repite). Rama `fix/session-copy-and-correlation-id` creada desde esa base. Estado del entorno observado para poder verificar: Go 1.27.1, Node 24.11.1, `gh` autenticado, y la pila arriba (gateway, jobs, risk, identity, postgres, rabbitmq).
- **L4 — Implementación delegada en superficies disjuntas.** Dos escritores `gentle-ai-worker` en segundo plano, lanzados juntos, con superficies disjuntas para que puedan escribir en paralelo: **worker A** (`mva4wmjz-1-gwjz`) en `frontend/**` para T1 y T3 (S2, S3, S5), y **worker B** (`mva4wmk6-2-vdn4`) en `backend/services/gateway/**` para T2 (S4). Ambos reciben el documento por referencia, con la instrucción de leerlo hasta `## Log` y de devolver por cada S# la evidencia de ROJO antes del arreglo. **Incidente registrado:** los dos primeros lanzamientos fueron rechazados por el runtime —a los mandatos les faltaba la sección canónica `## Allowed edit surfaces` que el propio runtime exige— y se relanzaron con la superficie declarada como lista de rutas, sin cambiar nada más del mandato. Ninguno de los dos escribe commits: los commits de unidad de trabajo los hace el padre tras verificar (S7).
- **L5 — Deriva documental encontrada y NO tomada.** `odd/tasks/README.md`, última sección, todavía afirma: «este repositorio usa rama más descripción, sin flujo issue-first, sin etiquetas `type:*` y sin plantilla de PR». Es falso desde el 2026-10-15: `odd/PR-CONVENTIONS.md` registra la adopción del flujo issue-first, las seis etiquetas `type:*` y `status:approved` existen en el repositorio y `.github/PULL_REQUEST_TEMPLATE.md` está versionada. Queda fuera de alcance por S1: esta ruta **solo lo reporta**, no lo corrige, porque la corrección pertenece a la ruta que adoptó la política y no a estos tres defectos.
- **L6 — Fila del índice agregada.** Fila de esta ruta añadida a la tabla «Revisiones» de `odd/tasks/README.md`, con enlace, estado, qué está entregado y qué falta, como exige la convención práctica de ese índice (regla 3).
- **L7 — T2 implementado, verificado por el padre y commiteado (`69791c1`).**
  - **El ROJO no se acepta como reportado: se reproduce.** Con `gateway.go` devuelto a `HEAD` por el padre (`git stash push` sobre ese único archivo, corrida, restauración verificada) las pruebas nuevas fallan con `X-Request-ID got 2 values ["13fe8ef9c0a287754e4793aa2873ba4d" "13fe8ef9-c0a2-8775-4e47-93aa2873ba4d"]`: los mismos 16 bytes en dos formatos, que es la causa directa del `request_id … , …` que la página pinta.
  - **Arreglo.** `ModifyResponse` en la closure `proxy(target)` —la que construye los dos handlers, jobs y risk— descarta el `X-Request-ID` del upstream. Como `ModifyResponse` corre **antes** de `copyHeader(rw.Header(), res.Header)` (comprobado por el padre en el fuente de la toolchain local, Go 1.27.1, `net/http/httputil/reverseproxy.go:571-575`), el `Set` del gateway queda como único valor. El id pasa a ser un UUID canónico con guiones.
  - **Decisión del padre más allá de la letra de S4, declarada como tal.** Fijar en `randomID()` los bits de versión 4 y de variante de RFC 9562 **no era necesario** para cumplir S4 (un solo id, un solo formato, el mismo literal): se añade porque el valor ahora se expone al navegador y se cita en tres logs, y llamarlo UUID mientras su nibble de versión es aleatorio es una afirmación que no se sostiene. La prueba pasa a exigir la forma de versión 4 y la variante `[89ab]`. Revertirlo son dos líneas y una expresión regular, si se prefiere el mínimo estricto.
  - **Verificado por el padre, no por el worker:** `go test ./...` EXIT=0, `go vet ./...` EXIT=0, `gofmt -l` sin salida. La fidelidad del helper de la prueba se comprobó en el fuente de `google/uuid` v1.6.0: `encodeHex` formatea los 16 bytes **sin** re-enmascarar versión ni variante, así que el eco de jobs y risk es idéntico y el helper no debilita la aserción de formato, que se apoya en una expresión regular independiente.
  - **Deriva declarada, no re-baselinizada.** `backend/docs/code-explanation/01-gateway.md` afirmaba el formato de 32 caracteres y que «no se garantiza conservación byte a byte del formato entre capas». Ambas frases se corrigieron porque describen la conducta que este commit cambia. Sus **rangos de línea no se re-baselinizan**: `odd/tasks/architecture/gateway-frontend-separation.md` L39 registra que eso exige autorización explícita. Deuda **medida**, no estimada: `gateway.go` pasa de **130 a 156** líneas, `randomID` de **119 a 137** y `failure` de **126 a 152**; `New` no se mueve (23) y `ModifyResponse` queda en 40. Las citas anteriores al punto de inserción no se desplazan; las posteriores sí.
  - **Deriva reportada por el worker y NO tomada, con motivo.** (a) `backend/contracts/openapi/firewatch-v1.yaml` no afirma nada del formato (0 coincidencias): no hay nada que corregir. (b) `tmp/**` está ignorado por git. (c) Las entradas de Log de `playwright-signed-in-round-trip.md` y `signed-in-round-trip-fixes.md`, y la L39 citada, son **observaciones históricas** del valor duplicado, y el repositorio decidió no reescribir Logs. (d) Las citas de este documento (S4 y L1) son la evidencia del defecto tal como el informe lo midió y se conservan como tales: describen el estado **previo**, y la conducta posterior está en este Log y en T2.
- **L8 — T1 y T3 entregados por el worker `mva4wmjz-1-gwjz` y revisados por el padre.** El worker los reportó como **partial**, y no por su propio trabajo: la roja era el guard de citas que mi commit de T2 había desplazado, y en vez de re-baselinizarlo lo escaló. Juicio correcto: la decisión de no re-baselinizar que registré en L7 estaba acotada a los anclajes de prosa del inventario de `code-explanation/`, no a un guard ejecutable. Lo que encontró la revisión del padre, y qué se hizo con cada cosa:
  - **Pérdida real, restaurada.** Al enrutar todo el copy idle por un ayudante que compone la frase, la rama de fallo de `HistoryView` y `SourcesView` pasó de «La API rechazó el token (401). La sesión se cerró; conecta nuevamente.» a una frase que ya no decía qué hacer. La sentencia del módulo vuelve a llevar la instrucción: nombrar el rechazo no es motivo para dejar de decir el siguiente paso.
  - **Pérdida real, NO restaurada y declarada.** `JobsView` perdió «El servidor determina si tienes permiso» de su estado sin sesión. No se restaura: `check.mjs` exige la declaración de que el servidor decide la autorización (`/el servidor/i`), y `JobsView` la sigue llevando en `:252`, `:457` y `:419`; el gate pasó sin esa frase y el copy idle queda consistente con el resto de la aplicación. Es una normalización visible aceptada, no silenciosa.
  - **Normalización visible, declarada.** El copy anónimo del historial ganó la palabra «autorizado». Sus dos pruebas se actualizaron, no se relajaron.
  - **Lo que sostiene S3.** El transporte pasa `rejected` solo si la petición llevó un bearer no vacío (`request.authenticated !== false && token !== ''`), `publish()` compara el motivo, y el 401 sigue bajando el token. Cada afirmación tiene su prueba, incluida la negativa: un 401 sin token deja el motivo en `null`.
  - **Lo que sostiene S5.** `#update-checked` dejó de ser un literal y ahora informa el resultado del último sondeo que efectivamente se asentó; su prueba exige que no afirme éxito cuando la tarjeta ya está en `down` y que no vuelva a decir «conecta un token».
- **L9 — T2c: la deriva que causó mi propio commit, y el alcance real de la regla.** Números medidos: `69791c1` insertó 11 líneas antes del bloque del id y 7 más dentro de él, así que las cuatro citas de `gateway.go` en la matriz de permisos pasaron de 39/80/102/108 a 50/98/120/126. **La primera corrida completa falló (1 de 245) por un error mío, y lo registro como tal:** corregí tres de las cuatro citas porque mi primer barrido llevaba `head -20` y truncó la salida; la cuarta vivía en la fila de `/v1/audit` (`SessionView.tsx:156`). El guard la encontró y paró la cadena, que es su trabajo. La regla que **no** aplica: el gate de S14 cubre los rangos de línea del inventario de `code-explanation/`; un guard ejecutable se re-apunta o la suite sale roja. Precedente idéntico: `e85a11c`, el mismo arreglo cuando PR #20 desplazó `gateway.go`. La prosa sobre la matriz citaba además 74 y 96-105, obsoletas **antes** de esta ruta; ahora cita los mismos anclajes que la prueba verifica.
- **L10 — T4, verificación agregada observada por el padre.** Frontend: `npx vitest run tests/views/session.test.tsx` → 10/10; `npm run verify` → **EXIT=0**, con 24 de 24 archivos y **245 de 245** pruebas, y las seis etapas (lint, typecheck, test, check, build, smoke) en verde; el smoke informa **2 activos referenciados**, sin cambio respecto de la línea base, que es la prueba de que la allowlist derivada de `dist/` no se tocó. Backend: `go test ./...` EXIT=0, `go vet ./...` EXIT=0, `gofmt -l` sin salida. Sonda viva de lectura contra el gateway desplegado: 401 con CORS, **sin** `Access-Control-Expose-Headers` y con el id de 32 hex de antes, lo que confirma que el contenedor es anterior a PR #20 y sostiene el límite declarado en S9. **Lo que esta corrida no prueba:** el camino en navegador y el eco real de jobs y risk en ejecución.
- **L11 — T5: `assess` no pudo producir nivel, y el plan que devolvió.** `gentle_review` con `operation: assess` sobre `main..HEAD` commiteado devolvió `risk: unassessable` y `nativeRisk: unassessable`, con la causa declarada: la evaluación nativa falló porque «untracked files require an explicit declaration»; `outcome_source: unknown`. Su propio plan es el que manda, no mi juicio: `writerSelfVerification: true`, `independentVerifier: true`, con la razón de que RDD está activo y el resultado nativo es desconocido, así que el candidato se trata como riesgo alto y el verificador independiente corre siempre. La verificación independiente se ejecuta por eso y su veredicto se registra aparte. Los archivos sin versionar que bloquean la evaluación nativa no son de esta ruta: `.gentle-ai-default-agent.json` y `opencode.json` son anteriores a ella. Declararlos o ignorarlos es decisión del repositorio, no de este alcance (S1).
