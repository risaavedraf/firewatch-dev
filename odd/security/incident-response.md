# RED-I3 — Plan de respuesta a incidentes

**ID:** `RED-I3` — *«Documentar el plan de respuesta a incidentes»* (`Referencias/tareas_verificacion_implementacion_IA.md:182`), dentro de
la sección que exige un **rollback que desconecte un modelo comprometido y restaure una versión segura sin detener el servicio crítico** (`:177`).
**Estado:** redactado el 2026-11-09. Roles, escalamiento y disparador de rollback definidos. **No ejercitado**: ver §8.
**Documentos que este plan enlaza y no repite:** runbook de operación (`backend/docs/operations.md:138-200`), tabla alerta→acción
(`backend/docs/operations.md:48-65`), reglas y umbrales (`backend/deploy/local/alerts.yml`), política de riesgo de IA
([`ai-risk-classification-policy.md`](ai-risk-classification-policy.md)) y registro de vectores ([`atlas-vector-mitigations.md`](atlas-vector-mitigations.md)).

## 1. Alcance

Cubre un incidente que afecte la **clasificación de riesgo publicada** o la **integridad del artefacto del modelo**: nivel erróneo,
modelo comprometido, indisponibilidad de la inferencia, corrupción del artefacto, fuga de datos de entrenamiento o pérdida de trazabilidad
de un trabajo. No cubre incidentes de infraestructura sin efecto sobre el producto (caída del host, del clúster o del proveedor cloud):
esos se rigen por el frente 14.

## 2. Roles

Los roles se definen **por función**, no por persona. El nombramiento es la decisión abierta **D3**
(`odd/entrega/acta-cierre.md:31,37`) y este documento no inventa nombres: un plan con nombres inventados se lee como cubierto y no lo está.

| Rol | Responsabilidad en el incidente | Nombrado |
| --- | --- | --- |
| **Comandante del incidente** | Declara severidad, decide contención y cierre, única voz oficial | **No** — D3 abierta |
| **Responsable de servicio** | Ejecuta contención y recuperación sobre `gateway`, `jobs`, `risk`, `prevention` | **No** — D3 abierta |
| **AI Security Officer** | Decide si el modelo sigue siendo confiable; aprueba retirar o restaurar el artefacto | Rol creado por la política; **sin nombramiento** |
| **Responsable del proyecto** | Autoriza el rollback mayor, comunica al mandante, firma el cierre | **Ricardo Saavedra F.** (aprobador registrado en la política) |
| **Observador de evidencia** | Congela métricas, logs, auditoría y DLQ antes de tocar nada | **No** — D3 abierta |

## 3. Severidades

| Sev | Definición | Ejemplo en FireWatch | Plazo de declaración |
| --- | --- | --- | --- |
| **S1** | Nivel de riesgo publicado es falso o el modelo está comprometido | Evasión confirmada, artefacto sustituido, `alto` publicado como `bajo` | Inmediato |
| **S2** | El producto no clasifica, o clasifica con dependencia caída | `inference` caído, `risk` sin broker, 5xx sostenido | 15 min |
| **S3** | Degradación sin pérdida de veracidad | DLQ con un mensaje, pool saturado, p95 alto sin cambio de RPS | 1 h |
| **S4** | Sin efecto sobre el nivel publicado | Serie ausente, dashboard roto | Día hábil |

## 4. Señales de detección y primera acción

Una señal sin acción asignada no es detección, es ruido. Cada fila nombra la regla existente y la acción inmediata.

| Señal | Fuente | Sev inicial | Primera acción |
| --- | --- | --- | --- |
| `FireWatchHighErrorRate` (`>1%` de 5xx, 5 min) | `alerts.yml` | S2 | Distinguir defecto de dependencia: `/v1/risk/status` y `/v1/jobs/status` nombran `database_unavailable` o `broker_unavailable` |
| `FireWatchHighLatencyP95` (`>500 ms`, 5 min) | `alerts.yml` | S3 | Si el p95 sube **sin** subir el RPS, el problema no es carga: revisar PostGIS, memoria y red |
| `FireWatchDeadLetterBacklog` (`>0`) | `alerts.yml` | S3 | Inspeccionar `risk.jobs.v1.dead` o `jobs.results.v1.dead` y re-drive si es recuperable |
| `FireWatchStuckJobs` (edad `>1200 s`) | `alerts.yml` | S3 | Buscar la entrega asociada en ambas DLQ; un trabajo atascado sin entrega trazable no tiene replay hoy: registrar y escalar |
| `FireWatchBrokerUnavailable` | `alerts.yml` | S3 | Lecturas siguen disponibles y los trabajos quedan encolados en PostgreSQL: es degradación, no caída |
| `FireWatchTargetDown` | `alerts.yml` | S2 | Revisar pod y readiness. **`inference` importa más que los otros**: `risk` falla cerrado sin él |
| `FireWatchDatabasePoolSaturated` | `alerts.yml` | S3 | Buscar la consulta lenta; subir el límite sin encontrarla solo posterga la falla |
| `desconocido` creciente en la distribución publicada | `firewatch_risk_predictions_total{level="desconocido"}` | S2 | Un nivel `desconocido` masivo significa que las entradas no llegan o el clasificador no responde: es el fail-closed funcionando, no un error del modelo |
| Nivel publicado que contradice el parte oficial de incendios | Observación humana | **S1** | Congelar la publicación y abrir el disparador de rollback (§6) |

