# Contrato de datos, procedencia y clasificador FireWatch

## Objetivo
Fijar las formas canonicas de datos, la interfaz de proveedor de datos y la interfaz de clasificacion, de modo que el sistema acepte indistintamente una API del cliente, una base privada o un descargador de fuentes publicas, y que el nivel Bajo/Medio/Alto pueda provenir de una regla academica documentada o de un modelo entrenado, sin refactorizacion.

## Alcance y criterios
- Solo se tocan el servicio `risk` y `contracts`. `jobs`, `frontend` y el despliegue quedan intactos.
- No se descargan datos reales: eso se difiere por decision del usuario. La regla academica se prueba con fixtures sinteticos y reloj fijo.
- `not_evaluated` y el CHECK `model_version = 'demo-fixture-v1'` deben desaparecer. Bajo/Medio/Alto pasa a existir de verdad en el sistema.
- `Level` incluye `desconocido` y el contrato prohibe mapear datos faltantes a `bajo`. Un dato ausente nunca es riesgo bajo.
- La regla academica se declara explicitamente como regla propia no oficial, conforme a lo que permiten las Bases cuando no hay etiquetas suficientes.
- Las formas compartidas viven bajo `contracts/gen/` porque los Dockerfiles copian `contracts/gen/` con `GOWORK=off`. La logica de dominio NO va en `contracts`.
- Los datos viajan por referencia, no dentro de los mensajes (principio ya declarado en `contracts/events/v1/README.md:15`).

## Tareas
1. [x] Formas canonicas en `backend/contracts/gen/observations/v1/`: `Observation`, `ObservationSet`, `Provenance` (con `retrieved_at`, `cutoff_at`, `payload_sha256`, `normalized_sha256`, `complete`, `source_kind`, `origin`) y `SourceDescriptor` (con `License`, `Attribution`, `Priority`).
2. [x] Interfaz `Provider` (`Describe()` + `Fetch(ctx, window)`) en `risk/internal/source/`, con registro de proveedores. `Describe()` es la fuente unica de la tabla de trazabilidad.
3. [x] Interfaz `Classifier` (`ID()`, `Kind()`, `Classify(ctx, Features) -> Assessment`) con `Assessment{Level, Factors, Rationale}`. `Level` incluye `desconocido`.
4. [x] Implementar `academic-rule-v1` sobre el contrato de features derivado de las cinco variables de las Bases (temperatura, humedad, viento, vegetacion/cobertura, incendios historicos), documentando umbrales y procedencia como regla no oficial.
5. [x] Sustituir el CHECK `model_version = 'demo-fixture-v1'` por una tabla `models` (id, version, kind, approved_at, metrics, active) con clave foranea, mas su migracion. Cerrada en la Rebanada B; la tabla gano ademas `params_hash` por refinamiento posterior del orquestador.
6. [x] Actualizar `contracts/openapi/firewatch-v1.yaml` donde fija los enums de `model_version`, `demo`, `risk_level` y `geometry.type`; y relajar el literal `zone_id = 'valparaiso-demo'` en la validacion de transporte. Cerrada en la Rebanada C: los tres enums obsoletos quedaron corregidos y ademas se declararon `factors`, `rationale` y `policy`; el literal de transporte se sustituyo por la allowlist declarada `SupportedZones`. `geometry.type` quedo igual a proposito: `Point` sigue siendo correcto, porque PostGIS guarda un punto. La tarea 7 cubre el resto.
7. [x] Pruebas: registro de proveedores, precedencia determinista publico sobre privado, `desconocido` que no degrada a `bajo`, regla academica con fixtures y reloj fijo, y migracion de `models`. Las cuatro primeras quedaron en la Rebanada A; la migracion de `models` cerro en la Rebanada B con pruebas unitarias de la migracion y con la ejecucion real del runner sobre dos bases.
8. [x] Verificacion end-to-end: `risk` publica un nivel real con factores y justificacion en lugar de `not_evaluated`. Cerrada en la Rebanada B: ver la evidencia abajo.

## Evidencia y limites de ejecucion
- Pruebas deterministas del servicio `risk` en verde; contrato OpenAPI coherente con las formas nuevas.
- La ingesta real de datos y los adaptadores de API/DB quedan FUERA de este work unit: se implementan cuando llegue el acceso a datos.
- `nogit`: el primer commit del repositorio sigue sin autorizar. Este work unit no produce commit ni revision nativa; la trazabilidad ODD queda incompleta por decision del usuario y asi debe declararse.
- No se ejecuta `backend/scripts/init_env.py` en el arbol real: escribe en ruta fija con O_CREAT|O_EXCL y generaria credenciales nuevas que dejarian huerfano el volumen de Postgres existente.

## Evidencia de la Rebanada A (contrato e interfaces)

Ejecutado y observado el 2026-10-11. Sin commit: `nogit`, el primer commit sigue sin autorizar, asi que la trazabilidad ODD de esta feature queda incompleta.

- `gofmt -l` sobre los paquetes nuevos: sin hallazgos.
- `go build ./...` y `go vet ./...`: correctos en `contracts/gen` y en `services/risk`.
- `go test ./...`: verde en `contracts/observations/v1`, `risk/internal/source` y `risk/internal/classify`, sin regresiones en los paquetes preexistentes de `risk`.
- 35 pruebas de nivel superior en verde (4 de contrato, 12 de `source`, 19 de `classify`), mas 14 subpruebas dentro de las pruebas parametrizadas de la regla.
- Ningun archivo preexistente fue editado: `demo.go`, `store.go`, `handler/api.go`, `broker.go`, `001_initial.sql`, `cmd/server/main.go`, el OpenAPI y `events.go` conservan su contenido original.
- `backend/go.work.sum` (452 bytes) fue reescrito por el propio toolchain de Go al compilar y probar. Solo contiene hashes de modulos externos y ninguna mencion a los modulos del proyecto. Es un efecto secundario generado, no una edicion deliberada, y queda declarado aqui por estar fuera de las superficies acordadas.
- La migracion de `models` no se prueba en esta rebanada: pertenece a la Rebanada B, donde el runner de migraciones debe reescribirse para conocer la version 2.

## Decisiones tomadas durante la Rebanada A

- `Provenance` gano el campo `Priority` ademas de `Origin`. Motivo: `Merge` necesita la precedencia a partir del conjunto por si solo, sin consultar el registro, y `ProvenanceFrom` copia ambas desde el descriptor para que las dos copias no puedan divergir. Es un cambio de contrato respecto de la lista original de campos.
- La regla academica es mas estricta que "faltante -> desconocido": tambien se abstiene ante instantes ausentes, instantes incoherentes (observacion posterior a la evaluacion), valores no finitos y datos con mas de 6 horas de antiguedad.
- La regla nunca lee el reloj del sistema: ambos instantes entran por `Features`, y una prueba lo demuestra usando instantes pasados que, si la regla consultara el reloj real, serian clasificados como vencidos.
- Se anadieron al contrato los nombres canonicos de variables y `VariableNames()`, para que un proveedor nuevo no necesite un caso especial en la clasificacion.
- `Merge` excluye los conjuntos marcados como incompletos y los reporta en `Skipped`, en lugar de descartarlos en silencio.

## Rebanada B — runner de migraciones, tabla `models` y persistencia del nivel real

Alcance: tareas 5 y 8 mas el bloqueador 9 del runner de migraciones. Autorizada por el orquestador.
Sin commit y sin revision nativa: `nogit` sigue vigente, el primer commit del repositorio continua sin autorizar.

### Condiciones no negociables

- (a) `001_initial.sql` no se edita nunca: la version 1 ya esta registrada en `schema_migrations`, asi que un cambio ahi no se volveria a ejecutar y seria silenciosamente inefectivo en cualquier base existente. Todo va en `002_models.sql` con `ALTER TABLE`.
- (b) El runner queda minimo: ordenado por version, idempotente, no-op sobre una base ya al dia y sin convertirse en un framework de migraciones.
- (c) `demo` pasa a significar "entrada sintetica o demostrativa" y se DERIVA de la procedencia que aporto los datos, no de una constante. El `CHECK (demo)` cae precisamente porque congelaba ese valor.
- (d) Factores y justificacion viven en `batches`, porque son propiedad de cada evaluacion. `models` guarda solo identidad y ciclo de vida.

### Decisiones tomadas durante la Rebanada B

