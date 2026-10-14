import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';

import { App } from './App';

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
