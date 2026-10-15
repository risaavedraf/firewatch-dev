import { PanelHeading, StatusCopy } from '../components/ui';
import { useSessionState } from '../app/store';
import { startLogin, type SessionState } from '../core/session';

// N8b — the session + permissions view (S13/S17). It carries two clearly
// separated things and nothing else:
//
//   1. the Keycloak sign-in control, with the honest note that the access token
//      lives in memory only — no token is persisted and no cookie is sent;
//   2. a documentation matrix of which REAL role may do what, cited guard by
//      guard as `ruta:línea` from the server's own code, plus the role this
//      session's token DECLARES.
//
// The retired mockup let the operator PICK a role. That selector is deliberately
// NOT ported: a page cannot grant a permission, so a selector would be a false
// authorization boundary. There is no role selector, switcher or picker anywhere
// in this view, and the decoded role never gates, hides, allows or alters a
// single request or panel — the server, which verifies the signature, is the only
// authority. The page decodes the payload WITHOUT verifying its signature and
// says so on screen.

export type PermissionRow = {
  readonly capability: string;
  readonly viewer: string;
  readonly admin: string;
  readonly guards: readonly string[];
  readonly note?: string;
};

// The matrix is exported so the test pins the rendered rows to the same table and
// re-reads each cited guard from disk instead of trusting a copy. The
// react-refresh rule wants a view module to export components only; this is a
// read-only documentation table, not a fast-refresh boundary, so its advisory is
// suppressed here on purpose, exactly as `PendingView` does.
//
// The guards were read from the repository, verified line by line:
//   backend/services/risk/internal/handler/security.go:42-43 accepts a token
//     whose realm_access.roles contains "viewer" OR "admin"; line 48 answers any
//     other role with 403 (an unverifiable token is 401 instead).
//   backend/services/risk/internal/handler/api.go:89 wraps the whole risk mux.
//   backend/services/jobs/internal/handler/security.go:43-44 accepts ONLY "admin";
//     line 49 answers a verified viewer with 403.
//   backend/services/jobs/internal/handler/api.go:160 wraps the whole jobs mux:
//     jobs status, job creation, job read and /v1/audit.
//   backend/services/gateway/internal/handler/gateway.go:39 only proxies
//     /v1/audit and adds no role of its own; line 74 answers /health/ready before
//     any bearer check; lines 96-105 require a verifiable bearer with a subject
//     but decide no role.
// eslint-disable-next-line react-refresh/only-export-components
export const PERMISSION_MATRIX: readonly PermissionRow[] = [
  {
    capability: 'Disponibilidad del gateway · GET /health/ready',
    viewer: 'Sí: responde 204 sin comprobar token ni rol',
    admin: 'Sí: responde 204 sin comprobar token ni rol',
    guards: ['backend/services/gateway/internal/handler/gateway.go:74'],
  },
  {
    capability: 'Autenticación de cualquier ruta /v1/* · bearer verificado en el gateway',
    viewer: 'Sí: exige un bearer verificable con sujeto; no decide ningún rol',
    admin: 'Sí: exige un bearer verificable con sujeto; no decide ningún rol',
    guards: [
      'backend/services/gateway/internal/handler/gateway.go:96',
      'backend/services/gateway/internal/handler/gateway.go:102',
    ],
  },
  {
    capability: 'Consultar el estado del servicio de riesgo · GET /v1/risk/status',
    viewer: 'Sí',
    admin: 'Sí',
    guards: [
      'backend/services/risk/internal/handler/api.go:41',
      'backend/services/risk/internal/handler/security.go:43',
      'backend/services/risk/internal/handler/api.go:89',
    ],
  },
  {
    capability: 'Consultar el último lote de una zona · GET /v1/risk/latest',
    viewer: 'Sí',
    admin: 'Sí',
    guards: [
      'backend/services/risk/internal/handler/api.go:80',
      'backend/services/risk/internal/handler/security.go:43',
    ],
  },
  {
    capability: 'Consultar el historial de una zona · GET /v1/risk/history',
    viewer: 'Sí',
    admin: 'Sí',
    guards: [
      'backend/services/risk/internal/handler/api.go:81',
      'backend/services/risk/internal/handler/security.go:43',
    ],
  },
  {
    capability: 'Consultar el catálogo de zonas · GET /v1/risk/zones',
    viewer: 'Sí',
    admin: 'Sí',
    guards: [
      'backend/services/risk/internal/handler/api.go:82',
      'backend/services/risk/internal/handler/security.go:43',
    ],
  },
  {
    capability: 'Consultar los escenarios sintéticos · GET /v1/risk/synthetic-scenarios',
    viewer: 'Sí',
    admin: 'Sí',
    guards: [
      'backend/services/risk/internal/handler/api.go:83',
      'backend/services/risk/internal/handler/security.go:43',
    ],
  },
  {
    capability: 'Consultar la procedencia geográfica · GET /v1/risk/sources',
    viewer: 'Sí',
    admin: 'Sí',
    guards: [
      'backend/services/risk/internal/handler/api.go:88',
      'backend/services/risk/internal/handler/security.go:43',
    ],
  },
  {
    capability: 'Solicitar un lote sintético · POST /v1/jobs',
    viewer: 'No: 403 forbidden',
    admin: 'Sí',
    guards: [
      'backend/services/jobs/internal/handler/api.go:52',
      'backend/services/jobs/internal/handler/security.go:44',
      'backend/services/jobs/internal/handler/api.go:160',
    ],
  },
  {
    capability: 'Consultar un trabajo · GET /v1/jobs/{id}',
    viewer: 'No: 403 forbidden',
    admin: 'Sí',
    guards: [
      'backend/services/jobs/internal/handler/api.go:135',
      'backend/services/jobs/internal/handler/security.go:44',
    ],
  },
  {
    capability: 'Consultar el estado del servicio de trabajos · GET /v1/jobs/status',
    viewer: 'No: 403 forbidden',
    admin: 'Sí',
    guards: [
      'backend/services/jobs/internal/handler/api.go:51',
      'backend/services/jobs/internal/handler/security.go:44',
    ],
  },
  {
    capability: 'Consultar el registro de actividad · GET /v1/audit',
    viewer: 'No: 403 forbidden',
    admin: 'Sí',
    guards: [
      'backend/services/jobs/internal/handler/api.go:152',
      'backend/services/jobs/internal/handler/security.go:44',
      'backend/services/gateway/internal/handler/gateway.go:39',
    ],
  },
  {
    capability: 'Presentar un tercer rol · cualquier valor que no sea viewer ni admin',
    viewer: 'No: 403 forbidden',
    admin: 'No: 403 forbidden',
    guards: [
      'backend/services/risk/internal/handler/security.go:48',
      'backend/services/jobs/internal/handler/security.go:49',
    ],
  },
  {
    capability: 'Rol efectivo que el proveedor de identidad entrega en el token',
    viewer: 'No verificado: depende de la configuración del reino y ningún endpoint lo informa',
    admin: 'No verificado: depende de la configuración del reino y ningún endpoint lo informa',
    guards: [],
    note: 'No verificado · no hay ruta ni guarda del servidor que consultar; la página solo muestra lo que el token declare',
  },
];

