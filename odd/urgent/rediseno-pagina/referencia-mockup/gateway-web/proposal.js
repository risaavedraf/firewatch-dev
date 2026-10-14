// FireWatch served-page script.
//
// No framework and no external dependency: the gateway Content-Security-Policy
// fixes `script-src 'self'`, `connect-src 'self' http://localhost:8081` and
// `img-src 'self'`, so nothing outside this origin can be loaded. The
// geospatial layer is therefore drawn here, in plain SVG, with our own
// schematic projection.
//
// The pure projection helpers are exported at the bottom ONLY so they can be
// verified offline with Node against a small GeoJSON fixture; a browser never
// defines `module` and never reaches that branch.
(() => {
  "use strict";

  const issuer = "http://localhost:8081/realms/firewatch";
  const clientID = "firewatch-demo";
  const redirectURI = "http://localhost:8080/";
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
  const PIPELINE_REFRESH_MS = 30000;
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
      if (selected && activeLevel !== "") shape.classList.add(`level-${activeLevel}`);
      else shape.classList.add("zone-neutral");
      shape.setAttribute("aria-pressed", String(selected));
      if (selected) shape.classList.add("zone-selected");
    }
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

  async function loadZones(isCurrent = () => true) {
    showMapEmpty("Consultando el catálogo de zonas analizadas…");
    try {
      const collection = await apiRequest("/v1/risk/zones", {}, isCurrent);
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
    document.getElementById("session-state").textContent = "Sin conexión";
    document.getElementById("session-detail").textContent = "Token requerido para consultar";
    document.getElementById("logout").hidden = true;
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
    updateZoneSummary();
    showMapEmpty("Conecta un token autorizado para cargar las zonas analizadas.");
    setText("history-count", "\u2014");
    document.getElementById("history-rows").replaceChildren();
    document.getElementById("history-state").textContent = "Conecta un token para consultar el historial.";
    document.querySelector(".history-panel").classList.remove("has-rows");
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
    refreshPipeline();
    const sequence = ++sessionSequence;
    const isCurrent = () => sequence === sessionSequence && accessToken !== "";
    loadZones(isCurrent)
      .then(() => (isCurrent() ? Promise.all([loadLatest(), loadHistory()]) : null))
      .then(() => {
        if (isCurrent()) document.getElementById("auth-status").textContent = "Sesión lista. Las acciones autorizadas se validan en el servidor.";
      });
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
      result = await apiRequestStatus("/v1/risk/sources", signal ? { signal } : {}, isCurrent);
    } catch {
      if (!isCurrent()) return;
      clearSourcesPanel("No fue posible consultar la procedencia; se muestra como no disponible.");
      return;
    }
    if (!isCurrent()) return;
    renderSources(result);
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
    if (entry.status === 200 && body && body.ready === true && body.reason === "" && fresh && !deadStale) {
      setText(targetId, "Verificado: ready con checked_at fresco y profundidad de cola dentro de la cota.");
      element.classList.add("risk-good");
      return { verified: true, unknown: false, unverified: false };
    }
    const reason = body && nonempty(body.reason) ? ` \u00b7 ${body.reason}` : "";
    const stale = fresh ? "" : " \u00b7 instantánea fuera de la cota de antigüedad del cliente";
    setText(targetId, `Desconocido: el estado no se acredita como saludable (HTTP ${entry.status}${reason}${stale}).`);
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
      fetchServiceStatus("/v1/risk/status", isCurrent, signal),
      fetchServiceStatus("/v1/jobs/status", isCurrent, signal),
    ]);
    if (!isCurrent()) return;
    renderPipeline(renderServiceStatus("pipeline-risk", risk), renderServiceStatus("pipeline-jobs", jobs));
  }

  function cancelPipelineRefresh() {
    pipelineSequence++;
    if (pipelineTimer !== null) clearTimeout(pipelineTimer);
    pipelineTimer = null;
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
    await Promise.all([loadSources(isCurrent, signal), loadPipelineStatus(isCurrent, signal)]);
    if (!isCurrent()) return;
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
    for (const id of ["risk-rationale", "risk-policy", "batch-origin", "batch-notice", "batch-validity"]) setText(id, "Sin consultar");
    document.getElementById("risk-factors").replaceChildren();
    document.getElementById("risk-status").classList.remove("risk-good", "risk-error");
  }

  // Lot operational validity is a property of the batch window (published_at /
  // valid_until). It is deliberately shown apart from the synthetic / input
  // recency notice: a published_at timestamp is not observation freshness.
  function renderLotValidity(batch) {
    const until = Date.parse(batch.valid_until);
    const published = nonempty(batch.published_at) ? `publicado ${batch.published_at}` : "publicación sin fecha informada";
    const untilText = Number.isFinite(until) ? `válido hasta ${batch.valid_until}` : "sin vencimiento informado";
    if (!Number.isFinite(until)) {
      setText("batch-validity", `Vigencia operacional del lote: sin información (${published}; ${untilText}).`);
      return;
    }
    const state = until <= Date.now() ? "Vencida" : "Vigente";
    setText("batch-validity", `Vigencia operacional del lote: ${state} (${published}; ${untilText}).`);
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
    setText("risk-level", "Sin consultar");
    setText("risk-pill", "SIN CONSULTAR");
    for (const id of ["batch-id", "data-status", "model-version", "valid-until", "batch-validity"]) setText(id, "Sin consultar");
    activeLevel = "";
    applyZoneLevels();
    let batch;
    try {
      batch = await apiRequest(`/v1/risk/latest?zone_id=${encodeURIComponent(requestedZone)}`, expected && jobController ? { signal: jobController.signal } : {}, () => sequence === latestRequestSequence && requestedZone === selectedZone);
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
      batches = await apiRequest(`/v1/risk/history?zone_id=${encodeURIComponent(requestedZone)}`, expected && jobController ? { signal: jobController.signal } : {}, () => sequence === historyRequestSequence && requestedZone === selectedZone);
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
    const rows = document.getElementById("history-rows");
    rows.replaceChildren();
    setText("history-count", batches.length);
    if (batches.length === 0) {
      state.textContent = "La API devolvió un historial vacío.";
      document.querySelector(".history-panel").classList.remove("has-rows");
      return;
    }
    for (const batch of batches) {
      const row = document.createElement("tr");
      for (const value of [batch.requested_at, batch.data_status, batch.id]) {
        const cell = document.createElement("td");
        cell.textContent = value == null ? "No informado" : String(value);
        row.append(cell);
      }
      rows.append(row);
    }
    document.querySelector(".history-panel").classList.add("has-rows");
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
      const response = await fetch("/health/ready", { cache: "no-store", credentials: "omit" });
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
      const job = await apiRequest(`/v1/jobs/${encodeURIComponent(id)}`, { signal: jobController.signal }, () => ownsJob(sequence, zone) && attempt === pollAttempts);
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
    setText("job-status", job.state === "published" && job.batch_id
      ? `Trabajo publicado · lote ${job.batch_id}`
      : `Trabajo ${job.state}; el servidor controla su procesamiento.`);
    document.getElementById("refresh-job").disabled = false;
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
      const job = await apiRequest("/v1/jobs", {
        method: "POST",
        signal: jobController.signal,
        headers: { "Content-Type": "application/json", "Idempotency-Key": crypto.randomUUID() },
        body: JSON.stringify({ zone_id: zone }),
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

  checkReadiness();
  handleLoginCallback();
})();
