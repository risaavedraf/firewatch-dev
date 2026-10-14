# Arquitectura de un sistema de predicción de incendios forestales

Región de Valparaíso, Chile

Informe técnico de arquitectura basado en arc42

| Asignatura | ITY1102 Arquitectura de sistemas IA |
| --- | --- |
| Estado | Informe técnico actualizado para producción empresarial |
| Alcance | Sistema web con roles viewer y admin, mapa y modelo predictivo |
| Fecha | 4 de octubre de 2026 |
| Integrantes | Roberto Armijo; Miguel Calderón; Ricardo Saavedra |

1. Introducción y objetivos

## 1.1. Visión general de los requisitos

El proyecto propone FireWatch, una plataforma de inteligencia artificial para estimar el riesgo de ocurrencia de incendios forestales en la Región de Valparaíso, Chile. Para ello, se busca integrar antecedentes históricos de incendios, información meteorológica y características territoriales, con el propósito de identificar zonas que presenten una mayor probabilidad estimada.

El sistema deberá permitir consultar la información mediante una aplicación web con un mapa interactivo. El usuario podrá seleccionar una zona y un período, observar la estimación de riesgo, revisar su vigencia y conocer el estado de los datos utilizados. El administrador contará con funciones para gestionar datos, revisar evaluaciones y controlar las versiones del modelo y revisar prioridades preventivas.

Objetivo general. Diseñar una arquitectura de sistema IA que integre información histórica, meteorológica y territorial para generar y visualizar estimaciones de riesgo y prioridades preventivas asociadas a incendios forestales, considerando seguridad, confiabilidad, observabilidad y recursos económicos limitados.

Alcance de uso. FireWatch entrega estimaciones orientativas para apoyar la prevención y la revisión territorial. No predice con certeza el punto, la fecha ni la hora de ignición, no emite alertas oficiales y no reemplaza las funciones de CONAF, SENAPRED, Bomberos ni de la autoridad municipal.

Los requisitos funcionales iniciales son los siguientes:

- Incorporar, validar y relacionar información proveniente de distintas fuentes mediante fecha y ubicación geográfica.

- Preparar los datos y entrenar y evaluar modelos de clasificación o estimación de riesgo.

- Generar y almacenar resultados asociados a una fecha de emisión, período de aplicación y versión del modelo.

- Permitir al viewer consultar el mapa de riesgo, el historial y el estado de actualización de los datos.

- Permitir al admin gestionar datos, revisar métricas, solicitar entrenamientos y aprobar versiones del modelo.

- Registrar las operaciones relevantes para conocer quién realizó un cambio y cuándo se efectuó.

- 1.1.1. Requisitos funcionales

RF-01. El sistema deberá incorporar información histórica de incendios, datos meteorológicos y características territoriales de la Región de Valparaíso.

RF-02. El sistema deberá validar los datos recibidos, revisando formatos, fechas, coordenadas, duplicados y valores faltantes antes de utilizarlos.

RF-03. El sistema deberá preparar los datos y ejecutar procesos de entrenamiento y evaluación de modelos de aprendizaje automático.

RF-04. El sistema deberá generar una estimación del riesgo o probabilidad de ocurrencia de incendios para una zona y período determinados.

RF-05. El usuario con rol viewer deberá consultar el mapa de la Región de Valparaíso, visualizar el nivel de riesgo, revisar la fecha de actualización y consultar el historial disponible.

RF-06. El usuario con rol admin deberá incorporar nuevos datos, registrar correcciones y revisar el estado de los procesos de carga y preparación.

RF-07. El usuario con rol admin deberá consultar las métricas de evaluación, revisar las versiones candidatas y aprobar o revertir una versión del modelo antes de utilizarla en producción.

RF-08. El sistema deberá almacenar las predicciones junto con la fecha de generación, el período de aplicación, la zona evaluada, los datos utilizados y la versión del modelo.

RF-09. El sistema deberá registrar las operaciones administrativas relevantes, identificando al usuario responsable, la acción realizada y la fecha correspondiente.

RF-10. El sistema deberá mostrar, cuando estén disponibles, los factores principales, la fuente, la fecha de emisión, la vigencia, la cobertura, la versión del modelo y el estado de incertidumbre asociado a cada estimación.

RF-11. El usuario con rol admin podrá registrar una acción preventiva orientativa vinculada a un sector, indicando prioridad, responsable, plazo y estado.

RF-12. El usuario con rol admin podrá adjuntar o referenciar evidencia, validar su suficiencia según un criterio definido y solicitar una reevaluación del riesgo residual.

RF-13. El sistema deberá mantener separados los incendios observados utilizados como referencia y las estimaciones generadas por el modelo, para evitar confundir una predicción con un hecho registrado.

- 1.1.2. Requisitos no funcionales

RNF-01. Seguridad y control de acceso. El sistema deberá autenticar a los usuarios y aplicar autorización basada en roles. El viewer tendrá acceso únicamente a consultas y visualizaciones, mientras que el admin podrá ejecutar funciones de gestión y administración.

RNF-02. Protección de la información. Las comunicaciones deberán realizarse mediante HTTPS. Las credenciales, claves y configuraciones sensibles deberán mantenerse fuera del código fuente y almacenarse mediante mecanismos protegidos.

RNF-03. Rendimiento. Como meta inicial para el prototipo, las consultas del mapa deberán responder en menos de dos segundos en condiciones normales. Este valor deberá validarse mediante pruebas de rendimiento.

RNF-04. Disponibilidad y continuidad. El sistema deberá mantener disponibles las últimas estimaciones válidas cuando una actualización falle y operar con una disponibilidad objetivo de 99,5% mensual, respaldada por redundancia de aplicación, base de datos administrada y procedimientos de recuperación.

RNF-05. Escalabilidad. La solución deberá permitir aumentar la capacidad de procesamiento y separar los componentes de consulta, preparación de datos y predicción. El dimensionamiento inicial se validará con una carga base de 20 usuarios concurrentes y aproximadamente 4 solicitudes por segundo, dejando capacidad para agregar réplicas.

