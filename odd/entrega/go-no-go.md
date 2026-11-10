# GNG-I1 — Documento Go/No-Go

**ID:** `GNG-I1` — *«Completar el historial de versiones del documento (fecha, versión, autor, descripción)»* (`Referencias/tareas_verificacion_implementacion_IA.md:462`), dentro de la sección de aprobación final, donde la decisión queda **firmada por un responsable** (`:459`).
**Estado:** documento vivo. **No declara aprobación de producción.** Las casillas de aprobación `GNG-V1` a `GNG-V5` siguen sin marcar.
**Autoridad de alcance:** `odd/tasks/v1-produccion.md` §S2 y §S4; acta de cierre `odd/entrega/acta-cierre.md` §5-§6.

## Historial de versiones

| Fecha | Versión | Autor | Descripción |
| --- | --- | --- | --- |
| 2026-11-09 | 0.1 | Equipo de desarrollo FireWatch | Creación del documento con el marco de decisión, la matriz de pruebas con su estado real y los bloqueos vigentes. **No emite decisión** |

## 1. Qué es y qué no es este documento

Es el registro donde una decisión Go/No-Go **podría** quedar tomada y firmada, con la evidencia disponible en el momento, tal como exige la
fuente (`Referencias/tareas_verificacion_implementacion_IA.md:459`).

**No es una aprobación.** Este documento se crea para que la decisión tenga un lugar donde ocurrir y un formato verificable. Hoy:

- No hay decisión APROBADO ni RECHAZADO.
- No hay firma de responsable.
- No se declara aptitud para producción, ni desempeño del modelo, ni validez científica.

Esas tres frases están aquí para que nadie cite este archivo como si dijera lo contrario.

## 2. Criterio de decisión

La decisión se toma **con la evidencia disponible**, y una fila sin evidencia no es una fila neutra: cuenta como no cumplida. El resultado
posible por fila es `Cumple`, `No cumple` o `No verificado`, y `No verificado` bloquea igual que `No cumple` mientras no se cierre.

| Prueba | Criterio | Evidencia en el repositorio | Resultado |
| --- | --- | --- | --- |
| Contratos HTTP y de eventos | Cada cambio de contrato tiene contract test que compara contra la respuesta real | `backend/contracts/openapi/firewatch-v1.yaml` y las suites por servicio en CI | Cumple |
| Esquema de datos y migraciones | Las migraciones se aplican y se reaplican sin error sobre una base aislada, y dos servicios no reclaman la misma versión | Guard de ledger en CI y su prueba de colisión (`backend/docs/ci-migration-readiness.md`) | Cumple |
| Seguridad de dependencias e imágenes | Sin vulnerabilidades conocidas en dependencias Go ni en las imágenes publicadas | `govulncheck` y escaneo de imágenes en CI (`backend/docs/architecture.md:60-61`) | Cumple con reserva: `BH-02` sigue parcial por el inventario y el escaneo de imágenes end-to-end |
| Latencia p95 | `p95 < 500 ms`, `max < 2000`, `http_req_failed < 0.01` bajo carga representativa (k6) | Ninguna corrida de k6 registrada todavía | **No verificado** — frente 6 abierto |
| Medición de señales operativas | Un scrape real registra RPS, p95, memoria y errores, y las reglas validan | `backend/docs/observability-measurement.md` (2026-11-09): RPS 8.95/5.84/2.98/2.95, p95 ≤ 4.75 ms, errores 0, `promtool` 7 reglas OK | Cumple, con el límite declarado: sin token, lo medido es el middleware |
| Regresión del modelo | Un modelo candidato no degrada frente a la versión en producción sobre datos con ground truth | No existe ground truth ni modelo real | **No verificado** — frente 2 y frente 12 bloqueados por S3 |
| Calidad científica del modelo | Métricas, tolerancias y protocolo escritos y ejecutados contra datos reales | El análisis de datos entregados concluye que no alcanzan (`backend/training/ANALYSIS-delivered-data.md` §4) | **No verificado** — bloqueado por S3 |
| Firma y promoción de imágenes | Una imagen firmada se verifica antes de habilitar su promoción, y el rollout se verifica con su rollback | Ninguna de las dos existe (`backend/deploy/prod/README.md:113`) | **No cumple** — frente 7 abierto |
| Respuesta a incidentes | El plan existe, está nombrado y hubo simulacro | Plan redactado (`odd/security/incident-response.md`); sin nombres (D3) y sin simulacro | **No verificado** — `RED-V1` abierto |
| Accesibilidad y experiencia | Criterios WCAG AA medidos por criterio, dentro del alcance que fije D5 | Sin medición registrada; D5 abierta | **No verificado** — frentes 9 y G1 abiertos |
| Interfaces del producto | La interfaz declarada existe y funciona | Siete tareas `V1-P-01…07` abiertas en `odd/tasks/v1-produccion.md` | **No cumple** — G6 abierto |

