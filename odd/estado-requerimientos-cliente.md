# Estado de los requerimientos del cliente — corte del 2026-10-20

**Propósito:** reunir en un solo lugar qué pidió el cliente, cuánto de eso tiene hoy un artefacto en el repositorio y qué falta exactamente. Fuente autoritativa del pedido: [tareas_verificacion_implementacion_IA.md](../Referencias/tareas_verificacion_implementacion_IA.md), **132 IDs** (72 `I` y 60 `V`).
**Base de código:** `267a4bb`, rama `docs/code-alignment-and-client-gaps`, fecha del corte **2026-10-20**.
**Regla del documento:** un estado **no** es una casilla marcada. Este corte no cierra ningún ID y no marca ninguna casilla del checklist.

## Cómo se lee el corte

Los 132 IDs ya tienen cada uno **una** fila con su estado y su razón de bloqueo en los cuatro registros (`backend/docs/checklist-evidence-u1.md`, `u2-u3.md`, `u4.md`, `u5.md`). Este documento **no repite esas filas**: agrega los estados, nombra el artefacto que falta y apunta a la fila que guarda la razón. Los significados son los que los propios registros declaran.

| Estado | Qué significa en este corte | Cuántos IDs |
|---|---|---|
| **Parcial** (`Partial`) | Existe un artefacto ejecutable en el repositorio que habla del ID. No es su criterio de cierre. | 60 |
| **Bloqueado** (`Blocked`) | El cierre depende de un insumo que el repositorio no contiene: dato real, umbral acordado, responsable nombrado, corrida de carga o clúster/toolchain autorizado. | 66 |
| **Pendiente** (`Pending`) | Ítem de trabajo definido y autocontenido, no iniciado, que no espera un insumo externo. | 5 |
| **Propuesto** (`Proposed`) | Hay una respuesta escrita (un N/A justificado) que ningún responsable ha aceptado. | 1 |
| **Implementado / cerrado** | Nadie cerró un criterio. | 0 |

**Para cerrar un ID** hacen falta cuatro cosas, y ninguna es un archivo que ya exista: evidencia reproducible, umbrales acordados con el equipo, un responsable nombrado y, cuando el criterio lo dice, datos reales. Una prueba en PASS observa un comportamiento acotado y ejecutable hoy; no cierra un requisito. Por eso los cinco IDs de aprobación `GNG-V1`…`GNG-V5` siguen literalmente sin marcar.

## Agregado

Los números de abajo se calcularon contando las filas de los cuatro registros con el siguiente comando, ejecutado desde la raíz del repositorio. No se copió ninguna cifra de un resumen previo.

```bash
grep -hoE '^\| `[A-Z]+-[A-Z]+[0-9]+` \| [A-Za-z]+' backend/docs/checklist-evidence-u*.md \
  | awk '{print $NF}' | sort | uniq -c
```

Salida observada: `66 Blocked`, `60 Partial`, `5 Pending`, `1 Proposed`. Las 132 filas son 132 IDs distintos: el mismo patrón aplicado a [la fuente](../Referencias/tareas_verificacion_implementacion_IA.md) con `grep -ohE '\b[A-Z]{2,3}-[IV][0-9]+\b' … | sort -u | wc -l` devuelve `132`, y el mismo filtro por `-I` y por `-V` devuelve `72` y `60`. No hay ID de la fuente sin fila ni fila sin ID de la fuente.

**Total por estado**

| Estado | IDs |
|---|---:|
| Parcial | 60 |
| Bloqueado | 66 |
| Pendiente | 5 |
| Propuesto | 1 |
| Cerrado | 0 |
| **Total** | **132** |

**Total por unidad** (misma medición, agrupada por registro y por prefijo de ID)

