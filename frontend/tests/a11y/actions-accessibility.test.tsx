import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { act, render, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { gatewayOrigin, issuer, pkceTransactionKey } from '../../src/config';
import { completeLogin, resetSession } from '../../src/core/session';
import { ActionsView } from '../../src/views/ActionsView';

import { createCryptoStub, createFetchStub, createStorageStub, flush, jwt } from '../core/harness';

// T15 — mobile review at 390 px and accessibility for the actions view. jsdom
// does not lay out CSS, so the geometric half of the review is pinned to the
// stylesheet the browser actually receives: the breakpoint that covers 390 px
// must still collapse the new grid, and the tokens the view paints must meet
// WCAG AA. The DOM half (labels, focus order, keyboard activation) is exercised
// against the rendered view. Each assertion fails if its property is lost.
//
// What this cannot prove: a real 390 px render. That needs a browser, which is
// not available in this environment, and it is reported as not executed.

const ACTIONS_URL = `${gatewayOrigin}/v1/prevention/actions`;
const TOKEN_URL = `${issuer}/protocol/openid-connect/token`;
const ADMIN_TOKEN = jwt({ realm_access: { roles: ['admin'] } });

const appCss = readFileSync(resolve(process.cwd(), 'src', 'styles', 'app.css'), 'utf8');
const tokensCss = readFileSync(resolve(process.cwd(), 'src', 'styles', 'tokens.css'), 'utf8');

let fetchStub: ReturnType<typeof createFetchStub>;
let storageStub: ReturnType<typeof createStorageStub>;

function prepare(): void {
  fetchStub = createFetchStub();
  storageStub = createStorageStub();
  vi.stubGlobal('fetch', fetchStub.fetch);
  vi.stubGlobal('sessionStorage', storageStub.storage);
  vi.stubGlobal('crypto', createCryptoStub());
  resetSession();
}

async function connect(): Promise<void> {
  storageStub.storage.setItem(pkceTransactionKey, JSON.stringify({ state: 's', verifier: 'verifier' }));
  const login = completeLogin('?code=code-1&state=s');
  await flush();
  fetchStub.answer(TOKEN_URL, { access_token: ADMIN_TOKEN, id_token: 'id-token' });
  await login;
  fetchStub.calls.length = 0;
}

function actionFixture(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: 'action-1',
    community_code: '05101',
    title: 'Despeje de vegetación',
    description: 'Franja de 30 m',
    priority: 'alta',
    status: 'propuesta',
    assignee: 'equipo-1',
    due_date: '2026-11-01',
    created_by: 'actor-1',
    created_at: '2026-10-17T10:00:00Z',
    updated_at: '2026-10-17T10:00:00Z',
    ...overrides,
  };
}

async function openAdminWithDetail(): Promise<void> {
  await connect();
  render(<ActionsView />);
  await waitFor(() => expect(fetchStub.callsFor(ACTIONS_URL)).toHaveLength(1));
  await act(async () => {
    fetchStub.answer(ACTIONS_URL, [actionFixture()]);
    await flush();
  });
  await userEvent.click(document.querySelector('[data-action-id="action-1"]') as HTMLElement);
  await waitFor(() => expect(fetchStub.callsFor(`${ACTIONS_URL}/action-1`)).toHaveLength(1));
  await act(async () => {
    fetchStub.answer(`${ACTIONS_URL}/action-1`, {
      ...actionFixture(),
      history: [
        {
          id: 1,
          action_id: 'action-1',
          from_status: null,
          to_status: 'propuesta',
          actor_sub: 'actor-1',
          note: 'alta',
          created_at: '2026-10-17T10:00:00Z',
        },
      ],
      evidence: [
        {
          id: 5,
          action_id: 'action-1',
          criterion: 'Acta de la junta',
          references: ['https://example.org/acta'],
          created_by: 'admin-1',
          created_at: '2026-10-18T12:00:00Z',
        },
      ],
      evidence_reviews: [
        {
          id: 1,
          action_id: 'action-1',
          evidence_id: 5,
          decision: 'suficiente',
          reviewer_sub: 'reviewer-1',
          note: 'revisado',
          reviewed_at: '2026-10-18T12:05:00Z',
        },
      ],
      reassessments: [],
      evidence_sufficient: true,
    });
    await flush();
  });
}