interface RoleDisplay {
  badge: string;
  value: string;
  status: string;
}

// The role shown is the one the access token declares, or an explicit
// "unavailable" state. There is no default, no fallback and no inference: an
// absent, unreadable or role-less token is never turned into a guessed role.
function roleDisplay(session: SessionState): RoleDisplay {
  if (session.status !== 'signed-in') {
    return {
      badge: 'SIN SESIÓN',
      value: 'No disponible',
      status:
        'Sin token en memoria: el rol declarado por la sesión no está disponible; conecta una sesión para leerlo.',
    };
  }
  if (session.roles === null) {
    return {
      badge: 'NO INFORMADO',
      value: 'No informado',
      status:
        'El token de acceso no declara realm_access.roles utilizables (el claim falta, no es un arreglo o viene vacío); el rol de la sesión no está informado y la página no supone ninguno.',
    };
  }
  const roles = session.roles.join(', ');
  return {
    badge: 'TOKEN LEÍDO',
    value: roles,
    status: `El payload del token declara los roles ${roles}. La página decodifica el payload sin verificar la firma: es un dato informativo del token, no una autorización, y el servidor, que sí verifica la firma, sigue siendo la única autoridad.`,
  };
}

export function SessionView() {
  const session = useSessionState();
  const role = roleDisplay(session);
  const authStatus =
    session.status === 'signed-in'
      ? 'Sesión lista. Las acciones autorizadas se validan en el servidor.'
      : session.status === 'signing-in'
        ? 'Iniciando el acceso seguro a Keycloak…'
        : '';

  return (
    <section className="view" id="session" data-view="session" aria-labelledby="session-title">
      <PanelHeading eyebrow="ACCESO A LA API" title="Sesión y permisos" titleId="session-title" />

      <div className="lower-grid">
        <article className="panel auth-panel">
          <div className="panel-heading">
            <div>
              <h3>Conectar sesión</h3>
            </div>
            <span className="lock-mark" aria-hidden="true">
              ⌑
            </span>
          </div>
          <p className="panel-note">
            Inicia sesión en Keycloak. FireWatch nunca solicita ni recibe tu contraseña; la API valida los permisos.
          </p>
          {/* F3 — one control, one state. `Shell.tsx` hides the logout control while
              signed out; this one is disabled while a session is established or still
              being established, so an authenticated operator can no longer see an
              enabled "Iniciar sesión" whose only effect would be to restart the OAuth
              flow. The status line below states which of the two situations it is. */}
          <button
            id="login"
            className="primary-button"
            type="button"
            disabled={session.status !== 'signed-out'}
            onClick={() => startLogin()}
          >
            Iniciar sesión con Keycloak
          </button>
          <p className="panel-note">
            El token de acceso permanece solo en memoria mientras esta página está abierta. No se guardan tokens ni se
            envían cookies.
          </p>
          <p id="auth-status" className="auth-status" role="status" aria-live="polite">
            {authStatus}
          </p>
        </article>

        <section
          className="panel access-panel permissions-panel"
          id="permissions"
          aria-labelledby="permissions-title"
        >
          <PanelHeading
            eyebrow="COORDINACIÓN"
            title="Perfiles y permisos"
            titleId="permissions-title"
            badge={
              <span className="neutral-tag" id="session-roles-badge">
                {role.badge}
              </span>
            }
          />
          <StatusCopy id="session-roles-status">{role.status}</StatusCopy>
          <dl className="data-list compact">
            <div>
              <dt>Roles declarados por el token</dt>
              <dd id="session-roles">{role.value}</dd>
            </div>
          </dl>
          <p id="session-role-note" className="panel-note">
            El rol se lee del claim <code>realm_access.roles</code> del token de acceso, que es la única fuente real: el
            gateway solo publica riesgo, trabajos y auditoría, así que no existe un endpoint de identidad al que
            preguntar. La página decodifica el payload y <strong>no verifica su firma</strong>; por eso el valor es un
            dato informativo sobre el token y no una autorización. Quien decide es el servidor, que sí verifica la
            firma: concede el riesgo con <code>viewer</code> o <code>admin</code> y las acciones de trabajo solo con{' '}
            <code>admin</code>.
          </p>
          <h4>Matriz de perfiles y permisos</h4>
          <p id="permissions-matrix-note" className="panel-note">
            Documentación verificada contra las guardas reales del servidor, no contra lo que la interfaz promete. Cada
            fila cita la guarda por <code>ruta:línea</code>:{' '}
            <code>backend/services/risk/internal/handler/security.go:43</code> acepta <code>viewer</code> o{' '}
            <code>admin</code> y <code>backend/services/jobs/internal/handler/security.go:44</code> acepta solo{' '}
            <code>admin</code>, con <code>api.go:89</code> y <code>api.go:160</code> envolviendo sus mux completos. Un
            token verificado sin ninguno de esos roles recibe 403; un token que no se puede verificar recibe 401.
          </p>
          <div className="table-wrap">
            <table>
              <caption className="sr-only">
                Matriz documentada de perfiles y permisos, con la guarda real del servidor que la sustenta
              </caption>
              <thead>
                <tr>
                  <th scope="col">Capacidad</th>
                  <th scope="col">viewer</th>
                  <th scope="col">admin</th>
                  <th scope="col">Guarda verificada (ruta:línea)</th>
                </tr>
              </thead>
              <tbody id="permissions-rows">
                {PERMISSION_MATRIX.map(entry => (
                  <tr key={entry.capability}>
                    <td>{entry.capability}</td>
                    <td>{entry.viewer}</td>
                    <td>{entry.admin}</td>
                    <td>{entry.guards.length > 0 ? entry.guards.join(' · ') : entry.note}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <p id="permissions-unimplemented" className="unimplemented-note">
            No portado a propósito: el mockup retirado permitía <em>elegir</em> un rol en la página. Ese selector no se
            implementa porque la página no puede conceder permisos y un selector sería un límite de autorización falso.
            Tampoco hay selector, conmutador ni preferencia de rol en ninguna parte de esta vista: el rol mostrado es el
            que el token declara y no altera ninguna petición, panel ni decisión.
          </p>
        </section>
      </div>
    </section>
  );
}
