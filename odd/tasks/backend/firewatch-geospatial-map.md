# Mapa geoespacial real de Valparaiso

## Objetivo

Mover a estado real un requisito de las Bases que hoy esta en cero: el mapa. Hoy la pagina servida dibuja una rejilla con un pin y PostGIS guarda un unico `Point` hardcodeado; no existe tabla de zonas, ni geometria de comunas, ni endpoint por bbox. Al cerrar esta feature el sistema debe tener las 38 comunas de Valparaiso en PostGIS, endpoints geoespaciales que sirvan `ST_AsGeoJSON`, y una capa de mapa real en la pagina servida del gateway.

No se predice fecha ni lugar, y la restriccion de la fuente se declara donde corresponde (ver abajo).

## Fuente verificada (comprobada por el orquestador, no se re-busca)

Servicio REST oficial del MOP que sirve la capa DPA de IDE Chile (SUBDERE, IGM, DIFROL, INE). Capa `Comunas`, id `1`.

Consulta verificada, filtrada por region, que devuelve exactamente las 38 comunas de Valparaiso con sus codigos CUT oficiales:

```text
https://rest-sit.mop.gob.cl/arcgis/rest/services/INTEROP/SERVICIO_DPA/MapServer/1/query?where=CUT_REG%3D%2705%27&outFields=CUT_COM,COMUNA,PROVINCIA,CUT_PROV&returnGeometry=true&outSR=4326&f=json
```

Datos duros comprobados:

- `supportedQueryFormats` es `JSON, AMF`. **No hay GeoJSON**: la conversion Esri JSON a GeoJSON es manipulacion de JSON pura y se hace con la biblioteca estandar de Python, como `init_env.py` y `smoke.py`, para que un clon limpio no necesite instalar nada.
- `maxRecordCount` es 1000 y son 38 filas: cabe en UNA pagina. No hace falta paginacion, pero SI hay que comprobar que la respuesta no venga truncada y registrar la bandera de completitud.
- Se pide `outSR=4326` para que la geometria entre directo en PostGIS sin reproyectar. La capa origen declara `wkid 5360`.
- Los 38 CUT van de `05101` a `05804`. Incluye Isla de Pascua (`05201`) y Juan Fernandez (`05104`), lejisimos del continente: son parte de la region y hay que tenerlos en cuenta para la vista y para el bbox.
- El poligono de la comuna Antartica NO existe en esta capa. No se inventa.

## Procedencia que hay que registrar (primera fila real de la tabla de trazabilidad)

La tabla de trazabilidad de las Bases estaba vacia hasta ahora. Esta es su primera fila real. Hay que registrar: endpoint, huella de la consulta, `retrieved_at`, `cutoff_at`, formato (Esri JSON de origen, GeoJSON derivado), y ademas:

- **ATRIBUCION**: Subsecretaria de Desarrollo Regional y Administrativo (SUBDERE), Instituto Geografico Militar (IGM) e Instituto Nacional de Estadisticas (INE), segun los metadatos del servicio.
- **RESTRICCION DECLARADA, textual del propio servicio**: las publicaciones y productos derivados de esta cartografia deben ser revisadas y validadas por DIFROL. NO se afirma que esten validadas. Un mapa web de limites comunales ES un producto derivado, asi que la restriccion aplica y hay que DECLARARLA tanto en la tabla como en el README. Para una demo academica es defendible; para produccion es una condicion real.

## Superficies previstas

- `backend/scripts/` (descargador nuevo, stdlib).
- `backend/services/risk/assets/geo/` (derivado versionado, embebido).
- `data/` en la raiz del repositorio: `data/raw/geo/` ignorado y `data/trazabilidad/` versionado (entregable de las Bases).
- `.gitignore` de la raiz (patron del crudo).
- `backend/services/risk/migrations/` (migraciones NUEVAS; jamas se edita la 001).
- `backend/services/jobs/migrations/` (migracion NUEVA; jamas se edita la 001).
- `backend/services/risk/internal/` (store, handler, servicio de validacion).
- `backend/services/jobs/internal/handler/api.go` y su store.
- `backend/contracts/openapi/firewatch-v1.yaml`.
- `backend/services/gateway/internal/handler/web/` (pagina servida).
- `backend/docs/code-explanation/` y `backend/README.md`.
- Este documento y `odd/tasks/firewatch-data-contract.md`.

## Particion propuesta en cinco rebanadas

Dependencias: `M1 -> M2 -> M3`, y `M2 -> M4 -> M5`. M3 y M4 son independientes entre si.

### M1 — Adquisicion y procedencia (sin base de datos)

Descargador `backend/scripts/fetch_comunas.py` en stdlib: consulta verificada, conversion Esri JSON a GeoJSON (`rings` a anillos de GeoJSON, con el cierre del anillo y el orden de orientacion correctos), comprobacion de truncamiento (`exceededTransferLimit`) y bandera de completitud. Escribe el **derivado versionado** en `backend/services/risk/assets/geo/valparaiso-comunas.geojson`, el **crudo ignorado** en `data/raw/geo/valparaiso-comunas-<fecha>.esri.json` con su checksum, y el registro de procedencia en `data/trazabilidad/valparaiso-comunas.provenance.json` con atribucion y restriccion DIFROL. El derivado vive DENTRO del arbol de `risk` a proposito: asi `go:embed` lo alcanza y el Dockerfile, que ya copia `services/risk/`, no se toca.