1. `models` se define como `id uuid PRIMARY KEY`, `version text NOT NULL UNIQUE`, `kind text NOT NULL`, `params_hash text`, `approved_at timestamptz`, `metrics jsonb NOT NULL DEFAULT '{}'`, `active boolean NOT NULL DEFAULT false`. `batches.model_version` pasa a ser clave foranea a `models(version)`. El `UNIQUE(version)` es lo que hace cumplir "una version es inmutable": no puede existir dos veces la misma version con parametros distintos.
2. `params_hash` es el hash determinista de los parametros efectivos del clasificador de esa version (`classify.ParamsHash` sobre `AcademicRuleV1.Parameters()`). Identifica un comportamiento, no una etiqueta.
3. Se siembran dos versiones y una red de seguridad. `academic-rule-v1` es la version que ejecuta el servicio, con su hash congelado. `demo-fixture-v1` es el marcador heredado que ya existe en bases creadas desde la 001: no tiene conjunto de parametros reconstruible, asi que su `params_hash` queda nulo. La red de seguridad registra cualquier otro `model_version` historico, porque una clave foranea no se puede crear mientras existan filas huerfanas.
4. `approved_at` queda nulo para `academic-rule-v1`: es una regla propia declarada no oficial, y `metrics` registra `official=false`. La columna queda para un modelo entrenado que si tenga una aprobacion que registrar.
5. `risk_level` es `NOT NULL` con `CHECK` sobre los cuatro niveles canonicos. Las filas anteriores a esta migracion se rellenan con `desconocido` y nunca con `bajo`: nadie las clasifico, y ese es exactamente el valor honesto.
6. El contrato gana `Synthetic` en `SourceDescriptor` y en `Provenance`, copiado por `ProvenanceFrom`. Es la unica forma honesta de derivar `demo` del tipo de proveedor que aporto los datos, sin parsear prosa de licencia ni dejar una constante en el sitio de escritura.
7. El instante de evaluacion de un lote es el instante de recuperacion del conjunto de datos (`Provenance.RetrievedAt`), no el reloj del sistema. Es coherente con la invariante de la Rebanada A de que la regla nunca lee el reloj, y en produccion recuperacion y clasificacion ocurren en la misma peticion, asi que coincide con el reloj. Con el proveedor fixture el conjunto es internamente coherente y produce un nivel real en lugar de abstenerse por una antiguedad artificial. Cambiar a reloj de sistema es una linea.
8. `kind = 'placeholder'` para la version heredada: ningun clasificador de este servicio la produce ni la producira.
9. `MODEL_ID` es opcional y por defecto `academic-rule-v1`. No se hace obligatoria porque `docker-compose.yml` no la define y esta fuera de las superficies permitidas: exigirla dejaria el stack existente sin arrancar.

### Superficie documental que queda desactualizada

`backend/docs/code-explanation/06-database.md` describe `migrations/embed.go:1-6` y la rama `migrate` de `cmd/server/main.go:48-75`. Ambas cambian en esta rebanada. El archivo esta fuera de las superficies de edicion permitidas, asi que no se actualiza aqui y queda declarado como pendiente.

### Evidencia de la Rebanada B

Ejecutada y observada el 2026-10-11. Sin commit y sin revision nativa: `nogit` sigue vigente, el primer commit del repositorio continua sin autorizar, y la trazabilidad ODD de esta feature queda incompleta por decision del usuario.

Entorno de verificacion: PostgreSQL 17 + PostGIS 3.5 en un contenedor desechable de nombre y puerto propios (`firewatch-verify-pg`, `127.0.0.1:55432`), creado al efecto y eliminado al terminar. El stack del usuario no se levanto, no se modifico y sus volumenes (`firewatch_postgres-data`, `firewatch_rabbitmq-data`, `firewatch_prometheus-data`) quedaron intactos. Nunca se ejecuto `docker compose down`, ni con ni sin `-p`.

#### Caso 1: migracion 002 desde cero

- `go run ./cmd/server migrate` aplico la version 1 y la version 2: `schema_migrations` quedo en `{1, 2}`.
- Una segunda ejecucion del mismo comando termino en 0 sin aplicar nada: sobre una base al dia el runner es un no-op.
- `pg_constraint` sobre `batches` ya NO contiene `batches_model_version_check` ni `batches_demo_check`, y si contiene `batches_model_version_fkey` (clave foranea a `models(version)`), `batches_risk_level_check` con los cuatro niveles canonicos y el preexistente `batches_zone_id_check`.
- Insertar un lote con `demo = false` y `model_version = 'academic-rule-v1'` es aceptado. Es la prueba directa de que los dos CHECK congelados ya no existen, y no solo de que se intento borrarlos: si el nombre autogenerado hubiera sido otro, el `DROP CONSTRAINT IF EXISTS` habria sido silencioso y este insert habria fallado.
- Insertar `model_version = 'does-not-exist'` falla con `violates foreign key constraint "batches_model_version_fkey"`; insertar `risk_level = 'not_evaluated'` falla con `violates check constraint "batches_risk_level_check"`.
- `models` contiene `academic-rule-v1` (kind `rule`, `params_hash = e17aaa5600eb67c9a91340e90057aec100f4e80ef0895662f373cffee66564c7`, `active = true`, `approved_at` nulo) y `demo-fixture-v1` (kind `placeholder`, `params_hash` nulo, inactivo).
- Reaplicar `002_models.sql` directamente con `psql` es un no-op completo: exit 0, avisos `already exists, skipping` / `does not exist, skipping`, `UPDATE 0`, `INSERT 0 0`.

#### Caso 2: migracion 002 sobre una base que ya estaba en la version 1

Este es el caso que podia romper, y por eso se reproduce de forma fiel.

- Base construida como la dejaba el runner anterior: `schema_migrations` con la version 1, `001_initial.sql` aplicado y dos lotes heredados con `model_version = 'demo-fixture-v1'` y `demo = true`.
- El runner real aplico solamente la version 2 y dejo `schema_migrations` en `{1, 2}`; la segunda ejecucion fue no-op.
- Los dos lotes heredados conservan su `model_version` y su `demo`, y su `risk_level` quedo en `desconocido` -- nunca en `bajo` -- con `factors` y `rationale` nulos. Nadie los clasifico, y ese es el valor honesto.
- La clave foranea se pudo crear porque la version heredada quedo registrada en `models` como `placeholder`.

#### Camino defensivo

En una tercera base con un `model_version` historico distinto de `demo-fixture-v1` (obtenido quitando el CHECK en la base desechable para simularlo, porque la 001 congelada lo hacia imposible), la red de seguridad de la 002 lo registra como `placeholder` y la clave foranea se crea igual. El lote conserva su valor y se rellena a `desconocido`. Sin esa red, la clave foranea habria fallado por filas huerfanas.

#### Defecto real hallado y corregido por esta verificacion

La primera ejecucion en base real fallo con `SQLSTATE 42804`: `column "approved_at" is of type timestamp with time zone but expression is of type text`.

- Causa: en un `VALUES` de varias filas, PostgreSQL resuelve un literal `NULL` sin tipo como `text` cuando todas las filas de esa columna son nulas, y `approved_at` es `timestamptz`.
- Correccion: los `NULL` de `params_hash` y `approved_at` quedaron casteados de forma explicita (`NULL::text`, `NULL::timestamptz`), con el motivo escrito en el comentario de la migracion.
- Importa declararlo: las pruebas estaticas de `migrations_test.go` no podian detectarlo y pasaban en verde. Solo la ejecucion contra un PostgreSQL real lo hizo visible, que es exactamente por lo que la entrega exigia los dos casos en base real. La prueba en base real no fue ceremonia: encontro un defecto que habria roto el arranque del servicio aprobado.

#### Pruebas y verificacion independiente

- `gofmt -l`, `go build ./...` y `go vet ./...` limpios en `backend/contracts/gen` y en `backend/services/risk`.
- `go test ./... -count=1` verde en ambos modulos.
- Con `TEST_DATABASE_URL` apuntando a la base desechable ya migrada, la suite completa tambien queda verde y las dos pruebas de integracion dejan de saltarse: `TestBatchDeduplicationAndOutOfOrderPublication` y `TestPublisherPersistsAndReadsBackTheFixtureAssessment`.
- La prueba de composicion persistio un lote real y se leyo de vuelta: `academic-rule-v1`, `demo = true`, `risk_level = alto`, cinco factores (`temperatura` 0.2125, `humedad_relativa` 0.25, `viento` 0.175, `cobertura_vegetal` 0.1375, `incendios_historicos` 0.12) y una justificacion que declara la regla como propia no oficial. El puntaje 0.895 sobre 1.000 se reprodujo con aritmetica independiente en Python, fuera del codigo.
- Verificacion independiente por una cabeza distinta a la que escribio, con contenedores propios en otros puertos y limpieza de los suyos: las nueve afirmaciones verificables fueron CONFIRMADAS, incluida la reproduccion independiente del hash (`e17aaa56...6467`) en Python, y tres de sus hallazgos se resolvieron o se declararon.
- Hallazgo LOW de esa verificacion, ya cerrado: el guard dorado del hash solo hacia `strings.Contains`, asi que el hex podria haber aparecido en un comentario mientras el valor almacenado se quedaba atras. Ahora el literal se extrae con una expresion regular y su cantidad se fija en exactamente uno. La re-verificacion delta lo confirmo con `go test -overlay` sobre copias manipuladas fuera del repositorio: con el hex correcto solo en un comentario y otro valor en el INSERT, la prueba falla, y falla por las dos vias, el conteo y el valor. Los otros diez hashes del arbol quedaron identicos, asi que el cambio quedo confinado a ese archivo de prueba.

