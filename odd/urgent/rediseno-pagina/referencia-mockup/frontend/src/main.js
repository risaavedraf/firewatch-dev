import { createInitialState, reducer } from './core/state.js';
import { allowedRoute } from './core/router.js';
import { shell } from './components/shell.js';
import { overview } from './views/overview.js';
import { history } from './views/history.js';
import { sources } from './views/sources.js';
import { jobs } from './views/jobs.js';
import { audit } from './views/audit.js';
import { access } from './views/access.js';

// Retired mock views (models comparison and preventive actions) were removed from the router and
// navigation; every entry here must stay importable and renderable.
const views = { overview, history, sources, jobs, audit, access };
const app = document.querySelector('#app');
const feedback = document.querySelector('#feedback');
let state = { ...createInitialState(), scenario: 'future' };
let toastTimer;

function announce(message) {
  clearTimeout(toastTimer);
  feedback.textContent = message;
  feedback.classList.toggle('is-visible', Boolean(message));
  toastTimer = setTimeout(() => feedback.classList.remove('is-visible'), 7000);
}

function render() {
  const focused = document.activeElement;
  const focusId = focused?.id;
  const focusCommand = focused?.dataset.command;
  const focusItem = focused?.dataset.id;
  const selection = focused instanceof HTMLInputElement && ['search', 'text'].includes(focused.type)
    ? [focused.selectionStart, focused.selectionEnd] : null;
  // pi-lens-ignore: no-inner-html-js -- pre-existing renderer for internally generated templates: every user-derived value passes through escape() in components/ui.js and the escaping invariant is asserted adversarially in scripts/check.mjs. The lint remedy (sanitizer dependency or renderer rewrite) is outside this mockup's dependency-free scope.
  app.innerHTML = shell(state, views[state.route](state));
  if (focusId) {
    const restored = document.getElementById(focusId);
    restored?.focus({ preventScroll: true });
    if (restored && selection) restored.setSelectionRange(...selection);
  } else if (focusCommand) {
    const restored = [...app.querySelectorAll('[data-command]')].find(element => element.dataset.command === focusCommand && element.dataset.id === focusItem);
    restored?.focus({ preventScroll: true });
  }
}

function dispatch(action) {
  const previousRoute = state.route;
  state = reducer(state, action);
  if (action.type === 'RESET') state = { ...state, scenario: 'future' };
  render();
  if (location.hash !== `#${state.route}`) historyReplace(state.route);
  if (state.route !== previousRoute) document.querySelector('#content').focus({ preventScroll: true });
  if (state.feedback && !['NAVIGATE', 'SELECT_SECTOR', 'SET_FILTER', 'SELECT_HISTORY'].includes(action.type)) announce(state.feedback);
}

function historyReplace(route) {
  window.history.replaceState(null, '', `#${route}`);
}

function setMenuOpen(open) {
  const menu = document.querySelector('.menu-button');
  menu.setAttribute('aria-expanded', String(open));
  document.querySelector('.sidebar').classList.toggle('is-open', open);
  if (!open) menu.focus({ preventScroll: true });
}

document.querySelector('.skip-link').addEventListener('click', event => {
  event.preventDefault();
  if (document.querySelector('.sidebar').classList.contains('is-open')) setMenuOpen(false);
  document.querySelector('#content').focus();
});

app.addEventListener('click', event => {
  const target = event.target.closest('[data-command]');
  if (!target || target.disabled) return;
  const type = target.dataset.command, id = target.dataset.id;
  if (type === 'MENU') {
    const expanded = target.getAttribute('aria-expanded') === 'true';
    setMenuOpen(!expanded);
    return;
  }
  if (type === 'SET_SCENARIO') dispatch({ type, scenario: id });
  else if (type === 'CLOSE_HISTORY') dispatch({ type: 'SELECT_HISTORY', id: null });
  else dispatch({ type, id });
});

app.addEventListener('change', event => {
  const { control } = event.target.dataset;
  const value = event.target.value;
  if (control === 'role') dispatch({ type: 'SET_ROLE', role: value });
  if (control === 'freshness') dispatch({ type: 'SET_FILTER', key: 'freshnessFilter', value });
});

app.addEventListener('input', event => {
  if (event.target.dataset.control === 'query') dispatch({ type: 'SET_FILTER', key: 'query', value: event.target.value });
});

app.addEventListener('submit', event => {
  const form = event.target.closest('[data-form]');
  if (!form) return;
  event.preventDefault();
  if (!form.reportValidity()) return;
  dispatch({ ...Object.fromEntries(new FormData(form)), type: form.dataset.form, id: form.dataset.id });
});

window.addEventListener('hashchange', () => {
  dispatch({ type: 'NAVIGATE', route: location.hash.slice(1) });
});

document.addEventListener('keydown', event => {
  if (event.key === 'Escape' && document.querySelector('.sidebar').classList.contains('is-open')) {
    setMenuOpen(false);
  }
});

state = { ...state, route: allowedRoute(location.hash.slice(1), state.role) };
historyReplace(state.route);
render();
