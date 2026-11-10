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

Ninguna de las cinco bloquea la entrega de E1. Todas bloquean la versión 1.0. **D1 quedó resuelta el 2026-11-09 y se registra en el §7**; las otras cuatro siguen sin responsable asignado a la fecha de esta acta.

| # | Decisión | Bloquea | Quién decide |
| --- | --- | --- | --- |
| D1 | Motor de datos productivo, backups y SLA | Frente 14 y parte del 6 | **Resuelta el 2026-11-09: ver §7.** La decide el responsable del proyecto |
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

## 6. Alcance de la versión 1.0

Decisión del responsable del proyecto, textual:

> «B, se cierra sin modelo real, no importa que no funcione bien pero dejamos el frente abierto para que más adelante consigamos los datos y armemos un modelo con los datos que si nos sirven»

Consecuencias:

1. **La versión 1.0 se cierra sin modelo real.** Un modelo que no funciona bien no es requisito: el que existe es un ensayo y así queda declarado.
2. **El frente del modelo queda abierto**, no abandonado. Lo que lo desbloquea está medido, no supuesto: meteorología diaria en días **sin** incendio (ERA5-Land o la red DMC) unida por (comuna, fecha), y acceso al registro de eventos de CONAF (DOI `10.71578/UXAUN5`, hoy 403 anónimo).
3. **El cómputo de v1.0 se hace sobre lo demás**: diseño web terminado, usuarios y asignación de trabajos, modelo de datos de la aplicación, los doce frentes abiertos del plan de producción y las unidades de hardening pendientes.

El plan de v1.0 con sus tareas vive en [`../tasks/v1-produccion.md`](../tasks/v1-produccion.md).

## 7. D1 resuelta — motor de datos productivo, backups y SLA (2026-11-09)

**Decisión del responsable del proyecto: Ricardo Saavedra F., 2026-11-09.** Cierra D1. Es una decisión de arquitectura del proyecto; el mandante no la contrafirma, y el §7.2 registra lo que sí queda abierto de su lado: la residencia de los datos.

### Motor elegido

**PostgreSQL 17 con PostGIS 3.5, autogestionado en contenedor, sobre AWS EC2 con Docker Compose.** No es una decisión nueva: es la que ya estaba tomada en [`../tasks/plan-produccion/architecture-change-aws.md`](../tasks/plan-produccion/architecture-change-aws.md) §3-§4 el 2026-11-06 —«PostGIS en contenedor (con Compose); administrada solo si se adopta Kubernetes»— y que el registro de decisiones nunca recogió, por eso D1 seguía figurando como abierta.

### 7.1 Por qué se propuso Oracle y por qué quedó fuera

**Oracle nunca fue un requisito del mandante.** Fue una propuesta del propio responsable del proyecto, y su motivo no era técnico sino de **residencia de datos**: los servidores están en Santiago y Valparaíso, y así los datos no salen de Chile. Corrección del responsable, textual:

> «usar oracle nunca fué un requisito del mandante, fué una idea mia porque los servidores estan en stgo y en valpo, asi los datos no salen de chile, lo de aws es una solución porque no podiamos crear la cuenta en oracle y la profe dijo que igual sería una buena opción, al menos para que funcione ya que la ultima presentación no lo pudimos tener montado funcionando en servidor»

Quedó fuera por una razón práctica, no por un análisis: no se pudo crear la cuenta en Oracle, y AWS entró como salida para tener el sistema **funcionando en un servidor**, que es lo que la presentación anterior no logró. El motor que se registra aquí es el que el código usa.

Lo que sí ata al código —medido en el repositorio, no supuesto— es lo siguiente, y es lo que costaría volver a Oracle:

| Dependencia | Dónde | Qué exige de Oracle |
| --- | --- | --- |
| `ON CONFLICT` (upsert) | 6 sitios, incluidos el outbox de `jobs` y `risk` y la proyección del directorio en `prevention` | No existe el mismo constructo: se escribe con `MERGE`, uno por uno |
| PostGIS (`geography`, `ST_*`) | geometría de zonas y GeoJSON de salida | SDO_GEOMETRY: funciones, índices y semántica distintas: se reescribe la capa geográfica |
| Tipo `uuid` | `directory_users` y los contratos | No nativo antes de 23ai: `RAW(16)` o `VARCHAR2`, y eso toca esquema y mapeo |
| `pgxpool` | los tres servicios | Otro driver (`godror`), en cada constructor de repositorio y sus pruebas |
| Runner de migraciones | aplica DDL y escribe el ledger en la misma transacción | El DDL de Oracle hace **commit implícito**: una migración fallida deja esquema a medias y sin fila en el ledger |

**Costo de volver a Oracle, sin adornos:** no es configuración, es un proyecto —reescribir SQL y capa geográfica en tres servicios, rehacer el runner, levantar un Oracle real para las pruebas de repositorio y volver a correr la verificación completa— y mientras dure deja esperando a los frentes 2, 6 y 12. Este acta no lo autoriza ni lo descarta: lo dejó de ser necesario al dejar de ser un requisito.

### 7.2 Residencia de datos: lo que verifiqué hoy

La preocupación original era real y no quedó resuelta por elegir AWS. Comprobado en las fuentes de AWS el 2026-11-09:

| Evidencia | Qué dice |
| --- | --- |
| `https://aws.amazon.com/about-aws/global-infrastructure/regions_az/` | Declara la región «South America (Chile)» con id **`sa-west-1`**, en Santiago, y la marca **`"available": false`** con **0** zonas de disponibilidad |
| `https://ip-ranges.amazonaws.com/ip-ranges.json` (`createDate` 2026-11-09) | Contiene `sa-west-1` y `sa-east-1`; ninguna región con otro código aparece en Sudamérica |

