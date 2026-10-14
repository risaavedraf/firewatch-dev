// FireWatch local product mockup — dependency-free presentation reference.
// This folder is a mockup: it is not served by the gateway, calls no backend, ingests no real
// records and loads no trained model. Every value below is illustrative in-memory state.
// Source descriptors are declarative placeholders: they deliberately carry no record counts,
// dates or quality metrics, because no verifiable ingestion exists in this folder.
export const CLOCK = '2026-01-15T15:00:00Z';

export function createInitialState() {
  return {
    role: 'viewer', scenario: 'current', sectorId: 's1', route: 'overview',
    query: '', freshnessFilter: 'all', selectedHistoryId: null,
    sectors: [
      { id: 's1', name: 'Cordón del Arrayán', kind: 'Interfaz bosque / viviendas', coverage: 92, risk: 'high', factors: ['Vegetación seca', 'Viento sostenido', 'Interfaz habitada'] },
      { id: 's2', name: 'Quebrada del Molino', kind: 'Corredor de vegetación', coverage: 84, risk: 'medium', factors: ['Continuidad de combustible', 'Pendiente expuesta'] },
      { id: 's3', name: 'Llanos del Roble', kind: 'Área rural abierta', coverage: 96, risk: 'low', factors: ['Menor carga vegetal', 'Humedad relativa'] },
      { id: 's4', name: 'Reserva del Estero', kind: 'Bosque de protección', coverage: 61, risk: 'not_evaluated', factors: ['Cobertura insuficiente'] }
    ],
    history: [
      { id: 'b104', sectorId: 's1', scenario: 'future', emittedAt: '2026-01-15T14:45:00Z', expiresAt: '2026-01-15T15:45:00Z', risk: 'high', model: 'scenario-v1', source: 'Paquete territorial A' },
      { id: 'b103', sectorId: 's2', scenario: 'future', emittedAt: '2026-01-15T14:30:00Z', expiresAt: '2026-01-15T15:30:00Z', risk: 'medium', model: 'scenario-v1', source: 'Paquete territorial A' },
      { id: 'b102', sectorId: 's1', scenario: 'future', emittedAt: '2026-01-15T12:00:00Z', expiresAt: '2026-01-15T13:00:00Z', risk: 'medium', model: 'scenario-v1', source: 'Paquete territorial B' },
      { id: 'demo-02', sectorId: 's1', scenario: 'current', emittedAt: '2026-01-15T14:45:00Z', expiresAt: '2026-01-15T15:45:00Z', risk: 'not_evaluated', model: 'demo-fixture-v1', source: 'Registro territorial' },
      { id: 'demo-01', sectorId: 's1', scenario: 'current', emittedAt: '2026-01-15T12:00:00Z', expiresAt: '2026-01-15T13:00:00Z', risk: 'not_evaluated', model: 'demo-fixture-v1', source: 'Registro territorial' }
    ],
    sources: [
      { id: 'weather', name: 'Meteorología', provider: 'Referencias de estaciones', quality: 'pending', detail: 'Formato, temporalidad y valores faltantes por verificar.' },
      { id: 'territory', name: 'Cobertura territorial', provider: 'Capas de suelo', quality: 'pending', detail: 'Coordenadas y duplicados por verificar.' }
    ],
    jobs: [{ id: 'job-01', scenario: 'current', status: 'published', batchId: 'demo-02', requestedAt: '14:45' }],
    audit: [{ id: 'event-01', action: 'Vista territorial disponible', actor: 'FireWatch', time: CLOCK, detail: 'Consulta de sectores y fuentes de referencia' }],
    sequence: 10, feedback: ''
  };
}
