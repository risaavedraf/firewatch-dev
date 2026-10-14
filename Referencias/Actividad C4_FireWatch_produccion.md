# Actividad C4 - FireWatch

Equipo: Roberto Armijo, Miguel Calderón y Ricardo Saavedra

## Nivel 1: Diagrama de Contexto

C4Context

title Diagrama de Contexto para el Sistema FireWatch

Enterprise_Boundary(b0, "Entorno de Operación") {

Person(viewer, "Usuario Viewer", "Consulta estimaciones de riesgo, vigencia e historial en el mapa.")

Person(admin, "Usuario Admin", "Gestiona datos, revisa métricas y aprueba versiones del modelo.")

System(firewatch, "Plataforma FireWatch", "Estima el riesgo de ocurrencia de incendios forestales mediante IA.")

}

System_Ext(conaf, "CONAF", "Provee registros históricos georreferenciados de incendios.")

System_Ext(meteo, "Dirección Meteorológica / NASA", "Provee variables meteorológicas (temperatura, humedad, viento).")

System_Ext(mapa, "Proveedor Cartográfico", "Provee capas base y cartografía para visualización.")

Rel(viewer, firewatch, "Consulta nivel de riesgo", "HTTPS")

Rel(admin, firewatch, "Administra el sistema", "HTTPS")

Rel(conaf, firewatch, "Entrega antecedentes históricos")

Rel(meteo, firewatch, "Entrega datos meteorológicos")

Rel(firewatch, mapa, "Solicita mapas base para renderizado")

## Nivel 2: Diagrama de Contenedores

C4Container

title Diagrama de Contenedores para el Sistema FireWatch

Person(viewer, "Usuario Viewer", "Consulta estimaciones de riesgo, vigencia e historial en el mapa.")

Person(admin, "Usuario Admin", "Gestiona datos, revisa métricas y aprueba versiones del modelo.")

System_Ext(conaf, "CONAF", "Provee registros históricos georreferenciados de incendios.")

System_Ext(meteo, "Dirección Meteorológica / NASA", "Provee variables meteorológicas.")

System_Ext(mapa, "Proveedor Cartográfico", "Provee capas base y cartografía.")

System_Boundary(fw, "Plataforma FireWatch") {

Container(web, "Interfaz Web", "React, TypeScript, MapLibre", "Presenta el mapa interactivo, filtros y funciones de administración.")

Container(api, "API de Aplicación", "Go, Gin", "Gestiona autenticación, autorización y consultas REST.")

Container(proc, "Procesamiento y ML", "Python, scikit-learn", "Valida datos, construye variables, entrena y evalúa modelos.")

Container(pred, "Servicio de Predicción", "Go, ONNX", "Ejecuta inferencias con el modelo aprobado.")

ContainerDb(db, "Base de Datos", "Oracle Autonomous Database", "Almacena usuarios, predicciones y auditoría.")

ContainerDb(storage, "Almacenamiento de Archivos", "Oracle Object Storage", "Conserva conjuntos de datos versionados y modelos.")

}

Rel(viewer, web, "Consulta mapa y estimaciones", "HTTPS")

Rel(admin, web, "Administra sistema y modelos", "HTTPS")

Rel(web, api, "Envía consultas y comandos", "JSON/HTTPS")

Rel(web, mapa, "Solicita mapas base para renderizado", "HTTPS")

Rel(api, db, "Lee/Escribe registros operativos", "SQL")

Rel(api, proc, "Coordina trabajos en segundo plano")

Rel(api, pred, "Solicita inferencias del modelo")

Rel(proc, conaf, "Obtiene históricos")

Rel(proc, meteo, "Obtiene variables climáticas")

Rel(proc, storage, "Sube/descarga archivos y modelos")

Rel(proc, db, "Registra métricas y versiones candidatas", "SQL")

Rel(pred, storage, "Carga el modelo ONNX en memoria")

## Nivel 3: Diagrama de Componentes del contenedor API de Aplicación

C4Component

title Diagrama de Componentes - API de Aplicación (Nivel 3)

Container(web, "Interfaz Web", "React, TypeScript", "Frontend SPA que consume la API.")

ContainerDb(db, "Base de Datos", "Oracle Autonomous", "Capa de persistencia.")

Container(proc, "Procesamiento y ML", "Python", "Trabajador en segundo plano.")

Container(pred, "Servicio de Predicción", "Go, ONNX", "Microservicio de inferencia.")

Container_Boundary(api_go, "API de Aplicación (Go, Gin)") {

Component(auth, "Identidad y Permisos", "Middleware JWT", "Autentica usuarios y aplica autorización basada en roles (viewer/admin).")

Component(consultas, "Módulo de Consultas", "Controlador (Go/Gin)", "Atiende peticiones de mapas, estimaciones de riesgo, vigencia e historial.")

Component(admin_mod, "Módulo de Administración", "Controlador (Go/Gin)", "Gestiona ingreso de datos, acciones preventivas, y aprueba versiones del modelo.")

Component(coord, "Coordinador de Trabajos", "Servicio (Go)", "Desencadena y registra el estado de tareas asíncronas de entrenamiento e ingesta.")

Component(pred_client, "Cliente de Predicción", "Cliente HTTP", "Se encarga de la comunicación directa con el motor de inferencia.")

Component(repo, "Capa de Acceso a Datos", "Repository", "Abstrae y centraliza las transacciones de lectura y escritura.")

}

