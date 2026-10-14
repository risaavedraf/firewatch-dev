# Urgente — preparación MVP FireWatch

**Propósito:** carpeta temporal para preparar la reunión con el jefe de proyecto y organizar el trabajo derivado del checklist IA. Es una cola de planificación, no una declaración de que sus tareas ya estén implementadas.

## Lectura rápida para mañana

La evidencia revisada describe un prototipo/demo local con clasificación académica y entradas sintéticas. Hay componentes backend y geoespaciales descritos como implementados y pruebas registradas en ODD, pero esta revisión no levantó el stack ni ejecutó esas pruebas. No se debe presentar como predicción basada en observaciones reales, producto desplegado ni sistema aprobado para producción.

| Afirmación | Estado que se puede comunicar | Evidencia / límite |
|---|---|---|
| Demo geoespacial y flujo backend | Implementación documentada; validar la demo en el entorno de presentación | `../tasks/backend/firewatch-app-funcional.md`; `../tasks/backend/firewatch-geospatial-map.md`; código de riesgo en `backend/services/risk/internal/handler/api.go` |
| Naturaleza de la clasificación | Regla académica; datos sintéticos, no riesgo observado | `../tasks/backend/firewatch-data-contract.md` |
| Pruebas frontend | Hay una ronda registrada; no acredita login/logout real en Keycloak ni revisión móvil | `../tasks/reviews/judgment-day-frontend-round2.md`; pendientes indicados en `../tasks/frontend/client-proposal-page.md` |
| Hardening | Pendiente/no demostrado por la documentación revisada | `../tasks/security/backend-hardening.md`; unidades `../tasks/security/backend-hardening/` |
| Preparación operativa y validación científica | No hay evidencia suficiente de carga/SLA, drift con ground truth, benchmark del modelo ni Go/No-Go firmado | `../../Referencias/tareas_verificacion_implementacion_IA.md` |

### Guion recomendado

1. Mostrar el flujo que se pueda ejecutar y confirmar antes de la reunión; si no se ejecutó, decirlo.
2. Describirlo como prototipo académico de demostración, con clasificación sintética.
3. Mostrar brechas como trabajo planificado, no como capacidades existentes.
4. Pedir al jefe de proyecto que confirme el alcance mínimo para clientes, los datos permitidos, los criterios de aceptación y quién aprueba riesgos/modelo.

**No afirmar sin nueva evidencia:** exactitud del modelo, riesgo real por comuna, rendimiento/SLA, disponibilidad, seguridad aprobada, monitoreo productivo, cumplimiento regulatorio o aptitud para decisiones de emergencia.

## Unidades ODD

Cada ID del checklist fuente está asignado una sola vez. “Pendiente de verificación” significa que esta organización no comprobó ese requisito de forma individual; no equivale a afirmar que el código relacionado no exista. La evidencia debe registrarse y el estado debe actualizarse al ejecutar cada unidad.

| Unidad | Dominio | IDs asignados | Archivo |
|---|---|---:|---|
| U1 | Gobierno, seguridad y protección de datos/modelo | 41 | `01-seguridad-gobierno.md` |
| U2 | Calidad, sesgo y fugas de datos | 18 | `02-pruebas-datos.md` |
| U3 | Evaluación y robustez del modelo | 11 | `03-pruebas-modelo.md` |
| U4 | MLOps, rendimiento, optimización y monitoreo | 42 | `04-operacion-monitoreo.md` |
| U5 | Stack, pruebas continuas, Go/No-Go y práctica | 20 | `05-ct-aprobacion-demo.md` |
| **Total** | | **132** | |

## Prioridad de trabajo

- **Antes de presentar:** confirmar alcance, preparar una demo reproducible y decir con claridad qué es sintético y qué no se ha medido. No intentar representar las 132 tareas como completadas.
- **Después de acordar el mínimo con el jefe de proyecto:** ejecutar las unidades U1–U5 conforme a los criterios de la fuente. No dar por supuesto que todo el checklist es requisito de la demo de mañana.
- **Antes de clientes:** acordar por escrito qué se puede mostrar, datos disponibles, criterios de aceptación, validaciones de autenticación y la lista de afirmaciones permitidas.

## Fuente de verdad y limpieza

El detalle normativo y los criterios íntegros están en `Referencias/tareas_verificacion_implementacion_IA.md`; este backlog los referencia por ID, no los reemplaza. El documento de seguimiento ODD es `../tasks/presentation/presentacion-mvp.md`. Esta carpeta es temporal y se puede retirar cuando el equipo lo decida; no existe una eliminación automática.
