# v1.0 producción — condiciones de cierre de la versión 1.0 de FireWatch

**Qué es:** documento ODD que define qué falta para llamar `v1.0-produccion` a esta aplicación: diseño web terminado, usuarios y asignación de trabajos, modelo de datos, los doce frentes de producción abiertos, el hardening pendiente, la interfaz declarada no implementada y el cierre documental.
**Estado:** abierto y sin ejecución — planificación únicamente, 46 tareas abiertas y ninguna cerrada; el modelo real queda fuera de este cierre por S2.
**Acta:** [`odd/entrega/acta-cierre.md`](../entrega/acta-cierre.md) §5 (corrección del responsable) y §6 (alcance de la versión 1.0).

## Specs

**S1 — La aplicación no está terminada.** Corrección del responsable del proyecto, textual:

> «con esto estoy dando por hecho de que la app NO está terminada, aún nos falta terminar el diseño de la pagina web, tener el modelo funcionando, agregar los usuarios para designarles los trabajos, el modelo de datos que necesitamos para que funcione la app, aún queda e, con esto listo no terminamos el trabajo, hay un monton de tasks no cerradas en odd que tenemos que cerrar antes de llamar a esta version el tav 1.0 produccion»

Consecuencia: lo entregado es un corte verificable de un sistema en construcción, no su versión final. La versión 1.0 exige cerrar las cuatro capacidades que la cita nombra —diseño de la página web, modelo, usuarios y asignación de trabajos, modelo de datos— y las tareas abiertas de `odd/`. Registro: `../entrega/acta-cierre.md:73-93`.

**S2 — v1.0 se cierra sin modelo real.** Decisión del responsable del proyecto, textual:

> «B, se cierra sin modelo real, no importa que no funcione bien pero dejamos el frente abierto para que más adelante consigamos los datos y armemos un modelo con los datos que si nos sirven»

Consecuencia, sin rodeos: un modelo con mal desempeño no es requisito de la versión 1.0. El frente del modelo queda abierto, no abandonado, y lo que lo desbloquea es S3. Registro: `../entrega/acta-cierre.md:94-106`.

**S3 — La condición de datos ya está medida.** El frente del modelo se desbloquea solo con dos insumos, y ambos están medidos: (a) meteorología diaria en días **sin** incendio —ERA5-Land o la red DMC— unida por (comuna, fecha); y (b) acceso al registro de eventos de CONAF, DOI `10.71578/UXAUN5`, hoy 403 anónimo (`plan-produccion/aws-ml-delivery.md:74`).

La autoridad es `backend/training/ANALYSIS-delivered-data.md` §4 (`:513-529`), que mide archivo por archivo y concluye que ninguno de los tres entregados trae clima diario por comuna ni un evento con comuna y fecha. Lo que pide, textual (`:531-540`):

> 1. **An event registry keyed by comuna CUT with a local date.** One row per fire,
>    not per region-day: that is the only way the label stops being a counter's
>    first difference. The product the plan's Log L5 names is access-gated; nothing
>    in these three files substitutes for it.
> 2. **Daily weather on every day, including quiet ones, at a point in each comuna.**
>    Hourly station readings aggregated to a declared civil day, or a reanalysis
>    already aggregated -- `meteo.py` shows both shapes. Without the quiet days the
>    features exist only where the label is 1.

**S4 — El tag `v1.0-produccion` no se emite hasta cerrar este documento.** `../entrega/acta-cierre.md` §5 (`:73-93`) establece que el tag queda bloqueado hasta cerrar las tareas abiertas de `odd/` y las capacidades faltantes, y que la entrega anterior no se llama `v1.0-produccion`; E4 (`:25-27`) se conserva como historia, no como decisión vigente. El §6 (`:94-106`) fija el alcance: la versión 1.0 se computa sobre el diseño web terminado, los usuarios y la asignación de trabajos, el modelo de datos, los doce frentes abiertos y las unidades de hardening pendientes.

## Tasks

