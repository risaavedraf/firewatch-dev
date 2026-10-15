# Separación gateway / frontend

Objetivo: la página que hoy embebe el gateway pasa a vivir en `frontend/` como aplicación propia en `:4173`, y el gateway queda como API pura en `:8080`.

Estado: **planificación cerrada, implementación no iniciada.** Arquitectura y alcance decididos por el usuario en cuatro preguntas de esta sesión.

Decisión clave: separación **de runtime**, no solo de carpetas. El gateway deja de servir HTML y necesita CORS explícito. Es la opción de mayor riesgo del par evaluado; se elige con consecuencia aceptada.

---

## Specs

### S1 — La web del gateway vive en `frontend/`

> "hay que hacer que la web de gateway esté en su propia carpeta que será la forntend/"

`frontend/` es "su propia carpeta". Los tres activos servidos hoy por el gateway (`index.html`, `proposal.css`, `proposal.js`) y su suite (`proposal-readback.test.cjs`) se trasladan ahí. No se renombran los activos: `proposal.css` y `proposal.js` conservan su nombre.

### S2 — `frontend/` pasa a ser la página real

> "mi idea principal era tener el mockup para ir haciendo la pagina real"

> "Se reemplaza: frontend/ pasa a ser la página real"

Los 23 archivos de la maqueta se retiran. Nada de la maqueta queda como superficie activa.

### S3 — Frontend y backend separados en runtime

> "se supone que frontend y backend van por separado, así es como tiene que funcionar"

```text
frontend/  ->  http://localhost:4173   (app, su propio servidor)
backend/   ->  http://localhost:8080   (solo /v1/*)
```

La página deja de usar rutas relativas: toda llamada al gateway usa origen absoluto.

### S4 — El gateway deja de servir HTML

> "Separación real: el gateway deja de servir HTML"

El usuario eligió explícitamente esta opción sobre "Separación de carpetas: el gateway sirve una copia". Se retiran `//go:embed`, `proposalAssets`, `fs.Sub`, `mux.Handle("/")` y la allowlist de `/`, `/proposal.css`, `/proposal.js` en `gateway.go`. El gateway responde **404** en `/`.

### S5 — Consecuencias aceptadas, enumeradas por el usuario

> "Hay que tocar:
> - Quitar `go:embed` y `mux.Handle("/")` en `gateway.go`
> - **CORS** en el gateway para el origen `:4173`
> - CSP del frontend con `connect-src` al gateway
> - Redirect URI de Keycloak: `:8080/` -> `:4173/`
> - Mover `TestGatewayUsesKeycloakRPLogout` y `TestGatewayKeepsPrototypeFilesPrivate`
> - Los 3 assets salen de `internal/handler/web/`"

Es alcance autorizado, no propuesta.

### S6 — Riesgo aceptado

> "**Riesgo: item 2** (CORS y guardas de autorización) **+ item 3** (`/` deja de existir)."

Verificación independiente obligatoria (regla de verificación, item 2).

### S7 — Pérdida aceptada

> "Ya no se puede abrir la demo con solo `docker compose up`."

El README raíz y el de `backend/` deben dejar de prometer un solo comando. `docker compose up` levanta la API; la página se abre aparte.

### S8 — Restricción técnica vinculante

`go:embed` no puede alcanzar fuera de su paquete: Go rechaza `..` en los patrones. Por eso S1 no admite un `move` y S4 es la única salida que deja una sola copia.

### S9 — Origen cruzado explícito, nunca comodín

El gateway solo emite CORS para un origen exacto, tomado de configuración. Sin `*`, sin reflejar el `Origin` recibido sin validar, sin comodín con credenciales. Ausencia de configuración = sin cabeceras CORS (fail-closed).

### S10 — Preflight antes de límite y autenticación

`OPTIONS` se responde antes del limitador de tasa y antes de la verificación de token. Las respuestas de error (`401`, `404`, `429`, `502`) también llevan cabeceras CORS; de lo contrario el navegador oculta el error real.

### S11 — Respaldo obligatorio antes de borrar

El repositorio **no tiene ningún commit** (`master` sin commits): sobrescribir o borrar es irreversible. Antes de retirar cualquier archivo de la maqueta o de `internal/handler/web/`, se copia el estado actual a un respaldo y se reporta la ruta y los hashes.

### S12 — Sin commits

`Safety` prohíbe commitear sin pedido explícito del usuario, y no lo hubo. El cierre de cada tarea registra su evidencia en este documento en lugar de un commit.

### S13 — Ninguna tarea escribe en archivos `.env` ni variantes

El repositorio prohíbe explícitamente leer `.env` y sus variantes: `backend/docs/code-explanation/README.md:143-144` los marca como *Excluido | Política de no lectura de archivos `.env` y variantes*, y lo repite para `services/gateway/.env.example` (62), `services/jobs/.env.example` (80) y `services/risk/.env.example` (118).

Por eso `WEB_ORIGIN` **no** se documenta en ningún `.env.example`. Un añadido a ciegas —escribir sin poder leer, con estado de fin de línea y claves existentes desconocidos— sería una escritura no verificable y no se autoriza.

La configuración funcional vive en `backend/docker-compose.yml`, que es un archivo legible y versionado, y la descubribilidad en `backend/README.md`. Queda como spec permanente para cualquier trabajo futuro sobre este repositorio.

### S14 — El manifiesto de `code-explanation/`: solo se corrige lo que esta feature invalida

`backend/docs/code-explanation/verify_structure.py` es una verificación reproducible: lee la tabla de inventario de `README.md`, compara el rango declarado de cada archivo contra sus líneas reales (`verify_structure.py:76`) y con `--snapshot` regenera `source-snapshot.json`.

**Corrección de exactitud — error del coordinador.** La primera versión de esta spec afirmaba que el verificador fallaba por "4 rangos". **Era falso.** Leí la salida con `tail -8` y tomé las líneas visibles como el total. La medición completa, sin truncar, da **16 rangos obsoletos y 9 archivos sin inventariar**. Regla que se deriva y se fija: un conteo de errores nunca se declara desde una salida truncada.

**Alcance de esta feature.** Solo se corrige lo que esta feature invalida o introduce:
- `services/gateway/cmd/server/main.go` `1-61` → `1-63`: T2 agregó la lectura de `WEB_ORIGIN`.
- Retiro de las 5 filas de inventario de `internal/handler/web/**`, borrado por T4. Dejarlas produce `Missing inventory file`.
- Alta de `docs/keycloak-realm-setup.md` con estado `Contexto`: documento nuevo de T6.
- Regeneración de `source-snapshot.json` con `--snapshot`.
- De paso y reportados aparte: `docker-compose.yml` `1-194` → `1-195` (T2) y los rangos de `gateway.go` y `gateway_test.go` que T4 invalidó.