#### Hallazgos declarados que NO se corrigen en esta rebanada

- **La derivacion de `demo` es correcta pero hoy inerte en el camino vivo.** `internal/service/demo.go:14` rechaza todo evento con `event.Demo == false`, y `internal/messaging/broker.go:88` lo invoca antes del publicador. Mientras ese literal de transporte exista, al publicador solo le llega la entrada sintetica y la columna `demo` solo puede valer `true` en produccion. La condicion (c) se cumple en el sitio de escritura -- que es lo que exigia --, pero su efecto solo se vuelve observable cuando la rebanada C relaje la validacion de transporte.
- **`MODEL_ID` se valida contra el registro en codigo, no contra la tabla `models`.** Un clasificador registrado en Go pero ausente de `models` arranca y falla recien al publicar, por clave foranea. Decision deliberada de esta rebanada: no agregar un modo de fallo nuevo al arranque del servicio aprobado.
- **El `params_hash` no se compara en tiempo de ejecucion.** La inmutabilidad de una version se sostiene con el guard dorado de las pruebas, no con una comprobacion de arranque. La Rebanada A.1 lleva la referencia de politica estructurada a cada evaluacion, y ahi se cierra ese circuito.
- **Artefactos desactualizados fuera de las superficies permitidas**: `backend/contracts/openapi/firewatch-v1.yaml:146,160` sigue enumerando `demo-fixture-v1` y `not_evaluated`, y `backend/docs/code-explanation/*` sigue describiendo el placeholder. Son rebanada C o posterior. (`scripts/smoke.py:99` y `backend/README.md` describian lo mismo, pero la Unidad 5 los corrigio y ya no estan en esta lista.)
- **`backend/services/risk/.env.example` no se pudo editar.** La politica de seguridad del arnes bloquea cualquier ruta `.env*`, tambien para la herramienta de escritura, y no se intento ninguna via alternativa para no eludir un control de seguridad. Es solo documental: `MODEL_ID` funciona sin declararla porque el codigo tiene por defecto `academic-rule-v1`. Contenido propuesto, a agregar al final: `# Selects the classifier version. Optional: it defaults to academic-rule-v1.` y `MODEL_ID=academic-rule-v1`.
- **Generados por el toolchain, fuera de las superficies**: `.codegraph/` (indice y demonio de CodeGraph, ya ignorado) y `.atl/` (registro y cache de skills de pi). `backend/go.work.sum` NO fue reescrito en esta rebanada: su fecha es anterior a la primera escritura del work unit.

## Unidad 5 — desbloqueo de la CI y ruta completa end-to-end

Ejecutada y observada el 2026-10-11, despues de aceptada la Rebanada B. Sin commit y sin revision nativa por la misma razon: sigue vigente `nogit`.

### Cambios

- `backend/scripts/smoke.py`: la afirmacion del nivel deja de ser una igualdad con `not_evaluated` y pasa a ser pertenencia al conjunto canonico `{bajo, medio, alto, desconocido}`, declarado como constante del modulo. Motivo: el nivel del lote sintetico se mueve con los parametros de la regla, asi que fijar un valor concreto seria fragil ante cualquier calibracion legitima y no probaria nada adicional. Se agrego ademas una afirmacion de que el lote publicado trae una justificacion no vacia: era exactamente el hueco que la tarea 8 no cubria por la ruta completa, porque solo se habia verificado a nivel de composicion.
- `backend/README.md`: se documenta `MODEL_ID` -- selecciona la version del clasificador que ejecuta `risk`, es opcional porque el valor por defecto esta en el codigo, y la version elegida debe tener fila en `models`. Ademas se corrigio la frase de cabecera, que afirmaba `model_version: demo-fixture-v1` y `risk_level: not_evaluated`: quedo falsa con la Rebanada B y ademas subestimaba el sistema, porque negaba justo la capacidad de clasificacion que las Bases piden.

### Ruta completa ejecutada

Proyecto de compose propio (`-p firewatch-verify`), nunca el del usuario. Ocho servicios levantados, los dos migradores en 0, realm importado, y `python scripts/smoke.py` en verde:

```
PASS: JWT/RBAC, input validation, idempotency, async publication, PostGIS, history and audit
PASS: malformed event sent to dead-letter queue
```

Lectura de vuelta por dos vias independientes. Desde PostgreSQL: `model_version = academic-rule-v1`, `demo = t`, `risk_level = alto`, cinco factores y la justificacion completa. Desde el gateway, con un JWT real emitido por Keycloak: HTTP 200 con `risk_level = "alto"`, cinco factores y la justificacion. El hueco declarado en la Rebanada B queda cerrado: la ruta completa llega al nivel real, lo persiste en PostGIS y lo devuelve.

### Defecto preexistente hallado, fuera de las superficies de esta unidad

**El stack no arranca desde cero, y es independiente de este work unit.**

`backend/deploy/local/firewatch-realm.json:16` declara `postLogoutRedirectUris` dentro del cliente, campo que Keycloak 26.5.2 (la imagen fijada en `docker-compose.yml`) ya no acepta en `ClientRepresentation`:

```
ERROR: Failed to run import
ERROR: Unrecognized field "postLogoutRedirectUris" (class org.keycloak.representations.idm.ClientRepresentation)
 at [Source: ...; line: 16, column: 32]
```

Consecuencia real: en una base de datos nueva la importacion del realm falla, `identity` entra en bucle de reinicio y ningun token se puede emitir, asi que el arranque documentado en el propio README (`init_env.py` -> `compose up` -> `smoke.py`) no funciona en un checkout limpio. El stack del usuario solo funciona porque su volumen ya contiene el realm importado el 2026-10-06, y el README ya advierte que cambiar el archivo de importacion no actualiza un realm ya inicializado. Es un defecto silencioso: no se nota hasta que alguien levanta el proyecto en una maquina o un volumen nuevos.

Correccion necesaria, en un archivo que esta fuera de las superficies permitidas: mover el valor a los atributos del cliente, es decir `attributes["post.logout.redirect.uris"] = "http://localhost:8080/"`, y eliminar la propiedad de nivel de cliente. El README ya describe esa misma URI de salida como configuracion del cliente, asi que el valor no cambia.

### Como se obtuvo la evidencia sin tocar ese archivo

Para no dejar la unidad sin evidencia y sin modificar ningun archivo entregado, se ejecuto el mismo stack con una copia del realm corregida y montada desde fuera del repositorio:

```
docker compose -f docker-compose.yml -f <override externo> -p firewatch-verify up -d
```

El override usa la etiqueta `!override` de Compose para reemplazar el montaje del realm del servicio `identity`. `docker-compose.yml` no se edito, el realm entregado no se edito, y la copia vivia en un directorio temporal que se borro al terminar. Con esa unica correccion la importacion pasa (`Realm 'firewatch' imported`, `Import finished successfully`) y la ruta completa se ejecuta. El resto de lo ejercitado -- gateway, jobs, RabbitMQ, risk, migraciones, publicacion, DLQ, PostGIS -- es codigo entregado sin cambios.

### Limpieza e impacto

`down -v` del proyecto propio, imagenes `firewatch-verify-*` eliminadas y directorio temporal borrado. Verificado despues: los once contenedores del usuario con el mismo estado que antes de empezar, sus cuatro volumenes intactos, y los tres proyectos de compose sin cambios. Nunca se ejecuto `down` sobre el proyecto `firewatch`.

### Nota fuera de superficies

`.gitignore` no ignora `__pycache__/` ni `*.pyc`. La validacion de sintaxis (`python -m py_compile`) creo `backend/scripts/__pycache__/smoke.cpython-311.pyc`, que se elimino al cerrar la unidad. Conviene agregar el patron antes del primer commit: hoy esos archivos entrarian al repositorio, y el repositorio todavia no tiene ninguno.

## Unidad 5.1 — el arranque limpio

Ejecutada y observada el 2026-10-11. Sin commit y sin revision nativa: `nogit` sigue vigente.

### Cambios

- `backend/deploy/local/firewatch-realm.json`: el cliente deja de declarar `postLogoutRedirectUris` en el nivel superior y su valor pasa a `attributes["post.logout.redirect.uris"]`. Nada mas cambia: `redirectUris`, `webOrigins`, `protocol`, los dos `protocolMappers`, los roles, los dos usuarios con sus marcadores de contrasena, `sslRequired` y `accessTokenLifespan` quedan igual.
- `.gitignore`: se cierran los huecos que habrian entrado al primer commit. `__pycache__/`, `*.pyc`, `.atl/`, `node_modules/`, `Thumbs.db`, `.DS_Store`, y `portable/` completo por decision del propietario del proyecto. `backend/.gitignore` NO se creo: los patrones viven en la raiz, que es un unico origen de verdad y cubre cualquier profundidad.

### Evidencia del arranque limpio