Ninguna tarea de esta tabla está cerrada y ninguna acredita implementación. Rutas: **inline** = trabajo documental o acotado que ejecuta quien mantiene este documento; **worker** = implementación delegada con superficie declarada; **bloqueado** = no puede empezar hasta que exista el insumo que se nombra en la misma celda.

### G1 — Diseño web (V1-D)

El único antecedente de diseño de la página está marcado **Histórico** en el índice (`README.md:38`). El plan que construyó la aplicación React declara la arquitectura y un sistema de diseño de partida (`../urgent/rediseno-pagina/app-react-vite.md:17,30`), enumera siete vistas (`:18`) mientras la aplicación monta ocho (`../../frontend/src/App.tsx:6-13`), y el documento de estructura que lo precede está declarado SUPERSEDIDO (`../urgent/rediseno-pagina/reestructuracion-frontend.md:3`). Ningún documento de `odd/` audita ni fija el diseño visual de las ocho vistas actuales.

Los tokens existen y se importan (`../../frontend/src/main.tsx:7`); `../../frontend/README.md:24` los resume en una línea, y la razón de contraste de `--muted` vive en un comentario dentro del propio CSS (`../../frontend/src/styles/tokens.css:25-29`), no en un documento del sistema. Las ocho vistas son `ActionsView`, `AuditView`, `HistoryView`, `JobsView`, `OverviewView`, `PendingView`, `SessionView` y `SourcesView`.

El límite de accesibilidad está declarado por la propia entrega (`../../frontend/README.md:149`: sin certificación WCAG, sin lector de pantalla, y la aserción de foco a 390 px cierra sobre BODY cuando la lista de acciones está vacía), y el alcance de accesibilidad sigue como decisión abierta D5 (`../entrega/acta-cierre.md:29-39`).

| ID | Tarea | S# | Cierra cuando | Ruta |
| --- | --- | --- | --- | --- |
| V1-D-01 | Documentar `frontend/src/styles/tokens.css` como sistema de diseño: paleta, escala tipográfica, espaciado, radios, sombras y uso por componente. | S1 | El documento enumera cada token declarado en `frontend/src/styles/tokens.css` con su valor y el componente o vista que lo consume, y cada par texto/fondo en uso lleva su contraste medido con la cifra. | inline |
| V1-D-02 | Auditar las ocho vistas contra ese sistema: `ActionsView`, `AuditView`, `HistoryView`, `JobsView`, `OverviewView`, `PendingView`, `SessionView` y `SourcesView`. | S1 | Existen ocho filas de auditoría, una por vista y ninguna ausente, y cada desviación encontrada cita el `archivo:línea` que la muestra. | inline |
| V1-D-03 | Restilizar cada vista según lo que marque la auditoría, sin cambiar comportamiento ni los identificadores que las pruebas fijan. | S1 | Cada desviación de V1-D-02 está cerrada en la vista que la declaró, o justificada por escrito con su razón, y la suite de pruebas de frontend sigue en verde. | worker |
| V1-D-04 | Accesibilidad con WCAG AA como meta medible. | S1 | Un informe registrado evalúa los criterios WCAG AA aplicables a las ocho vistas, con herramienta, versión y fecha declaradas, y cada criterio tiene resultado pasa/falla; los fallos quedan corregidos. | worker |
| V1-D-05 | Verificación visual con evidencia antes/después. | S1 | Por cada una de las ocho vistas existe una captura de antes y otra de después en el mismo navegador y viewport, con fecha y con su ruta registrada. | inline |

### G2 — Usuarios y asignación (V1-U)

El contrato no expone ninguna ruta de usuarios (`backend/contracts/openapi/firewatch-v1.yaml` no tiene `/v1/users` ni un endpoint de identidad) y ninguna migración crea una tabla de usuarios: las migraciones de los tres servicios crean `audit`, `batches`, `inbox`, `jobs`, `models`, `outbox`, `readings` y `zones`, más las cinco tablas del esquema `prevention`.