**Deuda declarada, fuera de alcance.** Quedan **12 rangos obsoletos y 7 archivos sin inventariar**, todos en archivos que esta feature nunca tocó. Se declaran con evidencia exacta en lugar de absorberse en silencio, porque inventariar archivos de trabajo ajeno obliga a decidir un estado documental que no pertenece a esta tarea:

```text
12 rangos:   services/jobs/** (5) y services/risk/** (7)
7 sin inventariar: services/jobs/** (2), services/risk/** (4), contracts/gen/geo/valparaiso-comunas.provenance.json (1)
```

Verificación de atribución, no afirmación: tras corregir lo propio, filtrar los errores restantes contra `gateway|frontend|deploy|keycloak` **no devuelve ninguno**. El verificador sigue en rojo por deuda previa y esta feature no la empeora.

Regla permanente, **reforzada tras violarla dos veces**: quien borra o cambia un archivo inventariado actualiza el inventario, regenera el snapshot **y vuelve a ejecutar el verificador al cerrar esa tarea**. No basta con haberlo ejecutado tras la tarea anterior: cada tarea que toca un archivo inventariado invalida la medición previa. Un conteo de errores nunca se declara desde una salida truncada.

### S15 — La página debe poder leer el identificador de correlación

`X-Request-ID` **no** es una cabecera de respuesta *safelisted* de CORS. En una respuesta cross-origin el navegador solo la entrega al JavaScript de la página si el servidor la nombra en `Access-Control-Expose-Headers`. El gateway ya devolvía el identificador, pero hasta esta spec **no lo exponía**, así que `response.headers.get('X-Request-ID')` devolvía `null` en la página y el 403 de `#audit` perdía su `request_id` **aunque el servidor lo hubiera enviado**. La causa estaba enteramente en el servidor: el cliente lee bien (`frontend/src/core/api.ts:174`) y `X-Request-ID` es la única cabecera de respuesta no safelisted que la página lee.

La obligación **se suma a S10**, no la reemplaza: toda respuesta cross-origin del origen configurado declara `Access-Control-Expose-Headers: X-Request-ID`, incluidas las de error (`401`, `403`, `404`, `429`, `502`), y **un origen no configurado sigue sin recibir ninguna cabecera CORS**, esta incluida. Como todo valor CORS de este gateway, se nombra explícitamente: nunca comodín.

Hallazgo y medición: `odd/tasks/reviews/playwright-signed-in-round-trip.md`. RED→GREEN propio en `TestGatewayExposesRequestIDToThePageFromConfiguredOrigin`, con el fail-closed extendido en `TestGatewayRejectsPreflightFromUnconfiguredOrigin` y `TestGatewayFailsClosedWithoutConfiguredOrigin`.

---

## Tasks

| ID | Qué | Specs | Ruta | Estado | Commit |
| --- | --- | --- | --- | --- | --- |
| T1 | Trasladar la página a `frontend/`, retirar la maqueta con respaldo | S1, S2, S11, S12 | inline (coordinador) | cerrada | n/a (S12) |
| T2 | Gateway: CORS con origen exacto y preflight | S9, S10, S12 | escritor delegado | cerrada | n/a (S12) |
| T3 | Página: base de API absoluta y orígenes OIDC | S3, S12 | escritor delegado | cerrada | n/a (S12) |
| T4 | Gateway: dejar de servir HTML y retirar los assets embebidos | S4, S5, S7, S14 | escritor delegado + borrado por el coordinador | cerrada | n/a (S12) |
| T5 | Servidor local del frontend y CSP con `connect-src` | S3, S5, S12 | inline (coordinador) | cerrada | n/a (S12) |
| T6 | Realm de Keycloak y configuración de entornos | S5, S7 | inline (coordinador) | cerrada | n/a (S12) |
| T7 | Documentación: README de frontend, arquitectura, README raíz | S7, S12, S13 | inline (coordinador) | cerrada | n/a (S12) |
| T8 | Verificación independiente | S6 | verificador | en curso | n/a (S12) |
| T9 | Reparar el inventario que T7 invalidó | S14 | inline (coordinador) | cerrada | n/a (S12) |
| T10 | Corregir los 3 defectos de documentación que T8 encontró | S7, S13, S14 | inline (coordinador) | cerrada | n/a (S12) |
| T11 | Re-verificación independiente tras las correcciones | S6 | verificador nuevo | cerrada | n/a (S12) |
| T12 | Corregir los 3 defectos que T11 encontró | S7, S12 | inline (coordinador) | cerrada | n/a (S12) |

### Detalle y aceptación

**T1 — Trasladar la página a `frontend/`, retirar la maqueta**
- Superficie: `frontend/**`.
- Entra: `index.html`, `proposal.css`, `proposal.js`, `tests/proposal-readback.test.cjs` (con rutas de lectura ajustadas al nuevo layout).
- Sale: los 18 archivos de maqueta restantes (`src/**`, `styles/**`, `scripts/check.mjs`, `scripts/smoke.mjs`, `tests/state.test.mjs`) y el `README.md` de la maqueta (se reescribe en T7).
- Aceptación: `frontend/` contiene la página real; ningún archivo de maqueta queda activo; respaldo externo con ruta y hashes reportados.
- Chequeo: respaldo verificado por hash antes de borrar; `node --test frontend/tests/proposal-readback.test.cjs` ejecuta desde el nuevo layout.
- Dependencias: ninguna. **Primera tarea.**

**T2 — Gateway: CORS con origen exacto y preflight**
- Superficie: `backend/services/gateway/internal/handler/gateway.go`, `cmd/server/main.go`, `internal/handler/gateway_test.go`, `backend/docker-compose.yml`. Los dos `.env.example` **salen de la superficie por S13**.
- Aceptación: preflight `OPTIONS /v1/risk/latest` desde el origen configurado responde 204 con `Access-Control-Allow-Origin`, `-Methods`, `-Headers` y `Vary: Origin`; un origen distinto no recibe ninguna cabecera CORS; `401`, `404`, `429` y `502` también llevan cabeceras CORS; el preflight no consume tokens del limitador ni exige `Authorization`; sin configuración no se emite ninguna cabecera CORS.
- Aceptación corregida por S13: el punto 8 pedía documentar `WEB_ORIGIN` en los dos `.env.example`, y S13 lo prohíbe. La parte funcional sí se cumplió y está verificada: `backend/docker-compose.yml:131` fija `WEB_ORIGIN: http://localhost:4173` dentro del `environment` del servicio gateway, no en el anchor `x-oidc` (líneas 3-7), así que no se filtra a `jobs` ni a `risk`. La demo local funciona sin tocar ningún `.env`. La descubribilidad se traslada a T7.
- Chequeo RED→GREEN: `go test ./services/gateway/...` con casos por cada aserción.
- Dependencias: ninguna. Paralelizable con T3, T5, T6.