Arbol NUEVO (copia fuera del repositorio, sin `.env` y sin `__pycache__`) y volumen NUEVO, siguiendo el camino exacto que documenta el README y sin ninguna correccion manual ni montaje alternativo:

1. `python scripts/init_env.py` dentro de la copia, nunca en el arbol real, porque escribe con `O_CREAT|O_EXCL` en una ruta relativa a su propia ubicacion. Genero un `.env` nuevo con sus ocho variables.
2. `docker compose -p firewatch-verify up -d --build` desde la copia: los dos migradores en `Exited (0)`, `schema_migrations=1,2` y `realm=firewatch` en la base de identidad.
3. `python scripts/smoke.py` desde la copia: `SMOKE_EXIT=0` con las dos lineas PASS, y lectura de vuelta en PostGIS con `model_version=academic-rule-v1`, `demo=t`, `risk_level=alto`, cinco factores y justificacion completa.

La importacion del realm en volumen nuevo, que es la prueba decisiva:

```
Realm 'firewatch' imported
KC-SERVICES0032: Import finished successfully
```

La copia quedo ademas byte a byte identica al arbol entregado: mismos archivos, mismos hashes, cero diferencias salvo el `.env` generado. Eso importa porque significa que la prueba corrio los artefactos entregados y no una variante.

### Respuesta explicita

**Si: el camino documentado por el README funciona de punta a punta desde un arbol y un volumen vacios, sin correccion manual y sin montaje alternativo.** Las dos unicas desviaciones respecto de los comandos del README fueron el nombre de proyecto (`-p firewatch-verify`, obligatorio para no tocar el stack del usuario) y el directorio de trabajo (la copia, obligatorio porque `init_env.py` no debe escribir en el arbol real). Ninguna de las dos cambia la configuracion, las imagenes ni el camino ejercitado. La verificacion independiente lo reprodujo con su propio proyecto, su propio arbol temporal y su propio volumen, y obtuvo el mismo resultado.

### Defecto preexistente que esto cierra

`deploy/local/firewatch-realm.json:16` declaraba `postLogoutRedirectUris`, campo que Keycloak 26.5.2 rechaza en `ClientRepresentation`. En un volumen vacio la importacion fallaba, `identity` entraba en bucle de reinicio y no se podia emitir ningun token. Una instancia nueva de cloud nace con volumen vacio, asi que el defecto habria bloqueado el despliegue entero. Cerrado y verificado con una importacion real desde cero.

### Ajuste posterior a la verificacion

El verificador noto que el comentario del bloque afirmaba que todo lo excluido era artefacto generado, y que `portable/` contiene tres archivos de texto pequenos (`portable/code-explanation/SKILL.md`, `portable/doc2md/SKILL.md`, `portable/code-explanation/assets/report-template.md`). La decision del propietario se mantiene -- `portable/` se ignora completo -- y se corrigio la afirmacion: ahora son dos grupos con dos comentarios, uno para lo generado y otro para lo excluido por decision, declarando que no es una exclusion por contenido. Es una edicion posterior al informe del verificador y la valide solo yo, con `git check-ignore -v`; se declara como tal.

### Limpieza e impacto

`down -v` del proyecto propio, los cinco tags de imagen `firewatch-verify-*` eliminados, la copia temporal y el arbol temporal del verificador borrados. Cero residuos en contenedores, volumenes, redes e imagenes. Despues: los once contenedores del usuario con el mismo estado que antes de empezar, sus cuatro volumenes intactos, y `backend/.env` con el mismo hash `a2766fd13976ffa74587f928c2899edea56e9570ea91b1b36f47ab4c6c622d02`. Nunca se ejecuto `down` sobre el proyecto `firewatch`.

## Unidad B.1 — la UI servida deja de afirmar cosas falsas

Alcance: la cara publica del entregable. La UI servida por el gateway afirma que la API entrega `not_evaluated` y que no hay inferencia; ambas cosas son falsas desde la Rebanada B y ademas subestiman el sistema, porque niegan justo la capacidad de clasificacion que las Bases piden. Autorizada por el orquestador. Sin commit y sin revision nativa: `nogit` sigue vigente.

### Superficies

- `backend/services/gateway/internal/handler/web/index.html` (meta, banda demo, banner de limite, pildora inicial).
- `backend/services/gateway/internal/handler/web/proposal.js` (etiquetado humano del nivel y literal del estado del lote).
- `backend/services/gateway/internal/handler/gateway_test.go` (solo aserciones de texto que el cambio rompe, mas una guarda negativa).
- `frontend/README.md` y `odd/tasks/frontend-react-vite-migration.md` (documentacion, no codigo).

### Tareas

1. [ ] Reescribir `index.html` para declarar entrada de demostracion y clasificacion SI calculada por una regla academica propia, declarada no oficial; nada de datos reales ni de ingestión inexistente.
2. [ ] Traducir el nivel a etiquetas humanas en mayusculas, con `INFORMACIÓN INSUFICIENTE` para el cuarto nivel, y corregir el literal fijo de `proposal.js`.
3. [ ] Actualizar las aserciones de texto de `gateway_test.go` y anadir una guarda negativa contra `not_evaluated` en la pagina servida.
4. [ ] Corregir en los dos documentos la afirmacion de que el contrato vivo es `demo-fixture-v1` con `not_evaluated`.

### Evidencia de la Unidad B.1

Ejecutada y observada el 2026-10-11, delegada a un escritor (`gentle-ai-worker`). Sin commit y sin revision nativa: `nogit` sigue vigente.

#### Cambios

- `index.html:8,13,33,39`: la meta description, la banda de demostracion, el banner de limite y la pildora inicial dejan de afirmar `not_evaluated` / "riesgo no evaluado". Ahora declaran entrada sintetica, clasificacion SI calculada por una regla academica propia declarada no oficial, ausencia de ingestión real y no sustitucion de CONAF/SENAPRED. La pildora inicial pasa de "NO EVALUADO" a "SIN CONSULTAR", que es lo que de verdad significa antes de consultar.
- `proposal.js:157-167,174-189`: nueva funcion `levelLabel` que traduce los cuatro niveles canonicos a etiquetas humanas en mayusculas, con `INFORMACIÓN INSUFICIENTE` para `desconocido` y `SIN CLASIFICAR` como respaldo; el nivel y la pildora se pintan con ella, y el literal fijo que afirmaba `riesgo not_evaluated` se sustituye por el estado honesto del lote.
- `gateway_test.go:25-33,148`: las dos aserciones de texto que el cambio rompia se actualizaron a `"SIN CONSULTAR"` + `"Entrada sintética"`, se anadio una asercion de que la pagina declara la `regla propia no oficial`, y se conservo la guarda negativa contra `not_evaluated`.
- `frontend/README.md:20,26` y `odd/tasks/frontend-react-vite-migration.md:78,90`: dejan de afirmar que el contrato vivo es `demo-fixture-v1` con `not_evaluated`; ahora distinguen el contrato vivo (nivel canonico calculado por `academic-rule-v1`) del escenario interno `current`, que conserva el placeholder y no debe leerse como el contrato vivo.

#### Verificacion

- `gofmt -l services/gateway`: reporta `services/gateway/internal/handler/gateway.go`, hallazgo PREEXISTENTE y fuera de las superficies (imports desordenados: `encoding/json` antes de `embed`; `cat -A` confirma finales Unix, no es artefacto de CRLF). Se declara y no se toca.
- `go build ./services/gateway/...`: BUILD_OK. `go vet ./services/gateway/...`: VET_OK.
- `go test ./services/gateway/... -count=1`: PASS -- `ok firewatch.local/gateway/internal/handler 0.623s`.
- `grep -n "not_evaluated\|NO EVALUADO\|no evaluado"` sobre `index.html` y `proposal.js`: sin coincidencias.
- Verificacion lectora del padre: revisadas `index.html:8-39`, `proposal.js:136-194`, `gateway_test.go:15-95`; la copia y las aserciones coinciden con lo especificado.

#### Limites declarados

- No hay verificacion de navegador de la pagina renderizada (no hay arnes de navegador instalado y no se instalan dependencias por esto); el texto servido se verifico por la pagina que devuelve `httptest` y por lectura directa.
- La verificacion independiente por una cabeza distinta a la que escribio se hizo despues, junto con la de la Unidad A.1 y sobre el mismo arbol; su veredicto esta en la seccion "Verificacion independiente consolidada" mas abajo.
- Ningun archivo fuera de las superficies fue creado ni editado por el escritor; el unico hallazgo fuera de superficies es el `gofmt` preexistente de `gateway.go`, declarado arriba. El orquestador edito ademas ESTE documento (`odd/tasks/firewatch-data-contract.md`) para registrar la unidad, de forma deliberada y fuera de las superficies del escritor; el verificador lo detecto por fecha y tiene razon en que la afirmacion "ningun archivo fuera de esos cinco cambio con el trabajo de B.1" era imprecisa y queda corregida aqui.

## Unidad A.1 — `Params` como unico origen de verdad

