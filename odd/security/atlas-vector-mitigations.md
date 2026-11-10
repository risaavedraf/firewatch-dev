# AME-I2 — Registro vector por vector con su mitigación asignada

**ID:** `AME-I2` — *«Documentar cada vector (inversión, extracción, inyección de prompts, otros aplicables) con su mitigación asignada»* (`Referencias/tareas_verificacion_implementacion_IA.md:75`).
**Estado:** ejecutado el 2026-11-09. Un vector por fila, una mitigación asignada por fila, y el estado real de esa mitigación.
**Depende de:** [`atlas-threat-model.md`](atlas-threat-model.md) (`AME-I1`) — de ahí salen los identificadores y el alcance.
**Marco:** MITRE ATLAS 2026.10 (`format-version 6.1.0`, `modified-date` 2026-10-28, `atlas-data/dist/v6/ATLAS-2026.10.yaml`).

**Leyenda de estado.** `Implementado` = existe el mecanismo y su evidencia es verificable en el repositorio. `Parcial` = existe parte
del mecanismo o existe sin prueba. `Ausente` = no hay mecanismo; la mitigación está asignada y no construida.

**Resumen honesto:** de **22 vectores aplicables**, **7 están implementados** y **15 son parciales**. Ningún vector queda sin mitigación asignada. Además, dentro de los parciales hay **7 elementos de mitigación completamente ausentes** (§3): un vector puede tener dos mitigaciones asignadas, una construida y otra no, y contarlo como «implementado» ocultaría el hueco.

## 1. Registro de vectores aplicables

| ATLAS | Vector | Mitigación asignada | ATLAS | Estado | Evidencia y qué falta |
| --- | --- | --- | --- | --- | --- |
| `AML.T0006` | Active Scanning | Límite agregado de peticiones por réplica y CORS de un solo origen, de modo que el sondeo cueste y no se responda a cualquier origen | `AML.M0004` | Parcial | Límite 20/s burst 40 **por réplica**, no distribuido por usuario (`backend/docs/architecture.md:56-58`). Falta límite distribuido en clúster |
| `AML.T0007` | Discover AI Artifacts | `/metrics` no se publica fuera de la red del servicio, y la versión del modelo se expone solo ahí | `AML.M0000` | Implementado | `inference` no publica puerto en `backend/docker-compose.yml:143-162`; Prometheus lo scrapea por nombre de servicio (`deploy/local/prometheus.yml`) |
| `AML.T0040` | AI Model Inference API Access | La inferencia solo es alcanzable desde `risk` en la red del compose; no hay ruta pública a `/predict` | `AML.M0019` | Implementado (en el despliegue modelado) | Igual que la fila anterior. **Riesgo residual:** cualquier despliegue que publique `inference` abre este vector sin cambiar código |
| `AML.T0043` | Craft Adversarial Data | Validación de rango de las cinco entradas y `desconocido` como nivel de primera clase cuando faltan datos | `AML.M0010` | Parcial | El contrato separa `desconocido` de `bajo` (`backend/services/risk/internal/classify/classifier.go:20-25`). **Falta** detección de datos adversariales: un valor dentro de rango físico se acepta |
| `AML.T0015` | Evade AI Model | Revisión humana del nivel publicado y registro de las cinco entradas de cada clasificación para poder reconstruir el caso | `AML.M0003` | Parcial | `risk_assessed` registra entradas y resultado (`odd/plan-produccion/05-observabilidad.md` L3). **Falta** endurecimiento del modelo, ensamble o fusión multi-sensor; no hay medición de tasa de evasión |
| `AML.T0020` | Training Data Poisoning | Análisis archivo por archivo de los datos entregados antes de entrenar, con la conclusión escrita de qué no pueden sostener | `AML.M0007` | Parcial | `backend/training/ANALYSIS-delivered-data.md` mide los tres archivos. **Falta** procedencia por registro y firma del origen |
| `AML.T0059` | Erode Dataset Integrity | Un corte de datos declarado y versionado por el pipeline, no una consulta viva al histórico | `AML.M0025` | Parcial | El análisis fija qué contiene cada archivo (§4). **Falta** un catálogo de datos con versión y huella por lote (`AML.M0023`) |
| `AML.T0005` | Replicate AI Model Behavior | Límite de consultas y ausencia de ruta pública a la inferencia | `AML.M0004` | Parcial | Mismos controles que `T0040`. **Falta** cuota por identidad: el modelo es de cinco entradas y se replica con pocas consultas |
| `AML.T0144` | Recover AI Model Internals | Permisos mínimos sobre el artefacto: montaje de solo lectura y proceso sin privilegios | `AML.M0001` | Parcial | El artefacto se monta `:ro` (`backend/docker-compose.yml:147-149`) y el contenedor corre como `65532` sin capacidades (`:10-19`). **Falta** cifrado del artefacto en reposo y firma; el JSON es legible por diseño |
| `AML.T0024` | Exfiltration via AI Inference API | Telemetría de las consultas y de la distribución de niveles publicados para detectar un patrón de extracción | `AML.M0024` | Parcial | `firewatch_risk_predictions_total{level=...}` y `firewatch_high_risk_predictions_total` existen (`05-observabilidad.md` L3). **Falta** alerta sobre la distribución; hoy las series se exponen y nadie las lee |
| `AML.T0035` | AI Artifact Collection | Escaneo de imágenes y solicitud de SBOM/provenance en CI, con base y dependencias fijadas por digest/commit | `AML.M0016` | Parcial | `backend/docs/architecture.md:60-61`. **Falta** firma de imagen: el frente 7 está abierto y `backend/deploy/prod/README.md:113` declara que no hay camino de rollback ni promoción |
| `AML.T0044` | Full AI Model Access | Contenedor distroless, sin root, sin capacidades y con FS de solo lectura: ejecutar dentro no es trivial y lo que se obtiene es el artefacto, no la base de datos | `AML.M0005` | Parcial | Ancla `*hardened` (`backend/docker-compose.yml:10-19`). **Falta** política de red que impida el pivote a otros servicios desde el contenedor comprometido |
| `AML.T0010` | Supply Chain Compromise | Escaneo de vulnerabilidades de dependencias Go e imágenes en CI | `AML.M0016` | Parcial | `govulncheck` y escaneo de imágenes en CI (`architecture.md:60-61`). **Falta** cubrir el artefacto del modelo en esa misma cadena: hoy no se escanea ni se verifica |
| `AML.T0115` | Publish Poisoned AI Artifacts | Validación del artefacto al arrancar, por el mismo camino que usa el servicio para servirlo | `AML.M0008` | Parcial | El `healthcheck` corre `/server -validate` (`backend/docker-compose.yml:149-155`): un artefacto corrupto o ausente impide arrancar. **Falta** verificación de procedencia (`AML.M0014`): valida forma, no origen |
| `AML.T0132` | Misconfigured or Publicly Exposed AI Services | Puertos no publicados por defecto y ausencia de ruta pública a la inferencia | `AML.M0019` | Implementado (en el despliegue modelado) | `inference` y `prevention` sin `ports` en el compose; solo `gateway`, `identity` y la UI de RabbitMQ publican, y en `127.0.0.1` |
| `AML.T0029` | Denial of AI Service | `risk` falla cerrado cuando `inference` no responde, con timeout acotado, y el producto degrada a `desconocido` en vez de inventar un nivel | `AML.M0036` | Implementado | `INFERENCE_TIMEOUT` 5 s y fail-closed (`backend/docker-compose.yml:167-171`; comentario del job `inference` en `deploy/local/prometheus.yml`). Límites de recursos por contenedor (`mem_limit 256m`, `cpus 1.0`) |
| `AML.T0034` | Cost Harvesting | Límite de peticiones y de recursos por contenedor | `AML.M0004` | Parcial | Mismo límite por réplica. **Falta** cuota por usuario o por token |
| `AML.T0031` | Erode AI Model Integrity | Monitoreo de la distribución de entradas y salidas, con reevaluación de drift por lote | `AML.M0024` | Parcial | Método y frecuencia documentados (`05-observabilidad.md` L5, S3). **Falta** la comparación contra ground truth: no existe dato real, y sin ella la erosión es invisible |
| `AML.T0049` | Exploit Public-Facing Application | Cuerpos, cabeceras y tiempos acotados, consultas parametrizadas, roles de aplicación sin DDL, y validación de token en cada ruta `/v1/*` | `AML.M0016` | Implementado | `architecture.md:59`; middleware de autenticación sobre `/v1/*`. Escaneo de vulnerabilidades en CI |
| `AML.T0012` | Valid Accounts | Vida de token de cinco minutos y validación de firma, emisor, audiencia, expiración, tipo y rol en cada servicio | `AML.M0019` | Implementado | `architecture.md:55`. **Riesgo residual declarado:** la revocación es por expiración, no por introspección |
| `AML.T0047` | AI-Enabled Product or Service | Registro de auditoría por trabajo y trazabilidad de qué se publicó y cuándo | `AML.M0024` | Implementado | Auditoría transaccional con el estado del trabajo (`architecture.md:22-25`) |
| `AML.T0137` | Training Data Information from AI Models | No se publica la distribución de entrenamiento en la API; las series agregadas quedan en `/metrics` interno | `AML.M0002` | Parcial | El nivel publicado es una etiqueta de cuatro valores. **Falta** control sobre los agregados: `firewatch_risk_predictions_total` describe la distribución y vive en un endpoint interno, no protegido por rol |