La asignación de una acción preventiva es texto libre a propósito y sin clave foránea (`backend/services/prevention/migrations/009_preventive_actions.sql:30,39`).

El realm declara dos roles, `viewer` y `admin` (`backend/deploy/local/firewatch-realm.json:8`); `operator` no es un rol del realm y los handlers lo rechazan como desconocido (`backend/services/risk/internal/handler/api_test.go:111`). El selector de rol de la página no está portado por decisión registrada (`frontend/src/views/SessionView.tsx:340-345`): el rol que se muestra es el que declara el token y no altera ninguna petición.

| ID | Tarea | S# | Cierra cuando | Ruta |
| --- | --- | --- | --- | --- |
| V1-U-01 | Directorio de usuarios leído del proveedor de identidad. | S1 | Existe un endpoint autenticado que devuelve los usuarios del realm con identificador y nombre para mostrar; la ruta, el esquema y sus errores están en `backend/contracts/openapi/firewatch-v1.yaml`, y un contract test los compara contra la respuesta real. | worker |
| V1-U-02 | Referencia de usuario en la asignación de acciones preventivas. | S1 | Una migración nueva en `backend/services/prevention/migrations/` sustituye `assignee text` por una referencia con clave foránea, con la versión libre que determine la regla de V1-M-04; se aplica y se vuelve a aplicar sin error sobre una base aislada con acciones existentes, y un identificador ausente del directorio se rechaza con un error documentado. | worker |
| V1-U-03 | Asignar una acción preventiva a una persona del directorio. | S1 | `POST /v1/prevention/actions` acepta el identificador de un usuario del directorio, lo persiste y lo devuelve en la lectura; una prueba cubre la asignación, el identificador inexistente y la acción sin asignar. | worker |
| V1-U-04 | Vista para elegir y ver el responsable de una acción preventiva. | S1 | La vista de acciones permite elegir un usuario del directorio al crear una acción y muestra el responsable de cada una; su prueba cubre sin asignar, asignada y error del servidor. | worker |

### G3 — Modelo de datos (V1-M)

No existe ERD ni documento de esquema de dominio: el esquema vive solo como SQL en `backend/services/{risk,jobs,prevention}/migrations/`.

Las versiones de migración son un recurso compartido: `risk` usa 001–008, 010, 012 y 013; `jobs` 001–004; `prevention` 009 y 011. Los tres escriben en el mismo ledger `schema_migrations`, y la versión reserva contra el árbol integrado, no contra un servicio (`backend/services/risk/migrations/migrations_test.go:66-72`). El choque de la versión 11 es el precedente: `030de28` movió `ml_fixture_v1` de 11 a 13 porque `prevention` ya reclamaba la 11, y agregó el guard que falla si dos servicios reclaman la misma versión (`backend/services/risk/migrations/migrations_test.go:88-92`).

D1 —motor de datos productivo, backups y SLA— sigue abierta y sin responsable asignado (`../entrega/acta-cierre.md:29-39`).

| ID | Tarea | S# | Cierra cuando | Ruta |
| --- | --- | --- | --- | --- |
| V1-M-01 | Inventario del esquema real por servicio. | S1 | Un documento lista cada tabla, columna, tipo y clave foránea que crean `backend/services/{risk,jobs,prevention}/migrations/`, y un diff contra el catálogo de una base migrada en aislado no muestra diferencias. | inline |
| V1-M-02 | Modelo de datos de dominio: entidades, relaciones y ERD. | S1 | El ERD cubre las trece tablas de V1-M-01 y cada relación dibujada aparece con su clave foránea en el inventario; ninguna tabla queda fuera del diagrama. | inline |
| V1-M-03 | D1 — motor de datos productivo, backups y SLA. | S1 | El acta registra el motor elegido, su impacto en migraciones y persistencia, la política de backups y el SLA, con responsable y fecha. | bloqueado — por D1, decisión humana (`../entrega/acta-cierre.md:29-39`) |
| V1-M-04 | Regla de versiones del ledger compartido de migraciones. | S1 | La regla está escrita junto al migrador, con el rango de versiones que usa cada servicio, y una prueba falla cuando dos servicios reclaman la misma versión, como el guard existente; la versión de la próxima migración de `prevention` queda determinada por esa regla. | inline |