Alcance: cerrar el hallazgo 1 de la revision independiente de la Rebanada B. El invariante NO es "el mapa se deriva del objeto" sino **"la conducta se lee SOLO del objeto"**. Autorizada por el orquestador. Sin commit y sin revision nativa: `nogit` sigue vigente.

### Particion declarada

La unidad se ejecuta en dos partes con evidencia separada, para que cada una sea verificable por si sola y no se convierta en un cambio multi-area:

- **A.1a — el candado.** Un solo objeto `Params` del que la regla lee su conducta, del que se derivan mecanicamente el mapa y el hash, unico camino de construccion, hash sobre la instancia efectivamente usada, y umbral de vencimiento inyectable con el valor por defecto congelado. Sin cambios de esquema.
- **A.1b — la referencia persistida.** `PolicyRef` con identificador, parametros efectivos y hash determinista, poblado por `Classify`, persistido junto al nivel en una migracion 003 y leido de vuelta.

### Tareas

A.1a:

1. [ ] `Params` con los campos de conducta exportados y etiquetados; `DefaultAcademicRuleV1Params()` congelado; derivacion mecanica de `Parameters()` con deteccion de campos sin etiqueta y de claves duplicadas; `Hash()` sobre esa misma derivacion.
2. [ ] La conducta (`score`, `levelFor`, `contributionOf`, umbral de vencimiento) son metodos de `Params`; `AcademicRuleV1` toma su `Params` por un unico camino de construccion; el `Rationale` de vencimiento usa el valor efectivo de la instancia.
3. [ ] Pruebas: derivacion que cubre cada campo exactamente (misma cardinalidad, sin duplicados), diferencial campo por campo (mutar exige cambio de hash y de conducta), golden del hash `e17aaa56...` conservado, vector de conducta congelado, ida y vuelta de la forma canonica, y guarda estructural de que no existe ninguna constante de conducta fuera del objeto.

A.1b:

4. [ ] `PolicyRef{PolicyID, Parameters, ParamsHash}` dentro de `Assessment`, poblado por `Classify` desde la instancia usada.
5. [ ] Migracion `003_policy_ref.sql`: columna `policy_ref jsonb` en `batches` (nunca se editan la 001 ni la 002); `Store.Publish` la escribe y `Store.List` la lee.
6. [ ] Pruebas de composicion e integracion: la referencia queda persistida y se lee de vuelta; su hash coincide con el de la instancia usada; las filas anteriores a la migracion quedan sin referencia (nunca inventada).

### Evidencia de la Unidad A.1

Ejecutada y observada el 2026-10-11. Implementada por escritores (`gentle-ai-worker`) y verificada por una cabeza distinta (`gentle-ai-verify`). Sin commit y sin revision nativa: `nogit` sigue vigente.

#### A.1a — el candado de parametros

Cambios:

- `params.go`: `Params` con 18 campos exportados y etiquetados `param:"..."`; `DefaultAcademicRuleV1Params()` como UNICO lugar del paquete donde aparecen numeros de conducta; `Parameters() (map[string]string, error)` derivado por reflexion en orden de declaracion (int64 en base 10, float64 con `'g',-1,64`), que falla ante un campo sin etiqueta, una etiqueta duplicada o un tipo no soportado; `Hash()` sobre esa misma derivacion; `validate()` para la costura de inyeccion.
- `academic_rule.go`: `AcademicRuleV1{params Params}`; `newAcademicRuleV1` como unico camino de construccion; `NewAcademicRuleV1()` con el default congelado y `NewAcademicRuleV1WithParams()` como costura de prueba; la conducta pasa a ser metodos de `Params` (`maxStaleness`, `score`, `levelFor`, `contributionOf`, `factorsOf`) y lee solo del receptor; se ELIMINARON todas las constantes de conducta de paquete; `AcademicRuleMaxStaleness` pasa a ser un valor DERIVADO del default y no una segunda fuente; el `Rationale` de vencimiento usa el umbral efectivo; `Policy` queda poblado en las seis salidas no-error de `Classify`.
- `classifier.go`: `PolicyRef{PolicyID, Parameters, ParamsHash}` con etiquetas json; `Assessment.Policy`; `Validate()` rechaza ademas una evaluacion sin `PolicyID` o sin `ParamsHash`.
- Pruebas: la derivacion cubre EXACTAMENTE cada campo (cardinalidad y rechazo de duplicados); diferencial sobre los 18 campos que exige cambio de hash Y de conducta; golden del mapa y del hash; vector de conducta congelado con la aritmetica escrita a mano; hash de la instancia efectivamente usada; ida y vuelta de la forma canonica; y guarda estructural que falla ante un numero de conducta fuera de `Params`, con controles positivos que demuestran que detecta.

Evidencia:

- `gofmt -l services/risk`: sin salida. `go build` y `go vet` sobre `./services/risk/...`: correctos. `go test ./services/risk/... -count=1`: verde.
- El hash derivado sigue siendo `e17aaa5600eb67c9a91340e90057aec100f4e80ef0895662f373cffee66564c7`, igual al congelado y al que ya quedo registrado en la migracion 002. No hubo que tocar la 002: la derivacion reproduce los 18 pares canonicos.
- Vector de conducta reproducido a mano fuera del codigo: all-mid 0.5, near-low 0.02925, near-high 0.994375, edge 0. Aritmetica verificada por el orquestador ademas del propio test.
- Verificacion independiente (cabeza distinta): CONFIRMADAS la reproduccion del hash en Python fuera de Go, la reaccion de la derivacion al saltarse un campo (tres pruebas fallan), la reaccion a una constante de conducta anadida fuera del objeto (falla la guarda estructural Y el vector de conducta), la presencia de `Policy` en las seis salidas, que el hash describe la instancia inyectada, y que no queda ninguna constante de conducta de paquete.

Limites declarados de A.1a:

- La guarda estructural permite los literales enteros `0` y `1` en los archivos de conducta porque son estructurales (`linear` devuelve 0 y 1). Un umbral de conducta escrito literalmente como `0` o `1` fuera de `Params` no seria marcado por la guarda. Es una concesion acotada y declarada.
- El vector de conducta congela cinco sondas (all-mid, near-low, near-high, edge y la vencida). Una conducta nueva que solo se manifieste fuera de esos rangos seria detectada por la guarda estructural solo si introduce un literal numerico.

#### A.1b — la referencia de politica persistida

Cambios: `003_policy_ref.sql` (columna `policy_ref jsonb` mas un CHECK de "referencia usable", e idempotente); `embed.go` con la version 3; `store.go` con `Policy *classify.PolicyRef` (puntero: nil significa "no se registro referencia" y se guarda NULL, nunca inventada), `coalesce(policy_ref,'null'::jsonb)` al leer y error explicito si una referencia almacenada no se puede decodificar; `publisher.go` copia la referencia desde la evaluacion; y pruebas de unidad, de integracion y de migracion.

Evidencia en base de datos real:

- Contenedor PostGIS desechable propio (`firewatch-verify-pg3`, `127.0.0.1:55434`, mismo `postgres-init.sh` del proyecto): la migracion fresca aplica 1, 2 y 3; la segunda corrida es no-op; `batches.policy_ref` es `jsonb`; `batches_policy_ref_check` acepta NULL y una referencia completa y RECHAZA `{"policy_id":"x"}`; partiendo de una base ya en la version 2 el runner aplica SOLO la 3.
- Con `TEST_DATABASE_URL` apuntando a esa base migrada, `TestPublisherPersistsAndReadsBackTheFixtureAssessment` y `TestBatchDeduplicationAndOutOfOrderPublication` CORREN (no se saltan) y pasan; la fila leida de vuelta trae `policy_id = academic-rule-v1` y `params_hash = e17aaa56...6467` con los 18 parametros; un lote publicado sin referencia vuelve con `policy` nulo, nunca inventado.
- Verificacion independiente, con SU PROPIO contenedor y su propia base: todos los puntos anteriores CONFIRMADOS, mas la respuesta HTTP con el objeto `policy` (sonda con `go test -overlay`, sin escribir en el arbol) y la comprobacion de que `smoke.py` no prohibe campos adicionales.

Limites declarados de A.1b:

- NO se ejecuto `smoke.py` end-to-end en esta unidad: exige el stack completo (Keycloak, RabbitMQ, gateway, jobs, risk) y se habia ejecutado en la Unidad 5 para el estado previo. La revision es estatica: ninguna asercion de `smoke.py` prohibe campos adicionales, y el campo nuevo es aditivo.
- `backend/contracts/openapi/firewatch-v1.yaml` NO se actualizo: sigue sin el campo `policy` y con los enums `demo-fixture-v1` / `not_evaluated` en `:146` y `:160`. Es rebanada C.
- `001_initial.sql` y `002_models.sql` NO se editaron, pero "no editado" es inferencia, no prueba: el repositorio sigue sin commits que sirvan de linea base, asi que la evidencia es la fecha de modificacion y las aserciones de contenido congelado.
- `docker rm -f` dejo dos volumenes anonimos del contenedor desechable; se eliminaron despues. Ningun volumen NOMBRADO del usuario fue tocado y nunca se ejecuto `docker compose down` sobre el proyecto `firewatch`.