| Unidad | Registro | IDs | Parcial | Bloqueado | Pendiente | Propuesto |
|---|---|---:|---:|---:|---:|---:|
| U1 — Gobierno, seguridad y protección de datos/modelo | [checklist-evidence-u1.md](../backend/docs/checklist-evidence-u1.md) | 41 | 14 | 23 | 4 | 0 |
| U2 — Calidad, sesgo y fugas de datos | [checklist-evidence-u2-u3.md](../backend/docs/checklist-evidence-u2-u3.md) | 18 | 8 | 9 | 0 | 1 |
| U3 — Evaluación y robustez del modelo | [checklist-evidence-u2-u3.md](../backend/docs/checklist-evidence-u2-u3.md) | 11 | 5 | 6 | 0 | 0 |
| U4 — MLOps, rendimiento, optimización y monitoreo | [checklist-evidence-u4.md](../backend/docs/checklist-evidence-u4.md) | 42 | 23 | 19 | 0 | 0 |
| U5 — Stack, Continuous Testing, Go/No-Go y práctica | [checklist-evidence-u5.md](../backend/docs/checklist-evidence-u5.md) | 20 | 10 | 9 | 1 | 0 |
| **Total** | | **132** | **60** | **66** | **5** | **1** |

U2 y U3 comparten un solo registro; la separación 18/11 se obtiene con el mismo patrón restringido a los prefijos `DAT|SES|LEA` y `MET|ROB|REG`, y coincide con la asignación de [las unidades ODD](urgent/README.md). El registro de U4 declara 42 IDs; dos de sus subsecciones (`OBS-I1`, `OBS-V1`) se registran en el archivo de U1 y no se cuentan dos veces.

## Qué falta, agrupado por los dominios del cliente

El agrupamiento sigue la estructura de la fuente: Parte A (secciones 1–3, seguridad), sección 4 (datos), sección 5 (modelo), secciones 6–8 (operación y monitoreo) y secciones 9–12 (stack, Continuous Testing y Go/No-Go).

### 1. Gobierno y seguridad (secciones 1–3 de la fuente · 41 IDs · [registro U1](../backend/docs/checklist-evidence-u1.md))

| Artefacto que falta | IDs que dependen | Dónde está la razón |
|---|---|---|
| Política interna de clasificación de riesgo de IA (escala bajo/limitado/alto/inaceptable), la categoría del sistema, el nombre del AI Security Officer y el procedimiento de aprobación previa a producción con quién firma | `GOB-I1`, `GOB-I2`, `GOB-I3`, `GOB-I4`, `GOB-V1`, `GOB-V2` | [u1:55-60](../backend/docs/checklist-evidence-u1.md) |
| Ejercicio de modelado de amenazas MITRE ATLAS, registro de vectores con su mitigación y el N/A firmado por un responsable para los vectores que no aplican | `AME-I1`, `AME-I2`, `AME-V1`, `AME-V2` | [u1:66-69](../backend/docs/checklist-evidence-u1.md) |
| Autenticación de Prometheus, identidades de broker por servicio, revisión de accesos que resuelva la credencial compartida y auditoría periódica de permisos con responsable | `ZT-I2`, `ZT-I3`, `ZT-V1`, `ZT-V2` | [u1:76-79](../backend/docs/checklist-evidence-u1.md) |
| Linaje e integridad criptográfica para un **dataset real de entrada de riesgo** (hoy solo hay linaje del activo geográfico), inventario de dependencias con dueños y un escaneo fresco aceptado | `PRV-I1`, `PRV-I2`, `PRV-I3`, `PRV-V1`, `PRV-V2` | [u1:85-89](../backend/docs/checklist-evidence-u1.md) |
| Determinación escrita de sensibilidad del dato real, mecanismo de anonimización o privacidad diferencial, evaluación de cifrado homomórfico/SMPC y análisis de reidentificación | `PRI-I1`, `PRI-I2`, `PRI-I3`, `PRI-V1` | [u1:95-98](../backend/docs/checklist-evidence-u1.md) |
| Pipeline de entrenamiento, dataset etiquetado y el registro de ejemplos ruidosos o engañosos añadidos al entrenamiento | `ENT-I1`, `ENT-I2`, `ENT-V1` | [u1:104-106](../backend/docs/checklist-evidence-u1.md) |
| Capa anti-contenido malicioso (el caso de inyección de prompts del criterio), rangos productivos acordados y una revisión explícita del contenido de salida | `GRD-I1`, `GRD-I2`, `GRD-V1`, `GRD-V2` | [u1:112-115](../backend/docs/checklist-evidence-u1.md) |
| Egress acotado (la política `dns-and-oidc` permite hoy TCP 443 a cualquier IPv4) y un entorno autorizado donde demostrar que el contenedor no alcanza hosts no autorizados | `SBX-I1`, `SBX-I2`, `SBX-V1` | [u1:121-123](../backend/docs/checklist-evidence-u1.md) |
| Métrica de distribución de entrada o confianza del modelo, y los umbrales que hacen que una alerta de desviación signifique algo | `OBS-I1`, `OBS-V1` | [u1:129-130](../backend/docs/checklist-evidence-u1.md) |
| Calendario de red teaming, plan de respuesta a incidentes, despliegue de artefactos de modelo versionados con rollback implementado y un simulacro con duración medida | `RED-I1`, `RED-I2`, `RED-I3`, `RED-V1`, `RED-V2` | [u1:136-140](../backend/docs/checklist-evidence-u1.md) |