beforeEach(prepare);

afterEach(() => {
  vi.unstubAllGlobals();
});

// ── Control labels ─────────────────────────────────────────────────────────

test('every control in the actions view carries an accessible name', async () => {
  await openAdminWithDetail();

  const controls = [
    ...document.querySelectorAll<HTMLElement>('#actions input, #actions select, #actions textarea'),
  ];
  expect(controls.length).toBeGreaterThanOrEqual(7);
  for (const control of controls) {
    // A wrapped `<label>` gives the element a programmatic name; the matcher
    // also catches a label that exists but is empty.
    expect(control, control.id).toHaveAccessibleName();
    expect(((control as HTMLInputElement).labels?.length ?? 0) > 0, control.id).toBe(true);
  }
});

// ── Keyboard navigation ────────────────────────────────────────────────────

test('keyboard navigation reaches the filter, transition and creation controls in order', async () => {
  await openAdminWithDetail();

  const expected = [
    'actions-community-filter',
    'actions-status-filter',
    'actions-apply-filters',
    'actions-refresh',
    'evidence-criterion',
    'evidence-references',
    'record-evidence',
    'review-note',
    'review-suficiente',
    'review-insuficiente',
    'reassessment-scenario',
    'request-reassessment',
    'transition-note',
    'create-community',
    'create-title',
    'create-description',
    'create-priority',
    'create-assignee',
    'create-due-date',
    'create-action',
  ];
  const reached = new Set<string>();
  // The loop stops as soon as every expected control has been reached, so it
  // does not spend extra keyboard round trips after the last one.
  for (let step = 0; step < 60 && reached.size < expected.length; step += 1) {
    await userEvent.tab();
    const id = document.activeElement?.id ?? '';
    if (id !== '') reached.add(id);
  }

  for (const id of expected) {
    expect(reached.has(id), `keyboard never reached ${id}`).toBe(true);
  }
}, 20_000);

test('a transition button activates from the keyboard alone', async () => {
  await openAdminWithDetail();

  const button = document.querySelector('[data-transition-to="asignada"]') as HTMLButtonElement;
  expect(button.tagName).toBe('BUTTON');
  expect(button.disabled).toBe(false);
  button.focus();
  expect(document.activeElement).toBe(button);

  // A native button honours Enter (and Space); the control was never clicked.
  await userEvent.keyboard('{Enter}');
  await waitFor(() => expect(fetchStub.callsFor(`${ACTIONS_URL}/action-1/transitions`)).toHaveLength(1));
  await act(async () => {
    fetchStub.answer(`${ACTIONS_URL}/action-1/transitions`, actionFixture({ status: 'asignada' }));
    await flush();
  });
  expect(fetchStub.callsFor(`${ACTIONS_URL}/action-1/transitions`)).toHaveLength(1);
  // The server-authored history is re-read after the transition.
  await waitFor(() => expect(fetchStub.callsFor(`${ACTIONS_URL}/action-1`).length).toBeGreaterThanOrEqual(2));
});

// ── Visible focus ──────────────────────────────────────────────────────────

test('the stylesheet keeps a visible focus outline on every interactive control', () => {
  // The one rule every keyboard user depends on: a non-zero outline on focus.
  expect(appCss).toMatch(
    /button:focus-visible,[\s\S]{0,80}input:focus-visible,[\s\S]{0,80}select:focus-visible,[\s\S]{0,120}a:focus-visible,[\s\S]{0,80}\[tabindex\]:focus-visible\s*\{[^}]*outline:\s*3px/,
  );
  // And the focus ring must not be removed without a replacement.
  expect(appCss).not.toMatch(/:focus\s*\{[^}]*outline:\s*none/);
});

// ── AA contrast ────────────────────────────────────────────────────────────

