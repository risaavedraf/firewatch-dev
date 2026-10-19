import { readdirSync, readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { render, waitFor } from '@testing-library/react';

import { routes } from '../../src/app/routes';
import { Shell } from '../../src/components/Shell';
import { gatewayOrigin } from '../../src/config';
import { resetSession } from '../../src/core/session';
import { ActionsView } from '../../src/views/ActionsView';
import { AuditView } from '../../src/views/AuditView';
import { HistoryView } from '../../src/views/HistoryView';
import { JobsView } from '../../src/views/JobsView';
import { PendingView } from '../../src/views/PendingView';

import { createCryptoStub, createFetchStub } from '../core/harness';

// Three layout defects reached a browser because no check owned them: the
// sidebar stopped 34 px above the viewport floor (the height offset of the
// demonstration strip, which the shell no longer renders), the sidebar
// workspace block was rendered with no rule of its own, and two panels
// (`model-panel` in the overview, `cycle-panel` in sources) were added to
// markup that no stylesheet rule padded, so their body text touched — and was
// clipped by — the panel border.
//
// jsdom has no layout engine, so the geometry is not measurable here. What is
// pinned is the shipped stylesheet the browser receives plus the markup that
// consumes it: every rule group below is read from `src/styles/app.css`.
const appCss = readFileSync(resolve(process.cwd(), 'src', 'styles', 'app.css'), 'utf8');

interface CssRule {
  selector: string;
  body: string;
  // The at-rule this rule lives inside, or null for a top-level rule. The
  // responsive rules are excluded from the checks below: a panel must carry its
  // padding in the base layout, not only below a breakpoint.
  atRule: string | null;
  declarations: string[];
}

// A small brace walker instead of a CSS parser dependency: the file is flat
// enough to read, and every assertion below fails loudly if this walk stops
// recognising the rules it is asked about.
function parseRules(css: string): CssRule[] {
  const text = css.replace(/\/\*[\s\S]*?\*\//g, '');
  const rules: CssRule[] = [];
  const open: Array<string | null> = [];
  let head = '';
  for (const character of text) {
    if (character === '{') {
      const selector = head.trim();
      head = '';
      if (selector.startsWith('@')) {
        open.push(selector);
        continue;
      }
      rules.push({
        selector,
        body: '',
        atRule: open.find(entry => entry !== null) ?? null,
        declarations: [],
      });
      open.push(null);
      continue;
    }
    if (character === '}') {
      const closed = open.pop();
      if (closed === null) rules[rules.length - 1].declarations = rules[rules.length - 1].body.split(';');
      continue;
    }
    if (open.includes(null)) {
      rules[rules.length - 1].body += character;
      continue;
    }
    head += character;
  }
  return rules;
}

const rules = parseRules(appCss);

function baseRules(selector: string): CssRule[] {
  return rules.filter(
    rule =>
      rule.atRule === null &&
      rule.selector
        .split(',')
        .map(part => part.trim().replace(/\s+/g, ' '))
        .includes(selector),
  );
}

function baseRule(selector: string): CssRule {
  const found = baseRules(selector);
  if (found.length !== 1) throw new Error(`expected exactly one top-level rule for ${selector}, found ${found.length}`);
  return found[0];
}

function declaration(rule: CssRule, property: string): string | null {
  const declared = rule.declarations.map(part => part.trim()).find(part => part.startsWith(`${property}:`));
  return declared === null || declared === undefined ? null : declared.slice(property.length + 1).trim();
}

// Every `panel <modifier>` the shipped views render. The modifier is read from a
// plain `className="…"` and from a template literal that appends a state class —
// `className={`panel history-panel${hasRows ? ' has-rows' : ''}`}` — because a
// conditional state class must not hide the modifier from this scan.
function panelModifiers(): string[] {
  const sources: string[] = [];
  const walk = (directory: string): void => {
    for (const entry of readdirSync(directory, { withFileTypes: true })) {
      const path = resolve(directory, entry.name);
      if (entry.isDirectory()) walk(path);
      else if (entry.name.endsWith('.tsx')) sources.push(readFileSync(path, 'utf8'));
    }
  };
  walk(resolve(process.cwd(), 'src'));

  const modifiers = new Set<string>();
  for (const source of sources) {
    for (const match of source.matchAll(/className=(?:"([^"]*)"|\{`([^`]*)`\})/g)) {
      const value = (match[1] ?? match[2] ?? '').replace(/\$\{[^}]*\}/g, ' ');
      const tokens = value.split(/\s+/).filter(Boolean);
      const panel = tokens.indexOf('panel');
      const modifier = panel === -1 ? undefined : tokens[panel + 1];
      if (modifier !== undefined) modifiers.add(modifier);
    }
  }
  return [...modifiers].sort();
}

test('the shell and the sidebar fill the viewport, so the sidebar paints no blank band (S1)', () => {
  // The retired demonstration strip was a 34 px band above the shell. It is no
  // longer rendered, so every offset that reserved its height is a hole.
  expect(appCss).not.toMatch(/100vh\s*-\s*34px/);

  const shell = baseRule('.app-shell');
  expect(declaration(shell, 'min-height')).toBe('100vh');

  const sidebar = baseRule('.sidebar');
  expect(declaration(sidebar, 'height')).toBe('100vh');
  expect(declaration(sidebar, 'position')).toBe('sticky');
});

test('the sidebar workspace block keeps the boxed rule set it was designed with (S2)', async () => {
  const fetchStub = createFetchStub();
  vi.stubGlobal('fetch', fetchStub.fetch);
  vi.stubGlobal('crypto', createCryptoStub());
  window.sessionStorage.clear();
  resetSession();

  const { container } = render(
    <Shell route={routes[0]}>
      <p>contenido</p>
    </Shell>,
  );
  fetchStub.answer(`${gatewayOrigin}/health/ready`, null, 204);
  await waitFor(() => expect(fetchStub.callsFor(`${gatewayOrigin}/health/ready`)).toHaveLength(1));

  // The markup and the rules are asserted together: one without the other is the
  // defect this test exists for.
  expect(container.querySelector('.workspace')).toBeInTheDocument();
  expect(container.querySelector('.workspace-mark')).toBeInTheDocument();

  const workspace = baseRule('.workspace');
  expect(declaration(workspace, 'display')).toBe('flex');
  expect(declaration(workspace, 'align-items')).toBe('center');
  expect(declaration(workspace, 'padding')).toBe('13px 10px');
  expect(declaration(workspace, 'border')).toMatch(/^1px solid/);
  expect(declaration(workspace, 'border-radius')).toBe('9px');

  const mark = baseRule('.workspace-mark');
  expect(declaration(mark, 'border-radius')).toBe('7px');
  expect(declaration(mark, 'background')).toBeTruthy();

  // The trailing map icon is pushed to the end of the row; without this it wraps
  // onto its own line, which is how the block was reported.
  expect(declaration(baseRule('.workspace > .icon'), 'margin-left')).toBe('auto');
});

test('every panel a view renders declares its own padding (S3)', () => {
  const modifiers = panelModifiers();
  expect(modifiers.length).toBeGreaterThan(0);

  const unpadded = modifiers.filter(
    modifier =>
      !baseRules(`.${modifier}`).some(rule => rule.declarations.some(part => part.trim().startsWith('padding:'))),
  );

  expect(unpadded).toEqual([]);
});

// The five views that painted their body straight onto the canvas. The
// stylesheet already declared a modifier for four of them; what the markup owes
// is the card. The heading belongs inside the card when the view states its
// title with a `panel-heading`, and stays outside when the view states it in an
// `intro-row` above the body.
const FLAT_VIEWS = [
  {
    modifier: 'history-panel',
    element: <HistoryView />,
    inside: ['history-query', 'history-state', 'history-rows'],
    outside: ['history-title'],
  },
  {
    modifier: 'audit-panel',
    element: <AuditView />,
    inside: ['audit-title', 'audit-status', 'audit-table', 'audit-count'],
    outside: [],
  },
  {
    modifier: 'jobs-panel',
    element: <JobsView />,
    inside: ['jobs-title', 'scenario-select', 'job-status', 'create-job'],
    outside: [],
  },
  {
    modifier: 'pending-panel',
    element: <PendingView />,
    inside: ['pending-title', 'pending-status', 'pending-rows'],
    outside: [],
  },
  {
    modifier: 'actions-panel',
    element: <ActionsView />,
    inside: ['actions-title', 'actions-community-filter', 'actions-table', 'actions-rows'],
    outside: [],
  },
];

test('the five flat views render their body inside the panel card their stylesheet pads (S3)', () => {
  vi.stubGlobal('fetch', createFetchStub().fetch);
  vi.stubGlobal('crypto', createCryptoStub());
  window.sessionStorage.clear();
  resetSession();

  for (const view of FLAT_VIEWS) {
    const { container } = render(view.element);
    const panel = container.querySelector(`article.panel.${view.modifier}`);
    expect(panel, `${view.modifier} must be the article that wraps the body`).not.toBeNull();

    for (const id of view.inside) {
      expect(panel?.querySelector(`#${id}`), `${view.modifier} must contain #${id}`).not.toBeNull();
    }
    // The eyebrow and the `h2` of an `intro-row` are the view's page heading, so
    // they stay outside the card the way the overview renders them.
    for (const id of view.outside) {
      expect(panel?.querySelector(`#${id}`), `${view.modifier} must not contain #${id}`).toBeNull();
      expect(container.querySelector(`#${id}`), `#${id} must still be rendered`).not.toBeNull();
    }

    // The card and the padded modifier travel together: a card whose modifier
    // is missing from the shared padding rule renders its body flush.
    expect(
      baseRules(`.${view.modifier}`).some(rule =>
        rule.declarations.some(part => part.trim().startsWith('padding:')),
      ),
      `${view.modifier} must declare a padding`,
    ).toBe(true);
  }

  // The `has-rows` class the history card states conditionally only means
  // anything while the rule that reads it exists, so both halves are pinned here.
  expect(appCss).toMatch(
    /\.history-panel:not\(\.has-rows\)\s*\.table-wrap\s*,\s*\.history-panel:not\(\.has-rows\)\s*\.filter-bar\s*\{\s*display:\s*none/,
  );

  // The actions list is one card; the detail and the creation keep their own.
  const actions = render(<ActionsView />);
  const listPanel = actions.container.querySelector('article.panel.actions-panel');
  expect(listPanel?.querySelector('article.actions-create')).toBeNull();
  expect(actions.container.querySelector('article.panel.actions-create')).not.toBeNull();
});
