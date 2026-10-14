import { render, screen } from '@testing-library/react';

import { App } from '../src/App';

// Scaffolding smoke test (N2): proves the jsdom + Testing Library + Vitest wiring
// works before any product code exists. It is replaced by the router tests in N4.
test('the application mounts a heading in the document', () => {
  render(<App />);

  expect(screen.getByRole('heading', { level: 1, name: 'FireWatch' })).toBeInTheDocument();
});
