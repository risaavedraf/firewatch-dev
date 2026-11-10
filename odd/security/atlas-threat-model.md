# AME-I1 — Ejercicio de modelado de amenazas MITRE ATLAS sobre el diseño de IA de FireWatch

**ID:** `AME-I1` — *«Ejecutar un ejercicio de modelado de amenazas con MITRE ATLAS sobre el diseño lógico»* (`Referencias/tareas_verificacion_implementacion_IA.md:74`).
**Estado:** ejecutado el 2026-11-09. Catálogo de amenazas y diagrama de flujo de datos incluidos.
**Registro de vectores y mitigaciones:** [`atlas-vector-mitigations.md`](atlas-vector-mitigations.md) (`AME-I2`).
**Marco y versión citada:** MITRE ATLAS **2026.10**, `format-version 6.1.0`, `modified-date` 2026-10-28, leído de
`https://raw.githubusercontent.com/mitre-atlas/atlas-data/main/dist/v6/ATLAS-2026.10.yaml`.
Cada identificador `AML.T…` y `AML.M…` de este documento y del registro fue tomado de ese archivo, no de memoria.
La release no incluye la arista técnica→táctica, así que este ejercicio **no asigna tácticas**: cita identificador y nombre, que son los datos verificables.

## 1. Qué se modela

El sistema de IA de FireWatch clasifica riesgo de incendio por zona a partir de cinco entradas
(`temperatura_c`, `humedad_relativa_pct`, `viento_kmh`, `cobertura_vegetal_pct`, `incendios_historicos`) y publica un nivel
`bajo | medio | alto | desconocido` (`backend/services/risk/internal/classify/classifier.go:30-33`).

Hay **dos** implementaciones intercambiables del mismo contrato de clasificación, y esto importa para el modelo de amenazas
porque tienen superficies distintas:

| Implementación | Qué es | Dónde corre | Se usa por defecto |
| --- | --- | --- | --- |
| `firewatch-rule-v2` | Clasificador determinista por reglas, en proceso | `risk` | **Sí** (`MODEL_ID` por defecto, `backend/docker-compose.yml:167`) |
| `valparaiso-logistic-v1` | Regresión logística serializada en JSON, servida por HTTP | `inference` (proceso aparte, distroless) | No: solo con `MODEL_ID=ml-fixture-v1` |

El artefacto del modelo se carga desde `/models/logistic.json`, montado de solo lectura
(`backend/docker-compose.yml:145-151`), y la versión y la huella del artefacto se exponen como
`firewatch_inference_model_info{version,data_hash}` (`backend/services/inference/internal/metrics/`).

**Ningún componente del producto usa un modelo de lenguaje ni un agente.** El pipeline de entrenamiento es Python y offline
(`backend/training/`), y no forma parte del camino de una petición.

## 2. Diseño lógico y flujo de datos

```text
                          FRONTERA 1: red no confiable
  navegador (otro origen) ─────┐
  cliente API (curl, script) ──┤
  atacante anónimo ────────────┤
                               v
                     ┌───────────────────┐
                     │ gateway :8080     │  límite 20/s burst 40 por réplica,
                     │ JWT RS256, CORS   │  cabeceras de identidad eliminadas
                     └─────────┬─────────┘
       ┌───────────────────────┼────────────────────────┐
       v                       v                        v
  ┌─────────┐            ┌──────────┐            ┌────────────┐
  │ jobs    │            │ risk     │            │ prevention │
  │ :8080   │            │ :8080    │            │ :8084      │
  └────┬────┘            └────┬─────┘            └─────┬──────┘
       │                      │                        │
       │              ┌───────┴────────┐               │
       │              v                v               │
       │     ┌─────────────────┐  ┌─────────┐          │
       │     │ clasificador    │  │inference│          │
       │     │ de reglas       │  │ :8080   │          │
       │     │ (en proceso)    │  │ /predict│          │
       │     └─────────────────┘  └────┬────┘          │
       │                               │               │
       v                               v               v
  ┌──────────────────────────────────────────────────────────┐
  │ PostgreSQL/PostGIS  (esquemas jobs, risk, prevention)     │
  └──────────────────────────────────────────────────────────┘
       ^
       │  outbox → RabbitMQ → inbox
       │
  ┌─────────────────────────┐        ┌──────────────────────────────┐
  │ artefacto del modelo    │───────>│ inference (FS de solo lectura)│
  │ /models/logistic.json   │        └──────────────────────────────┘
  └───────────^─────────────┘
              │  FRONTERA 3: pipeline offline (backend/training/, Python)
              │  datos entregados → análisis → artefacto
        ┌─────┴──────────────────────────────┐
        │ FRONTERA 2: artefacto y su cadena  │
        │ build de imágenes, dependencias Go │
        └────────────────────────────────────┘
```

