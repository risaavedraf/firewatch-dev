# U4 — MLOps, rendimiento, optimización y monitoreo

**Estado inicial:** pendiente de verificación individual. No hay evidencia suficiente en la auditoría previa de SLA/carga, drift con ground truth, escalabilidad medida o monitoreo operativo completo.
**Fuente de criterios:** `../../Referencias/tareas_verificacion_implementacion_IA.md`, secciones 3.3, 6, 7 y 8.
**Objetivo:** fijar objetivos antes de medir y comprobar servicio, recursos, recuperación y señales del modelo.

## Alcance — IDs de la fuente

- [ ] `SLA-I1` `SLA-I2` `SLA-I3` `SLA-I4` `SLA-V1` `SLA-V2` `SLA-V3` — objetivos, herramienta, prueba y reporte real de carga.
- [ ] `DRF-I1` `DRF-I2` `DRF-I3` `DRF-I4` `DRF-V1` `DRF-V2` — herramienta/método de drift, frecuencia, logging y evaluación con ground truth.
- [ ] `ESC-I1` `ESC-I2` `ESC-V1` `ESC-V2` — política de escalado, RTO y pruebas reales de escalado/recuperación.
- [ ] `OPT-I1` `OPT-I2` `OPT-I3` `OPT-V1` `OPT-V2` `OPT-V3` — propósito, hardware/software justificados y cuellos de botella.
- [ ] `ORQ-I1` `ORQ-I2` `ORQ-I3` `ORQ-V1` `ORQ-V2` — contenedores, orquestación, autoscaling, tradeoffs y medición previa.
- Coordinar con U1 las alertas de seguridad ante desviación; los IDs de esa verificación se asignan allí.
- [ ] `LOG-I1` `LOG-I2` `LOG-I3` `LOG-V1` — inputs, outputs, ground truth y utilidad del registro.
- [ ] `MET-I3` `MET-I4` `MET-V3` `MET-V4` `MET-V5` — endpoint /metrics, prioridades y lectura del dashboard.
- [ ] `ALR-I1` `ALR-I2` `ALR-V1` `ALR-V2` `ALR-V3` — umbrales, respuesta y evaluación según exista ground truth.

## Criterio de cierre

Los números de la plantilla son ejemplos, no objetivos FireWatch. Marcar los números como objetivo o medición; adjuntar configuración y reporte reproducible. Diferenciar Data Drift de Concept Drift y no activar reentrenamiento solo por un cambio de distribución.

## Verificación y dependencias

- [ ] Acordar SLO/SLA, carga representativa, RTO y herramientas antes de medir.
- [ ] Confirmar qué métricas expone el servicio y qué requiere ground truth.
- [ ] Ejecutar escenarios solo cuando exista entorno aislado y seguro; reportar errores y límites.
- [ ] Medir antes de escalar y justificar CPU/RAM/GPU por carga observada.