RNF-06. Confiabilidad de los resultados. El sistema no deberá publicar estimaciones generadas con datos incompletos, inválidos o insuficientes. Cada resultado deberá indicar su fecha de generación, vigencia y versión del modelo utilizada.

RNF-07. Observabilidad y trazabilidad. El sistema deberá registrar errores, tiempos de respuesta, ejecuciones de procesos, actualizaciones de datos, versiones de modelos y acciones administrativas, sin almacenar contraseñas ni tokens.

RNF-08. Flexibilidad y mantenibilidad. La arquitectura deberá permitir incorporar nuevas fuentes de información, modificar el modelo o actualizar el procesamiento sin tener que reconstruir completamente la aplicación.

RNF-09. Usabilidad y comprensión. El mapa deberá utilizar una leyenda clara para representar los niveles de riesgo. La interfaz deberá diferenciar entre información vigente, desactualizada e información insuficiente.

RNF-10. Actualización y vigencia. Un proceso automático deberá iniciar cada 15 minutos la ingesta, validación, inferencia y publicación de un nuevo lote cuando las fuentes estén disponibles. Esta frecuencia no implica reentrenar el modelo en cada ciclo; el reentrenamiento será una tarea separada, programada o solicitada por un admin.

Los valores de rendimiento, disponibilidad y cantidad de usuarios corresponden a metas iniciales de diseño. Deberán confirmarse mediante pruebas y podrán ajustarse después de revisar los datos disponibles y la infraestructura seleccionada.

## 1.2. Objetivos de calidad

| Objetivo | Aplicación en el sistema |
| --- | --- |
| Seguridad | Separar los roles viewer y admin, proteger credenciales y restringir el acceso a datos y funciones administrativas. |
| Confiabilidad | Evitar publicar lotes incompletos y señalar los resultados desactualizados o sin información suficiente. |
| Observabilidad | Registrar errores, ejecuciones, última actualización, versiones del modelo y consumo de recursos. |
| Flexibilidad | Permitir incorporar nuevas fuentes o reemplazar el modelo sin modificar toda la aplicación. |
| Escalabilidad | Separar las consultas de los procesos pesados y permitir aumentar recursos o distribuir componentes posteriormente. |

Las métricas y criterios de aceptación se concretarán en la sección 10. Por ahora, la resolución geográfica, el horizonte de predicción y la frecuencia de actualización se consideran decisiones pendientes de validar con los datos disponibles.

Para la operación se utilizará una meta de frescura inferior a 15 minutos para las estimaciones, condicionada por la periodicidad y disponibilidad real de cada fuente. La frecuencia de reentrenamiento se evaluará por separado y no se asumirá como una actualización continua.

## 1.3. Partes interesadas

| Parte interesada | Necesidad principal |
| --- | --- |
| Viewer | Consultar el mapa, las estimaciones, su vigencia y el historial de resultados. |
| Admin | Gestionar datos, ejecutar procesos, revisar métricas y controlar versiones del modelo. |
| Equipo de desarrollo | Diseñar, implementar, probar y mantener la solución con recursos limitados. |
| Docente y evaluadores | Revisar la coherencia de la arquitectura, sus decisiones y el cumplimiento de la pauta. |
| Responsable de acción | En el MVP, su asignación y la revisión de evidencia son gestionadas por el admin; no constituye un rol institucional habilitado. |

# 2. Restricciones

La propuesta está condicionada por el territorio de estudio, la operación continua 24/7, el presupuesto operacional controlado, el tamaño del equipo y el plazo de la evaluación. Estas restricciones influyen en la complejidad del diseño y en la selección de tecnologías.

## 2.1. Restricciones del proyecto

| ID | Restricción | Implicancia |
| --- | --- | --- |
| RES01 | Territorio: Región de Valparaíso | Las fuentes, coordenadas, zonas y visualizaciones deben corresponder al territorio estudiado. |
| RES02 | Presupuesto operacional controlado | El escenario seleccionado es producción empresarial, con un costo referencial de USD 1.200–1.800 mensuales. Las cuotas gratuitas se reservan para desarrollo y pruebas. |
| RES03 | Equipo de 2 a 3 estudiantes | La solución debe ser posible de construir, probar y documentar, aunque la operación productiva utilice servicios administrados. |
| RES04 | Plazo de la evaluación | Se prioriza un núcleo funcional, automatización y decisiones justificadas; la expansión a servicio crítico queda como evolución futura. |
| RES05 | Disponibilidad y calidad de datos | La arquitectura debe registrar procedencia, cobertura, formato y valores faltantes antes de entrenar. |

## 2.2. Condiciones técnicas de partida

- El sistema parte con datos disponibles; no considera instalar sensores, cámaras ni equipamiento de captura propio.

- La infraestructura se dimensionará inicialmente para CPU. La incorporación de GPU o TPU se justificará solo si las mediciones muestran un beneficio claro.

- Las fuentes deberán registrarse con su origen, período, cobertura, licencia y condiciones de uso.

- El alcance inicial prioriza la aplicación web. Una aplicación Android puede quedar como evolución posterior si el equipo y la evaluación la requieren.

- La elección final de fuente de datos, resolución territorial y período de predicción quedará sujeta a la inspección de los archivos disponibles.

# 3. Alcance y contexto del sistema

El sistema incluirá la incorporación y preparación de datos, el entrenamiento y la evaluación de modelos, la generación de estimaciones y una interfaz web para consultar resultados en un mapa. La aplicación se enfocará en la Región de Valparaíso y mostrará el período al que corresponde cada estimación.

## 3.1. Contexto de negocio

| Actor o sistema externo | Entrega o solicita | Recibe |
| --- | --- | --- |
| Viewer | Credenciales, zona y período de consulta | Mapa, estimación, vigencia, historial y estado de datos |
| Admin | Datos, correcciones, configuración y solicitudes de entrenamiento | Validaciones, métricas, estados de trabajos y auditoría |
| Fuente histórica de incendios | Registros geográficos y temporales disponibles | Consultas o descargas según mecanismos autorizados |
| Fuente meteorológica | Variables meteorológicas por fecha y ubicación | Consultas o descargas según disponibilidad |
| Proveedor cartográfico | Capas base para la visualización | Solicitudes de mapa desde la aplicación |

