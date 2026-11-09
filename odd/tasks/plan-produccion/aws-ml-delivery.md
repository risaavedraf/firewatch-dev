# FireWatch — entrega de producción con modelo sobre AWS

Feature: ejecutar el plan de implementación del modelo con inferencia en Go y etiquetas reales de CONAF, en dos frentes paralelos (modelo y despliegue). Depende de dos accesos externos; todo lo demás ya es ejecutable.
Estado: en curso · abierto 2026-11-06 · documento fuente: [`Referencias/FireWatch-Plan-implementacion-modelo-AWS.md`](../../../Referencias/FireWatch-Plan-implementacion-modelo-AWS.md)

## Specs

**S1 — Proveedor cloud.** El usuario decide pivotar a AWS EC2, contra lo que documentan `Referencias/Actividad C4_FireWatch_produccion.md:235`, `Actividad Dimensionamiento_FireWatch_produccion.md:48-50` y `EP1_...:474-478`, que dicen Oracle Cloud. El cambio se documenta en un registro propio; los originales de `Referencias/` no se reescriben.

**S2 — Quién ejecuta el modelo.** Textual: «todo el backend se hará con Go asi que el servicio de inferencia tiene que estar en go con el modelo guardado en onnx o similiar y python solo de entrenamiento, lo que se habla de fastapi es una sugerencia pero si ya tenemos todo hecho en go  sigamos con go». Forma cerrada: «Go interpreta el artefacto (pesos o árboles)». El artefacto es un JSON versionado con parámetros y preprocesamiento; no se usa `onnxruntime` ni cgo, porque los cuatro servicios compilan hoy con `CGO_ENABLED=0` sobre `gcr.io/distroless/static-debian12:nonroot` (`backend/services/*/Dockerfile:3,12-14`).

**S3 — Superficie de la inferencia.** El servicio no se publica a través del gateway. La consume `risk` por red interna. Razón en L3.

**S4 — Forma de despliegue.** Textual: «Documentar ambas y no cerrar aún». Se implementa Compose en una VM; Kubernetes queda documentado como objetivo, sin declarar el despliegue productivo cerrado.

**S5 — Etiqueta de entrenamiento.** Se intenta el producto CONAF 2010–2025 y se usa el registro 2002/03–2019/20 como respaldo. El estado real de ambas fuentes está verificado en L5 y condiciona las tareas T9.

**S6 — Cobertura comunal.** Textual: «Restringir a las comunas con datos y declararlo». Una comuna sin estación se muestra sin dato; no se imputa con reanálisis. En Valparaíso son 11 comunas con las siete variables, 15 parciales y 12 sin estación (L4).

**S7 — Orden de trabajo.** Textual: «Ambos bloques en paralelo»: A (modelo) y B (despliegue) avanzan a la vez, cada uno con su propio avance verificable.

## Non-goals

- No se entrena ni se publica un modelo con el sample de Maule ni con los 90 días regionales; ambos sirven para probar maquinaria, no como modelo del producto.
- No se implementa Kubernetes ahora, ni ECS.
- No se usa FIRMS como etiqueta mientras el acceso al registro CONAF esté disponible.
- No se modifican los documentos originales de `Referencias/`.

## Tasks

| ID | Spec | Tarea | Ruta | Commit |
| --- | --- | --- | --- | --- |
| T1 | S1, S4 | Registro del cambio de arquitectura Oracle→AWS y lectura dual Compose/k8s | inline | — |
| T2 | S2 | Contrato `firewatch-model/v1` + scorer Go (logístico y árboles) + preprocesamiento declarativo | worker | — |
| T3 | S2 | `backend/services/inference`: `POST /predict`, `/health`, `/metrics`, fixture, distroless, en `go.work` | worker (mismo slice que T2) | — |
| T4 | S2 | Test de equivalencia Go vs Python sobre set congelado, tolerancia 1e-9 | worker (dentro de T2/T3) | — |
| T5 | S4, S7 | `docker-compose.prod.yml` + Caddy, solo el proxy publica 443 | worker | — |
| T6 | S4, S7 | `VITE_*` en frontend, `KC_HOSTNAME`/redirect URIs y gateway `WEB_ORIGIN` | worker (mismo slice que T5) | — |
| T7 | S5 | Ensamblador comuna-día con el sample de Maule: unión por CUT, agregación evento→día, clamp de la etiqueta | worker | — |
| T8 | S2 | Clasificador `ml-v1` en `risk`: llamada HTTP con timeout, fallo cerrado, fila en `models` | pendiente | — |
| T9 | S5 | Solicitar acceso al DOI y escribir a CONAF por el producto 2010–2025 | bloqueado | — |
| T10 | S1 | EC2, dominio, security group y rol IAM para S3 | bloqueado | — |
| T11 | S1 | `deploy.yml`: build/test, push, pull & up, smoke contra la URL pública | pendiente | — |

