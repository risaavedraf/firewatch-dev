# Cambio de arquitectura — de Oracle Cloud a AWS

**Decisión:** el despliegue de producción de FireWatch pasa de Oracle Cloud a AWS EC2.
**Fecha:** 2026-11-06 · **Alcance:** proveedor cloud, almacenamiento de objetos y forma de despliegue.
**Estado:** decidido; el despliegue no está cerrado (ver §4).

## 1. Por qué se registra

Los documentos evaluados del proyecto describen otra arquitectura. Si el cambio no queda escrito, la corrección encuentra una entrega que declara algo distinto a lo que esos documentos muestran, y eso se lee como inconsistencia, no como decisión.

Evidencia de lo documentado, sin interpretación:

| Documento | Qué dice |
| --- | --- |
| `Referencias/Actividad C4_FireWatch_produccion.md:235` | Nodo de despliegue «Oracle Cloud» |
| `Referencias/Actividad C4_FireWatch_produccion.md:239` | «Kubernetes: réplicas y despliegue automatizado; Compose solo para desarrollo» |
| `Referencias/Actividad C4_FireWatch_produccion.md:61` | Contenedor de predicción: «Go, ONNX» |
| `Referencias/Actividad C4_FireWatch_produccion.md:259-261` | Almacenamiento: «Oracle Object Storage» |
| `Referencias/Actividad Dimensionamiento_FireWatch_produccion.md:48-50` | «Oracle Cloud como proveedor principal» |
| `Referencias/EP1_Equipo3_Armijo_Calderon_Saavedra2_produccion.md:474-478` | Decisión propuesta: Oracle Cloud |
| `Referencias/EP1_Equipo3_Armijo_Calderon_Saavedra2_produccion.md:492-496` | Decisión: ONNX ejecutado en Go |

Ninguna referencia del repositorio exige AWS. `Referencias/tareas_verificacion_implementacion_IA.md:327` acepta «Kubernetes / AWS ECS u otra» para escalado horizontal, es decir, deja el proveedor abierto.

## 2. Qué motiva el cambio

El plan `Referencias/FireWatch-Plan-implementacion-modelo-AWS.md` propone AWS por dos razones operativas, no por requisito académico:

1. **Costo y tiempo de operación.** Una VM con Docker Compose levanta el stack completo; un clúster de Kubernetes exige trabajo de plataforma que no produce ningún artefacto evaluable adicional.
2. **Credenciales por rol.** S3 con rol IAM evita llaves en disco en la instancia, que es un requisito de seguridad razonable y más simple de verificar que un equivalente autogestionado.

El argumento del plan que **no** se sostiene es que «la guía exige AWS (EC2)». Ver §1.

## 3. Qué cambia

| Tema | Antes (documentado) | Ahora |
| --- | --- | --- |
| Proveedor | Oracle Cloud | AWS EC2 |
| Almacenamiento de objetos | Oracle Object Storage | S3, acceso por rol IAM |
| Base de datos | Oracle Autonomous | PostGIS en contenedor (con Compose); administrada solo si se adopta Kubernetes |
| Ejecución del modelo | Go + ONNX | Go interpretando un artefacto JSON propio (§4) |

## 4. Qué no se cierra

**Forma de despliegue.** Se implementa Docker Compose en una VM y Kubernetes queda documentado como objetivo. No se declara despliegue productivo cerrado, y la documentación debe leerse así: Compose es lo que existe, Kubernetes es lo que se planea.

**Ejecución del modelo.** Se descarta `onnxruntime` en Go porque exige cgo y glibc, y los cuatro servicios compilan hoy con `CGO_ENABLED=0` sobre `gcr.io/distroless/static-debian12:nonroot` (`backend/services/*/Dockerfile:3,12-14`). El artefacto es un JSON versionado con pesos o árboles y el preprocesamiento declarado. La equivalencia se prueba contra un scorer de Python sobre un set congelado, no se asume.

**Base de datos administrada.** La pregunta «RDS o PostGIS autogestionado» solo aparece si se adopta Kubernetes. Con Compose no hay nada que decidir.

## 5. Cómo se lee la entrega

- Los documentos de `Referencias/` **no se reescriben**: son la evidencia de lo que se decidió en su momento y siguen siendo válidos como registro.
- Este documento es la fuente para saber qué cambió y por qué. Cualquier lector que compare C4 con el despliegue real debe pasar por aquí primero.
- Si se requiere una versión actualizada de C4, EP1 o Dimensionamiento para la entrega final, se produce como documento nuevo y fechado, no como edición silenciosa del original.

## 6. Pendientes que este cambio crea

| Pendiente | Dónde se resuelve |
| --- | --- |
| Dominio y certificado | `backend/deploy/prod/README.md` |
| Rol IAM con permiso de lectura sobre el bucket de datos y modelos | T10 de `aws-ml-delivery.md` |
| Apagado de la instancia al terminar las demostraciones | Runbook de producción |
| Actualización de C4/EP1 para la entrega final, si se pide | Documento nuevo, fechado |