**Consecuencia, sin rodeos:** desplegar hoy en AWS significa desplegar en **São Paulo (`sa-east-1`)**, y los datos salen de Chile. La región chilena está anunciada pero no disponible, así que la meta de residencia **no se cumple con el despliegue actual** y no hay forma de que este acta la cumpla escribiéndola mejor.

Las tres salidas posibles, para que la decisión sea explícita cuando toque:

1. **Aceptar São Paulo** y registrar que los datos salen de Chile. Es lo que ocurre si se despliega sin cambiar nada.
2. **Esperar `sa-west-1`**, sin fecha de disponibilidad publicada en la fuente consultada; bloquea el despliegue indefinidamente.
3. **Buscar un proveedor con región en Chile** — Oracle Cloud u otro —, lo que devuelve el problema del motor del §7.1 a la mesa con su costo.

**Decisión registrada el 2026-11-09: se despliega en São Paulo (`sa-east-1`) como solución temporal**, aceptando de forma explícita que los datos salen de Chile. La condición para revisarla está escrita, no implícita: que `sa-west-1` pase a disponible, o que se pueda abrir una cuenta en un proveedor con región en Chile.

**El traslado a Santiago no es un cambio de configuración: es una migración.** Cuando la región exista, hay que mover datos, backups y secretos, y volver a correr la verificación; por eso la política de backups del §7 tiene que poder exportar a la región de destino desde el primer día, y esa es la única decisión de este acta que mira hacia adelante en vez de registrar el presente.

Lo que **no** se hace: desplegar en São Paulo y dejar de mencionar la residencia. Queda escrito que hoy no se cumple, con fecha y con su condición de salida.

### Impacto en migraciones

**Ninguno.** Las 14 versiones del ledger compartido se aplican tal como están sobre el motor elegido. La regla de versiones y el conteo por servicio viven en [`../../backend/docs/migration-ledger.md`](../../backend/docs/migration-ledger.md); `risk` y `prevention` comparten `schema_migrations` y `jobs` migra su propia base. La restricción que sí introduce un motor autogestionado es otra y está abajo: el volumen persiste, y su backup es responsabilidad nuestra, no del proveedor.

### Impacto en persistencia

Se mantiene lo construido: PostGIS `geography` para zonas y su GeoJSON, `uuid` para identidades, `timestamptz` para instantes y `pgxpool` como pool. El inventario real —22 relaciones, 138 columnas, 19 claves primarias, 10 claves foráneas— vive en [`../../backend/docs/schema-inventory.md`](../../backend/docs/schema-inventory.md) y su modelo de dominio en [`../../backend/docs/domain-model.md`](../../backend/docs/domain-model.md). Con Compose el estado está en un volumen gestionado por nosotros: no hay backup automático de plataforma, y por eso la política de la sección siguiente no es opcional.

### Política de backups

| Parámetro | Valor |
| --- | --- |
| Método | Backup completo lógico (`pg_dump`) cada 15 minutos hacia S3, con retención de 30 días y cifrado en reposo |
| Verificación | Un restore completo sobre una instancia aislada, contabilizado y registrado con su duración |
| Frecuencia de prueba | Mensual, y después de cualquier cambio de esquema mayor |
| Qué no se promete | PITR por segundo: el RPO elegido no lo exige, y añadirlo es una mejora posterior, no un requisito de la 1.0 |

**Estado real: la política está escrita y no implementada.** No existe hoy ni el envío a S3 ni el restore probado; `../../backend/docs/verification.md:64` ya declaraba recuperación y SLA sin probar. Implementarla y probarla es trabajo de las tareas T3 y T4 del frente 14, no de esta acta, y este documento no acredita lo que no existe.

### SLA acordado

| Métrica | Meta |
| --- | --- |
| Disponibilidad mensual | **99,5%** (metas iniciales de `Referencias/EP1_Equipo3_Armijo_Calderon_Saavedra2_produccion.md:90`) |
| RPO | **15 minutos** — el ciclo de ingesta es de 15 minutos, así que una restauración cuesta como máximo un ciclo de trabajo aceptado |
| RTO | **1 hora** |
| Frescura de datos | Ciclo de 15 minutos, con alerta de trabajo atascado a los 1200 s (el doble del ciclo) |

**Ninguna de las cuatro está medida.** Son objetivos acordados, no resultados: el frente 6 mide carga y p95 con su benchmark, y el frente 14 debe ejecutar un restore real para convertir el RTO en un número observado. Ninguna cifra de este §7 se declara medición.

### Lo que este §7 no cierra

- **El restore probado.** Sin él, RPO y RTO son una promesa, no una capacidad.
- **La pregunta por el motor administrado.** Sigue condicionada a adoptar Kubernetes, como fijó `architecture-change-aws.md` §4; con Compose no hay nada que decidir.
- **El despliegue en EC2.** Dominio, certificado, security group, rol IAM y apagado de la instancia siguen en `../../backend/deploy/prod/README.md` y en los pendientes A4-A6 del §3.
- **La residencia permanente en Chile.** Hoy se acepta São Paulo como solución temporal (decisión del 2026-11-09, §7.2); que los datos vuelvan a Chile depende de que `sa-west-1` esté disponible o de abrir la cuenta en un proveedor con región chilena, y el traslado será una migración con verificación, no un cambio de configuración.