## Pipeline de entrenamiento

Estado de cada paso. «Existe» significa implementado y probado; «diseñado» significa que el contrato lo contempla pero no hay código; «falta» significa que no hay nada.

| Paso | Datos entregados | Ruta pública (fase 2) |
| --- | --- | --- |
| Cosecha con procedencia y caché | Existe: `fetch_sample.py` | Existe: `meteo.py`, dos fuentes sin credencial |
| Limpieza de llaves y fechas | Existe: CUT, dos formatos, nulos contados | Existe: centroides y ventanas de temporada |
| Unión de fuentes | No aplica: un solo archivo | Falta: panel comuna-temporada |
| Análisis exploratorio | Existe: `ANALYSIS-delivered-data.md`, generado y determinista | Falta para el panel |
| Partición temporal | Existe: julio-agosto entrena, septiembre evalúa | Falta: por temporada |
| Normalización | Existe: ajustada solo con entrenamiento | Falta, reutiliza el mismo código |
| Balanceo | Existe: `class_weight` balanceado | Falta |
| Modelo y comparación | Existe: logística contra bosque | Falta |
| Métricas y línea base | Existe: dos climatologías; ningún modelo las supera | Falta |
| Exportación del artefacto | Existe: artefacto de ensayo, marcado no servible | Falta el de temporada |

---

## Log

**L1 — solicitud original.** «que onda bro está el @"Referencias/FireWatch — Plan de implementación del modelo en AWS.pdf" que no puedes leer pero está el .md en la misma carpeta referencias por si necestias el texto y también están las imagenes del pdf real en @.pi-scratch/pdf para que lo puedas leer tal y como se hizo / me gustaría que analicemos el plan y podamos tomar decisiones a raiz de lo que dice y lo que tenemos porfa»

**L2 — lectura del plan y contraste con las referencias.** El plan afirma que «la guía exige AWS (EC2)» y que el C4 dibuja una API de inferencia FastAPI. Ninguna referencia del repositorio respalda eso: C4:235 dice Oracle Cloud, C4:239 dice Kubernetes con «Compose solo para desarrollo», C4:61 dibuja el contenedor `pred` como «Go + ONNX» y C4:259-261 usa Oracle Object Storage. `tareas_verificacion_implementacion_IA.md:327` acepta «Kubernetes / AWS ECS u otra». Lo que sí es correcto del plan: el diagnóstico de datos (69 positivos de 1440, 4,8 %), la partición temporal, la línea base de climatología, el fallo cerrado en `risk` y la advertencia de honestidad.

**L3 — por qué la inferencia no pasa por el gateway.** El gateway es el borde público: reverse proxy con mux fijo y auth, cuota y CORS en `backend/services/gateway/internal/handler/gateway.go:43,71-78`. Publicar `/predict` ahí permite generar carga arbitraria contra el modelo y sondear su comportamiento; además el frontend ya lee lotes publicados en Postgres, así que ningún cliente lo necesita. El plan lo dice igual: «va en la red interna, nunca publicado».

**L4 — perfilado de los tres CSV.** `dashboard_conaf`: 1440 filas = 16 regiones × 90 días, `nro_inc_act` monótono, 69 días-región positivos (4,79 %), Valparaíso 9. `nro_inc_ant` no es «ayer»: es la curva acumulada de la temporada anterior (431 días `ant > act`, 512 `ant < act`). `ant` y `prom_quint` correlacionan 0,94 entre sí y 0,93 con `act`, así que aportan poco más que la posición en la temporada. `sup_afec_act` tiene 3 revisiones a la baja. `emas`: 535 estaciones, 437 activas, `com_ia_id` es el CUT chileno, `primera_lectura` mínima 2010-01-01. `rtdata`: 5122 filas, 437 estaciones, un solo timestamp, 1437 duplicados por estación-variable, `rtd_i_calidad` y `rtd_i_resultadochecks` 100 % vacías, radiación de 1758 W/m² y presión de 593 hPa sin unidad declarada. Valparaíso: 38 estaciones, 26 comunas con datos, 11 con las siete variables, 12 sin estación.

