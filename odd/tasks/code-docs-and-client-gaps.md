# Documentación acorde al código y brechas frente al cliente

**Objetivo:** alinear `backend/docs/code-explanation/` con el árbol real y publicar un corte explícito de lo que el cliente pidió y sigue faltando, sin cerrar casillas ni certificar lo que no se releyó.
**Rama:** `docs/code-alignment-and-client-gaps` (desde `docs/reconcile-docs-odd-code` en `267a4bb`) · **Estrategia de entrega:** una rama, commits por unidad de trabajo · **Verificador:** `python -B backend/docs/code-explanation/verify_structure.py`, `python -B odd/tools/check_md_links.py`.

## Specs

- **S1** — «Quiero que la documentación esté acorde con lo que tenemos en codigo».

  Alcance medido: `verify_structure.py` reporta **165 errores** (114 `Uninventoried file`, 47 `Whole-file range mismatch`, 3 `forbidden link`, 1 `Source snapshot differs`) y el `README.md` de la colección declara **6 afirmaciones de contenido falsas**: migraciones (`06-database.md`: 1 `Initial` vs 3 en `jobs` y 7 en `risk`), readiness (`07-deployment.md`: `204` vs `200`/`503` en `jobs`/`risk`), reintentos (`04-messaging.md`: `1,2,4,8,16` vs `{1,2,4,8}`), interfaz `Repository` (`02-jobs.md`: falta `ZoneExists` y `Create` variádico), handler del broker (`03-risk.md`/`04-messaging.md`: `store.Publish` vs `publisher.Publish`) y preflight `OPTIONS` (`01-gateway.md`: rama `204` ausente). Cada una se re-verifica contra el código antes de reescribirse.

- **S2** — «marcar especificamente que es lo que nos falta que nos pidió el cliente».

  Fuente autoritativa del pedido: `Referencias/tareas_verificacion_implementacion_IA.md`, **132 IDs** (72 `I` y 60 `V`). Medición de partida: **132 de 132 IDs tienen exactamente una fila** en `backend/docs/checklist-evidence-u1.md`, `u2-u3.md`, `u4.md` y `u5.md`, con estados `Partial 60 · Blocked 66 · Pending 5 · Proposed 1` y **0 cerrados**. El corte consolidado no existe en un solo lugar: vive disperso entre esos cuatro registros, las cinco unidades de `odd/urgent/` y una tabla de `odd/urgent/README.md` fechada el **2026-10-14** que el código ya contradice (afirma que faltan el servicio `prevention`, Grafana, HPA y la herramienta de carga; los cuatro existen hoy, y `firewatch_high_risk_predictions_total` está en `backend/services/risk/internal/metrics/metrics.go:51`). Lo que sí sigue sin existir: trazas OpenTelemetry y firma de imágenes (0 coincidencias en el árbol).

- **S3** — Límite heredado que esta entrega no cruza: **ningún estado de ID se convierte en tarea cumplida**. `Partial` significa que existe un artefacto ejecutable en el repositorio que habla del ID, no que su criterio de cierre se haya alcanzado; `Blocked` significa que el cierre depende de un insumo que el repositorio no contiene (datos reales, umbral acordado, responsable nombrado, corrida de carga, clúster).

- **S4** — Límite de honestidad del verificador: una entrada solo puede declararse `Explicado` si su prosa se releyó contra el archivo. Lo no releído se declara `Parcial` o `Contexto` con su razón escrita. Regenerar `source-snapshot.json` sin ese paso quedaría prohibido por el propio `README.md` de la colección.

- **S5** — Fuera de alcance: no se toca la red de `odd/plan-produccion/` ni el despliegue en el servidor de Oracle; no se cierran los 132 IDs; no se ejecuta carga, clúster ni drift con ground truth.

- **S6** — **Convención de la re-derivación, decidida en L5.** Las citas de la colección son rangos dentro de un archivo nombrado en la frase anterior (`| mux/health, 96-107 |`). Un rango desplazado miente igual que una frase falsa, y parchear una celda deja el resto del documento desactualizado. Por eso cada documento se re-deriva completo: se relee cada archivo que cita, se reescribe cada rango contra el código de la rama, y cuando una afirmación ya no se sostiene se reescribe al comportamiento real o se declara retirada. Cada documento re-derivado lleva en su encabezado la fecha y el commit base `267a4bb` de la lectura. No se cambia el estilo de cita ni el verificador en esta entrega.

## Tasks