### 2. Datos (sección 4 · 18 IDs · [registro U2](../backend/docs/checklist-evidence-u2-u3.md))

| Artefacto que falta | IDs que dependen | Dónde está la razón |
|---|---|---|
| Inventario de fuentes de observación **de producción** y el nombre de quién lo mantiene: el servido hoy es un fixture sintético más un activo geográfico real | `DAT-I1` | [u2-u3:35](../backend/docs/checklist-evidence-u2-u3.md) |
| Reglas de validación y rangos físicamente válidos acordados para entradas reales, y el reporte que las contrasta con el dataset real | `DAT-I2`, `DAT-V1`, `DAT-V2`, `DAT-V4` | [u2-u3:36](../backend/docs/checklist-evidence-u2-u3.md), [:38](../backend/docs/checklist-evidence-u2-u3.md), [:39](../backend/docs/checklist-evidence-u2-u3.md), [:41](../backend/docs/checklist-evidence-u2-u3.md) |
| Política de nulos de las fuentes reales y la lista de campos críticos no anulables | `DAT-V3` | [u2-u3:40](../backend/docs/checklist-evidence-u2-u3.md) |
| Ruta de ingesta real y, con ella, la acción acordada por clase de fallo (rechazar, alertar o imputar) y a quién se alerta | `DAT-I3`, `DAT-V5` | [u2-u3:37](../backend/docs/checklist-evidence-u2-u3.md), [:42](../backend/docs/checklist-evidence-u2-u3.md) |
| Grupos a proteger o vigilar, métrica de paridad y su umbral acordado, estrategia de mitigación, reporte de cobertura por zona y por clase, y la aceptación del N/A de fairness | `SES-I1`, `SES-I2`, `SES-I3`, `SES-V1`, `SES-V2`, `SES-V3` | [u2-u3:48-53](../backend/docs/checklist-evidence-u2-u3.md) |
| Dataset etiquetado con separación temporal o geográfica justificada, inventario completo de variables *giveaway* y auditoría de fuga sobre una corrida de entrenamiento | `LEA-I1`, `LEA-I2`, `LEA-V1`, `LEA-V2` | [u2-u3:59-62](../backend/docs/checklist-evidence-u2-u3.md) |

### 3. Modelo (secciones 2.3 y 5 · 11 IDs · [registro U3](../backend/docs/checklist-evidence-u2-u3.md))

| Artefacto que falta | IDs que dependen | Dónde está la razón |
|---|---|---|
| Relación direccional documentada para un modelo entrenado (la actual es estructural de una regla congelada y no se hereda) y los resultados registrados del protocolo de evaluación real | `MET-I1`, `MET-I2`, `MET-V1`, `MET-V2` | [u2-u3:68-71](../backend/docs/checklist-evidence-u2-u3.md) |
| Escenario de ruido gaussiano y, sobre todo, una distribución real de entrada que perturbar | `ROB-I1` | [u2-u3:77](../backend/docs/checklist-evidence-u2-u3.md) |
| Tolerancia `X` acordada por el equipo, la métrica que acota, una línea base medida y la definición acordada de «fallo catastrófico» | `ROB-I2`, `ROB-V1`, `ROB-V2` | [u2-u3:78-80](../backend/docs/checklist-evidence-u2-u3.md) |
| Modelo candidato con Accuracy, F1 y Recall medidos y su mínimo aceptable, la corrida de reentrenamiento que produce el challenger y el responsable que acepta o rechaza el reemplazo | `REG-I1`, `REG-V1`, `REG-V2` | [u2-u3:86-88](../backend/docs/checklist-evidence-u2-u3.md) |

