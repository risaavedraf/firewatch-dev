import { render, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';

import { clearZoneState } from '../../src/app/store';
import { gatewayOrigin, issuer, pkceTransactionKey, timing } from '../../src/config';
import { completeLogin, resetSession } from '../../src/core/session';
import { SourcesView } from '../../src/views/SourcesView';

import { createFetchStub, createStorageStub, flush, jwt } from '../core/harness';

// N8b — the sources + services view. It carries the scope declaration (S15),
// reproduces the provenance fields verbatim, reproduces the restriction text
// without paraphrasing, and only reports a service as verified when the domain
// rule says its own snapshot earns it. It stubs `fetch` end to end so a failure
// can never be papered over with fixture data.

const TOKEN_URL = `${issuer}/protocol/openid-connect/token`;
const SOURCES_URL = `${gatewayOrigin}/v1/risk/sources`;
const RISK_STATUS_URL = `${gatewayOrigin}/v1/risk/status`;
const JOBS_STATUS_URL = `${gatewayOrigin}/v1/jobs/status`;

function provenanceBody(): Record<string, unknown> {
  return {
    geography: {
      provenance: {
        provider: 'MOP - IDE Chile (SERVICIO_DPA, capa Comunas)',
        attribution: 'Atribución literal de la fuente',
        restriction: 'Restricción literal de la fuente',
        record_count: 38,
        derived_sha256: 'c'.repeat(64),
      },
      source_cutoff_at: null,
      cutoff_semantics: 'retrieval_substitution: stored cutoff equals retrieved',
      integrity: { algorithm: 'sha256', matches: true },
    },
    risk_input: { synthetic: true, display_name: 'Perfil académico', id: 'academic' },
  };
}

function statusBody(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    ready: true,
    reason: '',
    checked_at: new Date().toISOString(),
    dependencies: {
      database: { state: 'ready' },
      broker: { state: 'ready' },
      dead_letter: { state: 'ready', depth: 0, max_depth: 1, age_seconds: 0, max_age_seconds: 120 },
    },
    ...overrides,
  };
}

function prepare() {
  const fetchStub = createFetchStub();
  const storageStub = createStorageStub();
  vi.stubGlobal('fetch', fetchStub.fetch);
  vi.stubGlobal('sessionStorage', storageStub.storage);
  return { fetchStub, storageStub };
}

async function connect(): Promise<ReturnType<typeof prepare>> {
  const context = prepare();
  const token = jwt({ realm_access: { roles: ['viewer'] } });
  context.storageStub.storage.setItem(pkceTransactionKey, JSON.stringify({ state: 's', verifier: 'v' }));
  const login = completeLogin('?code=code-1&state=s');
  await flush();
  context.fetchStub.answer(TOKEN_URL, { access_token: token });
  await login;
  context.fetchStub.calls.length = 0;
  return context;
}

function text(id: string): string {
  return document.getElementById(id)?.textContent ?? '';
}

