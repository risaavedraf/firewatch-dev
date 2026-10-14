# Guion de recorrido en navegador (N11)

Lo ejecuta **el usuario**, no la sesión: es el único paso que la automatización no puede dar.
Ninguna prueba de este repositorio abre un navegador real, así que el inicio y el cierre de
sesión contra Keycloak siguen sin estar verificados de punta a punta.

**Antes de empezar:** backend arriba (`gateway` en `:8080`, Keycloak en `:8081`), build hecho.

```sh
cd frontend
npm install
npm run build
npm run serve          # sirve dist/ en http://localhost:4173 con la CSP real
```

Abrir **`http://localhost:4173`** y **nunca** `http://127.0.0.1:4173`: el reino y el gateway
registran solo la forma `localhost`, y para el navegador son dos orígenes distintos.

## Recorrido

| # | Paso | Qué debe verse |
|---|---|---|
| 1 | Abrir sin sesión | Barra lateral con **7 entradas** en 4 grupos (Prevención, Información, Acceso, Coordinación) y **una sola vista** visible a la vez. El aviso de alcance aparece discreto en la barra superior, no como franja titular |
| 2 | Recorrer las 7 entradas | Cada clic cambia **solo** esa vista; la dirección queda en `#overview`, `#history`, `#sources`, `#jobs`, `#pending`, `#session`, `#audit`. Recargar la página en cualquier hash vuelve a la misma vista |
| 3 | Escribir `#no-existe` en la barra de direcciones | La página muestra Territorio y la dirección se corrige a `#overview` |
| 4 | Sin sesión, en Territorio | Métricas en `—`/`0`, mapa oculto con el mensaje de conexión, «Último lote publicado» en `SIN CONSULTAR` |
| 5 | En «Fuentes y servicios» | La declaración de alcance completa (entrada sintética, sin calibración, no sustituye a CONAF ni a SENAPRED), la atribución SUBDERE/IGM/INE y la restricción DIFROL, visibles dentro de su vista |
| 6 | En «Aún no implementado» | El inventario con motivo y ubicación prevista de cada capacidad; ningún control ofrece esas acciones |
| 7 | En «Sesión y permisos» | Solo el botón de inicio de sesión. **Ningún selector de rol** |
| 8 | Iniciar sesión como `demo-admin` | Vuelve a `http://localhost:4173/`, «Sesión conectada» en la tarjeta inferior, y el rol declarado por el token con la nota de que **no se verifica su firma** |
| 9 | Elegir una comuna con lote (por ejemplo `05109`) | Color de nivel, código CUT, comuna, provincia, factores con barra cuando hay `contribution`, justificación, política con hash, versión, estado del dato y vigencia |
| 10 | Elegir una comuna sin lote (por ejemplo `05101`) | «Sin lote publicado» explícito: **nunca** nivel bajo |
| 11 | Volver a la comuna anterior | La comuna que ya tenía lote **conserva su color** aunque no esté seleccionada |
| 12 | En «Historial de lotes» (zona `05501`) | Filas reales; buscar y filtrar por estado del dato reduce **solo** las filas ya devueltas; «Restablecer filtros» limpia ambos |
| 13 | En «Procesamiento» | El selector de escenario sintético sigue disponible; solicitar el lote crea el trabajo y el estado se actualiza con sondeo acotado |
| 14 | En «Registro de actividad» como admin | Cinco columnas reales (`actor_id`, `action`, `job_id`, `trace_id`, `occurred_at`) |
| 15 | Cerrar sesión | Todo se limpia: rol, auditoría, historial, colores del mapa, vigencia |
| 16 | Entrar como `demo-viewer` y abrir `#audit` | La vista **es alcanzable** y muestra el 403 explícito con su código y `request_id`. Ninguna vista queda oculta por rol |

## Qué capturar

- Una captura por vista (7), para comparar contra el diseño aprobado.
- La vista `#audit` con `demo-viewer` mostrando el 403: es la prueba de que no hay gating por rol
  en el cliente.
- Cualquier discrepancia visual con el mockup preservado en
  `odd/urgent/rediseno-pagina/referencia-mockup/frontend/`.

## Límites declarados

- La corrección visual y la accesibilidad formal **no** están verificadas por pruebas: solo por
  este recorrido.
- Desviaciones de reloj mayores que el margen del cliente (1000 ms) no están cubiertas.
- Si el gateway o Keycloak no están arriba, el recorrido se detiene en el paso 8 y lo que se
  observe hasta ahí sigue siendo evidencia válida de las vistas sin sesión.