## Rebanada C — contrato, validacion de transporte y documentacion

Alcance: cerrar el contrato OpenAPI desactualizado, dejar de tratar un evento bien formado como malformado, y corregir la documentacion que todavia describe el placeholder. Ampliada por el orquestador para incluir el consumidor de `jobs`, porque dejar la misma puerta ahi habria producido un arreglo a medias: risk dejaria de inertizar el flag, publicaria `demo = false` con un proveedor real, y jobs mandaria ese lote a la DLQ. Autorizada por el orquestador. Sin commit y sin revision nativa: `nogit` sigue vigente.

### Superficies

- `backend/contracts/openapi/firewatch-v1.yaml`
- `backend/services/risk/internal/service/` (`demo.go` y `demo_test.go`)
- `backend/services/jobs/internal/messaging/` y las pruebas de `jobs`
- `backend/docs/code-explanation/README.md`, `03-risk.md`, `05-contracts.md`, `08-tests-tools.md`
- Este documento (`odd/tasks/`)

### Regla de validacion de zona (definida en esta rebanada e informada antes del cambio)

El literal anterior mezclaba tres cosas distintas en una sola condicion booleana. La regla nueva las separa:

1. **Forma del evento** (violacion permanente, falla cerrado y va a la DLQ): `type == jobs.requested.v1`; `id`, `job_id` y `trace_id` parsean como UUID no nulo; `actor_id` no vacio y de a lo sumo 255 bytes; `requested_at` y `occurred_at` no nulos; `requested_at` no mas de un minuto en el futuro.
2. **Zona soportada** (allowlist declarada, falla cerrado): `zone_id` debe pertenecer a `SupportedZones`, hoy exactamente `{"valparaiso-demo"}`. Una zona desconocida o mal escrita se rechaza y va a la DLQ; el servicio NUNCA crea una zona nueva en silencio. Se descarto explicitamente la regla "cualquier zona no vacia" porque un error de tipeo inventaria una zona. ACOPLAMIENTO DECLARADO: `batches.zone_id` (risk) y `jobs.zone_id` siguen con `CHECK (zone_id = 'valparaiso-demo')` en las migraciones 001 congeladas, asi que ampliar la allowlist exige ademas una migracion nueva.
3. **`demo` deja de ser puerta de validez**: describe el origen de la solicitud, no la forma del evento; `batches.demo` ya se deriva de la procedencia del proveedor (rebanada B). Un evento bien formado con `demo = false` se ACEPTA en lugar de morir en la DLQ.

### Relacion lote-zona en `jobs`: NO se duplica como literal

Hallazgo que cambio el diseno: la relacion que el peer propuso como alternativa **ya existia** en el camino de `jobs`. `Store.Complete` actualiza el job con
`UPDATE jobs SET state='published',batch_id=$2 WHERE id=$1 AND actor_id=$3 AND zone_id=$4 AND state='queued'`,
y si afecta cero filas comprueba si el job ya estaba publicado con la MISMA zona; si no, devuelve "result does not match a known job". Es integridad contra ESTADO ALMACENADO, en la misma transaccion que el cambio de estado, sin constantes y sin migracion.

Por eso el cambio en `jobs` es solo el de `valid()`: se quitan `!e.Demo` y el literal de zona, y la relacion lote-job se deja donde ya se verifica de verdad. El literal habria obligado a una migracion para ampliar zonas justo en el lugar donde la base ya lo resuelve.

### Tareas

1. [ ] Relajar `service/demo.go` segun la regla de arriba (allowlist de zona, `demo` fuera de la puerta), y actualizar `demo_test.go`.
2. [ ] Corregir el OpenAPI: enums de `model_version`, `demo` y `risk_level`, y declarar `factors`, `rationale` y `policy` (que hoy la API devuelve y el contrato omite).
3. [ ] Corregir en `docs/code-explanation/` las afirmaciones que siguen diciendo que no hay clasificador o que el nivel es `not_evaluated`, y declarar que la instantanea de hashes quedo obsoleta desde la rebanada B.
4. [ ] Quitar de `valid()` en `jobs` las puertas de origen (`!e.Demo` y el literal de zona), conservar la forma, y anadir pruebas unitarias de `valid()` y una prueba de integracion de la relacion lote-job.

### Evidencia de la Rebanada C

Ejecutada y observada el 2026-10-11. Implementada por escritores (`gentle-ai-worker`). Sin commit y sin revision nativa: `nogit` sigue vigente.

#### Bloque 1 — OpenAPI, `service/` y documentacion

Cambios: `service/demo.go` (24→54 lineas) con `SupportedZones`/`SupportedZone` y `ValidateRequest` separando forma, allowlist y origen, sin `!event.Demo`; `demo_test.go` (37→56) con el caso `demo=false` como VALIDO, la negacion por typo y el pin de la allowlist; `contracts/openapi/firewatch-v1.yaml` (161→203) con `factors`, `rationale` y `policy` declarados y en `required`, `risk_level` canonico y sin los enums de `model_version`/`demo`; y correcciones de prosa en `03-risk.md`, `05-contracts.md`, `08-tests-tools.md` y `README.md`.

Evidencia:

- `gofmt -l services/risk` sin salida; `go build` y `go vet ./services/risk/...` exit 0; `go test ./services/risk/... -count=1` verde en los seis paquetes con pruebas.
- `go run github.com/getkin/kin-openapi/cmd/validate@v0.133.0 contracts/openapi/firewatch-v1.yaml`: exit 0, sin salida (es el comando exacto del CI).
- `python docs/code-explanation/verify_structure.py`: exit 1 con 32 errores (23 "Uninventoried file", 8 "Whole-file range mismatch", 1 "Source snapshot differs"). Comparado contra una linea base capturada ANTES de editar: el conjunto es identico y NINGUNO es nuevo. La instantanea de hashes NO se re-baselinea en esta rebanada; queda declarado que el verificador estructural seguira fallando hasta una re-baseline autorizada, y que ya fallaba desde la rebanada A.

Limites declarados del bloque 1: `info.description` del OpenAPI sigue diciendo "No real risk inference is performed" (defendible como "sin inferencia oficial", pero declarado); `03-risk.md` conserva citas obsoletas de rebanadas anteriores que este cambio no falsifico (`PublishDemo`, `store.go:29-89`); y `source-snapshot.json` queda intencionalmente viejo.

#### Bloque 2 — consumidor de `jobs`

Cambios: `messaging/broker.go` (236→274 lineas): `valid()` pierde `!e.Demo` y el literal de zona y conserva la forma; se agregan `permanentError` (interfaz estructural `Permanent() bool`, sin acoplar paquetes), `isPermanent` con `errors.As`, `retryBackoff` y `handleWithRetries`; `session` registra `attempts` y `permanent` y devuelve `ctx.Err()` si el contexto termino durante una pausa. `repository/store.go` (122→131): `errNoMatchingJob{jobID}` declara `Permanent() bool { return true }`. Nuevo `messaging/broker_test.go` (134 lineas, 6 pruebas) y `store_integration_test.go` (57→115) con `TestCompleteEnforcesStoredZoneRelationship`.

Evidencia:

- `gofmt -l services/jobs` sin salida; `go build` y `go vet ./services/jobs/...` exit 0; `go test ./services/jobs/... -count=1` verde.
- Contenedor PostGIS desechable propio (`firewatch-verify-pg4`, `127.0.0.1:55435`, mismo `postgres-init.sh`): con `TEST_DATABASE_URL` apuntando a `jobs_test` migrado, TODAS las pruebas corrieron (cero `--- SKIP`), incluidas `TestValidSeparatesShapeFromOrigin` (11 subcasos), las cuatro de la politica de reintentos y `TestCompleteEnforcesStoredZoneRelationship`.
- Las dos pruebas que exigio el orquestador, ejecutadas: un error PERMANENTE produce exactamente 1 intento y 0 pausas; un error NO tipificado produce exactamente 5 intentos con el backoff `[1s, 2s, 4s, 8s]`. La segunda es la que protege de descartar trabajo bueno por un fallo pasajero.
- Limpieza: contenedor y volumen anonimo eliminados; 18 contenedores y 12 volumenes identicos al baseline del usuario.

Cambio de conducta declarado: el bucle anterior pausaba 1, 2, 4, 8 **y 16** segundos (31 s en total, con una pausa INUTIL despues del ultimo intento). El nuevo pausa 1, 2, 4, 8 (15 s) y manda a la DLQ inmediatamente despues del quinto intento. Es el unico cambio de calendario y es una mejora: la pausa final no tenia nada despues. El hallazgo de "31 s por evento" de la revision de la rebanada B sigue siendo cierto para el broker de `risk`, que no se toco.

