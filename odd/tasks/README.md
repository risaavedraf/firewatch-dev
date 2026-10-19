# Índice de tareas — FireWatch

Busca por área y consulta el estado y los faltantes antes de abrir un documento. Este índice resume la evidencia registrada; no sustituye los criterios de aceptación de cada tarea.

## Cómo leer los estados

| Estado | Significado |
| --- | --- |
| **Listo** | La entrega indicada está terminada y tiene evidencia registrada. Un plan listo no significa código implementado. |
| **Parcial** | Existe una parte entregada, pero falta implementación o verificación dentro del alcance. |
| **Pendiente** | Trabajo propuesto o no iniciado; escribir un plan no completa su implementación. |
| **Bloqueado** | No puede avanzar por una dependencia externa concreta, indicada en «Qué falta». |
| **Histórico** | Antecedente retirado o reemplazado; no representa una capacidad actual. |
| **Por verificar** | La documentación no alcanza para confirmar el resultado o presenta contradicciones. |

Cuando solo una comprobación está bloqueada, la entrega puede ser **Parcial** y señalar ese bloqueo en «Qué falta». No se declara listo el sistema completo por haber completado una prueba aislada.

**Procedencia de la evidencia:** «observada» significa que esta sesión ejecutó o inspeccionó la comprobación; «reportada» significa que el resultado proviene del documento o de otra sesión, sin repetirse aquí. Un resultado reportado no se presenta como prueba propia. Ningún estado de este índice aprueba producción ni declara cumplidos los 132 IDs del checklist.

## Resumen rápido

