import { heading, panelHeader, table, chip, icon } from '../components/ui.js';

export function access(state) {
  const matrix = [
    ['Territorio e historial', 'Consultar sectores y publicaciones', 'Consultar sectores y publicaciones'],
    ['Acciones preventivas', 'Consultar responsables, plazos y evidencia', 'Crear acciones y actualizar su seguimiento'],
    ['Revisión de evidencia', 'Consultar el criterio registrado', 'Registrar suficiencia y solicitar reevaluación'],
    ['Fuentes y calidad', '—', 'Revisar hallazgos e incorporar registros'],
    ['Análisis territorial', '—', 'Revisar versiones y registrar decisiones'],
    ['Actualización de información', '—', 'Solicitar y dar seguimiento a registros'],
    ['Registro de actividad', '—', 'Consultar decisiones y contexto']
  ];
  return `${heading('Perfiles y permisos', 'Responsabilidades claras para prevenir.', 'Consulte qué tareas corresponden a cada perfil y cómo se revisan las acciones preventivas.')}
    <div class="two-columns"><section class="panel padded access-card">${icon('lock')}<h2>Perfil de trabajo</h2><p>Consulta se centra en la lectura territorial y el seguimiento. Coordinación reúne las tareas de gestión y revisión.</p><dl class="detail-list"><div><dt>Consulta</dt><dd>Territorio, historial y acciones preventivas</dd></div><div><dt>Coordinación</dt><dd>Gestión de referencias, acciones y evidencia</dd></div><div><dt>Perfil seleccionado</dt><dd>${chip('unknown', state.role === 'admin' ? 'Coordinación' : 'Consulta')}</dd></div></dl><p class="panel-footnote">Al pasar a Consulta desde una vista de gestión, se vuelve al territorio.</p></section>
    <section class="panel padded access-card">${icon('layers')}<h2>Del contexto a la revisión</h2><p><strong>Priorizar:</strong> considere vigencia, cobertura, factores y referencias históricas por separado.</p><p><strong>Actuar:</strong> defina responsable, plazo y prioridad para una tarea verificable.</p><p><strong>Revisar:</strong> registre evidencia, criterio de suficiencia y justificación antes de solicitar una reevaluación.</p></section></div>
    <section class="panel">${panelHeader('Responsabilidades por perfil', 'Consulta y coordinación de la prevención')}${table(['Área', 'Consulta', 'Coordinación'], matrix.map(row => `<tr>${row.map(cell => `<td>${cell}</td>`).join('')}</tr>`), 'Responsabilidades por perfil')}</section>
     <section class="panel padded">${panelHeader('Criterios para decidir')}<div class="three-columns"><div><h3>Vigencia y cobertura</h3><p class="muted">Considere la vigencia, cobertura y disponibilidad de información de cada sector.</p></div><div><h3>Referencias territoriales</h3><p class="muted">Los incendios observados aportan contexto histórico para la planificación.</p></div><div><h3>Revisión coordinada</h3><p class="muted">Registre evidencia y criterios claros al revisar el avance de cada acción.</p></div></div></section>`;
}
