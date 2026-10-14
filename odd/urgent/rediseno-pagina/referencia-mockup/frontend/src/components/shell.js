import { routes } from '../core/router.js';
import { escape, icon } from './ui.js';

export function shell(state, view) {
  let group = '';
  const navigation = Object.entries(routes).filter(([, route]) => !route.admin || state.role === 'admin').map(([id, route]) => {
    const heading = route.group !== group ? `<p class="nav-group">${escape(route.group)}</p>` : '';
    group = route.group;
    return `${heading}<a href="#${id}" class="nav-link ${state.route === id ? 'is-active' : ''}" ${state.route === id ? 'aria-current="page"' : ''}>${icon(route.icon)}<span>${escape(route.label)}</span>${state.route === id ? '<span class="nav-marker"></span>' : ''}</a>`;
  }).join('');
  return `
    <aside class="sidebar" id="navigation" aria-label="Navegación principal">
      <a class="brand" href="#overview">${icon('flame')}<span>FireWatch<small>PREVENCIÓN TERRITORIAL</small></span></a>
      <div class="workspace"><span class="workspace-mark">V</span><div>Valparaíso<small>Gestión territorial</small></div>${icon('layers')}</div>
      <nav>${navigation}</nav>
       <div class="sidebar-bottom">Prevención territorial<small>Coordinar. Actuar. Revisar.</small></div>
    </aside>
    <div class="main-shell">
      <header class="topbar">
        <button class="icon-button menu-button" type="button" data-command="MENU" aria-label="Abrir o cerrar navegación" aria-controls="navigation" aria-expanded="false">${icon('menu')}</button>
        <div class="breadcrumb">Espacio territorial <span>/</span> <strong>${escape(routes[state.route].label)}</strong></div>
        <div class="topbar-controls"><label class="role-control"><span>Perfil</span><select id="role-select" data-control="role" aria-label="Perfil"><option value="viewer" ${state.role === 'viewer' ? 'selected' : ''}>Consulta</option><option value="admin" ${state.role === 'admin' ? 'selected' : ''}>Coordinación</option></select></label><span class="avatar" aria-hidden="true">${state.role === 'admin' ? 'CO' : 'CN'}</span></div>
      </header>
       <div class="update-bar">${icon('history')} Información revisada: 15 ene 2026 · 15:00 UTC</div>
      <main id="content" tabindex="-1">${view}</main>
       <footer class="footer"><span>FireWatch <span class="footer-dot">·</span> Prevención territorial</span><button type="button" data-command="RESET">${icon('reset')} Restablecer filtros</button></footer>
    </div>`;
}
