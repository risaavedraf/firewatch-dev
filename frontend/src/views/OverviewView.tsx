// The territory overview: the consolidated metrics, the real territorial map and
// the latest published batch with its evidence.
//
// Three honesty rules shape this view:
//   1. every metric reports a real value with its own declared limit — the
//      catalogue length served by `/v1/risk/zones`, the history rows actually
//      returned for the selected zone, and the count of comunas whose level this
//      session actually read. Those levels come from two reads: the per-zone
//      `/v1/risk/latest` for the selected zone and the aggregate
//      `/v1/risk/batches` read once per session, and the "high level" card states
//      what the count means in either case;
//   2. the batch evidence is fail-closed: a payload that did not survive the
//      boundary guard is rendered as explicitly unavailable, never as a
//      plausible severity. A 404 on `latest` is "no published batch", never low;
//   3. operational validity is re-evaluated on its own bounded timer and is
//      cleared on logout, on a 401 reset and on a new selection, so a claim of
//      validity never survives the bound the batch itself declares.
//
// The element identifiers are the legacy contract preserved for N9; the visible
// copy is Spanish and is copied from the retired page.

import { useEffect, useMemo, useRef, useState } from 'react';

import {
  claimZoneBatchAggregateRead,
  clearZoneState,
  getSelectedZone,
  rememberZoneCatalogue,
  rememberZoneLevel,
  selectZone,
  useSelectedZone,
  useSessionState,
  useZoneLevels,
} from '../app/store';
import { idleCopy } from '../app/sessionCopy';
import { MapPanel, type MapLoadState, type ZoneLevelLookup } from '../components/map/MapPanel';
import { zoneLabel } from '../components/map/projection';
import { DataList, Metric, PanelHeading, StatusCopy, UnimplementedNote } from '../components/ui';
import { batches, history, latest, modelVersions, zones, type Failure, type Outcome } from '../core/api';
import type { SessionState } from '../core/session';
import {
  computeFactorBars,
  evaluateActiveModel,
  evaluateValidity,
  latestOutcome,
  levelLabel,
  presentBatch,
  validityLabel,
  type ActiveModel,
} from '../domain';
import type { RiskBatch, ZoneCollection, ZoneFeature } from '../types/api';

// Copy reproduced from the retired page. The operational-validity label and the
// synthetic-input notice are deliberately separate: a `published_at` timestamp
// is not observation freshness, and only the first one may say "Vigente".
const DEMO_ORIGIN = 'Sintético · Synthetic';
const DISCREPANT_ORIGIN = 'Procedencia discrepante con este entorno sintético';
const SYNTHETIC_NOTICE = 'Entorno sintético: no riesgo real observado.';
const MISSING_NOTICE = 'Aviso de la API ausente o inválido.';
const WRONG_ZONE_STATUS = 'La API devolvió un lote de otra zona; se descartó.';
const NO_BATCH_STATUS = 'La API no informa lotes publicados para esta zona.';
const IDLE_STATUS_NO_ZONE = 'Selecciona una zona para consultar el último lote.';
const LOADING_STATUS = 'Consultando el último lote…';
const DEMO_BATCH_STATUS =
  'Entradas de demostración sintéticas: no representan riesgo real observado. La publicación no acredita frescura de observaciones. Clasificación por regla propia no oficial.';
const READ_BATCH_STATUS = 'Lote recibido de la API · clasificación calculada por una regla propia no oficial.';

type CatalogueState = { kind: 'ready'; collection: ZoneCollection } | { kind: 'error' };
type CataloguePhase = 'idle' | 'loading' | 'ready' | 'error' | 'cleared';

type BatchState =
  | { kind: 'idle' }
  | { kind: 'loading' }
  | { kind: 'none' }
  | { kind: 'failed'; message: string }
  | { kind: 'batch'; batch: RiskBatch };

type HistoryState = { kind: 'idle' } | { kind: 'loading' } | { kind: 'ready'; count: number } | { kind: 'failed' };

interface ValidityView {
  text: string;
  nextDelayMs: number | null;
}

// One settled read of the selected zone. Keeping the zone it was read for lets
// the render derive the loading phase when the selection moves away, without an
// effect that clears state synchronously.
interface ReadState {
  zone: string;
  batch: BatchState;
  history: HistoryState;
  validity: ValidityView;
}

function isNonEmpty(value: unknown): value is string {
  return typeof value === 'string' && value.trim() !== '';
}

