// FireWatch served-page script.
//
// No framework and no external dependency. The page is served on its own origin
// (http://localhost:4173) and its Content-Security-Policy allows exactly three
// origins: itself for scripts, styles and images, plus the gateway
// (http://localhost:8080) and the identity provider (http://localhost:8081) for
// connections. The geospatial layer is therefore drawn here, in plain SVG, with
// our own schematic projection.
//
// The pure projection helpers are exported at the bottom ONLY so they can be
// verified offline with Node against a small GeoJSON fixture; a browser never
// defines `module` and never reaches that branch.
(() => {
  "use strict";

  const issuer = "http://localhost:8081/realms/firewatch";
  const clientID = "firewatch-demo";
  // Page origin and API origin are separate processes: the page is served on
  // :4173 while the gateway stays on :8080, so every gateway call is absolute.
  // The identity provider is NOT the gateway: `/token` resolves against
  // `issuer` below and must never receive `gatewayOrigin`.
  const redirectURI = "http://localhost:4173/";
  const gatewayOrigin = "http://localhost:8080";
  const transactionKey = "firewatch.pkce.transaction";
  const SVG_NAMESPACE = "http://www.w3.org/2000/svg";

  let accessToken = "";
  let idToken = "";
  let currentJobID = "";
  let selectedZone = "";
  let zoneCollection = null;
  // Canonical level of the last published batch for the SELECTED zone. Empty
  // means "no level computed for any zone", which is the honest default.
  let activeLevel = "";
  // Canonical level remembered for every zone whose latest batch this session has
  // already read, so the regional map keeps its colours when the selection moves
  // away instead of greying out every comuna but one. It is filled from the reads
  // the page already performs (no extra requests) and cleared on logout.
  let zoneLevels = new Map();
  // Rows the API returned for the SELECTED zone, kept so the search box and the
  // status filter can narrow them without issuing another read.
  let historyBatches = [];
  const zoneShapes = new Map();
  // Monotonic sequence for each zone-scoped request kind. A request captures
  // the value it started with and a response is discarded unless that value is
  // still current. This is what prevents a slow response for zone A from
  // colouring zone B: without it, the awaited response writes the global
  // activeLevel and the next render applies it to whatever zone is selected at
  // that instant, implying an assessment that was never computed for B. The two
  // kinds have SEPARATE counters because loadLatest and loadHistory are issued
  // together for one selection; a shared counter would make each one treat the
  // other's just-started request as a supersession and discard its own answer.
  let latestRequestSequence = 0;
  let historyRequestSequence = 0;
  let jobSequence = 0;
  // Monotonic session fence. `resetPrivateView` bumps it (401, logout or a
  // failed login) and every `connectSession` captures its own value, so a reply
  // that belonged to a superseded or closed session can neither repopulate the
  // map nor chain a fresh latest/history read after the view was cleared.
  let sessionSequence = 0;
  let jobController = null;
  let jobTimer = null;
  let jobDeadline = null;
  let expectedBatch = null;
  let pollAttempts = 0;
  const MAX_POLL_ATTEMPTS = 30;
  // Pipeline status refresh state. `pipelineTimer` holds the single recursive
  // setTimeout handle; `pipelineSequence` and `pipelineController` fence a
  // superseded response so it can neither overwrite newer state nor resurrect a
  // verified panel after logout.
  let pipelineSequence = 0;
  let pipelineTimer = null;
  let pipelineController = null;
  // Bounded deadline for ONE pipeline refresh cycle. It mirrors the job-path
  // deadline (`jobDeadline`): a read that does not answer inside the cota is
  // fenced and aborted, the panel stops asserting verification, and the next
  // cycle is still scheduled so an open page recovers without a reload.
  let pipelineDeadline = null;
  const PIPELINE_REFRESH_MS = 30000;
  const PIPELINE_DEADLINE_MS = 10000;
  // Batch operational validity re-evaluation. This environment defines no
  // setInterval, so a single bounded recursive setTimeout is replaced on every
  // render and cleared on logout, a 401 reset and a zone change.
  let validityTimer = null;
  let validityBatch = null;
  // Clock-skew safety margin for the operational validity label. It is applied
  // in the EXPIRE-EARLY direction only: the client subtracts it from the batch's
  // own `valid_until`, so a skewed client clock can never keep asserting
  // "Vigente" after the server bound. The timer below is therefore always
  // scheduled at or before `valid_until`, never after it.
  const VALIDITY_SKEW_MS = 1000;
  const VALIDITY_MAX_DELAY_MS = 3600000;
  // Client-side staleness bound. The backend publishes its dead-letter freshness
  // threshold as dependencies.dead_letter.max_age_seconds; when that field is
  // absent or non-numeric we fall back to this conservative constant so a status
  // snapshot is never treated as fresh by default. We do not invent a new
  // contract field: the payload bound is reused and the fallback stays local.
  const FALLBACK_STATUS_MAX_AGE_SECONDS = 30;

  function cancelJob() {
    jobSequence++;
    if (jobController) jobController.abort();
    if (jobTimer !== null) clearTimeout(jobTimer);
    stopDeadline();
    jobController = null;
    jobTimer = null;
    currentJobID = "";
    setText("job-id", "—");
    setText("job-requested", "—");
    setText("job-state", "Sin solicitud");
    expectedBatch = null;
    latestRequestSequence++;
    historyRequestSequence++;
    document.getElementById("refresh-job").disabled = true;
  }

  function stopDeadline() {
    if (jobDeadline !== null) clearTimeout(jobDeadline);
    jobDeadline = null;
  }

  function armDeadline(sequence, zone) {
    stopDeadline();
    jobDeadline = setTimeout(() => {
      if (!ownsJob(sequence, zone)) return;
      jobSequence++;
      latestRequestSequence++;
      historyRequestSequence++;
      if (jobController) jobController.abort();
      if (jobTimer !== null) clearTimeout(jobTimer);
      jobController = null;
      jobTimer = null;
      jobDeadline = null;
      document.getElementById("create-job").disabled = false;
      setText("job-status", "Tiempo de consulta automática agotado; publicación o lectura sin verificar. Puedes actualizar manualmente.");
    }, 65000);
  }

  function ownsJob(sequence, zone) {
    return sequence === jobSequence && zone === selectedZone && accessToken !== "";
  }

  function matchesBatch(batch, expected) {
    return batch && batch.zone_id === expected.zone && batch.job_id === expected.job && batch.id === expected.batch;
  }

  // --- Schematic geospatial projection -------------------------------------
  //
  // This is NOT a cartographic projection and NOT a basemap. It is a plain
  // equirectangular (plate carree) mapping with a cosine correction at the
  // mean latitude of the fitted extent, which keeps the communes from looking
  // stretched east-west at Valparaiso's latitude. A tile basemap or a vendored
  // mapping library cannot be used here: the CSP closes both `connect-src` and
  // `img-src`, so no tile provider is reachable and no external asset may be
  // loaded. Rendering the ANALYSED ZONES ourselves is the requirement: the
  // geography of the zones, not a base map.
  //
  // Two communes of the region are oceanic archipelagos far outside the
  // mainland coastline. Drawing them in the mainland frame would compress the
  // 36 continental communes into a sliver, so they are rendered as EXTERIOR
  // INSETS, each with its own local projection and its own scale, and the view
  // states plainly that they are NOT geographically positioned. Membership is
  // derived from the geometry ALONE: a feature whose bounds lie west of the
  // detached-longitude guard is insular, so a newly added island commune is an
  // inset automatically and there is no separate list to keep in sync.
  const DETACHED_LONGITUDE = -75; // mainland Valparaiso spans lon [-71.9, -70.1]
  const MAINLAND_VIEW = { width: 760, height: 420, padding: 14 };
  const INSET_VIEW = { width: 168, height: 132, padding: 10 };

  function geometryBounds(geometry) {
    const bounds = { minLon: Infinity, minLat: Infinity, maxLon: -Infinity, maxLat: -Infinity };
    const visit = (node) => {
      if (!Array.isArray(node)) return;
      if (typeof node[0] === "number" && typeof node[1] === "number") {
        const lon = node[0];
        const lat = node[1];
        if (!Number.isFinite(lon) || !Number.isFinite(lat)) return;
        if (lon < bounds.minLon) bounds.minLon = lon;
        if (lat < bounds.minLat) bounds.minLat = lat;
        if (lon > bounds.maxLon) bounds.maxLon = lon;
        if (lat > bounds.maxLat) bounds.maxLat = lat;
        return;
      }
      for (const child of node) visit(child);
    };
    if (!geometry || !Array.isArray(geometry.coordinates)) return null;
    visit(geometry.coordinates);
    return Number.isFinite(bounds.minLon) ? bounds : null;
  }

  function collectBounds(entries) {
    const union = { minLon: Infinity, minLat: Infinity, maxLon: -Infinity, maxLat: -Infinity };
    for (const entry of entries) {
      const bounds = entry && entry.bounds ? entry.bounds : null;
      if (!bounds) continue;
      if (bounds.minLon < union.minLon) union.minLon = bounds.minLon;
      if (bounds.minLat < union.minLat) union.minLat = bounds.minLat;
      if (bounds.maxLon > union.maxLon) union.maxLon = bounds.maxLon;
      if (bounds.maxLat > union.maxLat) union.maxLat = bounds.maxLat;
    }
    return Number.isFinite(union.minLon) ? union : null;
  }

  // Fits `bounds` into a `size` pixel box with a uniform scale, so the whole
  // extent stays inside [0, size.width] x [0, size.height] even when the
  // extent is a degenerate point.
  function createProjection(bounds, size, padding) {
    const innerWidth = Math.max(size.width - padding * 2, 1);
    const innerHeight = Math.max(size.height - padding * 2, 1);
    const latMean = (bounds.minLat + bounds.maxLat) / 2;
    const cosLat = Math.max(Math.cos((latMean * Math.PI) / 180), 1e-6);
    const spanX = Math.max((bounds.maxLon - bounds.minLon) * cosLat, 1e-9);
    const spanY = Math.max(bounds.maxLat - bounds.minLat, 1e-9);
    const scale = Math.min(innerWidth / spanX, innerHeight / spanY);
    const offsetX = padding + (innerWidth - spanX * scale) / 2;
    const offsetY = padding + (innerHeight - spanY * scale) / 2;
    return {
      latMean: latMean,
      cosLat: cosLat,
      scale: scale,
      project(lon, lat) {
        return {
          x: offsetX + (lon - bounds.minLon) * cosLat * scale,
          y: offsetY + (bounds.maxLat - lat) * scale,
        };
      },
    };
  }

  function pathData(geometry, projection) {
    if (!geometry || !Array.isArray(geometry.coordinates)) return "";
    const polygons = geometry.type === "Polygon" ? [geometry.coordinates] : geometry.coordinates;
    const parts = [];
    for (const polygon of polygons) {
      if (!Array.isArray(polygon)) continue;
      for (const ring of polygon) {
        if (!Array.isArray(ring) || ring.length === 0) continue;
        let command = "M";
        for (const point of ring) {
          if (!Array.isArray(point)) continue;
          const projected = projection.project(point[0], point[1]);
          parts.push(`${command}${projected.x.toFixed(2)} ${projected.y.toFixed(2)}`);
          command = "L";
        }
        parts.push("Z");
      }
    }
    return parts.join(" ");
  }

  // Splits the catalogue into mainland features and detached insular features.
  // Geometry-less features are dropped: the API already filters them out, and a
  // missing geometry is a declared state, not a shape to invent.
  function partitionZones(features) {
    const mainland = [];
    const insets = [];
    for (const feature of features) {
      if (!feature || !feature.geometry) continue;
      const bounds = geometryBounds(feature.geometry);
      if (!bounds) continue;
      // The longitude rule is the SINGLE criterion: no zone list is consulted.
      const detached = bounds.minLon < DETACHED_LONGITUDE;
      (detached ? insets : mainland).push({ feature: feature, bounds: bounds });
    }
    return { mainland: mainland, insets: insets };
  }

  function countVertices(geometry) {
    let total = 0;
    const visit = (node) => {
      if (!Array.isArray(node)) return;
      if (typeof node[0] === "number" && typeof node[1] === "number") {
        total += 1;
        return;
      }
      for (const child of node) visit(child);
    };
    if (geometry && Array.isArray(geometry.coordinates)) visit(geometry.coordinates);
    return total;
  }

  // --- Map DOM -------------------------------------------------------------

  function zoneLabel(properties) {
    const name = properties && properties.name ? String(properties.name) : "Zona sin nombre";
    const code = properties && properties.code ? String(properties.code) : "sin código";
    return `${name} (${code})`;
  }

  function showMapEmpty(message) {
    zoneShapes.clear();
    const svg = document.getElementById("map-svg");
    const empty = document.getElementById("map-empty");
    const insets = document.getElementById("map-insets");
    if (svg) svg.setAttribute("hidden", "");
    if (insets) insets.replaceChildren();
    if (empty) {
      empty.hidden = false;
      empty.textContent = message;
    }
  }

  function appendZoneShape(group, entry, projection) {
    const feature = entry.feature;
    const code = feature.properties && typeof feature.properties.code === "string" ? feature.properties.code : "";
    const shape = document.createElementNS(SVG_NAMESPACE, "path");
    shape.setAttribute("d", pathData(feature.geometry, projection));
    shape.setAttribute("class", "zone-shape zone-neutral");
    shape.setAttribute("data-zone", code);
    shape.setAttribute("role", "button");
    shape.setAttribute("tabindex", "0");
    shape.setAttribute("aria-label", zoneLabel(feature.properties));
    shape.addEventListener("click", () => selectZone(code));
    shape.addEventListener("keydown", (event) => {
      if (event.key !== "Enter" && event.key !== " ") return;
      event.preventDefault();
      selectZone(code);
    });
    const title = document.createElementNS(SVG_NAMESPACE, "title");
    title.textContent = zoneLabel(feature.properties);
    shape.append(title);
    group.append(shape);
    if (code !== "") zoneShapes.set(code, shape);
    return shape;
  }

  function renderZoneLayer() {
    const features = zoneCollection && Array.isArray(zoneCollection.features) ? zoneCollection.features : [];
    const split = partitionZones(features);
    if (split.mainland.length === 0 && split.insets.length === 0) {
      showMapEmpty("El catálogo de zonas no informa geometrías: no hay nada que dibujar.");
      return;
    }
    const svg = document.getElementById("map-svg");
    const empty = document.getElementById("map-empty");
    const insetHost = document.getElementById("map-insets");
    zoneShapes.clear();
    svg.replaceChildren();
    insetHost.replaceChildren();

    if (split.mainland.length > 0) {
      const bounds = collectBounds(split.mainland);
      const projection = createProjection(bounds, MAINLAND_VIEW, MAINLAND_VIEW.padding);
      const group = document.createElementNS(SVG_NAMESPACE, "g");
      group.setAttribute("class", "zone-layer");
      for (const entry of split.mainland) appendZoneShape(group, entry, projection);
      svg.append(group);
      svg.setAttribute("viewBox", `0 0 ${MAINLAND_VIEW.width} ${MAINLAND_VIEW.height}`);
      svg.setAttribute("preserveAspectRatio", "xMidYMid meet");
      svg.removeAttribute("hidden");
      empty.hidden = true;
    } else {
      svg.setAttribute("hidden", "");
      empty.hidden = false;
      empty.textContent = "El catálogo no informa geometrías continentales; se muestran solo los territorios insulares.";
    }

    for (const entry of split.insets) {
      const figure = document.createElement("figure");
      figure.setAttribute("class", "map-inset");
      const insetSvg = document.createElementNS(SVG_NAMESPACE, "svg");
      insetSvg.setAttribute("viewBox", `0 0 ${INSET_VIEW.width} ${INSET_VIEW.height}`);
      insetSvg.setAttribute("preserveAspectRatio", "xMidYMid meet");
      insetSvg.setAttribute("role", "group");
      insetSvg.setAttribute("aria-label", zoneLabel(entry.feature.properties));
      const projection = createProjection(entry.bounds, INSET_VIEW, INSET_VIEW.padding);
      const group = document.createElementNS(SVG_NAMESPACE, "g");
      appendZoneShape(group, entry, projection);
      insetSvg.append(group);
      const caption = document.createElement("figcaption");
      caption.textContent = zoneLabel(entry.feature.properties);
      figure.append(insetSvg, caption);
      insetHost.append(figure);
    }

    applyZoneLevels();
  }

  // Colours a polygon by its canonical level ONLY when a level exists for that
  // zone (the selected zone and its latest published batch). Every other
  // polygon is neutral. The catalogue has no per-commune classification, so the
  // view must never imply an assessment that the API did not compute.
  function applyZoneLevels() {
    for (const entry of zoneShapes) {
      const code = entry[0];
      const shape = entry[1];
      shape.classList.remove("zone-neutral", "zone-selected", "level-bajo", "level-medio", "level-alto", "level-desconocido");
      const selected = code === selectedZone;
      // The selection's own level comes from the panel's freshly read batch;
      // every other zone keeps the level the regional fan-out found, so a
      // published batch does not lose its colour when the selection moves.
      const level = selected ? activeLevel : zoneLevels.get(code) || "";
      if (level !== "") shape.classList.add(`level-${level}`);
      else shape.classList.add("zone-neutral");
      shape.setAttribute("aria-pressed", String(selected));
      if (selected) shape.classList.add("zone-selected");
    }
    // Summary card. No endpoint aggregates levels, so this counts only the zones
    // whose latest batch this session actually read; the card states that limit.
    let highZones = 0;
    for (const level of zoneLevels.values()) if (level === "alto") highZones += 1;
    setText("metric-high", highZones);
  }

  function canonicalLevel(level) {
    return level === "bajo" || level === "medio" || level === "alto" || level === "desconocido" ? level : "";
  }

  function currentZoneFeature() {
    if (!zoneCollection || !Array.isArray(zoneCollection.features) || selectedZone === "") return null;
    return zoneCollection.features.find(
      (feature) => feature && feature.properties && feature.properties.code === selectedZone,
    ) || null;
  }

  function updateZoneSummary() {
    setText("zone-chip-name", selectedZone === "" ? "Sin zona" : selectedZone);
    const feature = currentZoneFeature();
    const properties = feature ? feature.properties : {};
    for (const key of ["code", "name", "province"]) setText(`zone-${key}`, properties[key]);
    if (!feature) {
      setText("map-location", "Sin zona seleccionada");
      setText("map-coordinates", "La capa territorial se carga desde /v1/risk/zones al conectar.");
      return;
    }
    setText("map-location", zoneLabel(feature.properties));
    // Same longitude rule as the map partition: insular is a property of the
    // geometry, not of a hardcoded code list.
    const bounds = geometryBounds(feature.geometry);
    const insular = bounds && bounds.minLon < DETACHED_LONGITUDE ? " · incluye territorios insulares" : "";
    setText("map-coordinates", `${countVertices(feature.geometry)} vértices${insular}.`);
  }

  function populateZoneSelector(features) {
    const select = document.getElementById("zone-select");
    if (!select) return "";
    const previous = select.value;
    const options = [];
    for (const feature of features) {
      if (!feature || !feature.properties) continue;
      if (typeof feature.properties.code !== "string" || feature.properties.code === "") continue;
      options.push({
        code: feature.properties.code,
        name: typeof feature.properties.name === "string" ? feature.properties.name : feature.properties.code,
      });
    }
    options.sort((left, right) => left.code.localeCompare(right.code));
    select.replaceChildren();
    if (options.length === 0) {
      const option = document.createElement("option");
      option.value = "";
      option.textContent = "Catálogo sin zonas";
      select.append(option);
      select.disabled = true;
      return "";
    }
    for (const zone of options) {
      const option = document.createElement("option");
      option.value = zone.code;
      option.textContent = `${zone.name} (${zone.code})`;
      select.append(option);
    }
    const chosen = options.some((zone) => zone.code === previous) ? previous : options[0].code;
    select.value = chosen;
    select.disabled = false;
    return chosen;
  }

  // --- Synthetic scenario picker -------------------------------------------
  //
  // The catalogue is served by the authenticated risk API and its IDs are the
  // exact built-in profiles. The page never invents a profile name and never
  // sends an empty value: the server rejects an explicit null/empty scenario_id
  // with 400 invalid_demo_request, while an OMITTED field keeps the legacy
  // fixture. So `selectedScenario()` returns "" and the caller omits the field.
  function selectedScenario() {
    const select = document.getElementById("scenario-select");
    return select ? select.value : "";
  }

  function populateScenarioSelector(items) {
    const select = document.getElementById("scenario-select");
    if (!select) return "";
    const previous = select.value;
    const options = [];
    for (const item of items) {
      if (!item || typeof item.id !== "string" || item.id === "") continue;
      options.push({
        id: item.id,
        description: typeof item.description === "string" ? item.description : "",
      });
    }
    select.replaceChildren();
    const legacy = document.createElement("option");
    legacy.value = "";
    legacy.textContent = "Sin escenario (fixture analítico heredado)";
    select.append(legacy);
    for (const scenario of options) {
      const option = document.createElement("option");
      option.value = scenario.id;
      option.textContent = scenario.description === "" ? scenario.id : `${scenario.id} — ${scenario.description}`;
      select.append(option);
    }
    const chosen = options.some((scenario) => scenario.id === previous) ? previous : "";
    select.value = chosen;
    select.disabled = false;
    return chosen;
  }

  async function loadScenarios(isCurrent = () => true) {
    try {
      const catalogue = await apiRequest(`${gatewayOrigin}/v1/risk/synthetic-scenarios`, {}, isCurrent);
      if (!isCurrent()) return;
      if (!Array.isArray(catalogue)) {
        const select = document.getElementById("scenario-select");
        if (select) { select.value = ""; select.disabled = true; }
        setText("scenario-note", "La API no devolvió un catálogo de escenarios válido; el trabajo usaría el fixture analítico heredado.");
        return;
      }
      populateScenarioSelector(catalogue);
      setText("scenario-note", "El escenario se elige en esta página. El servidor asigna su instante de evaluación en UTC, una sola vez por trabajo; son perfiles de prueba, no condiciones observadas.");
    } catch (error) {
      if (!isCurrent()) return;
      if (error.message === "unauthorized") return;
      const select = document.getElementById("scenario-select");
      if (select) { select.value = ""; select.disabled = true; }
      setText("scenario-note", error.message === "forbidden"
        ? "La API denegó el permiso para listar escenarios (403); el trabajo usaría el fixture analítico heredado."
        : "No fue posible cargar el catálogo de escenarios; el trabajo usaría el fixture analítico heredado.");
    }
  }

  async function loadZones(isCurrent = () => true) {
    showMapEmpty("Consultando el catálogo de zonas analizadas…");
    try {
      const collection = await apiRequest(`${gatewayOrigin}/v1/risk/zones`, {}, isCurrent);
      // A superseded or post-reset response owns nothing: it must not write the
      // count, the selector, the map or the risk status after the view cleared.
      if (!isCurrent()) return "";
      if (!collection || collection.type !== "FeatureCollection" || !Array.isArray(collection.features)) {
        zoneCollection = null;
        setText("zone-count", "No informado");
        showMapEmpty("La API no devolvió una colección de zonas válida.");
        return "";
      }
      zoneCollection = collection;
      // The count is read from the live catalogue features, never from a
      // constant or a database row count: a hardcoded 39 would drift from the
      // served geometry the moment the source changes.
      setText("zone-count", collection.features.length);
      selectedZone = populateZoneSelector(collection.features);
      renderZoneLayer();
      updateZoneSummary();
      return selectedZone;
    } catch (error) {
      if (!isCurrent()) return "";
      if (error.message === "unauthorized") return "";
      zoneCollection = null;
      selectedZone = "";
      setText("zone-count", "No informado");
      showMapEmpty(error.message === "forbidden"
        ? "La API denegó el permiso para consultar las zonas analizadas (403)."
        : "No fue posible cargar el catálogo de zonas analizadas.");
      return "";
    }
  }

  // --- Session and API -----------------------------------------------------

  function setText(id, value) {
    document.getElementById(id).textContent = value == null || value === "" ? "No informado" : String(value);
  }

  function resetPrivateView(message) {
    sessionSequence++;
    cancelJob();
    cancelPipelineRefresh();
    clearBatchDetails();
    accessToken = "";
    idToken = "";
    currentJobID = "";
    selectedZone = "";
    zoneCollection = null;
    activeLevel = "";
    zoneLevels = new Map();
    // Repaint before the map is hidden: no colour survives a closed session.
    applyZoneLevels();
    document.getElementById("session-state").textContent = "Sin conexión";
    document.getElementById("session-detail").textContent = "Token requerido para consultar";
    document.getElementById("logout").hidden = true;
    renderSessionRole();
    document.getElementById("create-job").disabled = true;
    document.getElementById("refresh-job").disabled = true;
    setText("risk-level", "Sin datos");
    setText("risk-pill", "SIN CONSULTAR");
    setText("data-status", "Sin consultar");
    setText("model-version", "Sin consultar");
    setText("valid-until", "Sin consultar");
    setText("batch-id", "Sin consultar");
    setText("zone-count", "No informado");
    setText("batch-validity", "Sin consultar");
    setText("sources-badge", "SIN CONSULTAR");
    setText("sources-status", "Conecta un token autorizado para consultar la procedencia.");
    for (const id of SOURCES_FIELDS) setText(id, "No disponible");
    document.getElementById("sources-status").classList.remove("risk-good");
    setText("pipeline-badge", "SIN VERIFICAR");
    setText("pipeline-status", "Conecta un token autorizado para comprobar los servicios.");
    setText("pipeline-risk", "Sin verificar");
    setText("pipeline-jobs", "Sin verificar");
    clearAuditPanel("SIN CONSULTAR", "Conecta un token autorizado para consultar el registro de actividad.");
    document.getElementById("audit-refresh").disabled = true;
    document.getElementById("pipeline-status").classList.remove("risk-good", "risk-error");
    document.getElementById("pipeline-risk").classList.remove("risk-good");
    document.getElementById("pipeline-jobs").classList.remove("risk-good");
    const select = document.getElementById("zone-select");
    if (select) {
      select.replaceChildren();
      const option = document.createElement("option");
      option.value = "";
      option.textContent = "Selecciona una zona analizada";
      select.append(option);
      select.value = "";
      select.disabled = true;
    }
    const scenarioSelect = document.getElementById("scenario-select");
    if (scenarioSelect) {
      scenarioSelect.replaceChildren();
      const legacyOption = document.createElement("option");
      legacyOption.value = "";
      legacyOption.textContent = "Sin escenario (fixture analítico heredado)";
      scenarioSelect.append(legacyOption);
      scenarioSelect.value = "";
      scenarioSelect.disabled = true;
    }
    setText("scenario-note", "El escenario se elige en esta página. El servidor asigna su instante de evaluación en UTC, una sola vez por trabajo; son perfiles de prueba, no condiciones observadas.");
    updateZoneSummary();
    showMapEmpty("Conecta un token autorizado para cargar las zonas analizadas.");
    setText("history-count", "\u2014");
    setText("metric-batches", "\u2014");
    document.getElementById("history-rows").replaceChildren();
    document.getElementById("history-state").textContent = "Conecta un token para consultar el historial.";
    document.querySelector(".history-panel").classList.remove("has-rows");
    resetHistoryFilters();
    setText("job-status", "Disponible al conectar un token. El servidor determina si tienes permiso.");
    setText("job-state", "Sin solicitud");
    setText("job-id", "\u2014");
    setText("job-requested", "\u2014");
    setText("risk-status", message || "Conecta un token autorizado para consultar la API.");
    document.getElementById("auth-status").textContent = message || "";
  }

  function randomBase64URL(byteLength) {
    const bytes = crypto.getRandomValues(new Uint8Array(byteLength));
    let binary = "";
    for (const byte of bytes) binary += String.fromCharCode(byte);
    return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
  }

  async function startLogin() {
    try {
      const state = randomBase64URL(32);
      const verifier = randomBase64URL(32);
      const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(verifier));
      let binary = "";
      for (const byte of new Uint8Array(digest)) binary += String.fromCharCode(byte);
      const challenge = btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
      sessionStorage.setItem(transactionKey, JSON.stringify({ state, verifier }));
      const authorization = new URL(`${issuer}/protocol/openid-connect/auth`);
      authorization.search = new URLSearchParams({
        client_id: clientID,
        redirect_uri: redirectURI,
        response_type: "code",
        scope: "openid",
        state,
        code_challenge: challenge,
        code_challenge_method: "S256",
      });
      window.location.assign(authorization.href);
    } catch {
      document.getElementById("auth-status").textContent = "No fue posible iniciar el acceso seguro a Keycloak.";
    }
  }

  async function handleLoginCallback() {
    const callback = new URLSearchParams(window.location.search);
    const code = callback.get("code");
    const callbackState = callback.get("state");
    const oauthError = callback.get("error");
    if (!code && !callbackState && !oauthError) return;

    try {
      const storedTransaction = sessionStorage.getItem(transactionKey);
      sessionStorage.removeItem(transactionKey);
      const transaction = JSON.parse(storedTransaction || "null");
      if (oauthError) throw new Error("El inicio de sesión fue cancelado o rechazado por Keycloak.");
      if (!code || !callbackState || !transaction || callbackState !== transaction.state) {
        throw new Error("La respuesta de inicio de sesión no superó la validación de estado.");
      }
      const response = await fetch(`${issuer}/protocol/openid-connect/token`, {
        method: "POST",
        cache: "no-store",
        credentials: "omit",
        headers: { "Content-Type": "application/x-www-form-urlencoded" },
        body: new URLSearchParams({
          grant_type: "authorization_code",
          client_id: clientID,
          code,
          redirect_uri: redirectURI,
          code_verifier: transaction.verifier,
        }),
      });
      if (!response.ok) throw new Error("Keycloak no pudo completar el intercambio del código.");
      const tokenResponse = await response.json();
      if (typeof tokenResponse.access_token !== "string" || tokenResponse.access_token === "") {
        throw new Error("Keycloak no devolvió un token de acceso válido.");
      }
      connectSession(tokenResponse.access_token, tokenResponse.id_token);
    } catch (error) {
      resetPrivateView(error.message || "No fue posible completar el inicio de sesión.");
    } finally {
      window.history.replaceState(null, "", `${window.location.pathname}${window.location.hash}`);
    }
  }

  function connectSession(token, returnedIDToken) {
    accessToken = token;
    idToken = typeof returnedIDToken === "string" ? returnedIDToken : "";
    document.getElementById("session-state").textContent = "Sesión conectada";
    document.getElementById("session-detail").textContent = "Permisos evaluados por el servidor";
    document.getElementById("logout").hidden = false;
    document.getElementById("create-job").disabled = false;
    document.getElementById("auth-status").textContent = "Consultando endpoints protegidos…";
    renderSessionRole();
    refreshPipeline();
    const sequence = ++sessionSequence;
    const isCurrent = () => sequence === sessionSequence && accessToken !== "";
    loadZones(isCurrent)
      .then(() => (isCurrent() ? Promise.all([loadLatest(), loadHistory()]) : null))
      .then(() => {
        if (isCurrent()) document.getElementById("auth-status").textContent = "Sesión lista. Las acciones autorizadas se validan en el servidor.";
      });
    // Deliberately independent of the zone chain: a catalogue failure must never
    // block zones, latest or history, and the selector then stays at its
    // legacy-only disabled state instead of inventing a profile.
    loadScenarios(isCurrent);
    // Also independent: a denied register (403 for a non-admin) must never block
    // zones, latest, history or the scenario catalogue.
    document.getElementById("audit-refresh").disabled = false;
    loadAudit(isCurrent);
  }

  async function apiRequest(path, options = {}, isCurrent = () => true) {
    const response = await fetch(path, {
      ...options,
      cache: "no-store",
      credentials: "omit",
      headers: { ...(options.headers || {}), Authorization: `Bearer ${accessToken}` },
    });
    if (!isCurrent()) throw new Error("superseded");
    if (response.status === 401) {
      resetPrivateView("La API rechazó el token (401). La sesión se cerró; conecta nuevamente.");
      throw new Error("unauthorized");
    }
    if (response.status === 403) throw new Error("forbidden");
    if (!response.ok) throw new Error(`http_${response.status}`);
    return response.status === 204 ? null : response.json();
  }

  // Sources and status endpoints answer 503 with a meaningful JSON body, which
  // apiRequest discards. This dedicated reader keeps the existing apiRequest
  // behavior intact (and its tests untouched) while parsing the body on non-2xx
  // so the panel shows the reason code instead of a bare HTTP number.
  async function apiRequestStatus(path, options = {}, isCurrent = () => true) {
    const response = await fetch(path, {
      ...options,
      cache: "no-store",
      credentials: "omit",
      headers: { ...(options.headers || {}), Authorization: `Bearer ${accessToken}` },
    });
    if (!isCurrent()) throw new Error("superseded");
    if (response.status === 401) {
      resetPrivateView("La API rechazó el token (401). La sesión se cerró; conecta nuevamente.");
      throw new Error("unauthorized");
    }
    let body = null;
    try { body = await response.json(); } catch { body = null; }
    return { status: response.status, ok: response.ok, body: body };
  }

  function showRequestError(target, error, action) {
    if (error.message === "unauthorized") return;
    target.textContent = error.message === "forbidden"
      ? "La API denegó el permiso para esta acción (403). Solicita acceso a un administrador."
      : `No se pudo ${action} (HTTP ${error.message.replace("http_", "") || "sin conexión"}).`;
    target.classList.add("risk-error");
  }

  // --- Sources panel -------------------------------------------------------
  //
  // Everything here fails closed: a 503, a malformed payload or an integrity
  // hash that does not match is rendered as explicitly unavailable and never as
  // a healthy-looking source. The restriction text is reproduced verbatim; no
  // DIFROL approval or extra licence is claimed beyond the source's own words.
  const SOURCES_FIELDS = [
    "sources-provider", "sources-attribution", "sources-restriction", "sources-record-count",
    "sources-derived-sha256", "sources-integrity", "sources-cutoff", "sources-input",
  ];

  function clearSourcesPanel(reason) {
    setText("sources-badge", "NO DISPONIBLE");
    setText("sources-status", reason);
    for (const id of SOURCES_FIELDS) setText(id, "No disponible");
    document.getElementById("sources-status").classList.remove("risk-good");
  }

  function renderSources(result) {
    const geography = result.body && record(result.body.geography) ? result.body.geography : null;
    const provenance = geography && record(geography.provenance) ? geography.provenance : null;
    const integrity = geography && record(geography.integrity) ? geography.integrity : null;
    const usable = result.status === 200 && provenance && integrity && integrity.matches === true &&
      nonempty(provenance.provider) && nonempty(provenance.attribution) && nonempty(provenance.restriction) &&
      nonempty(provenance.derived_sha256) && Number.isFinite(provenance.record_count) && nonempty(geography.cutoff_semantics);
    if (!usable) {
      const code = result.body && nonempty(result.body.code) ? ` \u00b7 ${result.body.code}` : "";
      clearSourcesPanel(`Procedencia geográfica no disponible (HTTP ${result.status}${code}); la integridad no se acredita y no se muestra como fuente saludable.`);
      return;
    }
    setText("sources-badge", "PUBLICADA");
    setText("sources-provider", provenance.provider);
    setText("sources-attribution", provenance.attribution);
    setText("sources-restriction", provenance.restriction);
    setText("sources-record-count", provenance.record_count);
    setText("sources-derived-sha256", provenance.derived_sha256);
    setText("sources-integrity", `coincide (${integrity.algorithm || "hash"} verificado por el servicio)`);
    const cutoff = geography.source_cutoff_at === null || geography.source_cutoff_at === undefined
      ? "sin fecha de corte real; retrieved_at como sustituto"
      : geography.source_cutoff_at;
    setText("sources-cutoff", `${cutoff} \u00b7 ${geography.cutoff_semantics}`);
    const descriptor = record(result.body.risk_input) ? result.body.risk_input : null;
    setText("sources-input", descriptor && descriptor.synthetic === true
      ? `Entrada de riesgo sintética (${descriptor.display_name || descriptor.id || "sin identificar"}); no es riesgo real observado.`
      : "Entrada de riesgo no declarada como sintética por el servicio.");
    setText("sources-status", "Procedencia geográfica publicada e integridad del hash verificada por el servicio. La restricción de la fuente se reproduce literal; no se acredita aprobación de DIFROL ni licencia adicional.");
    document.getElementById("sources-status").classList.add("risk-good");
  }

  async function loadSources(isCurrent, signal) {
    let result;
    try {
      result = await apiRequestStatus(`${gatewayOrigin}/v1/risk/sources`, signal ? { signal } : {}, isCurrent);
    } catch {
      if (!isCurrent()) return;
      clearSourcesPanel("No fue posible consultar la procedencia; se muestra como no disponible.");
      return;
    }
    if (!isCurrent()) return;
    renderSources(result);
  }

  // --- Activity register ---------------------------------------------------
  //
  // `/v1/audit` is admin-only, accepts NO query parameter and the service bounds
  // the answer itself (the newest rows only), so the page never sends a limit or
  // an offset and never pages. Every entry is rendered with the five fields the
  // endpoint carries —`actor_id`, `action`, `job_id`, `trace_id`, `occurred_at`—
  // and the retired mockup's columns that no backend provides (an actor display
  // name, a free-text detail and its own event id) are marked as not implemented
  // in the panel instead of being simulated.
  const AUDIT_FIELDS = ["actor_id", "action", "job_id", "trace_id", "occurred_at"];

  function clearAuditPanel(badge, reason) {
    setText("audit-badge", badge);
    setText("audit-status", reason);
    document.getElementById("audit-status").classList.remove("risk-good");
    setText("audit-count", "\u2014");
    document.getElementById("audit-rows").replaceChildren();
    document.getElementById("audit-table").setAttribute("hidden", "");
  }

  function renderAudit(result) {
    const body = record(result.body) ? result.body : null;
    const code = body && nonempty(body.code) ? ` \u00b7 ${body.code}` : "";
    // Admin-only endpoint: a profile without the permission is a stated outcome,
    // not a hidden panel, so the 403 body itself names the reason and the request.
    if (result.status === 403) {
      const request = body && nonempty(body.request_id) ? ` \u00b7 request_id ${body.request_id}` : "";
      clearAuditPanel("SIN PERMISO", `La API denegó el permiso para consultar el registro de actividad (HTTP 403${code}${request}). Es un endpoint de solo administradores: solicita acceso a un administrador; la página no oculta ni simula los eventos.`);
      return;
    }
    // An event is an entry that states WHAT happened and WHEN. Without both the
    // payload is not a usable register and nothing is rendered from it: a partial
    // list would be an invented view of the activity.
    const events = result.status === 200 && Array.isArray(result.body) ? result.body : null;
    const usable = events !== null &&
      events.every((event) => record(event) && nonempty(event.action) && nonempty(event.occurred_at));
    if (!usable) {
      clearAuditPanel("NO DISPONIBLE", `La API no devolvió un registro de actividad utilizable (HTTP ${result.status}${code}); no se muestran eventos inventados ni parciales.`);
      return;
    }
    const rows = document.getElementById("audit-rows");
    rows.replaceChildren();
    for (const event of events) {
      const row = document.createElement("tr");
      for (const field of AUDIT_FIELDS) {
        const cell = document.createElement("td");
        // An identifier the service omits is labelled in place: nothing is filled
        // in from another field and no name is ever derived from an id.
        cell.textContent = nonempty(event[field]) ? event[field] : "No informado";
        row.append(cell);
      }
      rows.append(row);
    }
    setText("audit-count", events.length);
    setText("audit-badge", "LEÍDO");
    if (events.length === 0) {
      setText("audit-status", "El servicio devolvió un registro de actividad sin eventos.");
      return;
    }
    document.getElementById("audit-table").removeAttribute("hidden");
    setText("audit-status", "Registro real devuelto por /v1/audit. Es un endpoint de solo administradores y no acepta parámetros: el servicio acota la respuesta por sí mismo.");
    document.getElementById("audit-status").classList.add("risk-good");
  }

  async function loadAudit(isCurrent = () => accessToken !== "") {
    if (accessToken === "") return;
    const button = document.getElementById("audit-refresh");
    // This single global read needs one concurrency guard: the control itself.
    // One refresh in flight at a time, so an older answer can never land last.
    button.disabled = true;
    setText("audit-badge", "CONSULTANDO");
    setText("audit-status", "Consultando el registro de actividad en /v1/audit…");
    document.getElementById("audit-status").classList.remove("risk-good");
    let result;
    try {
      result = await apiRequestStatus(`${gatewayOrigin}/v1/audit`, {}, isCurrent);
    } catch {
      if (!isCurrent()) return;
      clearAuditPanel("NO DISPONIBLE", "No fue posible consultar el registro de actividad; se muestra como no disponible.");
      button.disabled = false;
      return;
    }
    if (!isCurrent()) return;
    button.disabled = false;
    renderAudit(result);
  }

  // --- Pipeline status panel -----------------------------------------------
  //
  // A service is verified ONLY when its own status endpoint reports ready with a
  // fresh checked_at (and a dead-letter depth inside the payload threshold). The
  // gateway 204 card is deliberately excluded: a superficial readiness 204 is
  // never pipeline proof. A 403 means "not allowed to check", not "down".
  function renderServiceStatus(targetId, entry) {
    const element = document.getElementById(targetId);
    element.classList.remove("risk-good");
    if (entry.status === 401 || entry.status === 403) {
      setText(targetId, "No autorizado para comprobar; el servicio queda sin verificar, sin marcarlo como no disponible.");
      return { verified: false, unknown: false, unverified: true };
    }
    const body = record(entry.body) ? entry.body : null;
    const dependencies = body && record(body.dependencies) ? body.dependencies : null;
    const deadLetter = dependencies && record(dependencies.dead_letter) ? dependencies.dead_letter : null;
    const maxAge = deadLetter && Number.isFinite(deadLetter.max_age_seconds)
      ? deadLetter.max_age_seconds
      : FALLBACK_STATUS_MAX_AGE_SECONDS;
    const checkedAt = body ? Date.parse(body.checked_at) : NaN;
    const fresh = Number.isFinite(checkedAt) && (Date.now() - checkedAt) / 1000 <= maxAge;
    const deadAge = deadLetter && Number.isFinite(deadLetter.age_seconds) ? deadLetter.age_seconds : null;
    const deadMaxAge = deadLetter && Number.isFinite(deadLetter.max_age_seconds) ? deadLetter.max_age_seconds : null;
    const deadStale = deadAge !== null && deadMaxAge !== null && deadAge > deadMaxAge;
    // Depth and dependency evidence are a precondition of the "dentro de la
    // cota" claim. The backend's own contract sets the dead-letter state to
    // ready only after sampling a REAL depth: `DeadLetterDepthUnknown = -1`
    // (broker.go) is deliberately outside the range of a real depth (>= 0), and
    // statusEvaluator (jobs and risk main.go) reports depth/max_depth as
    // non-negative integers and always serializes dependencies.database,
    // dependencies.broker and dependencies.dead_letter. So the claim needs
    // dependencies present, a database entry that is an object stating ready, a
    // non-negative known depth inside a non-negative known bound, and
    // dependency states that do not contradict `ready: true`. Without this gate
    // `deadStale` is false whenever `dependencies` is missing, so
    // `dependencies: {}` would claim a queue depth it never supplied, a payload
    // without `database` would claim a health it never stated, and `depth: -1`
    // would pass as if it were a real sample inside the bound.
    const deadDepth = deadLetter && Number.isFinite(deadLetter.depth) && deadLetter.depth >= 0 ? deadLetter.depth : null;
    const deadMaxDepth = deadLetter && Number.isFinite(deadLetter.max_depth) && deadLetter.max_depth >= 0 ? deadLetter.max_depth : null;
    const depthWithinBound = deadDepth !== null && deadMaxDepth !== null && deadDepth <= deadMaxDepth;
    // A stated dependency that contradicts `ready: true` is not evidence of
    // health. The broker and the dead-letter queue must state "ready" because
    // the depth claim rests on them, and the database entry must be present as
    // an object stating "ready" too: the backend always serializes it
    // (handler.Dependencies in the jobs and risk main.go) as ready or degraded,
    // so an absent, null, empty or non-object entry is missing evidence and
    // never implicit agreement with `ready: true`.
    const stateOf = (dependency) => dependency && record(dependency) && nonempty(dependency.state) ? dependency.state : null;
    const brokerReady = stateOf(dependencies && dependencies.broker) === "ready";
    const deadLetterReady = stateOf(deadLetter) === "ready";
    const databaseReady = dependencies !== null && record(dependencies.database) && stateOf(dependencies.database) === "ready";
    const dependenciesConsistent = brokerReady && deadLetterReady && databaseReady;
    if (entry.status === 200 && body && body.ready === true && body.reason === "" && fresh && !deadStale && depthWithinBound && dependenciesConsistent) {
      setText(targetId, "Verificado: ready con checked_at fresco y profundidad de cola dentro de la cota.");
      element.classList.add("risk-good");
      return { verified: true, unknown: false, unverified: false };
    }
    const reason = body && nonempty(body.reason) ? ` \u00b7 ${body.reason}` : "";
    const stale = fresh ? "" : " \u00b7 instantánea fuera de la cota de antigüedad del cliente";
    const gaps = [];
    if (!depthWithinBound) gaps.push(" \u00b7 profundidad de cola no acreditada como conocida y dentro de la cota");
    if (!dependenciesConsistent) gaps.push(" \u00b7 estados de dependencia ausentes o contradictorios con \"ready\"");
    setText(targetId, `Desconocido: el estado no se acredita como saludable (HTTP ${entry.status}${reason}${stale}${gaps.join("")}).`);
    return { verified: false, unknown: true, unverified: false };
  }

  function renderPipeline(riskResult, jobsResult) {
    const both = riskResult.verified && jobsResult.verified;
    const status = document.getElementById("pipeline-status");
    status.classList.toggle("risk-good", both);
    status.classList.toggle("risk-error", !both);
    setText("pipeline-badge", both ? "VERIFICADO" : "SIN VERIFICAR");
    if (both) {
      setText("pipeline-status", "Ambos servicios reportan ready con marca de tiempo fresca. La disponibilidad del gateway (204) no participa de esta verificación.");
      return;
    }
    const label = (name, result) => result.verified
      ? `${name} verificado`
      : result.unverified ? `${name} no autorizado para comprobar` : `${name} en estado desconocido`;
    setText("pipeline-status", `${label("servicio de riesgo", riskResult)}; ${label("servicio de trabajos", jobsResult)}. La tarjeta "Gateway" no acredita este estado.`);
  }

  async function fetchServiceStatus(path, isCurrent, signal) {
    try {
      return await apiRequestStatus(path, signal ? { signal } : {}, isCurrent);
    } catch {
      return { status: 0, ok: false, body: null };
    }
  }

  async function loadPipelineStatus(isCurrent, signal) {
    const [risk, jobs] = await Promise.all([
      fetchServiceStatus(`${gatewayOrigin}/v1/risk/status`, isCurrent, signal),
      fetchServiceStatus(`${gatewayOrigin}/v1/jobs/status`, isCurrent, signal),
    ]);
    if (!isCurrent()) return;
    renderPipeline(renderServiceStatus("pipeline-risk", risk), renderServiceStatus("pipeline-jobs", jobs));
  }

  function stopPipelineDeadline() {
    if (pipelineDeadline !== null) clearTimeout(pipelineDeadline);
    pipelineDeadline = null;
  }

  function markPipelineUnverified() {
    // Fail closed: the verification claim belongs to the cycle that just
    // missed its bound, so the badge and the per-service rows stop asserting
    // health instead of keeping a stale green state on screen. The provenance
    // panel is deliberately left alone: a published source snapshot does not
    // expire with this cycle's freshness bound.
    const status = document.getElementById("pipeline-status");
    status.classList.remove("risk-good");
    status.classList.add("risk-error");
    setText("pipeline-badge", "SIN VERIFICAR");
    setText("pipeline-status", "La consulta de estado no llegó dentro de la cota; el pipeline queda sin verificar y el ciclo se reintentará automáticamente.");
    setText("pipeline-risk", "Sin verificar");
    setText("pipeline-jobs", "Sin verificar");
    document.getElementById("pipeline-risk").classList.remove("risk-good");
    document.getElementById("pipeline-jobs").classList.remove("risk-good");
  }

  // Same deadline pattern as `armDeadline` on the job path: fence the cycle,
  // abort the hung reads and leave the view honest. Unlike the job path this
  // one also keeps the cadence alive, because the pipeline panel must recover
  // on its own rather than depend on a user action.
  function armPipelineDeadline(sequence) {
    stopPipelineDeadline();
    pipelineDeadline = setTimeout(() => {
      pipelineDeadline = null;
      if (sequence !== pipelineSequence || accessToken === "") return;
      pipelineSequence++;
      if (pipelineController) pipelineController.abort();
      pipelineController = null;
      markPipelineUnverified();
      pipelineTimer = setTimeout(refreshPipeline, PIPELINE_REFRESH_MS);
    }, PIPELINE_DEADLINE_MS);
  }

  function cancelPipelineRefresh() {
    pipelineSequence++;
    if (pipelineTimer !== null) clearTimeout(pipelineTimer);
    pipelineTimer = null;
    stopPipelineDeadline();
    if (pipelineController) pipelineController.abort();
    pipelineController = null;
  }

  async function refreshPipeline() {
    pipelineTimer = null;
    if (accessToken === "") return;
    const sequence = ++pipelineSequence;
    if (pipelineController) pipelineController.abort();
    pipelineController = new AbortController();
    const signal = pipelineController.signal;
    const isCurrent = () => sequence === pipelineSequence && accessToken !== "";
    armPipelineDeadline(sequence);
    await Promise.all([loadSources(isCurrent, signal), loadPipelineStatus(isCurrent, signal)]);
    if (!isCurrent()) return;
    stopPipelineDeadline();
    // Recursive setTimeout only: this environment defines no setInterval, and
    // each cycle replaces one bounded, cancelable timer while the session lives.
    pipelineTimer = setTimeout(refreshPipeline, PIPELINE_REFRESH_MS);
  }

  // Maps the canonical API levels to product labels. The raw tokens
  // (bajo/medio/alto/desconocido) are machine values and must never reach the
  // screen unpresented; "desconocido" is INFORMACIÓN INSUFICIENTE, a fourth
  // honest state, never a severity and never a lowercase machine token.
  function levelLabel(level) {
    switch (level) {
      case "bajo": return "BAJO";
      case "medio": return "MEDIO";
      case "alto": return "ALTO";
      case "desconocido": return "INFORMACIÓN INSUFICIENTE";
      default: return "SIN CLASIFICAR";
    }
  }

  const nonempty = (value) => typeof value === "string" && value.trim() !== "";
  const record = (value) => value !== null && typeof value === "object" && !Array.isArray(value);

  // Presentation validation, not policy fingerprint verification or observation evidence.
  function presentBatch(batch) {
    const factorValid = (factor) => record(factor) && nonempty(factor.name) &&
      typeof factor.known === "boolean" && typeof factor.note === "string" &&
      Number.isFinite(factor.value) && Number.isFinite(factor.contribution);
    const factors = Array.isArray(batch.factors) ? batch.factors : [];
    const policy = batch.policy;
    const policyValid = record(policy) && nonempty(policy.policy_id) && nonempty(policy.params_hash) &&
      record(policy.parameters) && Object.keys(policy.parameters).length > 0 &&
      Object.entries(policy.parameters).every(([key, value]) => nonempty(key) && typeof value === "string");
    const omissions = [];
    if (!canonicalLevel(batch.risk_level)) omissions.push("nivel");
    if (typeof batch.demo !== "boolean") omissions.push("procedencia demo");
    if (!factors.length || !factors.every(factorValid)) omissions.push("factores");
    if (!nonempty(batch.rationale)) omissions.push("justificación");
    if (!policyValid) omissions.push("política");
    return { valid: omissions.length === 0, omissions, factors: factors.filter(factorValid), policyValid };
  }

  function clearBatchDetails() {
    stopValidityRefresh();
    for (const id of ["risk-rationale", "risk-policy", "batch-origin", "batch-notice", "batch-validity"]) setText(id, "Sin consultar");
    document.getElementById("risk-factors").replaceChildren();
    document.getElementById("risk-status").classList.remove("risk-good", "risk-error");
  }

  function stopValidityRefresh() {
    if (validityTimer !== null) clearTimeout(validityTimer);
    validityTimer = null;
    validityBatch = null;
  }

  // Lot operational validity is a property of the batch window (published_at /
  // valid_until). It is deliberately shown apart from the synthetic / input
  // recency notice: a published_at timestamp is not observation freshness.
  //
  // The label must not wait for a user reload to notice that the batch expired,
  // so a bounded one-shot timer re-renders it once the safe bound passes.
  // Recursive setTimeout only (this environment defines no setInterval), each
  // render replaces the previous timer, and logout, a 401 reset and a zone
  // change clear it, so no live timer can outlive the batch it describes. The
  // delay is capped and the callback recurses, which keeps far expiries bounded
  // without a busy loop.
  //
  // A claim of operational validity must never survive the bound it claims, so
  // the assertion ends at `valid_until - VALIDITY_SKEW_MS` (early, never late).
  // That is why the scheduled flip is `deadline - now`: for a live batch the
  // timer fires no later than valid_until, and a timer that fires early simply
  // re-renders the still-current Vigente label and re-arms.
  function renderLotValidity(batch) {
    stopValidityRefresh();
    validityBatch = batch;
    const until = Date.parse(batch.valid_until);
    const published = nonempty(batch.published_at) ? `publicado ${batch.published_at}` : "publicación sin fecha informada";
    const untilText = Number.isFinite(until) ? `válido hasta ${batch.valid_until}` : "sin vencimiento informado";
    if (!Number.isFinite(until)) {
      setText("batch-validity", `Vigencia operacional del lote: sin información (${published}; ${untilText}).`);
      return;
    }
    const deadline = until - VALIDITY_SKEW_MS;
    const expired = deadline <= Date.now();
    setText("batch-validity", `Vigencia operacional del lote: ${expired ? "Vencida" : "Vigente"} (${published}; ${untilText}).`);
    if (expired) return;
    const delay = Math.min(deadline - Date.now(), VALIDITY_MAX_DELAY_MS);
    validityTimer = setTimeout(() => {
      validityTimer = null;
      if (validityBatch !== batch || accessToken === "") return;
      renderLotValidity(batch);
    }, delay);
  }

  function renderBatch(batch) {
    const presentation = presentBatch(batch);
    setText("batch-origin", batch.demo === true ? "Sintético · Synthetic" : "Procedencia discrepante con este entorno sintético");
    setText("batch-notice", `Entorno sintético: no riesgo real observado. ${nonempty(batch.notice) ? batch.notice : "Aviso de la API ausente o inválido."}`);
    renderLotValidity(batch);
    setText("risk-rationale", nonempty(batch.rationale) ? batch.rationale : "Justificación ausente o inválida");
    const factors = document.getElementById("risk-factors");
    factors.replaceChildren();
    for (const factor of presentation.factors) {
      const item = document.createElement("li");
      item.textContent = factor.known
        ? `${factor.name}: ${factor.value} · contribución ${factor.contribution} · ${factor.note}`
        : `${factor.name}: desconocido · ${factor.note}`;
      factors.append(item);
    }
    if (presentation.omissions.includes("factores")) {
      const item = document.createElement("li");
      item.textContent = "Factores ausentes o inválidos; solo se muestran entradas válidas.";
      factors.append(item);
    }
    setText("risk-policy", presentation.policyValid
      ? `${batch.policy.policy_id} · hash informado (no verificado): ${batch.policy.params_hash} · ${Object.entries(batch.policy.parameters).map(([key, value]) => `${key}=${value}`).join("; ")}`
      : "Política ausente o inválida");
    setText("risk-level", levelLabel(batch.risk_level));
    setText("risk-pill", levelLabel(batch.risk_level));
    setText("data-status", batch.data_status);
    setText("model-version", batch.model_version);
    setText("valid-until", batch.valid_until);
    setText("batch-id", batch.id);
    activeLevel = presentation.valid ? canonicalLevel(batch.risk_level) : "";
    // Recorded for the region too: once the selection moves on, this zone must
    // still show the batch it has, or lose the colour if the batch is invalid.
    if (typeof batch.zone_id === "string" && batch.zone_id !== "") {
      if (activeLevel === "") zoneLevels.delete(batch.zone_id); else zoneLevels.set(batch.zone_id, activeLevel);
    }
    applyZoneLevels();
    setText("risk-status", batch.demo === true
      ? "Entradas de demostración sintéticas: no representan riesgo real observado. La publicación no acredita frescura de observaciones. Clasificación por regla propia no oficial."
      : "Lote recibido de la API · clasificación calculada por una regla propia no oficial.");
    document.getElementById("risk-status").classList.remove("risk-error");
    if (!presentation.valid) setText("risk-status", `Lote incompleto: ${presentation.omissions.join(", ")}. Sin evaluación validada; entorno sintético, no riesgo real observado.`);
    document.getElementById("risk-status").classList.toggle("risk-good", presentation.valid);
    return presentation.valid;
  }

  async function loadLatest(expected = expectedBatch) {
    const status = document.getElementById("risk-status");
    if (selectedZone === "") {
      status.textContent = "Selecciona una zona para consultar el último lote.";
      return;
    }
    const requestedZone = selectedZone;
    const sequence = ++latestRequestSequence;
    status.classList.remove("risk-error", "risk-good");
    status.textContent = "Consultando el último lote…";
    // The batch metadata is being invalidated, so the validity timer that
    // describes the previous batch must not keep re-rendering its label.
    stopValidityRefresh();
    setText("risk-level", "Sin consultar");
    setText("risk-pill", "SIN CONSULTAR");
    for (const id of ["batch-id", "data-status", "model-version", "valid-until", "batch-validity"]) setText(id, "Sin consultar");
    activeLevel = "";
    applyZoneLevels();
    let batch;
    try {
      batch = await apiRequest(`${gatewayOrigin}/v1/risk/latest?zone_id=${encodeURIComponent(requestedZone)}`, expected && jobController ? { signal: jobController.signal } : {}, () => sequence === latestRequestSequence && requestedZone === selectedZone);
    } catch (error) {
      // A superseded request must not touch the view: the newer request owns it.
      if (sequence !== latestRequestSequence) return;
      if (error.message === "http_404") {
        // A 404 from latest means there is no published batch, not that the
        // zone is low risk. The polygon stays neutral; no colour is invented.
        status.textContent = "La API no informa lotes publicados para esta zona.";
        setText("risk-level", "Sin lote publicado");
        setText("risk-pill", "SIN LOTE");
        activeLevel = "";
        applyZoneLevels();
      } else showRequestError(status, error, "consultar el lote");
      return;
    }
    // Stale-response race guard. If the operator changed zone while this
    // request was in flight, the sequence has moved on and this answer belongs
    // to a superseded view: discard it wholly, leaving every pixel to the newer
    // request rather than colouring the new selection with the old zone's level.
    if (sequence !== latestRequestSequence || requestedZone !== selectedZone) return;
    if (expected && !matchesBatch(batch, expected)) {
      status.textContent = "El último lote no corresponde al trabajo publicado; aún no se verificó su lectura.";
      setText("risk-level", "Sin lote verificado");
      setText("risk-pill", "SIN VERIFICAR");
      return false;
    }
    // Defence in depth: never render a batch the server computed for a
    // different zone. Give the panel a terminal state so a discard cannot leave
    // it stuck on "Consultando…".
    if (!batch || batch.zone_id !== requestedZone) {
      status.textContent = "La API devolvió un lote de otra zona; se descartó.";
      activeLevel = "";
      applyZoneLevels();
      return;
    }
    return renderBatch(batch);
  }

  // The history panel shows the rows the API returned for ONE zone. Search and
  // the status filter are client-side over exactly those rows: they issue no
  // read and cannot reach another zone, while the badge keeps reporting the
  // returned length and the result count reports what is visible.
  function populateHistoryStatusFilter(batches) {
    const select = document.getElementById("history-status-filter");
    const previous = select.value;
    const statuses = [...new Set(batches.map((batch) => String(batch.data_status ?? "")).filter((value) => value !== ""))].sort();
    select.replaceChildren();
    const any = document.createElement("option");
    any.value = "";
    any.textContent = "Todos los estados";
    select.append(any);
    for (const status of statuses) {
      const option = document.createElement("option");
      option.value = status;
      option.textContent = status;
      select.append(option);
    }
    select.value = statuses.includes(previous) ? previous : "";
  }

  function renderHistoryRows() {
    const rows = document.getElementById("history-rows");
    const query = document.getElementById("history-query").value.trim().toLowerCase();
    const status = document.getElementById("history-status-filter").value;
    const visible = historyBatches.filter((batch) => {
      if (status !== "" && String(batch.data_status ?? "") !== status) return false;
      if (query === "") return true;
      return [batch.requested_at, batch.data_status, batch.id].some((value) => String(value ?? "").toLowerCase().includes(query));
    });
    rows.replaceChildren();
    for (const batch of visible) {
      const row = document.createElement("tr");
      for (const value of [batch.requested_at, batch.data_status, batch.id]) {
        const cell = document.createElement("td");
        cell.textContent = value == null ? "No informado" : String(value);
        row.append(cell);
      }
      rows.append(row);
    }
    setText("history-visible-count", `${visible.length} de ${historyBatches.length}`);
    document.querySelector(".history-panel").classList.toggle("has-rows", historyBatches.length > 0);
  }

  function resetHistoryFilters() {
    historyBatches = [];
    document.getElementById("history-query").value = "";
    populateHistoryStatusFilter([]);
    setText("history-visible-count", "\u2014");
  }

  async function loadHistory(expected = expectedBatch) {
    const state = document.getElementById("history-state");
    if (selectedZone === "") {
      state.textContent = "Selecciona una zona para consultar el historial.";
      return;
    }
    const requestedZone = selectedZone;
    const sequence = ++historyRequestSequence;
    state.classList.remove("risk-error");
    state.textContent = "Consultando historial…";
    let batches;
    try {
      batches = await apiRequest(`${gatewayOrigin}/v1/risk/history?zone_id=${encodeURIComponent(requestedZone)}`, expected && jobController ? { signal: jobController.signal } : {}, () => sequence === historyRequestSequence && requestedZone === selectedZone);
    } catch (error) {
      // A superseded request must not touch the view: the newer one owns it.
      if (sequence !== historyRequestSequence) return;
      showRequestError(state, error, "consultar el historial");
      return;
    }
    // Same stale-response guard as loadLatest: history fetched for one zone must
    // never replace the rows shown for another.
    if (sequence !== historyRequestSequence || requestedZone !== selectedZone) return;
    // Every row must belong to the zone this request was issued for. A response
    // that carries another zone is discarded as a whole, with a terminal state
    // instead of a permanent "Consultando historial…".
    if (!Array.isArray(batches) || !batches.every((batch) => batch && batch.zone_id === requestedZone)) {
      state.textContent = "La API devolvió un historial de otra zona; se descartó.";
      return;
    }
    if (expected && !batches.some((batch) => matchesBatch(batch, expected) && presentBatch(batch).valid)) {
      state.textContent = "El historial todavía no informa el lote del trabajo publicado.";
      return false;
    }
    setText("history-count", batches.length);
    setText("metric-batches", batches.length);
    historyBatches = batches;
    populateHistoryStatusFilter(batches);
    renderHistoryRows();
    if (batches.length === 0) {
      state.textContent = "La API devolvió un historial vacío.";
      return;
    }
    state.textContent = batches.every((batch) => batch.demo === true)
      ? "Registros sintéticos devueltos por la API."
      : "Registros devueltos por la API.";
    return batches.every((batch) => presentBatch(batch).valid);
  }

  // This card is ONLY the gateway process answer. A 204 here is not pipeline
  // proof: the verifiable pipeline state lives in the separate panel driven by
  // /v1/risk/status and /v1/jobs/status.
  async function checkReadiness() {
    const readinessStatus = document.getElementById("readiness-status");
    const readinessRefresh = document.getElementById("readiness-refresh");
    readinessStatus.textContent = "Consultando disponibilidad del gateway…";
    readinessRefresh.disabled = true;
    try {
      const response = await fetch(`${gatewayOrigin}/health/ready`, { cache: "no-store", credentials: "omit" });
      readinessStatus.textContent = response.ok
        ? `Gateway disponible (HTTP ${response.status}); solo disponibilidad del gateway, no acredita el pipeline.`
        : `Gateway no disponible (HTTP ${response.status}); solo disponibilidad del gateway.`;
      document.querySelector(".health-dot").classList.toggle("ready", response.ok);
      document.querySelector(".health-dot").classList.toggle("down", !response.ok);
    } catch {
      readinessStatus.textContent = "No fue posible consultar el gateway; su disponibilidad no acredita el pipeline.";
      document.querySelector(".health-dot").classList.remove("ready");
      document.querySelector(".health-dot").classList.add("down");
    } finally {
      readinessRefresh.disabled = false;
    }
  }

  async function loadJob() {
    if (!currentJobID) return;
    if (jobTimer !== null) clearTimeout(jobTimer);
    jobTimer = null;
    if (!jobController) {
      jobController = new AbortController();
      pollAttempts = 0;
    }
    const sequence = jobSequence;
    const zone = selectedZone;
    if (jobDeadline === null) armDeadline(sequence, zone);
    const id = currentJobID;
    const status = document.getElementById("job-status");
    const attempt = ++pollAttempts;
    status.textContent = "Consultando estado del trabajo…";
    try {
      const job = await apiRequest(`${gatewayOrigin}/v1/jobs/${encodeURIComponent(id)}`, { signal: jobController.signal }, () => ownsJob(sequence, zone) && attempt === pollAttempts);
      if (!ownsJob(sequence, zone) || attempt !== pollAttempts) return;
      if (job.id !== id || job.zone_id !== zone) {
        stopDeadline();
        status.textContent = "La API devolvió otro trabajo o zona; se descartó.";
        return;
      }
      await followJob(job, sequence, zone);
    } catch (error) {
      if (ownsJob(sequence, zone) && attempt === pollAttempts) {
        stopDeadline();
        showRequestError(status, error, "consultar el trabajo");
      }
    }
  }

  async function followJob(job, sequence, zone) {
    const attempt = pollAttempts;
    renderJob(job);
    if (job.state === "published" && job.batch_id) {
      expectedBatch = { zone, job: job.id, batch: job.batch_id };
      const results = await Promise.all([loadLatest(expectedBatch), loadHistory(expectedBatch)]);
      if (!ownsJob(sequence, zone) || attempt !== pollAttempts) return;
      if (results.every(Boolean)) {
        stopDeadline();
        setText("job-status", `Trabajo publicado · lote ${job.batch_id} verificado en último lote e historial.`);
        return;
      }
    }
    if (pollAttempts >= MAX_POLL_ATTEMPTS) {
      stopDeadline();
      setText("job-status", "Se agotó la consulta automática; publicación o lectura sin verificar. Puedes actualizar manualmente.");
      return;
    }
    jobTimer = setTimeout(loadJob, 2000);
  }

  function renderJob(job) {
    currentJobID = job.id;
    setText("job-state", job.state);
    setText("job-id", job.id);
    setText("job-requested", job.requested_at);
    // The selected profile is server-assigned metadata on the job; it is shown
    // only when the server actually reported one, never inferred by the page.
    const scenario = typeof job.scenario_id === "string" && job.scenario_id !== "" ? ` · escenario ${job.scenario_id}` : "";
    setText("job-status", job.state === "published" && job.batch_id
      ? `Trabajo publicado · lote ${job.batch_id}${scenario}`
      : `Trabajo ${job.state}${scenario}; el servidor controla su procesamiento.`);
    document.getElementById("refresh-job").disabled = false;
  }

  // --- Perfiles y permisos -------------------------------------------------
  //
  // Two clearly separated things, and nothing else:
  //   1. a documentation matrix of which REAL role may do what, documented from
  //      the actual server-side guards and cited by `path:line`;
  //   2. the role this session actually presents, decoded from the access token
  //      payload.
  //
  // The retired mockup let the operator PICK a role. That selector is NOT
  // ported: a page cannot grant a permission, so a selector would be a fake
  // authorization boundary. The decoded value is a FACT ABOUT THE TOKEN for
  // display only; the server is the only authority because only the server
  // verifies the signature.
  //
  // The role guards read from the repository, verified line by line:
  //   backend/services/risk/internal/handler/security.go:42-43  the risk service
  //     accepts a token whose realm_access.roles contains "viewer" OR "admin".
  //   backend/services/risk/internal/handler/security.go:48    any other role is
  //     answered with 403 forbidden (an unverifiable token is 401 instead).
  //   backend/services/risk/internal/handler/api.go:89          that guard wraps
  //     the whole risk mux, so it covers every /v1/risk route below.
  //   backend/services/jobs/internal/handler/security.go:43-44  the jobs service
  //     accepts ONLY "admin".
  //   backend/services/jobs/internal/handler/security.go:49    so a verified
  //     viewer is answered with 403 forbidden there.
  //   backend/services/jobs/internal/handler/api.go:160          that guard wraps
  //     the whole jobs mux: jobs status, job creation, job read and /v1/audit.
  //   backend/services/gateway/internal/handler/gateway.go:39   the gateway only
  //     proxies /v1/audit to the jobs service; it adds no role of its own.
  //   backend/services/gateway/internal/handler/gateway.go:74   /health/ready
  //     answers 204 before any bearer check, so it needs no role at all.
  //   backend/services/gateway/internal/handler/gateway.go:96-105  the gateway
  //     does require a verifiable bearer with a subject, but decides no role.
  // The role-carrying claim is the one the realm mapper emits in ACCESS tokens:
  // backend/deploy/local/firewatch-realm.json:28 declares `realm_access.roles`,
  // multivalued, with `access.token.claim: true` and `id.token.claim: false`.
  const PERMISSION_MATRIX = [
    {
      capability: "Disponibilidad del gateway · GET /health/ready",
      viewer: "Sí: responde 204 sin comprobar token ni rol",
      admin: "Sí: responde 204 sin comprobar token ni rol",
      guards: ["backend/services/gateway/internal/handler/gateway.go:74"],
    },
    {
      capability: "Autenticación de cualquier ruta /v1/* · bearer verificado en el gateway",
      viewer: "Sí: exige un bearer verificable con sujeto; no decide ningún rol",
      admin: "Sí: exige un bearer verificable con sujeto; no decide ningún rol",
      guards: ["backend/services/gateway/internal/handler/gateway.go:96", "backend/services/gateway/internal/handler/gateway.go:102"],
    },
    {
      capability: "Consultar el estado del servicio de riesgo · GET /v1/risk/status",
      viewer: "Sí",
      admin: "Sí",
      guards: ["backend/services/risk/internal/handler/api.go:41", "backend/services/risk/internal/handler/security.go:43", "backend/services/risk/internal/handler/api.go:89"],
    },
    {
      capability: "Consultar el último lote de una zona · GET /v1/risk/latest",
      viewer: "Sí",
      admin: "Sí",
      guards: ["backend/services/risk/internal/handler/api.go:80", "backend/services/risk/internal/handler/security.go:43"],
    },
    {
      capability: "Consultar el historial de una zona · GET /v1/risk/history",
      viewer: "Sí",
      admin: "Sí",
      guards: ["backend/services/risk/internal/handler/api.go:81", "backend/services/risk/internal/handler/security.go:43"],
    },
    {
      capability: "Consultar el catálogo de zonas · GET /v1/risk/zones",
      viewer: "Sí",
      admin: "Sí",
      guards: ["backend/services/risk/internal/handler/api.go:82", "backend/services/risk/internal/handler/security.go:43"],
    },
    {
      capability: "Consultar los escenarios sintéticos · GET /v1/risk/synthetic-scenarios",
      viewer: "Sí",
      admin: "Sí",
      guards: ["backend/services/risk/internal/handler/api.go:83", "backend/services/risk/internal/handler/security.go:43"],
    },
    {
      capability: "Consultar la procedencia geográfica · GET /v1/risk/sources",
      viewer: "Sí",
      admin: "Sí",
      guards: ["backend/services/risk/internal/handler/api.go:88", "backend/services/risk/internal/handler/security.go:43"],
    },
    {
      capability: "Solicitar un lote sintético · POST /v1/jobs",
      viewer: "No: 403 forbidden",
      admin: "Sí",
      guards: ["backend/services/jobs/internal/handler/api.go:52", "backend/services/jobs/internal/handler/security.go:44", "backend/services/jobs/internal/handler/api.go:160"],
    },
    {
      capability: "Consultar un trabajo · GET /v1/jobs/{id}",
      viewer: "No: 403 forbidden",
      admin: "Sí",
      guards: ["backend/services/jobs/internal/handler/api.go:135", "backend/services/jobs/internal/handler/security.go:44"],
    },
    {
      capability: "Consultar el estado del servicio de trabajos · GET /v1/jobs/status",
      viewer: "No: 403 forbidden",
      admin: "Sí",
      guards: ["backend/services/jobs/internal/handler/api.go:51", "backend/services/jobs/internal/handler/security.go:44"],
    },
    {
      capability: "Consultar el registro de actividad · GET /v1/audit",
      viewer: "No: 403 forbidden",
      admin: "Sí",
      guards: ["backend/services/jobs/internal/handler/api.go:152", "backend/services/jobs/internal/handler/security.go:44", "backend/services/gateway/internal/handler/gateway.go:39"],
    },
    {
      capability: "Presentar un tercer rol · cualquier valor que no sea viewer ni admin",
      viewer: "No: 403 forbidden",
      admin: "No: 403 forbidden",
      guards: ["backend/services/risk/internal/handler/security.go:48", "backend/services/jobs/internal/handler/security.go:49"],
    },
    {
      capability: "Rol efectivo que el proveedor de identidad entrega en el token",
      viewer: "No verificado: depende de la configuración del reino y ningún endpoint lo informa",
      admin: "No verificado: depende de la configuración del reino y ningún endpoint lo informa",
      guards: [],
      note: "No verificado · no hay ruta ni guarda del servidor que consultar; la página solo muestra lo que el token declare",
    },
  ];

  function renderPermissionMatrix() {
    const rows = document.getElementById("permissions-rows");
    rows.replaceChildren();
    for (const entry of PERMISSION_MATRIX) {
      const row = document.createElement("tr");
      // The guard column is a citation, never prose: `path:line` or an explicit
      // "No verificado" for the row that cannot be read from the server code.
      const guard = entry.guards.length > 0 ? entry.guards.join(" · ") : entry.note;
      for (const value of [entry.capability, entry.viewer, entry.admin, guard]) {
        const cell = document.createElement("td");
        cell.textContent = value;
        row.append(cell);
      }
      rows.append(row);
    }
  }

  // --- Session role decoded from the token ---------------------------------
  //
  // The page holds the access token in memory and never decoded it. That payload
  // is the ONLY real source of the session role: the gateway proxies exactly
  // /v1/risk, /v1/jobs and /v1/audit, so there is no whoami/me/profile endpoint
  // to ask. Reading the payload is DISPLAY ONLY: it never gates, blocks, allows,
  // hides or alters a request, a panel or a decision, and the page behaves
  // identically whatever the decoded value says. The server stays the only
  // authority because it verifies the signature and this page does not.
  //
  // This environment denies `atob`, `Buffer` and `TextDecoder`, so base64url and
  // UTF-8 decoding are implemented here in plain JavaScript.
  const BASE64URL_ALPHABET = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_";

  function decodeBase64URL(value) {
    if (typeof value !== "string") return null;
    // Trailing padding is tolerated; an interior or non-URL character is not,
    // because a payload that does not decode must never be guessed at.
    const text = value.replace(/=+$/, "");
    const bytes = [];
    let accumulator = 0;
    let bits = 0;
    for (const character of text) {
      const index = BASE64URL_ALPHABET.indexOf(character);
      if (index < 0) return null;
      accumulator = (accumulator << 6) | index;
      bits += 6;
      if (bits >= 8) {
        bits -= 8;
        bytes.push((accumulator >> bits) & 0xff);
        accumulator &= (1 << bits) - 1;
      }
    }
    return bytes;
  }

  // Strict UTF-8: an overlong, truncated or surrogate sequence is rejected
  // instead of being turned into replacement characters that would misrepresent
  // what the token actually declares.
  function decodeUTF8(bytes) {
    let text = "";
    for (let index = 0; index < bytes.length; index += 1) {
      const byte = bytes[index];
      let code;
      if (byte < 0x80) code = byte;
      else if (byte >= 0xc2 && byte <= 0xdf && index + 1 < bytes.length && (bytes[index + 1] & 0xc0) === 0x80) {
        code = ((byte & 0x1f) << 6) | (bytes[index + 1] & 0x3f);
        index += 1;
      } else if (byte >= 0xe0 && byte <= 0xef && index + 2 < bytes.length &&
        (bytes[index + 1] & 0xc0) === 0x80 && (bytes[index + 2] & 0xc0) === 0x80) {
        code = ((byte & 0x0f) << 12) | ((bytes[index + 1] & 0x3f) << 6) | (bytes[index + 2] & 0x3f);
        index += 2;
      } else if (byte >= 0xf0 && byte <= 0xf4 && index + 3 < bytes.length &&
        (bytes[index + 1] & 0xc0) === 0x80 && (bytes[index + 2] & 0xc0) === 0x80 && (bytes[index + 3] & 0xc0) === 0x80) {
        code = ((byte & 0x07) << 18) | ((bytes[index + 1] & 0x3f) << 12) | ((bytes[index + 2] & 0x3f) << 6) | (bytes[index + 3] & 0x3f);
        index += 3;
      } else return null;
      if (code > 0x10ffff || (code >= 0xd800 && code <= 0xdfff)) return null;
      if (code > 0xffff) {
        const shifted = code - 0x10000;
        text += String.fromCharCode(0xd800 + (shifted >> 10), 0xdc00 + (shifted & 0x3ff));
      } else text += String.fromCharCode(code);
    }
    return text;
  }

  // Returns the parsed payload, or null when the token is not a three-segment
  // JWT whose middle segment decodes to JSON. The payload is returned as parsed:
  // an array or a scalar is still "decodable" and is handled by the role reader.
  function decodeJWTPayload(token) {
    if (typeof token !== "string") return null;
    const parts = token.split(".");
    if (parts.length !== 3) return null;
    const bytes = decodeBase64URL(parts[1]);
    if (bytes === null) return null;
    const json = decodeUTF8(bytes);
    if (json === null) return null;
    try {
      return JSON.parse(json);
    } catch {
      return null;
    }
  }

  // The roles the token declares, or null when it declares none usable. No
  // default, no fallback and no inference: an absent, non-array, empty or
  // non-string list is "no informado", never "viewer".
  function sessionRolesFromPayload(payload) {
    if (!record(payload)) return null;
    const realmAccess = record(payload.realm_access) ? payload.realm_access : null;
    if (!realmAccess || !Array.isArray(realmAccess.roles)) return null;
    const roles = [];
    for (const role of realmAccess.roles) {
      if (!nonempty(role)) continue;
      const name = role.trim();
      if (!roles.includes(name)) roles.push(name);
    }
    return roles.length > 0 ? roles : null;
  }

  function renderSessionRole() {
    if (accessToken === "") {
      setText("session-roles-badge", "SIN SESIÓN");
      setText("session-roles", "No disponible");
      setText("session-roles-status", "Sin token en memoria: el rol declarado por la sesión no está disponible; conecta una sesión para leerlo.");
      return;
    }
    const payload = decodeJWTPayload(accessToken);
    if (payload === null) {
      setText("session-roles-badge", "NO DISPONIBLE");
      setText("session-roles", "No disponible");
      setText("session-roles-status", "El token de acceso en memoria no es un JWT cuyo payload pueda decodificarse, así que el rol de la sesión no está disponible. La página no lo deduce del código HTTP ni de ningún otro dato.");
      return;
    }
    const roles = sessionRolesFromPayload(payload);
    if (roles === null) {
      setText("session-roles-badge", "NO INFORMADO");
      setText("session-roles", "No informado");
      setText("session-roles-status", "El token de acceso no declara realm_access.roles utilizables (el claim falta, no es un arreglo o viene vacío); el rol de la sesión no está informado y la página no supone ninguno.");
      return;
    }
    setText("session-roles-badge", "TOKEN LEÍDO");
    setText("session-roles", roles.join(", "));
    setText("session-roles-status", `El payload del token declara los roles ${roles.join(", ")}. La página decodifica el payload sin verificar la firma: es un dato informativo del token, no una autorización, y el servidor, que sí verifica la firma, sigue siendo la única autoridad.`);
  }

  // The pure helpers are exported for offline verification only. `module` is
  // undefined in the browser, so this branch never runs there, and the DOM
  // wiring below is skipped under Node.
  if (typeof module !== "undefined" && module.exports) {
    module.exports = {
      DETACHED_LONGITUDE,
      geometryBounds,
      collectBounds,
      createProjection,
      pathData,
      partitionZones,
      countVertices,
    };
  }
  if (typeof document === "undefined") return;

  document.getElementById("login").addEventListener("click", startLogin);

  document.getElementById("logout").addEventListener("click", () => {
    const idTokenHint = idToken;
    sessionStorage.removeItem(transactionKey);
    let logoutURL;
    try {
      logoutURL = new URL(`${issuer}/protocol/openid-connect/logout`);
      logoutURL.search = new URLSearchParams({
        id_token_hint: idTokenHint,
        client_id: clientID,
        post_logout_redirect_uri: redirectURI,
      });
    } catch {
      resetPrivateView("Sesión cerrada localmente; no fue posible iniciar el cierre en Keycloak.");
      return;
    }
    resetPrivateView("Cerrando la sesión de Keycloak…");
    window.location.assign(logoutURL.href);
  });

  document.getElementById("create-job").addEventListener("click", async () => {
    const createJobButton = document.getElementById("create-job");
    if (!accessToken || selectedZone === "") return;
    cancelJob();
    clearBatchDetails();
    const sequence = jobSequence;
    const zone = selectedZone;
    // Captured at click time, like the zone: a later change in the picker must
    // not alter a request that is already in flight.
    const scenario = selectedScenario();
    pollAttempts = 0;
    jobController = new AbortController();
    armDeadline(sequence, zone);
    activeLevel = "";
    applyZoneLevels();
    setText("risk-level", "Sin lote verificado");
    setText("risk-pill", "SIN VERIFICAR");
    setText("risk-status", "Esperando publicación y lectura del trabajo solicitado; entradas de demostración, no riesgo real observado.");
    for (const id of ["batch-id", "data-status", "model-version", "valid-until", "batch-validity"]) setText(id, "Sin consultar");
    createJobButton.disabled = true;
    const status = document.getElementById("job-status");
    status.classList.remove("risk-error");
    status.textContent = "Solicitando trabajo a la API…";
    try {
      const job = await apiRequest(`${gatewayOrigin}/v1/jobs`, {
        method: "POST",
        signal: jobController.signal,
        headers: { "Content-Type": "application/json", "Idempotency-Key": crypto.randomUUID() },
        body: JSON.stringify(scenario === "" ? { zone_id: zone } : { zone_id: zone, scenario_id: scenario }),
      }, () => ownsJob(sequence, zone));
      if (!ownsJob(sequence, zone)) return;
      if (job.zone_id !== zone || !job.id) {
        stopDeadline();
        status.textContent = "La API devolvió un trabajo de otra zona; se descartó.";
        return;
      }
      if (job.state === "published") await followJob(job, sequence, zone);
      else { renderJob(job); loadJob(); }
    } catch (error) {
      if (ownsJob(sequence, zone)) {
        stopDeadline();
        showRequestError(status, error, "solicitar el trabajo");
      }
    } finally {
      if (ownsJob(sequence, zone)) createJobButton.disabled = !accessToken;
    }
  });

  document.getElementById("refresh-job").addEventListener("click", loadJob);
  document.getElementById("readiness-refresh").addEventListener("click", checkReadiness);
  // Re-reads the same parameterless endpoint once, with no filter or page state
  // to lose: the service decides which rows the answer carries.
  document.getElementById("audit-refresh").addEventListener("click", () => loadAudit());
  // Both controls narrow the rows already on screen. The element value is read
  // back from the DOM so the filter state has one source of truth.
  document.getElementById("history-query").addEventListener("input", (event) => {
    document.getElementById("history-query").value = event.target.value;
    renderHistoryRows();
  });
  document.getElementById("history-status-filter").addEventListener("change", (event) => {
    document.getElementById("history-status-filter").value = event.target.value;
    renderHistoryRows();
  });
  document.getElementById("reset-filters").addEventListener("click", () => {
    document.getElementById("history-query").value = "";
    document.getElementById("history-status-filter").value = "";
    renderHistoryRows();
  });

  function selectZone(code) {
    if (!accessToken || !zoneShapes.has(code)) return;
    document.getElementById("zone-select").value = code;
    cancelJob();
    selectedZone = code;
    clearBatchDetails();
    document.getElementById("create-job").disabled = !accessToken;
    setText("job-state", "Sin solicitud");
    setText("job-id", "—");
    setText("job-status", "Sin trabajo para la zona seleccionada.");
    setText("risk-level", "Sin consultar");
    setText("risk-pill", "SIN CONSULTAR");
    setText("batch-id", "Sin consultar");
    document.getElementById("history-rows").replaceChildren();
    activeLevel = "";
    updateZoneSummary();
    applyZoneLevels();
    loadLatest();
    loadHistory();
  }

  document.getElementById("zone-select").addEventListener("change", (event) => selectZone(event.target.value));

  // The sidebar mirrors the page sections as in-page anchors. Marking the link
  // the user actually opened keeps the mockup's active marker honest: it follows
  // the clicked link and the fragment the browser opened, issues no request and
  // never claims a section nobody chose.
  const NAV_LINK_IDS = ["nav-overview", "nav-history", "nav-sources", "nav-pipeline", "nav-jobs", "nav-session", "nav-audit", "nav-permissions"];
  function markActiveNav(activeID) {
    for (const id of NAV_LINK_IDS) {
      const link = document.getElementById(id);
      const isActive = id === activeID;
      link.classList.toggle("is-active", isActive);
      if (isActive) link.setAttribute("aria-current", "true");
      else link.removeAttribute("aria-current");
    }
  }
  for (const id of NAV_LINK_IDS) document.getElementById(id).addEventListener("click", () => markActiveNav(id));
  const openedSection = `nav-${window.location.hash.slice(1)}`;
  markActiveNav(NAV_LINK_IDS.includes(openedSection) ? openedSection : NAV_LINK_IDS[0]);

  renderPermissionMatrix();
  renderSessionRole();
  checkReadiness();
  handleLoginCallback();
})();
