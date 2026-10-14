# Dimensionamiento Estratégico de Infraestructura de Inteligencia Artificial (IA)

FireWatch: plataforma de predicción de incendios forestales

FECHA: 4 de octubre de 2026

Tabla de contenidos

# Historial de versiones

| Fecha | Versión | Autor | Descripción |
| --- | --- | --- | --- |
| 4/10/26 | 0.2 | Roberto Armijo, Miguel Calderón y Ricardo Saavedra | Ajuste a producción empresarial 24/7 y actualización cada 15 minutos |
|  |  |  |  |
|  |  |  |  |
|  |  |  |  |

# 1. Definición Estratégica y Alcance del Proyecto

Esta sección establece los objetivos de negocio, el caso de uso y las métricas de rendimiento críticas que regirán todas las decisiones de dimensionamiento, mitigando el error de implementar soluciones costosas sin valor tangible.

## 1.1 Caso de Uso Principal

Descripción: Inferencia en producción para la consulta del mapa de riesgo geográfico y entrenamiento periódico en segundo plano.

Datos requeridos: Conjuntos de datos tabulares con antecedentes históricos de incendios, información meteorológica y características territoriales.

## 1.2 Modelo de IA y Tamaño

Modelo: Random Forest o Regresión Logística.

Tamaño: Modelo clásico de Machine Learning para datos tabulares. El número de parámetros es mínimo en comparación con modelos de Deep Learning, requiriendo un consumo muy bajo de memoria.

## 1.3 Requisitos de Latencia (Inferencia)

Latencia Objetivo (Modelo): El endpoint de consulta de estimaciones debe responder muy rápido para mantener la fluidez del mapa interactivo web.

Rango aceptado: Menos de 500 ms en el percentil 95 bajo carga simulada (meta inicial máxima de 2 segundos para las consultas del mapa en condiciones normales).

## 1.4 Requisitos de Capacidad (Throughput)

Volumen de solicitudes: Escenario inicial de prueba con un volumen moderado.

Rango aceptado: 4 solicitudes por segundo (RPS) en el pico, basándose en un escenario de 20 usuarios concurrentes realizando una consulta cada cinco segundos.

## 1.5 Tipo de Despliegue

Descripción: Despliegue de producción empresarial 24/7, con redundancia de aplicación, base de datos administrada, respaldos, observabilidad y actualización automática de datos cada 15 minutos.

Detalle: Nube pública, con Oracle Cloud como proveedor principal y servicios pagados dimensionados para operación continua. Las cuotas gratuitas quedan limitadas a desarrollo y pruebas.

# 2. Dimensionamiento del Cómputo: Memoria y Potencia (GPU/TPU)

Nota de adaptación: Al basarse en datos tabulares, el proyecto no requiere Deep Learning ni procesamiento intensivo en GPU. El dimensionamiento se realiza sobre la CPU y la memoria RAM del sistema.

## 2.1 Requerimiento de memoria y ausencia de GPU/TPU

Precisión: formato tabular estándar de scikit-learn, procesado sobre CPU.

La memoria se dimensiona para los servicios de producción y no para un modelo de lenguaje.

- RAM para el servicio de predicción: 4 GB por réplica, incluyendo el modelo aprobado y el runtime ONNX.

- RAM para cada nodo API/Web: 8 GB, con dos réplicas detrás del balanceador.

- RAM para el trabajador Python: 16 GB y 4 vCPU, para ingesta, preparación, inferencia por lote y entrenamiento programado.

Requisito de hardware: CPU de propósito general, almacenamiento SSD y al menos dos réplicas de aplicación. No se requieren GPU, TPU ni VRAM para el modelo tabular base.

| Formato | Bytes por valor | Uso en FireWatch | Aplicación |
| --- | --- | --- | --- |
| FP32 | 4 Bytes | Sí | Entrenamiento e inferencia del modelo tabular en CPU. |
| FP16 / BF16 | 2 Bytes | No en el modelo base | Alternativa para modelos neuronales; no requerida en esta etapa. |

## 2.2 Selección de CPU y capacidad de procesamiento

Procesador seleccionado: CPU de propósito general en Oracle Cloud, distribuida entre dos nodos API/Web, un worker Python y dos réplicas de predicción.

Capacidad inicial: 2–4 vCPU por nodo API/Web, 4 vCPU para el worker y 2 vCPU por réplica de predicción. La asignación se validará con pruebas de carga.

Justificación: separar las consultas de los procesos de ingesta, preparación e inferencia evita que la actualización de datos cada 15 minutos degrade la respuesta del mapa. Las réplicas permiten mantenimiento y recuperación sin detener completamente el servicio.

Costo de cómputo: forma parte del rango de producción empresarial de USD 1.200–1.800 mensuales. No se presenta como el costo total aislado, porque el escenario incluye base de datos administrada, almacenamiento, seguridad, observabilidad, respaldos y operación 24/7.

# 3. Optimización de Rendimiento y Latencia (Software Sizing)

La optimización permite reducir la necesidad de ampliar constantemente la infraestructura (CapEx/OpEx) y asegurar que el sistema cumpla con el objetivo de latencia estricto (menor a 500 ms) en el mapa web.

A continuación se detallan las estrategias de optimización aplicadas al proyecto FireWatch:

| Estrategia | Descripción | Impacto en dimensionamiento |
| --- | --- | --- |
| Inferencia ONNX | Exportar el modelo tabular de scikit-learn a ONNX y ejecutarlo en Go. | Reduce procesos Python residentes y consumo de memoria en las consultas. |
| Pre-cálculo transaccional | Generar un lote de riesgo cada 15 minutos y publicarlo de forma atómica. | La API lee datos indexados y mantiene baja latencia en el mapa. |
| Worker asíncrono | Separar ingesta, preparación y entrenamiento de la atención a usuarios. | Evita contención de CPU y mantiene disponible la capa de consulta. |

# 4. Dimensionamiento de Almacenamiento, Red e I/O

El rendimiento del I/O y la red son fundamentales para la ingesta rápida de múltiples fuentes de datos externas y para garantizar la entrega de estimaciones en el mapa web sin latencia perceptible. Adaptado a un entorno comercial real (dejando atrás las limitaciones de la capa gratuita), el dimensionamiento es el siguiente:

- Almacenamiento de Datos Activos (I/O - Base de Datos)

- Tecnología: Oracle Autonomous Database administrada, con red privada, respaldos y escalamiento según el volumen de histórico y auditoría.

- Prioridad: Almacenamiento basado en discos de estado sólido (SSD/NVMe) para asegurar muy baja latencia en las consultas espaciales, resultados históricos y estados de auditoría.

- Capacidad: 50 a 100 GB iniciales (escalable automáticamente). Esto otorga suficiente margen frente a los 5 GB originales para almacenar el crecimiento del historial de incendios, los datos meteorológicos y las predicciones generadas.

- Almacenamiento de Archivos y Modelos (Object Storage)

- Tecnología: Oracle Object Storage para fuentes originales, conjuntos de entrenamiento, modelos aprobados y exportaciones cifradas.

- Prioridad: Conservar de forma segura las fuentes originales descargadas, los conjuntos de entrenamiento versionados, los modelos aprobados (.onnx) y las exportaciones lógicas de respaldo.

- Capacidad: 50 GB (escalable según el volumen de métricas e histórico guardado).

- Interconexión Intra-Servidor (I/O Local)

- Tecnología: Disco de bloque (Block Storage) atachado a la máquina virtual y red interna de contenedores.

- Prioridad: Un volumen SSD de 50 a 100 GB para el sistema operativo, las imágenes de Docker y el almacenamiento temporal durante la preparación intensiva de datos (extracción y transformación). La comunicación entre la API Go y el trabajador Python utilizará la red interna de contenedores, garantizando una latencia casi nula.

- Red para Gestión y Clientes

- Tecnología: red privada de Oracle Cloud con balanceador, WAF, TLS y reglas de seguridad; solo la entrada web queda expuesta públicamente.

- Prioridad: Manejar el tráfico web de clientes mediante HTTPS (puerto 443 vía Caddy) y mantener un flujo constante y seguro de conexiones (un pool controlado de conexiones) hacia la base de datos gestionada externa.

# 5. Análisis de Costo Total de Propiedad (TCO) y MLOps

Al optar por un modelo de nube pública comercial en lugar de un centro de datos propio, el análisis económico desplaza la inversión desde los gastos de capital hacia los costos operativos recurrentes. Basado en las proyecciones de escalabilidad del sistema, el modelo de costos se estructura en las siguientes fases:

- Gastos de Capital (CapEx)

- Hardware e Infraestructura del Sitio: $0 USD. Al desplegarse en una nube pública, no hay inversión inicial en compra de servidores físicos, racks ni refrigeración.

- Gastos Operativos (OpEx) El costo mensual estimado evoluciona directamente según el nivel de servicio, la disponibilidad y los requerimientos de la infraestructura:

- Prototipo académico: Para demostración, pocos usuarios y sin SLA. Su costo es de USD 0–20 mensuales.

- Piloto empresarial: referencia intermedia para pruebas internas, con un costo de USD 100–300 mensuales. No corresponde al escenario seleccionado para FireWatch.

- Producción empresarial: escenario seleccionado. Contempla servicio 24/7, base de datos administrada, redundancia, respaldos, seguridad, observabilidad y actualización automática cada 15 minutos. Su costo referencial es de USD 1.200–1.800 mensuales.

- Servicio crítico: Exige alta disponibilidad, múltiples servidores, soporte y el uso de fuentes de datos pagadas. Su costo es de USD 2.000–4.000 o más mensuales.

## 5.1 Estrategia de Optimización del TCO (Eficiencia Energética y Diseño):

- ¿El diseño incluye MLOps para permitir el escalado y evitar el sobre-aprovisionamiento estático? El diseño contempla GitHub Actions para pruebas y despliegues y un scheduler que registre cada lote, el modelo utilizado y sus métricas. Docker Compose se conserva para desarrollo; Kubernetes será el orquestador de despliegue, con servicios de datos redundantes y administrados. Estas capacidades deberán implementarse y validarse progresivamente.

- ¿La arquitectura está diseñada para ser escalable y elástica? Sí. La API y la predicción pueden replicarse horizontalmente; el worker ejecuta el ciclo de 15 minutos de forma asíncrona y la base de datos y el almacenamiento se amplían según el histórico.

Validación Final:

- Benchmarking: se deben ejecutar pruebas simulando al menos 20 usuarios concurrentes y 4 RPS como línea base, además de una prueba de recuperación y una prueba de actualización cada 15 minutos. El objetivo es validar <500 ms en el percentil 95 para consultas del mapa.

- Seguridad: La infraestructura garantizará cifrado (HTTPS), control de roles (viewer/admin) y cumplimiento normativo, reforzando la redundancia y las políticas de retención de datos en la medida que se avance hacia los modelos de producción más robustos.