Como fuentes candidatas se consideran los registros georreferenciados de incendios de CONAF, la información meteorológica de la Dirección Meteorológica de Chile y un proveedor de cartografía compatible con la aplicación. Antes de incorporarlas se verificará cobertura, formato, periodicidad, licencia y nivel de detalle.

Para el inventario inicial se evaluarán los registros de CONAF publicados mediante Datos para Resiliencia, productos meteorológicos de la Dirección Meteorológica de Chile y, como respaldo reproducible, NASA POWER. La cobertura, resolución, licencia, calidad, zona horaria y fecha de disponibilidad deberán comprobarse antes de entrenar; una fuente oficial no equivale automáticamente a un conjunto listo para modelar.

## 3.2. Límites y responsabilidades

Dentro del sistema se consideran las siguientes responsabilidades:

- Validar, preparar y relacionar información histórica, meteorológica y territorial.

- Entrenar, evaluar y versionar modelos de estimación de riesgo.

- Generar y almacenar resultados junto con fecha, vigencia y versión del modelo.

- Administrar usuarios, permisos y operaciones de auditoría.

- Consultar estimaciones e historial mediante una interfaz con mapa.

- Registrar errores, ejecuciones, actualizaciones y cambios de configuración.

Quedan fuera del alcance inicial la instalación de sensores o cámaras, la detección de incendios mediante video, la simulación de propagación del fuego, la coordinación de evacuaciones y la asignación de recursos de emergencia. Los incendios observados se mantendrán separados de las predicciones para evaluar posteriormente el desempeño del modelo.

Ciclo preventivo propuesto. La plataforma puede organizar la información como datos → riesgo orientativo → exposición → acción preventiva → responsable y plazo → evidencia → riesgo residual. Esta cadena apoya la revisión humana, pero el cierre administrativo de una acción no demuestra por sí mismo que el riesgo haya disminuido.

## 3.3. Contexto técnico

| Interacción | Decisión inicial |
| --- | --- |
| Navegador - API | HTTPS y REST con respuestas estructuradas, autenticación y autorización. |
| API - datos | Acceso controlado a la base de datos y a los metadatos de archivos. |
| Procesos - fuentes externas | Descarga o consulta mediante mecanismos autorizados y registro de procedencia. |
| Interfaz - cartografía | MapLibre como librería de visualización y proveedor de mapas desacoplado. |

# 4. Estrategia de solución

Se propone una arquitectura que separe la interfaz web, la lógica de aplicación y el procesamiento de datos y modelos. Esta organización permitirá desarrollar el prototipo con recursos limitados y modificar sus componentes conforme se validen los datos y las necesidades del proyecto.

## 4.1. Organización de la solución

| Componente | Tecnología propuesta | Responsabilidad |
| --- | --- | --- |
| Interfaz web | React y TypeScript | Presentar el mapa, filtros, historial y funciones administrativas. |
| Visualización geográfica | MapLibre GL JS | Representar las zonas y estimaciones; el proveedor de cartografía se seleccionará por licencia y límites de uso. |
| API de aplicación | Go y Gin | Gestionar autenticación, autorización, consultas y operaciones administrativas mediante REST sobre HTTPS. |
| Procesamiento y ML | Python y scikit-learn | Integrar datos, construir variables, entrenar modelos y evaluar resultados. |
| Servicio de predicción | Componente independiente | Ejecutar el modelo aprobado y generar estimaciones con trazabilidad. |
| Gestión preventiva | Módulo de administración | Mantener acciones orientativas, evidencia, responsables, plazos, estados y reevaluaciones del riesgo residual. |
| Persistencia | Oracle Autonomous Database y Object Storage | Mantener datos estructurados, predicciones, auditoría, conjuntos de datos y modelos. |

Para el prototipo, la autenticación y la administración de usuarios se organizarán como módulos de la API. El procesamiento del modelo tendrá un componente separado para poder actualizarlo y controlar sus recursos de manera independiente.

## 4.2. Estrategia de datos y aprendizaje automático

El modelo buscará estimar la ocurrencia de incendios forestales en una zona durante un período futuro. La resolución geográfica y el horizonte de predicción se establecerán después de comprobar el detalle y la cobertura de las fuentes.

Se cruzarán antecedentes de incendios observados, información meteorológica y características territoriales mediante ubicación y fecha. Cada conjunto conservará su procedencia y versión. La ausencia de un registro de incendio deberá revisarse antes de interpretarse como evidencia de que no ocurrió un evento.

Se propone comparar una regresión logística como modelo de referencia con un Random Forest. La elección final dependerá de la evaluación, considerando precisión, sensibilidad y falsas alertas. La validación respetará el orden temporal y utilizará únicamente información disponible al momento de emitir cada estimación.

Para el servicio de producción, la meta de frescura de las estimaciones será inferior a 15 minutos cuando las fuentes estén disponibles y el procesamiento termine correctamente. La frecuencia de reentrenamiento se evaluará por separado y no se ejecutará en cada ciclo de actualización.

La inferencia en Go mediante ONNX se mantiene como alternativa del planteamiento inicial. Su adopción requerirá comprobar la compatibilidad del modelo y del preprocesamiento y verificar que la conversión conserve los resultados. Si no se cumple esa condición, el servicio podrá ejecutar el modelo en Python manteniendo el mismo contrato de comunicación.

## 4.3. Estrategia de infraestructura y despliegue

Para producción se propone una arquitectura distribuida en Oracle Cloud, con balanceador y WAF de entrada, al menos dos nodos de aplicación, un trabajador de procesamiento separado, un servicio de predicción replicado, base de datos administrada con alta disponibilidad y almacenamiento de objetos para datasets, modelos y respaldos. El uso de contenedores se mantiene, pero Docker Compose queda limitado al desarrollo y a pruebas controladas.