- **Listo:** unidad backend sintética verificada en SQL y E2E autenticada aislada; entrega de presentación publicada en `main` (`f509666`, PR #31) con frentes 1/2/3/9 y el monitoreo local T25–T27; documentación React, presentación y revisiones acotadas. Esto no significa sistema aprobado ni modelo entrenado.
- **Parcial:** demo funcional, contrato de datos, mapa, separación gateway/frontend, rediseño de la página, unidades de hardening que esperan entorno y los frentes 1/2/9 en lo que no es fuente/modelo real; faltan comprobaciones o reconciliar evidencia.
- **Pendiente:** modelo entrenado real, fuente de datos real, carga/RTO medidos, firma/registry/rollout, gobierno y el hardening que exige entorno.
- **Límites de prueba:** no hay certificación WCAG ni medición con lector de pantalla; la aserción de foco a 390 px cierra sobre BODY cuando la lista de acciones está vacía. Siguen pendientes los requisitos científicos/de producción y el despliegue en Oracle. La pila del operador ya se recreó (L23) y el browser/Keycloak real ya se midió.
- **Histórico:** mockups retirados y plan React anterior.

Los estados siguientes provienen de **evidencia documental/reportada**, salvo las observaciones propias indicadas. No se volvieron a ejecutar tests para confeccionar estas tablas. Lee Tasks/Log del documento enlazado para el alcance exacto; sus actualizaciones posteriores prevalecen sobre esta fotografía.

## Trabajo actual

Las filas de esta sección resumen **documentos fuente** anteriores a la entrega de presentación; sus frases de «qué falta» describen el alcance de ese documento, no el estado actual. El estado vigente está en [Cierre para la presentación](presentation-client-ready.md) y en los README de [backend](../../backend/README.md) y [frontend](../../frontend/README.md).

| Entrega / documento | Estado | Qué está entregado | Qué falta |
| --- | --- | --- | --- |
| [MVP sintético urgente](backend/urgent-synthetic-mvp.md) | **Listo — backend en alcance probado (informe fuente)** | Seis perfiles; API/eventos/persistencia; null/vacío rechazados; UTC y suites SQL/vet PASS; E2E autenticada aislada job12 exit 0, cinco perfiles publicables, legacy/idempotencia/auth y limpieza propia observados | Límites de esa unidad: `partial-retrieval` runtime SKIP y advisories del harness; sin validación científica ni aprobación de producción. El selector/browser y la app React son posteriores a este informe y se leen en la entrega actual, no aquí. |
| [Demo local: pila + selector](demo/local-stack-and-scenario-selector.md) | **Parcial — pila migrada, selector y colores persistentes** | Migraciones `003`/`007` aplicadas a las bases locales **reales** (jobs `1 2 3`, risk `1 2 3 4 5 6 7`), columnas nuevas nullable, conteo de filas idéntico, 0 backfill y rerun idempotente; verificación independiente V1–V4/V6 PASS con dos correcciones registradas; selector de escenario y color persistente por comuna en la página con 44 pruebas frontend PASS y smoke PASS | Estado de esa sesión: el recorrido de navegador se cerró después en L14/L23 (lote por escenario, historial y logout real); el realm vivo rechazaba `http://localhost:4173/` en esa sesión y se corrigió en la consola, y L23 encontró y corrigió la deriva equivalente en la pila del operador. Límite vigente: una comuna no consultada permanece gris porque no hay endpoint agregado. |
| [Rediseño de la página](../urgent/rediseno-pagina/README.md) | **Histórico — rediseño de la página anterior** | El diseño y el estilo del mockup retirado aplicados a la **misma** página real (`frontend/index.html`, `proposal.css`, `proposal.js`), conservando todos los identificadores previos y el comportamiento; panel de auditoría real sobre `/v1/audit`, métricas reales, búsqueda y filtro del historial, matriz de perfiles con el rol real del token y barras de factores con `contribution` real; inventario visible de lo que el backend no respalda; **60 pruebas frontend PASS** y smoke PASS en la rama `frontend/rediseno-pagina` | Documento de la página anterior, reemplazada por la app React; el recorrido con sesión real se midió después en L14/L23. El mockup se conserva en `odd/urgent/rediseno-pagina/referencia-mockup/`; no se propone borrarlo. |
| [Plan frontend React](frontend/react-frontend-plan.md) | **Listo — documentación histórica** | Entrega documental con recheck independiente PASS acotado | La aplicación React 19 + TypeScript sí está implementada y servida (ver `frontend/README.md`); PWA sigue siendo propuesta condicional. |
| [Checklist urgente por unidades](../urgent/README.md) | **Parcial** | Guía y organización de cinco unidades | Verificar requisitos y reunir evidencia real; los 132 IDs no están declarados cumplidos |

**Evidencia backend — observada por la sesión propietaria:** corrección UTC con recheck independiente PASS; migraciones y suites completas jobs/risk con roles app/catálogos y vet exit 0; harness con revisión independiente de fuente y 17 tests ejecutados por el padre; E2E Keycloak/API/RabbitMQ/PostGIS aislada exit 0 (~65 s). Rechazo de null/vacío resuelto. Prueba runtime opcional `partial-retrieval` SKIP; límites documentados y fallos históricos preservados. Recursos desechables propios limpiados, pila existente intacta. Esto no cierra los 132 IDs ni aprueba producción.

## Backend y datos

Documentos fuente anteriores a la entrega; sus faltantes se leen contra su fecha. El recorrido OIDC/live posterior está en L13/L14/L22–L23.

| Documento | Estado | Qué está entregado | Qué falta |
| --- | --- | --- | --- |
| [Aplicación funcional e integración](backend/firewatch-app-funcional.md) | **Parcial** | Slices funcionales y L0–L4 documentados | Documento fuente anterior; el recorrido OIDC/live de la presentación está en L13/L14/L22–L23. Límite de esa unidad: brecha A.2 y comprobaciones de races no cubiertas por este informe. |
| [Contrato de datos y clasificación](backend/firewatch-data-contract.md) | **Parcial / por verificar** | Contrato, clasificador y persistencia con evidencia registrada | Documento fuente anterior; smoke/API viva/E2E posteriores: L13/L22. Límite de esa unidad: reconciliar la evidencia A.3; no asumir cierre por una entrada del Log. |
| [Mapa geoespacial](backend/firewatch-geospatial-map.md) | **Parcial** | M1–M5 declaradas cerradas | Documento fuente anterior; render/E2E posteriores: L14/L23. Límite de esa unidad: alcance de la evidencia A.2b refutada. |

## Seguridad

| Documento | Estado | Qué está entregado | Qué falta |
| --- | --- | --- | --- |
| [Plan de hardening backend](security/backend-hardening.md) | **Parcial — implementación en curso** | Plan y criterios de 12 unidades; **ejecutadas**: BH-01 (baseline de toolchain), BH-03 (tres capas de cuota del gateway), BH-07 (estado terminal, reporte cruzado del fallo y replay de operador, incluido el replay de un job `queued`), BH-08 (paridad del contrato, re-verificada) y BH-09 (pruebas negativas de autenticación, con la política ratificada por escrito); evidencia de los scans de dependencias de BH-02 | BH-02: inventario y scan de imágenes (exigen daemon de Docker y `trivy`); BH-04/05/06/11: entorno y accesos; BH-10: DB y broker aislados; BH-12: activación explícita; el escenario end-to-end de BH-07 y `-race` local. El **PR #29** quedó contenido en `main` y se cerró con un comentario (L24); no queda merge pendiente |

### Unidades de hardening

Cada unidad tiene un documento de planificación; su estado indica la **ejecución**, no la existencia del archivo.

| Unidad | Estado | Qué falta |
| --- | --- | --- |
| [BH-01 — Toolchain y baseline](security/backend-hardening/01-toolchain-baseline.md) | **Listo — verificado** | — (baseline registrado; la evidencia vive en el propio documento) |
| [BH-02 — Vulnerabilidades e imágenes](security/backend-hardening/02-vulnerability-scans.md) | **Parcial** | `govulncheck` limpio en los 4 módulos; falta el inventario y el scan de imágenes (daemon de Docker + `trivy`) |
| [BH-03 — Cuotas gateway](security/backend-hardening/03-gateway-rate-limits.md) | **Listo — verificado** | Tres capas (agregado pre-auth, por `sub`, anónimo compartido) implementadas y probadas; `-race` corre solo en CI |
| [BH-04 — Mínimo privilegio del broker](security/backend-hardening/04-broker-least-privilege.md) | **Pendiente** | Decisión de entorno y accesos antes de configurar y verificar permisos |
| [BH-05 — Seguridad de transporte](security/backend-hardening/05-transport-security.md) | **Pendiente** | Decisión de entorno antes de implementar y verificar los controles |
| [BH-06 — Egress Kubernetes](security/backend-hardening/06-kubernetes-egress.md) | **Pendiente** | Resolver entorno/routing y verificar restricciones |
| [BH-07 — Recuperación de jobs](security/backend-hardening/07-job-recovery.md) | **Listo — verificado**, con el escenario end-to-end pendiente | Estado terminal, reporte cruzado del fallo, replay de operador y replay de un job `queued` con confirmación explícita; falta el escenario con PostgreSQL y broker interrumpidos, y `-race` local |
| [BH-08 — Errores OpenAPI](security/backend-hardening/08-openapi-errors.md) | **Listo — verificado y re-verificado** | Sin faltantes propios; la re-verificación fue en línea porque la admisión rechazó al verificador independiente |
| [BH-09 — Autenticación negativa](security/backend-hardening/09-authentication-tests.md) | **Listo — verificado** | — (política de tokens ratificada por escrito el 2026-10-16) |
| [BH-10 — Idempotencia concurrente](security/backend-hardening/10-concurrent-idempotency.md) | **Pendiente** | Pruebas concurrentes en entorno aislado (DB y broker propios) |
| [BH-11 — Seguridad de despliegue](security/backend-hardening/11-deployment-security-verification.md) | **Pendiente** | Depende de BH-02/04/05/06; se valida al final |
| [BH-12 — Errores centralizados](security/backend-hardening/12-centralized-error-handling.md) | **Pendiente — diferida** | Activación explícita antes de implementar |

Los estados BH corresponden al plan registrado; no se infiere que cambios de otros trabajos cierren estas unidades sin actualizar sus criterios y evidencia. Fila y unidades actualizadas el 2026-10-16 con la ejecución de la sesión A: la evidencia por unidad está en cada documento y el estado consolidado en [`../plan-produccion/11-hardening-bh.md`](../plan-produccion/11-hardening-bh.md).

## Producción

Plan de producción en [`../plan-produccion/README.md`](../plan-produccion/README.md) (15 frentes, un documento por frente). Estado al 2026-10-16: **evidencia observada** en la sesión de cierre (suites locales y CI del PR #29) para los frentes 4, 10 y 11, y **reportada** por su sesión propietaria para 5 y 6. **Actualizado el 2026-10-18:** la entrega de presentación cerró los frentes 1, 2, 3 y 9 y el ítem de código del frente 5; el detalle y sus límites están en [`presentation-client-ready.md`](presentation-client-ready.md), Log L22–L28. El inventario con qué desbloquea cada frente vive en [`../plan-produccion/README.md`](../plan-produccion/README.md) §10.

| Frente | Estado | Qué falta |
| --- | --- | --- |
| [1 — Ingesta y scheduler](../plan-produccion/01-ingesta-scheduler.md) | **Parcial — seam y ciclo entregados** | Proveedor configurable, validación por registro y ciclo de 15 min verificados (L16/L22); la fuente de datos real sigue siendo el pendiente permitido |
| [2 — Servicio de inferencia](../plan-produccion/02-modelo-inferencia.md) | **Parcial — registro y segunda versión entregados** | `firewatch-rule-v2`, activación/reversión y arranque en frío verificados (L13/L22); el modelo entrenado real y su evaluación científica siguen pendientes |
| [3 — `prevention`](../plan-produccion/03-prevention.md) | **Listo — servicio entregado y verificado** | Servicio, migración `009`, contrato congelado y vista `#actions` verificados (L13/L14); separar la base de `risk` es decisión de producción |
| [4 — Fallos terminales y replay](../plan-produccion/04-dlq-replay-terminal.md) | **Listo — verificado** | Escenario end-to-end (PostgreSQL + broker interrumpidos) y `-race` local |
| [5 — Observabilidad](../plan-produccion/05-observabilidad.md) | **Parcial — monitoreo local completo** | Scrape de gateway/jobs/risk/prevention, 7 alertas y Grafana con panel versionado (L26/L27); faltan medir umbrales, *ground truth*, Alertmanager y monitoreo en Kubernetes |
| [6 — Autoscaling, carga y SLO](../plan-produccion/06-autoscaling-carga-slo.md) | **Parcial** | Ejecutar la carga, verificar el HPA y medir el RTO |
| [7 — Firma, registry y rollout](../plan-produccion/07-firma-registry-rollout.md) | **Pendiente** | Firma, promoción y despliegue verificado |
| [8 — Gobierno y seguridad (U1)](../plan-produccion/08-gobierno-seguridad-u1.md) | **Pendiente** | Responsables y aprobación de controles |
| [9 — Login browser/móvil/a11y](../plan-produccion/09-login-browser-a11y.md) | **Parcial — recorrido real medido** | Login/logout con Keycloak, 390 px, teclado y contraste medidos (L14/L23); límites de prueba: sin certificación WCAG ni lector de pantalla, y la aserción de foco a 390 px cierra sobre BODY |
| [10 — OpenAPI y contract tests](../plan-produccion/10-openapi-contract-tests.md) | **Listo — verificado** | Nada propio; la revisión independiente no corrió |
| [11 — Hardening BH-01…BH-11](../plan-produccion/11-hardening-bh.md) | **Parcial — sesión A cerrada** | Las unidades BH de arriba |
| [12 — Evaluación científica](../plan-produccion/12-evaluacion-cientifica.md) | **Bloqueado** | Datos reales, modelo y umbrales |
| [13 — Go/No-Go](../plan-produccion/13-go-no-go.md) | **Bloqueado** | Evidencia de datos y modelo, y criterios firmados |
| [14 — Motor de datos y continuidad](../plan-produccion/14-motor-datos-continuidad.md) | **Pendiente** | Decisión de motor productivo |
| [15 — Procedencia geoespacial (DIFROL)](../plan-produccion/15-procedencia-geoespacial.md) | **Pendiente** | Validación DIFROL |

## Frontend

| Documento | Estado | Qué está entregado | Qué falta |
| --- | --- | --- | --- |
| [Página y propuesta para cliente](frontend/client-proposal-page.md) | **Parcial — superada por la página React** | Dashboard, código PKCE/logout y checks registrados | CP-1/CP-2 abiertos; el recorrido browser/Keycloak real ya se midió (L14/L23), pero este documento describe la página anterior y no se usa como estado actual |
| [Plan React actual](frontend/react-frontend-plan.md) | **Listo — documentación histórica** | Plan y verificación documental acotada | La aplicación React 19 + TypeScript sí está implementada y servida (ver `frontend/README.md`); PWA sigue siendo propuesta condicional |
| [Plan técnico de migración](../../frontend/docs/react-migration-plan.md) | **Listo — documentación histórica** | Inventario, propuesta React/TypeScript/Vite/Node/npm, arquitectura y matriz | Migración ejecutada; PWA sigue siendo propuesta condicional, no funcionalidad actual |
| [Plan React anterior](frontend/frontend-react-vite-migration.md) | **Histórico** | Plan para el mockup retirado | No usar sus tareas como parity de la página actual; utilizar el plan técnico actual |
| [Reconciliación de docs y odd contra el código](reconcile-docs-odd-code.md) | **Listo — conciliación ejecutada** | Citas `archivo:línea` de `backend/docs/**` corregidas una por una contra el código; HPA de `prevention` añadido y `FireWatchTargetDown` Ampliado a los cuatro servicios; plan React marcado como histórico; índice y baseline refrescados | La deuda ya declarada de `backend/docs/code-explanation/` (instantánea del 2026-10-14) sigue abierta y no se re-baselineó aquí; el HPA y la alerta no se verificaron en clúster |
| [Mockup de producto](frontend/frontend-product-mockup.md) | **Histórico** | Antecedente de diseño del mockup retirado | No contar como producto servido actualmente |
| [Evolución del mockup preventivo](frontend/frontend-mockup/README.md) | **Histórico — diseño** | Backlog y planificación FWP-C02 | FWP-C01 y FWP01–08 no equivalen a funcionalidad entregada; requieren revisión de alcance antes de retomarse |

**Observación previa (histórica):** la suite de la página anterior dio 40 tests; la suite React actual es 358/358 y el login/logout real se midió en L14/L23. La accesibilidad formal sigue sin certificación completa.

## Arquitectura

| Documento | Estado | Qué está entregado | Qué falta |
| --- | --- | --- | --- |
| [Separación gateway / frontend](architecture/gateway-frontend-separation.md) | **Parcial** | Separación implementada; T11/T12 cerradas en documento | Documento de esa separación; el browser/Keycloak real se midió en L14/L23. Límites vigentes: los del baseline/manifiesto que el propio documento declara; no tomar el encabezado antiguo como única fuente |

## Revisiones

| Documento | Estado | Qué está entregado | Qué falta / límite |
| --- | --- | --- | --- |
| [Judgment Day backend — ronda 1](reviews/judgment-day-backend-round1.md) | **Listo — revisión acotada** | Veredicto APPROVED registrado para su lote | Ese resultado no cierra PostGIS/RabbitMQ/Keycloak live ni riesgos fuera del lote; el recorrido live posterior está en L13/L14/L22. Docker limitó esa evidencia. |
| [Judgment Day frontend — ronda 2](reviews/judgment-day-frontend-round2.md) | **Listo — revisión acotada** | APPROVED y 40/40 registrados | Ese resultado no demuestra móvil 390 px, login/logout browser ni accesibilidad; el recorrido real posterior está en L14/L23. |
| [Arreglos de la segunda ronda de navegador](reviews/browser-round2-fixes.md) | **Histórico — arreglos entregados y PR #28 contenido/cerrado (L24)** | Los tres defectos corregidos en seis commits (último `d7b2d4b`): el 401 expresa el rechazo, el id de correlación es único y canónico, y `#update-checked` ya no contradice la tarjeta. Veredicto independiente **5 PASS / 0 FAIL** sobre `main..f9eb60e`, con refutación intentada y fallida en S4; escalera del frontend **246/246** y `go test` + `vet` en verde; issue **#27** y PR **#28** | Registro de esa ronda; la autoridad nativa no se repite como estado actual. El PR #28 quedó contenido en `main` y cerrado con un comentario (L24). Límite de esa ronda: la nota del mapa queda residual y el id duplicado no se verificó en navegador contra la pila viva (S9). |

| [Verificación end-to-end de la presentación (T17)](reviews/presentation-client-t17-e2e.md) | **Parcial — informe histórico superado por L22** | Veredicto por S1–S6 con **S3 PASS solo en los flujos observados**: Chrome sin fallos, planificador con dos corridas reales a 899.962 s, `risk-test` contra PostgreSQL real como `risk_app`, `prevention` completo, activación y reversión de modelo por HTTP, `403` y `401` reales con su `request_id`, 390 px, teclado y 124 nodos de contraste sin fallos. Cuatro defectos reales encontrados y corregidos con RED observado | **S1 y S2 quedaron aprobadas después del informe por L22**: la corrida en frío de `MODEL_ID=firewatch-rule-v2` se ejecutó una vez y pasó, y la atribución de job14 quedó determinada. L23 midió el panel de modelo y la frescura en una pasada read-only (53 nodos de contraste, 0 fallos), sin una aserción independiente por cada texto nuevo; límites de prueba: foco BODY y sin certificación WCAG ni lector de pantalla. El informe se conserva como registro de su corrida sobre `4ea6142` |

## Presentación

| Documento | Estado | Qué está entregado | Qué falta |
| --- | --- | --- | --- |
| [Preparación de presentación MVP](presentation/presentacion-mvp.md) | **Listo — documentación** | Cinco unidades y guía preparadas | Ejecutar/verificar backlog y confirmar demo; no aprueba los 132 IDs ni producción |
| [Cierre para la presentación con el cliente](presentation-client-ready.md) | **Entregado en `main`; baseline actual `ef686c0`** | Frentes 1, 2, 3 y 9 más el ítem de código del frente 5 en `feat/presentacion-cliente` (40 commits): ingesta configurable con validación por registro y ciclo de 15 min verificado con marcas reales (899.962 s entre corridas), frescura por fuente, segunda versión de modelo con activación y reversión, servicio `prevention` completo (migración `009`), vista de acciones, paneles de frescura y de modelo, métricas en `gateway` y `jobs`, y después en `prevention`/Grafana con los PRs #33–#35, proxies del gateway, y CI/compose/k8s cableados. **28 paquetes Go y 358 pruebas de frontend verdes**, más `compose config` válido. Cuatro defectos reales encontrados por la verificación independiente y corregidos con RED observado (`8130791`, `3dd04a3`, `166c2d0`, `4ea6142`). | **S1, S2 y S3 sostenidas**: S3 PASS en los flujos observados (Chrome sin fallos, planificador PASS, `risk-test` contra PostgreSQL real, 124 nodos de contraste) y **S2 aprobada** por el arranque **en frío** de `MODEL_ID=firewatch-rule-v2` (L22), con la atribución de job14 ya determinada. **Entrega publicada:** `main` quedó en `f509666` con el PR #31 (issue #30) en el momento de la entrega; la ola de monitoreo siguió en los PRs #33–#35 y el baseline actual es `ef686c0`, con los 85 commits locales previos ya en GitHub y los PR #28/#29 contenidos en `main` y cerrados con su comentario; el clasificador de la demo quedó en `firewatch-rule-v2` (fijado por `MODEL_ID` y activado en el registro); la línea `PREVENTION_URL` se commiteó (`c57b1cb`); y el panel de modelo y la fila de renovación se midieron en la pasada read-only del 2026-10-18 (53 nodos de contraste sin fallos), sin una aserción independiente por cada texto nuevo. **Pendiente de personas:** el despliegue en el servidor de Oracle. |

## Publicación

| Documento | Estado | Qué está entregado | Qué falta |
| --- | --- | --- | --- |
| [Preparación del repositorio para publicación](publication/github-repo-prep.md) | **Listo — baseline verificado** | Árbol a commitear limpio de secretos y artefactos pesados; `.gitignore` con `tmp/`; READMEs reconciliados; verificador de enlaces en verde (0 rotos); commit base único; acceso SSH del equipo al repositorio remoto | `push` y publicación siguen siendo decisión del usuario; re-baseline de `code-explanation/` pendiente de autorización explícita |

## Estado de la organización

| Entrega | Estado | Qué falta |
| --- | --- | --- |
| Traslado de documentos por área | **Listo** | Los 9 documentos viven en `backend/`, `security/`, `frontend/`, `reviews/` y `presentation/`; este índice apunta a las rutas nuevas |
| Referencias con rutas antiguas en superficies ajenas | **Listo** | Verificado el 2026-10-17 con `odd/tools/check_md_links.py` (`BROKEN: 0`, `--self-test` 22/22): todas las referencias de `odd/urgent/README.md` resuelven; las dos rutas backtick que seguían root-relativas se corrigieron a relativas al documento |
| Duplicado residual en la raíz | **Listo** | Eliminado el 2026-10-17: `odd/tasks/backend-hardening/` contenía las versiones previas (`Status: pending`, casillas sin marcar) de 12 unidades; la copia trabajada y verificada vive en `security/backend-hardening/` |
| Área `publication/` agregada | **Listo** | El documento de preparación de publicación vive en `publication/` y este índice apunta a la ruta nueva |

## Convención práctica

**Regla para todo documento nuevo y para cada cambio de estado:**

1. Crea el documento en una carpeta temática: `frontend/`, `architecture/`, `reviews/`, `backend/`, `security/`, `presentation/` o `publication/`, según corresponda. No agregues nuevas tareas sueltas a la raíz; crea el área cuando sea necesaria.
2. Mantén juntos el plan y sus subtareas. Las rutas antiguas reservadas son excepciones temporales, no el patrón para trabajo nuevo.
3. En la misma entrega, añade o actualiza la fila del documento en este README: enlace, **estado**, **qué está entregado** y **qué falta**. No basta con agregar el enlace.
4. Actualiza la fila cuando una tarea avance, termine, se bloquee o sea reemplazada; registra la evidencia detallada en su documento. No marques Listo sin evidencia y no confundas documentación con implementación.
5. Mover un documento requiere actualizar sus enlaces relativos y los enlaces que lo apuntan, además de su fila. No reinterpretes resultados históricos ni edites superficies reservadas.

Formato para registrar una entrega:

```markdown
| [Nombre de la entrega](area/documento.md) | Pendiente | Ninguna entrega aún | Implementar y verificar los criterios del documento |
```

Antes de cerrar trabajo documental, comprueba que cada documento nuevo tenga carpeta, enlace válido, estado y faltantes explícitos en este índice.

**Convenciones de PR:** este repositorio adopta el flujo issue-first desde el 2026-10-15: un issue con `status:approved`, exactamente una etiqueta `type:*`, la plantilla de `.github/PULL_REQUEST_TEMPLATE.md` y ramas `tipo/descripcion`. La decisión y su evidencia están en [`../PR-CONVENTIONS.md`](../PR-CONVENTIONS.md).

### Rutas antiguas que siguen apareciendo

La organización física está completa: cada documento vive en su área y este índice apunta a la ruta nueva. Las referencias que todavía muestran rutas antiguas están en `odd/urgent/README.md` (superficie de la sesión backend) y en entradas de Log históricas, que no se reescriben por decisión del repositorio. La tabla de abajo resuelve la navegación.

### Rutas trasladadas

Los manifiestos históricos de `tmp/` no se reescribieron. Si citan una ruta anterior, utiliza esta tabla:

| Anterior dentro de `odd/tasks/` | Actual |
| --- | --- |
| `frontend-product-mockup.md` | `frontend/frontend-product-mockup.md` |
| `frontend-mockup/README.md` | `frontend/frontend-mockup/README.md` |
| `gateway-frontend-separation.md` | `architecture/gateway-frontend-separation.md` |
| `judgment-day-backend-round1.md` | `reviews/judgment-day-backend-round1.md` |
| `firewatch-app-funcional.md` | `backend/firewatch-app-funcional.md` |
| `firewatch-data-contract.md` | `backend/firewatch-data-contract.md` |
| `firewatch-geospatial-map.md` | `backend/firewatch-geospatial-map.md` |
| `backend-hardening.md` | `security/backend-hardening.md` |
| `backend-hardening/` | `security/backend-hardening/` |
| `client-proposal-page.md` | `frontend/client-proposal-page.md` |
| `frontend-react-vite-migration.md` | `frontend/frontend-react-vite-migration.md` |
| `judgment-day-frontend-round2.md` | `reviews/judgment-day-frontend-round2.md` |
| `presentacion-mvp.md` | `presentation/presentacion-mvp.md` |
