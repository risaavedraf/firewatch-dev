# Entrega de datos y modelo: informe para el mandante

Este informe explica por qué el modelo de riesgo de incendios no puede construirse con los datos entregados y qué entrega el sistema en su lugar, en términos verificables por quien evalúa. Documenta el **corte actual** del proyecto: no es el documento de cierre de la versión 1.0, que sigue abierta (ver `odd/entrega/acta-cierre.md`, §5).

**Anexos técnicos.** Las cifras de este informe provienen de dos anexos que son la autoridad: `backend/training/REPORT-delivered-data.md` (la medición) y `backend/training/ANALYSIS-delivered-data.md` (el análisis archivo por archivo). Ambos los generan scripts —`train_delivered.py` y `analyze_delivered.py`— y no deben editarse a mano: una edición manual es un defecto, no un atajo. Si una cifra difiere, la autoridad son los anexos.

**Convención numérica.** Las cifras se transcriben tal como aparecen en los anexos, con punto decimal y sin redondeo, para compararlas dígito a dígito; no se promedian ni se derivan valores nuevos.

**Fuentes citadas.** Los dos anexos; `odd/tasks/plan-produccion/aws-ml-delivery.md` (registro de la entrega, entradas L7 a L13); y `odd/plan-produccion/README.md` (estado de los quince frentes de producción).

## 1. Resumen ejecutivo

- Se entrenaron dos modelos —regresión logística y bosque— sobre los 90 días entregados y se midieron contra dos líneas base de tipo climatología. La regla que decide qué cuenta como victoria es explícita: superar una línea base exige, a la vez, mayor PR-AUC y menor Brier.
- Ninguno de los dos modelos supera a ninguna de las dos líneas base. La logística obtiene 0.1801 de PR-AUC y 0.1754 de Brier, contra 0.1766 y 0.0952 de la climatología; el bosque obtiene 0.1246 de PR-AUC y no emite ninguna alerta.
- La causa no es el método sino la entrada. De ocho propiedades que el modelo necesita, los tres archivos entregados responden una sola —la cobertura de estaciones— y ninguno trae meteorología diaria en días sin incendio, eventos identificados por comuna ni unidades declaradas.
- Por eso la entrega honesta no es un modelo: es el sistema que ya funciona —inferencia en Go, clasificador que falla cerrado, ensamblador comuna-día, pila de producción— más este hallazgo medido y reproducible con comandos.
- El artefacto que existe en el repositorio es un ensayo, marcado como no registrable en sus propios bytes y deliberadamente no servible por `risk`. No es un modelo en uso y no debe presentarse como tal.

## 2. Qué se entregó

Lo entregado es un sistema que se ejecuta hoy, no una maqueta: cinco piezas con verificación propia.

| Componente | Qué es | Verificación observada |
| --- | --- | --- |
| Servicio de inferencia (`backend/services/inference`) | Servicio Go que interpreta el artefacto del modelo —pesos o árboles— sin `onnxruntime` ni `cgo`, y expone `POST /predict`, `/health` y `/metrics`; se empaqueta sobre una imagen distroless | `go build` exit 0, `go vet` exit 0, `gofmt -l` sin salida y 5/5 paquetes de prueba en verde |
| Clasificador `ml-v1` en `risk` | Construye el vector de entrada desde el artefacto, llama a la inferencia con timeout y falla cerrado | 12/12 paquetes en verde; el guard de coherencia de unidades se probó a sí mismo rechazando un artefacto incoherente y deteniendo el arranque con el mensaje exacto |
| Ensamblador comuna-día (`backend/training`) | Descarga verificada por sha256, unión por CUT y fecha local en `America/Santiago`, agregación evento→día y recorte de la etiqueta | 951 eventos producen 8580 filas comuna-día, 780 con incendio, 171 eventos colapsados, 30/30 comunas resueltas por CUT y 0 filas descartadas; 71 pruebas pasan sin red |
| Pila de producción (`docker-compose.prod.yml` y `Caddyfile`) | 19 servicios más el proxy; solo el proxy publica 80/443; red `backend` interna; variables obligatorias con `${VAR:?required}`; cuerpo de solicitud limitado a 64KB | `docker compose -f backend/docker-compose.prod.yml config` exit 0, y exit 1 sin `FIREWATCH_DOMAIN`; `ports:` aparece una sola vez en el archivo de producción |
| Pruebas transversales | Frontend, humo y build de demostración | `npm test` 36 archivos / 358 pruebas en PASS; `npm run smoke` PASS; `npm run build:local` exit 0 |

