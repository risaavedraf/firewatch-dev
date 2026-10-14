# U2 — Pruebas y aseguramiento de datos

**Estado inicial:** pendiente de verificación individual. No concluir que existe pipeline de datos reales: el contrato revisado sitúa la ingesta real fuera del alcance documentado.
**Fuente de criterios:** `../../Referencias/tareas_verificacion_implementacion_IA.md`, secciones 2.1, 2.2 y 4.
**Objetivo:** verificar esquema, tipología, confiabilidad, sesgo geográfico/de clases y separación de entrenamiento/prueba.

## Alcance — IDs de la fuente

- [ ] `DAT-I1` `DAT-I2` `DAT-I3` — fuentes, validaciones críticas y acción ante fallos de esquema.
- [ ] `DAT-V1` `DAT-V2` `DAT-V3` `DAT-V4` `DAT-V5` — campos, tipos, nulos, dominio y confiabilidad de fuentes.
- [ ] `SES-I1` `SES-I2` `SES-I3` — grupos geográficos/de clase, métrica y estrategia de mitigación.
- [ ] `SES-V1` `SES-V2` `SES-V3` — subrepresentación, paridad y eventual N/A justificado.
- [ ] `LEA-I1` `LEA-I2` — separación temporal/geográfica y variables giveaway.
- [ ] `LEA-V1` `LEA-V2` — temporalidad estricta y ausencia de fuga de información.

## Avance sintético — primera unidad

Evidencia parcial para `DAT-I1`, `DAT-I2`, `DAT-I3`, `DAT-V1`–`DAT-V4` y `LEA-I2`: catálogo de seis perfiles, variables/unidades canónicas, valores finitos, ausencia explícita de humedad, timestamps reproducibles y rechazo de recuperación incompleta antes de publicar. Pruebas nuevas en `backend/services/risk/internal/source/synthetic_scenarios_test.go` y `backend/services/risk/internal/service/synthetic_scenarios_test.go`; contrato y límites en [`../../backend/docs/synthetic-scenarios.md`](../../backend/docs/synthetic-scenarios.md).

En la primera unidad, la ejecución focalizada, la suite de `risk` y `go vet ./...` terminaron en 0 (remedido el 2026-10-14: 293 pruebas PASS, 9 omitidas por depender de base de datos, 0 fallos; `go vet` sin diagnósticos). Esa primera unidad no levantó el stack ni probó persistencia real; la verificación independiente que la aprobó se registra en [`../tasks/backend/urgent-synthetic-mvp.md`](../tasks/backend/urgent-synthetic-mvp.md). Las etapas posteriores descritas abajo sí ejecutaron migraciones y suites contra bases desechables aisladas y una ruta autenticada completa, como evidencia documental registrada; ninguna de esas etapas cierra los IDs. La integración posterior añade selección opcional de escenarios en jobs/risk y metadatos en eventos e historial; las solicitudes sin escenario conservan el fixture anterior. El código pasó pruebas de los cuatro módulos y vet. Las migraciones y la persistencia real ya se ejecutaron en bases desechables aisladas: migraciones frescas y suites completas de jobs/risk bajo credenciales `jobs_app`/`risk_app`, con referencias cruzadas entre catálogos y `go vet` en 0. La corrección de rechazo de `scenario_id` nulo/vacío quedó verificada de forma independiente. El cierre de integración avanzó: el arnés aislado ejecutó una vez la ruta autenticada completa contra Keycloak/RabbitMQ/PostGIS en un stack desechable (exit 0), con 401/403, catálogo de seis perfiles, rechazo de entradas inválidas y cinco perfiles publicables. La sonda opcional de recuperación parcial quedó en `SKIP` por defecto y los límites/advertencias conocidos del arnés siguen vigentes; el stack real del usuario no se reconstruyó ni actualizó. El seguimiento conserva los intentos fallidos anteriores sin convertirlos en PASS retroactivo.

Los IDs permanecen sin marcar como completos: esto prueba comportamiento sintético, no confiabilidad de fuentes reales, rangos productivos aprobados, representatividad geográfica, sesgo ni ausencia de leakage en un modelo entrenado.

El registro por ID de esta evidencia vive en `backend/docs/checklist-evidence-u2-u3.md`, **en la rama `backend/urgent-synthetic-closure` (aún sin mergear a `main`)**; al mergearse, esta referencia debe convertirse en enlace relativo. Ese registro no cierra ningún ID: los 15 IDs que dependen de datos reales, umbral acordado o responsable quedan bloqueados con el input exacto que falta.

## Criterio de cierre

Adjuntar reporte reproducible de validación de datos, definir umbrales con el equipo y distinguir ausencia de datos de resultado válido. No calcular métricas de paridad sin grupos, datos y métrica acordados; no afirmar que un conjunto sintético representa la realidad.

## Verificación y dependencias

- [ ] Inventariar fuentes reales utilizadas frente a datos de demostración.
- [ ] Registrar esquema, tipos, rangos, categorías, valores faltantes y acciones ante fallo.
- [ ] Documentar la estrategia de partición para evitar leakage y comprobarla en los datos disponibles.
- [ ] Justificar métricas y decisiones N/A antes de marcar los IDs como cerrados.