Limites declarados del bloque 2: el broker de `risk` NO recibio la distincion permanente/transitorio porque sus violaciones de forma y de allowlist ya se resuelven en la puerta previa al bucle y su `Handle` es E/S transitoria por diseno; queda declarado, no silenciado. Y las citas de `docs/code-explanation` para `jobs/messaging/broker.go`, `jobs/repository/store.go` y su prueba de integracion quedaron obsoletas con este bloque, porque `docs/code-explanation/` no estaba en las superficies del bloque 2; se declaran como deuda, no se disimulan.

#### Verificacion independiente de la Rebanada C

Una cabeza distinta, solo lectura, con su propio contenedor desechable: **14 de 14 afirmaciones CONFIRMADAS y ninguna refutada**. Lo mas util fue lo que encontro de mas: cinco afirmaciones obsoletas que nuestros PROPIOS cambios habian falsificado y que la rebanada no habia corregido.

- `04-messaging.md:59` seguia diciendo que `valid` exige "demo true, zona fija", falso desde el bloque 2.
- `05-contracts.md:65` atribuia el match actor/zona a `broker.go:valid`, cuando vive en `repository/store.go` (`Complete`), y su rango de lineas estaba corrido.
- `05-contracts.md` cerraba diciendo "no se modificaron contratos", en contradiccion con esta misma rebanada.
- `03-risk.md` y `08-tests-tools.md` citaban `PublishDemo`, que no existe: el metodo es `Store.Publish`.
- `contracts/openapi/firewatch-v1.yaml:4` decia "No real risk inference is performed", que se lee como "no hay clasificador".

Los cinco se corrigieron en un escritor acotado, mas `07-deployment.md:13`, que citaba `PublishDemo`. No se agregaron enlaces, encabezados ni citas nuevas y kin-openapi sigue validando (exit 0).

Confirmado tambien, y declarado como limite: el broker de `risk` NO tiene clasificacion permanente/transitorio. Hoy no hay ningun error obviamente permanente alcanzable desde su `Handle` (el proveedor fixture y la regla son deterministas), pero un proveedor real con zonas mezcladas, un `RetrievedAt` cero o una evaluacion que no pase `Validate()` serian deterministas y aun asi se reintentarian cinco veces con el calendario viejo de 1+2+4+8+16 s. No hay perdida de datos, pero queda para la Unidad A.2, declarado y no arreglado.

Limite estructural ya conocido, actualizado: `verify_structure.py` pasa de 32 a 36 errores, y los 4 nuevos los introdujo NUESTRA propia rebanada de `jobs` (`broker_test.go` no esta inventariado y tres archivos de `jobs` cambiaron de tamano). La instantanea sigue sin re-baselinearse por decision declarada: es deuda de documentacion, no un hallazgo oculto. Nota de alcance: el literal de zona sigue existiendo en `services/jobs/internal/handler/api.go:41`, que es la validacion HTTP del cuerpo del POST, no el camino del consumidor; el consumidor quedo sin literales.

### Verificacion independiente consolidada

Una cabeza distinta a las que escribieron ejecuto la verificacion el 2026-10-11, solo lectura y con contenedores propios. Resultado: 14 de 15 afirmaciones CONFIRMADAS y dos precisiones que se corrigen aqui en lugar de disimularse.

- **B4, refutada como estaba formulada**: el verificador encontro por fecha un SEXTO archivo modificado en la ventana de B.1, `odd/tasks/firewatch-data-contract.md`. Es la edicion de este documento por el orquestador, deliberada y fuera de las superficies del escritor. La afirmacion "ningun archivo fuera de esos cinco cambio" era imprecisa y queda corregida arriba.
- **A7, incierta en el sentido estricto de git**: el contenido de `001_initial.sql` y `002_models.sql` es el esperado, pero la propiedad "no modificado" no se puede PROBAR sin un commit de linea base. Es inferencia por fecha y por aserciones de contenido.
- **Concesion declarada de la guarda**: los enteros `0` y `1` estan exentos en los archivos de conducta; el verificador lo confirmo y lo dejo escrito.

No verificado por el verificador, y declarado: la representacion en navegador de la pagina servida (no hay arnes de navegador), la validacion del OpenAPI contra la API viva, y `smoke.py` end-to-end. El verificador tampoco leyo `frontend/src/**`, `backend/docs/code-explanation/**`, `.github/workflows/**` ni el cuerpo completo de `store.go`/`publisher.go`; lo declaro.

## Unidad A.3 — cerrar las vias abiertas del candado

Origen: un ataque adversarial del revisor, en solo lectura, encontro CUATRO vias rotas en la A.1. TRES de esas cuatro mutaciones dejaban la suite existente en VERDE: el defecto de fondo no es solo el codigo, es que las pruebas daban confianza falsa.

### Vias halladas

1. **Fuga por constante suelta ESCONDIDA DENTRO de `DefaultAcademicRuleV1Params`.** La guarda eximia esa funcion entera, reportaba `violations = 0` sobre la fuente mutada, y la suite pasaba con la conducta cambiada.
2. **La exencion de `0` y `1`.** `if value == 1 { return 1 }` pasa la guarda, pasa la suite y cambia el nivel de medio a alto. Los controles positivos existentes (0.5 y `4 > 3`) no cubren la frontera de su propia exencion.
3. **Cambio sin literal.** `case score < p.BandBajoMax` convertido en `<=`. Sin literal, la guarda reporta 0 violaciones, la suite queda verde y el nivel en 0.34 cambia de medio a bajo. NINGUNA guarda estatica ve esto.
4. **Segundo constructor.** `classify.AcademicRuleV1{}` compila desde un paquete externo, produce nivel alto para datos benignos, PASA `Validate()` y se registra bajo el ID congelado `academic-rule-v1`.

Confirmado intacto por ese ataque, y que NO debe tocarse: la derivacion por reflexion (incluye campos no exportados, no permite sobrescritura silenciosa de claves y rechaza tag duplicado); el orden es irrelevante; la representacion canonica es bit-exacta salvo NaN, que `validate()` ya bloquea; nada se autorregenera; y el diferencial de los 18 campos cambia hash y conducta en todos.

### Principio rector

El hash congela los CAMPOS de `Params`, NO la conducta. Solo es suficiente si la conducta es inalcanzable fuera del objeto, y hoy no lo es. Por eso hacen falta DOS candados, y la guarda estructural baja de rango: reduce el espacio de ataque, no es el candado.

### Tareas

1. [ ] A.3.1 Exenciones de la guarda: `0` y `1` permitidos SOLO en posiciones sintacticas estructurales (indice, limite de slice, retorno de las normalizaciones, incremento) y NUNCA en comparaciones ni en aritmetica; `DefaultAcademicRuleV1Params` deja de eximirse como cuerpo entero y pasa a exigir exactamente un `return` con un literal compuesto y ninguna otra sentencia.
2. [ ] A.3.2 Impersonacion: campo de construccion NO EXPORTADO en el struct de la REGLA (para que el valor cero falle cerrado), verificacion del hash esperado para el ID congelado en el registro, y verificacion del hash de la referencia contra el esperado en la publicacion.
3. [ ] A.3.3 El segundo candado: tabla dorada de CONDUCTA con hash congelado, ademas de las pruebas hostiles que demuestren que las tres mutaciones de arriba ahora fallan.
4. [ ] A.3.4 Reescribir los comentarios que afirman una garantia que el mecanismo no tiene, para que digan lo que SI garantiza (parametros congelados y conducta atada a una tabla dorada) y lo que NO (que la conducta sea inalcanzable fuera del objeto).

### Condiciones no negociables

- El campo de construccion no exportado va en el struct de la REGLA (`AcademicRuleV1`), NUNCA en `Params`. Motivo duro: `deriveParameters` SI incluye campos no exportados, asi que un campo nuevo en `Params` cambiaria el mapa derivado, cambiaria el hash y romperia la migracion 002 ya aplicada en bases existentes.
- **INVARIANTE**: al terminar la A.3, `DefaultAcademicRuleV1Params().Hash()` debe seguir siendo exactamente `e17aaa5600eb67c9a91340e90057aec100f4e80ef0895662f373cffee66564c7`. Queda como asercion explicita; si cambia, se DESHACE el cambio, no se re-congela el valor nuevo.

### Criterio de densidad de la tabla dorada (decidido e informado antes de escribir)

Dos tablas, no una, con unas 170 expectativas congeladas en total:

- **(a) Nivel por puntaje**, con cada frontera de banda EXACTA mas su vecino a un ulp (`math.Nextafter`) por lado, el grid de 0.01 sobre [0,1], los puntos medios de banda, y ademas `0`, `1`, valores FUERA de [0,1] y el caso `NaN`/`Inf` fijado explicitamente en vez de dejarlo indefinido. Es la tabla que caza un `<` convertido en `<=`, porque ese ataque solo se manifiesta exactamente sobre la frontera.
- **(b) Conducta por entradas**, con barrido por variable desde una base media (9 valores: bajo el umbral, umbral exacto, umbral+-1 ulp, 1/4, 1/2, 3/4, alto-1 ulp, alto exacto, alto+1 ulp) para las 5 variables, mas all-low/all-mid/all-high/all-edge, mas la puerta de vencimiento exacta y a +-1 s, mas combinaciones mixtas. Se congela el nivel y los 5 aportes con `%.17g`.

