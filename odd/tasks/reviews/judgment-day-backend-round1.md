# Judgment Day backend — ronda 1 de 2

Lote de corrección autorizado por el usuario sobre el árbol congelado del backend: 115 archivos, 16.073 líneas, `FROZEN_MANIFEST_SHA256=393c68e0c8314cc1126d1d591f51e02a00695ac795a856e23d500b637af8125d`. Ledger de filas congeladas: `tmp/judgment-day-20261013-083729/frozen-ledger-backend.md`, sha256 `1cc2a8bddf6dbbe9f30001dd56522537219a200dbbb17d8ad909bd509c3ffd27`. Máximo dos rondas. Sin commits: el repositorio no tiene historia y el usuario mantiene la decisión vigente de no crear commits.

## Specs

S1. El POST de creación de trabajos desde el origen de la página debe dejar de ser bloqueado por el navegador. El preflight del gateway debe declarar `Idempotency-Key` como cabecera permitida, porque la página lo envía (`frontend/proposal.js:1182`: `"Idempotency-Key": crypto.randomUUID()`) y el servicio lo exige (`backend/services/jobs/internal/handler/api.go:40` responde `400 invalid_idempotency_key` sin él). El allow-list no debe convertirse en comodín y el `Origin` debe seguir comparándose exacto y fail-closed. Filas congeladas: JD-A-001, JD-B-001. Frase textual del usuario: "Sí: las 2 CRITICAL y las 3 WARNING".

S2. La disposición a recibir tráfico de jobs y risk se decide por la base de datos, no por el broker. Decisión textual del usuario: "Alinear con la doc: readiness = base de datos". `/health/ready` responde 200 con el cuerpo de estado cuando el ping acotado a la base de datos funciona, y 503 con el mismo cuerpo y una razón no vacía cuando no funciona. El estado del broker y de la DLQ no cambia el código HTTP de esa ruta. Filas congeladas: JD-A-002, JD-B-002.

S3. No se elimina ni se debilita ninguna señal honesta de dependencias. Decisión textual del usuario: "Broker y DLQ pasan a reportarse como degradación en el cuerpo de /health/ready y en /metrics, sin sacar la réplica." El cuerpo de `/health/ready` sigue exponiendo `dependencies.broker` y `dead_letter` (profundidad y edad), la ruta de estado autenticada sigue devolviendo 200/503 con `reason`, y `/metrics` sigue exponiendo el ready del broker y la profundidad/edad de la DLQ. Un broker caído o una DLQ con backlog deben seguir siendo visibles por alguna superficie.

S4. La ruta de publicación no puede reetiquetar datos de forma silenciosa. Decisión textual del usuario: "Fail-closed solo para proveedores no sintéticos". Cuando la procedencia de la recuperación declara `synthetic: false`, la publicación falla con razón registrada si las etiquetas de zona de la propia recuperación no identifican la zona solicitada, y no persiste ninguna evaluación ni geometría atribuida a otra comuna. Con `synthetic: true` se conserva la divergencia documentada hoy en `backend/services/risk/internal/service/publisher.go:158-164`. Filas congeladas: JD-A-004, JD-B-004.

S5. Una recuperación declarada incompleta no puede producir una evaluación determinada. Si `Provenance.Complete` es falso, la ruta de publicación no persiste evaluación y falla de forma recuperable con razón registrada, en el mismo espíritu que el guard existente de `Merge` ("a partial retrieval must not be able to satisfy a key by accident"). Filas congeladas: JD-A-003, JD-B-003.

S6. La identidad de una lectura en `Merge` no puede perder precisión. Decisión derivada de la fila congelada JD-A-005 / JD-B-005: dos lecturas distintas de la misma variable y zona dentro del mismo segundo no deben colisionar ni perderse, y el resultado debe seguir siendo independiente del orden de entrada.

S7. Los textos que describen lo cambiado se actualizan con el cambio. `backend/docs/architecture.md` debe dejar de afirmar que `/health/ready` sólo consulta la base de datos como contrato incumplido: debe describir la semántica nueva y seguir declarando dónde se ven las dependencias degradadas.

## Tasks

- T1 → S1 — route: worker — superficies: `backend/services/gateway/internal/handler/gateway.go`, `backend/services/gateway/internal/handler/gateway_test.go` — commit: ninguno (decisión vigente).
- T2 → S2, S3, S7 (jobs) — route: worker — superficies: `backend/services/jobs/cmd/server/main.go`, `backend/services/jobs/cmd/server/main_test.go`, `backend/docs/architecture.md` — commit: ninguno.
- T3 → S2, S3, S4, S5, S6 (módulo risk, escritor único por módulo) — route: worker — superficies: `backend/services/risk/cmd/server/main.go`, `backend/services/risk/cmd/server/main_test.go`, `backend/services/risk/internal/service/publisher.go`, `backend/services/risk/internal/service/publisher_test.go`, `backend/services/risk/internal/source/provider.go`, `backend/services/risk/internal/source/provider_test.go` — commit: ninguno.
- T4 → S1-S7 — route: gentle-ai-verify independiente, después de T1-T3 — commit: ninguno.
- T5 → S1-S7 — route: re-juicio acotado read-only de las cinco filas congeladas (ronda 1 de 2) — commit: ninguno.

