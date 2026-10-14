# U3 — Evaluación, regresión y robustez del modelo

**Estado inicial:** pendiente de verificación individual. La evidencia revisada identifica una regla académica sintética y no demuestra un benchmark científico contra datos reales.
**Fuente de criterios:** `../../Referencias/tareas_verificacion_implementacion_IA.md`, secciones 2.3 y 5.
**Objetivo:** definir pruebas metamórficas justificadas, escenarios adversarios y comparación de modelo candidato con baseline.

## Alcance — IDs de la fuente

- [ ] `MET-I1` `MET-I2` — invariancia y direccionalidad cuando exista relación documentada.
- [ ] `MET-V1` `MET-V2` — resultados registrados y justificación; no asumir monotonicidad de Random Forest.
- [ ] `ROB-I1` `ROB-I2` — escenario de ataque y umbral de tolerancia definidos por el equipo.
- [ ] `ROB-V1` `ROB-V2` — degradación medida y ausencia de fallos catastróficos bajo perturbaciones pequeñas.
- [ ] `REG-I1` — tabla comparativa de métricas y valores objetivo acordados.
- [ ] `REG-V1` `REG-V2` — comparación Champion/Challenger y política para no reemplazar ante regresión.
- Dependencia con U1: los resultados de robustez deben poder relacionarse con el entrenamiento adversario cuando aplique.

## Avance sintético — primera unidad

Evidencia parcial para `MET-I1` y `MET-V1`: la matriz de publicación comprueba que cambiar solo el identificador de zona del request sintético conserva el nivel y la justificación. Se verifica el comportamiento de `academic-rule-v1`, sus cinco factores y su fingerprint congelado, no la exactitud de un modelo entrenado.

Escenarios disponibles como preparación parcial para `ROB-I1`: humedad ausente, observaciones vencidas y recuperación incompleta. No se cierra `ROB-I2`, `ROB-V1` ni `ROB-V2`: falta tolerancia aprobada y evaluación científica con ground truth. Tampoco se cierra direccionalidad con una prueba que no se haya ejecutado.

Las pruebas focalizadas, la suite de `risk` y `go vet ./...` terminaron en 0; la verificación independiente aprobó esta unidad acotada. Evidencia detallada: [`../tasks/backend/urgent-synthetic-mvp.md`](../tasks/backend/urgent-synthetic-mvp.md) y [`../../backend/docs/synthetic-scenarios.md`](../../backend/docs/synthetic-scenarios.md). Los IDs completos siguen pendientes. La selección vía backend API está implementada, cubierta por pruebas unitarias y verificada contra PostgreSQL en bases desechables aisladas (migraciones y suites completas de jobs/risk bajo credenciales de servicio). Una ejecución aislada del arnés observó además la ruta autenticada completa contra Keycloak/RabbitMQ/PostGIS en un stack desechable (exit 0). El frontend aún no tiene selector y el stack real del usuario no se reconstruyó. La aprobación de la primera unidad no se extiende automáticamente a esa integración posterior.

## Criterio de cierre

Registrar dataset, versión de modelo, protocolo de evaluación, métricas observadas, umbrales acordados y decisión. No rellenar resultados “por medir” con números ilustrativos de la plantilla. Si todavía no hay modelo/datos comparables, dejarlo como propuesta o bloqueado.

## Verificación y dependencias

- [ ] Confirmar qué artefacto se evalúa y si es un modelo entrenado o una regla académica.
- [ ] Establecer baseline y métricas antes de ejecutar la comparación.
- [ ] Ejecutar pruebas en un conjunto documentado, separado del entrenamiento.
- [ ] Mantener el modelo vigente si el candidato no satisface los criterios acordados.