**El servicio de inferencia.** Es la pieza que se pidió en Go, con el modelo guardado como artefacto versionado. Se eligió un JSON con parámetros y preprocesamiento declarativo porque los cuatro servicios compilan con `CGO_ENABLED=0`; una dependencia nativa habría roto esa condición. El servicio no se publica por el gateway: lo consume `risk` por red interna, de modo que no puede usarse como superficie pública de carga o de sondeo (L3).

**El clasificador y el fallo cerrado.** `risk` delega la clasificación y trata todo fallo como fallo: timeout, error de transporte, respuesta 5xx, cuerpo malformado o rechazo devuelven error y cero evaluaciones. No se persiste nada y nunca se consulta la regla de respaldo. En términos del mandante: si el modelo no puede responder, el sistema lo dice, no inventa un nivel de riesgo.

**El ensamblador comuna-día.** Convierte eventos de CONAF en filas de comuna y día. Su resultado actual usa el único origen abierto disponible —la región de Maule, desde el 2018-08-16— y sirve como maquinaria probada, no como conjunto nacional.

**La pila de producción.** El Compose define 19 servicios más el proxy y una red interna que impide exponer los servicios de datos. El `Caddyfile` declara dos sitios explícitos, un certificado cada uno, cabeceras de seguridad y una política de contenido que nombra el origen de autenticación.

**Los límites de esta pieza, dichos en su lugar.** El servicio de inferencia figura en `backend/docker-compose.yml`, pero todavía no en `docker-compose.prod.yml` (L13). No hay un modelo real registrado: la migración 011 siembra `ml-fixture-v1` inactivo y deja `ml-v1` sin registrar a propósito.

## 3. Qué se midió

La medición se hizo sobre el archivo `dashboard_conaf_202609281623 (1).csv` —1440 filas, 16 regiones, 90 días— con la corrida registrada `2026-11-06T22:50:25Z`. La etiqueta es el avance diario de un contador acumulado de incidentes, recortado en cero. El archivo contiene 69 días-región positivos sobre 1440, un 4.79%, y 16 filas sin predecesor cuya etiqueta es estructuralmente cero.

### 3.1 La partición

| Lado | Meses | Días-región | Positivos | Tasa base |
| --- | --- | --- | --- | --- |
| Entrenamiento | julio y agosto | 992 | 27 | 2.72% |
| Prueba | septiembre | 448 | 42 | 9.38% |

La partición es temporal y nunca aleatoria: septiembre sigue a agosto, y una partición aleatoria permitiría que el modelo aprendiera un día de septiembre a partir de uno de julio. La corrida verifica que los dos lados sean disjuntos y estén estrictamente ordenados antes de ajustar cualquier cosa. La tasa base del mes en que se juzga es 3.4 veces la del entrenamiento (9.38% contra 2.72%), porque la temporada sube: un umbral elegido en julio y agosto subalerta en septiembre, y eso es una propiedad de la ventana entregada, no una decisión de modelado.

### 3.2 La tabla de métricas

Cuatro evaluadores: dos modelos y dos líneas base. Cada umbral se eligió solo con la partición de entrenamiento —máximo F1 sobre una grilla de 0.001, empates al umbral menor— y se aplicó sin cambios a septiembre. Los modelos se ajustaron con `class_weight="balanced"` y semilla `20260928`.

| Evaluador | Rol | PR-AUC | Recall en su umbral | Brier | ROC-AUC | Umbral | Alertas |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `logistic` | modelo | 0.1801 | 0.3810 | 0.1754 | 0.7001 | 0.660 | 71 |
| `forest` | modelo | 0.1246 | 0.0000 | 0.0928 | 0.5453 | 0.611 | 0 |
| `climatology` | línea base | 0.1766 | 0.2143 | 0.0952 | 0.6739 | 0.330 | 40 |
| `region_frequency` | línea base | 0.1261 | 0.1905 | 0.0882 | 0.6363 | 0.049 | 56 |

La línea base `climatology` puntúa `p = 1 - exp(-r)`, donde `r` es el avance diario de `nro_inc_prom_quint`; `region_frequency` puntúa la tasa de positivos de cada región medida en el entrenamiento. Son líneas base de tipo climatología: no miran meteorología, solo la temporada y el historial regional.

### 3.3 La regla, y el par que se ve como victoria sin serlo