El entrenamiento inicial utilizará CPU. La incorporación de GPU o TPU necesitará una justificación basada en mediciones del entrenamiento y la inferencia. La configuración de producción evitará un punto único de fallo mediante réplicas de aplicación, balanceador y servicios administrados.

## 4.4. Seguridad, observabilidad y confiabilidad

La API comprobará los permisos en cada operación. El rol viewer accederá a consultas y visualizaciones; el admin podrá gestionar datos, revisar evaluaciones y aprobar versiones del modelo. Estas modificaciones quedarán registradas en una auditoría.

Se registrarán errores, duración de procesos, fecha de actualización de los datos y versión utilizada en cada predicción. Si una actualización falla, se conservarán los últimos resultados válidos indicando su antigüedad. Las zonas sin información suficiente mostrarán explícitamente esa condición.

El despliegue mediante CI/CD incorporará pruebas, construcción de contenedores, comprobación del servicio y posibilidad de regresar a la versión anterior. La publicación de un modelo requerirá revisar sus métricas antes de activarlo.

# 5. Vista de bloques

Esta sección describe los componentes del sistema, sus responsabilidades y sus relaciones. La descomposición se organiza en bloques funcionales que pueden implementarse como módulos o contenedores según las restricciones del despliegue.

## 5.1. Estructura general

| Viewer / admin |  | Interfaz web |  | API de aplicación |
| --- | --- | --- | --- | --- |
| Fuentes externas |  | Procesamiento de datos y ML |  | Servicio de predicción |
|  |  |  |  | Base de datos y archivos versionados |

Figura 1. Vista de bloques de la solución

El diagrama representa relaciones entre componentes. Su distribución en servidores y contenedores se detalla en la vista de despliegue.

Alineación C3/C4. El nivel de contexto identifica actores y sistemas externos; el nivel de contenedores muestra la web, la API, el procesamiento ML, la predicción, la persistencia y el almacenamiento; el nivel de componentes descompone la API de aplicación; y el nivel de código descompone el contenedor Procesamiento y ML. Por tanto, las vistas de componentes de la API y del procesamiento ML son complementarias y no deben interpretarse como una única cadena jerárquica.

## 5.2. Responsabilidades e interfaces

| Bloque | Responsabilidad | Interfaces principales |
| --- | --- | --- |
| Interfaz web | Mostrar el mapa, estimaciones, vigencia, historial y funciones administrativas según el rol. | API mediante HTTPS/REST y proveedor de cartografía. |
| API de aplicación | Autenticar, autorizar, consultar resultados, recibir solicitudes y registrar cambios. | Interfaz web, base de datos y procesamiento. |
| Procesamiento de datos y ML | Incorporar fuentes, validar información, construir variables, entrenar y evaluar modelos. | Fuentes, base de datos, archivos y predicción. |
| Servicio de predicción | Cargar el modelo aprobado y ejecutar inferencias sobre los lotes de datos preparados; no calcula el riesgo en cada clic del viewer. | Entradas, metadatos de aprobación y modelos. |
| Base de datos | Mantener usuarios, observaciones, estimaciones, versiones, trabajos y auditoría. | Acceso autorizado de los componentes. |
| Archivos versionados | Conservar fuentes originales, conjuntos de entrenamiento, modelos y reportes. | Lectura y escritura controladas. |

## 5.3. Organización interna de los bloques principales

La API de aplicación se dividirá en tres módulos:

- Identidad y permisos: autenticación y autorización de los roles viewer y admin.

- Consultas: acceso a estimaciones por zona y período, historial y estado de actualización.

- Administración: gestión de datos, solicitudes de entrenamiento, revisión de métricas y aprobación de versiones y seguimiento de acciones preventivas.

El bloque de procesamiento de datos y ML contendrá cuatro módulos:

- Ingesta: obtención de datos y registro de su procedencia.

- Validación y preparación: revisión de formatos, fechas, coordenadas, duplicados y valores faltantes.

- Entrenamiento y evaluación: construcción de conjuntos versionados, comparación de modelos y generación de métricas.

- Coordinación de trabajos: ejecución de tareas programadas o solicitadas por un administrador, con registro de estado.

## 5.4. Contratos principales entre componentes

La consulta del mapa devolverá la zona o geometría correspondiente, la estimación, el período al que aplica, la fecha de generación, la versión del modelo y el estado de los datos, los factores explicativos y la incertidumbre disponible. La interfaz distinguirá entre resultados vigentes, desactualizados y zonas sin información suficiente.

Las operaciones extensas, como incorporar un conjunto de datos o entrenar un modelo, se ejecutarán en segundo plano. La API entregará un identificador de trabajo que permitirá consultar su avance o error.

Cada modelo candidato quedará asociado con sus datos de entrenamiento, preprocesamiento y métricas. El administrador podrá aprobar una versión para uso operativo mediante una acción auditada. El servicio de predicción verificará esa aprobación antes de utilizarla y conservará la versión empleada en cada resultado.

# 6. Vista de ejecución

Esta sección distingue dos comportamientos: la consulta del viewer, que lee un lote de estimaciones ya publicado, y la actualización automática, que cada 15 minutos incorpora datos, ejecuta el procesamiento y publica una versión coherente.

## 6.1. Consulta del riesgo por parte del viewer

| Paso | Componente | Acción |
| --- | --- | --- |
| 1 | Viewer / web | El usuario selecciona una zona y un período. |
| 2 | Interfaz web | Solicita estimaciones a la API mediante HTTPS/REST. |
| 3 | API | Valida sesión, permisos y parámetros de entrada. |
| 4 | Base de datos | Entrega el último lote publicado, su vigencia, versión del modelo y antigüedad. |
| 5 | Interfaz web | Muestra mapa, detalle del riesgo e indicaciones de vigencia. |

Figura 2. Secuencia simplificada de consulta del viewer

Si el acceso no está autorizado, la API rechaza la solicitud y la interfaz muestra un mensaje sin exponer detalles internos. La respuesta exitosa incluye la fecha de generación y la versión utilizada.