Cierre: pruebas unitarias de la conversion con un fixture pequeno embebido (sin red), una ejecucion real contra el servicio registrada como evidencia, y el registro de procedencia escrito.

### M2 — Esquema y carga de zonas

Migracion NUEVA de `risk` con la tabla `zones` (`code` PK, `name`, `province`, `kind`, `geometry geometry(Geometry,4326)`) e indice GiST, mas la fila heredada `valparaiso-demo` (`kind = 'demo'`, sin geometria) para que las filas historicas sigan siendo validas.

**Apartamiento deliberado de la letra de la especificacion, informado antes de escribir**: las 38 comunas NO se siembran en el SQL de la migracion. Se siembran desde el derivado embebido en el paso idempotente del camino de `migrate`, que ya existe para la geometria. Motivo: sembrar CUT, nombre y provincia en SQL Y tenerlos tambien en el GeoJSON crea DOS fuentes del mismo dato, y eso es exactamente el defecto de deriva silenciosa que ya hemos corregido tres veces (enums del OpenAPI, parametros a mano, instantanea con rangos). El objetivo del orquestador -- que un volumen NUEVO quede sembrado automaticamente al levantar -- se cumple igual, porque el paso corre en `migrate`. La migracion se queda con lo que solo ella puede hacer: la tabla, el indice y la fila heredada, que no esta en el activo geografico. Carga **idempotente** de la geometria desde el derivado embebido, ejecutada en el camino de `migrate` como paso posterior a las migraciones. Motivo: un volumen NUEVO debe quedar sembrado automaticamente al levantar; si la siembra fuera manual, el despliegue en cloud quedaria con el mapa vacio hasta que alguien se acordara, que es exactamente el defecto del realm. Comportamiento explicito de "arranca sin datos": la tabla existe, la geometria puede faltar, y el sistema muestra informacion insuficiente en lugar de inventar una clasificacion.

Cierre: migracion probada contra PostgreSQL real en contenedor desechable, las 38 filas con su CUT y su provincia, y la ruta "sin geometria cargada" ejercitada.

### M3 — Los cinco lugares que acotan la zona derivan de la tabla

Ver el checklist al final. Los dos `CHECK` de las migraciones 001 congeladas se relajan con migraciones NUEVAS en cada servicio, preferentemente sustituyendolos por una **clave foranea** a `zones(code)`, que es lo que de verdad impide crear una zona inexistente. El handler de `jobs` y la allowlist de `risk` dejan de llevar literales.

Cierre: un CUT inexistente o mal escrito se rechaza en los cinco lugares; un CUT valido se acepta en los cinco; ninguna migracion congelada fue editada.

### M4 — Endpoints geoespaciales y contrato

`GET /v1/risk/zones` (listado) y `GET /v1/risk/zones?bbox=...` sirviendo `ST_AsGeoJSON`, con bbox validado (orden, rango, dimension) y respuesta vacia honesta. **No requiere tocar el router del gateway ni la CSP**: el gateway ya proxya `/v1/risk/` completo y la pagina ya puede hacer `fetch` al mismo origen. El OpenAPI declara los endpoints nuevos y deja de fijar `geometry.type` al enum `Point`.

Cierre: bbox vacio, bbox invalido y zona inexistente cubiertos por pruebas; `ST_AsGeoJSON` verificado contra PostGIS real.

### M5 — Capa de mapa real en la pagina servida

Reemplaza la rejilla y el pin por una capa real dibujada desde el GeoJSON de M4. **No es "poner un mapa": es RENDERIZAR nosotros mismos** las geometrias de las 38 comunas con una proyeccion propia, sin biblioteca externa y sin teselas, coloreadas por nivel de riesgo, con la atribucion y la restriccion DIFROL visibles en la vista.

LIMITE DURO DECLARADO: un basemap de teselas queda FUERA por CSP. La politica fija `connect-src 'self'` e `img-src 'self'`, asi que no se puede usar OpenStreetMap, MapTiler ni ningun proveedor de teselas por red, y MapLibre exigiria ademas vendorizar la biblioteca en el repositorio. Incorporar un basemap seria una decision DELIBERADA que exige vendorizar una biblioteca Y relajar `connect-src` e `img-src`; nunca un accidente. Y no es una perdida: el requisito de las Bases es representar geograficamente las ZONAS ANALIZADAS, no reproducir un mapa base.

Se queda en la pagina del gateway, sin migrar a React. Respeta Isla de Pascua y Juan Fernandez sin romper el encuadre del continente.

Cierre: la capa dibuja las 38 comunas desde la API real, la vista declara procedencia y restriccion, y la pagina sigue sin romper la CSP.

### Verificacion final

Una verificacion independiente de la feature completa, con contenedor desechable propio y mutaciones en copias fuera del arbol, mas el re-baseline del inventario estructural como ultimo acto.

## Decisiones resueltas (2026-10-11)