Superar una línea base exige **ambas** condiciones: mayor PR-AUC y menor Brier. Una PR-AUC más alta con un Brier peor es un intercambio, y llamarlo victoria es cómo se registra un modelo por la razón equivocada.

| Modelo | Contra | ¿La supera? | Por qué |
| --- | --- | --- | --- |
| `logistic` | `climatology` | no | Brier 0.1754 no baja de 0.0952 |
| `logistic` | `region_frequency` | no | Brier 0.1754 no baja de 0.0882 |
| `forest` | `climatology` | no | PR-AUC 0.1246 no supera 0.1766 |
| `forest` | `region_frequency` | no | PR-AUC 0.1246 no supera 0.1261; Brier 0.0928 no baja de 0.0882 |

La trampa está en el par de la logística con `region_frequency`: la logística **lidera** la PR-AUC —0.1801 contra 0.1261, una ventaja de 0.0540— y **pierde** el Brier por 0.0872. Es el caso que se presentaría como mejora porque una métrica está en verde y otra en rojo; la regla lo descarta porque exige las dos. Frente a `climatology`, la logística queda en 0.1801 contra 0.1766 de PR-AUC: el registro de la entrega califica esa diferencia —0.0035— como ruido con 42 positivos y señala que perder 0.080 de Brier no la compensa. El bosque, en cambio, no emite ninguna alerta en septiembre y su recall es 0.0000: no es un modelo que falle por poco, es un modelo que no responde.

### 3.4 Lo que la medición garantiza por construcción

Tres garantías dependen del código, no de la esperanza, y las tres se probaron:

- La etiqueta es el avance diario recortado del acumulado, y las 3 revisiones a la baja de `sup_afec_act` se cuentan y se listan en el anexo, no se absorben en silencio.
- Ninguna columna derivada del acumulado alcanza la matriz de características. El guard se midió: agregar `nro_inc_act` eleva la PR-AUC de septiembre de 0.0826 a cerca de 0.31, y agregar su diferencia diaria reproduce la etiqueta exactamente (1.00); un test fija ambos límites.
- La normalización no puede ver septiembre: la función recibe una sola matriz, y `center`, `scale` e `impute` se ajustaron solo con el entrenamiento, en ese orden.

El resultado no se puede maquillar con una columna prohibida, y por eso la medición sirve como evidencia y no solo como reporte.

## 4. Por qué no alcanza

El análisis archivo por archivo hace la pregunta anterior al modelo: qué contiene cada archivo y qué no puede sostener. Los tres archivos entregados son un contador de incendios por región y día (10 columnas, 1440 filas, cero columnas meteorológicas), un catálogo de 535 estaciones (metadatos, sin lecturas) y una fotografía instantánea de la red (5122 filas, 437 estaciones, un solo instante: 2026-09-28 09:00:00.000 -0300). Ninguno declara licencia, cita ni unidad.

### 4.1 La cascada, comprimida

Ocho propiedades que la entrada del modelo necesita, y qué archivo las responde.

| Propiedad que la entrada necesita | ¿La responde algún archivo? | Qué decide la respuesta |
| --- | --- | --- |
| Un valor meteorológico diario para un día-comuna | no | 0 columnas meteorológicas de 10 en el tablero; el catálogo no trae lecturas; la instantánea tiene 1 instante y a lo más 1 lectura por estación-día |
| Meteorología en días sin incendio | no | el tablero no tiene meteorología; la instantánea cubre un día y no es un día etiquetado |
| Un evento de incendio con fecha y comuna | no | 16 etiquetas de región, sin comuna y sin identidad de evento |
| Cobertura de estaciones por comuna | **sí** | 535 estaciones sobre 235 comunas, con 23 filas cuya etiqueta de región y CUT no coinciden; la instantánea lo confirma solo para ese instante |
| Una marca de calidad por lectura | no | 5 columnas vacías en toda la instantánea, incluidas las dos de calidad |
| Una unidad declarada y la elevación de la estación | no | ninguna unidad por variable; no hay columna de elevación, solo coordenadas |
| Una llave de comuna que una los archivos | no | solo el catálogo tiene CUT, y con 23 filas a recodificar; la instantánea se une por identificador de estación |
| Un lapso temporal suficiente para una partición temporal | no | 90 días (2026-07-01 a 2026-09-28), un solo tramo de temporada |

