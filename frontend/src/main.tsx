import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';

import { App } from './App';
import { bindZoneStateToSession } from './app/store';
import './styles/tokens.css';
import './styles/app.css';

// Installed before the first render: whenever the session ends — logout, a 401 or
// a refused callback — the private view is emptied, so no view can keep showing
// the previous session's zone or colours.
bindZoneStateToSession();

const container = document.getElementById('root');

if (!container) {
  // A missing mount point is a build defect, not a runtime condition the app can
  // repair: failing loudly beats mounting into nothing and rendering a blank page.
  throw new Error('FireWatch: #root mount point is missing from index.html');
}

createRoot(container).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