**L5 — verificación de fuentes de etiqueta.** Geoportal 2010–2025: la ficha `01d06ae3-122e-41fc-9512-1dd2b5067112` existe con ISO 19115 y contacto `jorge.faundez@conaf.cl`, pero no expone distribución, WFS ni descarga. DOI `10.71578/UXAUN5`: Dataverse real con 19 archivos por temporada (`data/20022003/20022003.csv`, 1,55 MB), pero todos con `"restricted": true` y `GET /api/access/datafile/5234` devuelve 403 anónimo; requiere cuenta, condiciones de uso y solicitud de acceso. `github.com/cvidalmsu/FONDEF-IDEA-`: abierto, CC BY 4.0, pero solo región Maule 2018-09-03 → 2019-05-28 con 951 eventos entre 2018-08-16 y 2019-05-28; `extended_training_data.csv` es Maule, solo clase positiva, 1140 filas. Sirve como plantilla de esquema, no como etiqueta.

**L6 — decisión de ejecución.** El usuario elige «Ambos bloques en paralelo». Se abre esta entrega con dos slices en background y superficies de edición disjuntas.

**L7 — primer slice de despliegue (T5, T6).** Ejecutado por un worker en background y spot-checkeado por el orquestador. Entregado: `backend/docker-compose.prod.yml` (19 servicios + `caddy`; solo el proxy publica 80/443; red `backend` con `internal: true`; `${VAR:?required}` para todo lo que cambia por entorno; orden de migraciones intacto), `backend/deploy/prod/Caddyfile` (dos sitios explícitos, un certificado cada uno, `request_body` 64KB, cabeceras de seguridad, CSP nombrando el origen de auth) y `backend/deploy/prod/README.md` (runbook operativo con las tres superficies que deben coincidir en el origen del navegador y la obligación de apagar la instancia). `frontend/src/config.ts` lee `VITE_GATEWAY_ORIGIN`, `VITE_ISSUER` y `VITE_REDIRECT_URI` con fallback local explícito.

Verificación observada: `docker compose -f backend/docker-compose.prod.yml config` exit 0, y exit 1 sin `FIREWATCH_DOMAIN`; `ports:` aparece una sola vez en el archivo de producción; `npm test` 36 archivos / 358 pruebas PASS; `npm run smoke` PASS; `npm run build:local` exit 0.

Dos defectos encontrados y corregidos en esta sesión:
1. El guard usaba `import.meta.env.PROD`, que Vite activa en **todo** build, no solo con `--mode production`. El bundle del demo lanzaba al cargar y rompía el flujo documentado `build` + `serve`. Se cambia a `MODE === 'production'` y se añade `build:local`; CI, `verify` y README apuntan ahí.
2. El build de producción compilaba sin variables y fallaba recién en el navegador, así que un despliegue podía publicar una página en blanco. Se añade `frontend/scripts/require-build-env.mjs` al script `build`: sin las tres variables devuelve exit 1 y nombra las que faltan. Con variables, exit 0.

Pendiente de este slice: los dos `.env.example` fueron rechazados por la política de rutas sensibles del harness (worker y orquestador), así que las variables quedan documentadas en el runbook y en `config.ts`. El Caddyfile **no** se validó por ejecución: no hay daemon de Docker en el entorno, así que su sintaxis solo se probará en el primer `up` real.

**L8 — slice de inferencia y frontera de commits.** El worker de inferencia cerró T2, T3 y T4. Verificación propia del orquestador, no reportada: `go build ./services/inference/...` desde `backend/` exit 0, `go vet` exit 0, `gofmt -l` sin salida, `go test -count=1 ./...` 5/5 paquetes ok, 16 archivos Go, fixtures y `expected.csv` presentes. Sin verificar en local: `-race` y `govulncheck`, que `_go-service.yml` ejecuta en CI; el host no tiene gcc y no se autorizó descargar herramientas. Esos dos son lo primero que hay que mirar en la primera corrida de CI.

