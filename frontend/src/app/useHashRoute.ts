import { useEffect, useState } from 'react';

import { defaultRoute, routeFromHash, type RouteDefinition } from './routes';

function currentHash(): string {
  return window.location.hash;
}

// Reads the view out of the address bar and keeps it in step with the browser's
// back/forward buttons. A hash nobody declared is replaced — in the address bar
// and in what is rendered — by the default view, so a stale or mistyped link
// lands somewhere true instead of on a blank page.
export function useHashRoute(): RouteDefinition {
  const [route, setRoute] = useState<RouteDefinition>(() => routeFromHash(currentHash()));

  useEffect(() => {
    const sync = () => {
      const next = routeFromHash(currentHash());
      setRoute(next);
      if (currentHash() !== next.hash) {
        window.history.replaceState(null, '', `${window.location.pathname}${window.location.search}${next.hash}`);
      }
    };
    sync();
    window.addEventListener('hashchange', sync);
    return () => window.removeEventListener('hashchange', sync);
  }, []);

  return route;
}

export { defaultRoute };