## 6.2. Actualización de las estimaciones

La actualización automática se iniciará cada 15 minutos como un trabajo en segundo plano:

- Incorporación: se obtienen los datos disponibles y se registra su fuente, fecha y versión.

- Preparación: se validan formatos, coordenadas, períodos y valores faltantes, y se construyen las variables requeridas.

- Inferencia: el servicio de predicción utiliza el modelo aprobado y su preprocesamiento asociado.

- Publicación: los resultados se guardan como un nuevo lote y se verifica su integridad antes de habilitar su consulta.

La publicación del lote se realizará mediante una operación transaccional para que las consultas utilicen una versión coherente durante la actualización. Cada ejecución registrará identificador, datos utilizados, versión del modelo, duración y estado final.

## 6.3. Entrenamiento y activación del modelo

El administrador podrá solicitar un entrenamiento desde la interfaz. La API comprobará sus permisos, registrará la solicitud y entregará un identificador para consultar el avance.

El proceso utilizará un conjunto de datos versionado, realizará la preparación y el entrenamiento, y evaluará el modelo mediante una separación temporal. Los resultados quedarán registrados como una versión candidata. El administrador revisará las métricas y los criterios de aceptación antes de aprobarla.

La activación requerirá comprobar la compatibilidad entre el modelo, su preprocesamiento y el servicio de predicción. Se conservará la versión anterior para permitir una reversión y la auditoría identificará quién solicitó el entrenamiento, quién aprobó el cambio y cuándo se realizó.

Seguimiento preventivo. Después de publicar una estimación, el admin podrá registrar una sugerencia de revisión, una acción orientativa y su evidencia. La reevaluación posterior se almacenará como una lectura independiente, indicando si la prioridad se mantiene, disminuye o aumenta y qué información sustenta la decisión.

## 6.4. Comportamiento ante fallos

| Situación | Respuesta del sistema |
| --- | --- |
| Una fuente no responde | Registrar el error, aplicar reintentos limitados y mantener visibles la antigüedad y vigencia de los últimos resultados. El siguiente ciclo se reintentará automáticamente. |
| Zona con información insuficiente | Mostrar el estado “Información insuficiente” y conservar la causa registrada. |
| Falla la inferencia o validación | Impedir la publicación y mantener el lote anterior. |
| Modelo candidato no cumple criterios | Conservarlo como evaluado y mantener activa la versión aprobada. |
| Base de datos indisponible | Informar la indisponibilidad y evitar cambios incompletos. |

# 7. Vista de despliegue

## 7.1. Distribución de los componentes

El despliegue de producción se realizará en Oracle Cloud utilizando una red privada, un balanceador de carga y dos nodos de aplicación para evitar que una sola máquina sea un punto único de fallo. La base de datos y el almacenamiento de objetos se contratarán como servicios administrados con respaldo y monitoreo.

| Viewer / admin |  | Balanceador + WAF |  | Nodos API/Web x2 |
| --- | --- | --- | --- | --- |
| Scheduler cada 15 min |  | Worker Python |  | Predicción replicada |
| Fuentes externas |  | Base de datos HA |  | Object Storage |
| Operación |  | Logs y métricas |  | CI/CD y alertas |

Figura 3. Vista de despliegue inicial en Oracle Cloud

Docker Compose se utilizará en desarrollo y pruebas locales. El despliegue utilizará Kubernetes para orquestar los contenedores de aplicación. La base de datos y el almacenamiento de objetos se provisionarán por separado. Para el desarrollo local del backend se aprobó PostgreSQL/PostGIS; la selección del motor de datos de producción deberá confirmarse antes de desplegar, ya que la implementación inicial no incluye compatibilidad con Oracle.

## 7.2. Recursos computacionales y dimensionamiento

El entrenamiento inicial utilizará Python y scikit-learn sobre CPU, considerando los modelos tabulares propuestos: regresión logística y Random Forest. La incorporación de GPU o TPU requerirá demostrar una mejora que justifique su costo y complejidad.

Como escenario inicial de prueba se proponen 20 usuarios concurrentes, con una consulta cada cinco segundos por usuario, equivalente a aproximadamente 4 solicitudes por segundo. Esta cifra es una hipótesis de dimensionamiento y deberá validarse mediante pruebas.

| Componente | Capacidad de producción | Criterio |
| --- | --- | --- |
| Nodos API/Web | 2 réplicas; 2–4 vCPU y 8 GB RAM por nodo | Balanceador y escalamiento horizontal. |
| Servicio de predicción | 2 réplicas; 2 vCPU y 4 GB RAM por réplica | Inferencia tabular en CPU y modelo aprobado en modo lectura. |
| Worker Python | 1 nodo; 4 vCPU y 16 GB RAM | Ingesta, validación, inferencia por lote y entrenamiento programado. |
| Base y almacenamiento | Servicios administrados con HA, respaldos y 50–100 GB iniciales | Escalamiento según histórico, modelos y retención. |

Antes del despliegue se comprobará la compatibilidad de las imágenes con la arquitectura elegida por el proveedor, junto con las dependencias de Oracle y ONNX Runtime. La decisión ARM64 o x86 deberá basarse en disponibilidad, soporte y costo, no solo en la cuota gratuita.

## 7.3. Estimación inicial de costos

| Recurso | Configuración o consumo previsto | Costo mensual estimado |
| --- | --- | --- |
| Cómputo y runtime | Nodos API/Web, predicción y worker | USD 250–400 |
| Base de datos administrada | Alta disponibilidad, conexiones privadas y respaldos | USD 350–500 |
| Almacenamiento y backups | Object Storage, snapshots y exportaciones cifradas | USD 150–250 |
| Red y seguridad | Balanceador, WAF, TLS, reglas privadas y transferencia | USD 150–250 |
| Observabilidad | Logs, métricas, alertas y trazas | USD 100–150 |
| CI/CD y registro | Imágenes versionadas y automatización de despliegues | USD 50–100 |
| Fuentes y contingencia | Datos con licencia, variación regional y margen operativo | USD 150–150 |
| Total producción | Servicio 24/7 con actualización automática cada 15 minutos | USD 1.200–1.800/mes |