Cuatro commits en `feat/aws-ml-delivery`, tras reescribir la rama (no publicada) para dividir el bloque de inferencia:

| Commit | Contenido | Líneas autoradas |
| --- | --- | --- |
| `67d8dac` | Documento de feature, registro del cambio Oracle→AWS, fila del índice | 151 |
| `52a511c` | Artefacto, scorer, fixtures, scorer de referencia en Python, entrada en `go.work` | 2222 |
| `113b755` | Handlers, métricas, `cmd`, Dockerfile, OpenAPI, workflow | 2132 |
| `03b4480` | Compose de producción, Caddyfile, runbook, `VITE_*`, guard de build | 680 |

Integridad de la reescritura verificada: `git diff 1d5ba0a HEAD` muestra solo esta entrada de Log, es decir, no se perdió ni cambió nada más. `go build ./services/inference/...` exit 0 y `go test -count=1 ./...` 5/5 paquetes ok sobre HEAD.

**Frontera de slice: sigue excedida en los dos commits de inferencia.** El objetivo de revisión es ~400 líneas y quedaron en ~2100-2200. El volumen lo domina el material de prueba —fixtures, tests de contrato, OpenAPI—, no el código de producción. Bajarlo más exigiría separar tests de implementación por paquete, lo que empeora la lectura y rompe la regla de mantener los tests junto al comportamiento que cubren. Decisión: se aceptan los cuatro commits; quien revise debe leer el código de producción y recorrer los tests por paquete, y no tratar el conteo como si midiera dificultad.

**L9 — T8 y el desajuste que aparece al conectar el modelo.** `09b755d` (1573 líneas autoradas). El clasificador de modelo llama a `POST /predict` con timeout, y todo fallo —timeout, transporte, 5xx, cuerpo malformado, rechazo— devuelve error y cero `Assessment`, así que no se persiste nada y nunca se consulta la regla. Verificado: `go vet` exit 0 y 11/11 paquetes de test ok; migración 011 siembra `ml-fixture-v1` inactivo y deja `ml-v1` sin registrar a propósito.

**El hallazgo que importa.** Al construir el mapa de features se descubrió que el camino de publicación de `risk` solo puede poblar **2 de las 7** variables del contrato: `humidity_rel` y `wind_speed`. Las otras cinco —`temp_max`, `temp_min`, `wind_dir`, `precip_sum`, `radiation_sol`— viajan como `null` explícito, porque un proveedor que entrega condiciones instantáneas no permite derivar un máximo diario ni una precipitación acumulada de 7 días. El artefacto las imputa y el factor queda registrado, nunca en cero silencioso.

Es el problema de alineación entrenamiento/inferencia hecho código: un modelo entrenado con agregados diarios **no puede servirse** desde un sistema que no guarda historia de lecturas, y el arranque en frío lo agrava porque la acumulación local empieza el día del despliegue. La conclusión es que los agregados, si entran al contrato, tienen que venir del mismo origen histórico que se usó para entrenar (consulta con caché), no de acumulación local.

**L10 — el artefacto pasa a ser la única autoridad del contrato.** `1075015` (+749 -134). El clasificador lee `GET /v1/model` al construirse, rechaza un documento ilegible o incoherente con un error de arranque que nombra el servicio y la razón, y cachea el plan de entrada ordenado. La tabla escrita a mano pasó a ser un mapa de nombre de artefacto a lectura construible: mapeada y disponible envía el valor, mapeada e incompleta conserva el nulo con cobertura, y no mapeada envía un nulo que la nombra. El vector cubre siempre exactamente lo declarado, en su orden.

El guard de coherencia de unidades se probó a sí mismo: al comparar artefacto y clasificador rechazó `precip_sum` declarado en `mm/day` contra el `mm` que `risk` construye, y el arranque falló con el mensaje exacto. Es el comportamiento correcto —una suma es una acumulación, no un promedio diario— y se corrigió en el mismo commit: los dos fixtures declaran `mm`.

Estado verificado: `risk` 12/12 paquetes, `inference` 5/5, `vet` y `gofmt` limpios. Siguen sin ejecutarse las 17 pruebas de base de datos, `-race` y `govulncheck`.