### G4 — Frentes de producción abiertos (V1-F)

De los quince frentes de `../plan-produccion/README.md`, tres están cerrados (3, 4 y 10) y doce siguen abiertos: 1, 2, 5, 6, 7, 8, 9, 11, 12, 13, 14 y 15 (`:258-289` de ese documento). Esos doce frentes concentran 65 de las 78 tareas de frente; los tres cerrados aportan las 13 restantes.

El frente 2 no condiciona el cierre de la versión 1.0: entra aquí solo como el frente diferido que describe S3, y su fila lo declara.

| ID | Tarea | S# | Cierra cuando | Ruta |
| --- | --- | --- | --- | --- |
| V1-F-01 | Frente 1 — Ingesta de fuentes reales y ciclo de 15 min (`../plan-produccion/01-ingesta-scheduler.md`). | S1 | La prueba de sustitución pasa sin cambios fuera de `backend/services/risk/internal/source/`, el fixture es determinista con semilla, y el ciclo de 15 min y la frescura tienen evidencia reproducible con umbrales acordados, de modo que `backend/docs/verification.md:98-101` deja de declararlos entre lo no establecido. | inline |
| V1-F-02 | Frente 2 — Servicio de inferencia: solo como frente diferido; la sustitución real no se exige en v1.0. | S2, S3 | No condiciona la versión 1.0. Cierra cuando exista meteorología diaria en días sin incendio unida por (comuna, fecha) y acceso al registro de eventos de CONAF, es decir, cuando se cumpla S3. | bloqueado — por S3 (datos meteorológicos y registro CONAF) |
| V1-F-05 | Frente 5 — Observabilidad productiva (`../plan-produccion/05-observabilidad.md`); `LOG-I3` (ground truth) queda fuera, depende de S3. | S1 | Un scrape de Prometheus en un entorno aislado registra RPS, p95, memoria y errores de los cuatro servicios; las cifras quedan marcadas como medición y no como objetivo (`../urgent/04-operacion-monitoreo.md:21`); y `promtool` valida `backend/deploy/local/alerts.yml` sin error. | inline |
| V1-F-06 | Frente 6 — Autoscaling, carga y p95/SLO (`../plan-produccion/06-autoscaling-carga-slo.md`). | S1 | Queda registrado un reporte pasa/falla de k6 con `p(95)<500`, `max<2000` y `http_req_failed<0.01`, el escalado del HPA observado en clúster y el RTO medido en ≤ 15 min. | inline |
| V1-F-07 | Frente 7 — Firma, registry y rollout (`../plan-produccion/07-firma-registry-rollout.md`). | S1 | Una imagen firmada se verifica antes de habilitar su promoción, la promoción entre entornos queda registrada y automatizada, y un rollout en Kubernetes se verifica con su rollback en el entorno de destino. | inline |
| V1-F-08 | Frente 8 — Gobierno y seguridad, U1 (`../plan-produccion/08-gobierno-seguridad-u1.md`). | S1 | Los 41 IDs de U1 tienen estado, enlace de evidencia, criterio de aprobación y resultado en `backend/docs/checklist-evidence-u1.md`, cada N/A queda justificado (`../urgent/01-seguridad-gobierno.md:21-23`), y el responsable de seguridad de IA está nombrado. | inline |
| V1-F-09 | Frente 9 — Login real en navegador, móvil y accesibilidad (`../plan-produccion/09-login-browser-a11y.md`). | S1 | Una medición registrada cubre los criterios WCAG con resultado por criterio y una prueba con lector de pantalla, dentro del alcance que D5 fije por escrito, sin fallos abiertos. | inline |
| V1-F-11 | Frente 11 — Hardening backend BH-01…BH-11 (`../plan-produccion/11-hardening-bh.md`). | S1 | Las doce unidades tienen su verificación registrada en su documento y ninguna queda en estado `pending`; BH-12 cierra solo con su activación explícita registrada. | worker |
| V1-F-12 | Frente 12 — Evaluación científica (`../plan-produccion/12-evaluacion-cientifica.md`). | S3 | El protocolo, las métricas, las tolerancias y los criterios están escritos, y la evaluación se ejecuta contra datos reales con ground truth. | bloqueado — por S3 (datos reales y modelo evaluable) |
| V1-F-13 | Frente 13 — Go/No-Go y firma (`../plan-produccion/13-go-no-go.md`). | S3 | El historial de versiones y la matriz de pruebas tienen resultado por criterio, y la decisión APROBADO/RECHAZADO está firmada por el responsable, hoy sin definir. | bloqueado — por S3 y por la firma del responsable |
| V1-F-14 | Frente 14 — Motor de datos y continuidad (`../plan-produccion/14-motor-datos-continuidad.md`). | S1 | El motor productivo está decidido y documentado con su impacto en migraciones y persistencia, la política de backups y restauración está probada, y el RPO/RTO acordado se cumple en una prueba de recuperación. | bloqueado — por D1, decisión humana |
| V1-F-15 | Frente 15 — Procedencia geoespacial y restricción DIFROL (`../plan-produccion/15-procedencia-geoespacial.md`). | S1 | La primera fila real de trazabilidad queda registrada con endpoint, huella de consulta, `retrieved_at`, `cutoff_at` y los formatos de origen y derivado; la atribución a SUBDERE, IGM e INE consta; y la restricción DIFROL está declarada textualmente en la tabla y el README. | inline |