Como FireWatch operará 24/7 y actualizará automáticamente sus datos cada 15 minutos, el escenario seleccionado es producción empresarial. El costo mensual referencial se estima entre USD 1.200 y USD 1.800 e incluye cómputo redundante, base de datos administrada, almacenamiento, respaldos, seguridad, observabilidad y operación. La cifra final dependerá de la región, el tráfico, la retención y las fuentes de datos contratadas.

## 7.4. Redes y seguridad del despliegue

El acceso web utilizará HTTPS mediante Caddy, con certificados administrados automáticamente y un nombre DNS configurado. Las reglas de red permitirán los puertos 80 y 443 para acceso web y certificados, y SSH mediante llave restringido a las direcciones autorizadas.

La base de datos de producción se ubicará en una red privada y se accederá mediante TLS, reglas de seguridad y cuentas de aplicación con privilegios mínimos. No se expondrá el endpoint de la base de datos a Internet; los secretos y certificados se administrarán mediante un servicio de secretos.

El pool de conexiones se dimensionará por servicio y se supervisará mediante métricas de saturación, errores y latencia. Los límites exactos dependerán del tamaño contratado para la base de datos administrada y deberán probarse bajo carga antes de la puesta en producción.

## 7.5. Integración y despliegue continuo

GitHub Actions ejecutará las pruebas y construirá imágenes para las arquitecturas verificadas por el equipo. Las imágenes aprobadas se publicarán con una versión identificable y su digest se utilizará para actualizar los despliegues de Kubernetes. Docker Compose se conservará para desarrollo y pruebas locales.

| Etapa | Resultado |
| --- | --- |
| 1. Cambio revisado | Se recibe un cambio en el repositorio. |
| 2. Pruebas y construcción | Se ejecutan pruebas y se construye la imagen para la arquitectura validada. |
| 3. Validación | Si falla, se bloquea la publicación; si pasa, se etiqueta la versión. |
| 4. Despliegue | Kubernetes actualiza las réplicas con la imagen aprobada. |
| 5. Salud y reversión | Se verifica el servicio; ante un fallo se restaura la imagen anterior. |

Las comprobaciones posteriores verificarán el inicio de los servicios, la conexión a la base de datos y una consulta representativa. La reversión requerirá conservar imágenes anteriores y utilizar migraciones compatibles con ambas versiones. La promoción del modelo seguirá el procedimiento de evaluación y aprobación descrito en el punto 6.3.

## 7.6. Recuperación y limitaciones

La capa de aplicación se desplegará con al menos dos réplicas detrás de un balanceador. La alta disponibilidad no elimina todos los riesgos, por lo que se mantendrán respaldos, pruebas de restauración y un procedimiento de reversión para datos, código y modelos.

La base de datos administrada y el almacenamiento de objetos incluirán respaldos automáticos, retención definida y exportaciones lógicas cifradas. La recuperación se comprobará mediante pruebas periódicas de restauración y reconstrucción del servicio.

Se registrarán consumo de recursos, errores, duración de trabajos, última actualización correcta y versión del modelo. El tamaño real de las exportaciones y los tiempos de recuperación deberán medirse para ajustar la propuesta.

# 8. Conceptos transversales

Esta sección establece las reglas que compartirán la interfaz, la API, los procesos de datos y el servicio de predicción para mantener un comportamiento coherente en una operación 24/7.

## 8.1. Identidad y control de acceso

| Operación | Viewer | Admin |
| --- | --- | --- |
| Consultar mapa, estimaciones, vigencia e historial | Sí | Sí |
| Revisar métricas detalladas y estado de trabajos | No | Sí |
| Incorporar datos y registrar correcciones | No | Sí |
| Solicitar entrenamientos | No | Sí |
| Aprobar o revertir versiones del modelo | No | Sí |
| Administrar cuentas y consultar auditoría | No | Sí |

La API comprobará la identidad, el estado de la cuenta y los permisos en cada solicitud. Toda operación sin autorización explícita será rechazada. Los componentes internos utilizarán credenciales de servicio con permisos delimitados.

Se propone utilizar JWT de duración corta y renovación revocable. Las cookies de sesión tendrán los atributos HttpOnly, Secure y SameSite, y las operaciones que modifiquen información incorporarán protección CSRF. Las contraseñas se almacenarán utilizando un algoritmo de hash resistente y una sal individual.

## 8.2. Gestión de datos y modelos

Cada conjunto de datos conservará su procedencia, versión, fecha de obtención y período representado. Durante la preparación se homologarán coordenadas, referencias temporales e identificadores territoriales.

Los archivos originales se conservarán como referencia. Las correcciones realizadas por un administrador generarán una nueva versión, registrando el motivo y el responsable. Se mantendrán diferenciados los registros observados, los datos preparados y las predicciones.

La incorporación de ejemplos de ausencia de incendios requerirá comprobar la cobertura de los registros. Cada modelo quedará vinculado con sus datos de entrenamiento, variables, preprocesamiento, métricas y criterios de clasificación del riesgo. La presentación de porcentajes de probabilidad requerirá evaluar su calibración.

La interfaz utilizará estados comunes para los resultados: vigente, desactualizado e información insuficiente. La vigencia se determinará según las fechas y reglas de actualización definidas para cada fuente y estimación.

## 8.3. Observabilidad y auditoría

Los componentes producirán registros estructurados con fecha y hora, componente, operación, resultado, duración e identificador de seguimiento. Cuando corresponda, incluirán el identificador del trabajo y las versiones del conjunto de datos y del modelo.

El identificador de seguimiento permitirá relacionar una solicitud administrativa con su procesamiento y resultado. Se observarán especialmente el tiempo de respuesta y proporción de errores de la API, la duración de trabajos, la antigüedad de los datos, el consumo de CPU, memoria y almacenamiento, y la disponibilidad de la base de datos y del servicio de predicción.