1. **Como entra la geometria**: subcomando/ paso de siembra DENTRO del arbol de `risk`, con `go:embed` del derivado, ejecutado en el camino de `migrate`. Sin tocar ningun Dockerfile, porque el Dockerfile de `risk` ya copia `services/risk/`. Descartadas: ruta externa por entorno (mete un paso manual en el arranque) y `.sql` generado (duplica los datos y no es mas legible que el GeoJSON).
2. **Rutas**: derivado VERSIONADO en `backend/services/risk/assets/geo/valparaiso-comunas.geojson`; crudo IGNORADO en `data/raw/geo/` con su checksum; procedencia VERSIONADA en `data/trazabilidad/`, que es el entregable de las Bases y va SEPARADA del activo de producto.
3. **Enum de zona del OpenAPI**: `type: string` con descripcion y ejemplo; la lista vive en `/v1/risk/zones`. Un enum de 38 CUTs derivaria en silencio.
4. **`geometry.type` en el OpenAPI**: objeto cuyo miembro `type` es un enum de `Point`, `Polygon` y `MultiPolygon`, descrito como geometria GeoJSON conforme a RFC 7946. NO se usa `oneOf`: el miembro `type` ES el discriminador en GeoJSON, y reimplementar cada forma duplicaria el estandar y derivaria.
5. **Red en CI**: CI NO llama al servicio. El fixture embebido es la prueba y la ejecucion real queda como evidencia manual declarada.

## Condiciones no negociables

- Nunca se edita `001_initial.sql` de ningun servicio: todo va en migraciones nuevas.
- Nunca se ejecuta `backend/scripts/init_env.py` en el arbol real: solo en copias.
- Siempre `-p firewatch-verify` al levantar contenedores; nunca `down -v` sobre `firewatch`.
- Un solo escritor a la vez; los verificadores de solo lectura pueden correr en paralelo.
- Sin commit y sin revision nativa: `nogit` sigue vigente.
- No se migra el frontend a React ni se toca `frontend/`.

## Checklist: los SIETE lugares que acotan la zona

Dejar uno atras crea una inconsistencia silenciosa entre lo que la API acepta y lo que la base admite. Los lugares sexto y septimo no estaban en la lista original: los encontro el orquestador al revisar el camino de lectura de `risk` y al recibir la evidencia del escritor.

1. [x] `backend/services/jobs/internal/handler/api.go` — la comparacion con el literal se sustituyo por una consulta al catalogo (`ZoneExists`): zona ausente 400, error de almacenamiento 503, cualquier codigo valido aceptado.
2. [x] `SupportedZones` en `backend/services/risk/internal/service/demo.go` — ELIMINADO. La validacion de transporte conserva solo la forma (zona no vacia) y la pertenencia quedo en la clave foranea del esquema.
3. [x] `CHECK (zone_id = 'valparaiso-demo')` en `backend/services/risk/migrations/001_initial.sql:5` — sustituido por `batches_zone_id_fkey` en la migracion nueva 005.
4. [x] `CHECK (zone_id = 'valparaiso-demo')` en `backend/services/jobs/migrations/001_initial.sql:5` — sustituido por `jobs_zone_id_fkey` en la migracion nueva 002.
5. [x] Enum de zona del `backend/contracts/openapi/firewatch-v1.yaml` — las tres propiedades pasaron a `type: string` con ejemplo y descripcion que nombra el catalogo.
6. [x] `backend/services/risk/internal/handler/api.go` — el parametro `zone_id` de LECTURA consulta la tabla `zones` a traves del repositorio; 400 si no existe, 503 si falla el almacenamiento.
7. [x] `backend/contracts/openapi/firewatch-v1.yaml:127` — el parametro COMPARTIDO `Zone` perdio su enum propio. Verificado: `grep -n "enum: \[valparaiso-demo\]"` no devuelve nada en todo el contrato.
8. [ ] `backend/services/gateway/internal/handler/web/proposal.js:2` — la pagina servida fija `const zone = "valparaiso-demo"`. No es una puerta que rechace nada, pero SI es un literal de zona en codigo de produccion, y con el catalogo real la pagina debe poder elegir zona. Se resuelve en M5.

## Tareas

1. [ ] M1 — descargador stdlib, conversion Esri JSON a GeoJSON, derivado versionado, crudo ignorado, registro de procedencia con atribucion y restriccion DIFROL.
2. [ ] M2 — migracion de `zones` con GiST, siembra de las 38 comunas y de la fila heredada, carga idempotente de geometria, arranque sin datos.
3. [ ] M3 — los cinco lugares acotan la zona desde la tabla; claves foraneas en lugar de `CHECK` congelados.
4. [ ] M4 — endpoints por listado y bbox con `ST_AsGeoJSON`, validacion de bbox y OpenAPI declarado.
5. [ ] M5 — capa de mapa real en la pagina servida, sin dependencias externas y con atribucion y restriccion visibles.
6. [ ] Verificacion independiente de la feature y re-baseline del inventario estructural.

### Evidencia de M1 — adquisicion y procedencia

Ejecutada y observada el 2026-10-11. Implementada por `gentle-ai-worker`. Verificacion independiente en curso mientras se escribe esto.

Cambios: `backend/scripts/fetch_comunas.py` (stdlib, con `--dry-run` y `--allow-incomplete`), `backend/scripts/test_fetch_comunas.py` (15 pruebas, SIN red), derivado versionado `backend/services/risk/assets/geo/valparaiso-comunas.geojson` (38 features, 2.137.628 bytes, sha256 `02ef8e8f...`), crudo ignorado `data/raw/geo/valparaiso-comunas-20261011.esri.json` (2.217.407 bytes, sha256 `79492486...`), procedencia versionada `data/trazabilidad/valparaiso-comunas.provenance.json` y el patron estrecho `data/raw/geo/valparaiso-comunas-*.esri.json` en `.gitignore` (mas estrecho de lo que yo describi al planificar: ignora ese activo concreto, no cualquier `.esri.json`).

Evidencia real:

