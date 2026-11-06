# FireWatch — entrega de producción con modelo sobre AWS

Feature: ejecutar el plan de implementación del modelo con inferencia en Go y etiquetas reales de CONAF, en dos frentes paralelos (modelo y despliegue). Depende de dos accesos externos; todo lo demás ya es ejecutable.
Estado: en curso · abierto 2026-11-06 · documento fuente: [`Referencias/FireWatch-Plan-implementacion-modelo-AWS.md`](../../Referencias/FireWatch-Plan-implementacion-modelo-AWS.md)

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

## Log

**L1 — solicitud original.** «que onda bro está el @"Referencias/FireWatch — Plan de implementación del modelo en AWS.pdf" que no puedes leer pero está el .md en la misma carpeta referencias por si necestias el texto y también están las imagenes del pdf real en @.pi-scratch/pdf para que lo puedas leer tal y como se hizo / me gustaría que analicemos el plan y podamos tomar decisiones a raiz de lo que dice y lo que tenemos porfa»

**L2 — lectura del plan y contraste con las referencias.** El plan afirma que «la guía exige AWS (EC2)» y que el C4 dibuja una API de inferencia FastAPI. Ninguna referencia del repositorio respalda eso: C4:235 dice Oracle Cloud, C4:239 dice Kubernetes con «Compose solo para desarrollo», C4:61 dibuja el contenedor `pred` como «Go + ONNX» y C4:259-261 usa Oracle Object Storage. `tareas_verificacion_implementacion_IA.md:327` acepta «Kubernetes / AWS ECS u otra». Lo que sí es correcto del plan: el diagnóstico de datos (69 positivos de 1440, 4,8 %), la partición temporal, la línea base de climatología, el fallo cerrado en `risk` y la advertencia de honestidad.

**L3 — por qué la inferencia no pasa por el gateway.** El gateway es el borde público: reverse proxy con mux fijo y auth, cuota y CORS en `backend/services/gateway/internal/handler/gateway.go:43,71-78`. Publicar `/predict` ahí permite generar carga arbitraria contra el modelo y sondear su comportamiento; además el frontend ya lee lotes publicados en Postgres, así que ningún cliente lo necesita. El plan lo dice igual: «va en la red interna, nunca publicado».

**L4 — perfilado de los tres CSV.** `dashboard_conaf`: 1440 filas = 16 regiones × 90 días, `nro_inc_act` monótono, 69 días-región positivos (4,79 %), Valparaíso 9. `nro_inc_ant` no es «ayer»: es la curva acumulada de la temporada anterior (431 días `ant > act`, 512 `ant < act`). `ant` y `prom_quint` correlacionan 0,94 entre sí y 0,93 con `act`, así que aportan poco más que la posición en la temporada. `sup_afec_act` tiene 3 revisiones a la baja. `emas`: 535 estaciones, 437 activas, `com_ia_id` es el CUT chileno, `primera_lectura` mínima 2010-01-01. `rtdata`: 5122 filas, 437 estaciones, un solo timestamp, 1437 duplicados por estación-variable, `rtd_i_calidad` y `rtd_i_resultadochecks` 100 % vacías, radiación de 1758 W/m² y presión de 593 hPa sin unidad declarada. Valparaíso: 38 estaciones, 26 comunas con datos, 11 con las siete variables, 12 sin estación.

**L5 — verificación de fuentes de etiqueta.** Geoportal 2010–2025: la ficha `01d06ae3-122e-41fc-9512-1dd2b5067112` existe con ISO 19115 y contacto `jorge.faundez@conaf.cl`, pero no expone distribución, WFS ni descarga. DOI `10.71578/UXAUN5`: Dataverse real con 19 archivos por temporada (`data/20022003/20022003.csv`, 1,55 MB), pero todos con `"restricted": true` y `GET /api/access/datafile/5234` devuelve 403 anónimo; requiere cuenta, condiciones de uso y solicitud de acceso. `github.com/cvidalmsu/FONDEF-IDEA-`: abierto, CC BY 4.0, pero solo región Maule 2018-09-03 → 2019-05-28 con 951 eventos y 502 fechas nulas; `extended_training_data.csv` es Maule, solo clase positiva, 1140 filas. Sirve como plantilla de esquema, no como etiqueta.