### G5 — Hardening (V1-H)

Estado de las unidades en `README.md:63-77`: BH-01, BH-03, BH-07, BH-08 y BH-09 cerradas o implementadas; BH-02 parcial —los cuatro módulos Go están limpios y falta el inventario y el scan de imágenes, que exigen daemon de Docker y `trivy` (`security/backend-hardening/02-vulnerability-scans.md:3,7`)—; BH-04, BH-05, BH-06, BH-10, BH-11 y BH-12 pendientes o diferidas.

Cada fila cierra con lo que su propio documento pide. Este documento no adelanta ninguna de esas verificaciones.

| ID | Tarea | S# | Cierra cuando | Ruta |
| --- | --- | --- | --- | --- |
| V1-H-02 | BH-02 — inventario de imágenes y scan de imágenes (`security/backend-hardening/02-vulnerability-scans.md`). | S1 | El inventario de imágenes con sus digests y el scan con la versión de `aquasec/trivy` que fija CI quedan registrados por target, con fecha, y el resultado se distingue explícitamente de un fallo del escáner. | inline |
| V1-H-04 | BH-04 — mínimo privilegio del broker (`security/backend-hardening/04-broker-least-privilege.md`). | S1 | La matriz de operaciones permitidas por servicio está escrita, cada servicio tiene identidad propia, y una publicación o un consumo no permitido se rechaza en el broker de destino con evidencia sanizada de allow/deny. | worker |
| V1-H-05 | BH-05 — seguridad de transporte (`security/backend-hardening/05-transport-security.md`). | S1 | Cada ruta de transporte tiene su prueba: el texto plano se rechaza en configuración no de desarrollo, un certificado no confiable o vencido falla, y la excepción de desarrollo no puede habilitar texto plano en producción. | worker |
| V1-H-06 | BH-06 — egress de Kubernetes (`security/backend-hardening/06-kubernetes-egress.md`). | S1 | En el entorno autorizado, los flujos requeridos —DNS, IdP/JWKS, broker, base y servicios internos— funcionan, un destino no permitido se deniega, y se confirma que el plugin instalado es el que aplica la restricción. | worker |
| V1-H-10 | BH-10 — idempotencia concurrente (`security/backend-hardening/10-concurrent-idempotency.md`). | S1 | Con base y broker aislados, el mismo actor con la misma clave produce un único resultado lógico, actores y claves distintos quedan independientes, y los conflictos de unicidad y el rollback no dejan estado parcial ni efectos duplicados; la repetibilidad y la limpieza quedan registradas. | worker |
| V1-H-11 | BH-11 — seguridad de despliegue verificada (`security/backend-hardening/11-deployment-security-verification.md`). | S1 | Los controles de BH-04, BH-05 y BH-06 se verifican sobre el despliegue elegido, non-root, sistema de archivos de solo lectura y capabilities quedan registrados con sus excepciones, y el reporte de exposición de secretos es sanizado. | worker |
| V1-H-12 | BH-12 — errores centralizados (`security/backend-hardening/12-centralized-error-handling.md`). | S1 | La activación explícita está registrada y, después, las categorías de error y su mapeo de estado son iguales entre servicios, `request_id` se propaga igual y ningún error interno llega crudo al cliente, con contract tests y documentación actualizados. | bloqueado — por activación explícita |

