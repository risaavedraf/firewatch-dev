# Acciones de CI fuera de Node 20

**Estado:** fusionado en `main` (PR #67, `4457d38`), con las ocho corridas de CI en verde.
**Origen:** el pendiente que L21 de `odd/tasks/kubernetes-deployment.md` deja fuera del frente de Kubernetes, autorizado en esta sesión.

## Specs

**S1.** «las acciones de CI siguen apuntando a Node 20 (`actions/checkout` en los cuatro workflows, `setup-python`, `setup-node`, `setup-go`) y `ubuntu-latest` migra a Ubuntu 26 en octubre de 2026. Rama propia, con su corrida.»

**S2.** La verificación es la corrida de CI sobre esta rama. Un YAML que parsea no acredita que los workflows ejecuten: el runtime de la acción y la etiqueta del runner solo se prueban en el runner.

## Tasks

| ID | Tarea | Spec | Rama | Estado |
| --- | --- | --- | --- | --- |
| T1 | Las cuatro acciones suben a la versión vigente que corre sobre `node24`, fijada por SHA como el resto del repositorio | S1, S2 | `chore/ci-actions-refresh` | hecho |
| T2 | `runs-on` deja de seguir `ubuntu-latest` y fija `ubuntu-24.04`, para que la migración a Ubuntu 26 sea un acto deliberado y no una corrida que cambia sola | S1, S2 | `chore/ci-actions-refresh` | hecho |

**Alcance.** Cuatro workflows (`frontend`, `integration`, `training`, `_go-service`) y once líneas. No se toca ninguna otra cosa de las acciones ni de los pasos.

**Ninguna rama se fusiona sin decisión humana.**

**Sobre el límite de 80 columnas.** `yamllint` corre aquí con sus valores por defecto y sin configuración en el repositorio, así que marca líneas que ya existen en `main`. Nueve de ellas son comentarios y continuaciones de shell en `training.yml` y se podrían reflowar; las dos de `_go-service.yml` no: fijan `aquasec/trivy:0.69.3@sha256:<64 hex>` dentro de una continuación de shell, y una línea que pinnea un digest de 64 caracteres no baja de 80 columnas en ninguna forma. Reflowar las nueve dejaría las dos imposibles en rojo igual, así que no se toca ninguna: `.yamllint.yml` declara la política real (120 columnas, aviso) para quien corra `yamllint` a mano, y las líneas quedan como están.

## Log

**L1 (2026-11-10) — autorización.** «puedes hacer todo en diferentes ramas porfa».

**L2 (2026-11-10) — evidencia de las versiones.** Cada versión se resolvió contra el repositorio de la acción y se comprobó su runtime antes de fijarla: `actions/checkout` v7.0.1, `actions/setup-python` v7.1.0, `actions/setup-node` v7.1.0 y `actions/setup-go` v7.0.0 declaran las cuatro `using: node24` en su `action.yml`. Los SHA fijados son los del commit al que apunta cada etiqueta (`refs/tags/<tag>^{}`), no el objeto de etiqueta anotada:

| Acción | Versión | SHA fijado |
| --- | --- | --- |
| `actions/checkout` | v7.0.1 | `3d3c42e5aac5ba805825da76410c181273ba90b1` |
| `actions/setup-python` | v7.1.0 | `6d4fd22b20465ccc0f7354476588e8df7c9ca0e0` |
| `actions/setup-node` | v7.1.0 | `949feb2413d6458794dcd2491c4babbbce0c15c1` |
| `actions/setup-go` | v7.0.0 | `b7ad1dad31e06c5925ef5d2fc7ad053ef454303e` |

El comentario de cada línea vuelve a nombrar solo el major (`# v7`), que es la convención que ya usaba el repositorio (`# v4`, `# v5`); el detalle exacto vive en esta tabla.

**L3 (2026-11-10) — por qué el runner se fija y no se deja en `ubuntu-latest`.** `ubuntu-latest` es un alias que GitHub mueve por su cuenta: cuando migre a Ubuntu 26, estos cuatro workflows cambiarán de sistema operativo en una corrida que nadie revisó. Fijar `ubuntu-24.04` convierte esa migración en un cambio con su propio PR y su propia corrida, que es lo mismo que S1 pide para las acciones. Migrar a 26 sigue pendiente y es una decisión aparte, no un efecto colateral.
