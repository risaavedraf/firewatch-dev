# Plan de tareas: Verificación e Implementación de un Sistema IA

**Asignatura:** ITY1102 Arquitectura de Sistemas IA (DuocUC)
**Caso de aplicación:** clasificación de riesgo de incendios forestales en Valparaíso (niveles Bajo / Medio / Alto)
**Fuentes consolidadas:**

| # | Documento | Tema |
|---|-----------|------|
| 1 | `2_4_5_Plantilla_de_Seguridad.docx` | Plan de seguridad de sistema IA (3 fases, 10 puntos) |
| 2 | `2_4_6_Guía_Pruebas_Sistema_IA.docx` | Guía teórica de pruebas para arquitectura IA |
| 3 | `2_4_7_Plantilla_de_Pruebas_de_sistemas_IA.docx` | Plantilla de pruebas (5 bloques) |
| 4 | `PRESENTACION_PRUEBAS_SISTEMAS_IA.pdf` | Presentación: pruebas de datos, modelo e infraestructura |
| 5 | `PRESENTACION_OPTIMIZACION_MONITOREO_1_.pdf` | Presentación EA2 Sesión 4.2: optimización, monitoreo, Prometheus + Grafana |

---

## Cómo leer este documento

- 🔍 **V = Verificación:** comprobar, medir, auditar o validar algo que ya existe o que se propone.
- 🛠 **I = Implementación:** construir, configurar, programar o documentar algo.
- Cada tarea tiene un ID (por ejemplo `DAT-V1`, `DAT-I1`) para poder referenciarla.
- **Regla de oro de las plantillas (presentación de Pruebas, diapositiva 14):**
  1. Responder solo con lo que realmente existe; si algo no fue medido, marcarlo como **objetivo / propuesta**.
  2. No inventar métricas ni herramientas; justificar **N/A** cuando un punto no corresponda.
  3. Cada decisión debe tener **evidencia esperada** y **criterio de aprobación**.
- La guía aclara que **no se espera completar la plantilla perfectamente**: es iterativo, pero las respuestas deben ser **coherentes y justificadas**.
- Los valores numéricos que aparecen abajo (p. ej. `< 200 ms`, `500 req/s`, `Disparate Impact > 0.8`, `CPU > 70%`) son **ejemplos de las plantillas**, no valores del proyecto. El equipo debe definir los suyos.

---

## Índice