**Fronteras de confianza**

| # | Frontera | Qué la cruza | Control declarado |
| --- | --- | --- | --- |
| 1 | Red no confiable → gateway | Peticiones HTTP de cualquier origen | Bearer RS256, CORS de un solo origen, límite agregado por réplica (`backend/docs/architecture.md:56-58`) |
| 2 | Cadena de suministro → servicios | Imágenes, dependencias Go, artefacto del modelo | Escaneo de imágenes, SBOM/provenance solicitados, digest/commit pinneados, `firewatch_inference_model_info` con `data_hash` (`:60-61`) |
| 3 | Pipeline offline → artefacto en ejecución | El JSON del modelo | Ninguno automático: el artefacto entra por `build`/`volume` y el servicio solo lo valida al arrancar (`-validate`) |
| 4 | Servicios → `/metrics` | Series internas | Puerto no publicado en el compose local; en clúster, red interna |

La frontera 3 es la más débil y así se declara: **no hay firma del artefacto ni promoción entre entornos**
(`backend/deploy/prod/README.md:113`), y el frente 7 de `odd/plan-produccion/` está abierto justo por eso.

## 3. Actores

| Actor | Acceso | Evidencia |
| --- | --- | --- |
| `admin` | Comandos de trabajo, replay de fallidos | Rol del realm (`backend/deploy/local/firewatch-realm.json:8`) |
| `viewer` | Lectura de resultados e histórico | Ídem |
| `operator` | **No existe**: el handler rechaza el rol como desconocido | `backend/services/risk/internal/handler/api_test.go:111` |
| Anónimo | Solo `/health/live`, `/health/ready`; el resto responde 401 | Medición del 2026-11-09: 4xx masivo sin token |
| Atacante externo | Frontera 1 | — |
| Insider del pipeline | Frontera 3 | — |

## 4. Catálogo de amenazas

Cada fila nombra la técnica ATLAS, dónde aterriza **en este sistema** y por qué es alcanzable aquí. La columna «¿aplica?» es la
decisión de este ejercicio, no una propiedad del marco.