- `python backend/scripts/test_fetch_comunas.py`: 15 pruebas OK, sin red.
- `python backend/scripts/fetch_comunas.py`: exit 0; `record_count 38/38`, `complete true`, `exceeded_transfer_limit false`; tipos de geometria `{MultiPolygon: 7, Polygon: 31}`.
- Determinismo: `--dry-run` no escribe nada y produce el MISMO sha256 del derivado que la corrida real.
- El derivado parsea como FeatureCollection con 38 features, CUT ordenados de `05101` a `05804`, incluidas `05201` (Isla de Pascua) y `05104` (Juan Fernandez); propiedades `CUT_COM`, `COMUNA`, `PROVINCIA`, `CUT_PROV`; cero violaciones de orientacion o cierre (todos los anillos exteriores antihorarios y cerrados).
- `git check-ignore -v` confirma que el crudo esta ignorado y que el derivado NO lo esta; `git ls-files --others --exclude-standard data` solo lista la procedencia.
- `query_sha256` recomputado de forma independiente: `9bef5bd58006f9053c3d6603b622a0064f721cd33213e10fe8619adf324b4a4f`.

**Hallazgo del dato real**: todos los anillos vienen en sentido HORARIO y sin anidamiento, asi que las 38 son anillos exteriores: 7 `MultiPolygon` por islas y 31 `Polygon`. El conversor igual clasifica por area firmada medida y asigna huecos por punto-en-poligono, y el fixture ejercita esa rama con un hueco y un multiparte; que el dato real no traiga huecos no la vuelve codigo muerto, porque la capa podria traerlos.

La comuna Antartica NO aparece en la capa y no se invento. `cutoff_at` quedo igual a `retrieved_at` porque el servicio no publica corte, y la procedencia lo declara en vez de estimarlo. La restriccion DIFROL viaja textual y NO se afirma que la cartografia este validada.

**Matiz que el revisor verifico directamente en los metadatos del servicio**: la CAPA declara aprobacion de circulacion DIFROL (res. 183), pero el PRODUCTO DERIVADO sigue sujeto a revision y validacion. La procedencia no afirma lo primero y si declara lo segundo, que es la lectura correcta y la que conviene conservar: tener el insumo aprobado no valida el derivado.

Desviaciones respecto de la lista de campos, declaradas: el registro gano `record_count_matches_expected` (se pidio en el cuerpo y se omitio de la lista), y `restriction_source` quedo como la URL concreta de metadatos en lugar de la frase generica, que es mas verificable.

### Evidencia de M2 — esquema y siembra de zonas

Ejecutada y observada el 2026-10-11. Implementada por `gentle-ai-worker`.

Cambios: `migrations/004_zones.sql` (tabla `zones`, indice GiST, fila heredada `valparaiso-demo` y `GRANT SELECT` a `risk_app`); `embed.go` con la version 4; `assets/assets.go` (`go:embed` del derivado); `internal/zones/seed.go` (`ParseComunas` con rechazos, `Seed` transaccional idempotente, `SeedEmbedded`); `internal/zones/seed_test.go` (38 zonas ordenadas, nombres y provincias cruzados contra las propiedades, 7 casos negativos) y `seed_integration_test.go`; `cmd/server/main.go` (el camino de `migrate` confirma el esquema, siembra, y falla con exit distinto de cero si la siembra falla).

Evidencia real, con contenedor PostGIS desechable propio (`firewatch-verify-pg5`, `127.0.0.1:55436`):

- `migrate` aplica `1,2,3,4` y registra `zones_seeded count=38`, exit 0.
- `zones`: 39 filas. `comuna` 38 con geometria NO nula; `demo` 1 con geometria nula y `kind='demo'`. Codigos de `05101` a `05804`. Tipos de geometria `MULTIPOLYGON 7, POLYGON 31`, que coincide con el derivado de M1.
- `ST_IsValid`: **0 filas invalidas**. `ST_SRID <> 4326`: **0 filas**. Se reportan, no se reparan.
- Idempotencia: la huella md5 de la tabla completa es identica antes y despues de una segunda corrida (`39|4|3e063ef06d5aa0a984cb0628306a17e8`).
- `go test ./services/risk/internal/zones/... -count=1` con `TEST_DATABASE_URL`: las tres pruebas CORREN y pasan.
- `gofmt` limpio; build/vet/test 0 en los siete paquetes con pruebas.
- Limpieza: contenedor y volumen anonimo eliminados; contenedores y volumenes NOMBRADOS del usuario sin cambios.

Comprobacion del padre, para que la decision de una sola fuente no quede en prosa: `grep -c "05[0-9]\{3\}" 004_zones.sql` = **0**. La migracion no duplica ni una comuna.

Desviaciones declaradas: `Seed` liga `kind` como parametro en lugar de fijarlo a `'comuna'`, y liga la geometria como cadena tipada (un `[]byte` viajaria como `bytea` y `ST_GeomFromGeoJSON` lo rechazaria); la mitad de re-siembra de la prueba de integracion necesita un rol con `INSERT` en `zones` (el dueno de la migracion), asi que bajo `risk_app` esa mitad se declara y se deja al dueno en vez de aprobarse en silencio; y el estado "arranca sin datos" queda representado por la columna nullable mas la fila demo, sin escenificar por separado un estado solo-migracion-sin-siembra, porque el runner siembra en la misma invocacion.

### M3 — CONSTRAINTS ARQUITECTONICOS HALLADOS Y DECISION APROBADA

**Una clave foranea NO puede cruzar bases de datos.** `jobs` y `risk` viven en bases distintas (`5432/jobs` y `5432/risk` en `docker-compose.yml`), asi que "sustituir los dos CHECK por una FK a `zones(code)`", tal como se planeo, no se puede hacer en los dos servicios. Opciones evaluadas:

- **(A) Replicar `zones` en la base de `jobs`** con una copia byte-identica del activo y una prueba de igualdad. FK local en cada base, pero el activo aparece dos veces en el arbol.
- **(B) Solo `risk` lleva FK.** `jobs` valida forma (no vacio y patron de CUT) y no puede rechazar en la API un CUT con formato valido pero inexistente; ese trabajo queda encolado y muere en la DLQ de `risk`. Simple, sin duplicacion, pero pierde el rechazo en la frontera que se pedia.
- **(C) `jobs` consulta la API de `risk`.** Acoplamiento de disponibilidad en un camino de escritura. Descartada.
- **(D) Mover el activo al modulo compartido `contracts/gen/geo/`.** AMBOS Dockerfiles ya copian `contracts/gen/` ENTERO y ambos `go.mod` ya tienen el `replace`, asi que los dos servicios embeberian los MISMOS bytes: una sola fuente de verdad, FK local en cada base, y CERO cambios de Dockerfile. El precio es rehacer la ubicacion del embed de M1 y M2.

**DECISION APROBADA: (D).** Es la unica que conserva a la vez la fuente unica y el rechazo en la API de los dos servicios. Refinamientos del orquestador, incorporados como condiciones:

1. **UN SOLO ARTEFACTO.** El GeoJSON completo vive en `contracts/gen/geo/` y los dos servicios embeben los MISMOS bytes. NO se crea un catalogo aparte: el catalogo de codigos, nombres y provincias es una proyeccion en memoria del mismo artefacto, calculada por cada consumidor cuando la necesita. Un catalogo separado seria un segundo origen del mismo dato y su consistencia dependeria de otra prueba que habria que mantener; que `jobs` embeba unos megabytes que no usa es irrelevante comparado con la deriva silenciosa.
2. **La tabla `zones` de `jobs` NO lleva columna de geometria.** Solo `code`, `name`, `province` y `kind`, porque su unico uso es la clave foranea. Las filas salen del MISMO artefacto embebido, ignorando la geometria. Eso expresa bien lo que ese servicio necesita y deja la tabla local mas pequena.
3. **La fila heredada `valparaiso-demo`** (`kind = 'demo'`, sin geometria) debe existir en LAS DOS bases, o las filas historicas de `jobs` dejarian de ser validas al anadir la clave foranea.
4. **Condicion sobre `contracts/gen`**: esa carpeta no comparte LOGICA de dominio. Un activo de DATOS no la viola, pero el paquete debe quedarse en bytes y un accesor minimo, sin comportamiento.
5. **Condicion de evidencia**: la decision se sostiene sobre dos lineas de Dockerfile y dos de `go.mod`. Hay que citarlas TEXTUALMENTE con archivo y numero de linea en la evidencia, porque si una de esas cuatro no fuera como se cree, la decision se cae.
6. **Prueba nueva**, que antes no era posible: el CONJUNTO DE CODIGOS de `zones` debe coincidir entre las dos bases. Es la unica forma de comprobar que la fuente unica se materializo igual en los dos sitios.

Al mover el activo, el sha256 del derivado debe seguir siendo `02ef8e8ff49ab2ca37e5f52c3666d3993fe53730bfd7d7920c933fcec9b5da76`. Si cambia, se PARA y se declara: cambiaria por bytes distintos, no por la mudanza.

### Evidencia de M3 — los lugares que acotan la zona

Ejecutada y observada el 2026-10-11, en dos escritores (lado `risk` y lado `jobs`).

Lado `risk`: migracion 005 que sustituye el CHECK congelado por `batches_zone_id_fkey`; `SupportedZones` ELIMINADO; el camino de lectura consulta la tabla por `ZoneExists`; el contrato pierde los enums de zona.
Lado `jobs`: migracion 002 con el catalogo LOCAL sin columna de geometria; fila heredada; su propia clave foranea; proyeccion del activo compartido y siembra idempotente en `migrate`; el handler del POST consulta el catalogo en lugar de comparar con el literal.

Evidencia real, con contenedor PostGIS desechable propio por lado (`firewatch-verify-pg6` y `pg7`):

- `risk`: versiones `1..5`; `batches_zone_id_check` ELIMINADO y `batches_zone_id_fkey` presente; insertar un lote con zona desconocida FALLA por la clave foranea; `05101` y `valparaiso-demo` se aceptan; reaplicar 005 a mano es no-op.
- `jobs`: versiones `1,2`; el nombre real del CHECK en la base resulto ser `jobs_zone_id_check` (verificado antes de depender de el); `jobs_zone_id_fkey` presente; insertar un job con zona desconocida FALLA por la clave foranea; `05101` y `valparaiso-demo` aceptados.
- Catalogos: `jobs` 39 filas (38 comunas + 1 demo, SIN columna de geometria); `risk` 39 filas (38 con geometria no nula + 1 demo). **Prueba cruzada nueva**: el conjunto de codigos de las DOS bases coincide exactamente (`comm -3` vacio; `TestCatalogueMatchesRiskReference` corrio, no se salto).
- OpenAPI: valida con el comando del CI (exit 0) y `grep -n "enum: \[valparaiso-demo\]"` no devuelve NADA en todo el contrato.
- Limpieza: los dos contenedores y sus volumenes anonimos eliminados; contenedores y volumenes NOMBRADOS del usuario sin cambios.

