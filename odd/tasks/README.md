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

- **Listo:** unidad backend sintética verificada en SQL y E2E autenticada aislada; documentación React, presentación y revisiones acotadas. Esto no significa React/PWA implementados, pila existente actualizada ni sistema aprobado.
- **Parcial:** demo funcional, contrato de datos, mapa, página actual y separación gateway/frontend; faltan comprobaciones o reconciliar evidencia.
- **Pendiente:** implementar React/PWA si se autoriza y ejecutar hardening.
- **Límites pendientes:** selector frontend/browser, actualización autorizada de la pila existente, prueba runtime opcional `partial-retrieval` y requisitos científicos/de producción. Docker y DB aislada ya se utilizaron; no son bloqueos actuales de la unidad backend.
- **Histórico:** mockups retirados y plan React anterior.

Los estados siguientes provienen de **evidencia documental/reportada**, salvo las observaciones propias indicadas. No se volvieron a ejecutar tests para confeccionar estas tablas. Lee Tasks/Log del documento enlazado para el alcance exacto; sus actualizaciones posteriores prevalecen sobre esta fotografía.

## Trabajo actual

| Entrega / documento | Estado | Qué está entregado | Qué falta |
| --- | --- | --- | --- |
| [MVP sintético urgente](backend/urgent-synthetic-mvp.md) | **Listo — backend en alcance probado** | Seis perfiles; API/eventos/persistencia; null/vacío rechazados; UTC y suites SQL/vet PASS; E2E autenticada aislada job12 exit 0, cinco perfiles publicables, legacy/idempotencia/auth y limpieza propia observados | `partial-retrieval` runtime SKIP y advisories del harness; selector/browser y React/PWA no implementados; pila existente no actualizada; sin validación científica ni aprobación de producción |
| [Demo local: pila + selector](demo/local-stack-and-scenario-selector.md) | **Parcial — pila migrada y selector implementado** | Migraciones `003`/`007` aplicadas a las bases locales **reales** (jobs `1 2 3`, risk `1 2 3 4 5 6 7`), columnas nuevas nullable, conteo de filas idéntico, 0 backfill y rerun idempotente; verificación independiente V1–V4/V6 PASS con dos correcciones registradas; selector de escenario en la página con 42 pruebas frontend PASS | Recorrido de navegador con sesión real y catálogo **autenticado** pendientes (requieren credenciales de demostración); `identity` solo se inició, no se recreó, y las marcas de tiempo de imagen no prueban procedencia por sí solas |
| [Plan frontend React](frontend/react-frontend-plan.md) | **Listo — documentación** | Entrega documental con recheck independiente PASS acotado | Implementación React/PWA no autorizada por ese trabajo y no realizada |
| [Checklist urgente por unidades](../urgent/README.md) | **Parcial** | Guía y organización de cinco unidades | Verificar requisitos y reunir evidencia real; los 132 IDs no están declarados cumplidos |

**Evidencia backend — observada por la sesión propietaria:** corrección UTC con recheck independiente PASS; migraciones y suites completas jobs/risk con roles app/catálogos y vet exit 0; harness con revisión independiente de fuente y 17 tests ejecutados por el padre; E2E Keycloak/API/RabbitMQ/PostGIS aislada exit 0 (~65 s). Rechazo de null/vacío resuelto. Prueba runtime opcional `partial-retrieval` SKIP; límites documentados y fallos históricos preservados. Recursos desechables propios limpiados, pila existente intacta. Esto no cierra los 132 IDs ni aprueba producción.

## Backend y datos

| Documento | Estado | Qué está entregado | Qué falta |
| --- | --- | --- | --- |
| [Aplicación funcional e integración](backend/firewatch-app-funcional.md) | **Parcial** | Slices funcionales y L0–L4 documentados | Resolver/reflejar brecha A.2 y cerrar comprobaciones OIDC/races/live restantes |
| [Contrato de datos y clasificación](backend/firewatch-data-contract.md) | **Parcial / por verificar** | Contrato, clasificador y persistencia con evidencia registrada | Reconciliar checklist/evidencia A.3 contradictoria y completar smoke/API viva/E2E; no asumir cierre solo por una entrada del Log |
| [Mapa geoespacial](backend/firewatch-geospatial-map.md) | **Parcial** | M1–M5 declaradas cerradas | Render en navegador y E2E gateway/Keycloak; resolver alcance de evidencia A.2b refutada |

## Seguridad

| Documento | Estado | Qué está entregado | Qué falta |
| --- | --- | --- | --- |
| [Plan de hardening backend](security/backend-hardening.md) | **Pendiente — implementación** | Plan y criterios de 12 unidades | BH-01–BH-11 pendientes; BH-12 diferida. El plan no acredita scans ni implementación |

### Unidades de hardening

Cada unidad tiene un documento de planificación; su estado indica la **ejecución**, no la existencia del archivo.

