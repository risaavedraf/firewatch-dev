# Handoff — ruta completa de la demo (backend verificado + selector en la página)

**Destino:** sesión `01a13a74-2649-7758-abe9-2890eaaa05d9` (nueva orquestadora).
**Origen:** sesión `01a13724-2745-7467-986f-2d752a72ca92` (backend sintético, trabajo ya cerrado).
**Fecha:** cierre de la unidad backend sintética; este documento es un **handoff de orquestación**, no una autorización.

## 0. Qué se te pide

Continuar la orquestación de la **ruta completa**: llevar lo ya verificado en un entorno desechable al entorno local del usuario y hacer que el escenario se seleccione **desde la página**.

Estado de autorizaciones: el usuario aprobó la ruta mínima (actualizar la pila) y está evaluando autorizar el cambio de frontend para el selector. **No asumas ninguna de las dos autorizaciones**: confírmalas antes de tocar nada.

## 1. Listo y verificado (no repetir)

| Área | Estado | Dónde |
|---|---|---|
| Seis perfiles sintéticos deterministas | Implementado, pruebas PASS | `backend/services/risk/internal/source/synthetic_scenarios.go` |
| Selección opcional `scenario_id` en jobs | Implementado; `null`/`""` rechazados con 400 `invalid_demo_request`; omitido = fixture legacy | `backend/services/jobs/internal/handler/api.go` |
| `evaluated_at` asignado por el servidor, una vez, en UTC | Implementado; reintento idempotente conserva escenario e instante originales y no añade fila de outbox | jobs + risk |
| Contrato de eventos | Campos opcionales `ScenarioID`/`EvaluatedAt` y enum estable | `backend/contracts/gen/events/v1/events.go` |
| Migraciones | `003_synthetic_scenario.sql` (jobs), `007_synthetic_evaluation.sql` (risk), registradas en arranque y en `migrations.All()` | `backend/services/{jobs,risk}/migrations/` |
| Catálogo autenticado | `GET /v1/risk/synthetic-scenarios` | risk service |
| Contrato HTTP | Actualizado | `backend/contracts/openapi/firewatch-v1.yaml` |
| Documentación | Tres documentos al día con límites explícitos | `backend/README.md`, `backend/docs/synthetic-scenarios.md`, `backend/docs/synthetic-verification.md` |
| Arnés de verificación aislado | Implementado; 17 pruebas PASS; E2E real PASS | `backend/scripts/verify_synthetic.py`, `backend/scripts/test_verify_synthetic.py` |
| Evidencia por ID | Actualizada sin cerrar criterios no cumplidos | `odd/urgent/02-pruebas-datos.md`, `odd/urgent/03-pruebas-modelo.md` |

**Evidencia ejecutada (observada, no reportada):**

- Suites completas de `jobs` y `risk` con roles de aplicación (`jobs_app`/`risk_app`), catálogos cruzados y `go vet`: exit 0.
- Revisión independiente del recorte UTC: PASS, sin omisiones.
- E2E autenticada real en pila desechable (Keycloak → gateway → jobs → RabbitMQ → risk → PostGIS): exit 0 en ~65 s, con 401/403/400, catálogo de seis perfiles, cinco perfiles publicables con factores y huella de política, comportamiento legacy e idempotencia.
- Arnés: 17 pruebas PASS (`python -m unittest discover -s backend/scripts -p test_verify_synthetic.py -v`, ~27 s, exit 0) más revisión independiente de fuente.

**Especificación y bitácora completas:** `odd/tasks/backend/urgent-synthetic-mvp.md` (58 entradas de Log, `L1`–`L58`). Es la fuente de verdad del alcance probado.

## 2. NO listo (aquí empieza tu trabajo)