**L6 — decisión de ejecución.** El usuario elige «Ambos bloques en paralelo». Se abre esta entrega con dos slices en background y superficies de edición disjuntas.

**L7 — primer slice de despliegue (T5, T6).** Ejecutado por un worker en background y spot-checkeado por el orquestador. Entregado: `backend/docker-compose.prod.yml` (19 servicios + `caddy`; solo el proxy publica 80/443; red `backend` con `internal: true`; `${VAR:?required}` para todo lo que cambia por entorno; orden de migraciones intacto), `backend/deploy/prod/Caddyfile` (dos sitios explícitos, un certificado cada uno, `request_body` 64KB, cabeceras de seguridad, CSP nombrando el origen de auth) y `backend/deploy/prod/README.md` (runbook operativo con las tres superficies que deben coincidir en el origen del navegador y la obligación de apagar la instancia). `frontend/src/config.ts` lee `VITE_GATEWAY_ORIGIN`, `VITE_ISSUER` y `VITE_REDIRECT_URI` con fallback local explícito.

Verificación observada: `docker compose -f backend/docker-compose.prod.yml config` exit 0, y exit 1 sin `FIREWATCH_DOMAIN`; `ports:` aparece una sola vez en el archivo de producción; `npm test` 36 archivos / 358 pruebas PASS; `npm run smoke` PASS; `npm run build:local` exit 0.

Dos defectos encontrados y corregidos en esta sesión:
1. El guard usaba `import.meta.env.PROD`, que Vite activa en **todo** build, no solo con `--mode production`. El bundle del demo lanzaba al cargar y rompía el flujo documentado `build` + `serve`. Se cambia a `MODE === 'production'` y se añade `build:local`; CI, `verify` y README apuntan ahí.
2. El build de producción compilaba sin variables y fallaba recién en el navegador, así que un despliegue podía publicar una página en blanco. Se añade `frontend/scripts/require-build-env.mjs` al script `build`: sin las tres variables devuelve exit 1 y nombra las que faltan. Con variables, exit 0.

Pendiente de este slice: los dos `.env.example` fueron rechazados por la política de rutas sensibles del harness (worker y orquestador), así que las variables quedan documentadas en el runbook y en `config.ts`. El Caddyfile **no** se validó por ejecución: no hay daemon de Docker en el entorno, así que su sintaxis solo se probará en el primer `up` real.

**L8 — slice de inferencia y frontera de commits.** El worker de inferencia cerró T2, T3 y T4. Verificación propia del orquestador, no reportada: `go build ./services/inference/...` desde `backend/` exit 0, `go vet` exit 0, `gofmt -l` sin salida, `go test -count=1 ./...` 5/5 paquetes ok, 16 archivos Go, fixtures y `expected.csv` presentes. Sin verificar en local: `-race` y `govulncheck`, que `_go-service.yml` ejecuta en CI; el host no tiene gcc y no se autorizó descargar herramientas. Esos dos son lo primero que hay que mirar en la primera corrida de CI.

Tres commits en `feat/aws-ml-delivery`:

| Commit | Contenido | Líneas autoradas |
| --- | --- | --- |
| `b106549` | Documento de feature, registro del cambio Oracle→AWS, fila del índice | 137 |
| `8948e7b` | Servicio `inference`, contrato del artefacto, fixtures, OpenAPI, workflow | 4354 |
| `1d5ba0a` | Compose de producción, Caddyfile, runbook, `VITE_*`, guard de build | 680 |

**Frontera de slice: excedida.** El objetivo de revisión es ~400 líneas por unidad; el commit de inferencia lo supera en un orden de magnitud. Antes de publicar hay que dividirlo: reescribir la rama local (no está publicada, así que es seguro) o dejar un commit y dividir en PRs encadenados por grupo de archivos — contrato y scorer, servicio y handlers, tests y fixtures. La decisión queda abierta y no se publica nada hasta resolverla.

Pendiente conocido de este slice: el servicio **no** está en `backend/docker-compose.yml`, ni en `SERVICES` del Makefile, ni en los despliegues. Es trabajo posterior y se dejó fuera a propósito para no romper el arranque local mientras no exista un artefacto real.