**L11 — ensamblador comuna-día y dos correcciones al registro.** `b07ca08` (+3019). `backend/training/` nace con descarga verificada por sha256 que imprime licencia y cita, un fixture recortado con procedencia y checksums, y un ensamblador que llavea por CUT y fecha local en `America/Santiago`. El sample completo da 951 eventos → 8580 filas comuna-día, 780 con incendio, 171 eventos colapsados, 30/30 comunas resueltas por CUT y 0 filas descartadas. 71 pruebas pasan sin red, con un guard de sockets que se verificó que muerde.

**Corrección 1.** El «502 fechas nulas» que reporté en L5 era un artefacto de mi parser, no un defecto del origen: 502 filas escriben la fecha como `d-mmm-yyyy` y 449 como `dd/mm/yyyy`, y pandas no interpreta los meses en español. Ambas formas se parsean y no se descarta ninguna fila. El rango real empieza el 2018-08-16.

**Corrección 2, la que importa.** De las siete features del artefacto, este origen solo puede producir `humidity_rel`, y como **media diaria**, mientras `risk` envía la última lectura instantánea: mismo nombre y unidad, estimador distinto, registrado en `divergences` de cada manifiesto. Las otras seis son imposibles aquí: el registro CONAF es un catálogo de eventos, así que solo trae meteorología **en los días con incendio**. No hay serie diaria y no hay días tranquilos.

De ahí una conclusión de arquitectura: la meteorología no puede salir del registro de incendios. Tiene que venir de una fuente independiente unida por (comuna, fecha) —ERA5-Land o la red DMC—, y el registro de CONAF aporta únicamente la etiqueta. Cualquier fuente futura que no traiga lecturas en días sin incendio deja esas columnas vacías exactamente donde importa.

Sin ejecutar: `ruff` no está instalado en este host, así que el paso de lint del workflow queda sin verificar hasta su primera corrida.

Dos deudas menores de este slice: el README de `services/inference` sigue diciendo «Not wired yet», y `INFERENCE_URL`/`INFERENCE_TIMEOUT` no están en `backend/docs`. La migración 011 nunca se ejecutó contra Postgres real: no hay daemon ni `TEST_DATABASE_URL` en este host, así que las 11 pruebas de base quedaron en skip.

Pendiente conocido de este slice: el servicio **no** está en `backend/docker-compose.yml`, ni en `SERVICES` del Makefile, ni en los despliegues. Es trabajo posterior y se dejó fuera a propósito para no romper el arranque local mientras no exista un artefacto real.

**L12 — fase «datos entregados» cerrada.** `faa925a` (ensayo de entrenamiento) y `c5776ab` (análisis). El resultado central: sobre septiembre, con julio y agosto para entrenar, **ningún modelo le gana a ninguna de las dos climatologías**. Logística 0,1801 PR-AUC y 0,1754 Brier contra 0,1766 y 0,0952 de la climatología; el bosque recuerda 0,000. La ventaja de 0,0035 en PR-AUC es ruido con 42 positivos, y perder 0,080 de Brier no lo compensa.

Tres garantías por construcción, no por esperanza: la etiqueta es el avance diario recortado del acumulado y las tres revisiones a la baja se cuentan; ninguna columna derivada del acumulado alcanza la matriz de features, y un test mide cuánto vale ese guard (0,0826 limpio → ~0,31 con el acumulado → 1,00 con su diferencia diaria); la normalización no puede ver septiembre porque la función recibe una sola matriz.

**Correcciones al registro anterior.** (1) Las correlaciones que reporté como 0,94 y 0,93 salen solo de Valparaíso; sobre el panel son 0,888 y 0,653/0,612, y el 0,94 aparece recién en el agregado nacional diario con 90 puntos. (2) La presión de 593 hPa es correcta: estación 145, Lago Chungará, unos 4.295 m. Fue un falso positivo mío. (3) `ema_c_codigo` tiene 425 valores no vacíos pero solo 184 distintos: no es llave de estación, así que la alineación con la red DMC que describí como «un test de una hora» no existe por esa vía.

**Hallazgos que se vuelven requisitos.** 39 grupos duplicados de `rtdata` traen valores en conflicto, así que deduplicar exige la fila de mayor `rtd_dt_versiondt`. El máximo de `radiation_sol` del artefacto (1758) es exactamente el dato imposible: el contrato adoptó como cota el valor corrupto. 23 filas de `emas` tienen un CUT que contradice su etiqueta de región, artefacto de numeración previa a 2018. Y 86 estaciones con primera lectura en 2010-01-01 00:00 más 78 vacías: es un piso de migración, no el inicio de la red.