**T3 — Página: base de API absoluta y orígenes OIDC**
- Superficie: `frontend/proposal.js`, `frontend/tests/proposal-readback.test.cjs`.
- Aceptación: toda llamada al gateway usa origen absoluto (`http://localhost:8080`); `redirectURI` pasa a `http://localhost:4173/`; el `issuer` de Keycloak sigue siendo `http://localhost:8081/realms/firewatch`; ninguna ruta `/v1/*`, `/health/ready` ni `/token` queda relativa.
- Chequeo RED→GREEN: la suite Node afirma el origen absoluto en cada una de las 9 rutas consumidas (`/v1/risk/zones`, `/latest`, `/history`, `/status`, `/sources`, `/v1/jobs`, `/v1/jobs/{id}`, `/v1/jobs/status`, `/health/ready`).
- Dependencias: T1.

**T4 — Gateway: dejar de servir HTML**
- Superficie: `backend/services/gateway/internal/handler/gateway.go`, `backend/services/gateway/internal/handler/gateway_test.go`, el retiro de `backend/services/gateway/internal/handler/web/**` completo, y por S14 `backend/docs/code-explanation/README.md` y `backend/docs/code-explanation/source-snapshot.json`.
- Aceptación: `GET /` responde 404 `not_found`; `/proposal.css`, `/proposal.js` y `/prototipos/**` también responden 404; el gateway conserva `/health/live`, `/health/ready`, todo `/v1/*` y el CORS de T2; `TestGatewayServesProposalWithoutChangingAPIAuthentication`, `TestGatewayUsesKeycloakRPLogout`, `TestGatewayKeepsPrototypeFilesPrivate` y `TestGatewayServesGeospatialMapLayer` se retiran (su contenido sobrevive en la suite Node de `frontend/`); el paquete compila sin `embed` ni `io/fs`.
- Aceptación por S14: el inventario de `code-explanation/README.md` pierde sus 5 filas web, suma la fila de `docs/keycloak-realm-setup.md`, corrige el rango de `gateway/cmd/server/main.go` y `docker-compose.yml`, y se regenera `source-snapshot.json` con `python verify_structure.py --snapshot`. El verificador **no** queda en verde: quedan 12 rangos y 7 archivos sin inventariar, todos ajenos a esta feature y declarados como deuda en S14. Lo exigible es que **ningún error restante pertenezca a esta feature**, comprobado filtrando la lista.
- **No tocar** `06-database.md:39`: menciona `//go:embed 001_initial.sql` de otro paquete (migraciones), no el embedding del gateway.
- Chequeo: `go test`, `go vet`, `go build` en `services/gateway`, más `python verify_structure.py` desde `backend/docs/code-explanation/`.
- Dependencias: T1 (los activos ya están en `frontend/`), T2 (CORS ya funciona antes de cortar la página).

**T5 — Servidor local del frontend y CSP**
- Superficie: `frontend/scripts/serve.mjs`, `frontend/package.json` y **`frontend/scripts/smoke.mjs`** (recreado). T1 retiró el arnés de humo que esta tarea usaba como chequeo, así que el chequeo de la tarea exige volver a tener uno. No es alcance añadido: es el chequeo que la propia spec pide.
- Aceptación: la allowlist sirve exactamente `index.html`, `proposal.css`, `proposal.js`; la CSP pasa de `connect-src 'none'` a `connect-src http://localhost:8080 http://localhost:8081`; se mantienen `default-src 'self'`, sin `unsafe-inline` en `script-src`, sin listado de directorio, solo GET/HEAD, y el rechazo de traversal codificado.
- Chequeo: `npm run smoke` (o el equivalente que sobreviva) contra un servidor efímero en `127.0.0.1:4173`; 200 en los tres activos y 404 en el resto.
- Dependencias: T1.

**T6 — Realm de Keycloak y configuración de entornos**
- Superficie: `backend/deploy/local/firewatch-realm.json`, `backend/docs/keycloak-realm-setup.md` (nuevo).
- **No toca `.env.example` ni `docker-compose.yml`**: esas superficies son de T2, que introduce `WEB_ORIGIN`. El solapamiento se detectó y se corrigió antes de delegar.
- Aceptación: el cliente `firewatch-demo` acepta `http://localhost:4173/` como `redirectUris` y `http://localhost:4173` como `webOrigins`, además de los valores actuales de `:8080` que **no se eliminan**; el atributo existente `post.logout.redirect.uris` (que **sí existe hoy**, en `attributes`, con valor `http://localhost:8080/`) pasa a `http://localhost:8080/##http://localhost:4173/` usando el separador `##`; `standardFlowEnabled` sigue en `true` y no se resetea ni sobrescribe un realm ya inicializado.
- Nota de importación: `command: [start-dev, --import-realm]` solo importa cuando el realm **no existe**. Sobre un realm ya inicializado este cambio de JSON no tiene efecto, por lo que se documentan los pasos manuales de Admin Console. Esa es la razón de ser del documento de T6.
- Chequeo: validación JSON del realm; documento con los pasos manuales de Admin Console para un realm existente.
- Dependencias: ninguna. Paralelizable con T2, T3, T5.

**T7 — Documentación**
- Superficie: `frontend/README.md`, `backend/docs/architecture.md`, `README.md` (raíz), `backend/README.md`.
- Aceptación: queda escrito que el gateway es API pura y responde 404 en `/`; que la página se sirve en `:4173` y **debe abrirse como `http://localhost:4173`, nunca como `http://127.0.0.1:4173`** (son orígenes distintos para el navegador, y el redirect URI y el `WEB_ORIGIN` están registrados para `localhost`); que `docker compose up` ya no abre la demo (S7); el `README.md` de `frontend/` deja de describir la maqueta retirada y describe la página real, la base de API y el redirect URI.
- Aceptación añadida por S13: documentar `WEB_ORIGIN` en `backend/README.md`, junto al Quick start y a los endpoints locales: qué hace, que la coincidencia es exacta y que sin definir no hay CORS. No se documenta en ningún `.env.example`.
- Dependencias: T1–T6.

