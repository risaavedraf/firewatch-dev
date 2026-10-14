import { render, within } from '@testing-library/react';

import { NOT_IMPLEMENTED, PendingView } from '../../src/views/PendingView';

// N8 — the inventory of capabilities the retired mockup offered and the backend
// cannot back. Declaring them is part of the delivery: every entry is stated,
// never simulated, and the view must not expose a control that would offer any
// of them. The suite reads the exported list instead of a copy so the row count
// assertion can never drift from what the view actually renders.

test('the section carries id="pending", data-view="pending" and the legacy anchors', () => {
  const { container } = render(<PendingView />);

  const section = container.querySelector('section#pending') as HTMLElement | null;
  expect(section).toBeInTheDocument();
  expect(section).toHaveAttribute('data-view', 'pending');

  // These are the five identifiers the retired harness resolved by id; losing
  // any of them is a silent regression, not a cosmetic change.
  for (const id of ['pending-title', 'pending-badge', 'pending-status', 'pending-rows']) {
    expect(document.getElementById(id)).not.toBeNull();
  }
  // The panel states its own status instead of leaving the absence implicit.
  expect(document.getElementById('pending-badge')).toHaveTextContent(/NO IMPLEMENTADO/i);
});

test('every capability in the exported list renders as a row with its three columns filled', () => {
  render(<PendingView />);

  const tbody = document.getElementById('pending-rows') as HTMLElement | null;
  expect(tbody).not.toBeNull();

  const rows = tbody as HTMLElement;
  const rendered = [...rows.querySelectorAll('tr')];
  expect(rendered).toHaveLength(NOT_IMPLEMENTED.length);

  rendered.forEach((row, index) => {
    const expected = NOT_IMPLEMENTED[index];
    const cells = [...row.querySelectorAll('td')];

    expect(cells).toHaveLength(3);
    expect(cells[0]).toHaveTextContent(expected.capability);
    expect(cells[1]).toHaveTextContent(expected.reason);
    expect(cells[2]).toHaveTextContent(expected.location);

    for (const cell of cells) {
      expect(cell.textContent?.trim()).not.toBe('');
      // Plain text only: no cell wraps an action or a simulated control.
      expect(cell.children).toHaveLength(0);
    }
  });
});

test('the view offers no button, link or interactive control for any listed capability', () => {
  const { container } = render(<PendingView />);

  const view = within(container);
  expect(view.queryByRole('button')).toBeNull();
  expect(view.queryByRole('link')).toBeNull();

  const interactive = container.querySelectorAll(
    'button, a, input, select, textarea, summary, [role="button"], [role="link"], [role="checkbox"], [role="switch"], [role="tab"], [role="menuitem"]',
  );
  expect(interactive).toHaveLength(0);
});
