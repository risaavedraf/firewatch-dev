import { selectedSector, sectorRisk, latestBatch, freshness } from '../core/selectors.js';
import { terrainMap } from '../components/terrain-map.js';
import { heading, metric, panelHeader, chip, escape, formatTime, table, icon, presentationName } from '../components/ui.js';

export function overview(state) {
  const sector = selectedSector(state), batch = latestBatch(state);
  const rows = state.sectors.map((item, index) => `<tr><td><button class="text-button" type="button" data-command="SELECT_SECTOR" data-id="${escape(item.id)}"><span class="row-index">0${index + 1}</span>${escape(item.name)}</button></td><td>${chip(sectorRisk(state, item))}</td><td>${item.coverage} %</td><td>${escape(item.kind)}</td><td>${item.id === state.sectorId ? '<span class="selected-label">Seleccionado</span>' : '—'}</td></tr>`);
  return `${heading('Prevención territorial', 'Priorizar el territorio. Orientar la acción.', 'Revise información, identifique necesidades preventivas y dé seguimiento a las referencias territoriales.')}
    <div class="metrics-grid metrics-three">
      ${metric('Ámbito de consulta', `${state.sectors.length} sectores`, 'Entornos y cobertura territorial', 'map')}
       ${metric('Prioridad territorial', state.sectors.filter(item => sectorRisk(state, item) === 'high').length, 'Sectores que requieren coordinación preventiva', 'shield', 'metric-attention')}
      ${metric('Última emisión', formatTime(state.history.find(row => row.scenario === state.scenario)?.emittedAt), '15 ene 2026 · UTC', 'history')}
    </div>
     <div class="overview-grid"><section class="panel map-panel">${panelHeader('Vista territorial', 'Información para coordinar la prevención', '<span class="small-label">SECTORES</span>')}${terrainMap(state)}</section>
       <section class="panel sector-detail" aria-label="Detalle del sector seleccionado"><div class="detail-kicker">SECTOR ${escape(sector.id.slice(1).padStart(2, '0'))} ${icon('pin')}</div><h2>${escape(sector.name)}</h2><p class="muted">${escape(sector.kind)}</p><div class="risk-summary">${chip(sectorRisk(state, sector))}<p>Considere cobertura, vigencia y referencias al coordinar la atención.</p></div>
         <dl class="detail-list"><div><dt>Registro</dt><dd>${formatTime(batch?.emittedAt)} UTC</dd></div><div><dt>Vigencia hasta</dt><dd>${formatTime(batch?.expiresAt)}${batch ? ' UTC' : ''}</dd></div><div><dt>Estado del dato</dt><dd>${chip(freshness(batch))}</dd></div><div><dt>Cobertura</dt><dd>${sector.coverage} %</dd></div><div><dt>Referencia</dt><dd>${escape(presentationName(batch?.model ?? 'Sin registro'))}</dd></div><div><dt>Fuente</dt><dd>${escape(batch?.source ?? 'Sin datos disponibles')}</dd></div></dl>
         <div class="factor-section"><h3>Factores de atención</h3>${sector.factors.map((factor, i) => `<div class="factor"><span>${escape(factor)}</span><div class="factor-track"><span style="width:${85 - i * 18}%"></span></div></div>`).join('')}</div>
         <div class="uncertainty">${icon('info')}<div><strong>Información para coordinar la prevención</strong><p>${sector.id === 's4' ? 'Cobertura incompleta; se requiere información adicional para establecer una prioridad.' : 'La prioridad se basa en los factores territoriales registrados y requiere revisión contextual.'}</p></div></div>
        <a class="button button-primary full-width" href="#history">Consultar historial ${icon('arrow')}</a>
      </section></div>
     <section class="panel">${panelHeader('Prioridad por sector', 'Considere vigencia, cobertura y contexto territorial.')}${table(['Sector', 'Prioridad', 'Cobertura', 'Entorno', 'Selección'], rows, 'Resumen de sectores')}</section>`;
}