**T8 — Verificación independiente**
- Verificador distinto del escritor, item 2 (S6).
- Aceptación: reproduce el preflight permitido y el rechazado, comprueba que el token sigue siendo obligatorio en `/v1/*`, que `/` es 404, que la suite Node pasa desde el layout nuevo, y confirma que no quedó ninguna referencia colgante a la maqueta ni a los assets embebidos.
- Dependencias: T1–T7.

### Orden de ejecución

```text
T1  -->  T2  --+
         T3  --+-->  T4  -->  T7  -->  T8
         T5  --+
         T6  --+
```

T1 primero porque T2–T6 razonan sobre el layout nuevo. T2 antes de T4 porque la página debe poder llamar cross-origin antes de que el gateway deje de servirla.

---

## Log

**L1** (usuario, verbatim):
> "oye bro pero cual es la pagina que se usa realmente la que se abre desde frontend o en el gateway? porque la que me parece que tiene las cosas que fuimos agregando a lo ultimo es la del gateway pero se supone que debía ser la que está en frontend"

Respuesta verificada: la página servida por el gateway (`internal/handler/web/`) es la oficial; `frontend/` es maqueta local sin consumidores. Evidencia: `gateway.go:21`, `frontend/README.md:3,5`, `odd/tasks/frontend/frontend-product-mockup.md:62`.

**L2** (usuario, verbatim):
> "quiero que la pagina del gateway se migre a la carpeta frontend, mi idea principal era tener el mockup para ir haciendo la pagina real"

Intención de cambio registrada. Habilitó la implementación de esta feature.

**L3** (usuario, verbatim, respuesta personalizada 1):
> "hay que hacer que la web de gateway esté en su propia carpeta que será la forntend/"

Registrado como S1. Nota: la palabra "forntend" es un error de tipeo del usuario, transcrito sin corregir dentro de la cita.

**L4** (usuario, selección 2):
> "Se reemplaza: frontend/ pasa a ser la página real"

Registrado como S2. Pérdida aceptada y declarada: el reducer determinista, 12 tests, `check.mjs`, `smoke.mjs` y las 6 vistas quedan sin cobertura.

**L5** (usuario, verbatim, respuesta personalizada 3):
> "se supone que frontend y backend van por separado, así es como tiene que funcionar"

Registrado como S3. Corroborado por el despliegue: el ingress solo expone `/v1/` (`backend/deploy/k8s/base/ingress.yaml:19-25`) y `gateway-entry` solo admite ingress-nginx en 8080 — la página `/` nunca se expuso en Kubernetes.

**L6** (usuario, selección 4):
> "Separación real: el gateway deja de servir HTML"

Registrado como S4. Descartó "Separación de carpetas: el gateway sirve una copia" y "Por etapas".

**L7** (coordinador, hallazgo técnico):
`go:embed` no puede referenciar `..`; por tanto `frontend/` no puede embeberse desde el paquete del gateway. Precedente del repo para el caso inverso: el GeoJSON derivado se commitea dentro del módulo Go (`backend/contracts/gen/geo/`) precisamente para que `go:embed` lo alcance, según el comentario de `.gitignore`.

**L8** (coordinador, hallazgo de riesgo):
El repositorio no tiene commits (`git log` informa `your current branch 'master' does not have any commits yet`) y el árbol está completamente sin seguimiento. Se fija S11: respaldo externo verificado por hash antes de retirar cualquier archivo, siguiendo el precedente `Temp/firewatch-l4-retired-*/frontend-mirror` de `odd/tasks/firewatch-app-funcional.md:36`.

**L9** (coordinador, inventario verificado):
Rutas que consume la página, extraídas de `proposal.js`: `/health/ready` (1036), `/token` (575), `/v1/risk/zones` (431), `/v1/risk/sources` (717), `/v1/risk/status` (788), `/v1/risk/latest` (932), `/v1/risk/history` (982), `/v1/jobs` (1173), `/v1/jobs/{id}` (1067), `/v1/jobs/status` (789). Constantes OIDC en `proposal.js:15-17`: `issuer = "http://localhost:8081/realms/firewatch"`, `clientID = "firewatch-demo"`, `redirectURI = "http://localhost:8080/"`.

**L10** (coordinador, inventario verificado — **corregido en L11**):
Realm `firewatch`, cliente `firewatch-demo` público: `redirectUris: ["http://localhost:8080/"]`, `webOrigins: ["http://localhost:8080"]`, `standardFlowEnabled: true`, `directAccessGrantsEnabled: true`.

**L11** (coordinador, corrección de exactitud):
El log L10 y la aceptación original de T6 afirmaban que el realm no configuraba la URI de redirección posterior al cierre de sesión. **Era falso.** El valor vive en `attributes` en la línea 18 de `backend/deploy/local/firewatch-realm.json`:

```json
"attributes": {"pkce.code.challenge.method": "S256", "post.logout.redirect.uris": "http://localhost:8080/"}
```

El inventario original leyó la clave de nivel superior `postLogoutRedirectUris` con Python y obtuvo `None`. Corrección aplicada a L10 y a la aceptación de T6. Patrón a recordar: Keycloak 26 guarda el post-logout redirect dentro de `attributes` con nombres punteados, no en una clave de nivel superior.

**L12** (coordinador, investigación acotada):
Incertidumbre real: el separador para múltiples valores de `post.logout.redirect.uris`. Resuelto por investigación: se concatenan en una única cadena separada por `##`. Fuente: documentación de `keycloak-config-cli`, <https://adorsys.github.io/keycloak-config-cli/config/adding-multiple-post-logout-redirect-uris/>.

**L13** (coordinador, riesgo futuro registrado, sin acción):
Las notas de actualización de Keycloak 26.6.0 introducen el ejecutor de política `secure-client-uris`, que exige HTTPS y hace **fallar la validación al actualizar** clientes existentes que usen `http` en "Post logout redirect URIs". El repositorio fija `quay.io/keycloak/keycloak:26.5.2` (`backend/docker-compose.yml:61`), así que no aplica hoy. Se registra para que una futura subida de versión no rompa el logout local por sorpresa.

**L14** (coordinador, ejecución T1):
Traslado completado. Respaldo en `tmp/gateway-frontend-separation-1791858059/` con `MANIFEST.md`: `diff -r` idéntico en ambos árboles y 29/29 archivos idénticos por `cmp`, 0 distintos. Los 4 activos copiados byte a byte desde `internal/handler/web/`; `frontend/` pasa de 23 a 6 archivos. Se recalcularon 3 rutas en el test trasladado: `${__dirname}/index.html` → `../index.html`, `${__dirname}/proposal.js` → `../proposal.js`, y la lectura del GeoJSON real de `../../../../../contracts/gen/geo/` a `../../backend/contracts/gen/geo/`. Baselines: 36/36 antes y 36/36 después. Guarda previa al borrado (14 archivos en `src/`, 2 en `styles/`) abortaba si el conteo no coincidía. Sin referencias colgantes a la maqueta.