La auditoría registrará cambios de permisos, correcciones de datos, entrenamientos y activaciones de modelos. Los registros tendrán rotación y acceso restringido. Se excluirán contraseñas, tokens, secretos y datos personales innecesarios.

## 8.4. Manejo de errores y consistencia

La API utilizará un formato común de error con un código, un mensaje comprensible y un identificador de seguimiento. Los detalles técnicos quedarán disponibles en registros protegidos.

Los fallos transitorios de conexión podrán generar reintentos limitados con espera progresiva. Los errores de permisos o de datos inválidos requerirán corrección antes de volver a ejecutar la operación. Los trabajos tendrán identificadores únicos y controles para evitar efectos duplicados al reintentarse.

Las escrituras relacionadas utilizarán transacciones y los nuevos lotes de estimaciones se habilitarán únicamente después de completar sus validaciones.

## 8.5. Configuración, compatibilidad y recuperación

La configuración de fuentes, frecuencias, límites de recursos y versiones activas se mantendrá separada del código. Los secretos se suministrarán mediante mecanismos protegidos del entorno de ejecución.

Los contratos de la API y de entrada al modelo se versionarán. Antes de publicar un cambio se comprobará su compatibilidad con la interfaz, el preprocesamiento y los datos existentes.

La recuperación utilizará las imágenes anteriores de los servicios, las versiones aprobadas del modelo y las exportaciones descritas en el punto 7. La reconstrucción y reimportación deberán probarse para establecer objetivos de recuperación realistas.

# 9. Decisiones arquitectónicas

Esta sección documenta las decisiones clave (ADR - Architecture Decision Records) que guían el diseño del sistema, comparando alternativas y justificando la opción elegida en base a las restricciones del proyecto.

## 9.1. Proveedor cloud e infraestructura. Se evaluaron AWS, Azure, GCP y Oracle Cloud. La decisión propuesta es Oracle Cloud con servicios pagados de producción, por continuidad con el diseño actual, soporte para base de datos administrada y posibilidad de separar cómputo, red, almacenamiento y observabilidad.

Decisión: Oracle Cloud en un escenario de producción empresarial, utilizando red privada, balanceador, nodos de aplicación redundantes, base de datos administrada, Object Storage y monitoreo.

Justificación: el sistema debe mantenerse operativo 24/7 y recibir actualizaciones automáticas cada 15 minutos. La separación de consulta, procesamiento e inferencia permite escalar cada función y mantener la latencia del mapa aun cuando se ejecute un lote. El rango presupuestario de referencia es USD 1.200–1.800 mensuales; las cuotas gratuitas se consideran únicamente una alternativa de desarrollo, no el escenario de producción.

## 9.2. Comunicación entre componentesAlternativas: gRPC, GraphQL, REST/HTTPS.

Decisión: API REST sobre HTTPS.

Justificación: REST es el estándar más maduro y rápido de implementar para un equipo reducido (RES03). Dado que las interacciones principales son la carga de polígonos/datos en el mapa y envíos de formularios, REST cubre perfectamente la necesidad sin agregar la complejidad de gRPC o GraphQL.

## 9.3. Enfoque del Modelo de Machine LearningAlternativas: Redes Neuronales Profundas (Deep Learning) vs. Modelos Tabulares (Regresión Logística, Random Forest).

Decisión: Modelos Tabulares (Random Forest como candidato principal).

Justificación: Los datos (históricos, clima, territorio) son de naturaleza tabular. Random Forest maneja bien relaciones no lineales sin requerir los altos recursos computacionales (GPU) que exige el Deep Learning. Esto respeta la restricción de infraestructura basada en CPU.

## 9.4. Motor de InferenciaAlternativas: Inferencia en Python (API dedicada) vs. Inferencia en Go (vía ONNX).

Decisión: Inferencia en Go utilizando ONNX (con Python como plan B).

Justificación: Exportar el modelo de scikit-learn a ONNX permite que la API en Go realice la inferencia de manera nativa, reduciendo la latencia y el consumo de memoria al no tener que mantener un proceso Python constantemente activo.

## 9.5. Estrategia de DespliegueAlternativas: Kubernetes, Servicios Serverless, Docker Compose en VM única.

Decisión actualizada: Docker Compose para desarrollo y pruebas locales; Kubernetes para despliegue.

Justificación: Compose facilita un entorno local reproducible. Kubernetes permite declarar réplicas, comprobaciones de salud, límites de recursos y políticas de red para el despliegue. Su configuración inicial se desarrollará de forma gradual y deberá validarse en el entorno elegido; disponer de manifiestos no demuestra por sí solo alta disponibilidad ni cumplimiento del SLA.

# 10. Requisitos de calidad

Los atributos de calidad se evaluarán según los siguientes objetivos medibles y criterios de aceptación, diseñados para el contexto de un prototipo.

| Atributo | Métrica / Meta | Criterio de Aceptación |
| --- | --- | --- |
| Rendimiento | Latencia en consultas de mapa | El endpoint de consulta de estimaciones (viewer) debe responder en < 500 ms en el percentil 95, bajo una carga simulada de 20 usuarios concurrentes. |
| Disponibilidad | Uptime del servicio API | El servicio deberá alcanzar 99,5% de disponibilidad mensual como objetivo, respaldado por réplicas, balanceador y base de datos administrada. |
| Seguridad | Protección de endpoints | 100% de los endpoints de administración e ingesta de datos deben rechazar peticiones (401/403) que no presenten un JWT válido de rol admin. |
| Observabilidad | Trazabilidad de operaciones | 100% de los entrenamientos y actualizaciones de lotes deben generar un registro de auditoría con: ID de usuario, timestamp, resultado y versión del modelo. |
| Calidad Predictiva | Recall (Sensibilidad) y AUC-ROC | Se priorizará el Recall para minimizar falsos negativos. El modelo base deberá superar un AUC-ROC > 0,70 en el conjunto de validación temporal. |
| Actualización | Vigencia y frescura de datos | El proceso automático deberá iniciar cada 15 minutos. Cada resultado indicará fecha de emisión, antigüedad y estado; la meta se validará mediante pruebas y monitoreo. |
| Explicabilidad | Factores y trazabilidad | Los resultados visibles deberán indicar versión del modelo, lote de datos y factores principales cuando estén disponibles; si no, deberán informar que la explicación es insuficiente. |