Solo la cobertura de estaciones queda respondida, y la instantánea puede confirmarla únicamente para un instante. Esa es la medición completa en una tabla: lo entregado es un **conteo** de incendios por región-día más un **catálogo** de estaciones, y la medición que convertiría cualquiera de los dos en entrada de modelo —clima diario en una comuna, en todos los días, con unidad y marca de calidad— no está en ninguno.

### 4.2 Las tres ausencias que más pesan

**No hay meteorología diaria en los días tranquilos.** Es la ausencia que decide el resultado. Sin días sin incendio, las variables existen solo donde la etiqueta vale 1, y el modelo aprende a describir incendios, no a anticiparlos. La instantánea agrava el problema: un solo timestamp no es un día, así que `TEMP_MAX`, `TEMP_MIN` y `PRECIPITACION` no pueden ser agregados diarios, digan lo que digan sus nombres.

**No hay un evento identificado por comuna.** La etiqueta es la diferencia diaria de un contador en una región, no un evento con causa, lugar y superficie. Por eso no se puede preguntar qué comuna quemó, ni comparar dos comunas de la misma región, ni unir el incendio con la estación que lo rodea.

**No hay unidad declarada.** Ninguna de las tres fuentes declara unidad por variable. El caso medido es la radiación: el máximo del artefacto, 1758.0 W/m2, es exactamente la lectura más alta del archivo, y 4 lecturas superan la constante solar de 1361 W/m2 en la estación 326, La Serena. Una cota copiada de los datos no puede fallar sobre los datos. La presión ilustra lo mismo sin ser un defecto: 593.03 hPa en la estación 145, Lago Chungará, es correcta para unos 4295 m, pero sin elevación no se compara con una estación al nivel del mar. La evapotranspiración lleva el caso al extremo: 290 de 292 lecturas quedan fuera de una cota diaria, con un máximo de 686.778, porque la columna no declara unidad ni período.

### 4.3 La razón de arquitectura, no de volumen

El registro de incendios de CONAF es un catálogo de eventos, así que solo trae meteorología **en los días con incendio**. No hay serie diaria y no hay días tranquilos. De las siete variables del artefacto, ese origen solo puede producir `humidity_rel`, y como media diaria, mientras `risk` envía la última lectura instantánea: mismo nombre y unidad, estimador distinto, registrado como divergencia en cada manifiesto.

La conclusión no depende del proveedor elegido: la meteorología no puede salir del registro de incendios. Tiene que venir de una fuente independiente unida por comuna y fecha —reanálisis o la red de estaciones— y el registro de CONAF aporta únicamente la etiqueta. La misma brecha aparece al conectar la inferencia: el camino de publicación de `risk` solo puede poblar 2 de las 7 variables del contrato —`humidity_rel` y `wind_speed`—; las otras cinco viajan como nulo explícito, se imputan y el factor queda registrado, nunca en cero silencioso.

### 4.4 La resolución también es un límite

Aun con meteorología perfecta, el tablero no distingue lugares: 16 etiquetas de región para regímenes muy distintos. En los 90 días entregados, Araucanía suma 13 días-región positivos y Metropolitana 10, mientras Atacama y Magallanes no avanzan el contador ni una vez. Una región es un promedio de comunas que no se parecen entre sí, y un modelo entrenado a esa escala aprende qué región acostumbra a quemar, no qué condición produce el incendio.

## 5. Qué haría falta

Los cinco pedidos, en el orden en que el análisis los prioriza. Cada uno se enuncia con lo que habilitaría, porque una lista de ausencias sin consecuencia es un reclamo, no un plan.

1. **Un registro de eventos llaveado por CUT de comuna y fecha local.** Una fila por incendio, no por región-día. Habilitaría: que la etiqueta deje de ser la primera diferencia de un contador y pase a ser un hecho con lugar y fecha, que es el requisito de todo lo demás.
2. **Meteorología diaria en todos los días, incluidos los tranquilos, en un punto de cada comuna.** Lecturas horarias agregadas a un día civil declarado, o un reanálisis ya agregado. Habilitaría: las siete variables que el contrato ya declara y, por tanto, una entrada que no sea un calendario.
3. **Unidad y período de agregación declarados por variable, y elevación por estación.** Habilitaría: validar la instantánea y hacer comparables la presión del altiplano y la de la costa, hoy en una misma columna sin elevación.
4. **Una marca de calidad por lectura, con conjunto de códigos documentado.** Dos columnas vacías no son una señal de calidad, son un esquema que nunca se llenó. Habilitaría: distinguir una lectura válida de una que el sensor no pudo entregar.
5. **El CUT posterior a 2018 en todas las filas, con la etiqueta de región consistente.** Habilitaría: la unión por comuna de la que dependen el panel y el producto; sin ella, 23 filas del catálogo ponen estaciones en la región equivocada.