// `setText` in the retired page: an absent or empty field is labelled in place,
// never filled in from another field and never coerced into a plausible value.
function orUninformed(value: string | undefined): string {
  return value === undefined || value === '' ? 'No informado' : value;
}

function failureMessage(failure: Failure, action: string): string {
  if (failure.kind === 'forbidden') {
    return 'La API denegó el permiso para esta acción (403). Solicita acceso a un administrador.';
  }
  const detail = failure.status !== undefined ? `HTTP ${failure.status}` : 'sin conexión';
  return `No se pudo ${action} (${detail}).`;
}

// The first comuna the catalogue offers, in the same order the map's selector
// presents them. The retired page selected it on load; reproducing that keeps the
// metrics real from the first read instead of waiting for a click. It is only
// used to fill an empty selection (S2): a zone the reader already chose outranks
// it, and a remount must not move the selection back here.
function firstCatalogueCode(collection: ZoneCollection): string {
  const codes: string[] = [];
  for (const feature of collection.features) {
    const code = feature?.properties?.code;
    if (typeof code === 'string' && code !== '') codes.push(code);
  }
  codes.sort((left, right) => left.localeCompare(right));
  return codes.length > 0 ? codes[0] : '';
}

// The display names the loaded catalogue carries, keyed by the code the shared
// store selects on. Only a feature with a real code contributes: a label without
// a key would name a zone the chip can never select, and the format is the map's
// own `zoneLabel` so the selector and the chip can never disagree.
function catalogueZoneLabels(collection: ZoneCollection): Map<string, string> {
  const labels = new Map<string, string>();
  for (const feature of collection.features) {
    const code = feature?.properties?.code;
    if (typeof code === 'string' && code !== '') labels.set(code, zoneLabel(feature.properties));
  }
  return labels;
}

interface RiskCopy {
  pill: string;
  level: string;
  status: string;
}

function riskCopy(batch: BatchState, session: SessionState): RiskCopy {
  switch (batch.kind) {
    case 'idle':
      return {
        pill: 'SIN CONSULTAR',
        level: 'Sin datos',
        status: session.status === 'signed-in' ? IDLE_STATUS_NO_ZONE : idleCopy(session, 'consultar la API'),
      };
    case 'loading':
      return { pill: 'SIN CONSULTAR', level: 'Sin consultar', status: LOADING_STATUS };
    case 'none':
      return { pill: 'SIN LOTE', level: 'Sin lote publicado', status: NO_BATCH_STATUS };
    case 'failed':
      return { pill: 'NO DISPONIBLE', level: 'Sin datos', status: batch.message };
    case 'batch': {
      const presentation = presentBatch(batch.batch);
      if (!presentation.valid) {
        return {
          pill: 'NO DISPONIBLE',
          level: 'NO DISPONIBLE',
          status: `Lote incompleto: ${presentation.omissions.join(', ')}. Sin evaluación validada; entorno sintético, no riesgo real observado.`,
        };
      }
      const label = levelLabel(batch.batch.risk_level);
      return { pill: label, level: label, status: batch.batch.demo === true ? DEMO_BATCH_STATUS : READ_BATCH_STATUS };
    }
  }
}

// The batch state a `latest` outcome maps to. The `rememberZoneLevel` side effect
// lives here so an unreadable batch forgets the zone instead of keeping a stale
// colour on the map.
function batchFrom(zone: string, result: Outcome<RiskBatch | null>): BatchState {
  if (result.ok) {
    if (result.data === null) {
      // A 404 is an absence the map paints neutral, never a low risk.
      rememberZoneLevel(zone, null);
      return { kind: 'none' };
    }
    const presentation = presentBatch(result.data);
    const level = presentation.valid ? presentation.level : '';
    rememberZoneLevel(zone, level === '' ? null : level);
    return { kind: 'batch', batch: result.data };
  }
  const failure = result.failure;
  if (failure.reason === 'zone_mismatch') {
    rememberZoneLevel(zone, null);
    return { kind: 'failed', message: WRONG_ZONE_STATUS };
  }
  if (failure.status !== undefined) {
    const outcome = latestOutcome(failure.status);
    if (outcome === 'unauthorized') return { kind: 'loading' };
    if (outcome === 'forbidden') return { kind: 'failed', message: failureMessage(failure, 'consultar el lote') };
    if (outcome === 'no-published-batch') {
      rememberZoneLevel(zone, null);
      return { kind: 'none' };
    }
  }
  // A body that did not survive the boundary guard is explicitly unavailable,
  // never a plausible partial batch.
  if (failure.kind === 'malformed') {
    return { kind: 'failed', message: 'La API devolvió un lote ilegible; se muestra como no disponible.' };
  }
  return { kind: 'failed', message: failureMessage(failure, 'consultar el lote') };
}