### 11. Riesgos y deuda técnica

Se identifican los siguientes riesgos inherentes a las decisiones tomadas y la deuda técnica asumida para cumplir con los plazos (RES04):

Riesgos de Datos:

Desbalance de clases: Hay muchos más días sin incendios que con incendios. Se requerirán técnicas como SMOTE o ajuste de pesos en el modelo, de lo contrario, el sistema predecirá siempre "sin riesgo".

Falsos negativos geográficos: La ausencia de registro de un incendio en el historial de CONAF no garantiza que no haya ocurrido un conato menor.

Riesgos de infraestructura y operación empresarial:

El crecimiento de costos, la indisponibilidad regional, la dependencia del proveedor y la saturación de fuentes externas pueden afectar la continuidad. Se mitigarán mediante alertas presupuestarias, límites de consumo, respaldos, infraestructura redundante, pruebas de restauración y documentación de una eventual migración.

Compatibilidad de arquitectura: algunas librerías nativas de Python podrían requerir validación adicional en ARM64. La arquitectura del proveedor se seleccionará según soporte, disponibilidad y costo; las imágenes se probarán en CI/CD antes de la puesta en producción.

Deuda Técnica (Mejoras pendientes):

Dependencia de la infraestructura: la producción utiliza dos réplicas de aplicación, balanceador y servicios administrados. Docker Compose en una sola VM queda limitado al desarrollo y a pruebas controladas.

Observabilidad avanzada: se requiere centralizar métricas, logs, trazas y alertas con retención y responsables definidos; la configuración inicial se validará durante el piloto.

Automatización del reentrenamiento: El reentrenamiento actual requiere la intervención del admin. La orquestación automática (ej. vía Apache Airflow) se omite por presupuesto y complejidad.

Riesgo de interpretación operativa: un usuario puede confundir el nivel estimado con una alerta oficial o con una orden de intervención. Se mitigará mostrando límites, vigencia, fuentes e incertidumbre en la interfaz y manteniendo la revisión humana.

Transferencia regional: las métricas reportadas por estudios de otros países no representan el desempeño de FireWatch en Valparaíso. El modelo deberá entrenarse y validarse con datos locales, particiones temporales y, cuando sea posible, bloques espaciales.

Acción confundida con efecto: registrar una evidencia o cerrar administrativamente una tarea no prueba que el riesgo se haya reducido. La reevaluación se conservará como resultado independiente y se informará la incertidumbre.

# Fuentes técnicas, antecedentes y referencias

- arc42. (2026). Building block view. https://docs.arc42.org/section-5/

- arc42. (2026). Runtime view. https://docs.arc42.org/section-6/

- arc42. (2026). Deployment view. https://docs.arc42.org/section-7/

- Oracle. (2026). Always Free resources. https://docs.oracle.com/en-us/iaas/Content/FreeTier/freetier_topic-Always_Free_Resources.htm

- Oracle. (2026). Always Free Autonomous AI Database. https://docs.oracle.com/en-us/iaas/autonomous-database-serverless/doc/autonomous-always-free.html

- Open Web Application Security Project. (s. f.). Authorization cheat sheet. https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html

- Open Web Application Security Project. (s. f.). Logging cheat sheet. https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html

- scikit-learn developers. (2026). Cross-validation: Evaluating estimator performance.

Vidal-Silva, C., et al. (2025). Wildfire Occurrence and Damage Dataset for Chile (1985–2024): A Real Data Resource for Early Detection and Prevention Systems. Data, 10(7), 93. https://doi.org/10.3390/data10070093

Jain, P., Coogan, S. C. P., Subramanian, S. G., Crowley, M., Taylor, S., & Flannigan, M. D. (2020). A review of machine learning applications in wildfire science and management. Environmental Reviews, 28(4), 478–505. https://doi.org/10.1139/er-2020-0019

Cilli, R., et al. (2022). Explainable artificial intelligence (XAI) detects wildfire occurrence in the Mediterranean countries of Southern Europe. Scientific Reports, 12, 16349. https://doi.org/10.1038/s41598-022-20347-9

Illarionova, S., et al. (2025). Exploration of geo-spatial data and machine learning algorithms for robust wildfire occurrence prediction. Scientific Reports, 15, 10712. https://doi.org/10.1038/s41598-025-94002-4

CONAF / Datos para Resiliencia. (s. f.). Registro de eventos de incendios forestales. https://datospararesiliencia.cl/api/datasets/export?exporter=html&persistentId=doi%3A10.71578%2FUXAUN5

NASA POWER. (s. f.). Daily API y fuentes de datos meteorológicos. https://power.larc.nasa.gov/docs/services/api/temporal/daily/

Dirección Meteorológica de Chile. (s. f.). Productos de datos meteorológicos RE3017 y RE3008. https://climatologia.meteochile.gob.cl/

Duoc UC. (2026). Guía práctica: Creación de diagramas C4 aplicada al proyecto de riesgo de incendios forestales. Material de asignatura.

# 12. Declaración de uso de IA

Declaración. Durante el desarrollo de este informe se utilizaron herramientas de inteligencia artificial generativa como apoyo para organizar información, revisar redacción, comparar alternativas arquitectónicas y estructurar propuestas de requisitos, infraestructura y despliegue. La selección del alcance, las decisiones técnicas, la revisión de fuentes y la responsabilidad por el contenido final corresponden al equipo integrado por Roberto Armijo, Miguel Calderón y Ricardo Saavedra.

# Anexos

Anexo A. Actividad C4 corregida: diagramas de contexto, contenedores, componentes, código, dinámica y despliegue.

Anexo B. Actividad de Dimensionamiento corregida: memoria, CPU, almacenamiento, red, TCO y estrategia MLOps.