Olas: T1, T2 y T3 en paralelo (módulos Go distintos, superficies disjuntas). T4 después. T5 después de T4.

## Log

L1 (petición original del usuario, textual): "que onda bro, ayer estuvimos haciendo unos arreglos de lo que es el frontend, hoy me gustaría hacer un judgment day de lo que es la app completa, back y front, porfa bro"

L2 (decisiones de alcance, textual): "Dos rondas: backend y luego frontend" y "Todo el código ejecutable".

L3 (evidencia de la ronda de descubrimiento): dos jueces ciegos concurrentes, `jd-judge-a` (mv6lxghg-1-znw8) y `jd-judge-b` (mv6lxghm-2-y5i2), read-only sobre el árbol congelado. Cada uno devolvió cinco filas con clase de evidencia `deterministic`. Coincidencia 5/5 en el mismo conjunto de defectos; sin hallazgos únicos de un lado. Convergencia no es prueba: el padre ancló las dos filas severas en código antes de congelarlas (gateway.go:66 + api.go:40 + proposal.js:1182; workloads.yaml:39/83/127 + `ReadyReason` + `healthHandler` 503 + architecture.md:56).

L4 (aprobación del lote, textual): "Sí: las 2 CRITICAL y las 3 WARNING". Las tres WARNING se autorizan como lote separado: el contrato de Judgment Day no les programa corrección por sí mismo.

L5 (decisión de semántica, textual): "Alinear con la doc: readiness = base de datos".

L6 (decisión de zona, textual): "Fail-closed solo para proveedores no sintéticos". Motivo: el código documenta la divergencia de etiqueta como deliberada y la reconciliación de nombres de zona estaba diferida como tarea aparte; el daño real aparece cuando entra un proveedor no sintético.

L7 (revisión de semántica previa): la disposición fail-closed ante broker y DLQ fue deliberada en L0/L3b ("salud honesta" tras siete apariciones de señales que afirmaban más de lo que podían sostener). S3 existe para no deshacer esa propiedad: cambia quién decide el enrutamiento, no qué se reporta. L3b registró "health204/503 equivalente"; S2 la revisa de forma deliberada a 200 con cuerpo.

L8 (límite de verificación declarado): Docker no está disponible en esta sesión (`docker version` no alcanza el daemon), así que las suites de integración con PostGIS y RabbitMQ quedan saltadas. La verificación independiente del lote es de nivel unitario y de conducta con dobles, y ese límite debe declararse en el veredicto en lugar de presentarse como cobertura total.

L9 (estado de repositorio): el árbol no tiene ningún commit (`master` sin historia). El binding del árbol congelado es un hash de contenido, no un diff, y la trazabilidad por commit está ausente por decisión del usuario.

L10 (entrega de los escritores): T1 `mv6mr5b9-3-pf8b` (gateway), T2 `mv6mr5be-4-mb1q` (jobs, `architecture.md`, OpenAPI) y T3 `mv6mr5bj-5-bcip` (módulo risk completo). Los tres reportaron RED observado con el mensaje del defecto y GREEN con el mismo comando, `gofmt` y `vet` limpios, y los cambios deliberados de comportamiento previamente fijado.

L11 (reconciliación del padre, no del reporte): recomputado el manifest congelado con el mismo comando, el árbol pasó de 115 a 115 archivos: exactamente doce archivos modificados, ninguno añadido y ninguno eliminado, y coinciden uno a uno con las superficies autorizadas de T1, T2 y T3 (`gateway.go`, `gateway_test.go`, `jobs/cmd/server/main.go`, `jobs/cmd/server/main_test.go`, `architecture.md`, `firewatch-v1.yaml`, `risk/cmd/server/main.go`, `risk/cmd/server/main_test.go`, `publisher.go`, `publisher_test.go`, `provider.go`, `provider_test.go`). Nada bajo `frontend/` fue tocado. Total de líneas 16073 → 16598.

L12 (riesgo de T2 refutado): T2 declaró que `backend/scripts/smoke.py:93` se colgaría al esperar 204 de `/health/ready`. El padre lo refutó: `API = "http://localhost:8080"` y en `docker-compose.yml:132` ese puerto lo publica el gateway, cuyo `/health/ready` es un 204 estático que el lote no modificó. Queda como observación fuera del lote que ese sondeo no valida realmente a jobs y risk.