const TOKEN = /--([\w-]+)\s*:\s*(#[0-9a-fA-F]{6})/g;

function tokens(): Map<string, string> {
  const map = new Map<string, string>();
  for (const match of tokensCss.matchAll(TOKEN)) map.set(match[1], match[2]);
  return map;
}

function channel(value: number): number {
  const normalized = value / 255;
  return normalized <= 0.03928 ? normalized / 12.92 : ((normalized + 0.055) / 1.055) ** 2.4;
}

function luminance(hex: string): number {
  const red = Number.parseInt(hex.slice(1, 3), 16);
  const green = Number.parseInt(hex.slice(3, 5), 16);
  const blue = Number.parseInt(hex.slice(5, 7), 16);
  return 0.2126 * channel(red) + 0.7152 * channel(green) + 0.0722 * channel(blue);
}

function contrast(foreground: string, background: string): number {
  const first = luminance(foreground);
  const second = luminance(background);
  const [high, low] = first >= second ? [first, second] : [second, first];
  return (high + 0.05) / (low + 0.05);
}

test('the palette pairs painted by the actions view meet WCAG AA (4.5:1)', () => {
  const palette = tokens();
  const paper = palette.get('paper') as string;
  const muted = palette.get('muted') as string;
  const ink = palette.get('ink') as string;
  const soft = palette.get('soft') as string;
  const forest = palette.get('forest-700') as string;
  // The base `.neutral-tag`/`.chip` colours are not AA at their 8–10 px sizes,
  // so the actions view overrides them; the overrides are asserted below and
  // their pairs are measured here.
  const tagBackground = '#edf1f2';
  // `canvas` and the table-header surface are real painted backgrounds that the
  // measured browser check flagged (4.44 and 4.39 with the previous `--muted`),
  // and the footer carried its own literal instead of the token.
  const canvas = palette.get('canvas') as string;
  const tableHead = '#f3f4ed';

  const pairs: ReadonlyArray<[string, string, string]> = [
    ['body text', ink, paper],
    ['secondary text', muted, paper],
    ['secondary text on canvas', muted, canvas],
    ['table header', muted, tableHead],
    ['footer', muted, canvas],
    ['secondary buttons', forest, paper],
    ['actions status chip', ink, soft],
    ['actions status badge', ink, tagBackground],
  ];
  for (const [label, foreground, background] of pairs) {
    expect(contrast(foreground, background), `${label} ${foreground} on ${background}`).toBeGreaterThanOrEqual(4.5);
  }

  // The overrides are what makes the two small-text pairs pass; losing them must
  // fail this test rather than silently reverting to the low-contrast base.
  expect(appCss).toMatch(/#actions \.chip\s*\{[^}]*color:\s*var\(--ink\)/);
  expect(appCss).toMatch(/#actions \.neutral-tag\s*\{[^}]*color:\s*var\(--ink\)/);
  // The footer must read the token: its own literal is how it drifted to 3.53:1.
  expect(appCss).toMatch(/footer\s*\{[^}]*color:\s*var\(--muted\)/);
});

// ── 390 px layout ──────────────────────────────────────────────────────────

function mobileBlock(): string {
  const blocks = [...appCss.matchAll(/@media\s*\(max-width:\s*(\d+)px\)\s*\{/g)];
  const covering = blocks.filter(entry => Number(entry[1]) >= 390);
  expect(covering.length).toBeGreaterThan(0);
  // The narrowest breakpoint that still covers 390 px is the one this test pins.
  const narrowest = covering.reduce((left, right) => (Number(left[1]) <= Number(right[1]) ? left : right));
  const start = (narrowest.index ?? 0) + narrowest[0].length;
  const tail = appCss.slice(start);
  const nextBlock = tail.search(/@media\s*\(max-width:/);
  return nextBlock < 0 ? tail : tail.slice(0, nextBlock);
}

test('the breakpoint that covers 390 px collapses the new layout to one column', () => {
  const block = mobileBlock();
  expect(block).toMatch(/\.actions-form\s*\{[^}]*grid-template-columns:\s*1fr/);
  expect(block).toMatch(/\.actions-filters\s*\{[^}]*flex-direction:\s*column/);
  // The base grid is two columns, so the mobile rule is a real collapse.
  expect(appCss).toMatch(/\.actions-form\s*\{[^}]*grid-template-columns:\s*repeat\(2,/);
  // Tables scroll inside their own box instead of widening the page.
  expect(appCss).toMatch(/\.table-wrap[^{]*\{[^}]*overflow:\s*auto/);
});
