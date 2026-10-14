export const routes = {
  overview: { label: 'Territorio', icon: 'map', group: 'Prevención' },
  history: { label: 'Historial', icon: 'history', group: 'Prevención' },
  sources: { label: 'Fuentes y calidad', icon: 'layers', group: 'Información', admin: true },
  jobs: { label: 'Actualización de información', icon: 'activity', group: 'Información', admin: true },
  audit: { label: 'Registro de actividad', icon: 'list', group: 'Coordinación', admin: true },
  access: { label: 'Perfiles y permisos', icon: 'lock', group: 'Coordinación' }
};
export function allowedRoute(route, role) {
  return Object.hasOwn(routes, route) && (!routes[route].admin || role === 'admin') ? route : 'overview';
}
