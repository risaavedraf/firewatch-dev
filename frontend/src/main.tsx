import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';

import { App } from './App';
import { bindZoneStateToSession } from './app/store';
import { resetInterruptedLogin, resumePendingLogin } from './core/session';
import './styles/tokens.css';
import './styles/app.css';

// A return from the identity provider is an ordinary page load whose query carries
// the callback. Consuming it is the whole round trip: without this call the code
// is never exchanged and the session can never be established.
void resumePendingLogin(window.location.search);

// `persisted` is true only for a back/forward-cache restore, which is exactly the
// case where a redirect that was in flight never completed. Without this, a
// restored page would keep an interrupted sign-in and its disabled control.
window.addEventListener('pageshow', (event) => {
  if (event.persisted) resetInterruptedLogin();
});

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