### 4. Operación y monitoreo (secciones 6–8 · 42 IDs · [registro U4](../backend/docs/checklist-evidence-u4.md))

| Artefacto que falta | IDs que dependen | Dónde está la razón |
|---|---|---|
| Una corrida de carga ejecutada con sus cifras de p95/p99, RPS y errores. El escenario está escrito y el umbral declarado (`p(95)<500`, `max<2000`, `http_req_failed<0.01`); la ejecución no existe | `SLA-I3`, `SLA-V1`, `SLA-V2`, `SLA-V3`, `MET-V3`, `MET-V4` | [u4:46](../backend/docs/checklist-evidence-u4.md), [:48-50](../backend/docs/checklist-evidence-u4.md), [:111-112](../backend/docs/checklist-evidence-u4.md) |
| Aceptación por un responsable de los objetivos hoy declarados como objetivo y no como medición (p95 < 500 ms, 4 RPS, RTO ≤ 15 min, umbrales de alerta) | `SLA-I1`, `SLA-I2`, `SLA-I4`, `ALR-I1` | [u4:44-47](../backend/docs/checklist-evidence-u4.md), [:119](../backend/docs/checklist-evidence-u4.md) |
| Prueba de escalado en un clúster real y la política de autoescalado aceptada por un responsable | `ESC-I1`, `ESC-V1`, `ORQ-I3`, `ORQ-V2` | [u4:70](../backend/docs/checklist-evidence-u4.md), [:72](../backend/docs/checklist-evidence-u4.md), [:92](../backend/docs/checklist-evidence-u4.md), [:94](../backend/docs/checklist-evidence-u4.md) |
| Simulacro de recuperación con RTO medido | `ESC-I2`, `ESC-V2` | [u4:71](../backend/docs/checklist-evidence-u4.md), [:73](../backend/docs/checklist-evidence-u4.md) |
| Evaluación de drift ejecutada con datos de referencia, la prueba estadística elegida con su umbral, distribuciones reales y ground truth para el drift de concepto | `DRF-I1`, `DRF-I2`, `DRF-I3`, `DRF-V1`, `DRF-V2` | [u4:59-64](../backend/docs/checklist-evidence-u4.md) |
| Fuente de ground truth, su esquema y un responsable: sin resultado observado no hay re-evaluación ni detección de cambio | `LOG-I3`, `LOG-V1`, `ALR-V1`, `ALR-V2` | [u4:102-103](../backend/docs/checklist-evidence-u4.md), [:121-122](../backend/docs/checklist-evidence-u4.md) |
| Perfilado de RAM, almacenamiento y red, y la decisión de propósito de optimización que un responsable firma | `OPT-I1`, `OPT-V1`, `OPT-V2`, `OPT-V3` | [u4:79](../backend/docs/checklist-evidence-u4.md), [:82-84](../backend/docs/checklist-evidence-u4.md) |
| Evaluación de beneficios y costos de la orquestación con cifras, prioridad de métricas aceptada, dueño del dashboard y ruta de escalamiento de alertas | `ORQ-V1`, `MET-I4`, `MET-V5`, `ALR-I2`, `ALR-V3` | [u4:93](../backend/docs/checklist-evidence-u4.md), [:110](../backend/docs/checklist-evidence-u4.md), [:113](../backend/docs/checklist-evidence-u4.md), [:120](../backend/docs/checklist-evidence-u4.md), [:123](../backend/docs/checklist-evidence-u4.md) |

### 5. Stack, CI y Go/No-Go (secciones 9–12 · 20 IDs · [registro U5](../backend/docs/checklist-evidence-u5.md))