| ATLAS | Técnica | ¿Aplica? | Dónde aterriza en FireWatch | Por qué es alcanzable aquí |
| --- | --- | --- | --- | --- |
| `AML.T0006` | Active Scanning | Sí | `gateway :8080` | El gateway es la única superficie HTTP publicada; responde a sondeo de rutas y el 401 confirma existencia de `/v1/*` |
| `AML.T0007` | Discover AI Artifacts | Sí | `/metrics` de `inference` y `risk` | `firewatch_inference_model_info` publica versión y `data_hash` del artefacto; cualquier lectura de `/metrics` enumera la familia del modelo |
| `AML.T0040` | AI Model Inference API Access | Sí | `POST /predict` de `inference` | Existe un contrato HTTP de inferencia real (`backend/services/inference/README.md`); hoy no publicado fuera de la red del compose, pero es el camino previsto con `MODEL_ID=ml-fixture-v1` |
| `AML.T0043` | Craft Adversarial Data | Sí | Entradas del clasificador | Las cinco entradas vienen de fuentes externas (meteorología, vegetación, histórico) y el modelo es tabular: pequeñas perturbaciones dentro de rango físico son indistinguibles de un dato válido |
| `AML.T0015` | Evade AI Model | Sí | Salida de nivel de riesgo | Un incendio clasificado `bajo` no dispara la misma respuesta operativa que `alto`; ese es exactamente el efecto buscado |
| `AML.T0020` | Training Data Poisoning | Sí | `backend/training/` y los archivos entregados | El entrenamiento consume archivos externos (CONAF, meteorología) sin procedencia firmada por registro |
| `AML.T0059` | Erode Dataset Integrity | Sí | Serie histórica de incendios | Las etiquetas dependen de un registro con acceso restringido y de una fecha local declarada; una corrección de datos cambia el pasado del modelo |
| `AML.T0005` | Replicate AI Model Behavior | Sí | `inference` | Con acceso a `/predict`, un modelo logístico de cinco entradas se aproxima con cientos de consultas |
| `AML.T0144` | Recover AI Model Internals | Sí | Artefacto `/models/logistic.json` | El artefacto es JSON legible por diseño (no cifrado); quien lo lea tiene los coeficientes exactos |
| `AML.T0024` | Exfiltration via AI Inference API | Sí | `risk` → `inference` | Cada consulta devuelve una etiqueta derivada de datos del territorio; agregarlas revela la distribución de entrenamiento |
| `AML.T0035` | AI Artifact Collection | Sí | Repositorio e imágenes | No hay firma de imagen ni de artefacto todavía; la colección no necesita más que lectura |
| `AML.T0044` | Full AI Model Access | Sí | Contenedor `inference` | Distroless, no root y FS de solo lectura reducen el camino, pero cualquier ejecución dentro del contenedor entrega el artefacto |
| `AML.T0010` | Supply Chain Compromise | Sí | Build de imágenes, dependencias Go, base distroless | CI fija digest/commit y escanea, pero el artefacto del modelo queda fuera de esa cadena |
| `AML.T0115` | Publish Poisoned AI Artifacts | Sí | Artefacto publicado y `testdata/logistic.json` | El artefacto se monta desde el repositorio; un cambio en ese archivo cambia el modelo sin cambiar código |
| `AML.T0132` | Misconfigured or Publicly Exposed AI Services | Sí | `inference` y `/metrics` | Hoy `inference` no publica puerto en el compose local; un despliegue que lo publique o un `NetworkPolicy` ausente lo expone |
| `AML.T0029` | Denial of AI Service | Sí | `risk` → `inference` | `risk` falla cerrado ante `inference` ausente: la indisponibilidad degrada la clasificación a `desconocido` de forma deliberada |
| `AML.T0034` | Cost Harvesting | Sí | Gateway y `inference` | El límite es por réplica, no distribuido por usuario; un cliente puede sostener tráfico dentro del límite |
| `AML.T0031` | Erode AI Model Integrity | Sí | Modelo en producción | Sin ground truth no hay detección de degradación; el drift se vigila por distribución de entradas, no por calidad |
| `AML.T0049` | Exploit Public-Facing Application | Sí | `gateway` | Es la única aplicación publicada; su superficie es `/v1/*` y `/health*` |
| `AML.T0012` | Valid Accounts | Sí | Roles `admin`/`viewer` | Un token robado dura hasta cinco minutos; la revocación es por expiración, no por introspección |
| `AML.T0047` | AI-Enabled Product or Service | Sí | Producto completo | Un atacante puede usar el producto legítimamente como etapa: consultar riesgo para elegir dónde actuar |
| `AML.T0137` | Training Data Information from AI Models | Sí | Salidas del clasificador | La distribución de niveles y los logueos de entradas (`risk_assessed`) describen el conjunto de entrenamiento |

### Vectores declarados N/A

| ATLAS | Técnica | Por qué no aplica aquí |
| --- | --- | --- |
| `AML.T0051` | LLM Prompt Injection | El producto no invoca ningún modelo de lenguaje: no hay prompt, ni contexto, ni instrucción que inyectar. El texto libre de `assignee` de una acción preventiva (`backend/services/prevention/migrations/009_preventive_actions.sql:30,39`) se almacena y se muestra, no se interpreta como instrucción |
| `AML.T0054` | LLM Jailbreak | Igual que el anterior: sin LLM no hay política de comportamiento que eludir |
| `AML.T0080`, `AML.T0110`, `AML.T0053` | Context poisoning, tool poisoning, tool invocation | No hay agentes ni herramientas invocables por un modelo |
| `AML.T0088` | Generate Deepfakes | El producto no genera ni consume contenido sintético audiovisual |

Estas exclusiones son la mitad del ejercicio: un `N/A` justificado vale tanto como una mitigación, y sin la justificación el
registro sería una lista de buenas intenciones.

## 5. Supuestos del ejercicio

1. **Sin ground truth, la amenaza se mide por exposición, no por impacto calibrado.** No hay datos que permitan decir cuánto daño
   produce una evasión; el registro asigna mitigación, no probabilidad.
2. **El despliegue modelado es el del compose local más el diseño de clúster declarado**, no un entorno productivo existente.
3. **Un atacante con acceso a la red interna ya ganó la frontera 1**; las amenazas internas se evalúan sobre esa premisa.
4. **El modelo por defecto es el de reglas.** Donde el vector solo existe con `inference` activo, se dice explícitamente.

## 6. Qué no prueba este ejercicio

- No asigna probabilidad, impacto económico ni riesgo residual cuantificado: no hay datos para eso y fingirlo sería peor que no tenerlo.
- No sustituye una prueba de penetración ni un *red team*: modela, no ejecuta ataques.
- No cubre la infraestructura del clúster (red, IAM del proveedor, control de nodos), que pertenece al frente 7 y al hardening.
- No valida el desempeño del modelo: `AML.T0015` describe el vector, no mide cuánto resiste el clasificador actual.