1. **La pila local NO está actualizada.** Las migraciones `003` y `007` solo se aplicaron en bases desechables. Las bases reales del usuario no las tienen, y los servicios locales no están reconstruidos. **Nada de esto está probado en su entorno.**
2. **No existe selector de escenarios en la página.** El frontend actual muestra último resultado e historial, pero la selección solo funciona por API.
3. **`partial-retrieval` no tiene prueba de runtime.** El arnés lo reporta `SKIP` por defecto; la negativa a publicar está probada aparte, por pruebas unitarias.
4. **React/PWA no está implementado.** Solo hay documentación y plan.
5. **Sin aprobación científica ni de producción.** No hay calibración, métricas de modelo entrenado, equidad, ni firma Go/No-Go. Los 132 IDs del checklist no están cerrados.
6. **Sin base Git.** `master` está sin nacer y los archivos sin rastrear: no hay commits, baseline ni `git diff` disponible para comparar. El `assess` nativo devuelve `unassessable` por esta razón; usa verificación independiente, nunca una aprobación nativa fabricada.
7. **Duplicado pendiente de decisión humana:** `odd/tasks/backend-hardening/` (12 archivos) sigue en la raíz; su copia verificada está en `odd/tasks/security/backend-hardening/`. No lo borres tú.

**Límites conocidos del arnés que siguen vigentes** (documentados en `backend/docs/synthetic-verification.md`, sección *Known limits*): la comparación de instantes no prueba serialización UTC en el cable; `latest` no afirma `evaluated_at`; el 401 no se sondea por ruta; el reintento ante 429 no prueba el limitador; la unicidad de outbox se infiere; `--timeout` no es techo estricto; la comprobación opcional puede dar falso positivo.

## 3. Próximos pasos recomendados

1. **Obtener autorización explícita** del usuario para tocar la pila local y, si procede, el frontend.
2. **Respaldar los volúmenes antes de migrar.** Las migraciones son aditivas y hacia adelante, pero se aplican sobre datos reales. Riesgo: items 1 y 5.
3. **Actualizar la pila completa y en conjunto.** El cambio de contrato es opcional pero inseguro con consumidores viejos conviviendo: servicios locales se actualizan juntos.
4. **Verificar en local, no asumir.** Catálogo autenticado, un trabajo por escenario, y que página/historial reflejen el resultado.
5. **Autorizar el cambio de frontend** para el selector. Ojo: la instrucción `S4` reservó el código frontend a la otra sesión y esa sesión solo generó documentación. **No modifiques frontend sin autorización explícita del usuario.**
6. **Prueba en navegador:** login, selección de zona y escenario, resultado, historial, logout.
7. **Mantener el mensaje honesto de la demo:** datos sintéticos, regla académica, sin calibración.

## 4. Restricciones que no debes romper

- No modificar `.env*`, despliegue, compose, workflows, credenciales ni volúmenes existentes.
- No tocar la migración inmutable `001`.
- Recursos Docker desechables solo para verificación; limpiar únicamente los propios; **preservar la evidencia de fallos**, nunca reescribir un fallo como aprobado.
- No reescribir bitácoras históricas ni citas en prosa de documentos históricos; el índice `odd/tasks/README.md` funciona como resolvedor de navegación.
- No reintroducir rutas antiguas: los documentos viven en carpetas por área (`backend/`, `frontend/`, `security/`, `reviews/`, `presentation/`, `architecture/`).
- Sesión par: `01a13736` (documentación React y planificación). No le escribas en sus superficies ni muevas sus archivos.

## 5. Comandos de referencia (ya ejecutados por la sesión saliente)

```sh
# Arnés aislado real: crea su propia pila desechable, exige las imágenes locales
python backend/scripts/verify_synthetic.py

# Pruebas del arnés, sin Docker real (~27 s)
python -m unittest discover -s backend/scripts -p test_verify_synthetic.py -v
```

Migraciones locales: se aplican por el entrypoint `migrate` de cada servicio con su `DATABASE_URL` (así se hizo en la verificación aislada con `GOWORK=off DATABASE_URL=... go run ./cmd/server migrate`). **Confirma el mecanismo real en el compose antes de usarlo**; no lo asumas.

El gateway ya reenvía `/v1/risk/`: no requiere cambios de ruteo.

## 6. Memoria persistente disponible

- Espejo completo de la especificación: `mem_get_observation(1616)`, clave `odd/urgent-synthetic-mvp/tasks`.
- Evidencia E2E autenticada y convenciones de reorganización: observaciones `1641` y `1646`.

## 7. Criterio de cierre de esta ruta

La ruta completa se cierra cuando: la pila local tiene las migraciones aplicadas y los servicios reconstruidos; el escenario se selecciona desde la página; el recorrido de navegador está observado; y los fallos, omisiones y límites quedan escritos en la especificación con su evidencia. Nada de eso autoriza publicación, despliegue a producción ni afirmaciones científicas.
