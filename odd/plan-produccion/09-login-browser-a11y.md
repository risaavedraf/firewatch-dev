# Frente 9 — Login real en navegador, móvil y accesibilidad

- **Grupo de entrega:** G5 — Producto y cierre.
- **Estado:** Parcial — el recorrido browser/Keycloak y la revisión móvil/accesibilidad se midieron en L14/L23 (Chrome, 390 px, teclado y contraste calculado). **Falta:** certificación WCAG y prueba con lector de pantalla; los límites están en README §7.
- **Riesgo (Task Size):** medio — ítem 6 si no hay prueba automatizada que capture regresiones de login; sin datos/seguridad nuevos más allá del flujo OIDC existente.
- **Depende de:** realm Keycloak inicializado, cliente `firewatch-demo` configurado y entorno navegador/API disponible.
- **Seam / superficie:** `firewatch-demo` (OIDC Authorization Code + PKCE S256), callback y URI post-logout; la API conserva autorización *server-side*.

**Objetivo.** Completar y verificar el acceso OIDC real y la experiencia de login/logout en navegador, móvil y accesibilidad.

**Documentación real (redirección).**
- [`../tasks/frontend/client-proposal-page.md:13-26,39-45`](../tasks/frontend/client-proposal-page.md) — checks y próximo paso.
- [`../tasks/README.md:77-88`](../tasks/README.md) — estado frontend.

## Specs

- **S1** — El realm persistente aplica la URI de post-logout documentada y los valores requeridos para login.
- **S2** — Login/logout real con Keycloak y una llamada API autenticada completan el recorrido.
- **S3** — La interfaz se verifica visualmente en navegador y en móvil.
- **S4** — Se evalúa accesibilidad formal (las pruebas existentes no la demuestran).

## Tasks

| ID | Spec | Ruta | Descripción | Commit |
| --- | --- | --- | --- | --- |
| T1 | S1 | inline | Aplicar en Admin Console los ajustes no destructivos del cliente (post-logout, valores de login). | — |
| T2 | S2 | inline | Ejecutar login/logout real + llamada API autenticada. | — |
| T3 | S3 | inline | Verificación visual navegador y móvil. | — |
| T4 | S4 | inline | Evaluación de accesibilidad formal. | — |

Ruta por defecto: **inline**. El recorrido de navegador puede ejecutarse con el actor de revisión de página.

## Dependencias y bloqueos humanos

- **Bloqueo humano:** aplicar los ajustes no destructivos en Admin Console; resolver la selección nativa de archivos antes de declarar revisión/cierre.

## Criterios de cierre

- `../tasks/frontend/client-proposal-page.md:24-26,42-45`: checks y próximo paso; falta prueba live.

## Log

- **L1 (2026-10-16)** — Creación del desglose por spec a partir de [`README.md`](README.md) §1 (frente 9). Documento de planificación; sin implementación.
- **L2 (2026-10-20)** - Estado corregido a Parcial: recorrido browser/Keycloak y revisión móvil/accesibilidad medidos en L14/L23; faltan certificación WCAG y lector de pantalla (README §7).
