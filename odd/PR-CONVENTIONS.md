# Convenciones de PR — FireWatch

**Decisión vigente, 2026-10-15:** este repositorio **adopta el flujo issue-first** de la skill `gentle-ai-branch-pr`. Reemplaza la decisión del 2026-10-14, que mantenía el flujo liviano; el porqué del cambio y lo que se conserva del registro anterior están abajo.

## Qué significa en la práctica

- **Un issue aprobado primero.** El PR enlaza un issue que ya lleva la etiqueta `status:approved`. `Closes #N` / `Fixes #N` / `Resolves #N` cuando el PR cierra el issue; `Refs #N` cuando no lo cierra.
- **Exactamente una etiqueta `type:*`** en cada PR, y su casilla marcada en la plantilla: `type:bug`, `type:feature`, `type:docs`, `type:refactor`, `type:chore` o `type:breaking-change`.
- **Plantilla obligatoria:** [`.github/PULL_REQUEST_TEMPLATE.md`](../.github/PULL_REQUEST_TEMPLATE.md).
- **Una rama por unidad de trabajo, nombrada `tipo/descripcion`** con el tipo del commit: `fix/…`, `chore/…`, `docs/…`, `feat/…`. Es un cambio respecto de la convención anterior (`area/descripcion`), porque el nombre de la rama ahora declara el tipo que el PR va a etiquetar.
- **Commits en Conventional Commits**, sin trailer `Co-Authored-By`.

## Qué se creó para que esto sea cierto

| Requisito de la skill | Estado anterior (2026-10-14) | Estado desde este cambio |
| --- | --- | --- |
| Issue con `status:approved` | Cero issues; la etiqueta no existía | Etiqueta `status:approved` creada |
| Exactamente una etiqueta `type:*` | No existía ninguna | Las seis `type:*` creadas |
| `.github/PULL_REQUEST_TEMPLATE.md` | No existía | Creada |
| Rama `tipo/descripcion` | La convención era `area/descripcion` | Convención declarada arriba |

Las etiquetas `type:*` y `status:approved` son **etiquetas de repositorio**, no de rama: existen desde que se crearon y por eso el PR que las acompaña ya puede usarlas. Este PR es el **bootstrap** que el propio documento anterior anticipaba («se hace una sola vez, en un PR de configuración; se aplica desde el PR siguiente»): es la única excepción admitida a la regla de issue aprobado, porque la regla todavía no existía cuando se abrió.

## Lo que este cambio NO hizo

- **No se agregó validación de nombre de rama en CI.** La decisión quedó abierta a propósito: nada en `.github/workflows/` la impone, así que la convención de nombres es una convención y no una garantía. Decirlo es más honesto que insinuar una aplicación que no existe.
- **No se retrofiteó ningún PR ya abierto ni fusionado.** Los PRs #1, #3 y #5–#15 quedan como están.
- **No se tocó ningún workflow.** Este cambio no dispara CI: los workflows escuchan `backend/**` y `.github/workflows/**`, y este PR solo toca `.github/PULL_REQUEST_TEMPLATE.md` y este documento.

## Por qué se cambió de decisión

El usuario lo pidió explícitamente para que el trabajo entrante calzara con la skill de PRs, y el documento anterior dejaba el camino escrito: crear las etiquetas, agregar la plantilla y decidir lo de CI. La ausencia de cualquiera de las tres no bloqueaba un merge —no hay workflow que valide issues ni nombres de rama—, así que adoptarlo es una decisión de política, no una reparación. Queda registrada como tal y no se presenta como un requisito técnico.

## Historia

El **2026-10-14** el repositorio decidió lo contrario: flujo liviano de rama más descripción, sin issues, sin etiquetas y sin plantilla, con la evidencia de que los PRs #1 y #3 se habían fusionado así. Esa decisión y su razonamiento siguen legibles en el historial de este archivo; dejó de estar vigente el **2026-10-15**, cuando el usuario eligió el flujo completo de la skill sobre la alternativa de mantener la convención liviana.