Auditoria de cierre del padre, `grep` de `valparaiso-demo` en codigo de produccion: solo quedan apariciones legitimas — los cuatro ejemplos del OpenAPI, las dos migraciones 001 CONGELADAS, las filas heredadas de 002 y 004, la prosa de comentarios, y `proposal.js:2`, que queda registrado como octavo lugar para M5. `smoke.py` lo usa como dato de prueba.

Limites declarados: la igualdad de codigos entre bases solo se comprueba si AMBAS variables de entorno estan puestas, asi que CI y compose deben definir `TEST_ZONES_REFERENCE_DATABASE_URL` para que corra de verdad; y el lado `risk` sigue sin distincion permanente/transitorio, de modo que una zona que pase la forma y no exista en el catalogo la rechaza la clave foranea DESPUES de agotar el presupuesto de reintentos. Es la deuda de A.2, ahora con un disparador concreto.

### Evidencia de M4 — endpoints geoespaciales

Cambios: `repository/store.go` con `BBox`, `Zone` y `ListZones` (solo zonas con geometria, filtro opcional por `ST_Intersects`, orden por codigo); `handler/api.go` con la ruta `GET /v1/risk/zones`, la coleccion GeoJSON y `parseBBox` como funcion PURA; `api_test.go` con 15 casos de bbox y 8 de handler; `store_integration_test.go`; y el OpenAPI con el endpoint, los esquemas de zona y UN esquema `Geometry` compartido por `Batch.geometry` y las features de zona.

Evidencia: `gofmt` limpio, build/vet/test verdes y kin-openapi exit 0. Contra PostGIS real: 39 zonas, 38 con geometria; bbox continental `-72,-34,-70,-32` devuelve 36 filas y **`05201` NO aparece**; bbox lejana devuelve 0; y las pruebas de integracion CORREN. Payload medido por el verificador independiente: **1.543.388 bytes (1,47 MiB)**, sin gzip ni recorte. Se declara aceptable para una demo regional y **NO** para produccion; antes de exponerlo ampliamente habria que anadir gzip/ETag, simplificacion o servir por viewport (el endpoint ya acepta bbox).

### Evidencia de M5 — la capa de mapa real

Cambios: `index.html` con el contenedor del mapa, el selector de zona, la leyenda y el bloque de atribucion y restriccion; `proposal.js` con proyeccion propia (equirectangular con correccion por coseno, declarada como esquematica y NO cartografica), particion continente/islas, render SVG, coloreado por nivel y el cableado del selector; `proposal.css`; y una prueba nueva en `gateway_test.go`.

Evidencia: la rejilla y el pin desaparecieron; ni la pagina ni el script contienen ya el literal de zona; la atribucion y la restriccion son **identicas byte a byte** al registro de procedencia (verificado con Python y fijado por la prueba de Go, tras corregir un punto final que las separaba); y contra el catalogo REAL, el continente queda en 36 comunas y las islas en dos recuadros propios. El verificador aporto ademas un contrafactual demoledor: dibujar las 38 en un solo marco comprime el continente a **35x42 px**, que es exactamente lo que los recuadros evitan.

LIMITE DURO DECLARADO: un basemap de teselas queda FUERA por CSP (`connect-src 'self'`, `img-src 'self'`). No hay OpenStreetMap, ni MapTiler, ni MapLibre: la capa se renderiza con proyeccion propia. Incorporar un basemap exigiria vendorizar una biblioteca Y relajar la CSP, y seria una decision DELIBERADA, nunca un accidente. No es una perdida: el requisito de las Bases es representar geograficamente las zonas analizadas, no reproducir un mapa base.

### Verificacion independiente de la feature

16 afirmaciones: **12 CONFIRMADAS, 1 REFUTADA, 1 PARCIAL y 4 inciertas por falta de linea base git**. Lo que aporto, y que no se habria visto leyendo el codigo:

- Re-descargo el servicio y obtuvo bytes IDENTICOS al crudo; reimplemento su PROPIO conversor Esri a GeoJSON y las 38 comunas coinciden; y recomputo todos los hashes de la procedencia, incluida la reconstruccion de la formula del `query_sha256`.
- Escribio SU PROPIA tabla de 29 casos de bbox (todas correctas) y comparo las 38 geometrias servidas contra su propio `ST_AsGeoJSON`.
- **REFUTADA** mi afirmacion de que el guard cruzado protegia en CI: `TEST_ZONES_REFERENCE_DATABASE_URL` no estaba definida en NINGUN sitio del repositorio, asi que la prueba se SALTABA en CI y en `make integration`. Proteccion latente es confianza falsa. **CERRADO**: la variable quedo en `jobs-test`, mas la dependencia `risk-test-migrate` sin la cual `make integration` FALLABA en volumen nuevo; y la prueba ahora CORRE, declarando la identidad de cada conexion (`jobs_app/jobs_test` y `risk_app/risk_test`) y leyendo 39 codigos por lado. El peer cerro ademas un hueco residual: sin esa identidad y sin exigir conjuntos NO vacios, la igualdad podia ser trivialmente cierta.
- **PARCIALMENTE REFUTADA** mi afirmacion mas importante: una CARRERA podia pintar la zona recien seleccionada con el nivel de la zona anterior. **CERRADO** con un contador de secuencia por tipo de peticion mas la comprobacion de que la respuesta pertenece a la zona pedida, verificado con ocho comprobaciones.
- Deriva cerrada: la lista `INSET_ZONE_CODES` hardcodeada (CUARTA lista literal eliminada; ahora los recuadros salen solo de la regla de longitud y el reparto quedo identico), la descripcion obsoleta de `POST /v1/jobs` en el OpenAPI, dos comentarios con la ruta vieja del activo, y `03-risk.md` documentando `SupportedZones` y `SupportedZone`, que ya no existen.
- **BLOQUEADOR DE CI PREEXISTENTE, CORREGIDO**: `_go-service.yml` ejecuta `test -z "$(gofmt -l .)"` y `services/gateway/internal/handler/gateway.go` tenia los imports desordenados desde el 2026-10-07. `gofmt -l services/gateway` queda VACIO y la prueba del gateway pasa.