**L15** (coordinador, ejecución T6):
Realm actualizado: `redirectUris` y `webOrigins` con `:4173` agregado y `:8080` conservado; `post.logout.redirect.uris` a `http://localhost:8080/##http://localhost:4173/`. Validado con Python: los tres campos contienen ambos orígenes y el separador `##` divide en exactamente 2 entradas. Nuevo documento `backend/docs/keycloak-realm-setup.md` en inglés, siguiendo la convención de `backend/docs/` (fuera de `code-explanation/`, que es español). Documenta por qué `--import-realm` no aplica sobre un realm existente y da los pasos manuales aditivos en Admin Console. **No ejecutado**: el chequeo programático del realm contra un Keycloak vivo; se declara como no verificado y se remite al navegador.

**L16** (coordinador, ejecución T5, escritor inline):
Se recreó `frontend/scripts/smoke.mjs` porque T1 retiró el arnés que esta tarea usaba como chequeo. RED observado con el `serve.mjs` viejo: exit 1, `AssertionError: /: connect-src still 'none'`. GREEN tras reescribir la allowlist a los 3 activos y la CSP a `connect-src http://localhost:8080 http://localhost:8081`: exit 0. `package.json` pasa a `firewatch-frontend`, `test` apunta al test real y se elimina `check` (`check.mjs` ya no existe). El smoke sigue cubriendo lo que el anterior cubría: HEAD, 405, 404 de rutas retiradas, allowlist explícita y traversal crudo/codificado. Hallazgo operativo registrado en el propio `serve.mjs`: la página debe abrirse como `http://localhost:4173` y **nunca** como `http://127.0.0.1:4173`, porque el redirect URI de Keycloak y el origen permitido del gateway están registrados para el origen `localhost`; son orígenes distintos para el navegador.

**L17** (coordinador, ejecución T3, escritor delegado `gentle-ai-worker`):
Base absoluta aplicada por el worker. Constante `gatewayOrigin = "http://localhost:8080"` en `frontend/proposal.js:22`, 9 call sites prefijados, `redirectURI` a `http://localhost:4173/` con `issuer` y `clientID` intactos, `/token` sin prefijar. Evidencia TDD del worker: RED 37 tests / 0 pass / 37 fail (exit 1), GREEN 37/37/0 (exit 0), más triangulación mutando `/token` a `gatewayOrigin` para confirmar que la protección tiene mordida. Verificación propia del coordinador: 9 call sites con `gatewayOrigin`, cero rutas relativas al gateway, `/auth`, `/token` y `/logout` sobre `issuer`, 37/37 y smoke PASS.

**L18** (coordinador, seguimiento de T3):
El comentario de cabecera de `frontend/proposal.js` (líneas 1-8) quedó **factualmente falso**: afirmaba que el gateway fijaba la CSP y que "nada fuera de este origen puede cargarse", cuando el diseño nuevo llama a dos orígenes externos. El worker lo detectó y lo reportó, pero no lo tocó por estar fuera de su superficie. Corregido por el coordinador tras cerrar T3.

**L19** (coordinador, ejecución y verificación de T2):
El worker `gentle-ai-worker` implementó S9 y S10 con RED→GREEN propio: RED 12 tests / 4 FAIL / 8 PASS (exit 1) contra el código sin CORS, con fallos representativos `gateway_test.go:370: 401 response must carry the configured origin, got ""` y `gateway_test.go:442: preflight 0 got 401 want 204`; GREEN 12/12 (exit 0). Verificación propia del coordinador, no aceptada por reporte: `go test -count=1 ./...` ok, conteo por `grep -c "^--- PASS"` = **12**, `grep -c "^--- FAIL"` = **0**, con los 7 tests nuevos presentes por nombre; `go vet ./...` exit 0; `go build ./...` exit 0. El embedding quedó intacto, comprobado por inspección: `gateway.go:21-22` conserva `//go:embed` y `proposalAssets`.

**L20** (coordinador, falsa alarma resuelta):
El reporte del worker decía que `WEB_ORIGIN` quedó "inside the existing `<<: *oidc` block", lo que habría significado filtrarlo a `jobs` y `risk`. Verificado: está en `backend/docker-compose.yml:131`, dentro del bloque `environment` del servicio `gateway`, no en la definición del anchor `x-oidc` (líneas 3-7). Solo aplica al gateway. La redacción era imprecisa; el código es correcto.

**L21** (coordinador, decisión sobre el punto 8 de T2):
El punto 8 pedía documentar `WEB_ORIGIN` en `backend/.env.example` y `backend/services/gateway/.env.example`. El worker se detuvo y pidió plan, invocando la política de rutas sensibles. La política es real y del propio repositorio, no una restricción inventada: `backend/docs/code-explanation/README.md:143-144`. Se evaluaron las tres salidas que ofreció:
- **A** (el coordinador escribe los dos archivos): descartada, la política vincula también al coordinador.
- **B** (autorizar un añadido a ciegas por shell, sin leer): **rechazada explícitamente**. Escribir un archivo que no se puede leer, con estado de fin de línea y claves existentes desconocidos, produce una escritura no verificable. Es exactamente el modo de fallo que S11 existe para evitar.
- **C** (omitir y registrar): aceptada, y **redirigida** en lugar de dejada como hueco. Se fija **S13** para que ninguna tarea futura vuelva a intentarlo. La parte funcional de la aceptación sí se cumplió: la demo local funciona sin tocar ningún `.env`, porque el compose fija `WEB_ORIGIN` directamente en el servicio. La descubribilidad se traslada a T7, en `backend/README.md`.

**L22** (coordinador, hallazgo pendiente para T4):
`backend/docs/code-explanation/` mantiene un `source-snapshot.json` con hashes e inventario de archivos, más un `verify_structure.py` reproducible. T4 borra `internal/handler/web/**` y modifica `gateway.go`, así que ese inventario queda invalidado. Debe comprobarse y regenerarse o declararse obsoleto como parte de T4; dejarlo sin tocar publicaría un snapshot falso.

**L23** (coordinador, investigación de L22 — origen del fallo y alcance real):
Se ejecutó `python verify_structure.py` en `backend/docs/code-explanation/` **antes** de tocar nada y **ya fallaba**. Mecanismo verificado leyendo el código: los rangos no salen del snapshot sino de la tabla de inventario del `README.md`; `verify_structure.py:76` compara el intervalo declarado de cada archivo contra su conteo real de líneas y solo entonces calcula los hashes.