**Estado de la rama.** 13 commits sobre `f3f1d02`, unas 25.000 líneas autoradas. Python en 306 pruebas verdes; los paquetes Go de `risk` (12) e `inference` (5) en verde.

**Sigue sin verificarse.** `ruff` no está instalado en este host; las 17 pruebas de Go que necesitan Postgres; `-race` y `govulncheck`, que corren en CI; y el Caddyfile, que nunca se ejecutó por falta de daemon.

**L13 — cierre de rama: scrape de `inference`, enlace y deudas de verificación.** Este slice no agrega funcionalidad: paga lo que L11/L12 dejaron abierto y corrige un enlace que apuntaba a un archivo inexistente.

**Un hallazgo que sí era defecto.** `inference` publica `/metrics` desde `113b755` y nadie lo scrapeaba: `prometheus.yml` tenía cinco jobs y `FireWatchTargetDown` enumeraba `jobs|risk|gateway|prevention`. El servicio del que `risk` depende para clasificar era el único que podía caerse sin alerta, y justo cuando el diseño de `risk` es fallar cerrado: su ausencia degrada el producto sin señalar la causa. Se agrega el job `inference` con target `inference:8080` —el mismo puerto por el que `risk` lo llama— y la cláusula correspondiente en la alerta, con la razón escrita en ambos archivos para que nadie la borre por parecer redundante.

**Enlace roto.** `aws-ml-delivery.md:4` apuntaba a `../../Referencias/`, que desde `odd/tasks/plan-produccion/` resuelve a `odd/Referencias/` y no existe. Corregido a `../../../`. El chequeo de enlaces reportaba `BROKEN: 0` con el defecto presente, así que esa clase de enlace no está cubierta por él.

**Corrección a L12.** `ruff` **sí** está instalado en este host (0.15.11); la afirmación de L12 era falsa. La primera corrida real falló: dos `E731` en `backend/training/tests/test_train_delivered.py:303-304`, dos `lambda` asignadas a variable que convivían con los `def` equivalentes. Corregidas a `def`; `python -m ruff check .` da «All checks passed!». La deuda real no era «no se pudo correr» sino «falla», y `training.yml:41` la habría marcado en rojo en la primera corrida de CI.

**Verificaciones nuevas en verde.** `pytest -q` → 306 pasan en 14,49 s, offline y contra fixture. `govulncheck` en `risk` e `inference` con la versión que fija CI (`v1.9.0`, `_go-service.yml:36`) y también con `@latest` → «No vulnerabilities found» en ambos casos.

**Sigue sin verificarse, ahora con razón medida.** `go test -race` no corre en este host: `CGO_ENABLED=0` y no hay compilador C, así que falla con «-race requires cgo»; lo ejecuta el runner Linux de CI. Las 17 pruebas de Go que necesitan Postgres siguen sin correr: el daemon de Docker está apagado y hay un PostgreSQL escuchando en :5432 cuya credencial no está disponible para este proceso. El Caddyfile sigue sin ejecutarse: no hay binario `caddy` ni daemon.

**Deudas de L8 saldadas en el camino.** El README de `services/inference` ya no dice «Not wired yet», e `INFERENCE_URL`/`INFERENCE_TIMEOUT` están documentadas en `backend/docs/model-activation.md`; el servicio además ya figura en `backend/docker-compose.yml`. Lo que sigue fuera es su lugar en `docker-compose.prod.yml:21`.

**Repositorio.** `.gitignore` excluye `data/*.csv` —los tres archivos entregados, identificados por sha256 en `backend/training/tests/fixtures/delivered/PROVENANCE.md`— y `.pi-scratch/`, que solo contiene las páginas renderizadas del PDF. El plan fuente y su transcripción entran a `Referencias/`: el documento de ODD los cita, y dejarlos sin versionar mantenía el enlace apuntando a un archivo que ningún clon tendría.

**L14 — colisión de versión de migración entre `risk` y `prevention`, y el lint de CI.** El CI del PR #54 falló en dos jobs, y los dos fallos tenían causa propia.

