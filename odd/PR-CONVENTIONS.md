# Convenciones de PR — FireWatch

**Decisión registrada el 2026-10-14:** este repositorio mantiene el **flujo liviano** de PRs — rama más descripción. **No** se adopta el flujo issue-first de la skill `gentle-ai-branch-pr`.

## Qué significa en la práctica

- Una rama por unidad de trabajo, nombrada `area/descripcion` (por ejemplo `backend/urgent-synthetic-closure`, `frontend/react-vite`, `ci/ct-gates`).
- El PR se abre contra `main` con una descripción que declara: qué cambia, evidencia observada, límites, y el resultado de los checks con sus run IDs.
- **No** se crean issues, ni se aplican etiquetas `type:*` o `status:approved`, ni se usa plantilla de PR.

## Por qué se decidió así

La skill de PRs exige cuatro cosas que este repositorio no tiene. Crearlas sería una decisión de política que no se tomó:

| Requisito de la skill | Estado verificado del repositorio |
| --- | --- |
| Enlazar un issue con `status:approved` | **Cero issues**; la etiqueta no existe |
| Exactamente una etiqueta `type:*` | **No existe ninguna**; solo las 10 por defecto de GitHub |
| `.github/PULL_REQUEST_TEMPLATE.md` | **No existe**; `.github/` solo contiene `workflows/` |
| Rama `type/descripcion` | La convención real del repo es `area/descripcion` |

Ninguna de esas ausencias bloquea un merge: no hay workflow que valide issues ni nombres de rama. Los PRs #1 y #3 se fusionaron y se abrieron con este flujo.

## Si más adelante se quiere adoptar el flujo de la skill

Se hace una sola vez, en un PR de configuración: crear las etiquetas `type:*` y `status:approved`, agregar `.github/PULL_REQUEST_TEMPLATE.md`, y decidir si se añade validación de nombre de rama en CI. Se aplica desde el PR siguiente; no se retrofitean PRs ya abiertos.