Las cinco discrepancias y su atribución:

| Archivo | Declarado | Real | Origen |
| --- | --- | --- | --- |
| `docker-compose.yml` | 1-194 | 1-195 | **esta feature** (T2 agregó `WEB_ORIGIN`) |
| `services/risk/.../publisher_integration_test.go` | 1-123 | 1-133 | deuda previa |
| `contracts/gen/geo/geo.go` | 1-20 | 1-25 | deuda previa |
| `contracts/openapi/firewatch-v1.yaml` | 1-307 | 1-568 | deuda previa |
| `scripts/test_fetch_comunas.py` | 1-217 | 1-228 | deuda previa |

Cuatro de cinco son ajenas a este trabajo. Se fija **S14** y se amplía T4 para cerrar el verificador, con la instrucción explícita de reportar las cuatro preexistentes por separado para que la atribución no se mezcle. Verificado también que el verificador **no se ejecuta en CI** (`.github/workflows/` no lo menciona), así que el fallo no estaba bloqueando ninguna puerta automática; es exactitud documental, no un build roto.

Impacto en prosa medido antes de ampliar el alcance, para no crecer sin evidencia: `01-gateway.md` tiene **cero** menciones de los activos web o del embed; `06-database.md:39` menciona `//go:embed 001_initial.sql` de **otro** paquete (migraciones) y no debe tocarse; el único impacto real son 5 filas de la tabla de inventario en `code-explanation/README.md`.

**L24** (coordinador, verificación de seguridad pendiente de T2):
Se confirmó por ejecución propia que el CORS de T2 funciona (12/12 tests, `go vet` y `go build` limpios) pero **no se revisó el bloque CORS leyendo el código en busca de fallos que los tests no cubran**. Al ser item 2, esa lectura queda asignada a T8, con el verificador independiente. T2 y T4 tocan el mismo archivo, así que la revisión se hace con el archivo ya estabilizado.

**L25** (coordinador, ejecución de T4 — el borrado lo hizo el padre):
El writer `gentle-ai-worker` completó el cambio de código pero **no pudo borrar**: el arnés de seguridad de agentes hijos deniega operaciones de borrado ("children cannot authorize data loss"). Lo hizo el coordinador, y esa división es correcta: el hijo detectó el límite y lo reportó en vez de rodearlo.

Antes de borrar se **re-verificó el respaldo** — el único paso que hace esto reversible — con `diff -r` (idéntico) y `cmp` archivo por archivo (6/6 idénticos) contra `tmp/gateway-frontend-separation-1791858059/gateway-web/`. Solo entonces se ejecutó `rm -rf` sobre `backend/services/gateway/internal/handler/web/`.

Evidencia propia, no aceptada por reporte: gateway **9 PASS / 0 FAIL**, `go vet` exit 0, `go build` exit 0, `gofmt -l` vacío; frontend **37 pass** y smoke OK; `grep` de `go:embed|proposalAssets|fs.Sub` en `gateway.go` sin resultados. El conteo 9 es exactamente el predicho (7 CORS + filtros + el nuevo 404 de `/`).

**L26** (coordinador, corrección de un error propio):
S14 se había construido sobre evidencia truncada. La medición original fue `python verify_structure.py 2>&1 | tail -8`, y las 5 líneas visibles se declararon como el total. La medición completa da **16 rangos obsoletos y 9 archivos sin inventariar**. El worker lo detectó al no poder cumplir la aceptación y lo reportó como premisa equivocada en vez de forzar el resultado. S14 queda reescrita con los números reales, el error declarado dentro de la propia spec, y la regla derivada: un conteo de errores no se declara desde una salida truncada.

**L27** (coordinador, decisión de alcance sobre la deuda del manifiesto):
Se evaluaron las tres salidas. Se descartó barrer los 19 problemas restantes: inventariar archivos de `jobs/` y `risk/` obliga a asignarles un estado documental (`Explicado`/`Parcial`/`Contexto`) que requiere leer y juzgar trabajo ajeno, y corregir solo los rangos dejaría el verificador igual de rojo porque los 7 archivos sin inventariar seguirían ahí. Barrer a medias no compra nada salvo ruido.

Decisión: corregir **solo** lo que esta feature invalida o introduce, y declarar el resto con evidencia exacta como deuda preexistente, ofreciendo una tarea separada.

Atribución verificada, no afirmada. Tras corregir lo propio, el verificador pasó de 16→12 rangos y de 9→7 sin inventariar, `Missing inventory file` quedó en 0 (las 5 filas web se retiraron bien) y `Source snapshot differs` en 0. Filtrar los 19 errores restantes contra `gateway|frontend|deploy|keycloak` **no devuelve ninguno**. Los 19 viven en `services/risk` (11), `services/jobs` (7) y `contracts/gen` (1): rutas que esta feature nunca editó.

**L28** (coordinador, ejecución de T7):
Cuatro documentos escritos por el coordinador, en el idioma de cada uno: `frontend/README.md` nuevo en inglés (el anterior lo retiró T1, así que T7 lo crea), `backend/README.md` y `backend/docs/architecture.md` en inglés, `README.md` raíz en inglés.

Decisiones de contenido, no solo redacción:
- Se **eliminó la duplicación** de los pasos de Admin Console de Keycloak que vivían en `backend/README.md`. Estaban repetidos y con valores de `:8080`; ahora el README remite a `docs/keycloak-realm-setup.md`. Dos copias de un procedimiento manual garantizan que una quede vieja.
- `WEB_ORIGIN` documentado en `backend/README.md` según S13, junto a los endpoints locales, con las tres propiedades que importan: coincidencia exacta, fail-closed sin definir, nunca comodín.
- Se documentó que la página debe abrirse como `localhost` y no como `127.0.0.1`. Es una trampa real: el servidor responde igual, pero el navegador considera dos orígenes distintos y el login falla.
- `architecture.md` declara ahora que el gateway **no** sirve la aplicación y que el CORS es higiene aplicada por el navegador, no autorización: la autorización sigue siendo el chequeo de bearer sobre cada `/v1/*`, con cualquier origen.

Verificación propia de T7: comprobador de enlaces relativos sobre los 4 documentos sin enlaces rotos; los dos directorios de respaldo citados existen (23 y 6 archivos); probe LSP sobre los 4 con **0 diagnósticos**. Se corrigió una imprecisión propia: la ruta del respaldo no decía que es relativa a la raíz del repositorio.