| Artefacto que falta | IDs que dependen | Dónde está la razón |
|---|---|---|
| Tabla de herramientas por área con su justificación real, evaluación de la arquitectura contra los cinco pilares AWS Well-Architected ML Lens y el mapa herramienta → pruebas de las secciones 4–6 | `STK-I1`, `STK-I2`, `STK-V1` | [u5:46-48](../backend/docs/checklist-evidence-u5.md) |
| Etapa de validación de datos en CI, etapa de benchmark de modelo y etapa de carga con umbral acordado | `CT-I2`, `CT-I3`, `CT-I4` | [u5:79-81](../backend/docs/checklist-evidence-u5.md) |
| Protección de rama y *required status checks* reales en el host, y una ruta de promoción a producción que funcione como compuerta | `CT-I1`, `CT-V1` | [u5:78](../backend/docs/checklist-evidence-u5.md), [:82](../backend/docs/checklist-evidence-u5.md) |
| Informe de validación con datos reales, informe de evaluación de modelo con métricas, resultados de infraestructura/SLA aprobados, decisión de despliegue y el documento Go/No-Go con su historial de versiones | `GNG-I1`, `GNG-I2`, `GNG-V1`, `GNG-V2`, `GNG-V3`, `GNG-V4` | [u5:92-97](../backend/docs/checklist-evidence-u5.md) |
| El nombre y la firma del responsable del Go/No-Go. Ningún documento lo sustituye | `GNG-V5` | [u5:98](../backend/docs/checklist-evidence-u5.md) |
| Lectura bajo carga controlada (RPS, p95, memoria, errores, distribución de riesgo) y la decisión escrita que se desprende de ella | `ACT-I3`, `ACT-V1`, `ACT-V2` | [u5:106-108](../backend/docs/checklist-evidence-u5.md) |

## Ausente por completo

Cuatro artefactos no existen en ninguna parte del árbol. Se verificó con búsqueda, no con memoria de un documento anterior.

| Artefacto | Prueba de ausencia | Declaración que lo confirma |
|---|---|---|
| Trazas distribuidas con OpenTelemetry | `grep -rl "opentelemetry" backend/` no devuelve nada, y los cinco `go.mod` (`contracts/gen`, `services/gateway`, `jobs`, `prevention`, `risk`) no lo declaran. No hay span ni exportador OTLP en `backend/`. Lo único que existe es un identificador de correlación `trace_id` en [events.go:27](../backend/contracts/gen/events/v1/events.go), propagado desde `X-Request-ID`; un id de correlación no es una traza. | [backend/README.md:126](../backend/README.md) lista «distributed tracing» como trabajo siguiente |
| Firma de imágenes y promoción de registry | `grep -rl cosign backend/ .github/` y `grep -rl sigstore backend/ .github/` no devuelven nada (exit 1 en ambos). La CI produce SBOM y atestación de provenance, pero no firma ni promueve. | [architecture.md:57](../backend/docs/architecture.md) declara firma, promoción y rollout automatizado como trabajo de despliegue posterior; el frente está planificado en [07-firma-registry-rollout.md](plan-produccion/07-firma-registry-rollout.md) |
| Un modelo entrenado (y su pipeline de entrenamiento) | No hay pipeline, ni dataset etiquetado, ni artefacto de modelo. El clasificador activo es una regla determinista congelada por hash, `academic-rule-v1`. | [architecture.md:63](../backend/docs/architecture.md) sitúa el modelo real y la aprobación de producción en entregas posteriores |
| Ground truth (resultado observado de un incendio) | No hay fuente, ni esquema, ni una sola fila de resultado observado. | [synthetic-scenarios.md:47](../backend/docs/synthetic-scenarios.md) enumera las afirmaciones que el repositorio no hace, entre ellas el benchmark de modelo y la medición de drift |

## Presente y hoy mal documentado

Una tabla de la bandeja de planificación ([odd/urgent/README.md](urgent/README.md), fechada el 2026-10-14) afirma que faltan cosas que el código ya tiene. Estos son los puntos concretos, con la corrección y su anclaje.