Inciertas por falta de linea base git, declaradas como INCIERTAS y no como confirmadas: que las migraciones 001 nunca se reescribieran, que las migraciones nuevas sean nuevas de verdad, y que la CSP no haya cambiado.

No verificado, y declarado: render en navegador real, y la ruta completa end-to-end por el gateway con Keycloak.

## Unidad A.2 — integridad, salud honesta y requisito de despliegue

Alcance autorizado el 2026-10-11. **Deja de ser limpieza de higiene y pasa a ser REQUISITO DEL DESPLIEGUE.** Las dos cosas que morderian la primera vez que esto corra en cloud con un volumen nuevo son la salud que MIENTE mientras el servicio descarta trabajos, y el broker de `risk` sin distincion permanente/transitorio. El disparador concreto ya declarado en M3 —una zona que pase la regla de forma y NO exista en el catalogo, rechazada por la clave foranea TRAS agotar los reintentos— es exactamente el escenario de una demo en vivo.

### Tareas

1. [x] Distincion permanente/transitorio en el broker de `risk` (A.2a).
2. [x] Profundidad de la cola muerta como metrica y salud que no miente (A.2b).
3. [x] Integridad de la tabla `models` (A.2c).
4. [x] Verificacion del `params_hash` contra la base, en el arranque (A.2c).
5. [x] Prueba obligatoria de que un error NO tipificado SIGUE reintentando (A.2a).

### Nota de diseno sobre la duplicacion

El mecanismo permanente/transitorio vive en CADA servicio y no en un paquete compartido. No es un descuido: el proyecto ya decidio por escrito que los adaptadores de seguridad y mensajeria son LOCALES y no un SDK compartido, y que un cambio en ellos se prueba en los DOS consumidores. `contracts/gen` es un modulo de contratos y DATOS, sin logica. Asi que el patron se duplica a proposito, citando esa decision.

### Evidencia de A.2

Ejecutada el 2026-10-11 en tres rebanadas, cada una con su escritor y verificacion contra PostGIS desechable propio.

**A.2a — permanente vs transitorio en el broker de `risk`.** Mismo mecanismo conservador que el de `jobs` (interfaz `Permanent() bool`, `errors.As`, backoff `[1s,2s,4s,8s]` y `handleWithRetries` extraida para poder probarla). Se tipifica como PERMANENTE solo lo irrepetible: violacion de clave foranea `23503` (zona desconocida o version de modelo no registrada), evaluacion que no pasa `Validate()` y suplantacion de politica. Se deja TRANSITORIO a proposito: fallos del proveedor, rechazos de `featuresFrom`, errores del clasificador y cualquier otro fallo del store, porque el proveedor se re-consulta en cada intento y un estado pasajero podria no repetirse. La puerta de forma previa al bucle queda intacta y sigue mandando a la DLQ de inmediato.
Prueba obligatoria: un error NO tipificado produce exactamente CINCO intentos con el backoff exacto, y un permanente produce UN intento y CERO pausas. Contra PostGIS real, las dos claves foraneas se tipifican permanentes.

**A.2b — salud honesta.** `firewatch_dead_letter_depth` como gauge real, por declaracion pasiva de la cola muerta y refresco cada 3 s, con representacion explicita de DESCONOCIDO cuando no se puede verificar; y `/health/ready` con 503 por base caida, broker caido, profundidad DESCONOCIDA o backlog por encima de `DLQ_READY_MAX_DEPTH` (default 0). En los DOS servicios con broker. Decision del escritor que se adopta: la profundidad desconocida tambien da 503, porque una cola que no se puede verificar no es un "listo" honesto. Se conservaron los nombres de metrica existentes.
Compatibilidad verificada LEYENDO `smoke.py`: su unica comprobacion de readiness es la linea 93, ANTES de inyectar el evento malformado, y ademas la responde el GATEWAY, no el consumidor. No rompe el camino documentado.
HALLAZGO DECLARADO PARA LA UNIDAD DE CLOUD: el gateway responde `/health/ready` por si mismo, sin consultar a los servicios de atras, asi que un health check de ingress apuntado al gateway daria 204 aunque `risk` y `jobs` estuvieran caidos.

**A.2c — integridad del registro.** Migracion 006 con `CHECK (kind <> 'placeholder' OR active = false)` y el indice unico parcial de un solo activo, con pre-check sobre datos reales ANTES de imponerlas (0 violaciones, 1 activo). Y verificacion FAIL-CLOSED en el arranque: la version seleccionada debe existir en `models` y, cuando hay expectativa congelada, su `params_hash` debe coincidir; si no, el servicio NO ARRANCA. Eso cierra de raiz el escenario que motivo la unidad: arrancar sano mientras descarta todo. No se exige `active` (expresa el ciclo de vida del registro, no la seleccion del servicio) y el camino de `migrate` retorna antes de la verificacion.
Evidencia: PostGIS real migrado a versiones `1..6`; la 006 aplicada dos veces (idempotente); rechazo COMPROBADO de un segundo activo por el indice unico y de un placeholder activo por el CHECK; y con `TEST_DATABASE_URL`, 93 pruebas PASS, 0 FAIL, 0 SKIP.