El primer pedido no se sustituye con los otros cuatro, y ninguno de los tres archivos lo contiene. El producto de eventos que el registro de la entrega identifica está sujeto a solicitud de acceso: requiere cuenta, condiciones de uso y autorización, y una consulta anónima devuelve 403. Gestionar ese acceso es una tarea abierta, no un defecto del sistema.

## 6. Qué NO está verificado

Lo que sigue no está medido, y decirlo es parte de la entrega.

| Tema | Estado | Consecuencia práctica |
| --- | --- | --- |
| Trazado distribuido (OpenTelemetry) | **No implementado** | No se puede seguir una solicitud entre servicios. El frente 5 lo registra como pendiente y el plan de cierre lo excluye; existen métricas de Prometheus, paneles de Grafana y alertas |
| 17 pruebas de Go con PostgreSQL | **Nunca corrieron** en el host de desarrollo | La migración 011 y las rutas que dependen de la base no tienen evidencia local; las ejecuta la CI con los perfiles de prueba |
| `Caddyfile` | **Nunca se ejecutó**: no hay binario `caddy` ni daemon de Docker en el host | Su sintaxis solo se probará en el primer `up` real; el Compose sí se validó con `docker compose config` |
| `go test -race` | **No puede correr** en ese host: falla con «-race requires cgo», porque `CGO_ENABLED=0` y no hay compilador de C | La detección de carreras queda cubierta únicamente por el runner Linux de CI |
| Artefacto del repositorio | **Es un ensayo, no un modelo en uso** | No debe registrarse ni activarse; no es servible por `risk` |

**El artefacto, con precisión.** Se llama `delivered-data-rehearsal-v1`, es de tipo `logistic` y su sha256 es `33970a895cd4ea280e5b116bdf4477204f42a38d40efe06c89d8a1bba65575e2`. No es servible por `risk` porque su conjunto de características incluye un `one-hot` de región, y `risk` construye su vector desde una publicación que no lleva identidad de región: todos los indicadores llegarían vacíos. La marca de ensayo vive en los bytes del propio artefacto —la palabra `rehearsal` en `version`, y las claves `metrics.rehearsal`, `metrics.servable_by_risk`, `metrics.not_servable_reason` y `metrics.must_not_be_registered`—, no solo en la documentación. No se puede añadir una clave de nivel superior porque el lector Go decodifica con `DisallowUnknownFields`. El artefacto existe para probar el exportador contra el lector Go: el documento emitido se relee con una transcripción en Python de `artifact.Validate` y se rechaza si falla una regla.

**Lo que sí quedó verificado, para no confundir las listas.** `govulncheck` en `risk` e `inference`, con la versión que fija la CI (v1.9.0) y con `@latest`, responde «No vulnerabilities found» en ambos casos. `python -m pytest -q` da 306 pruebas en verde y `python -m ruff check .` responde «All checks passed!».

**Una deuda de la pila.** El servicio de inferencia todavía no figura en `docker-compose.prod.yml`, como se indicó en la sección 2.

## 7. Cómo reproducirlo

Los comandos de la medición son los de la sección «How to reproduce» del anexo de medición; el del análisis es el que ese anexo indica para regenerarlo. Se ejecutan desde la raíz del repositorio, salvo donde se indica.

```bash
# La corrida real: el export tal como se entregó, desde la raíz del repositorio.
python backend/training/train_delivered.py

# Sin red, contra el fixture comprometido (es lo que usan las pruebas).
python backend/training/train_delivered.py \
    --source backend/training/tests/fixtures/dashboard-conaf-sample.csv \
    --out backend/training/out/fixture

# Pruebas y lint, desde backend/training.
python -m pytest -q
python -m ruff check .
```

El análisis archivo por archivo se regenera con un solo comando, también desde la raíz:

```bash
python backend/training/analyze_delivered.py
```

El primer comando produce la partición, la tabla de métricas, la comparación par por par y el artefacto de ensayo. Las cifras se verifican comparando esa salida con los anexos; si un número cambia, cambia en el anexo y este informe lo cita de nuevo.