**El que importaba.** `prevention/internal/repository` falló con nueve pruebas, y solo en esta rama. La causa no era un glob ni un permiso: `prevention` registra `preventive_evidence` como versión **11** en el ledger compartido (`services/prevention/migrations/embed.go:39`), y esta rama registró `ml_fixture_v1` también como **11** (`services/risk/migrations/embed.go`). Los dos migradores de test apuntan a la misma `risk_test` (`docker-compose.yml:287,298`), y CI corre `jobs-test` primero (`integration.yml:26`), cuyo `depends_on` sobre `risk-test-migrate` (`docker-compose.yml:248`) graba el 11 de risk antes de que corra `prevention-test-migrate`. Prevention entonces encuentra la fila 11 ya escrita, se salta su propia 011, y las tres tablas de evidencia nunca se crean. Los `permission denied` del log son ruido de `t.Cleanup` que la migración 009 no concede a propósito (`009_preventive_actions.sql:76-77`), no la causa.

Fix: la migración de esta rama pasa de 11 a **13** —`013_ml_fixture_v1.sql` con el SQL intacto, `embed.go` y `migrations_test.go`—; la 11 de prevention está en `main` desde `1f131c9` y no se mueve. El `INSERT ... ON CONFLICT DO NOTHING` guardado se re-ejecuta como no-op en cualquier base que ya haya grabado el 11 de la rama. Se agrega `TestVersionsDoNotCollideWithPrevention`, que lee `../../prevention/migrations/embed.go` y falla si dos servicios reclaman la misma versión, además de fallar si no parsea ninguna para no pasar en vacío. RED antes del renumerado con el mensaje que nombra la colisión, GREEN después. Nota de lectura: L9 y L12 dicen «migración 011»; el número vigente es 013 y el registro histórico se conserva tal como se escribió.

**El segundo, y una corrección a lo que yo había reportado.** El job de lint no fallaba por código: CI fija `ruff==0.17.4` y este host tiene `0.15.11`. La versión de CI marca cinco `RUF100` —`noqa` inútiles— sobre reglas no habilitadas: tres `ARG001` en `tests/test_fetch_sample.py` y dos `S310` en `fetch_sample.py`. Mi «All checks passed» local no era equivalente al gate, y esa afirmación quedó registrada arriba sin la salvedad de la versión. Las cinco directivas se reemplazan por comentarios que conservan la razón —la firma `(url, timeout)` imita `urlopen`, y la URL es la constante https o el seam de test— sin tocar la configuración de ruff.

Verificado tras el cambio: `ruff check --select RUF100 .` y `ruff check .` en `backend/training` exit 0; `pytest -q` 306 pasan; `go build` y `go vet` exit 0 en `risk` y `prevention`; `go test ./migrations` en `risk` en verde con el guard nuevo; el grep de versiones deja a risk en `1 2 3 4 5 6 7 8 10 12 13` y a prevention en `9 11`, sin intersección. Sigue sin verificarse en runtime: no hay daemon Docker ni `TEST_DATABASE_URL`, así que las nueve pruebas de repositorio siguen en skip en este host y la reparación del esquema se argumenta por la semántica del ledger, no observada.

**L15 — defecto conocido del guard de migraciones; se deja anotado, no se corrige en esta sesión.** El guard de L14 (`TestVersionsDoNotCollideWithPrevention`) lee `../../prevention/migrations/embed.go`, que existe en el árbol integrado pero **no dentro del contenedor** `risk-test`: esa imagen copia solo `services/risk/`. Resultado medido en CI tras `030de28`: el job *Backend integration* falla en el paso `risk-test` con `open ../../prevention/migrations/embed.go: no such file or directory`, mientras los cuatro jobs de lint y las suites por servicio pasan. Es un error de diseño del propio guard, no del renumerado 011→013, que sí quedó corregido.

Nadie lo arregla en esta sesión por decisión del responsable del proyecto: se deja anotado para la siguiente. Lo que hay que decidir entonces, con dos caminos válidos: (a) el guard se salta con mensaje explícito cuando el registro hermano no está disponible, y la comprobación de intersección de versiones se muda a un paso de CI que corre con el árbol completo; o (b) el guard se retira de `services/risk` y la comprobación vive donde el árbol integrado siempre existe. **Estado del PR #54: rojo en *Backend integration* por este motivo, con el resto de la CI en verde.**
