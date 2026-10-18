# T17 — informe de verificación end-to-end

**Veredicto: PARCIAL — no certificar todavía la entrega completa.**
Código funcional verificado hasta `4ea6142`; los commits posteriores hasta `aba34fe` cambian solo `odd/tasks/presentation-client-ready.md`. No se modificó código de producto desde esta sesión, ni se hicieron commits, push o merges. Las correcciones las hizo la sesión autorizada `01a14b07`.

## Veredicto por spec

| Spec | Veredicto | Evidencia / límite |
| --- | --- | --- |
| S1 | Parcial | Recorrido principal local demostrado, pero falta prueba concluyente del runtime configurado con v2. No declarar entrega totalmente lista. |
| S2 | Parcial | Ingesta sintética, clasificación/publicación v1, historial, ciclos, catálogo y activación/reversión transaccional demostrados. No se consiguió demostrar una inferencia del proceso con `MODEL_ID=firewatch-rule-v2`. |
| S3 | Parcial; flujos ejecutados PASS | Backend, prevención durable y Chrome funcionan en los recorridos descritos. La prueba global no está cerrada por la falta anterior y no es una certificación integral de accesibilidad. |
| S4 | No reevaluada | La coordinación/worktrees corresponde a la implementación previa, no a este recorrido. |
| S5 | No reevaluada | No se volvió a auditar la limpieza documental original. Los artefactos de esta verificación están en `tmp/t17-verification/`, fuera de Git. |
| S6 | Respetada | Datos/clasificadores sintéticos; sin afirmaciones de modelo entrenado, calibración científica ni despliegue productivo. |

## Comprobaciones observadas

- Docker disponible. La pila habitual `firewatch` se conservó sin reinicios, cambios de datos ni actualización de imágenes.
- `make -C backend integration`: **exit 127**, `make: command not found`. Se ejecutaron sus tres comandos Compose equivalentes en un proyecto/bases aislados: `jobs-test`, `risk-test`, `prevention-test`, todos **exit 0** (job1).
- Suites Go de gateway/jobs/risk/prevention: **exit 0**, con **23 pruebas DB omitidas** en la ejecución nativa (job2). No se cuentan esas omisiones como PASS de integración.
- Retest de `8130791` con PostgreSQL real: `risk-test -v` **exit 0**; `TestActivateModelMovesTheSingleActiveVersionAndKeepsOneActive` **PASS**, no omitido (job6).
- Ledger compartido: versiones **1–10**, tablas `prevention.actions` / `prevention.action_history`, y `has_table_privilege('risk_app','models','UPDATE') = true` (job7).
- Recorrido sintético autenticado: catálogo, GeoJSON, rechazo viewer, entradas inválidas sin cambios públicos, cinco escenarios publicados, fixture heredada, idempotencia y no publicación de recuperación parcial durante la ventana acotada (job3, antes del fallo original de activación).
- Ciclo manual admin **200 complete:true**, zonas publicadas, nuevo lote visible en historial y frescura; viewer **403** sin alterar historial (job7).
- Activación v2 **200**, exactamente una versión activa; reversión v1 **200**, exactamente una activa; `approved_at` presente (job7).
- Prevención: admin crea **201/propuesta/historial vacío**; viewer **403** sin crear. Rechazo **409 invalid_transition** conserva detalle/historial completos. Nueve transiciones válidas producen nueve entradas; estados aceptada y cancelada rechazan nuevas transiciones sin cambios durables. Rechazo de transición viewer tampoco escribe (job7).
- Métricas reales gateway/jobs: buckets, sum, count, errores y señales de proceso; jobs conserva broker/DLQ (job7). No se confunde esto con corregir el denominador de la alerta T20.
- **Chrome job11, build `4ea6142`: exit 0; BROWSER FAILURES: []**. PKCE admin/viewer real, modelo/frescura, lista/alta/detalle, transición con nota e historial numérico real, viewer deshabilitado, logout RP y limpieza de sesión. Request IDs reales de respuestas **401 persistente y 403** visibles en UI.
- Móvil: documento y viewport de **390 px**, sin overflow horizontal; recorrido de **30 Tab** con foco visible en controles observados. No equivale a auditar toda combinación de teclado o tecnología asistiva.
- Contraste de texto renderizado: **124 nodos**, **0 fallos**, **0 omitidos** dentro del conjunto medido; decorativos/aria-hidden y controles deshabilitados excluidos por definición. No es una certificación WCAG completa ni una corrida axe.
- Frontend job12: **307/307**, lint, typecheck y gate de honestidad **exit 0**. Build de job11 **exit 0**.

## Ciclo automático observado, no simulado

Logs `risk` con `cycle_complete`, `zones:1`:

- `2026-10-18T15:55:47.841946712Z`
- `2026-10-18T16:10:47.803802942Z`

Separación aproximada: **899.962 s**. Lectura posterior: `last_run_at=16:10:47.803800738Z`, `next_run_at=16:25:47.803800738Z`, `complete=true`, `age_seconds=7.9353`.