**L29** (coordinador, alcance de T8):
T8 debe cubrir lo que ninguna verificación por ejecución de tests puede cubrir, en este orden de prioridad:
1. **Lectura del bloque CORS de `gateway.go`** (L24). Es item 2 y nunca se revisó leyendo el código. Buscar específicamente: comparación exacta de origen, un camino de escape en el fail-closed, orden de cabeceras para que `failure()` conserve las CORS, y que el preflight no pueda saltarse el límite ni la autenticación en ninguna ruta.
2. Reproducir el preflight permitido y el rechazado, y comprobar que el bearer sigue siendo obligatorio en `/v1/*`.
3. Confirmar `GET /` = 404 y la suite Node desde el layout nuevo.
4. Confirmar que no quedan referencias colgantes a la maqueta ni a los assets embebidos.

Sigue **sin verificar en navegador** el ciclo real de login/logout, y el realm contra un Keycloak vivo. T8 no puede declararlos verificados; debe declararlos como no ejecutados. Tampoco puede verificar la sincronización entre `WEB_ORIGIN` y el realm sin levantar el entorno completo.

**L30** (coordinador, fallo de proceso propio — detectado por T8):
El verificador independiente encontró **22 errores**, no los 19 que yo había declarado. Los tres de diferencia eran **míos**, y el fallo no fue de cálculo sino de secuencia.

Qué pasó, exactamente:
1. Tras T4 corrí el verificador y la atribución quedó limpia: 12 rangos + 7 sin inventariar, ninguno mío. **Esa medición era correcta.**
2. Después ejecuté T7, que editó `backend/README.md` (`1-95` → 91 líneas reales) y `backend/docs/architecture.md` (`1-60` → 63 líneas reales). Ambos están inventariados, así que T7 invalidó dos rangos y el snapshot.
3. **No volví a correr el verificador tras T7.** Cerré T7 verificando enlaces y diagnósticos LSP, que no cubren S14.
4. Reporté "0 míos" al usuario y entregué esa afirmación a T8 sin volver a medirla.

El verificador independiente la falsificó, que es precisamente su función.

Regla que se deriva y se fija: **una tarea que toca un archivo inventariado invalida la verificación de integridad, así que el verificador se re-ejecuta después de esa tarea, no solo después de la tarea que motivó la spec.** El chequeo de cierre de T7 estaba incompleto: enlaces y LSP no dicen nada sobre el manifiesto.

**L31** (coordinador, reparación de T9): reparación ejecutada por el coordinador: `backend/README.md` a `1-91`, `docs/architecture.md` a `1-63`, snapshot regenerado con `--snapshot`. Resultado medido: **12 rangos + 7 sin inventariar = 19**, `Missing inventory file` en 0, `Source snapshot differs` en 0, y filtrar los 19 restantes contra `gateway|frontend|deploy|keycloak|README.md|architecture.md` **no devuelve ninguno**. Los 19 viven en `services/risk` (11), `services/jobs` (7) y `contracts/gen` (1).

**L32** (coordinador, respuesta a una consulta del verificador):
El verificador preguntó si podía correr tests y scripts dado que yo había prohibido crear archivos. La restricción estaba sobreextendida y se corrigió con un límite explícito. Al acotarla apareció un riesgo que no había previsto: **`verify_structure.py --snapshot` reescribe `source-snapshot.json`**, el artefacto mismo que el verificador debe inspeccionar. Autorizarlo habría vuelto el chequeo tautológico, porque el archivo siempre coincide consigo mismo recién escrito. Queda prohibido explícitamente, junto con toda escritura en el repositorio, todo comando git mutante y toda instalación de dependencias.

También se respondió una pregunta de alcance: el verificador quería inspeccionar "scripts PowerShell/password grant". Verificado por `find`: **no existe ningún `.ps1` ni `.psm1`** en el repositorio. Las menciones a *password grant* y *PowerShell* son solo prosa en `backend/README.md` y `backend/docs/code-explanation/07-deployment.md` y `08-tests-tools.md`; ninguno apunta a `localhost:8080/` esperando la página. No hay defecto de este cambio en esa dirección.

**L33** (T8, veredicto del verificador independiente):

Lo que **sí** verificó: coincidencia exacta de origen (`gateway.go:57-60`, sin prefijo, sufijo, subcadena ni normalización de mayúsculas); fail-closed con configuración vacía; orden de cabeceras CORS antes de cada `failure()` (líneas 57-61 frente a 86, 90, 95, 102 y el despacho del proxy en 106), con los tests de `401`, `404`, `429` y `502` pasando; `Max-Age: 600` defendible; Go 9 PASS / 0 FAIL, Node 37/0, smoke OK, `vet`/`build`/`gofmt` limpios. Y **no demostró ningún defecto de seguridad** en el CORS.

Lo que **falsificó**: mi afirmación de "19 errores, ninguno mío". Midió 22. Ya reparado en T9; su medición fue anterior a la reparación, y el re-chequeo posterior confirma 19.

**Corrección a mi encuadre del preflight.** Yo le pedí comprobar si "un `OPTIONS` puede saltarse el límite o la autenticación". Su respuesta es más precisa que mi pregunta: literalmente, un `OPTIONS` de origen permitido **sí** omite el limitador y el bearer en cualquier ruta (líneas 64-69), pero retorna de inmediato sin proxear nada, así que **ninguna operación protegida es alcanzable por esa exención**. "Ningún OPTIONS omite límite ni autenticación" es falso; "ninguna operación protegida se alcanza por esa exención" está sostenido por lectura. Se registra la formulación correcta.

También declaró no verificados, correctamente, el ciclo real de login/logout en navegador, el realm contra un Keycloak vivo, y respuestas de endpoints desplegados.

**L34** (T10, correcciones de documentación que T8 encontró — tres defectos, dos de ellos afirmaciones falsas mías):

1. **Dependencia inexistente en `backend/docs/keycloak-realm-setup.md`.** Yo afirmaba que retirar las entradas `:8080` "rompería la demo PowerShell, que todavía usa el password grant contra `http://localhost:8080/`". **Falso.** Confirmado leyendo `backend/scripts/smoke.py:58-64`: `token_for()` envía `grant_type=password` con `client_id`, `username` y `password`, **sin `redirect_uri`**. El password grant no usa redirect URIs, así que esos valores no le hacen falta. Reescrito para decir lo único cierto: **nada en este repositorio los requiere ya**, y se conservan porque estrechar la configuración de un cliente público existente no aporta nada aquí y rompería a cualquier consumidor ya configurado que apunte a `:8080`.
2. **La misma afirmación falsa en `backend/README.md`.** "deliberately kept for the password-grant demo scripts" → corregido con el mismo fundamento.
3. **`backend/docs/architecture.md` prometía de más.** Decía que el gateway aplica el bearer "a cada petición `/v1/*` con cualquier origen", sin la excepción de preflight. Añadida la excepción explícita.

