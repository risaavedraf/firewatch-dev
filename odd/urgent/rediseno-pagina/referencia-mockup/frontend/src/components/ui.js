export function escape(value = '') {
  return String(value).replace(/[&<>"']/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[char]);
}

const paths = {
  flame: '<path d="M12 2c2 5-3 6-1 10 1-2 3-3 4-5 4 4 5 7 3 11-3 6-12 4-13-1-1-4 1-6 3-9 0 3 1 4 2 4-1-4 2-6 2-10Z"/>',
  map: '<path d="m3 5 6-2 6 2 6-2v16l-6 2-6-2-6 2V5Zm6-2v16m6-14v16"/>',
  history: '<path d="M3 11a9 9 0 1 1 2 7M3 4v7h7m2-5v6l4 2"/>',
  shield: '<path d="m12 3 8 3v6c0 5-8 9-8 9s-8-4-8-9V6l8-3Zm-4 9 3 3 5-6"/>',
  layers: '<path d="m12 3 10 5-10 5L2 8l10-5Zm-10 9 10 5 10-5M2 16l10 5 10-5"/>',
  model: '<path d="M8 5h8v14H8V5ZM3 8h5m8 0h5M3 16h5m8 0h5M10 2v3m4-3v3m-4 14v3m4-3v3"/>',
  activity: '<path d="M2 12h5l3-8 4 16 3-8h5"/>',
  list: '<path d="M9 5h12M9 12h12M9 19h12M3 5h1M3 12h1M3 19h1"/>',
  lock: '<rect x="5" y="10" width="14" height="11" rx="2"/><path d="M8 10V6a4 4 0 0 1 8 0v4m-4 5v2"/>',
  arrow: '<path d="M5 12h14m-5-5 5 5-5 5"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  check: '<path d="m5 12 4 4L19 6"/>',
  menu: '<path d="M4 6h16M4 12h16M4 18h16"/>',
  search: '<circle cx="10" cy="10" r="6"/><path d="m15 15 6 6"/>',
  reset: '<path d="M4 10a8 8 0 1 1 1 8M4 3v7h7"/>',
  info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v6m0-10v1"/>',
  pin: '<path d="M19 10c0 5-7 11-7 11S5 15 5 10a7 7 0 1 1 14 0Z"/><circle cx="12" cy="10" r="2"/>'
};

export function icon(name, className = '') {
  return `<svg class="icon ${escape(className)}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${paths[name] ?? paths.info}</svg>`;
}

export const labels = {
  high: 'Atención prioritaria', medium: 'Atención preventiva', low: 'Seguimiento habitual', not_evaluated: 'Sin información suficiente',
  current: 'Vigente', stale: 'Vencido', unknown: 'Sin vigencia',
  queued: 'En cola', published: 'Publicado', candidate: 'Candidato', active: 'Activo', archived: 'Archivado',
  pending: 'Pendiente', reviewed: 'Revisado', open: 'Pendiente', in_progress: 'En curso', closed: 'Cerrada',
  sufficient: 'Suficiente', insufficient: 'Insuficiente'
};

export function chip(value, label = labels[value] ?? value) {
  return `<span class="chip chip-${escape(value)}"><span class="chip-dot"></span>${escape(label)}</span>`;
}

export function button(label, command, { id = '', disabled = false, style = 'secondary', glyph = '', extra = '' } = {}) {
  return `<button class="button button-${escape(style)}" type="button" data-command="${escape(command)}" data-id="${escape(id)}" ${disabled ? 'disabled' : ''} ${extra}>${glyph ? icon(glyph) : ''}${escape(label)}</button>`;
}

export function heading(kicker, title, description, action = '') {
  return `<div class="page-heading"><div><p class="eyebrow">${escape(kicker)}</p><h1>${escape(title)}</h1><p class="page-description">${escape(description)}</p></div>${action}</div>`;
}

export function panelHeader(title, subtitle = '', action = '') {
  return `<div class="panel-heading"><div><h2>${escape(title)}</h2>${subtitle ? `<p>${escape(subtitle)}</p>` : ''}</div>${action}</div>`;
}

export function metric(label, value, note, glyph = 'activity', tone = '') {
  return `<article class="metric ${escape(tone)}"><div class="metric-top"><span>${escape(label)}</span>${icon(glyph)}</div><strong>${escape(value)}</strong><p>${escape(note)}</p></article>`;
}

export function formatTime(value) {
  if (!value) return 'Sin datos';
  return new Intl.DateTimeFormat('es-CL', { timeZone: 'UTC', hour: '2-digit', minute: '2-digit', hour12: false }).format(new Date(value));
}

export function presentationName(value) {
  const aliases = {
    'scenario-v1': 'Bosque · versión 01', 'scenario-v2': 'Bosque · versión 02',
    'demo-fixture-v1': 'Información territorial', 'demo-01': 'Registro 01', 'demo-02': 'Registro 02',
    'Administrador simulado': 'Coordinación'
  };
  return Object.hasOwn(aliases, value) ? aliases[value] : value;
}

export function empty(message) {
  return `<div class="empty">${icon('search')}<h3>Sin resultados</h3><p>${escape(message)}</p></div>`;
}

export function table(headers, rows, caption) {
  return `<div class="table-scroll" tabindex="0" role="region" aria-label="${escape(caption)}"><table><caption class="sr-only">${escape(caption)}</caption><thead><tr>${headers.map(header => `<th scope="col">${escape(header)}</th>`).join('')}</tr></thead><tbody>${rows.join('')}</tbody></table></div>`;
}

export function sectorOptions(state, selected = state.sectorId) {
  return state.sectors.map(sector => `<option value="${escape(sector.id)}" ${sector.id === selected ? 'selected' : ''}>${escape(sector.name)}</option>`).join('');
}