## 5. Escalamiento

```text
regla de Prometheus  →  quien mira el dashboard  →  comandante del incidente
                                                        │
                          ┌─────────────────────────────┼─────────────────────────────┐
                          v                             v                             v
                 Responsable de servicio        AI Security Officer        Responsable del proyecto
                 (S2, S3 técnicos)              (S1, duda de modelo)       (S1, comunicación y firma)
```

**Estado real del escalamiento, sin adornos:** las reglas **se evalúan y disparan**, pero **no hay Alertmanager**
(`backend/docs/operations.md:65-66,207`). Hoy la detección depende de que alguien mire la UI de Prometheus, Grafana o las rutas de
estado. Es decir: **el escalamiento automático no existe; existe el procedimiento manual y el nombre de quién decide.** Cerrar ese hueco
es trabajo del frente 5/6, no de este documento, y darlo por hecho aquí sería el defecto.

Canales declarados: el repositorio como registro durable del incidente, y la ruta alerta→acción de `alerts.yml` como guion de qué hacer.
Los contactos personales no se escriben en el repositorio.

## 6. Disparador de rollback

**El disparador se documenta con condiciones objetivas; no se ejercitó.**

Se activa cuando **cualquiera** de estas condiciones se cumple:

1. Un incidente **S1** confirmado, con evidencia de que el nivel publicado es falso o el artefacto fue sustituido.
2. El artefacto del modelo no coincide con la versión registrada: `firewatch_inference_model_info{version,data_hash}` cambia sin un cambio autorizado.
3. Una tasa de evasión confirmada sobre casos revisados supera lo que el AI Security Officer acepta mantener publicado.
4. `inference` responde con errores de validación de entrada de forma persistente y `risk` empieza a publicar `desconocido` en masa, con el clasificador de reglas disponible y sano.

### Procedimiento, en orden

| Paso | Acción | Efecto sobre el servicio | Evidencia a conservar antes |
| --- | --- | --- | --- |
| 1 | Congelar la evidencia: serie de `firewatch_risk_predictions_total`, `firewatch_inference_model_info`, logs `risk_assessed`, filas de auditoría y DLQ | Ninguno | Todo lo que permite reconstruir qué se publicó y cuándo |
| 2 | Retirar el modelo comprometido: detener `inference` y devolver `risk` a `MODEL_ID=firewatch-rule-v2` | **El servicio sigue publicando**: el clasificador de reglas es el camino por defecto y no depende de `inference` | `MODEL_ID` previo y `data_hash` en uso |
| 3 | Verificar que `risk` publica con el clasificador de reglas y que los trabajos encolados se procesan | Se recupera el nivel publicado; puede cambiar el criterio | Estado de readiness y `/v1/risk/status` |
| 4 | Revisar los trabajos publicados durante la ventana del incidente y marcar los que requieran reclasificación | Solo esa ventana | Lista de `job_id` con su nivel original derivada de la auditoría |
| 5 | Restaurar una versión segura del artefacto solo con aprobación del AI Security Officer y del responsable del proyecto | Vuelve a la inferencia; solo si el artefacto nuevo tiene procedencia verificada | Diff del artefacto y su `data_hash` |

**Por qué el paso 2 no detiene el servicio crítico.** FireWatch tiene dos clasificadores intercambiables del mismo contrato
(`backend/services/risk/internal/classify/classifier.go`), y el despliegue por defecto **no** usa la inferencia (`MODEL_ID` por defecto
`firewatch-rule-v2`, `backend/docker-compose.yml:167`). El "botón de pánico" existe por diseño: retirar el modelo reinicia la publicación
con reglas, no apaga el producto.

**Lo que falta y se declara:** no hay firma del artefacto (`backend/deploy/prod/README.md:113`), así que el paso 5 restaura un archivo y
compara su `data_hash`, no verifica una firma. Y **`RED-I2` (procedimiento de rollback) sigue bloqueado**: el rollback de un artefacto
desplegado nunca se ejecutó, así que este procedimiento está escrito y no probado.

## 7. Ciclo del incidente

1. **Declarar**: sev, comandante, hora, primera señal. Registro en el repositorio, no en un chat que se pierde.
2. **Contener**: desconectar la causa (modelo, dependencia o ruta) sin apagar el producto; el paso 2 de §6 es el camino por defecto.
3. **Erradicar**: corregir el defecto de origen — artefacto, entrada o configuración — con su prueba.
4. **Recuperar**: reanudar la publicación, re-clasificar la ventana afectada, re-drive de lo recuperable.
5. **Cerrar**: post-mortem escrito con línea de tiempo, causa, lo que falló en la detección y qué cambia. Firma el responsable del proyecto.

## 8. Qué no está probado

- **Ningún simulacro.** No hubo drill de S1 ni de rollback; `RED-V1` sigue abierto por eso.
- **Ningún incidente real** ocurrió en este sistema.
- **Sin Alertmanager**, la detección automática→escalamiento no existe; el plan describe el trabajo manual correcto.
- **Sin responsables nombrados** (D3), los roles están definidos y vacíos: un plan sin nombres asignados no se puede ejecutar bajo presión.