1. [Gobierno y diseño de seguridad](#1-gobierno-y-diseño-de-seguridad)
2. [Protección de datos y del modelo](#2-protección-de-datos-y-del-modelo)
3. [Implementación y operación segura](#3-implementación-y-operación-segura)
4. [Pruebas de datos (Data Pipeline Assurance)](#4-pruebas-de-datos-data-pipeline-assurance)
5. [Evaluación y robustez del modelo](#5-evaluación-y-robustez-del-modelo)
6. [Infraestructura y operación (MLOps)](#6-infraestructura-y-operación-mlops)
7. [Optimización de infraestructura](#7-optimización-de-infraestructura)
8. [Monitoreo: Prometheus + Grafana](#8-monitoreo-prometheus--grafana)
9. [Stack tecnológico de pruebas](#9-stack-tecnológico-de-pruebas)
10. [Continuous Testing (CT) en MLOps](#10-continuous-testing-ct-en-mlops)
11. [Aprobación final Go / No-Go](#11-aprobación-final-go--no-go)
12. [Actividad práctica de la Sesión 4.2](#12-actividad-práctica-de-la-sesión-42)
13. [Matriz de trazabilidad y orden sugerido](#13-matriz-de-trazabilidad-y-orden-sugerido)

---

# PARTE A: Plan de seguridad (Documento 1)

## 1. Gobierno y diseño de seguridad

> **Fase I del plan de seguridad.** Define el marco, identifica amenazas y diseña el acceso.

### 1.1 Políticas y clasificación de riesgos

**Qué dice la fuente:** definir un marco normativo interno que clasifique los sistemas de IA por nivel de riesgo (bajo, limitado, alto, inaceptable), alineado con regulaciones como el **EU AI Act**, y designar roles claros (ej. **AI Security Officer**) que aprueben los modelos antes de producción.

**🛠 Implementación**
- [ ] `GOB-I1` Redactar la política interna de clasificación de riesgo (bajo / limitado / alto / inaceptable).
- [ ] `GOB-I2` Clasificar el sistema de riesgo de incendios dentro de esa escala y justificar la categoría.
- [ ] `GOB-I3` Designar formalmente el rol de **AI Security Officer** (u equivalente) y registrar su nombre.
- [ ] `GOB-I4` Definir el procedimiento de **aprobación de modelo previo a producción** (quién firma, con qué evidencia).

**🔍 Verificación**
- [ ] `GOB-V1` Confirmar que la clasificación es consistente con el EU AI Act (u otra regulación elegida).
- [ ] `GOB-V2` Verificar que ningún modelo llegue a producción sin la aprobación del responsable designado.

### 1.2 Modelado de amenazas específico para IA

**Qué dice la fuente:** usar marcos como **MITRE ATLAS**; no basta analizar la infraestructura. Hay que identificar vectores propios de IA: **inversión del modelo**, **extracción de parámetros** e **inyección de prompts**.

**🛠 Implementación**
- [ ] `AME-I1` Ejecutar un ejercicio de modelado de amenazas con MITRE ATLAS sobre el diseño lógico.
- [ ] `AME-I2` Documentar cada vector (inversión, extracción, inyección de prompts, otros aplicables) con su mitigación asignada.

**🔍 Verificación**
- [ ] `AME-V1` Verificar que cada amenaza identificada tenga un control asociado en las secciones 2 y 3.
- [ ] `AME-V2` Justificar como **N/A** los vectores que no aplican (por ejemplo, inyección de prompts si el modelo no es generativo, como el Random Forest tabular del proyecto).

### 1.3 Arquitectura de confianza cero (Zero Trust)

**Qué dice la fuente:** ningún usuario, aplicación o proceso tiene confianza implícita. El acceso a modelos, datos de entrenamiento y pipelines de inferencia debe estar **segmentado, autenticado y regido por mínimo privilegio**.

**🛠 Implementación**
- [ ] `ZT-I1` Segmentar el acceso a: (a) modelos, (b) datos de entrenamiento, (c) pipeline de inferencia.
- [ ] `ZT-I2` Configurar autenticación para API, base de datos (PostGIS), Prometheus y Grafana.
- [ ] `ZT-I3` Aplicar mínimo privilegio con roles y permisos por componente.

**🔍 Verificación**
- [ ] `ZT-V1` Revisar que no existan accesos implícitos (servicios sin autenticación, credenciales compartidas).
- [ ] `ZT-V2` Auditar periódicamente los permisos otorgados.

---

## 2. Protección de datos y del modelo

> **Fase II del plan de seguridad.**

### 2.1 Cadena de suministro de datos (Data Provenance)

**Qué dice la fuente:** garantizar trazabilidad completa (**Data Lineage**), validar criptográficamente las fuentes para prevenir **data poisoning**, y auditar bibliotecas de terceros y modelos pre-entrenados descargados de repositorios públicos (vulnerabilidades y puertas traseras).

**🛠 Implementación**
- [ ] `PRV-I1` Registrar el linaje de cada dataset (origen, transformaciones, versión).
- [ ] `PRV-I2` Implementar validación criptográfica de fuentes (hashes / firmas).
- [ ] `PRV-I3` Mantener un inventario de dependencias y modelos pre-entrenados de terceros.

**🔍 Verificación**
- [ ] `PRV-V1` Verificar la integridad de las fuentes antes de cada ingesta.
- [ ] `PRV-V2` Auditar bibliotecas y modelos externos en busca de vulnerabilidades o puertas traseras.

### 2.2 Privacidad por diseño y anonimización

**Qué dice la fuente:** aplicar protección **antes del entrenamiento**: **Privacidad Diferencial** (evitar reidentificación) y **Cifrado Homomórfico** o **Computación Multipartita Segura (SMPC)** cuando los datos son altamente sensibles.

**🛠 Implementación**
- [ ] `PRI-I1` Identificar si el dataset contiene datos personales o sensibles.
- [ ] `PRI-I2` Aplicar privacidad diferencial o anonimización donde corresponda.
- [ ] `PRI-I3` Evaluar si se requiere cifrado homomórfico / SMPC; si no, justificar N/A.

**🔍 Verificación**
- [ ] `PRI-V1` Comprobar que no sea posible reidentificar usuarios a partir de los datos de entrenamiento.

### 2.3 Entrenamiento robusto y pruebas adversarias

**Qué dice la fuente:** someter el modelo a **entrenamiento adversario**: inyectar ruido intencional y ejemplos engañosos en el set de entrenamiento para que resista manipulaciones sutiles.

**🛠 Implementación**
- [ ] `ENT-I1` Generar ejemplos con ruido / engañosos y añadirlos al entrenamiento.
- [ ] `ENT-I2` Documentar el tipo y volumen de perturbación utilizada.

**🔍 Verificación**
- [ ] `ENT-V1` Medir el desempeño del modelo con y sin entrenamiento adversario (se conecta con la sección 5.2).

---

## 3. Implementación y operación segura

> **Fase III del plan de seguridad.**

### 3.1 Guardrails de entrada y salida

**Qué dice la fuente:** capa de filtrado entre usuario y modelo. **Entrada:** detectar y bloquear jailbreak e inyección de prompts. **Salida:** prevenir fugas de información sensible, alucinaciones dañinas o código malicioso.

**🛠 Implementación**
- [ ] `GRD-I1` Implementar validación de entrada (formato, rangos, contenido malicioso).
- [ ] `GRD-I2` Implementar revisión de salida (fugas de datos, respuestas dañinas).

**🔍 Verificación**
- [ ] `GRD-V1` Probar intentos de manipulación de entrada y confirmar que se bloquean.
- [ ] `GRD-V2` Confirmar que las salidas no exponen información sensible.

### 3.2 Aislamiento de entornos (Sandboxing)

**Qué dice la fuente:** ejecutar modelos en **contenedores estrictamente aislados** de la red corporativa y limitar llamadas externas (internet) salvo necesidad estricta, para mitigar ejecución remota de código.

**🛠 Implementación**
- [ ] `SBX-I1` Contenerizar el servicio de inferencia (Docker).
- [ ] `SBX-I2` Restringir la red del contenedor y bloquear salida a internet por defecto.

**🔍 Verificación**
- [ ] `SBX-V1` Verificar que el contenedor no pueda alcanzar la red corporativa ni hosts externos no autorizados.

### 3.3 Monitoreo de deriva y observabilidad (visión de seguridad)

**Qué dice la fuente:** alertas tempranas que midan no solo latencia y disponibilidad, sino la **calidad matemática de las predicciones**. Un Drift repentino puede ser el único indicador de un ataque en curso.

**🛠 Implementación**
- [ ] `OBS-I1` Configurar alertas sobre distribución de entradas y confianza del modelo (ver sección 8).

**🔍 Verificación**
- [ ] `OBS-V1` Simular una desviación repentina y confirmar que la alerta se dispara.

### 3.4 Red Teaming continuo y plan de respuesta

**Qué dice la fuente:** validar la seguridad periódicamente con **Red Teaming** (automatizado y manual). Debe existir un **"botón de pánico" / rollback** que desconecte un modelo comprometido y restaure una versión segura **sin detener el servicio crítico**.

**🛠 Implementación**
- [ ] `RED-I1` Definir un calendario periódico de ejercicios de Red Teaming.
- [ ] `RED-I2` Implementar el procedimiento de **rollback** a la versión anterior del modelo (versionado de modelos).
- [ ] `RED-I3` Documentar el plan de respuesta a incidentes.

**🔍 Verificación**
- [ ] `RED-V1` Ejecutar un simulacro de rollback y medir cuánto tarda.
- [ ] `RED-V2` Confirmar que el servicio sigue disponible durante el rollback.

---

# PARTE B: Plan de pruebas (Documentos 2, 3 y 4)

> **Idea central:** probar IA es más que probar código. Se requiere una estrategia multidimensional **Código + Datos + Modelo + Infraestructura**, porque los modelos son probabilísticos (estocásticos) y dependen de los datos.

## 4. Pruebas de datos (Data Pipeline Assurance)

**Objetivo:** garantizar integridad, calidad y equidad de los datos antes de entrenar o inferir ("Garbage In, Garbage Out").

### 4.1 Verificación de esquema y tipología

**🛠 Implementación**
- [ ] `DAT-I1` Listar las **fuentes de datos** (S3, SQL/PostGIS, API, etc.).
- [ ] `DAT-I2` Definir las reglas de validación críticas:
  - [ ] Integridad referencial (sin nulos en campos clave).
  - [ ] Rangos numéricos permitidos (por ejemplo, temperatura y humedad dentro de rangos físicos válidos).
  - [ ] Categorías permitidas.
  - [ ] Valores que respeten reglas de dominio.
- [ ] `DAT-I3` Definir la **acción ante fallo de esquema** (rechazar lote, alerta a Slack, imputación automática).

**🔍 Verificación**
- [ ] `DAT-V1` ¿Existen los campos esperados?
- [ ] `DAT-V2` ¿Tienen el tipo correcto?
- [ ] `DAT-V3` ¿Hay nulos críticos?
- [ ] `DAT-V4` ¿Los valores respetan las reglas de dominio?
- [ ] `DAT-V5` Confirmar que las fuentes son confiables.
- **Evidencia esperada:** reporte de validación. **Criterio:** campos requeridos presentes. **Resultado:** Pasa / Falla.

### 4.2 Análisis de sesgos (Bias Testing)

> Nota: la guía habla de grupos demográficos; la presentación aclara que **en este proyecto el foco puede ser geográfico o por clases**, no necesariamente demográfico.

**🛠 Implementación**
- [ ] `SES-I1` Definir los grupos a proteger o vigilar (edad, género, **zona geográfica**, clase de riesgo).
- [ ] `SES-I2` Definir la **métrica de paridad** (ejemplo de plantilla: Disparate Impact Ratio > 0.8).
- [ ] `SES-I3` Definir la **estrategia de mitigación** (oversampling, reweighting, etc.).

**🔍 Verificación**
- [ ] `SES-V1` Revisar clases y zonas **subrepresentadas** (cobertura por zona y por clase).
- [ ] `SES-V2` Calcular la métrica de paridad y compararla con el umbral.
- [ ] `SES-V3` Justificar N/A si no hay grupos humanos afectados (la presentación indica que *Fairness aplica cuando el caso involucra grupos humanos*).

### 4.3 Prevención de fugas de datos (Data Leakage)

**🛠 Implementación**
- [ ] `LEA-I1` Separar entrenamiento y prueba con criterio **temporal o geográfico** y justificarlo.
- [ ] `LEA-I2` Eliminar variables **"giveaway"** (que contienen la respuesta implícita) y listarlas.

**🔍 Verificación**
- [ ] `LEA-V1` Confirmar que el set de test es **estrictamente posterior** al de train (validación temporal).
- [ ] `LEA-V2` Confirmar que no hay información futura ni del test en el entrenamiento (de lo contrario las métricas quedan invalidadas).

---

## 5. Evaluación y robustez del modelo

**Objetivo:** validar la lógica, seguridad y rendimiento comparativo del artefacto de IA.

### 5.1 Pruebas metamórficas (invariancia y direccionalidad)

**🛠 Implementación**
- [ ] `MET-I1` Escribir al menos un caso de **invariancia**: al cambiar X (irrelevante), la predicción Y debe mantenerse. *Ejemplo del proyecto: un identificador de visualización que no entra al modelo no debe cambiar el nivel de riesgo.*
- [ ] `MET-I2` Escribir casos de **direccionalidad** solo si existe una relación lógica **documentada** (al aumentar X, Y debe subir/bajar).

**🔍 Verificación**
- [ ] `MET-V1` Ejecutar los casos de invariancia y registrar resultados.
- [ ] `MET-V2` Justificar cada caso direccional: **no asumir monotonicidad** si el modelo no la garantiza (un Random Forest no la garantiza por diseño).

### 5.2 Robustez y seguridad (Adversarial Testing)

**🛠 Implementación**
- [ ] `ROB-I1` Definir el **escenario de ataque simulado** (ruido gaussiano, datos incompletos, valores extremos, entradas manipuladas).
- [ ] `ROB-I2` Definir el **umbral de tolerancia** (el Accuracy no debe caer más de X% bajo ataque; X lo define el equipo).

**🔍 Verificación**
- [ ] `ROB-V1` Ejecutar el escenario y medir la caída de desempeño.
- [ ] `ROB-V2` Confirmar que no hay **fallos catastróficos** ante pequeñas perturbaciones.

### 5.3 Regresión del modelo (Champion vs Challenger)

**🛠 Implementación**
- [ ] `REG-I1` Completar la tabla de benchmarking:

| Métrica clave | Valor actual (Prod / Champion) | Valor mínimo aceptable (Challenger) | Resultado |
|---------------|-------------------------------|--------------------------------------|-----------|
| Accuracy | _por medir_ | _por definir_ | Pasa / Falla |
| F1-Score | _por medir_ | _por definir_ | Pasa / Falla |
| Recall | _por medir_ | _por definir_ | Pasa / Falla |

**🔍 Verificación**
- [ ] `REG-V1` Comparar el modelo candidato con el vigente al reentrenar.
- [ ] `REG-V2` **No reemplazar** al Champion si el Challenger empeora métricas clave, salvo justificación aceptada.
- **Evidencia esperada:** tabla comparativa. **Criterio:** Challenger no inferior.

---

## 6. Infraestructura y operación (MLOps)

**Objetivo:** validar que la arquitectura soporta la carga y detecta degradaciones en tiempo real. *"El modelo puede ser correcto y el servicio fallar."*

### 6.1 Rendimiento del sistema (SLA)

**🛠 Implementación**
- [ ] `SLA-I1` Definir **latencia máxima de inferencia p95** (ejemplo de plantilla: < 200 ms).
- [ ] `SLA-I2` Definir el **throughput requerido** (ejemplo de plantilla: 500 req/s).
- [ ] `SLA-I3` Elegir la herramienta de **prueba de carga** (JMeter, K6, Locust).
- [ ] `SLA-I4` Marcar cada valor como **objetivo** o **medición real**.

**🔍 Verificación**
- [ ] `SLA-V1` Ejecutar la prueba de carga con usuarios concurrentes.
- [ ] `SLA-V2` Medir p95 / p99, RPS y errores bajo carga.
- [ ] `SLA-V3` Comparar medido vs objetivo y registrar Pasa / Falla.
- **Evidencia esperada:** benchmark / reporte de carga.

### 6.2 Monitoreo de desviación (Drift)

**Definiciones clave**
- **Data Drift:** cambia la distribución de las **entradas** (ej. humedad o temperatura distinta a la de desarrollo).
- **Concept Drift:** cambia la **relación entre entradas y resultado real**; requiere ground truth y reevaluación para confirmarse.
- ⚠️ *Un cambio de datos no significa automáticamente que el modelo deba reentrenarse.*

**🛠 Implementación**
- [ ] `DRF-I1` Elegir la herramienta de monitoreo (Evidently AI, SageMaker Model Monitor u otra).
- [ ] `DRF-I2` Definir la prueba estadística para Data Drift (Kolmogorov-Smirnov, distancia Jensen-Shannon).
- [ ] `DRF-I3` Definir la **frecuencia de reevaluación** con ground truth para Concept Drift.
- [ ] `DRF-I4` Implementar **logging** de inputs, outputs y ground truth (base del monitoreo; ver sección 8).

**🔍 Verificación**
- [ ] `DRF-V1` Comparar la distribución de entradas reales vs las de desarrollo.
- [ ] `DRF-V2` Cuando haya ground truth, recalcular Accuracy / Recall / F1 y detectar degradación.

### 6.3 Escalabilidad y recuperación

**🛠 Implementación**
- [ ] `ESC-I1` Definir la **política de auto-scaling** (ejemplo de plantilla: +1 nodo si CPU > 70%).
- [ ] `ESC-I2` Definir el **RTO** (tiempo máximo para restaurar el servicio tras un fallo).

**🔍 Verificación**
- [ ] `ESC-V1` Verificar que la arquitectura escala horizontalmente ante picos (Kubernetes / AWS ECS u otra).
- [ ] `ESC-V2` Simular un fallo y medir el tiempo real de recuperación frente al RTO.

---

## 7. Optimización de infraestructura

> **Documento 5.** Distingue dos significados de "optimización": del **modelo** (ajustar pesos y función de pérdida) y de la **infraestructura** (CPU/GPU/TPU, memoria, red, contenedores, capacidad, costos). Este curso se centra en la segunda. Principio: **primero medir, luego escalar**.

### 7.1 Decisiones de hardware y recursos

**🛠 Implementación**
- [ ] `OPT-I1` Definir el propósito de optimización: entrenamiento (reducir tiempos), inferencia (menor latencia), escalabilidad (crecer sin colapsar), FinOps (evitar recursos ociosos).
- [ ] `OPT-I2` Elegir hardware: CPU para modelos clásicos; GPU/TPU para deep learning. *El Random Forest tabular del proyecto puede partir en CPU.*
- [ ] `OPT-I3` Incorporar software optimizado (CUDA, TensorRT, PyTorch, TensorFlow) **solo si** corresponde al modelo y a la carga.

**🔍 Verificación**
- [ ] `OPT-V1` Identificar cuellos de botella: RAM, almacenamiento y red también los generan.
- [ ] `OPT-V2` Detectar **data starvation** (procesador ocioso esperando datos).
- [ ] `OPT-V3` Confirmar que no se está usando "el hardware más costoso" sin necesidad.

### 7.2 Contenedores, orquestación y auto-scaling

**🛠 Implementación**
- [ ] `ORQ-I1` **Contenedores:** aislar aplicación y dependencias para despliegues reproducibles.
- [ ] `ORQ-I2` **Orquestación:** mantener réplicas, recuperar fallos y distribuir carga.
- [ ] `ORQ-I3` **Auto-scaling:** aumentar o reducir capacidad según políticas y métricas.

**🔍 Verificación**
- [ ] `ORQ-V1` Evaluar beneficios (rendimiento, eficiencia, estabilidad, menor costo operativo) contra costos (complejidad, inversión, obsolescencia, **vendor lock-in**).
- [ ] `ORQ-V2` Confirmar que se mide **antes** de escalar: escalar sin medir puede subir el costo sin resolver el problema.

---

## 8. Monitoreo: Prometheus + Grafana

> **Documento 5.** Monitorear no es solo mirar CPU: hay que **correlacionar modelo, servicio e infraestructura**. Flujo: **Aplicación IA (/metrics) → Prometheus (scrape + series temporales) → Grafana (dashboards y alertas)**. Grafana visualiza, Prometheus recolecta, la aplicación debe exponer las métricas.

### 8.1 Las tres capas de monitoreo

| Capa | Qué observar |
|------|--------------|
| **Modelo** | Accuracy / Recall / F1 (si hay ground truth), Data Drift, Concept Drift, distribución de entradas |
| **Servicio** | Latencia, Throughput / RPS, tasa de errores, disponibilidad |
| **Infraestructura** | CPU/GPU/TPU, RAM/VRAM, disco y red, contenedores y réplicas |

### 8.2 Logging (base del monitoreo del modelo)

**🛠 Implementación**
- [ ] `LOG-I1` Registrar **Inputs** (temperatura, humedad, viento, vegetación, historial).
- [ ] `LOG-I2` Registrar **Outputs** (riesgo Bajo / Medio / Alto).
- [ ] `LOG-I3` Registrar **Ground Truth** cuando se observe el resultado real.

**🔍 Verificación**
- [ ] `LOG-V1` Confirmar que los registros permiten reevaluar la calidad y detectar cambios.

### 8.3 Métricas del proyecto de incendios

**🛠 Implementación**
- [ ] `MET-I3` Exponer en `/metrics`:
  - [ ] **Demanda:** solicitudes totales, RPS / throughput, errores HTTP, disponibilidad.
  - [ ] **Latencia:** p50 / p95 de inferencia.
  - [ ] **Recursos:** CPU y RAM del proceso/contenedor; disco/red si son cuello de botella.
  - [ ] **Modelo y datos:** conteo Bajo/Medio/Alto, variables de entrada, señales de drift, calidad cuando haya ground truth.
- [ ] `MET-I4` Priorizar CPU, RAM, latencia, RPS, errores y acceso a datos (PostGIS). **No** priorizar GPU/VRAM si no existe GPU.

**🔍 Verificación (cómo interpretar)**
- [ ] `MET-V3` Si suben los RPS y la latencia se mantiene estable → el servicio absorbe la carga.
- [ ] `MET-V4` Si **p95 aumenta sin más RPS** → investigar código, PostGIS, red, memoria o dependencias.
- [ ] `MET-V5` El dashboard debe **generar preguntas técnicas**, no solo mostrar números.

### 8.4 Alertas y respuesta

**🛠 Implementación**
- [ ] `ALR-I1` Definir **umbrales y tendencias sostenidas** (ejemplo: p95 de latencia sobre un límite durante varios minutos).
- [ ] `ALR-I2` Documentar las acciones de respuesta:
  - [ ] Investigar causa.
  - [ ] Escalar si corresponde.
  - [ ] Corregir datos o código.
  - [ ] **Reentrenar solo con evidencia.**

**🔍 Verificación**
- [ ] `ALR-V1` Con ground truth: evaluar Accuracy, Recall y F1.
- [ ] `ALR-V2` Sin ground truth inmediato: evaluar distribución, confianza, errores, latencia y uso del servicio.
- [ ] `ALR-V3` Confirmar que cada alerta conduce a una **acción técnica** concreta.

---

## 9. Stack tecnológico de pruebas

**Idea:** no existe un único "framework maestro"; se combinan herramientas por área y se **justifica técnicamente** cada elección.

**🛠 Implementación** (completar la tabla con herramientas realmente usadas o propuestas)

| Área de prueba | Opciones mencionadas en las fuentes | Herramienta seleccionada | Justificación técnica |
|----------------|-------------------------------------|--------------------------|-----------------------|
| Validación de datos | Great Expectations, TF Data Validation | _por definir_ | _por definir_ |
| QA del modelo | Deepchecks, Giskard | _por definir_ | _por definir_ |
| Drift monitoring | Evidently AI, Model Monitoring (SageMaker) | _por definir_ | _por definir_ |
| Código / API | PyTest, pruebas unitarias | _por definir_ | _por definir_ |
| Carga | JMeter, K6, Locust | _por definir_ | _por definir_ |
| Orquestación | Airflow, Kubeflow | _por definir_ | _por definir_ |
| Observabilidad | Prometheus, Grafana, Docker Compose | _por definir_ | _por definir_ |
| Arquitectura | AWS Well-Architected (ML Lens), guías cloud | _por definir_ | _por definir_ |

- [ ] `STK-I1` Completar la tabla sin inventar herramientas que no se usarán.
- [ ] `STK-I2` Evaluar la arquitectura contra los 5 pilares del **AWS Well-Architected ML Lens**: excelencia operativa, seguridad, fiabilidad, eficiencia de rendimiento y optimización de costos.

**🔍 Verificación**
- [ ] `STK-V1` Confirmar que cada herramienta elegida cubre al menos una prueba definida en las secciones 4 a 6.

---

## 10. Continuous Testing (CT) en MLOps

**Qué dice la fuente:** cada cambio importante (código nuevo o modelo reentrenado) debe activar pruebas automáticas, para reducir el riesgo de llevar a producción cambios que deterioren datos, modelo o arquitectura. Previene la **degradación silenciosa** (el sistema "funciona" pero decide mal).

**Flujo:** Cambio → Validaciones de datos → Robustez + benchmark del modelo → Carga + recuperación.

**🛠 Implementación**
- [ ] `CT-I1` Definir el disparador (commit de código o reentrenamiento).
- [ ] `CT-I2` Automatizar la etapa de datos (por ejemplo, Great Expectations).
- [ ] `CT-I3` Automatizar la etapa de modelo (robustez + benchmark, por ejemplo Deepchecks o Giskard).
- [ ] `CT-I4` Automatizar la etapa de infraestructura (carga + recuperación).

**🔍 Verificación**
- [ ] `CT-V1` Confirmar que un cambio que degrada datos, modelo o arquitectura **bloquea** el paso a producción.

---

## 11. Aprobación final Go / No-Go

**Qué dice la fuente:** la decisión final se toma con la **evidencia disponible** y queda firmada por un responsable.

**🛠 Implementación**
- [ ] `GNG-I1` Completar el historial de versiones del documento (fecha, versión, autor, descripción).
- [ ] `GNG-I2` Armar la **matriz de pruebas** (prueba, criterio, evidencia, resultado):

| Prueba | Criterio | Evidencia | Resultado |
|--------|----------|-----------|-----------|
| Esquema de datos | Campos requeridos presentes | Reporte de validación | Pasa / Falla |
| Latencia p95 | Objetivo definido por el equipo | Benchmark / carga | Medido vs objetivo |
| Regresión del modelo | Challenger no inferior | Tabla comparativa | Go / No-Go |

**🔍 Verificación (casillas de aprobación)**
- [ ] `GNG-V1` **Validación de Datos:** APROBADO
- [ ] `GNG-V2` **Evaluación de Modelo:** APROBADO
- [ ] `GNG-V3` **Pruebas de Infraestructura:** APROBADO
- [ ] `GNG-V4` **Decisión de despliegue:** APROBADO PARA PRODUCCIÓN / RECHAZADO
- [ ] `GNG-V5` **Firma del responsable.**

---

## 12. Actividad práctica de la Sesión 4.2

> Prometheus + Grafana aplicados al servicio de riesgo. La prueba de retardo es **didáctica**: crea una degradación controlada para aprender a leer métricas, no simula una emergencia real.

**🛠 Implementación**
- [ ] `ACT-I1` **Levantar:** usar Docker Compose para iniciar API, Prometheus y Grafana.
- [ ] `ACT-I2` **Programar:** completar el contador `high_risk_predictions_total` para que aumente cuando el nivel de riesgo sea **Alto**.
- [ ] `ACT-I3` **Generar carga:** enviar solicitudes normales y una **latencia artificial controlada**.

**🔍 Verificación**
- [ ] `ACT-V1` **Interpretar:** analizar RPS, p95, memoria, errores y distribución de riesgo.
- [ ] `ACT-V2` Explicar por escrito **qué decisión tomarían** a partir de lo observado (¿escalar?, ¿investigar código/PostGIS?, ¿reentrenar?).

---

## 13. Matriz de trazabilidad y orden sugerido

### 13.1 Orden de ejecución recomendado

| Etapa | Qué hacer | Secciones |
|-------|-----------|-----------|
| 1 | Gobierno y amenazas (definir el marco antes de construir) | 1 |
| 2 | Datos: validación, sesgos, fugas, procedencia, privacidad | 2.1, 2.2, 4 |
| 3 | Modelo: metamórficas, robustez, regresión | 2.3, 5 |
| 4 | Infraestructura: contenedores, SLA, escalabilidad, optimización | 3.2, 6, 7 |
| 5 | Observabilidad: logging, métricas, dashboards, alertas, drift | 3.3, 6.2, 8, 12 |
| 6 | Automatización (CT) y respuesta a incidentes | 3.4, 10 |
| 7 | Stack documentado y decisión Go / No-Go | 9, 11 |

### 13.2 Conteo de tareas por área

| Área | 🛠 Implementación | 🔍 Verificación |
|------|------------------|-----------------|
| Gobierno y diseño (1) | 9 | 6 |
| Datos y modelo seguros (2) | 8 | 4 |
| Operación segura (3) | 8 | 6 |
| Pruebas de datos (4) | 8 | 10 |
| Modelo (5) | 5 | 6 |
| MLOps (6) | 10 | 7 |
| Optimización (7) | 6 | 5 |
| Monitoreo (8) | 7 | 7 |
| Stack, CT, Go/No-Go (9–11) | 8 | 7 |
| Actividad práctica (12) | 3 | 2 |
| **Total** | **72** | **60** |

### 13.3 Qué prueba cubre qué riesgo

| Riesgo | Prueba o control que lo cubre |
|--------|-------------------------------|
| Datos corruptos o fuera de rango | Esquema y tipología (4.1) |
| Discriminación o subrepresentación | Bias testing (4.2) |
| Métricas infladas artificialmente | Data leakage (4.3) |
| Modelo frágil ante ruido o ataque | Robustez adversaria (5.2), entrenamiento adversario (2.3) |
| Un modelo nuevo peor que el actual | Regresión Champion vs Challenger (5.3) |
| Servicio lento o caído | SLA y carga (6.1), escalabilidad y RTO (6.3) |
| Degradación silenciosa | Drift (6.2), monitoreo (8), CT (10) |
| Envenenamiento de datos | Data Provenance (2.1) |
| Modelo comprometido | Red Teaming y rollback (3.4), sandboxing (3.2) |
| Costos innecesarios | Optimización y FinOps (7) |

---

*Documento generado a partir de las cinco fuentes cargadas. Los valores numéricos son ejemplos de las plantillas; deben ser reemplazados por los objetivos y mediciones reales del proyecto.*