Rel(web, consultas, "Solicita estimaciones y métricas", "JSON/HTTPS")

Rel(web, admin_mod, "Ejecuta acciones administrativas", "JSON/HTTPS")

Rel(consultas, auth, "Valida sesión (JWT)")

Rel(admin_mod, auth, "Valida sesión y rol (Admin)")

Rel(consultas, repo, "Consulta información")

Rel(admin_mod, repo, "Registra auditoría y actualizaciones")

Rel(admin_mod, coord, "Solicita entrenamiento o preparación de datos")

Rel(coord, proc, "Envía comando de ejecución")

Rel(coord, pred_client, "Solicita inferencia durante cada lote programado")

Rel(pred_client, pred, "Solicita evaluación del modelo")

Rel(repo, db, "Ejecuta operaciones", "SQL")

## Nivel 4: Diagrama de Código del contenedor Procesamiento y ML

classDiagram

direction TB

class GestorIngesta {

+obtener_historicos_conaf()

+obtener_clima_meteo()

+registrar_procedencia()

}

class PreprocesadorDatos {

+validar_formatos_fechas()

+homologar_coordenadas()

+imputar_valores_faltantes()

}

class ModeloRiesgo {

+version_candidata: String

+entrenar_random_forest()

+evaluar_metricas_auc()

+exportar_modelo_onnx()

}

class CoordinadorML {

+iniciar_preparacion()

+iniciar_entrenamiento()

+actualizar_estado_trabajo()

}

CoordinadorML --> GestorIngesta : Coordina

GestorIngesta --> PreprocesadorDatos : Envía crudos

PreprocesadorDatos --> ModeloRiesgo : Envía limpios

La consulta del viewer no ejecuta una inferencia en cada clic. Lee el último lote publicado por el proceso automático. La ingesta, preparación, inferencia y publicación se ejecutan cada 15 minutos cuando las fuentes están disponibles.

## 5. Vista Dinámica: Consulta del lote publicado por el Viewer

C4Dynamic

title Vista Dinámica - Consulta del lote publicado por el Viewer

Person(viewer, "Usuario Viewer", "Analista o ciudadano")

Container(web, "Interfaz Web", "React + MapLibre")

Container(api, "API de Aplicación", "Go")

ContainerDb(db, "Base de Datos", "Oracle Autonomous")

Rel(viewer, web, "1. Selecciona zona y período en el mapa")

Rel(web, api, "2. Solicita estimaciones (JSON/HTTPS)")

Rel(api, api, "3. Valida sesión y permisos de acceso")

Rel(api, db, "4. Consulta el último lote publicado, vigencia y versión")

Rel(db, api, "5. Entrega registros y metadatos")

Rel(api, web, "6. Responde con los resultados estructurados")

Rel(web, viewer, "7. Muestra niveles de riesgo y vigencia en el mapa")

## 6. Vista de Despliegue - Producción empresarial

C4Deployment

title Vista de Despliegue - Escenario: Producción Empresarial 24/7 (USD 1.200–1.800/mes)

Deployment_Node(cloud, "Oracle Cloud", "Producción 24/7, red privada, redundancia y seguridad") {

Deployment_Node(vm, "Capa de aplicación redundante", "Dos nodos detrás de un balanceador") {

Deployment_Node(docker, "Contenedores de aplicación", "Kubernetes: réplicas y despliegue automatizado; Compose solo para desarrollo") {

Container(caddy, "Balanceador y WAF", "Entrada segura", "Termina TLS, aplica reglas y distribuye el tráfico.")

Container(api, "API y Web", "Go + React", "Atiende consultas y funciones administrativas.")

Container(proc, "Worker de procesamiento", "Python + Scheduler", "Cada 15 minutos ingesta, valida y prepara los datos.")

Container(pred, "Servicio de predicción", "Go + ONNX", "Ejecuta inferencias batch con el modelo aprobado.")

}

}

Deployment_Node(paas_db, "Base de Datos", "Oracle Autonomous HA, red privada") {

ContainerDb(db, "Persistencia", "Relacional", "Almacena datos, auditoría y estimaciones.")

}

Deployment_Node(paas_obj, "Almacenamiento de Objetos", "Datasets, modelos y respaldos") {

ContainerDb(storage, "Object Storage", "Archivos y backups", "Conserva datasets, modelos versionados y exportaciones cifradas.")

}

}

Rel(caddy, api, "Distribuye consultas HTTPS")

Rel(api, db, "Conexión a la base de datos")

Rel(proc, storage, "Lee fuentes y guarda lotes/modelos")

Rel(pred, storage, "Carga modelos aprobados")