| Unidad | Estado | Qué falta |
| --- | --- | --- |
| [BH-01 — Toolchain y baseline](security/backend-hardening/01-toolchain-baseline.md) | **Pendiente** | Ejecutar y verificar la unidad |
| [BH-02 — Vulnerabilidades e imágenes](security/backend-hardening/02-vulnerability-scans.md) | **Pendiente** | Ejecutar scans y registrar resultados |
| [BH-03 — Cuotas gateway](security/backend-hardening/03-gateway-rate-limits.md) | **Pendiente** | Implementar y probar aislamiento de cuotas |
| [BH-04 — Mínimo privilegio del broker](security/backend-hardening/04-broker-least-privilege.md) | **Pendiente** | Configurar y verificar permisos con autorización del entorno |
| [BH-05 — Seguridad de transporte](security/backend-hardening/05-transport-security.md) | **Pendiente** | Implementar y verificar controles de transporte |
| [BH-06 — Egress Kubernetes](security/backend-hardening/06-kubernetes-egress.md) | **Pendiente** | Resolver entorno/routing y verificar restricciones |
| [BH-07 — Recuperación de jobs](security/backend-hardening/07-job-recovery.md) | **Pendiente** | Acordar política y verificar recuperación/retries |
| [BH-08 — Errores OpenAPI](security/backend-hardening/08-openapi-errors.md) | **Pendiente** | Verificar paridad de errores con el contrato |
| [BH-09 — Autenticación negativa](security/backend-hardening/09-authentication-tests.md) | **Pendiente** | Completar pruebas negativas de autenticación |
| [BH-10 — Idempotencia concurrente](security/backend-hardening/10-concurrent-idempotency.md) | **Pendiente** | Pruebas concurrentes en entorno aislado |
| [BH-11 — Seguridad de despliegue](security/backend-hardening/11-deployment-security-verification.md) | **Pendiente** | Verificar configuración y prerrequisitos de despliegue |
| [BH-12 — Errores centralizados](security/backend-hardening/12-centralized-error-handling.md) | **Pendiente — diferida** | Activación explícita antes de implementar |

Los estados BH corresponden al plan registrado; no se infiere que cambios de otros trabajos cierren estas unidades sin actualizar sus criterios y evidencia.

## Frontend

| Documento | Estado | Qué está entregado | Qué falta |
| --- | --- | --- | --- |
| [Página y propuesta para cliente](frontend/client-proposal-page.md) | **Parcial** | Dashboard, código PKCE/logout y checks registrados | CP-1/CP-2 abiertos y aceptación browser/Keycloak vivo |
| [Plan React actual](frontend/react-frontend-plan.md) | **Listo — documentación** | Plan y verificación documental acotada | React y PWA todavía no implementados |
| [Plan técnico de migración](../../frontend/docs/react-migration-plan.md) | **Listo — documentación** | Inventario, propuesta React/TypeScript/Vite/Node/npm, arquitectura y matriz | Autorizar y ejecutar migración; PWA es propuesta condicional, no funcionalidad actual |
| [Plan React anterior](frontend/frontend-react-vite-migration.md) | **Histórico** | Plan para el mockup retirado | No usar sus tareas como parity de la página actual; utilizar el plan técnico actual |
| [Mockup de producto](frontend/frontend-product-mockup.md) | **Histórico** | Antecedente de diseño del mockup retirado | No contar como producto servido actualmente |
| [Evolución del mockup preventivo](frontend/frontend-mockup/README.md) | **Histórico — diseño** | Backlog y planificación FWP-C02 | FWP-C01 y FWP01–08 no equivalen a funcionalidad entregada; requieren revisión de alcance antes de retomarse |

**Observación propia previa:** la suite frontend existente se ejecutó en esta sesión y dio exit 0, 40 tests aprobados, sin skips. Eso no demuestra login/logout real, visual móvil, accesibilidad formal ni integración live; no se repitió para este índice.

## Arquitectura

| Documento | Estado | Qué está entregado | Qué falta |
| --- | --- | --- | --- |
| [Separación gateway / frontend](architecture/gateway-frontend-separation.md) | **Parcial** | Separación implementada; T11/T12 cerradas en documento | Browser/Keycloak vivo y límites históricos de baseline/manifiesto no resueltos; no tomar el encabezado antiguo como única fuente |

## Revisiones

| Documento | Estado | Qué está entregado | Qué falta / límite |
| --- | --- | --- | --- |
| [Judgment Day backend — ronda 1](reviews/judgment-day-backend-round1.md) | **Listo — revisión acotada** | Veredicto APPROVED registrado para su lote | No cierra PostGIS/RabbitMQ/Keycloak live ni riesgos fuera del lote; Docker limitó evidencia |
| [Judgment Day frontend — ronda 2](reviews/judgment-day-frontend-round2.md) | **Listo — revisión acotada** | APPROVED y 40/40 registrados | Mobile 390 px, login/logout browser y accesibilidad no demostrados por ese resultado |

## Presentación

| Documento | Estado | Qué está entregado | Qué falta |
| --- | --- | --- | --- |
| [Preparación de presentación MVP](presentation/presentacion-mvp.md) | **Listo — documentación** | Cinco unidades y guía preparadas | Ejecutar/verificar backlog y confirmar demo; no aprueba los 132 IDs ni producción |

## Publicación

| Documento | Estado | Qué está entregado | Qué falta |
| --- | --- | --- | --- |
| [Preparación del repositorio para publicación](publication/github-repo-prep.md) | **Listo — baseline verificado** | Árbol a commitear limpio de secretos y artefactos pesados; `.gitignore` con `tmp/`; READMEs reconciliados; verificador de enlaces en verde (0 rotos); commit base único; acceso SSH del equipo al repositorio remoto | `push` y publicación siguen siendo decisión del usuario; re-baseline de `code-explanation/` pendiente de autorización explícita |

## Estado de la organización

| Entrega | Estado | Qué falta |
| --- | --- | --- |
| Traslado de documentos por área | **Listo** | Los 9 documentos viven en `backend/`, `security/`, `frontend/`, `reviews/` y `presentation/`; este índice apunta a las rutas nuevas |
| Referencias con rutas antiguas en superficies ajenas | **Pendiente** | `odd/urgent/README.md` conserva rutas relativas antiguas; su sesión propietaria las actualizará con la tabla old→new de abajo |
| Duplicado residual en la raíz | **Pendiente** | `odd/tasks/backend-hardening/` sigue en la raíz y debe eliminarse; su copia verificada vive en `security/backend-hardening/`. Decisión humana pendiente: no se consolidó ni se borró |
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
