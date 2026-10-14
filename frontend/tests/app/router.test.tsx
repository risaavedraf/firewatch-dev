import { act, render, screen } from '@testing-library/react';

import { App } from '../../src/App';
import { routes } from '../../src/app/routes';

// N4 — the nine real areas are now seven views, and exactly one of them is
// rendered at a time. The retired page stacked all nine sections in one scroll,
// which is the symptom the user reported ("puedo scrolleando ver toda la info,
// no por separado").

function navigate(hash: string) {
  window.location.hash = hash;
  act(() => {
    window.dispatchEvent(new Event('hashchange'));
  });
}

function renderedViews(): string[] {
  return [...document.querySelectorAll('[data-view]')].map(node => node.getAttribute('data-view') ?? '');
}

beforeEach(() => {
  window.location.hash = '';
});

test('the seven views are declared once, with the two merged pairs and no duplicate hash', () => {
  const ids = routes.map(route => route.id);
  // Declared in navigation order: the four groups are Prevención, Información,
  // Acceso and Coordinación, so `#pending` (Información) precedes `#session`
  // (Acceso) and the sidebar order is the declared order.
  expect(ids).toEqual(['overview', 'history', 'sources', 'jobs', 'pending', 'session', 'audit']);
  expect(new Set(ids).size).toBe(ids.length);
  // `#session` carries the permissions matrix and `#sources` carries the pipeline
  // status: the two merged pairs must not survive as their own routes, or the
  // merge would have silently dropped them instead of grouping them.
  expect(ids).not.toContain('permissions');
  expect(ids).not.toContain('pipeline');
});

test('exactly one view is rendered at a time and the default view is the territory', () => {
  render(<App />);

  expect(renderedViews()).toEqual(['overview']);
});

test('an unknown or empty hash normalises to the default view instead of rendering nothing', () => {
  render(<App />);

  navigate('#not-a-real-view');
  expect(renderedViews()).toEqual(['overview']);

  navigate('');
  expect(renderedViews()).toEqual(['overview']);
});

test('every declared view is reachable by its hash and only that one is rendered', () => {
  render(<App />);

  for (const route of routes) {
    navigate(route.hash);
    expect(renderedViews()).toEqual([route.id]);
  }
});

test('the navigation offers every view, including the "not implemented yet" inventory that had no entry', () => {
  render(<App />);

  for (const route of routes) {
    const link = screen.getByRole('link', { name: new RegExp(route.label, 'i') });
    expect(link).toHaveAttribute('href', route.hash);
  }
  // The inventory is the reason this test exists: it was reachable only by
  // scrolling before, and the honesty rule requires it to be visible on its own.
  expect(screen.getByRole('link', { name: /aún no implementado/i })).toHaveAttribute('href', '#pending');
});

test('the active navigation entry follows the opened view and marks nothing else', () => {
  render(<App />);

  navigate('#audit');
  const active = [...document.querySelectorAll('nav a[aria-current="true"]')];
  expect(active).toHaveLength(1);
  expect(active[0]).toHaveAttribute('href', '#audit');
});
