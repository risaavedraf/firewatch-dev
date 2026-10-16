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
- **T7 — S7 — issue aprobado y PR** — ruta: `gh` — hecho: issue **#27** (`status:approved` y `type:bug`) y PR **#28** contra `main` con la plantilla y `Closes #27`; el merge es decisión del usuario (ver L13)
- **T8 — Revisión nativa del candidato** — ruta: `gentle_review` con el lineage del proveedor, sobre el rango commiteado `main..HEAD`, autorizada por el usuario en L13 — en curso

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
- **L12 — T5: veredicto independiente recibido (`mva5ke9t-3-9m5z`), y qué se hizo con lo que encontró.** Veredicto: **5 PASS, 0 FAIL** — S2, S3, S4, S5 y S6 — sobre el rango `main..f9eb60e`. Sus propios conteos: `npx vitest run` 24 de 24 archivos y 245 de 245 pruebas; `npm run verify` con las seis etapas en exit 0 (el gate informa 37 fuentes y 9 sentencias; el build 48 módulos; el smoke 2 activos); `go test ./...` exit 0 y `go test -count=1 -v` con 14 PASS y 0 FAIL; `gofmt -l` sin salida.
  - **La refutación que intentó y no logró, que es lo que da valor al PASS.** Escribió un programa de borrador **fuera del árbol del repositorio** cuyo upstream emite `x-request-id` con dos valores y `X-REQUEST-ID` con uno, por asignación directa al mapa, y lo proxeó con el mismo `ModifyResponse`: la salida final trae **un solo** valor, el del gateway. Confirma además, leyendo el fuente de la toolchain local, que `MIMEHeader.Del` canonicaliza la clave y que `readMIMEHeader` canonicaliza las recibidas, así que la capitalización no puede esquivar el descarte. Y barrió los consumidores reales del formato: solo `frontend/src/core/api.ts:174` y el `uuid.Parse` de jobs y risk, ambos agnósticos a los guiones; ningún script ni workflow lo lee.
  - **Lo que aportó sobre S6, y que solo un tercero podía aportar.** No encontró ninguna aserción eliminada sin reemplazo: las nueve líneas `expect` modificadas se reescribieron en su lugar, las **cinco** aserciones de snapshot completo siguen existiendo y ahora incluyen `reason: null` —quedaron **más** estrictas— y `api.test.ts` pasó de comparar `.status` a comparar el snapshot completo. Eso es exactamente la evidencia que S6 exigía, y la produjo alguien distinto del autor.
  - **Residual 1, corregido (T5b).** `roleDisplay` (`SessionView.tsx:189-192`) mostraba «Sin token en memoria… conecta una sesión para leerlo» con la sesión rechazada: verdadero, pero es **la vista cuyo sujeto es la sesión** y no explicaba el rechazo. Es S2 incompleto, no cosmético. Se corrigió test-first: prueba escrita primero, **ROJO observado** (`expected 'Sin token en memoria: el rol declarad…' to match /rechazó el token/i`, 1 fallida de 11), arreglo después, **enfocado en verde** (11 de 11), y la escalera completa relanzada sobre el resultado. El comentario del código lo dice sin adornos: el panel cuyo sujeto es la sesión nombra por qué no hay ninguna.
  - **Residual 2, NO corregido y declarado.** `MapPanel.tsx:113` conserva «La capa territorial se carga desde /v1/risk/zones al conectar» como texto por defecto sin zona seleccionada. No se cambia: describe de qué depende la capa, no la historia del usuario, y en el mismo panel `map-empty` ya nombra el rechazo (S2 verificado). Añadir la rama del motivo a un lector de coordenadas sería ruido, no corrección.
  - **Cobertura, sin extenderla en silencio.** El veredicto cubre `main..f9eb60e`. El commit de T5b es **posterior** al veredicto y por tanto **no está cubierto por él**: lo cubren la corrida enfocada y la escalera completa del padre. El commit de documentación `b958f73` también es posterior y no toca código.
  - **Lo que el verificador no pudo verificar, declarado por él:** el camino en navegador y el eco real de jobs y risk en ejecución (S9), y el `odd/tasks/README.md` que estaba modificado fuera del rango, hoy ya commiteado. La nota del mapa y el panel de rol no tenían prueba dedicada que fijara su copy tras un rechazo: eso quedó cubierto por la prueba nueva de T5b para el panel de rol, y sigue sin prueba para la nota del mapa, que es el residual 2.
- **L13 — Decisión del usuario y publicación, verbatim.** Ante las dos preguntas de este turno respondió:

> Sí, inicia la revisión nativa del candidato
> Sí: abre el issue y el PR

  Ejecutado en ese orden de importancia, no de comodidad:
  - **Issue #27** con `status:approved` y `type:bug`: los tres hallazgos con sus mediciones, la **corrección de severidad** del hallazgo 3 (no es una región viva en esta base), los criterios de aceptación y el alcance excluido declarado.
  - **Rama publicada** (`git push -u origin fix/session-copy-and-correlation-id`): condición necesaria del PR que el usuario autorizó, no una decisión separada de esta ruta.
  - **PR #28** contra `main`, con la plantilla del repositorio rellenada, **exactamente una** etiqueta `type:bug` y `Closes #27`. El cuerpo declara explícitamente que el veredicto independiente cubre hasta `f9eb60e` y que los dos commits posteriores no están cubiertos por él.
  - **El merge no lo ejecuta esta ruta.**
  - **Lo que la autoridad nativa dijo al ser consultada, antes de empezar:** no hay revisión pendiente, y su proyección de *workspace* ve **cero** cambios —`paths: []`, `base_tree == current_candidate_tree`— porque todo el trabajo está commiteado. Para revisar la rama hace falta un rango commiteado, y su única acción disponible es `start`, precedida por la resolución de la selección de archivos sin versionar. Esos archivos son `.gentle-ai-default-agent.json` y `opencode.json`, anteriores a esta ruta y ajenos a ella.
  - **Registro honesto del orden:** el PR se abrió con la verificación independiente ya recibida, pero **antes** de la revisión nativa, que el usuario autorizó en la misma respuesta. Si la revisión nativa encuentra un bloqueante, la corrección entra como commit nuevo en la misma rama y el PR se actualiza; no se presenta como revisado antes de estarlo.
- **L14 — T8: el intento de revisión nativa, lo que el proveedor dijo, y por qué no ha arrancado.** Autorizada por el usuario en L13. Secuencia observada, sin inventar ningún paso:
  1. **`inspect` inicial.** Su proyección de *workspace* ve **cero** cambios (`paths: []`, `base_tree == current_candidate_tree`), porque todo el trabajo está commiteado: la revisión de esta rama hay que pedirla como **rango commiteado**. La acción disponible es `start`, precedida por la resolución de la selección de archivos sin versionar, y los elegibles son exactamente `.gentle-ai-default-agent.json` y `opencode.json`.
  2. **`inspect` con `untrackedScope: exclude`** → siguiente transición `empty_candidate_base_ref_required`, que confirma la lectura anterior.
  3. **Primer `START`** con `baseRef: main` y `committedOnly: true` → rechazado **sin mutación** (`mutation_performed: false`): `native-start-retained-selection-candidate-mismatch`, porque la selección retenida estaba ligada a la proyección de *workspace* y el rango commiteado es otra proyección.
  4. **`select-intended-untracked`**, la continuación que el propio proveedor nombró → **rechazado porque su `selectionBinding` no es JSON válido**: en su objeto `submission` el arreglo `argumentTokens` nunca se cierra antes de la clave `operationToken`, y el validador marca el error en la posición 885, exactamente ahí. **No se reparó, a propósito:** alterar un binding opaco está prohibido por el contrato del proveedor, y completarlo habría sido inventar autoridad. Se registra como defecto del proveedor, no como error de transcripción: se reprodujo carácter por carácter lo que él emitió.
  5. **Salida por el mecanismo que el propio proveedor recomienda** —textualmente, «to keep a path out of the inventory permanently, ignore it through `.gitignore` or `.git/info/exclude`»—: dos líneas **locales** en `.git/info/exclude` para esos dos artefactos, que no pertenecen a esta ruta ni a ningún candidato. Respaldo del archivo en `.git/info/exclude.bak-before-review`. Nada versionado cambió y los dos archivos siguen intactos. Reversible en una línea.
  6. **Nuevo `inspect`** → sin requisito de selección, otra vez `empty_candidate_base_ref_required`.
  7. **Segundo `START`** → el candidato por fin se resuelve bien: proyección `base-diff`, **24 rutas**, `base_tree 5319ad72` (= `main`). Y termina en un `stop` **terminal** del proveedor sobre sí mismo: `managed_assets_outdated`, con un activo obsoleto (`sha256:390dc4a9…`) y la continuación exacta `C:/Users/rikar/.pi/agent/npm/node_modules/gentle-pi/.gentle-ai/v4.1.1/gentle-ai.exe sync`. Otra vez: **`mutation_performed: false`**, sin lineage.
  8. **Decisión del usuario, verbatim:** «Ejecuta el sync y sigue con la revisión» (opción «sync-and-continue»). Ese comando escribe **fuera del proyecto**, en la instalación del paquete, así que se pidió autorización nombrando la ruta absoluta antes de ejecutarlo; el usuario la dio.
  - **Estado al escribir esto:** el `sync` corre en segundo plano. La revisión nativa **no ha comenzado y no existe lineage alguno**. Todo lo anterior queda registrado como intento fallido con su causa exacta, y el ciclo natural del proveedor tampoco lo declara exitoso: `authority_applicability: unrelated` y `action: start`.
  - **Lo que este intento sí probó, y por eso vale la pena haberlo hecho:** que el candidato se resuelve correctamente como rango commiteado con sus 24 rutas, que el estado de autoridad está limpio (`lineages: 0`, sin conflictos ni candidatos legados) y que el único obstáculo restante es un activo gestionado del propio paquete, ajeno al contenido de la rama.