### G6 — Interfaz declarada no implementada (V1-P)

`frontend/src/views/PendingView.tsx:22-65` declara siete capacidades que el backend no respalda y las rotula «Aún no implementado». La lista se exporta y una prueba fija su número de filas contra el contenido renderizado (`frontend/tests/views/pending.test.tsx:35`).

Cada fila cierra cuando la capacidad existe en el contrato o en el backend y en la página, y su entrada sale de esa lista; el nombre de la fila es el que la vista usa.

| ID | Tarea | S# | Cierra cuando | Ruta |
| --- | --- | --- | --- | --- |
| V1-P-01 | `Cobertura y entorno por sector`. | S1 | El contrato publica cobertura y entorno por comuna con contract test, la tabla de sectores los muestra, y la entrada sale de `NOT_IMPLEMENTED`. | worker |
| V1-P-02 | `Contexto y prioridad por sector`. | S1 | El backend calcula prioridad regional y contexto por comuna con una regla escrita, la ficha de detalle los muestra, y la entrada sale de `NOT_IMPLEMENTED`. | worker |
| V1-P-03 | `Estados candidate, in_progress, reviewed y closed`. | S1 | Los cuatro estados existen en el contrato y en la persistencia de acciones, cada transición válida está probada y una transición inválida se rechaza, los chips los muestran, y la entrada sale de `NOT_IMPLEMENTED`. | worker |
| V1-P-04 | `Historial de todas las zonas`. | S1 | Un endpoint agregado devuelve el historial sin exigir `zone_id`, está documentado en el contrato con contract test, la vista de historial lo consume, y la entrada sale de `NOT_IMPLEMENTED`. | worker |
| V1-P-05 | `Revisar e incorporar registros, y hallazgos de calidad`. | S1 | Revisar e incorporar registros tiene una acción autenticada y probada, existe un catálogo de hallazgos de calidad con su endpoint y su vista, y la entrada sale de `NOT_IMPLEMENTED`. | worker |
| V1-P-06 | `Publicar lote, avance manual y reevaluación`. | S1 | Las tres acciones existen como endpoints autenticados con autorización por rol, una prueba cubre el rechazo por rol y el estado resultante, la página las ofrece, y la entrada sale de `NOT_IMPLEMENTED`. | worker |
| V1-P-07 | `Nombre del actor, detalle libre e id de evento`. | S1 | `GET /v1/audit` devuelve nombre del actor, detalle libre e identificador de evento por fila, documentado en el contrato con contract test, la vista de auditoría los muestra, y la entrada sale de `NOT_IMPLEMENTED`. | inline |

### G7 — Cierre documental (V1-C)

Cinco IDs pueden cerrarse sin datos nuevos: `GOB-I1` (`backend/docs/checklist-evidence-u1.md:55`), `AME-I1` (`:66`), `AME-I2` (`:67`), `RED-I3` (`:138`) y `GNG-I1` (`backend/docs/checklist-evidence-u5.md:92`). Ninguno está cerrado: los cinco figuran como `Pending` y autocontenidos (`backend/docs/checklist-evidence-u1.md:147`, `backend/docs/checklist-evidence-u5.md:115`).

