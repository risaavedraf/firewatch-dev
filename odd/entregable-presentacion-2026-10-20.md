# Entrega para la presentación — lo que tenemos y lo que falta

**Corte:** 2026-10-20, rama `docs/code-alignment-and-client-gaps` sobre `main`.
**Fuente del pedido:** [`Referencias/tareas_verificacion_implementacion_IA.md`](../Referencias/tareas_verificacion_implementacion_IA.md), 132 IDs.
**Regla de este documento:** es un guion para leer en voz alta, no un cierre. **Ningún ID está cerrado y ninguna casilla está marcada.** Todo lo que sigue es lectura de archivo: no se ejecutó prueba, script, contenedor ni clúster. Una prueba en PASS no cierra un requisito.

---

## 1. LO QUE TENEMOS

| Capacidad | Evidencia |
|---|---|
| Ciclo de ingesta automático al arranque y cada 15 minutos, con frescura declarada por fuente (`last_run_at` + 900 s) y replay de operador autenticado por job | `backend/README.md:38`; `backend/services/jobs/internal/handler/api.go:187` |
| Servicio `prevention` como módulo y proceso propio (puerto 8084, esquema `prevention`, migración 009), alcanzado por el gateway | `backend/docs/architecture.md:34`; `backend/docker-compose.yml:166`; `backend/deploy/k8s/base/workloads.yaml:134` |
| Observabilidad local: Prometheus raspa cinco objetivos (jobs, risk, gateway, rabbitmq, prevention), siete reglas de alerta, dashboard versionado como archivo y Grafana con acceso anónimo deshabilitado y contraseña de administrador obligatoria | `backend/deploy/local/prometheus.yml:8-32`; `backend/deploy/local/alerts.yml:12`; `backend/deploy/local/grafana/dashboards/firewatch-overview.json:190`; `backend/docker-compose.yml:187-199` |
| Métricas de riesgo por nivel y contador de alto riesgo, poblados solo después de persistir el lote | `backend/services/risk/internal/metrics/metrics.go:56-60`; `backend/services/risk/internal/metrics/metrics_test.go:24` |
| Latencia (histograma), clases de error y señales de proceso en los cuatro servicios | `backend/services/gateway/internal/metrics/http.go:74`; `backend/services/jobs/internal/metrics/http.go:75`; `backend/services/risk/internal/metrics/http.go:64`; `backend/services/prevention/internal/metrics/http.go:73` |
| Autoescalado declarado: un `HorizontalPodAutoscaler` por servicio, 2→6 réplicas al 70 % de CPU, y cuatro `PodDisruptionBudget` con `minAvailable: 1` | `backend/deploy/k8s/base/autoscaling.yaml:14`, `:20-21`, `:86`; `backend/deploy/k8s/base/workloads.yaml:208` |
| Escenario de carga escrito con los umbrales acordados: 4 RPS constantes, `p(95)<500`, `max<2000`, `http_req_failed<0.01` | `backend/deploy/load/k6-risk.js:29`, `:36-40` |
| Seis perfiles sintéticos con resultado esperado declarado, generación determinista y selector de escenario en la interfaz | `backend/docs/synthetic-scenarios.md:17-22`; `backend/services/risk/internal/source/synthetic_scenarios.go:23`; `frontend/src/views/JobsView.tsx:400-410` |

**Octava capacidad, si hay que nombrarla aparte:** la CI verifica cada servicio (formato, integridad de módulos, `go test -race`, `go vet`, `govulncheck`, SBOM/provenance y escaneo de imagen) y la integración levanta el stack, corre el smoke y valida el contrato OpenAPI y los manifiestos — `.github/workflows/_go-service.yml:27-36`, `:49`, `:56`; `.github/workflows/integration.yml:26-30`, `:31-37`. Está configurada y corrió verde en `main` (`backend/docs/architecture.md:57`).

---

## 2. LO QUE FALTA