- **L15 — T8: la revisión nativa queda armada y pendiente de un reinicio de Pi. Cuatro intentos, un error idéntico, una causa raíz observada.**
  - **El `START` sí creó el lineage.** `review-8e28018d0a11714d` sobre `main..d7b2d4b`: tier **medium**, **24 archivos**, **775 líneas**, presupuesto de corrección 200, los cuatro lentes, estado `reviewing`. Su STATUS ligado devolvió `collect` con los cuatro slots del grupo materialize, uno por lente y en orden, sobre la revisión `7663efcf`.
  - **Primera captura del grupo: pronóstico, no corrida.** Transporte `pi_host_relay`, **4 corridas de modelo**, los cuatro lentes, `mutation_performed: false`.
  - **La captura autorizada falló cuatro veces con el mismo error, byte por byte:** `Pi host relay reviewer launch configuration is invalid: no model is configured for review-risk; assign it a model in the agent model routing config`. Siempre con `prepared_reviewers: 0`, `submitted_reviewers: 0` y `mutation_performed: false`: **no se gastó ninguna corrida de modelo ni se mutó la autoridad**.
  - **Causa raíz hallada por observación, no por conjetura.** Se descartaron dos hipótesis con evidencia, no con razonamiento: (a) los modelos que el usuario autorizó y yo asigné en `opencode.json` **sí** persistieron y su JSON es válido, y el `sync` declara explícitamente que «file-backed model/profile values are not runtime assignments»; (b) un segundo `sync` no cambió nada («no managed sync actions needed»). La causa está en `C:/Users/rikar/.pi/agent/subagents.json`: su mapa `model_profiles` definía perfiles para `jd-judge-a`, `jd-judge-b`, `jd-fix-agent`, `gentle-ai-explore`, `gentle-ai-verify` y `gentle-ai-worker`, y para **ninguno** de los seis agentes de revisión. Ese archivo es el enrutamiento real: es el único de la instalación que asigna modelo por agente, y es exactamente lo que el error nombra.
  - **Arreglo aplicado, con autorización explícita del usuario en cada archivo.** (1) `opencode.json` del proyecto (sin versionar): `openai/gpt-6.1-sol` para los seis. (2) `C:/Users/rikar/.pi/agent/subagents.json`, **fuera del proyecto** y autorizado nombrando la ruta absoluta: seis perfiles nuevos con `openai-codex/gpt-6.1-sol` y `effort: low`, que es el literal de proveedor y el esfuerzo que ese mismo archivo ya usa para los revisores adversariales `jd-judge-a`, `jd-judge-b` y `gentle-ai-verify`. El JSON se validó después de editar y el archivo completo se imprimió para revisarlo; quedó con 12 perfiles.
  - **Lo que el cuarto intento demostró, y es el dato que cierra el diagnóstico:** con el perfil ya presente y válido, el error es idéntico. El relevo vivo **no relee** ese archivo: es caché de arranque, no configuración. **Un reinicio de Pi es la única salida que queda**, y es acción del usuario porque termina esta sesión.
  - **Estado entregado al reinicio, para que la revisión se retome y no se reinicie:** el lineage `review-8e28018d0a11714d` y sus cuatro slots viven en `.git/gentle-ai/review-transactions/v2/`. La sesión siguiente debe consultar `gentle_review status` con ese `lineageId` y **volver a pedir los bindings al STATUS**: reenviar los transcritos en este Log está prohibido por el propio contrato del proveedor y además no funcionaría.
  - **Nada de esto afecta al PR #28.** La revisión nativa era la vista externa adicional; la verificación independiente (5 PASS / 0 FAIL sobre `main..f9eb60e`) y la escalera completa en 246/246 ya están registradas y publicadas, y el merge sigue siendo decisión del usuario.