function historyFrom(result: Outcome<RiskBatch[]>): HistoryState {
  return result.ok ? { kind: 'ready', count: result.data.length } : { kind: 'failed' };
}

// Operational validity is a property of the batch window. The label is computed
// here, never during render: `Date.now()` is impure, and this runs from async
// callbacks and the bounded timer instead.
function computeValidity(batch: BatchState): ValidityView {
  if (batch.kind !== 'batch') return { text: 'Sin consultar', nextDelayMs: null };
  const decision = evaluateValidity(batch.batch, Date.now());
  return { text: validityLabel(decision), nextDelayMs: decision.nextDelayMs };
}

export function OverviewView() {
  const session = useSessionState();
  const selectedZone = useSelectedZone();
  const levels = useZoneLevels();
  const signedIn = session.status === 'signed-in';

  const [catalogue, setCatalogue] = useState<CatalogueState | null>(null);
  const [read, setRead] = useState<ReadState | null>(null);
  const [activeModel, setActiveModel] = useState<ActiveModel | null>(null);
  const [sessionClosed, setSessionClosed] = useState(false);
  // Whether a real session was ever established. It separates "never connected"
  // (the count is still unknown, `—`) from "the session was closed" (the count
  // is explicitly uninformed), which the retired page distinguished the same way.
  const hadSession = useRef(false);

  // The catalogue: read once per session and abandoned the moment the session
  // closes, so a late answer cannot repopulate a view that no longer exists.
  useEffect(() => {
    if (!signedIn) return undefined;
    const controller = new AbortController();
    let active = true;
    void (async () => {
      const result = await zones(controller.signal);
      if (!active) return;
      if (!result.ok) {
        if (result.failure.reason === 'superseded' || result.failure.kind === 'unauthorized') return;
        setCatalogue({ kind: 'error' });
        return;
      }
      setCatalogue({ kind: 'ready', collection: result.data });
      // The labels are recorded with the selection, never before it: the chip may
      // only name a zone whose catalogue this session actually loaded.
      rememberZoneCatalogue(catalogueZoneLabels(result.data));
      // S2 — the auto-selection only fills an EMPTY selection. This effect runs on
      // every mount of this view, so selecting unconditionally replaced the zone
      // the reader had chosen with the first catalogue code (Valparaíso, 05101)
      // as soon as they left the map and came back. The selected zone is read from
      // the store rather than from this render so the effect keeps running once
      // per session instead of once per selection.
      if (getSelectedZone() === '') {
        const first = firstCatalogueCode(result.data);
        if (first !== '') selectZone(first);
      }
    })();
    return () => {
      active = false;
      controller.abort();
    };
  }, [signedIn]);

  // S1/S4 — one aggregate read per session, so every comuna's last published
  // level is on the map without a click. It runs alongside the catalogue read and
  // is claimed through the store, which is what makes it per-session rather than
  // per-mount: coming back to this view finds the claim taken and issues nothing,
  // while a new session claims it again.
  //
  // Each element is validated with the same fail-closed `presentBatch` the risk
  // panel uses, and only a valid one may set a level. An element that fails
  // validation, and a zone the answer omits, are left alone: never a level, never
  // a default, never `bajo`. A failed, malformed, 401/403 or 503 answer paints
  // nothing at all, leaving the map exactly as the per-zone reads leave it.
  //
  // The read deliberately carries no view-lifetime abort: it belongs to the
  // session, so leaving the view mid-flight must still colour the map for the
  // next visit. The transport's own fences protect it — the bounded deadline and
  // the token check that discards an answer issued before a logout.
  useEffect(() => {
    if (!signedIn) return;
    if (!claimZoneBatchAggregateRead()) return;
    void (async () => {
      const result = await batches();
      if (!result.ok) return;
      for (const entry of result.data) {
        const presentation = presentBatch(entry);
        if (!presentation.valid || presentation.level === '') continue;
        rememberZoneLevel(entry.zone_id, presentation.level);
      }
    })();
  }, [signedIn]);

  // One selection reads both endpoints. The transport fences every response by
  // its own sequence and by the token it was issued with, and this effect's
  // cleanup aborts whatever the previous selection started.
  useEffect(() => {
    if (!signedIn || selectedZone === '') return undefined;
    const controller = new AbortController();
    let active = true;
    void (async () => {
      const [latestResult, historyResult] = await Promise.all([
        latest(selectedZone, controller.signal),
        history(selectedZone, controller.signal),
      ]);
      if (!active) return;
      // A superseded or aborted answer belongs to a request the view already
      // replaced: it owns nothing and must not repaint the panel.
      if (latestResult.ok === false && (latestResult.failure.reason === 'superseded' || latestResult.failure.reason === 'aborted')) {
        return;
      }
      const batch = batchFrom(selectedZone, latestResult);
      setRead({ zone: selectedZone, batch, history: historyFrom(historyResult), validity: computeValidity(batch) });
    })();
    return () => {
      active = false;
      controller.abort();
    };
  }, [signedIn, selectedZone]);

  // T13 — the active model version, read from the model registry. The read is
  // independent of the zone chain: a registry that is unreachable or does not
  // report an active version is stated, never replaced with the batch's version
  // as if it were the active one.
  useEffect(() => {
    if (!signedIn) return undefined;
    const controller = new AbortController();
    let active = true;
    void (async () => {
      const outcome = await modelVersions(controller.signal);
      if (!active) return;
      setActiveModel(outcome.ok ? evaluateActiveModel(outcome.data) : evaluateActiveModel(null));
    })();
    return () => {
      active = false;
      controller.abort();
    };
  }, [signedIn]);

  // Ending a session clears everything the private view was describing. The
  // shared zone state is cleared here as well so the view stays honest even when
  // it is mounted without the session binding installed at application start.
  useEffect(() => {
    if (signedIn) {
      hadSession.current = true;
      return;
    }
    if (!hadSession.current) return;
    hadSession.current = false;
    clearZoneState();
    setCatalogue(null);
    setRead(null);
    setActiveModel(null);
    setSessionClosed(true);
  }, [signedIn]);

  // The bounded validity re-evaluation. `evaluateValidity` caps the delay at
  // `timing.validityMaxDelayMs` and expires the claim `timing.validitySkewMs`
  // early, so this single chained timer re-renders the label no later than the
  // bound the batch declares. Replacing the batch or the selection clears it.
  const currentRead = selectedZone !== '' && read !== null && read.zone === selectedZone ? read : null;
  const validityBatch = currentRead !== null && currentRead.batch.kind === 'batch' ? currentRead.batch.batch : null;
  const validityDelay = currentRead !== null ? currentRead.validity.nextDelayMs : null;
  useEffect(() => {
    if (!signedIn || validityBatch === null || validityDelay === null) return undefined;
    const timer = setTimeout(() => {
      setRead(previous =>
        previous === null || previous.zone !== selectedZone
          ? previous
          : { ...previous, validity: computeValidity(previous.batch) },
      );
    }, validityDelay);
    return () => clearTimeout(timer);
  }, [validityBatch, validityDelay, signedIn, selectedZone]);

  // The store hands the view a `ReadonlyMap`; `MapPanel` reads a single
  // `levelFor`. An unread level stays `''`, which the map paints neutral. The
  // map's contract types the absent level as `''` rather than `null`.
  const zoneLevelLookup = useMemo(
    (): ZoneLevelLookup => ({ levelFor: (code: string) => levels.get(code) ?? '' }),
    [levels],
  );

  const highCount = useMemo(() => {
    let count = 0;
    for (const level of levels.values()) if (level === 'alto') count += 1;
    return count;
  }, [levels]);

  const cataloguePhase: CataloguePhase =
    catalogue !== null ? catalogue.kind : signedIn ? 'loading' : sessionClosed ? 'cleared' : 'idle';
  const features = catalogue !== null && catalogue.kind === 'ready' ? catalogue.collection.features : [];
  const selectedFeature: ZoneFeature | null =
    selectedZone === '' ? null : (features.find(feature => feature.properties.code === selectedZone) ?? null);

  const zoneCountText =
    cataloguePhase === 'ready'
      ? String(features.length)
      : cataloguePhase === 'error' || cataloguePhase === 'cleared'
        ? 'No informado'
        : '—';
  const mapCollection = catalogue !== null && catalogue.kind === 'ready' ? catalogue.collection : null;
  const mapState: MapLoadState =
    cataloguePhase === 'ready' ? 'ready' : cataloguePhase === 'loading' ? 'loading' : cataloguePhase === 'error' ? 'error' : 'idle';

  const currentBatch: BatchState =
    selectedZone === ''
      ? { kind: 'idle' }
      : currentRead === null
        ? { kind: 'loading' }
        : currentRead.batch;
  const currentHistory: HistoryState =
    selectedZone === ''
      ? { kind: 'idle' }
      : currentRead === null
        ? { kind: 'loading' }
        : currentRead.history;

  const risk = riskCopy(currentBatch, session);
  const historyText = currentHistory.kind === 'ready' ? String(currentHistory.count) : '—';
  const validityText = currentRead === null ? 'Sin consultar' : currentRead.validity.text;

  const batchData = currentBatch.kind === 'batch' ? currentBatch.batch : null;
  const presentation = batchData === null ? null : presentBatch(batchData);
  const factorBars = batchData === null ? [] : computeFactorBars(batchData);
  const showFactorOmission = presentation !== null && presentation.omissions.includes('factores');
  const originText = batchData === null ? 'Sin consultar' : batchData.demo === true ? DEMO_ORIGIN : DISCREPANT_ORIGIN;
  const noticeText =
    batchData === null
      ? SYNTHETIC_NOTICE
      : `${SYNTHETIC_NOTICE} ${isNonEmpty(batchData.notice) ? batchData.notice : MISSING_NOTICE}`;
  const rationaleText =
    batchData === null
      ? 'Sin consultar'
      : isNonEmpty(batchData.rationale)
        ? batchData.rationale
        : 'Justificación ausente o inválida';
  const policyText =
    batchData === null
      ? 'Sin consultar'
      : presentation !== null && presentation.policyValid && batchData.policy.parameters
        ? `${batchData.policy.policy_id} · hash informado (no verificado): ${batchData.policy.params_hash} · ${Object.entries(
            batchData.policy.parameters,
          )
            .map(([key, value]) => `${key}=${value}`)
            .join('; ')}`
        : 'Política ausente o inválida';
  // After a batch settles, a field the service omitted is labelled in place.
  const batchField = (value: string | undefined): string => (batchData === null ? 'Sin consultar' : orUninformed(value));

  // T13 — the active model version and its `synthetic` mark. The mark is shown
  // only when the registry declares it: an undeclared mark is its own state, and
  // an unreachable registry is stated instead of a plausible version.
  const modelBadge =
    activeModel === null
      ? 'SIN CONSULTAR'
      : !activeModel.available
        ? 'NO DISPONIBLE'
        : activeModel.synthetic === true
          ? 'SINTÉTICO'
          : activeModel.synthetic === false
            ? 'NO SINTÉTICO'
            : 'NO DECLARADO';
  const modelStatusText = activeModel === null ? 'Sin consultar el registro de modelos.' : activeModel.label;

  return (
    <section className="view" id="overview" data-view="overview" aria-labelledby="overview-title">
      <div className="intro-row">
        <div>
          <p className="eyebrow">VISTA TERRITORIAL · VALPARAÍSO</p>
          <h2 id="overview-title">Estado de la zona</h2>
          <p className="subheading">Consulta de lote y trazabilidad disponible desde el servicio FireWatch.</p>
        </div>
        <span className="neutral-tag" id="input-scope-badge">
          ENTRADA SINTÉTICA · SIN CALIBRACIÓN
        </span>
      </div>

      <section className="metrics-grid metrics-three" aria-label="Resumen real de la sesión">
        <Metric
          label="Zonas analizadas"
          icon="zones"
          value={<span id="zone-count">{zoneCountText}</span>}
          note={
            <>
              Comunas del catálogo servido por <code>/v1/risk/zones</code>: features reales, no un valor copiado.
            </>
          }
        />
        <Metric
          label="Lotes en el historial"
          icon="batches"
          value={<span id="metric-batches">{historyText}</span>}
          note={
            <>
              Registros que <code>/v1/risk/history</code> devolvió para la zona seleccionada; sin agregar métricas
              calculadas.
            </>
          }
        />
        <Metric
          label="Comunas con nivel alto"
          icon="attention"
          attention
          value={<span id="metric-high">{highCount}</span>}
          note={
            <>
              Cuenta solo las comunas cuyo último lote esta página leyó y validó: lo que devolvió{' '}
              <code>/v1/risk/batches</code> al iniciar la sesión, más la zona seleccionada. Si esa lectura agregada falla,
              la cuenta refleja únicamente lo leído por zona. Una comuna sin lote publicado, o cuyo lote no pasó la
              validación, queda sin nivel y no se cuenta.
            </>
          }
        />
      </section>

      <article className="panel model-panel" id="model-panel" aria-labelledby="model-panel-title">
        <PanelHeading
          eyebrow="REGISTRO Y LOTE"
          titleId="model-panel-title"
          title="Versión de modelo activa"
          badge={
            <span className="neutral-tag" id="model-synthetic">
              {modelBadge}
            </span>
          }
        />
        <StatusCopy id="model-status">{modelStatusText}</StatusCopy>
        <dl className="data-list compact">
          <div>
            <dt>Versión activa (registro)</dt>
            <dd id="model-active-version">
              {activeModel !== null && activeModel.available ? activeModel.version : 'No informada'}
            </dd>
          </div>
          <div>
            <dt>Versión del último lote</dt>
            <dd id="model-batch-version">{batchField(batchData?.model_version)}</dd>
          </div>
        </dl>
        <p className="panel-note" id="model-panel-note">
          La marca sintética se reproduce tal como el servicio la declara. Una versión sin marca declarada se muestra
          como no declarada; la página no la supone no sintética. La fila del registro es la versión que el registro
          presenta como activa; la fila del lote es la versión que de verdad lo clasificó, y ambas pueden diferir. Una
          activación o su reversión rige para las publicaciones que resuelven su clasificador después de esa llamada:
          las que ya estaban en curso conservan la instantánea que resolvieron. Antes de una activación explícita, el
          clasificador en uso es el que fija <code>MODEL_ID</code> al arrancar, y que una fila del registro esté
          sembrada como activa no prueba que el servicio la haya usado.
        </p>
      </article>

      <section className="dashboard-grid" aria-label="Estado del riesgo y ubicación">
        <MapPanel
          collection={mapCollection}
          loadState={mapState}
          selectedCode={selectedZone}
          zoneLevels={zoneLevelLookup}
          onSelect={selectZone}
          sessionReason={session.reason}
        />

        <article className="panel risk-panel">
          <PanelHeading
            eyebrow="ESTADO DEL RIESGO"
            title="Último lote publicado"
            badge={
              <span className="risk-pill" id="risk-pill">
                {risk.pill}
              </span>
            }
          />
          <StatusCopy id="risk-status">{risk.status}</StatusCopy>
          <div className="risk-state">
            <span className="risk-symbol" aria-hidden="true">
              —
            </span>
            <div>
              <strong id="risk-level">{risk.level}</strong>
              <small>Clasificación informada por el backend</small>
            </div>
          </div>
          <DataList
            items={[
              { label: 'Código CUT', value: <span id="zone-code">{orUninformed(selectedFeature?.properties.code)}</span> },
              { label: 'Comuna', value: <span id="zone-name">{orUninformed(selectedFeature?.properties.name)}</span> },
              {
                label: 'Provincia',
                value: <span id="zone-province">{orUninformed(selectedFeature?.properties.province)}</span>,
              },
            ]}
          />
          <p id="batch-origin">{originText}</p>
          <p id="batch-notice">{noticeText}</p>
          <StatusCopy id="batch-validity">{validityText}</StatusCopy>
          <h4>Factores</h4>
          <ul id="risk-factors">
            {factorBars.map((bar, index) => (
              <li key={`${bar.factor.name}-${index}`}>
                {bar.factor.known
                  ? `${bar.factor.name}: ${bar.factor.value} · contribución ${bar.factor.contribution} · ${bar.factor.note}`
                  : `${bar.factor.name}: desconocido · ${bar.factor.note}`}
                {bar.widthPercent !== null ? (
                  <div className="factor-track">
                    <span style={{ width: `${bar.widthPercent}%` }} />
                  </div>
                ) : null}
              </li>
            ))}
            {showFactorOmission ? <li>Factores ausentes o inválidos; solo se muestran entradas válidas.</li> : null}
          </ul>
          <h4>Justificación</h4>
          <p id="risk-rationale">{rationaleText}</p>
          <h4>Política informada</h4>
          <p id="risk-policy">{policyText}</p>
          <DataList
            items={[
              { label: 'Estado del dato', value: <span id="data-status">{batchField(batchData?.data_status)}</span> },
              { label: 'Versión', value: <span id="model-version">{batchField(batchData?.model_version)}</span> },
              { label: 'Vigencia hasta', value: <span id="valid-until">{batchField(batchData?.valid_until)}</span> },
              { label: 'Identificador', value: <span id="batch-id">{batchField(batchData?.id)}</span> },
            ]}
          />
          <UnimplementedNote>
            Cobertura por sector: la API no publica un porcentaje de cobertura por comuna, así que esta ficha no lo
            muestra.
          </UnimplementedNote>
        </article>
      </section>
    </section>
  );
}