**Verificacion independiente de A.2 realizada en el relevo del 2026-10-11: A.2b REFUTADA en ambos servicios; correccion pendiente de aprobacion.**

- RabbitMQ real `4.1.8-management`: durante bloqueo del handler de 8.001 s, profundidad real DLQ=1, cache=0 y decision de readiness saludable en risk y jobs. Tras liberar, refresco aproximadamente a 9.104 s: cache=1 y `dead_letter_backlog`. La promesa de refresco cada 3 s NO se sostiene durante procesamiento sincronico. Causa: `risk/internal/messaging/broker.go:151–165` y `jobs/internal/messaging/broker.go:152–166`. El consumidor permanecio original; solo el relay outbox fue sustituido en copias temporales. PASS del probe significa defecto reproducido, no conducta correcta.
- PostGIS `17-3.5`, contenedor propio en loopback: runner dos veces exit 0, versiones 1..6, 38 comunas sembradas; 006 reaplicada dos veces exit 0. Segundo activo rechazado por `models_single_active`; placeholder activo rechazado por `models_placeholder_not_active_check` (ambos psql exit 3).
- Binario original risk: modelo ausente, hash NULL, hash incorrecto y DB inaccesible producen exit 1 antes del broker (0.047–0.063 s). Hash correcto supera registro y sigue ejecutando; migrate con hash incorrecto exit 0, bypass intencional. HTTP baseline NO verificado: el harness consulto 18089 aunque el servidor usa 8080.
- `go test ./... -count=1`, vet de risk/jobs y build de risk correctos en copias. Risk con DB: 93 PASS, 0 FAIL, 0 SKIP. Primer intento del harness: 87 PASS/7 FAIL por rol/schema ausentes y probe Rabbit residual; corregido solo en entorno temporal y repetido. Integracion DB jobs y perdida/reconexion live NO ejercitadas.
- Evidencia: `C:/Users/rikar/AppData/Local/Temp/firewatch-a2-es67qlx5/`, fuentes de probes, `db_verify.py` y `db-command-output.txt`. Contenedores propios Rabbit/PostGIS eliminados; docker ps final vacio. Verificador no escribio archivos en el repositorio. Solo este registro ODD fue actualizado por el coordinador.
- Sin commit ni revision nativa; trazabilidad ODD incompleta por decision del usuario. Cloud bloqueado y frontend pendiente aprobacion de particion.

### Deuda declarada al cerrar esta sesion

- Verificacion independiente A.2 completada con defecto de salud confirmado en ambos brokers; correccion y re-verificacion pendientes.
- El gateway responde readiness sin consultar a los servicios de atras.
- Dos advertencias PREEXISTENTES de pi-lens en `verify_structure.py`, no corregidas porque cambiar el manejo de errores es cambio de conducta.
- `VerifyRegisteredModel` vive en el paquete `classify` y no en `cmd/server`, porque un `main_test.go` habria exigido una novena superficie.

## Estado

**Feature CERRADA el 2026-10-11** en cinco rebanadas (M1..M5), con verificacion independiente y re-baseline estructural final. Sin commit y sin revision nativa: `nogit` sigue vigente, asi que la trazabilidad ODD depende de ESTE documento y de su espejo en memoria.

- M1 adquisicion y procedencia: CERRADA.
- M2 esquema y siembra de zonas: CERRADA.
- M3 los lugares que acotan la zona (siete, mas el literal de la pagina, que se resolvio en M5): CERRADA.
- M4 endpoints geoespaciales y contrato: CERRADA.
- M5 capa de mapa real en la pagina servida: CERRADA.
- Verificacion independiente: 12 confirmadas, 1 refutada y 1 parcial, AMBAS con su cierre, y 4 inciertas declaradas como inciertas.
- Re-baseline estructural: `verify_structure.py` en `exit 0` con `errors: []`, y ahora ignora caches generados por diseno, para que el inventario describa FUENTE y no artefactos locales.

DEUDA DECLARADA QUE QUEDA:

- El broker de `risk` sigue sin distincion permanente/transitorio (Unidad A.2). Tiene ahora un disparador concreto: una zona que pase la regla de forma y no exista en el catalogo la rechaza la clave foranea DESPUES de agotar el presupuesto de reintentos.
- Dos advertencias PREEXISTENTES de pi-lens en `verify_structure.py` (L83, `json.loads` sin manejo de error, y el bloque de imports), NO corregidas a proposito: cambiar el manejo de errores es un cambio de conducta y exige su propia rebanada.
- El catalogo completo se sirve sin gzip (~1,47 MiB). Aceptable para demo, NO para produccion: haria falta gzip/ETag, simplificacion o servirlo por viewport (el endpoint ya acepta bbox).
- No se pudo ejercitar el render en navegador real ni la ruta completa end-to-end por el gateway con Keycloak.
- Inciertas por falta de linea base git, declaradas como INCIERTAS: que las migraciones 001 nunca se reescribieran, que las migraciones nuevas sean nuevas de verdad, y que la CSP no haya cambiado.

SIGUIENTE EN COLA: Unidad A.2 (limpieza de integridad). No se empieza sin autorizacion.
