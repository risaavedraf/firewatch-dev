# FireWatch — guion único para la reunión de hoy

**Fecha:** 2026-10-20 · **Base:** `main`, con la entrega documental del 2026-10-20 (`ef686c0` más los PR #44–#49 y #51).
**Una regla antes de empezar:** este documento no cierra requisitos. Ningún ID está cerrado y ninguna casilla está marcada. Todo lo que dice «existe» se leyó o se buscó en el árbol; no se levantó el stack ni se ejecutó una prueba funcional para escribirlo.

---

## 1. Qué es FireWatch

FireWatch es un sistema que mira el clima y el terreno de una comuna y entrega un nivel de riesgo de incendio: bajo, medio, alto o desconocido.
Está construido como una aplicación real: cuatro servicios, una base de datos con mapas, un monitor de salud y una página web.
Hoy funciona de punta a punta **con observaciones inventadas**: el mapa usa geometría real de las comunas, pero el clima y el terreno salen de datos sintéticos. No usa observaciones reales de incendios y no tiene un modelo entrenado.

---

## 2. Lo que ya funciona

Todo esto existe en el repositorio y lo abrí línea por línea para comprobarlo.
Que exista **no** significa que esté medido bajo carga. Eso está en la sección 3.

| Qué hace, en simple | Dónde está la prueba |
|---|---|
| Se abastece solo cada 15 minutos y **muestra** la frescura de cada fuente (no dispara una alerta si una se atrasa); un administrador puede reenviar un trabajo que falló y debe confirmarlo si sigue en cola | `backend/README.md:38` · `backend/services/risk/internal/cycle/runner.go:25` · `backend/services/jobs/internal/handler/security.go:43` · `backend/services/jobs/internal/handler/api.go:187`, `:203-207` |
| Tiene un servicio aparte para las acciones preventivas, con su propia parte de la base de datos | `backend/docs/architecture.md:34` · `backend/deploy/k8s/base/workloads.yaml:134` |
| Tiene un tablero de monitoreo local: vigila cinco fuentes, tiene siete reglas que evalúan en Prometheus —sin Alertmanager, así que no las envía a nadie— y un panel gráfico guardado como archivo | `backend/deploy/local/prometheus.yml:8-32` · `backend/deploy/local/alerts.yml:12-104` · `backend/docker-compose.yml:187-199` · `backend/deploy/local/grafana/dashboards/firewatch-overview.json:1` |
| Cuenta cuántas predicciones entrega de cada nivel y cuántas son de riesgo alto | `backend/services/risk/internal/metrics/metrics.go:56-60` |
| Los cuatro servicios miden cuánto tardan, cuántos errores dan y cuánta memoria usan | `backend/services/gateway/internal/metrics/http.go:73-74` · `backend/services/prevention/internal/metrics/http.go:72-73` · `backend/services/jobs/cmd/server/main.go:146` · `backend/services/risk/cmd/server/main.go:229-233` |
| El despliegue está escrito para crecer solo: de 2 a 6 copias por servicio cuando la CPU llega al 70 % | `backend/deploy/k8s/base/autoscaling.yaml:14`, `:20-21`, `:33` (y `:38`, `:62`, `:86` para los otros tres servicios) |
| La prueba de carga está escrita con los límites ya acordados (4 peticiones por segundo, 95 % bajo 500 ms) | `backend/deploy/load/k6-risk.js:29` · `:36-40` |
| Hay seis escenarios inventados, cada uno con su resultado esperado, y se eligen desde la pantalla | `backend/docs/synthetic-scenarios.md:17-22` · `frontend/src/views/JobsView.tsx:400-410` |

**Y por debajo de todo eso:** la verificación automática revisa cada servicio (formato, pruebas, vulnerabilidades, escaneo de la imagen) y validó el contrato de la API y los archivos de despliegue. Está configurada y corrió en verde según el registro del repositorio: `.github/workflows/_go-service.yml:27-36` (formato, módulos, pruebas, vet y `govulncheck`) · `:49-63` (imagen y escaneo con Trivy) · `.github/workflows/integration.yml:26-37` · `backend/docs/architecture.md:57`.

---

## 3. Lo que falta: tres cajas distintas

**La idea que ordena la reunión:** de los 66 requisitos bloqueados, **27 mencionan a un responsable** que debe decidir o firmar, pero casi todos necesitan además datos reales, un modelo o una corrida. Solo un puñado —`GOB-I3`, `SES-I1`, `OPT-I1`— depende únicamente de una decisión. Los nombres y los umbrales se acuerdan en una reunión; los datos, el modelo y las mediciones, no. Las tres cajas de abajo agrupan por tipo de insumo y se solapan: no son un reparto exacto de los 66.

### Caja 1 — Falta hacer (hay que construir algo)

| Qué falta, en simple | Por qué falta | ID que lo bloquea |
|---|---|---|
| Conectar una fuente real de observaciones | El adaptador HTTP/JSON existe y se configura con `FIREWATCH_SOURCE_URL`, pero no hay endpoint real, credenciales ni datos: por defecto sirve un fixture sintético | `DAT-I1`, `DAT-I2` |
| Un modelo entrenado con su exactitud, su precisión y su recall medidos | No se construyó: lo que clasifica hoy es una regla fija escrita a mano | `REG-I1`, `MET-I1` |
| Guardar el resultado real de un incendio (qué pasó después) | No hay ni la fuente ni el formato | `LOG-I3`, `ALR-V1` |
| Un conjunto de datos etiquetado para entrenar y evaluar | No existe el conjunto | `LEA-I1` |
| Firmar las imágenes y publicarlas en un registro oficial | No está implementado; la verificación automática produce el inventario, no la firma | `CT-V1` |
| Trazas distribuidas para seguir una petición entre servicios | No está implementado; hoy solo hay un identificador de correlación. La lista del mandante no tiene un ID para esto | — (`backend/docs/architecture.md:55`) |
| Correr la prueba de carga y anotar los números | El escenario está escrito; la corrida no existe | `SLA-V1`, `SLA-V2` |
| Probar el crecimiento automático en un servidor de verdad | El archivo está escrito; nadie lo aplicó en un servidor | `ESC-V1` |

**Cuatro cosas que el sistema no tiene en ninguna forma:** trazas OpenTelemetry, firma de imágenes y promoción a un registro, un modelo entrenado, y el resultado observado de un incendio real. Conviene decirlas antes de que las pregunten.

### Caja 2 — Falta decidir (alguien acuerda, firma o da permiso)

| Qué hay que decidir | Quién lo decide | ID que desbloquea |
|---|---|---|
| Aceptar los objetivos de velocidad y de carga como compromiso del equipo | Un responsable con nombre | `SLA-I1`, `SLA-I2` |
| Aceptar cuánto puede tardar una recuperación y cuándo debe sonar una alerta | Un responsable con nombre | `ESC-I2`, `ALR-I1` |
| Nombrar al dueño del tablero y documentar la respuesta de cada alerta | Un responsable con nombre | `MET-I4`, `ALR-I2` |
| Nombrar formalmente al oficial de seguridad (la política la podemos redactar nosotros) | La organización | `GOB-I3` |
| Firmar el Go/No-Go (la autorización para producción) | Un responsable con nombre | `GNG-V5` |
| Acordar cuánto ruido aguanta el sistema antes de considerarlo un fallo grave | Un responsable con nombre | `ROB-I2`, `ROB-V1` |
| Acordar qué rangos de datos son válidos para el mundo real | Un responsable con nombre | `DAT-I2`, `DAT-V4` |
| Autorizar un servidor o entorno aislado para medir | Permiso de la organización | `SBX-V1` |

### Caja 3 — Lo que podemos cerrar sin ustedes

Cinco requisitos son trabajo propio, ya definido y sin insumo externo: `GOB-I1` (redactar la política), `AME-I1` y `AME-I2` (modelado de amenazas MITRE ATLAS y sus mitigaciones), `RED-I3` (plan de respuesta a incidentes) y `GNG-I1` (historial de versiones del documento). Ninguno cierra un criterio por sí solo, pero ninguno espera una reunión. `SES-V3` es distinto: hay un «no aplica» escrito que solo espera que un responsable lo acepte.

### En qué orden conviene desbloquear

1. **Cerrar los cinco documentos que no dependen de nadie** → mueve `GOB-I1`, `AME-I1`, `AME-I2`, `RED-I3` y `GNG-I1` sin costo externo.
2. **Acuerdo escrito de umbrales y responsables** → nombra al dueño y fija el número de 27 requisitos bloqueados; el resto sigue esperando datos, modelo o corrida.
3. **Datos reales y resultado observado** → libera `DAT-*`, `DRF-*`, `SES-*`, `LEA-*`, `LOG-*`.
4. **Modelo entrenado, versionado y con vuelta atrás** → libera `MET-*`, `REG-*`, `RED-I2`.
5. **Corrida de carga y un servidor real** → libera `SLA-*`, `ESC-*`, `ORQ-*`.
6. **Firma de imágenes y promoción** → libera `CT-*`.

---

## 4. Los 132 requisitos, como una lista de deberes

**Qué son.** El mandante entregó una lista de 132 requisitos: 72 de implementación y 60 de verificación. Cada uno tiene un código, por ejemplo `GOB-I1` o `DAT-V2`.

**Dónde están.**

- El pedido original, con el criterio de cada uno: `Referencias/tareas_verificacion_implementacion_IA.md`.
- Una fila por requisito, con su estado, lo que existe y lo que falta: `backend/docs/checklist-evidence-u1.md` (41), `u2-u3.md` (29), `u4.md` (42), `u5.md` (20).
- El resumen: `odd/estado-requerimientos-cliente.md`.
- Los mismos requisitos agrupados en cinco unidades de trabajo: `odd/urgent/01-…` a `05-…`.
- Para ver uno concreto: `grep -rn 'GOB-I1' backend/docs/checklist-evidence-u*.md Referencias/tareas_verificacion_implementacion_IA.md` o, en PowerShell, `Select-String -Path backend/docs/checklist-evidence-u*.md,Referencias/tareas_verificacion_implementacion_IA.md -Pattern 'GOB-I1'`.

**Cuántos hay en cada estado.**

| Estado | Cuántos | Qué significa, en una frase |
|---|---:|---|
| Con algo hecho (parcial) | 60 | Existe algo en el repositorio que habla del requisito, pero eso no es su criterio de cierre. |
| Bloqueado | 66 | Falta un insumo que el repositorio no tiene: un dato real, un umbral, un responsable o una corrida. |
| Pendiente | 5 | Es trabajo definido y acotado, no empezado, y no espera nada de afuera. |
| Propuesto | 1 | Hay una respuesta escrita (un «no aplica» justificado) que ningún responsable firmó. |
| Cerrado | 0 | Nadie cerró ningún criterio. |

**Qué hace falta para cerrar uno.** Cuatro cosas a la vez: evidencia reproducible, umbrales acordados, un responsable con nombre y, cuando el criterio lo pide, datos reales. Por eso hay 0 de 132: una prueba en verde no aporta ninguna de las cuatro por sí sola. Ese «0» aplica una regla de cierre más estricta que la lista original: con el criterio literal, seis IDs de implementación que solo piden definir o elegir (`SLA-I1`, `SLA-I2`, `SLA-I3`, `ESC-I1`, `DRF-I1`, `CT-I1`) ya tienen su artefacto; lo que falta es que un responsable los acepte.

**De los 66 bloqueados, qué espera cada uno.** Un requisito puede necesitar más de un insumo, así que esto es una clasificación por palabra clave, no exacta.

| Qué espera | Cuántos |
|---|---:|
| Un responsable que decida o firme | 27 |
| Un umbral acordado (p95, RPS, RTO, tolerancia) | 12 |
| Una corrida o medición ejecutada | 9 |
| Datos reales u observados | 8 |
| Un artefacto que no existe (dataset, modelo, clasificación) | 10 |
| **Suma** | **66** |

27 + 12 = **39 requisitos donde una decisión es uno de los insumos**, pero no el único: la mayoría necesita además datos reales, un modelo o una corrida. Una firma no los cierra por sí sola.

---

## 5. Qué decir y qué no decir hoy

**Se puede decir:**

1. «Tenemos el sistema completo ejecutando el flujo sobre datos sintéticos, con contratos, observabilidad y pruebas registradas en verde.»
2. «Lo que no tenemos es la materia prima real, el modelo entrenado y ninguna medición bajo carga.»
3. «De los 66 requisitos bloqueados, 27 nombran a un responsable que debe decidir o firmar; solo un puñado depende únicamente de esa decisión.»
4. «El clasificador de hoy es una regla escrita a mano: funciona, pero no está calibrada contra incendios reales.»
5. «Los datos que se ven en la pantalla son seis escenarios inventados: prueban el sistema, no el terreno.»

**No se puede decir:**

1. «El modelo tiene tal exactitud» — no existe ninguna métrica de modelo.
2. «Tal comuna tiene riesgo alto de verdad» — el nivel sale de datos inventados.
3. «Cumplimos el SLA» o «tenemos 99,5 % de disponibilidad» — son objetivos escritos, no mediciones.

---

## 6. Glosario

| Palabra | Qué significa | Comparación cotidiana |
|---|---|---|
| Requisito (ID) | Una obligación de la lista del mandante, con un código propio | Una línea de la lista de útiles del colegio |
| Umbral | El número acordado que separa «normal» de «avisar» | Decir «avísame si tardo más de medio segundo» |
| Bloqueado | No se puede terminar porque falta algo que no está en el repositorio | Una receta a la que le falta el horno |
| Ground truth | El resultado real de lo que pasó después | Saber si llovió de verdad, no si el pronóstico acertó |
| Sintético | Dato inventado con reglas fijas, no observado | Un simulador de vuelo, no un avión |
| Clúster | Un grupo de servidores coordinados donde corre el sistema | Un edificio con varios departamentos |
| HPA (autoescalado) | Abrir más copias del servicio cuando hay mucha carga | Contratar más cajeros cuando la fila crece |
| SLA | La promesa de velocidad y disponibilidad, medida | La garantía de entrega de un reparto |
