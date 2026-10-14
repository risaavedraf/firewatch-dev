# Preparación de presentación MVP FireWatch

Objetivo: ordenar las 132 tareas del checklist de verificación IA en unidades ODD y preparar una presentación honesta del estado del MVP.
Rama: `master` (sin cambios de rama ni commits en esta preparación documental).
Entrega: backlog temporal en `odd/urgent/`; no implementa por sí mismo las tareas del backlog.

## Specs

### S1 — Organizar el checklist completo en tareas ODD

Solicitud original: “me gustaría tener todo listo para la presentación de mañana divide todas esas tareas en el sistema odd, haz más de unos archivos y colocalos en una carpeta que se llame "urgent" o algo así, para sacarlo luego”

- Alcance: crear una carpeta temporal `odd/urgent/`, un índice de lectura y unidades de trabajo que cubran todos los IDs de `Referencias/tareas_verificacion_implementacion_IA.md`.
- Aceptación: cada ID del checklist aparece en exactamente una unidad de trabajo y conserva su texto/criterio en la fuente original enlazada.
- Verificación: comprobar cobertura y unicidad de los 132 IDs contra la fuente.
- Restricción: esta preparación solo crea y organiza documentación; no declara implementadas las tareas asignadas ni ejecuta cambios de producto.

### S2 — Preparar el estado comunicable para la presentación

- Alcance: resumir lo que la evidencia ODD ya permite decir, sus límites y las preguntas que deben acordarse antes de prometer un MVP a clientes.
- Aceptación: separar evidencia documentada, capacidades por validar y trabajo pendiente; no inventar mediciones ni afirmar operación real.
- Verificación: lectura estructural del índice y comprobación de referencias a los documentos ODD existentes.

## Tasks

- T1 — S1 — inline — completado: cinco unidades ODD por dominio en `odd/urgent/`; sin commit (preparación documental solicitada).
- T2 — S2 — inline — completado: guía de presentación y límites en `odd/urgent/README.md`; sin commit (preparación documental solicitada).

## Log

- L1 — Solicitud original: “me gustaría tener todo listo para la presentación de mañana divide todas esas tareas en el sistema odd, haz más de unos archivos y colocalos en una carpeta que se llame "urgent" o algo así, para sacarlo luego”
- L2 — El checklist contiene 132 IDs (72 implementación y 60 verificación). La auditoría precedente encontró evidencia de demo académica con datos sintéticos, pero no prueba operación real, evaluación completa del modelo ni preparación de producción. Los estados por ID quedan explícitamente pendientes de verificación; las casillas del documento fuente no se tratan como evidencia.
- L3 — La carpeta `odd/urgent/` es una bandeja temporal de planificación. Puede retirarse más adelante cuando el equipo decida; no borrar ni mover su contenido automáticamente.
- L4 — El repositorio estaba en `master` y mostraba sus archivos como no rastreados antes de esta preparación. Se preservó ese estado: no se cambió de rama, no se ejecutaron pruebas ni se crearon commits.
- L5 — Espejo Engram pendiente: el guardado falló porque Engram detectó varias sesiones activas del proyecto y no hay una identidad de sesión autoritativa disponible.