## 2. Vectores declarados N/A

Se justifican en [`atlas-threat-model.md`](atlas-threat-model.md) §4: `AML.T0051` (prompt injection), `AML.T0054` (jailbreak),
`AML.T0080` (context poisoning), `AML.T0110` (tool poisoning), `AML.T0053` (tool invocation) y `AML.T0088` (deepfakes). La razón
común es una sola y verificable: **el producto no invoca un modelo de lenguaje ni un agente, y no genera contenido sintético.**

## 3. Los siete elementos de mitigación que no existen

Estos siete son **huecos dentro de filas parciales**: la mitigación está asignada y ese elemento concreto **no está construido**. Nombrarlos por
separado evita que el conteo de «parciales» los diluya.

1. `AML.T0115` — verificación de procedencia del artefacto (`AML.M0014`): valida forma, no origen.
2. `AML.T0144` — cifrado del artefacto y firma (`AML.M0001`, `AML.M0013`).
3. `AML.T0035` — firma de imagen y promoción verificada (frente 7, abierto).
4. `AML.T0044` — política de red que impida el pivote desde un contenedor comprometido.
5. `AML.T0010` — el artefacto del modelo dentro de la cadena escaneada.
6. `AML.T0015` — endurecimiento del modelo o medición de su resistencia a evasión.
7. `AML.T0031` — comparación de calidad contra ground truth, bloqueada por el dato real (S3).

Los siete comparten una causa: **no hay artefacto firmado ni dato real**. Ninguno se cierra escribiendo documentación.

## 4. Qué no prueba este registro

- No asigna probabilidad ni impacto: es una tabla de decisión, no un cálculo de riesgo.
- `Implementado` significa que el mecanismo existe y su evidencia es legible en el repositorio; **no** que se haya probado un ataque contra él.
- Los estados describen el despliegue modelado (compose local más diseño de clúster), no un entorno productivo.