La ejecución manual previa acabó a `15:56:13.392299665Z`: el automático ocurrió **874.4115 s** después y **25.5885 s antes** del `next_run_at` recalculado tras el manual. El ticker conserva su fase de arranque; `next_run_at` deriva de la última ejecución + intervalo, no es necesariamente el próximo disparo real. La etiqueta de la demo debe distinguirlo.

**Job7 conserva exit 1** por el fallo de captura de salida de navegador UTF-8/cp1252 de la sonda original. Su fase de scheduler sí pasó; la prueba de navegador corregida es **job11 exit 0**. No presentar job7 como arnés completamente verde.

## Defectos reproducidos y corregidos por la otra sesión

| Defecto público | Corrección | Retest observado |
| --- | --- | --- |
| Admin activa v2 y obtiene `503 storage_unavailable` | `8130791`, migración 010 con GRANT UPDATE | DB job6 y HTTP job7 PASS |
| Token admin+viewer crea acción y obtiene 403 según orden de roles | `3dd04a3`, comprobación por pertenencia | Admin201/viewer403 y estados durables job7 PASS |
| UI rechaza lista200/alta201 como `invalid_shape` (`assignee:null`, historial.id numérico) | `166c2d0`, lectores/tipos/fixtures reales | Interacción con historial numérico job11 PASS |
| Contraste de texto 4.44/4.39 y footer3.53, por debajo de4.5 | `4ea6142`, token muted/footer | 124 textos renderizados sin fallos, job11 PASS |

## Falta de prueba: runtime v2

- Job13 observó registro activo v2 y job nuevo publicado con v1. Su aserción de intercambio en caliente era una **suposición incorrecta de la sonda**, no un requisito demostrado ni un defecto del registro. El registro se revirtió en `finally`.
- Job14, en otra pila, confirmó con lotes reales que cambiar el registro y **reiniciar sin cambiar `MODEL_ID`** conserva `academic-rule-v1`. La afirmación anterior de que bastaba reiniciar para adoptar la fila activa fue retractada por la sesión de implementación; el arranque selecciona `MODEL_ID` o v1 por defecto.
- Después, job14 detuvo su contenedor risk y creó otro con `MODEL_ID=firewatch-rule-v2` y alias risk. La espera de API por gateway **agotó 240 s**, y el job salió **1**.
- **No se conservó el log/inspect de la nueva instancia `risk-v2` antes del cleanup**. Se conservaron los del contenedor viejo y prevention. Por tanto no se puede distinguir una falla del proceso v2 de un problema de sustitución/alias/conectividad de la sonda. No atribuir causa ni publicar un defecto v2 como probado.
- Hash de v2 en migración 008 y constante de código coinciden en la lectura estática; eso no sustituye la prueba runtime.
- No se reintentó automáticamente tras el timeout.

**Necesita decisión:** autorizar una nueva corrida diagnóstica acotada, manteniendo cada instancia y capturando estado, env no secreto, red y logs de todas las instancias antes de limpiar, para resolver esta falta de prueba. No declarar T17 completo mientras siga abierta.

## Límites y recursos

- Navegador en `http://localhost:4173`. Se reenviaron peticiones reales desde los orígenes fijos localhost:8080/8081 a puertos efímeros propios, sin respuestas simuladas, para no alterar la pila habitual. El issuer y CSP del frontend se conservaron. No se demostró el despliegue final con los puertos habituales ocupados por la pila actual.
- Playwright1.64.1 disponible vía npx sin instalación; Chrome155.0.8059.84 por canal chrome. El headless shell empaquetado no estaba instalado. Axe no estaba disponible sin instalar paquetes; no se instaló.
- Las falsas alarmas de sondas (ráfaga429, búsqueda literal de histogram, selectores de vista/estado equivocados, foco nativo inferido solo del outline del host, 401 puntual supersedido y decodificación cp1252) se conservaron como fallos de prueba, no como defectos de producto.
- Contenedores de las pilas de verificación ya eliminados por ownership; la pila habitual conserva sus seis contenedores. El volumen nombrado creado por job1 fue retirado tras comprobar su creación en el log, etiqueta de proyecto y ausencia de referencias. No se ejecutó prune ni se afirma haber auditado todos los volúmenes anónimos del host.
- `backend/go.work.sum` estaba modificado al llegar y se preservó. No quedan cambios de código de producto hechos por este verificador.

## Artefactos y comandos

- `tmp/t17-verification/runtime.py`: sonda API, ledger, métricas y scheduler; `T17_OBSERVE_SCHEDULE=1 python -u tmp/t17-verification/runtime.py`.
- `tmp/t17-verification/browser_existing.py`: navegador sobre una pila propia todavía viva; `python -u tmp/t17-verification/browser_existing.py`.
- `tmp/t17-verification/browser.cjs`, `browser-retest.log`, `browser-readonly.log`, `browser-events.jsonl` y capturas PNG.
- `tmp/t17-verification/model_runtime.py`: la sonda de reinicio/v2 que terminó por timeout; **no relanzarla sin resolver la decisión anterior**.
- Logs completos de jobs1–14: directorio de salida de herramientas `C:/Users/rikar/AppData/Local/Temp/gpoI7sjE/gentle-jobs-VJNOyF/`. Son evidencia de esta sesión, no archivos versionados ni un gate CI permanente.