La tabla se serializa de forma canonica y su SHA-256 se congela igual que el de parametros; el literal se conserva para que el diff sea legible. Criterio de cobertura: **toda frontera de decision mas su vecindad inmediata**, no un muestreo suelto.

### Evidencia de la Unidad A.3

Ejecutada y observada el 2026-10-11. Implementada por dos escritores y verificada por una cabeza distinta con mutaciones hostiles en copias FUERA del arbol. Sin commit y sin revision nativa: `nogit` sigue vigente.

#### A.3.1 — guarda estructural

Reglas vigentes: `DefaultAcademicRuleV1Params` debe ser EXACTAMENTE un `return` de un literal compuesto y ninguna otra sentencia; el literal `1` queda prohibido como operando de comparacion y en aritmetica salvo la excepcion estrecha y comentada de `falling`; los literales flotantes quedan prohibidos fuera del compuesto de defaults; las comparaciones con `0` siguen permitidas. Se agregaron siete controles positivos nuevos con fuentes sinteticas.

Desviacion declarada y aceptada: `0` NO se prohibe como operando de comparacion, porque el codigo congelado tiene `len(...) > 0`, `age < 0` y `p.MaxStalenessSeconds < 0`, que son longitud y coherencia, no umbrales de regla. El ataque demostrado era el literal `1`.

#### A.3.2 — impersonacion

- El campo de construccion no exportado vive en el struct de la REGLA, nunca en `Params` (`deriveParameters` recorre campos no exportados, asi que un campo nuevo ahi cambiaria el hash). El valor cero falla CERRADO en `Classify`.
- `FrozenPolicyFingerprints` + `PolicyFingerprinter`: el registro rechaza un ID congelado que no sepa reportar su huella o que reporte otra.
- `Publisher.Publish` verifica la huella de la politica contra la expectativa congelada antes de persistir.
- Pruebas: el valor cero se rechaza en registro y en clasificacion; un stub con ID NUEVO se sigue aceptando; una huella falsa bajo el ID congelado la rechazan registro Y publicacion.

#### A.3.3 — el segundo candado (tabla dorada)

`behavior_table_test.go` (539 lineas): mesa (a) con 118 puntajes a nivel, incluidos cada frontera de banda EXACTA y su vecino INTERNO a un ulp, `0`, `1`, valores fuera de `[0,1]`, el grid de 0.01 y `NaN`/`+Inf`/`-Inf` fijados; mesa (b) con 57 sondas a nivel mas cinco aportes (**342 valores**). Literal congelado de 179 lineas y su sha256 congelado (`5403ebfd...fc27`). Sin ruta de regeneracion: no hay flag `-update`, ni `os.WriteFile`/`os.Create`, ni `go:generate`.

#### A.3.4 — comentarios

`params.go` se edito SOLO en comentarios (cuatro bloques) para decir que el hash congela la forma de los parametros, no la conducta, y que el candado de la conducta es la tabla dorada. Se corrigio ademas el comentario de `TestLinearAndFallingLowEdgeIsInclusive`, que exageraba lo que esa prueba demuestra.

#### Verificacion hostil independiente

- **M1**, constante escondida DENTRO de `DefaultAcademicRuleV1Params`: la guarda FALLA. Antes del arreglo esta misma mutacion pasaba en verde.
- **M2**, `if value == 1 { return 1 }`: la guarda FALLA en la forma directa; en la forma que la guarda PERMITE (mover el `1` a un helper de retorno) la caza la TABLA DORADA. Es la demostracion de que hacen falta dos candados.
- **M3**, `<` convertido en `<=` en `levelFor`: FALLAN la prueba de fronteras y la tabla dorada. Es la mutacion sin literal que motivo toda la unidad.
- **M4**, `AcademicRuleV1{}` desde otro paquete: `Classify` falla cerrado, el registro lo rechaza, y una huella falsa bajo el ID congelado la rechazan registro y publicacion; un stub con ID nuevo se acepta.
- **Invariante**: `DefaultAcademicRuleV1Params().Hash()` sigue siendo `e17aaa5600eb67c9a91340e90057aec100f4e80ef0895662f373cffee66564c7`, con la tabla y el hash de parametros en verde.

#### DOS AFIRMACIONES MIAS QUE EL VERIFICADOR REFUTO

1. Dije que la mutacion simetrica dentro de `linear` (`<= low` a `<`) la cazarian las otras cuatro variables y la prueba nombrada. **FALSO**: esa mutacion es PRESERVADORA DE CONDUCTA para umbrales distintos, porque el camino alternativo calcula exactamente `+0.0` en el borde bajo y exactamente `1.0` en el alto. No habia hueco; habia una afirmacion mia equivocada y un comentario de prueba que la repetia. El comentario quedo corregido: esa prueba es un pin de VALOR, no un guardia de mutacion.
2. El criterio "umbral +-1 ulp" se cumple solo del lado INTERNO. El lado externo se sondea a distancia 1 y no a un ulp, porque en la zona de recorte ambos lados de la comparacion devuelven el mismo valor y el ulp externo no agrega poder de deteccion. La formulacion honesta del criterio es: **el ulp INTERNO de cada umbral, exacto y a un ulp**.

#### Limites declarados de A.3

- Que el cambio de A.3.4 fuese solo de comentarios NO se puede PROBAR sin linea base git; la evidencia es el hash de parametros intacto, la tabla congelada y el verificador estructural.
- Las tres entradas no finitas de la mesa (a) son inalcanzables desde `Classify` (refuta lo no finito antes de puntuar), pero SI son aserciones sobre `levelFor` como funcion total.
- La instantanea estructural se re-baselineo al cerrar A.3, y se re-sello de nuevo tras la correccion del comentario, quedando en `exit 0` con `errors: []`.

### Evidencia de la Unidad A.3 — cierre

## Unidad pendiente — MAPA GEOESPACIAL REAL (NO empezada)

Es el siguiente requisito de las Bases sin cubrir: hoy el mapa es un SVG con cuatro poligonos a mano y PostGIS guarda un solo punto. No se empieza sin autorizacion explicita.

### Checklist de zona — CINCO lugares acotan la zona, no uno

Al introducir la tabla real de comunas o sectores hay que tocar o verificar los cinco. Dejar uno atras crea una inconsistencia silenciosa entre lo que la API acepta y lo que la base admite:

1. `services/jobs/internal/handler/api.go:41` — validacion HTTP del cuerpo del `POST /v1/jobs`.
2. La allowlist `SupportedZones` en `services/risk/internal/service/demo.go` — puerta del consumidor de `risk`.
3. `CHECK (zone_id = 'valparaiso-demo')` en `services/risk/migrations/001_initial.sql:5` (tabla `batches`).
4. `CHECK (zone_id = 'valparaiso-demo')` en `services/jobs/migrations/001_initial.sql:5` (tabla `jobs`).
5. El enum de zona del OpenAPI: `contracts/openapi/firewatch-v1.yaml` (parametro `zone_id` del POST y propiedades `zone_id` de `Job` y `Batch`).

Los puntos 3 y 4 NO se editan directamente: las migraciones 001 estan congeladas, asi que ampliar zonas exige una migracion NUEVA que quite o relaje esos CHECK, y el punto 5 se actualiza en el mismo cambio. Los puntos 1 y 2 se amplian en el mismo cambio.

## Decision sobre la deuda de documentacion estructural

`backend/docs/code-explanation/source-snapshot.json` guarda sha256, tamano y lineas, y el `README.md` de esa carpeta fija rangos `1-N` por archivo. Por tanto CUALQUIER edicion invalida sus entradas: es un piso de ruido permanente, y un piso de ruido esconde regresiones reales, porque una lista que siempre tiene 36 errores deja de mirarse.

**Decision del orquestador**: re-baselinear como un acto deliberado y unico al cerrar un hito de codigo, con el diff revisado, y dejar escrito por que. No se hace a mitad de una unidad en vuelo, porque la invalidaria otra vez al siguiente cambio.

**Causa raiz anotada, NO arreglada ahora** (candidato, no vale la pena desviarse en este hito): si la herramienta quiere detectar deriva ESTRUCTURAL, no deberia anclarse a numeros de linea, que cambian con cada edicion, sino a identidad de archivo y a simbolos.

**Instruccion adicional del orquestador para el re-baseline**: las cifras en PROSA del `README.md` (conteos de explicados, contextuales, excluidos, hashes) son exactamente el dato que se vuelve falso solo, y ya se volvio falso una vez. Al re-baselinear NO se reescriben a mano: o se eliminan del texto y se deja que la herramienta las imprima al correr, o el README remite a esa salida en lugar de afirmar numeros. Una cifra escrita a mano en documentacion es una afirmacion que nadie verifica y que caduca en silencio; si no puede generarse, es mejor no afirmarla.