## 3. Estado de los frentes de cierre

Resumen del único inventario vinculante: `odd/tasks/v1-produccion.md`. **46 tareas, 6 cerradas** (`V1-F-05`, observabilidad medida y reglas validadas, más `V1-C-01`…`V1-C-05`, el cierre documental), 40 abiertas, de las cuales 6 bloqueadas por insumo externo o decisión humana:

| Frente | Tema | Estado |
| --- | --- | --- |
| G1 | Diseño web | Abierto — 5 tareas |
| G2 | Usuarios y asignación | Abierto — 4 tareas |
| G3 | Modelo de datos | Abierto — 4 tareas, una bloqueada por D1 |
| G4 | Frentes de producción | Abierto — 12 tareas, 4 bloqueadas por S3/D1; `V1-F-05` cerrada |
| G5 | Hardening | Abierto — 7 tareas, `BH-12` bloqueada por activación explícita |
| G6 | Interfaz declarada no implementada | Abierto — 7 tareas |
| G7 | Cierre documental | En ejecución |

## 4. Bloqueos vigentes, nombrados

1. **S3 — datos reales.** Sin meteorología diaria en días sin incendio y sin acceso al registro de eventos de CONAF, el frente del modelo no
   se desbloquea y las filas de calidad y regresión quedan en `No verificado`.
2. **D1 — motor de datos productivo.** Decisión humana pendiente; condiciona `V1-M-03` y `V1-F-14`.
3. **D3 — responsables de gobierno.** El AI Security Officer y el comandante de incidente no tienen nombre; sin ellos no hay quién apruebe ni
   quién firme.
4. **D5 — alcance de accesibilidad.** Sin alcance escrito no hay criterio de aceptación para el frente 9.

## 5. Qué haría falta para emitir APROBADO

Las once filas de §2 en `Cumple`, el frente 7 con firma y promoción verificadas, el frente 6 con su reporte de k6 pasa/falla, y la firma de un
responsable nombrado. Nada de eso depende de este documento.

## 6. Qué haría falta para emitir RECHAZADO

Una sola fila en `No cumple` sin plan de corrección aceptado, o una medición que contradiga un criterio. Un `RECHAZADO` no es un fracaso del
proyecto: es la decisión correcta cuando la evidencia no alcanza, y hoy la evidencia no alcanza para un `APROBADO`.

## 7. Firma

```text
Decisión:            [ ] APROBADO PARA PRODUCCIÓN      [ ] RECHAZADO

Responsable:         ______________________________________

Nombre y rol:        ______________________________________

Fecha:               ______________________________________

Documento firmado sobre la evidencia de:  versión 0.1 de este documento
```

Vacía por decisión, no por olvido: `GNG-V1` a `GNG-V5` no se marcan hasta que exista un responsable nombrado y la evidencia de §2 lo sostenga.
