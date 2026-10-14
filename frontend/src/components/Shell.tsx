import type { ReactNode } from 'react';

import { navigationGroups, routes, type RouteDefinition } from '../app/routes';
import { Icon } from './ui';

// The application shell: sidebar, topbar, update bar and footer, ported from the
// page that was already reviewed. Every element identifier the retired page
// exposed survives here, because the behaviour that reads them (session state,
// gateway readiness, zone chip, reset filters) is ported with it.
//
// One thing deliberately does *not* survive: the demonstration strip is no
// longer a full-width headline above everything. The honesty statement it
// carried is still made — as a discreet, always-visible technical notice that
// leads to the data-status view, where the whole declaration lives (S15).
export function Shell({ route, children }: { route: RouteDefinition; children: ReactNode }) {
  return (
    <>
      <a className="skip-link" href="#content">
        Saltar al contenido
      </a>
      <div className="app-shell">
        <aside className="sidebar">
          <a className="brand" href="#overview" aria-label="FireWatch, inicio">
            <span className="brand-mark" aria-hidden="true">
              F
            </span>
            <span>
              <strong>FireWatch</strong>
              <small>Prevención territorial</small>
            </span>
          </a>
          <div className="workspace">
            <span className="workspace-mark" aria-hidden="true">
              V
            </span>
            <div>
              Valparaíso
              <small>Ámbito regional</small>
            </div>
            <Icon name="territory" />
          </div>
          <nav aria-label="Navegación principal">
            {navigationGroups.map(group => (
              <div key={group}>
                <p className="nav-group">{group}</p>
                {routes
                  .filter(candidate => candidate.group === group)
                  .map(candidate => {
                    const active = candidate.id === route.id;
                    return (
                      <a
                        key={candidate.id}
                        id={`nav-${candidate.id}`}
                        className={active ? 'nav-link is-active' : 'nav-link'}
                        href={candidate.hash}
                        aria-current={active ? 'true' : undefined}
                      >
                        <Icon name={candidate.icon} />
                        <span>{candidate.label}</span>
                      </a>
                    );
                  })}
              </div>
            ))}
          </nav>
          <div className="sidebar-bottom">
            <section className="health-card" aria-labelledby="health-title">
              <div className="health-head">
                <strong id="health-title">Gateway</strong>
                <span className="health-dot" aria-hidden="true" />
              </div>
              <p id="readiness-status" role="status" aria-live="polite">
                Consultando disponibilidad del gateway…
              </p>
              <button id="readiness-refresh" className="side-action" type="button">
                Actualizar estado
              </button>
            </section>
            <div className="session-card">
              <span className="session-mark" aria-hidden="true">
                ●
              </span>
              <div>
                <strong id="session-state">Sin conexión</strong>
                <small id="session-detail">Token requerido para consultar</small>
              </div>
              <button id="logout" type="button" hidden>
                Cerrar sesión
              </button>
            </div>
          </div>
        </aside>

        <div className="main-area">
          <header className="topbar">
            <div>
              <span className="breadcrumbs">
                FireWatch <span>/</span> {route.group}
              </span>
              <h1>{route.title}</h1>
            </div>
            <div className="zone-chip">
              <span aria-hidden="true">⌖</span>
              <div>
                <small>ZONA</small>
                <strong id="zone-chip-name">Sin zona</strong>
              </div>
            </div>
          </header>
          <div className="update-bar">
            <Icon name="refresh" />
            <span>
              Información revisada:{' '}
              <strong id="update-checked">sin verificar, conecta un token para comprobar los servicios</strong>
            </span>
            <a
              className="technical-notice"
              id="data-status-notice"
              href="#sources"
              aria-label="Aviso de alcance: el servicio entrega entrada sintética de demostración, sin calibración ni uso operativo. Ver el estado de datos para el detalle completo."
            >
              Entrada sintética · aviso de alcance
            </a>
          </div>
          <main id="content">
            {children}
            <footer>
              <span>FireWatch · La API no realiza predicciones ni recomendaciones operativas.</span>
              <button id="reset-filters" type="button">
                <Icon name="refresh" />
                Restablecer filtros
              </button>
            </footer>
          </main>
        </div>
      </div>
    </>
  );
}
