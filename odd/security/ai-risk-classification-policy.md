# GOB-I1 — Política interna de clasificación de riesgo de IA

**ID:** `GOB-I1` — *«Redactar la política interna de clasificación de riesgo (bajo / limitado / alto / inaceptable)»* (`Referencias/tareas_verificacion_implementacion_IA.md:60`), con roles que aprueben los modelos antes de producción (`:57`).
**Estado:** adoptada el 2026-11-09 por el responsable del proyecto.
**Regulación elegida:** Reglamento (UE) 2024/1689 — **EU AI Act**, con la escala de cuatro niveles que ya usaba el marco de referencia del proyecto (`Referencias/tareas_verificacion_implementacion_IA.md:57`).
**Documentos que esta política obliga:** [`atlas-threat-model.md`](atlas-threat-model.md), [`atlas-vector-mitigations.md`](atlas-vector-mitigations.md) e [`incident-response.md`](incident-response.md).

## 1. Objeto y alcance

Esta política fija cómo FireWatch clasifica un sistema de IA por nivel de riesgo, qué exige cada nivel y quién aprueba. Aplica a todo
componente que produzca una salida usada para decidir algo fuera del propio software: hoy el clasificador de riesgo de incendio, en
cualquiera de sus dos implementaciones (reglas o modelo logístico).

## 2. Escala

| Nivel | Definición | Ejemplo | Exigencia mínima |
| --- | --- | --- | --- |
| **Bajo** | No influye en una decisión sobre personas, bienes o servicios; su error es reversible y detectable por el usuario | Sugerencia de orden de una lista | Documentación del modelo y prueba de que no decide nada |
| **Limitado** | Influye en una decisión con un humano en el bucle que puede revertirla; el error es visible y corregible | Priorización de una cola que un operador reordena | Human in the loop, registro de la sugerencia y de su aceptación, métricas de calidad |
| **Alto** | Informa una decisión operativa sobre personas, bienes o infraestructura crítica; su error puede causar daño y no es evidente para quien recibe la salida | **Clasificación de riesgo de incendio que orienta prevención y despliegue de recursos** | Todo lo de «limitado», más: evaluación de riesgos documentada, modelo de amenazas, plan de respuesta a incidentes, monitoreo con alertas y aprobación previa a producción |
| **Inaceptable** | Prohibido por la regulación elegida, o incompatible con los valores declarados del proyecto | Identificación biométrica masiva, puntuación social | No se construye. Si existe, se retira |

## 3. Mapeo a la regulación elegida

**Autoevaluación, no dictamen jurídico.** FireWatch se clasifica **alto**.

- El sistema produce una salida (`bajo`/`medio`/`alto`/`desconocido`) que orienta decisiones de prevención de incendios forestales,
  una actividad de protección de infraestructura y de personas.
- Es análogo a la categoría de **infraestructura crítica** del Anexo III del EU AI Act: componentes de seguridad en la gestión y
  operación de infraestructura crítica.
- La salida se presenta como nivel, no como probabilidad, así que quien la lee tiende a tratarla como un hecho, no como una estimación.
  Ese es precisamente el riesgo que la clasificación «alto» obliga a gobernar.

**Consecuencia operativa, sin rodeos:** por ser «alto», un modelo **no** entra a producción sin aprobación del AI Security Officer, y el
proyecto debe mantener evaluación de riesgos, modelo de amenazas, registro de vectores, plan de incidentes y monitoreo con alertas.
Los cuatro existen como documentos en este repositorio; su estado real está en
[`atlas-vector-mitigations.md`](atlas-vector-mitigations.md) §1 y en [`incident-response.md`](incident-response.md) §8, y **no** está completo.

Un componente que no produzca ninguna salida decisoria —por ejemplo, una métrica interna de observabilidad— es **bajo** por definición y no
requiere este aparato.

## 4. Roles

| Rol | Autoridad | Nombrado |
| --- | --- | --- |
| **AI Security Officer** | Aprueba o rechaza la entrada a producción de un modelo; puede retirarlo ante un incidente S1 | **No** — decisión D3 abierta (`odd/entrega/acta-cierre.md:31,37`). El rol existe y está vacío |
| **Responsable del sistema de IA** | Mantiene el artefacto, su procedencia y su documentación | **No** — D3 abierta |
| **Responsable del proyecto** | Adopta esta política, autoriza el rollback mayor y firma el cierre de un incidente | **Ricardo Saavedra F.** |

La distinción importa: **nombrar el rol no es nombrar a la persona.** Mientras el AI Security Officer no tenga nombre, ninguna aprobación de
modelo puede registrarse como válida, y esto se declara en lugar de simularlo.

## 5. Regla del nivel desconocido

FireWatch tiene una regla de producto que esta política eleva a regla de gobierno: **un dato ausente o vencido nunca se publica como riesgo
`bajo`.** El contrato de clasificación trata `desconocido` como valor de primera clase y lo separa explícitamente de `bajo`
(`backend/services/risk/internal/classify/classifier.go:20-25`).

Razón: para quien lee el resultado, «bajo» y «no sabemos» son hechos operativos opuestos. Publicar un `bajo` inventado por falta de dato es
la falla más grave que este sistema puede producir, y es la que la política prohíbe primero.

## 6. Registro de adopción

> **Adoptada** por **Ricardo Saavedra F.**, responsable del proyecto, el **2026-11-09**.
>
> **Qué significa esta adopción:** la escala de §2, el mapeo de §3 y las exigencias por nivel son la regla interna vigente del proyecto, y
> FireWatch se clasifica **alto**.
>
> **Qué no significa:** no es un dictamen jurídico, no acredita conformidad con el EU AI Act, no nombra al AI Security Officer y no aprueba
> ningún modelo para producción. La aprobación de un modelo concreto es un acto aparte del AI Security Officer, y hoy ese rol no tiene
> nombre.

## 7. Revisión

Se revisa cuando cambie la regulación elegida, cuando se añada un componente que produzca decisiones nuevas, o cuando un incidente revele que
la clasificación era demasiado baja. La próxima revisión queda abierta una vez nombrado el AI Security Officer, porque es quien debe adoptarla
en su parte técnica.

## 8. Lo que esta política no hace

- No sustituye asesoría legal ni una certificación.
- No aprueba ningún modelo: aprobar es un acto registrado del AI Security Officer.
- No convierte en implementado nada de lo que el registro de vectores declara parcial o ausente.
