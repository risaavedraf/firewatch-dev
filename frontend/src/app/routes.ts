import type { IconName } from '../components/ui';

// The seven views of the application. The nine areas of the retired page are all
// still here: `#sources` also carries the dependent-services status and
// `#session` also carries the permissions matrix, which is the grouping the user
// chose (D1). `#pending` gained the navigation entry it never had (D2): the
// "not implemented yet" inventory is exactly the kind of honesty statement that
// must not depend on someone scrolling to the bottom of the page.
export type ViewId = 'overview' | 'history' | 'sources' | 'jobs' | 'session' | 'pending' | 'audit';

export interface RouteDefinition {
  id: ViewId;
  hash: `#${ViewId}`;
  label: string;
  group: string;
  icon: IconName;
  // Breadcrumb and document-title copy, kept beside the route so a view cannot
  // disagree with the place the navigation says it is.
  title: string;
}

export const routes: readonly RouteDefinition[] = [
  { id: 'overview', hash: '#overview', label: 'Territorio', group: 'Prevención', icon: 'territory', title: 'Estado de la zona' },
  { id: 'history', hash: '#history', label: 'Historial de lotes', group: 'Prevención', icon: 'history', title: 'Historial de lotes' },
  { id: 'sources', hash: '#sources', label: 'Fuentes y servicios', group: 'Información', icon: 'sources', title: 'Fuentes y servicios' },
  { id: 'jobs', hash: '#jobs', label: 'Procesamiento', group: 'Información', icon: 'jobs', title: 'Procesamiento asíncrono' },
  { id: 'pending', hash: '#pending', label: 'Aún no implementado', group: 'Información', icon: 'pending', title: 'Aún no implementado' },
  { id: 'session', hash: '#session', label: 'Sesión y permisos', group: 'Acceso', icon: 'session', title: 'Sesión y permisos' },
  { id: 'audit', hash: '#audit', label: 'Registro de actividad', group: 'Coordinación', icon: 'audit', title: 'Registro de actividad' },
];

export const defaultRoute: RouteDefinition = routes[0];
export const navigationGroups: readonly string[] = [...new Set(routes.map(route => route.group))];

// Routing is hash-based on purpose. The served page is one document and the
// server deliberately has no SPA fallback, so a hash keeps every view linkable
// and reloadable without inventing a server rewrite rule; it also keeps the
// Keycloak redirect URI valid for whichever view is open.
export function routeFromHash(hash: string): RouteDefinition {
  return routes.find(route => route.hash === hash) ?? defaultRoute;
}
