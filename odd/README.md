# odd/ — mapa de la documentación

**Propósito:** un solo punto de entrada. Cada carpeta responde una pregunta distinta; este archivo dice cuál abrir y cuál no leer como estado vigente. Última revisión: **2026-10-20**.

## Empieza por aquí

| Si vienes a… | Abre | Qué encuentras |
|---|---|---|
| Preparar la reunión con el mandante | [`presentacion-mandante.md`](presentacion-mandante.md) | Guion en lenguaje simple: qué funciona, qué falta, qué se puede y qué no se puede decir |
| Saber cuánto falta de los 132 requisitos | [`estado-requerimientos-cliente.md`](estado-requerimientos-cliente.md) | Corte del 2026-10-20: 60 parciales, 66 bloqueados, 5 pendientes, 1 propuesto, 0 cerrados, y qué artefacto falta por dominio |
| Saber qué construir y en qué orden | [`plan-produccion/README.md`](plan-produccion/README.md) | 15 frentes hacia producción, un archivo por frente; el estado consolidado está en su §10 |
| Saber qué se ejecutó y qué se verificó | [`tasks/README.md`](tasks/README.md) | Índice por área, con estado, qué está entregado y qué falta en cada entrega |
| Leer el pedido original del cliente | [`../Referencias/tareas_verificacion_implementacion_IA.md`](../Referencias/tareas_verificacion_implementacion_IA.md) | Los 132 IDs (72 de implementación, 60 de verificación) con su criterio de aprobación |
| Hacer un cambio en el repositorio | [`PR-CONVENTIONS.md`](PR-CONVENTIONS.md) | Flujo issue-first, etiquetas, plantilla y nombres de rama |
| Comprobar que la documentación no tiene enlaces rotos | [`tools/check_md_links.py`](tools/check_md_links.py) | Verificador de enlaces relativos; no toca la red ni reescribe archivos |

## Las cuatro carpetas

| Carpeta | Qué es | Fecha y estado | Entrada |
|---|---|---|---|
| [`plan-produccion/`](plan-produccion/) | Plan hacia la versión de producción: 15 frentes, cada uno con su spec, tareas y log. Es el «qué falta y en qué orden» | Vivo; el estado consolidado es su §10 (actualizado el 2026-10-18) | [`README.md`](plan-produccion/README.md) |
| [`tasks/`](tasks/) | Registros de ejecución ODD: qué se planificó, qué se hizo y con qué evidencia. Es el historial, no el plan | Vivo; se actualiza al cerrar cada entrega | [`README.md`](tasks/README.md) |
| [`urgent/`](urgent/) | Cola de planificación del 2026-10-14 para la reunión con el jefe de proyecto: cinco unidades U1–U5 que reparten los 132 IDs | **Foto histórica**, corregida el 2026-10-20; el corte vigente es `estado-requerimientos-cliente.md` | [`README.md`](urgent/README.md) |
| [`tools/`](tools/) | Scripts de verificación. Hoy solo el verificador de enlaces | Vivo | [`check_md_links.py`](tools/check_md_links.py) |

## Cómo se relacionan

```text
Referencias/  →  estado-requerimientos-cliente  →  urgent/  →  plan-produccion/  →  tasks/  →  presentacion-mandante
(el pedido)      (el corte de hoy)                 (unidades)   (frentes y orden)    (ejecución)  (comunicación)
```

Cada eslabón responde una pregunta distinta y **no reemplaza al anterior**. Si dos documentos discrepan, gana el más específico: para un ID, su fila en `backend/docs/checklist-evidence-u*.md`; para un frente, `plan-produccion/README.md` §10; para una entrega, el `## Log` de su documento en `tasks/`.

**`plan-produccion/` no es el checklist del mandante.** Los 132 IDs de `Referencias/` son los criterios de aceptación; `plan-produccion/` es el orden de ingeniería para llegar a producción. Cita esos IDs, pero además incluye trabajo que el checklist no pide (firma y promoción de imágenes, motor de datos, DIFROL, hardening) y deja explícitamente fuera la fuente real y el modelo entrenado. Por eso ejecutar los 15 frentes no marca por sí solo un ID, y cerrar los 132 tampoco equivale a terminar el plan.

## Qué es histórico y no debe leerse como estado

- [`urgent/rediseno-pagina/`](urgent/rediseno-pagina/) y su [`referencia-mockup/`](urgent/rediseno-pagina/referencia-mockup/MANIFEST.md): rediseño y mockup retirados, reemplazados por la aplicación React. Se conservan como referencia visual, no como producto servido.
- [`tasks/frontend/frontend-mockup/`](tasks/frontend/frontend-mockup/README.md), [`tasks/frontend/frontend-product-mockup.md`](tasks/frontend/frontend-product-mockup.md) y [`tasks/frontend/frontend-react-vite-migration.md`](tasks/frontend/frontend-react-vite-migration.md): planificación del mockup anterior.
- La sección «Trabajo actual» de [`tasks/README.md`](tasks/README.md) resume documentos anteriores a la entrega de presentación. El estado vigente por frente está en `plan-produccion/README.md` §10.

El resto se lee con la columna **Estado** del índice de `tasks/`: ese índice distingue Listo, Parcial, Pendiente, Bloqueado, Histórico y Por verificar.

## Dónde va un documento nuevo

1. **Trabajo por ejecutar** → `plan-produccion/NN-nombre.md`, con su fila en la tabla §7 del README de esa carpeta.
2. **Registro de una ejecución** → `tasks/<área>/nombre.md`, con su fila en `tasks/README.md` (estado, qué está entregado, qué falta).
3. **Corte de estado con fecha** → raíz de `odd/`, con la fecha en el título, y el puntero actualizado en este mapa.
4. **Material para una audiencia** (mandante, equipo) → raíz de `odd/`, como `presentacion-mandante.md`.
5. **No crear un segundo índice.** Este mapa orienta y `tasks/README.md` indexa; los estados viven en un solo lugar y los demás documentos apuntan a él.

## Verificación

```powershell
python -B odd/tools/check_md_links.py
python -B odd/tools/check_md_links.py --self-test
```

Exit code `0` significa cero enlaces relativos rotos. El verificador lee el árbol de trabajo, no `HEAD`, y avisa cuando ambos difieren.

## Regla de lectura que vale para todo `odd/`

Un estado **no** es una casilla marcada. Una prueba en verde observa un comportamiento acotado y no cierra un requisito del cliente. Cerrar un ID exige evidencia reproducible, umbrales acordados, un responsable con nombre y, cuando el criterio lo pide, datos reales. Por eso puede haber pruebas en PASS y, aun así, `0 de 132` cerrados.