| ID | Spec | Ruta | Descripción | Estado | Commit |
|---|---|---|---|---|---|
| T1 | S2, S3 | `odd/estado-requerimientos-cliente.md`, `odd/urgent/README.md` | Corte consolidado de brechas y refresco de la tabla obsoleta | pendiente | — |
| T2 | S1 | `backend/docs/code-explanation/01..08*.md` | Seis afirmaciones de contenido corregidas contra el código | pendiente | — |
| T3 | S1, S4 | `backend/docs/code-explanation/README.md` | Inventario extendido (114) y estados ajustados (47) con su razón | pendiente | — |
| T4 | S1, S4 | `backend/docs/code-explanation/source-snapshot.json` | Snapshot regenerado y verificador en exit 0, con evidencia registrada | pendiente | — |
| T5 | S1, S6 | `backend/docs/code-explanation/01-gateway.md` | Re-derivación completa (13 citas; preflight `OPTIONS` ausente) | pendiente | — |
| T6 | S1, S6 | `backend/docs/code-explanation/02-jobs.md` | Re-derivación completa (7 citas; interfaz `Repository`) | pendiente | — |
| T7 | S1, S6 | `backend/docs/code-explanation/03-risk.md` | Re-derivación completa (`publisher.Publish`) | pendiente | — |
| T8 | S1, S6 | `backend/docs/code-explanation/04-messaging.md` | Re-derivación completa (45 citas; cronograma de reintentos) | pendiente | — |
| T9 | S1, S6 | `backend/docs/code-explanation/06-database.md` | Re-derivación completa (4 y 9 migraciones, no 1) | pendiente | — |
| T10 | S1, S6 | `backend/docs/code-explanation/07-deployment.md` | Re-derivación completa (readiness `200`/`503`) | pendiente | — |
| T11 | S1, S6 | `backend/docs/code-explanation/05-contracts.md`, `08-tests-tools.md` | Re-derivación completa de las dos partes restantes | pendiente | — |

## Log

- **L1 (2026-10-20)** — Petición original, verbatim: «ok, entonces arreglemos las cosas de la documentaci» y, al pedir profundidad: «Quiero que la documentación esté acorde con lo que tenemos en codigo, y marcar especificamente que es lo que nos falta que nos pidió el cliente».
- **L2 (2026-10-20)** — **Estado de partida medido.** Rama base `docs/reconcile-docs-odd-code` en `267a4bb`, árbol limpio, 9 commits sin publicar. `verify_structure.py`: 165 errores — desglose 114/47/3/1. `check_md_links.py`: 105 archivos, `BROKEN: 0`. `npm test` (frontend): 36 archivos, 358/358. `kubectl kustomize backend/deploy/k8s/base`: 4 `HorizontalPodAutoscaler`. Entorno: `promtool`, `kind`, `minikube`, `helm` y `k6` no instalados; daemon de Docker detenido.
- **L3 (2026-10-20)** — **Por qué la rama se apoya en la reconciliación y no en `main`.** `backend/docs/checklist-evidence-u4.md:70` afirma «one `HorizontalPodAutoscaler` per service», verdadero solo con el HPA de `prevention` que añade `2c6ea95`; y `checklist-evidence-u1.md` ya cita `architecture.md:57` con cuatro servicios. Basar esta entrega en `main` reintroduciría afirmaciones falsas o forzaría a repetir correcciones ya hechas.
- **L5 (2026-10-20)** — **La deriva de anclas es la causa real, medida.** Las 6 afirmaciones falsas declaradas no son frases aisladas: cada documento es una tabla de rangos relativos a un archivo que creció. Ejemplos verificados hoy: `01-gateway.md` sitúa el preflight en `gateway.go:88-96`, que hoy contiene la configuración del limitador; `07-deployment.md` sitúa el readiness en `main.go:96-107`, que hoy contiene el sembrado de zonas; `06-database.md` describe una única migración `Initial` cuando `services/jobs/migrations/` tiene 4 archivos y `services/risk/migrations/` tiene 9, y el runner recorre una lista de versiones (`jobs/cmd/server/main.go:52-90`, `risk/cmd/server/main.go:57-100`). En total hay **186 citas con rango** repartidas en los 9 documentos. De ahí S6: se re-deriva documento por documento; parchear celdas sueltas produciría documentos internamente incoherentes.

- **L6 (2026-10-20)** — **Corrección a la propia declaración del README.** El `README.md` de la colección afirma que el código tiene «**3** migraciones en `jobs` y **7** en `risk`». Hoy son **4** y **9**: la afirmación fechada también envejeció. Se corrige junto con las demás, con el conteo releído del árbol y no del texto anterior.

- **L4 (2026-10-20)** — **Los 132 IDs no se re-enumeran.** Cada uno ya tiene su fila con evidencia y razón de bloqueo en los cuatro registros; duplicarlos en un documento nuevo crearía dos fuentes de verdad. El corte nuevo aporta la agregación por estado y dominio, la lista de lo que falta y el puntero a la fila, no una copia.
