import { useEffect, type ReactElement } from 'react';

import { useHashRoute } from './app/useHashRoute';
import { Shell } from './components/Shell';
import type { ViewId } from './app/routes';
import { ActionsView } from './views/ActionsView';
import { AuditView } from './views/AuditView';
import { HistoryView } from './views/HistoryView';
import { JobsView } from './views/JobsView';
import { OverviewView } from './views/OverviewView';
import { PendingView } from './views/PendingView';
import { SessionView } from './views/SessionView';
import { SourcesView } from './views/SourcesView';

const views: Record<ViewId, () => ReactElement> = {
  overview: OverviewView,
  history: HistoryView,
  actions: ActionsView,
  sources: SourcesView,
  jobs: JobsView,
  session: SessionView,
  pending: PendingView,
  audit: AuditView,
};

export function App() {
  const route = useHashRoute();

  useEffect(() => {
    document.title = `${route.title} | FireWatch`;
  }, [route.title]);

  const View = views[route.id];

  return (
    <Shell route={route}>
      <View />
    </Shell>
  );
}