| Qué falta | Por qué falta | ID que lo bloquea |
|---|---|---|
| Ingesta de una fuente real de observaciones y su validación contra rangos productivos | **Dato real**: el repositorio solo trae un fixture sintético y un activo geográfico real | `DAT-I1`, `DAT-I2`, `DAT-V1` |
| Modelo entrenado con Accuracy, F1 y Recall medidos | **No construido**: el clasificador activo es una regla determinista congelada por hash (`academic-rule-v1`) | `REG-I1`, `MET-I1` |
| Ground truth: el resultado observado de un incendio | **Dato real**: no hay fuente, ni esquema, ni una fila | `LOG-I3`, `ALR-V1` |
| Medición de drift contra datos de referencia y de concepto | **Umbral acordado + dato real**: falta la prueba estadística elegida, su umbral y la distribución real que perturbar | `DRF-I1`, `DRF-I2`, `DRF-V1` |
| Corrida de carga con p95/p99, RPS y errores medidos | **Clúster/entorno**: el escenario está escrito y no se ejecutó | `SLA-I3`, `SLA-V1`, `SLA-V2` |
| Aceptación de los objetivos hoy declarados (p95 < 500 ms, 4 RPS, RTO ≤ 15 min, umbrales de alerta) | **Responsable con nombre**: nadie los firmó como objetivo comprometido | `SLA-I1`, `SLA-I2`, `ALR-I1` |
| Política interna de clasificación de riesgo de IA y oficial de seguridad designado | **Responsable con nombre / decisión organizacional**: no existe el documento ni la persona | `GOB-I1`, `GOB-I3` |
| Documento Go/No-Go con su firma | **Responsable con nombre**: ningún documento sustituye la firma | `GNG-V1`, `GNG-V5` |

Agregado exacto de los 132 IDs, contado sobre las filas de los cuatro registros: **60 parciales, 66 bloqueados, 5 pendientes, 1 propuesto, 0 cerrados** ([`estado-requerimientos-cliente.md`](estado-requerimientos-cliente.md)).

---

## 3. Cuatro ausencias totales

Conviene decirlas antes de que las pregunten.

| Ausencia | Cómo se comprueba |
|---|---|
| **Trazas distribuidas con OpenTelemetry** | `grep -rl opentelemetry backend/` no devuelve nada (exit 1) y ningún `go.mod` lo declara. Lo que existe es un identificador de correlación `trace_id`, que no es una traza (`backend/docs/architecture.md:55`) |
| **Firma de imágenes y promoción de registry** | `grep -rl 'cosign\|sigstore' backend/ .github/` no devuelve nada (exit 1). La CI produce SBOM y provenance; firmar y promover no está implementado (`backend/docs/architecture.md:57`) |
| **Un modelo entrenado y su pipeline** | No hay pipeline, ni dataset etiquetado, ni artefacto de modelo. El clasificador es una regla (`backend/services/risk/internal/classify/academic_rule.go:13`) |
| **Ground truth de incendios observados** | No hay fuente ni esquema de resultado observado; el propio repositorio declara que no mide drift ni benchmark de modelo (`backend/docs/synthetic-scenarios.md:47`) |

---

## 4. Frases permitidas y prohibidas

**Se puede afirmar hoy:**

1. «El backend implementa el ciclo de ingesta de 15 minutos con frescura por fuente y un replay de operador autenticado.»
2. «Hay cuatro servicios Go con métricas de latencia, error y proceso, siete reglas de alerta y un dashboard versionado como archivo.»
3. «El autoescalado está declarado: cuatro HPA de 2 a 6 réplicas al 70 % de CPU, con dos réplicas por servicio y `minAvailable: 1`.»
4. «El escenario de carga está escrito con los umbrales acordados; lo que falta es ejecutarlo, no la herramienta.»
5. «La CI verifica formato, integridad, pruebas con detector de carreras, vet, vulnerabilidades, SBOM/provenance y escaneo de imagen, y validó el contrato OpenAPI y los manifiestos.»
6. «El clasificador activo es una regla académica determinista, congelada por hash, que no está calibrada contra incendios observados.»
7. «Los datos que se sirven hoy son seis perfiles sintéticos: prueban comportamiento del código sobre entradas inventadas.»

**No se puede afirmar:**

1. «El modelo tiene tal exactitud, precisión o recall.» — No existe ningún Accuracy, F1 ni Recall en el repositorio, y una prueba en PASS no es una métrica de modelo.
2. «Tal comuna tiene riesgo real alto.» — El nivel lo produce una regla sobre datos inventados; `desconocido` es un nivel de primera clase y no un riesgo bajo.
3. «Cumplimos el SLA, el 99,5 % de disponibilidad o el p95 de 500 ms.» — Son objetivos declarados; no hay ninguna medición de latencia, throughput, disponibilidad ni RTO.

---

## 5. Límites de esta entrega

- **No cierra ningún ID y no marca casillas.** Los cinco IDs de aprobación (`GNG-V1`…`GNG-V5`) siguen sin marcar.
- **Nada se ejecutó.** Ningún `go test`, script, contenedor ni clúster corrió para producir este documento; cada ancla es una lectura de archivo, verificada abriendo la línea citada.
- **La colección documental no es una capacidad del producto.** Que la documentación de código esté verificada estructuralmente no dice nada sobre el comportamiento del sistema en producción.
- **Si una afirmación resulta falsa, se corrige contra el código o se retira.** No se suaviza.
