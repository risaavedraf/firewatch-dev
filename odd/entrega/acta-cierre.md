# Acta de cierre de la entrega

Registra las decisiones tomadas en la sesión de definición del 2026-11-09 sobre qué se entrega, en qué documento y con qué disparador de cierre. No acredita implementación: el estado de cada frente vive en [`../plan-produccion/README.md`](../plan-produccion/README.md) y en [`../tasks/README.md`](../tasks/README.md).

## 1. Decisiones tomadas

### E1 — Qué se entrega: entrega académica defendible

El sistema completo, más la demostración de que el modelo no es viable con los datos entregados. Se firma con el documento, no con un modelo en producción.

La no-viabilidad deja de ser una disculpa y pasa a ser el hallazgo principal: dos modelos medidos contra dos baselines de tipo climatología, con la regla de victoria declarada antes de mirar el resultado, y ninguno gana. El detalle técnico vive en `backend/training/REPORT-delivered-data.md` y `backend/training/ANALYSIS-delivered-data.md`.

Queda fuera de esta entrega entrenar un modelo real. Eso exige una fuente meteorológica independiente unida por (comuna, fecha) —ERA5-Land o la red DMC—, que no existe en el repositorio. Se puede retomar después del cierre; no condiciona esta entrega.

### E2 — Qué documento lee el mandante: informe propio en español

`odd/entrega/informe-datos-y-modelo.md`, escrito a mano, con los dos reportes generados como anexos técnicos. Razón: los reportes generados están en inglés, declaran que editarlos a mano es un defecto, y no son el documento que alguien lee para decidir si aprueba.

Los anexos no se editan nunca a mano. Cualquier número del informe que no esté en ellos es un defecto del informe.

### E3 — Superficie de entrega: repositorio remoto

`main` al día en `origin`, rama publicada y PR #54 como evidencia de revisión. Antes de esta sesión el entregable completo existía únicamente en un disco local: `main` estaba dos commits adelante del remoto y la rama de trabajo no se había publicado nunca.

### E4 — Disparador de cierre: tag de versión

**Superseded por la corrección del §5.** El disparador original era el tag `v1.0-produccion` sobre el commit de merge de PR #54. Esta acta lo conserva como historia, no como decisión vigente: la corrección del 2026-11-09 establece que esta entrega **no** es la versión 1.0 y que el tag no puede emitirse hasta cerrar lo que el §5 enumera.

## 2. Decisiones pendientes

Ninguna de las cinco bloquea la entrega de E1. Todas bloquean la versión 1.0, y ninguna tiene responsable asignado a la fecha de esta acta.

| # | Decisión | Bloquea | Quién decide |
| --- | --- | --- | --- |
| D1 | Motor de datos productivo, backups y SLA | Frente 14 y parte del 6 | Arquitectura + cliente |
| D2 | Registry, llaves de firma y acceso al clúster | Frente 7 y la ejecución del 6 | Infraestructura |
| D3 | Responsables de gobierno y aprobación de controles U1 | Frente 8 — 41 de los 132 IDs | Cliente / jefe de proyecto |
| D4 | Qué subconjunto de los 132 IDs es requisito real | Todo el marcado de IDs | Cliente |
| D5 | Alcance de accesibilidad: ¿WCAG AA? ¿lector de pantalla? | Cierre del frente 9 | Producto |

## 3. Tareas abiertas al momento de esta acta

| # | Tarea | Responsable |
| --- | --- | --- |
| A1 | Informe de E2 revisado y commiteado | Sesión de trabajo actual |
| A2 | CI de PR #54 en verde: dos jobs fallan y se están corrigiendo | Sesión de trabajo actual |
| A3 | Los 17 tests de Go que necesitan Postgres, y el Caddyfile, nunca ejecutados en el host de desarrollo | Requiere entorno con Docker disponible |
| A4 | T9 bloqueado: el producto CONAF tiene DOI restringido (403 anónimo) | Tercero |
| A5 | T10 bloqueado: EC2, dominio, security group e IAM | Infraestructura |
| A6 | T11 pendiente: `deploy.yml` | Sesión de trabajo actual |

## 4. Estado verificado en esta sesión

Ejecutado en el host de desarrollo, con su resultado:

| Verificación | Resultado |
| --- | --- |
| `pytest -q` (paneles de entrenamiento) | 306 pruebas pasan |
| `ruff check .` | limpio, tras corregir dos `E731` que habrían roto el job de lint |
| `govulncheck` v1.9.0 en `risk` e `inference` | sin vulnerabilidades |
| Configuración de Prometheus y alertas | parsea; la alerta de target caído cubre los cinco jobs de aplicación |

No ejecutado, con causa declarada:

| Verificación | Causa |
| --- | --- |
| `go test -race` | requiere cgo y no hay compilador C en el host; lo ejecuta CI sobre Linux |
| 17 tests de Go con Postgres | no hay daemon Docker ni credencial para el PostgreSQL del puerto 5432 |
| Caddyfile | no hay binario `caddy` ni daemon |

Ninguna de las tres se declara cumplida por aparecer en esta lista.

## 5. Corrección del 2026-11-09: esta entrega no es la versión 1.0

Corrección del responsable del proyecto, textual:

> «con esto estoy dando por hecho de que la app NO está terminada, aún nos falta terminar el diseño de la pagina web, tener el modelo funcionando, agregar los usuarios para designarles los trabajos, el modelo de datos que necesitamos para que funcione la app, aún queda e, con esto listo no terminamos el trabajo, hay un monton de tasks no cerradas en odd que tenemos que cerrar antes de llamar a esta version el tav 1.0 produccion»

Lo que esta corrección establece, y que las secciones anteriores de esta acta no decían:

1. **La aplicación no está terminada.** Lo entregado es un corte verificable de un sistema en construcción, no su versión final.
2. **El tag `v1.0-produccion` queda bloqueado** hasta cerrar las tareas abiertas de `odd/` y las capacidades faltantes.
3. **Cuatro capacidades nombradas faltan**, y ninguna estaba mapeada como tal en los quince frentes de `odd/plan-produccion/README.md`:

| # | Capacidad faltante | Estado en la documentación de ODD |
| --- | --- | --- |
| F1 | Diseño terminado de la página web | Parcial: hay vista general y procedencia; el frente 9 cubre login y accesibilidad, no el diseño |
| F2 | Modelo funcionando | No existe: el servicio de inferencia está, el modelo válido no. Medido en `REPORT-delivered-data.md` |
| F3 | Usuarios y asignación de trabajos | No mapeado como frente propio; hay autenticación por Keycloak |
| F4 | Modelo de datos de la aplicación | Parcial: motor de datos pendiente (D1), sin esquema de dominio documentado |

La consecuencia operativa es una sola: **la entrega de E1 no se llama `v1.0-produccion`**. El inventario de lo que falta para llegar a esa versión, con los IDs de ODD que hay que cerrar, es trabajo de la fase siguiente y no se resuelve en esta acta.