Además, T8 señaló que mi afirmación "no quedan referencias colgantes" era falsa como enunciado irrestricto: `odd/tasks/frontend-react-vite-migration.md:31` enlazaba a `frontend/src/core/router.js`, `frontend/src/main.js` y `frontend/tests/state.test.mjs`, los tres borrados por T1. Siguiendo el precedente del repo de **no reescribir la historia** (`odd/tasks/frontend/frontend-product-mockup.md:60`), se añadió una nota de supersesión al inicio del documento en vez de reescribir su plan, y se declara que los tres enlaces están muertos y dónde vive la copia.

**L35** (coordinador, re-chequeo del manifiesto tras T10):
Las tres correcciones fueron reemplazos de una línea, así que los conteos no cambiaron (91, 63 y 58) y no hubo rangos que corregir; los hashes sí, así que se regeneró el snapshot **inmediatamente, en el mismo paso**, aplicando la regla reforzada de S14 en vez de repetir el fallo de T7. Resultado medido: `exit=1`, **12 rangos + 7 sin inventariar = 19**, `Source snapshot differs` en 0, y ningún error restante menciona `README.md`, `architecture.md` ni `keycloak`.

**L36** (T11, veredicto del segundo verificador independiente — verifier nuevo, el primero no se reutilizó):

Confirma lo esencial y **no encontró ningún defecto de seguridad en el CORS**, esta vez con evidencia adicional a la lectura: ejecutó un `go test -overlay` con archivos exclusivamente en un directorio temporal, o sea sin escribir nada en el repositorio.

| Comprobación | Resultado observado |
| --- | --- |
| Manifiesto | exactamente **12 rangos + 7 sin inventariar**, `Missing inventory file` 0, `Source snapshot differs` 0 |
| Go | **139 PASS** de nivel superior, **0 FAIL**, 11 SKIP; 343 PASS contando subtests |
| `go vet` / `go build` / `gofmt -l` | limpios |
| Node | **37 pass / 0 fail** |
| Smoke | PASS |
| Overlay temporal (3 tests, aditivo) | **3/3 PASS**, en el propio árbol del repo |

Ese overlay **cierra el hueco que dejó el primer verificador**: token inválido **con origen permitido** en `/v1/risk/latest` → **401**, y **cero llamadas upstream** para un `OPTIONS` permitido. Es la prueba dinámica de que el CORS no debilita la autorización, y de que la exención de preflight no alcanza ninguna operación protegida. Item 2 queda atendido por lectura **y** por ejecución.

El coordinador acotó el overlay a **añadir** un archivo de test, prohibiendo que sobreescribiera `gateway.go` o `gateway_test.go`, y exigió los hashes de ambos antes y después. Motivo: un overlay que reemplaza la implementación o el testigo vuelve la verificación circular, y habría producido evidencia sin valor.

Limitación declarada por el verificador, y es correcta: la preservación byte a byte de los documentos históricos **no se puede probar**, porque el repositorio no tiene línea base. No se declara como verificado.

**L37** (T12, los 3 defectos que T11 encontró — dos de ellos afirmaciones mías sin comprobar):

1. **Mi nota de supersesión decía que `scripts/smoke.mjs` ya no existe. Es falso: yo mismo lo recreé en T5.** Escribí la nota desde el inventario del plan de T1 en lugar de desde el árbol actual. Corregido: `scripts/check.mjs` sí desapareció; `smoke.mjs` existe de nuevo, pero verifica la página actual y no la maqueta.
2. **T11 no existía en la tabla de tareas del documento**, solo en el plan visible. Corregido, junto con T12.
3. **`frontend/README.md` describía la CSP como más estricta de lo que es.** Yo escribía "permite scripts, estilos e imágenes desde el propio origen" y "nada más es alcanzable", cuando la política real incluye `style-src 'self' 'unsafe-inline'` e `img-src 'self' data:`. Describir un control de seguridad como más fuerte de lo que es resulta peor que no describirlo. Reescrito con la política literal y una lista de lo que cada directiva permite **y lo que no**, más la advertencia de que un despliegue que estreche `style-src` o `img-src` debe re-comprobar el render en vez de suponer.

**Verificación de las tres correcciones, por observación directa y no por afirmación**: el manifiesto sigue en 19 con `snapshot_diff` 0 y ningún error que mencione `frontend/`, `odd/` o mis documentos; `frontend/scripts/` contiene `serve.mjs` y `smoke.mjs` y **no** `check.mjs`; y las tres directivas que el README declara coinciden con lo que `serve.mjs` envía.

**L38** (coordinador, decisión de no abrir una tercera ronda de verificación):
Se evaluó explícitamente y se decidió no re-verificar una vez más. Motivo, no comodidad: las tres correcciones de T12 son **exclusivamente documentales**, ninguna toca un archivo del inventario —comprobado ejecutando el verificador, no asumido—, y por tanto no alteran ningún artefacto sobre el que T11 haya dictaminado. Las tres afirmaciones corregidas se comprobaron una a una contra el artefacto real por observación directa. Una tercera ronda repetiría las mismas comprobaciones de CORS, manifiesto y suites, cuyo estado no cambió.

Queda anotado como decisión del coordinador, reversible: si se prefiere una tercera verificación independiente, la única superficie nueva a cubrir son esas tres frases.

**L39** (hallazgo posterior, del recorrido con sesión real; arreglado después de esta feature):
El recorrido con sesión en navegador midió que el 403 de `#audit` no mostraba su `request_id`. La causa **no** estaba en el cliente —`frontend/src/core/api.ts:174` lee la cabecera correctamente— sino aquí: el bloque CORS de este gateway nunca declaró `Access-Control-Expose-Headers`, y `X-Request-ID` no es safelisted, así que el navegador la ocultaba a la página. Se fija como **S15** y se corrige en la rama `fix/gateway-expose-request-id`, con RED→GREEN propio y el fail-closed cubierto por prueba. Consecuencia declarada: las afirmaciones de `guion-recorrido.md:38` y `README.md:104` que el informe del recorrido marcó como falsas **vuelven a ser ciertas por el arreglo del servidor**, no por corregir el texto. Deuda que este cambio agranda y declara: `backend/docs/code-explanation/01-gateway.md` ancla rangos de líneas de `gateway.go` que ya estaban obsoletos antes de tocar el archivo, y este arreglo les suma **2 líneas**; no se re-baseliniza porque exige autorización explícita (S14).
