import { heading, panelHeader, table, escape, formatTime, presentationName } from '../components/ui.js';

export function audit(state) {
  return `${heading('Registro de actividad', 'Cada decisión deja contexto.', 'Consulte perfil, acción y fecha para dar seguimiento a la coordinación territorial.')}
    <section class="panel">${panelHeader('Registro de actividad', 'Últimos 100 eventos')}${table(['Evento', 'Perfil', 'Acción', 'Detalle', 'Hora UTC'], state.audit.map(event => `<tr><td><code>${escape(event.id)}</code></td><td>${escape(presentationName(event.actor))}</td><td><strong>${escape(event.action)}</strong></td><td class="audit-detail">${escape(['Modelo aprobado', 'Modelo revertido'].includes(event.action) ? presentationName(event.detail) : event.detail)}</td><td>15 ene · ${formatTime(event.time)}</td></tr>`), 'Registro de decisiones')}</section>
    <div class="quiet-note">La revisión de actividad complementa la evidencia de cada acción preventiva.</div>`;
}