| Lo que decía el documento anterior | Lo que hay hoy en el código | Anclaje |
|---|---|---|
| El servicio `prevention` falta | Existe como módulo y proceso propio (puerto 8084, esquema `prevention`, migración `009`), con su vista de navegador | [services/prevention/cmd/server/main.go](../backend/services/prevention/cmd/server/main.go), [workloads.yaml:134](../backend/deploy/k8s/base/workloads.yaml), [architecture.md:34](../backend/docs/architecture.md) |
| Grafana no existe | Grafana 11.5.2 es un servicio del perfil `observability`, con acceso anónimo deshabilitado y contraseña de administrador obligatoria; el datasource y el dashboard están versionados como archivos | [docker-compose.yml:187](../backend/docker-compose.yml), [firewatch-overview.json](../backend/deploy/local/grafana/dashboards/firewatch-overview.json) |
| Faltan HPA y autoescalador | Cuatro `HorizontalPodAutoscaler` (uno por servicio, 2→6 réplicas, CPU 70 %) | [autoscaling.yaml:12](../backend/deploy/k8s/base/autoscaling.yaml) — `gateway` en `:14`, `jobs` en `:38`, `risk` en `:62`, `prevention` en `:86` |
| Falta la herramienta de carga | El escenario k6 está escrito con los umbrales acordados (`p(95)<500`, `max<2000`, `http_req_failed<0.01`). Lo que falta es **ejecutarlo**, no la herramienta | [k6-risk.js](../backend/deploy/load/k6-risk.js), [operations.md:86-88](../backend/docs/operations.md) |
| El contador `high_risk_predictions_total` no existe | Se emite como `firewatch_high_risk_predictions_total` y tiene prueba y panel de dashboard | [metrics.go:56](../backend/services/risk/internal/metrics/metrics.go) (nombre exigido, comentado en `:51`), [metrics_test.go:24](../backend/services/risk/internal/metrics/metrics_test.go), [firewatch-overview.json:190](../backend/deploy/local/grafana/dashboards/firewatch-overview.json) |
| Faltan el ciclo de 15 minutos y el replay de DLQ | El ciclo corre al arranque y cada 15 minutos con frescura por fuente; el replay es una ruta autenticada con token de confirmación | [backend/README.md:38](../backend/README.md), [architecture.md:63](../backend/docs/architecture.md), [api.go:187](../backend/services/jobs/internal/handler/api.go) |
| «U4: 10 parciales, 32 bloqueados» | El registro de U4 declara 42 IDs: 23 con evidencia parcial y 19 bloqueados | [checklist-evidence-u4.md](../backend/docs/checklist-evidence-u4.md) |

La misma tabla sigue siendo correcta en lo que importa para el cliente: no hay ingesta real, no hay modelo entrenado, no hay medición de carga/SLA ni de drift con ground truth, y el Go/No-Go no está firmado.

## Límites

- **No hay mediciones.** No existe corrida de carga, ni p95/p99/RPS medidos, ni RTO medido, ni disponibilidad observada. Los objetivos de latencia, throughput y recuperación están escritos como objetivo, no como resultado.
- **No hay drift con ground truth.** No hay fuente de resultado observado, así que ni el drift de datos contra producción ni el drift de concepto son medibles hoy.
- **No hay exactitud de modelo.** Ningún Accuracy, F1 ni Recall existe en el repositorio: el clasificador activo es una regla académica no calibrada contra incendios observados.
- **Solo datos sintéticos.** Los seis perfiles sintéticos y el fixture prueban comportamiento del código sobre entradas inventadas; no prueban calidad de dato real, representatividad geográfica ni sesgo.
- **No hay despliegue productivo.** Los manifiestos de Kubernetes son una base de staging con imágenes `replace-before-deploy`; no hay rollout ejecutado ni entorno autorizado.
- **Este documento no cierra ningún ID.** Es un corte de lectura: agrega estados ya registrados, nombra artefactos ausentes y apunta a las filas que guardan cada razón. Cerrar un ID sigue exigiendo evidencia reproducible, umbrales acordados, responsable nombrado y, donde el criterio lo pide, datos reales.
- **Ningún anclaje es una medición.** Todos los `archivo:línea` de este documento son lecturas de archivo, verificadas con búsquedas en el árbol. No se ejecutó prueba, corrida de carga, clúster ni despliegue: el verificador de enlaces relativos del repositorio informa `BROKEN: 0` y es lo único que corrió sobre este corte.