El corte documental del cierre incluye regenerar `../presentacion-mandante.md` —hoy fechado el 2026-10-20 (`../presentacion-mandante.md:3`)— y actualizar el acta.

| ID | Tarea | S# | Cierra cuando | Ruta |
| --- | --- | --- | --- | --- |
| V1-C-01 | `GOB-I1` — política interna de clasificación de riesgo de IA. | S1 | La política existe en el repositorio con su escala y su mapeo a la regulación elegida, su adopción queda registrada con el aprobador nombrado, y su fila de `backend/docs/checklist-evidence-u1.md` cita el documento. | inline |
| V1-C-02 | `AME-I1` — ejercicio de modelado de amenazas MITRE ATLAS. | S1 | El ejercicio existe como documento con su catálogo de amenazas y el diagrama de flujo de datos del diseño de IA, y su fila de `backend/docs/checklist-evidence-u1.md` lo enlaza. | inline |
| V1-C-03 | `AME-I2` — registro vector por vector con su mitigación. | S1 | El registro enumera los vectores ATLAS aplicables —inversión, extracción, inyección de prompt y los demás— con una mitigación asignada por vector, depende de V1-C-02, y su fila lo enlaza. | inline |
| V1-C-04 | `RED-I3` — plan de respuesta a incidentes. | S1 | El plan existe con roles, escalamiento y el disparador de rollback, y su fila de `backend/docs/checklist-evidence-u1.md` lo enlaza. | inline |
| V1-C-05 | `GNG-I1` — documento Go/No-Go con historial de versiones. | S1 | El documento existe con su historial de versiones —fecha, versión, autor y descripción—, sin declarar aprobación de producción, y su fila de `backend/docs/checklist-evidence-u5.md` lo enlaza. | inline |
| V1-C-06 | Regenerar `odd/presentacion-mandante.md` con el corte de cierre. | S1 | El documento se regenera contra el commit del cierre, su encabezado declara la base y la fecha, y `python -B odd/tools/check_md_links.py` devuelve `BROKEN: 0`. | inline |
| V1-C-07 | Actualizar el acta de cierre con el estado de v1.0. | S4 | `../entrega/acta-cierre.md` registra la fecha de cierre de este documento, el commit sobre el que se emite `v1.0-produccion` y el estado del tag, y su §5 deja de bloquearlo. | inline |

## Log

- **L1 (2026-11-09) — Corrección del responsable del proyecto, textual:**
  > «con esto estoy dando por hecho de que la app NO está terminada, aún nos falta terminar el diseño de la pagina web, tener el modelo funcionando, agregar los usuarios para designarles los trabajos, el modelo de datos que necesitamos para que funcione la app, aún queda e, con esto listo no terminamos el trabajo, hay un monton de tasks no cerradas en odd que tenemos que cerrar antes de llamar a esta version el tav 1.0 produccion»

  La entrega anterior deja de llamarse `v1.0-produccion` y el tag queda bloqueado (S1, S4).
- **L2 (2026-11-09) — Decisión del responsable del proyecto, textual:**
  > «B, se cierra sin modelo real, no importa que no funcione bien pero dejamos el frente abierto para que más adelante consigamos los datos y armemos un modelo con los datos que si nos sirven»

  El modelo real queda fuera de la versión 1.0 y el frente queda abierto (S2, S3).
- **L3 (2026-11-09) — Alcance de la versión 1.0 fijado en esta sesión.** El inventario de partida: 0 de 132 IDs de requisito cerrados (`../estado-requerimientos-cliente.md:17`); 12 de los 15 frentes de producción abiertos (`../plan-produccion/README.md:258-289`); 65 de las 78 tareas de frente en esos doce frentes. El modelo se excluyó del cierre por decisión del responsable (S2). Este documento es el plan resultante: 46 tareas abiertas, ninguna cerrada, y no acredita implementación ni evidencia nueva.
