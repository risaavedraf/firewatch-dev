# U1 — Gobierno, seguridad y protección del modelo

**Estado inicial:** pendiente de verificación por ID. La auditoría previa encontró que el hardening backend está documentado como pendiente; no extrapolar ese hallazgo a cada control sin inspección.
**Fuente de criterios:** `../../Referencias/tareas_verificacion_implementacion_IA.md`, secciones 1–3.
**Objetivo:** reunir amenazas, controles de acceso, procedencia, privacidad y operación segura en una unidad ODD verificable.

## Alcance — IDs de la fuente

- [ ] `GOB-I1` `GOB-I2` `GOB-I3` `GOB-I4` — política, clasificación, responsable y aprobación previa a producción.
- [ ] `GOB-V1` `GOB-V2` — consistencia regulatoria y verificación de aprobación.
- [ ] `AME-I1` `AME-I2` `AME-V1` `AME-V2` — modelado MITRE ATLAS, mitigaciones y justificación de N/A.
- [ ] `ZT-I1` `ZT-I2` `ZT-I3` `ZT-V1` `ZT-V2` — segmentación, autenticación, mínimo privilegio y auditoría.
- [ ] `PRV-I1` `PRV-I2` `PRV-I3` `PRV-V1` `PRV-V2` — linaje, integridad y auditoría de dependencias/modelos externos.
- [ ] `PRI-I1` `PRI-I2` `PRI-I3` `PRI-V1` — sensibilidad, privacidad y riesgo de reidentificación.
- [ ] `ENT-I1` `ENT-I2` `ENT-V1` — entrenamiento adversario y comparación medida.
- [ ] `GRD-I1` `GRD-I2` `GRD-V1` `GRD-V2` — validación de entradas/salidas y pruebas de controles aplicables.
- [ ] `SBX-I1` `SBX-I2` `SBX-V1` — contenedor, restricción de red y verificación de aislamiento.
- [ ] `OBS-I1` `OBS-V1` — alertas de distribución/confianza y simulación de desviación.
- [ ] `RED-I1` `RED-I2` `RED-I3` `RED-V1` `RED-V2` — calendario, rollback, respuesta a incidentes y simulacro medido.

## Criterio de cierre

Para cada ID, registrar estado (implementado, parcial, propuesto, N/A justificado o pendiente), enlace de evidencia, criterio de aprobación y resultado. N/A requiere justificación. No afirmar aprobación de seguridad sin evidencia reproducible y responsable identificado.

## Verificación y dependencias

- [ ] Confirmar primero con el equipo responsable la categoría de riesgo, alcance de datos y responsable de aprobación.
- [ ] Inspeccionar documentos y controles existentes antes de proponer cambios.
- [ ] Ejecutar únicamente verificaciones autorizadas y registrar limitaciones de herramientas/entorno.
- [ ] Actualizar los estados de cada ID; no usar el estado de esta unidad como prueba de aprobación.