L13 (verificación independiente, T4 `mv6n7fvd-6-lzcx`, read-only): S1 PASS, S2 FAIL, S3 FAIL, S4 PASS, S5 UNPROVEN, S6 PASS, S7 FAIL. El verificador recomputó el manifest por su cuenta: 115/115 archivos, doce modificados, ninguno añadido ni eliminado, líneas 16073 → 16598. Suites de integración saltadas por ausencia de Docker y de `TEST_DATABASE_URL`, con las líneas de SKIP registradas. Conclusión explícita del verificador: las pruebas verdes no tapan que risk pinte un contrato equivocado en el cuerpo.

L14 (correcciones requeridas por la verificación, completan el mismo lote): (a) el campo `ready` del cuerpo del sondeo debe llevar la decisión completa de dependencias en los dos servicios y el código HTTP sigue siendo la decisión de enrutamiento, con jobs como referencia y el evaluador único compartido en risk para que las dos rutas no puedan divergir otra vez; (b) `architecture.md` debe definir explícitamente qué significa ese campo; (c) la prueba del flag demo debe afirmar que hubo exactamente una escritura, porque hoy una ausencia silenciosa de escritura la pasaría.

L15 (unidad separada autorizada por el usuario): `backend/services/risk/internal/messaging/broker.go:185` descarta `workErr` del registro de mensajes muertos, así que las razones de rechazo de las guardas nuevas llegarían a la DLQ sin causa legible. El usuario eligió corregirlo como lote separado verificado aparte; el alcance congelado de la ronda 1 sigue siendo de doce archivos y esta unidad no se mezcla con esa evidencia.

L16 (riesgo latente registrado, no corregido): la verificación observó que `Store.Publish` acepta una evaluación sin procedencia, de modo que un futuro llamador directo en producción podría esquivar las guardas de la ruta de publicación. Hoy no existe tal llamador; se registra como riesgo latente para una ronda posterior.

L17 (límite de trazabilidad, aceptado y declarado): los bytes previos a la corrección no existen en ninguna parte, porque el repositorio no tiene commits y nadie tomó una instantánea de contenido. Se puede reconstruir qué archivos cambiaron recomputando el manifest, no el antes. Por eso ninguna afirmación histórica sobre esquemas, rutas o códigos eliminados es demostrable, y la verificación lo marca como UNPROVEN en lugar de darlo por bueno.

L18 (correcciones verificadas aparte, T7 `mv6nyvgw-8-blag`): unidad A PASS —los cuatro símbolos divergentes desaparecieron y las tres aserciones movidas vuelven a fallar bajo el contrato rechazado—, C PASS (falsable en los dos sentidos: falla con cero escrituras y con dos), D PASS como corrección, y B PASS en el contenido actual con la preservación histórica en UNPROVEN. El OpenAPI quedó confirmado consistente sin haber sido tocado. El padre cerró con la instantánea intermedia el hueco que el verificador no pudo cerrar: solo seis archivos cambiaron en la ronda de correcciones, y son los seis autorizados.

L19 (re-juicio acotado, ronda 1 de 2): dos re-jueces ciegos e independientes (`jd-judge-a` mv6o4lwx-9-t0li y `jd-judge-b` mv6o4lx2-a-8qvz), read-only, sobre el árbol corregido, con las tres instantáneas a la vista para identificar por su cuenta qué cambió en cada ronda. Los dos resolvieron las cinco filas congeladas como `verified`, con coincidencia 5/5 y sin resoluciones divergentes. JD-A-002 quedó resuelto por construcción —el código HTTP del sondeo dejó de depender del broker— y no con un clúster real, que sigue sin poder levantarse en esta sesión.

L20 (veredicto terminal): `JUDGMENT: APPROVED`. Ninguna fila severa sobrevive. Rondas de corrección usadas: 1 de 2. Evidencia acumulada: verificación independiente del lote (S1, S4 y S6 PASS; S2, S3 y S7 falladas; S5 no probada), correcciones A-D, verificación independiente de las correcciones (A, C y D PASS; B PASS en lo actual), y dos re-jueces convergentes sobre las cinco filas. Binding final: 115 archivos, catorce modificados, cero añadidos y cero eliminados; seis archivos cambiados en la ronda de correcciones, los seis autorizados.

L21 (límites que el veredicto NO cubre, declarados en lugar de omitidos): (1) las suites de integración quedaron saltadas por ausencia de Docker, con once líneas de SKIP registradas: nada se probó contra PostGIS, RabbitMQ o Keycloak reales; (2) la preservación histórica es indemostrable sin bytes previos, así que «no se eliminó ningún esquema ni ruta» queda como afirmación no probada; (3) quedan dos observaciones fuera del lote sin corregir: `Store.Publish` acepta una evaluación sin procedencia, de modo que un futuro llamador directo podría esquivar las guardas, y el 204 estático del gateway hace que el sondeo de `backend/scripts/smoke.py` no valide realmente a jobs ni a risk.