beforeEach(() => {
  resetSession();
  clearZoneState();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

test('the section keeps id=sources, data-view=sources and every legacy anchor', () => {
  prepare();
  render(<SourcesView />);

  const section = document.getElementById('sources');
  expect(section).not.toBeNull();
  expect(section).toHaveAttribute('data-view', 'sources');

  for (const id of [
    'sources-title',
    'sources-badge',
    'sources-status',
    'sources-provider',
    'sources-attribution',
    'sources-restriction',
    'sources-record-count',
    'sources-derived-sha256',
    'sources-integrity',
    'sources-cutoff',
    'sources-input',
    'pipeline',
    'pipeline-title',
    'pipeline-badge',
    'pipeline-status',
    'pipeline-risk',
    'pipeline-jobs',
    'pipeline-gateway',
  ]) {
    expect(document.getElementById(id)).not.toBeNull();
  }
});

test('without a session both panels state they are unconsulted and issue no read', () => {
  const { fetchStub } = prepare();
  render(<SourcesView />);

  expect(fetchStub.calls).toHaveLength(0);
  expect(text('sources-badge')).toBe('SIN CONSULTAR');
  expect(text('sources-status')).toBe('Conecta un token autorizado para consultar la procedencia.');
  expect(text('sources-provider')).toBe('Sin consultar');
  expect(text('pipeline-badge')).toBe('SIN VERIFICAR');
  expect(text('pipeline-risk')).toBe('Sin verificar');
});

test('a read in flight states the load and shows no healthy-looking source', async () => {
  const { fetchStub } = await connect();
  render(<SourcesView />);
  await flush();

  for (const url of [SOURCES_URL, RISK_STATUS_URL, JOBS_STATUS_URL]) {
    expect(fetchStub.callsFor(url)).toHaveLength(1);
  }
  expect(text('sources-badge')).toBe('CONSULTANDO');
  expect(text('sources-status')).toMatch(/consultando procedencia/i);
  expect(text('sources-provider')).toBe('Sin consultar');
  expect(text('pipeline-status')).toMatch(/consultando servicios/i);

  fetchStub.answer(SOURCES_URL, provenanceBody());
  fetchStub.answer(RISK_STATUS_URL, statusBody());
  fetchStub.answer(JOBS_STATUS_URL, statusBody());
  await waitFor(() => expect(text('sources-badge')).toBe('PUBLICADA'));
});

test('the scope declaration lives inside this view and names the real limits', () => {
  prepare();
  render(<SourcesView />);

  const scope = document.querySelector('#sources .warning-banner');
  expect(scope).not.toBeNull();
  expect(scope).toHaveTextContent(/Entrada de demostración/i);
  expect(scope).toHaveTextContent(/no calibrada/i);
  expect(scope).toHaveTextContent(/CONAF ni a SENAPRED/i);
});

test('the actions and findings note stays visible inside the view', () => {
  prepare();
  render(<SourcesView />);

  const note = document.querySelector('#sources .unimplemented-note');
  expect(note).not.toBeNull();
  expect(note).toHaveTextContent(/hallazgos de calidad/i);
});

test('provenance is reproduced verbatim and the integrity line only claims what the service verified', async () => {
  const { fetchStub } = await connect();
  render(<SourcesView />);
  await flush();

  fetchStub.answer(SOURCES_URL, provenanceBody());
  fetchStub.answer(RISK_STATUS_URL, statusBody());
  fetchStub.answer(JOBS_STATUS_URL, statusBody());
  await waitFor(() => expect(text('sources-badge')).toBe('PUBLICADA'));

  const body = provenanceBody() as { geography: { provenance: Record<string, string | number> } };
  const provenance = body.geography.provenance;
  expect(text('sources-provider')).toBe(provenance.provider);
  expect(text('sources-attribution')).toBe(provenance.attribution);
  expect(text('sources-restriction')).toBe(provenance.restriction);
  expect(text('sources-restriction')).not.toMatch(/aprobad|licencia adicional/i);
  expect(text('sources-record-count')).toBe('38');
  expect(text('sources-derived-sha256')).toBe(provenance.derived_sha256);
  expect(text('sources-integrity')).toMatch(/coincide/i);
  expect(text('sources-integrity')).toMatch(/sha256/i);
  expect(text('sources-integrity')).not.toMatch(/verificado por el cliente/i);
  expect(text('sources-cutoff')).toMatch(/sin fecha de corte real/i);
  expect(text('sources-input')).toMatch(/sintética/i);
  expect(text('sources-input')).toMatch(/no es riesgo real observado/i);
  expect(text('sources-status')).toMatch(/no se acredita aprobación de DIFROL/i);
});

test('provenance fails closed on a 503 body and never looks like a healthy source', async () => {
  const { fetchStub } = await connect();
  render(<SourcesView />);
  await flush();

  fetchStub.answer(SOURCES_URL, { code: 'geography_integrity_unavailable' }, 503);
  fetchStub.answer(RISK_STATUS_URL, statusBody());
  fetchStub.answer(JOBS_STATUS_URL, statusBody());
  await waitFor(() => expect(text('sources-badge')).toBe('NO DISPONIBLE'));

  expect(text('sources-status')).toMatch(/no disponible/i);
  expect(text('sources-status')).toContain('geography_integrity_unavailable');
  expect(text('sources-provider')).toBe('No disponible');
  expect(text('sources-integrity')).toBe('No disponible');
});

test('an integrity mismatch fails closed even on a 200', async () => {
  const { fetchStub } = await connect();
  render(<SourcesView />);
  await flush();

  const mismatch = provenanceBody();
  (mismatch.geography as { integrity: { matches: boolean } }).integrity.matches = false;
  fetchStub.answer(SOURCES_URL, mismatch);
  fetchStub.answer(RISK_STATUS_URL, statusBody());
  fetchStub.answer(JOBS_STATUS_URL, statusBody());
  await waitFor(() => expect(text('sources-badge')).toBe('NO DISPONIBLE'));

  expect(text('sources-provider')).toBe('No disponible');
  expect(text('sources-status')).toMatch(/no disponible/i);
});

test('a malformed provenance payload fails closed', async () => {
  const { fetchStub } = await connect();
  render(<SourcesView />);
  await flush();

  fetchStub.answer(SOURCES_URL, { geography: {} });
  fetchStub.answer(RISK_STATUS_URL, statusBody());
  fetchStub.answer(JOBS_STATUS_URL, statusBody());
  await waitFor(() => expect(text('sources-badge')).toBe('NO DISPONIBLE'));

  expect(text('sources-provider')).toBe('No disponible');
});

test('a forbidden provenance read is stated as denied permission, not as unavailable shape', async () => {
  const { fetchStub } = await connect();
  render(<SourcesView />);
  await flush();

  fetchStub.answer(SOURCES_URL, { code: 'forbidden' }, 403);
  fetchStub.answer(RISK_STATUS_URL, statusBody());
  fetchStub.answer(JOBS_STATUS_URL, statusBody());
  await waitFor(() => expect(text('sources-badge')).toBe('NO DISPONIBLE'));

  expect(text('sources-status')).toMatch(/permiso/i);
  expect(text('sources-status')).toContain('403');
});

test('a service is verified only when its own snapshot earns it, and the gateway is excluded', async () => {
  const { fetchStub } = await connect();
  render(<SourcesView />);
  await flush();

  fetchStub.answer(SOURCES_URL, provenanceBody());
  fetchStub.answer(RISK_STATUS_URL, statusBody());
  fetchStub.answer(JOBS_STATUS_URL, statusBody());
  await waitFor(() => expect(text('pipeline-badge')).toBe('VERIFICADO'));

  expect(text('pipeline-risk')).toMatch(/verificado/i);
  expect(text('pipeline-jobs')).toMatch(/verificado/i);
  expect(text('pipeline-status')).toMatch(/no participa de esta verificación/i);
  expect(text('pipeline-gateway')).toMatch(/204/);
  expect(text('pipeline-gateway')).toMatch(/no acredita el pipeline/i);
});

test('a stale snapshot is unknown, never reported as verified', async () => {
  const { fetchStub } = await connect();
  render(<SourcesView />);
  await flush();

  const stale = statusBody({ checked_at: '2020-01-01T00:00:00Z' });
  fetchStub.answer(SOURCES_URL, provenanceBody());
  fetchStub.answer(RISK_STATUS_URL, stale);
  fetchStub.answer(JOBS_STATUS_URL, stale);
  await waitFor(() => expect(text('pipeline-badge')).toBe('SIN VERIFICAR'));

  expect(text('pipeline-risk')).toMatch(/desconocido/i);
  expect(text('pipeline-risk')).not.toMatch(/verificado/i);
  expect(text('pipeline-status')).not.toMatch(/verificado/i);
});

test('logout returns both panels to the explicit unconsulted state without keeping green marks', async () => {
  const { fetchStub } = await connect();
  render(<SourcesView />);
  await flush();

  fetchStub.answer(SOURCES_URL, provenanceBody());
  fetchStub.answer(RISK_STATUS_URL, statusBody());
  fetchStub.answer(JOBS_STATUS_URL, statusBody());
  await waitFor(() => expect(text('sources-badge')).toBe('PUBLICADA'));
  expect(text('pipeline-badge')).toBe('VERIFICADO');

  resetSession();
  await waitFor(() =>
    expect(text('sources-status')).toBe('Conecta un token autorizado para consultar la procedencia.'),
  );
  expect(text('sources-provider')).toBe('No disponible');
  expect(text('pipeline-risk')).toBe('Sin verificar');
  expect(text('pipeline-badge')).toBe('SIN VERIFICAR');
});

test('a 403 check reads as not authorized to check, never as down, and cannot green the pipeline', async () => {
  const { fetchStub } = await connect();
  render(<SourcesView />);
  await flush();

  fetchStub.answer(SOURCES_URL, provenanceBody());
  fetchStub.answer(RISK_STATUS_URL, statusBody());
  fetchStub.answer(JOBS_STATUS_URL, { code: 'forbidden' }, 403);
  await waitFor(() => expect(text('pipeline-jobs')).toMatch(/no autorizado para comprobar/i));

  expect(text('pipeline-jobs')).not.toMatch(/ca[ií]do|down/i);
  expect(text('pipeline-risk')).toMatch(/verificado/i);
  expect(text('pipeline-badge')).toBe('SIN VERIFICAR');
});

test('the pipeline refresh is rescheduled while signed in and cancelled on logout', async () => {
  vi.useFakeTimers();
  try {
    const { fetchStub } = await connect();
    render(<SourcesView />);
    await flush();

    fetchStub.answer(SOURCES_URL, provenanceBody());
    fetchStub.answer(RISK_STATUS_URL, statusBody());
    fetchStub.answer(JOBS_STATUS_URL, statusBody());
    await flush();
    const firstCycle = fetchStub.callsFor(RISK_STATUS_URL).length;
    expect(firstCycle).toBeGreaterThan(0);

    // One bounded cycle later the view asks again: a verification claim is
    // re-checked rather than kept indefinitely from one snapshot.
    await vi.advanceTimersByTimeAsync(timing.pipelineRefreshMs);
    await flush();
    expect(fetchStub.callsFor(RISK_STATUS_URL).length).toBe(firstCycle + 1);

    resetSession();
    await flush();
    const afterLogout = fetchStub.callsFor(RISK_STATUS_URL).length;

    // A closed session must not keep probing the API in the background.
    await vi.advanceTimersByTimeAsync(timing.pipelineRefreshMs * 3);
    await flush();
    expect(fetchStub.callsFor(RISK_STATUS_URL).length).toBe(afterLogout);
  } finally {
    vi.useRealTimers();
  }
});
