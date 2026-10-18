import { describe, expect, test } from 'vitest';

import {
  ACTION_STATUSES,
  ACTION_TERMINAL_STATUSES,
  actionStatusLabel,
  declaresAdminRole,
  isActionStatus,
  isTransitionAllowed,
  nextStatuses,
  transitionAffordances,
} from '../../src/domain/prevention';

// S3 — the preventive-action state machine, transcribed from the frozen
// contract in `odd/tasks/presentation-client-ready.md` §«Contrato congelado de
// prevention». The map is the single place the client claims to know the valid
// transitions; the server remains the authority and a `409 invalid_transition`
// is still rendered when the two disagree.

describe('prevention status vocabulary', () => {
  test('declares every status of the frozen contract exactly once', () => {
    expect(ACTION_STATUSES).toEqual([
      'propuesta',
      'asignada',
      'en_curso',
      'bloqueada',
      'enviada',
      'aceptada',
      'devuelta',
      'cancelada',
    ]);
    expect(new Set(ACTION_STATUSES).size).toBe(ACTION_STATUSES.length);
    expect(ACTION_TERMINAL_STATUSES).toEqual(['aceptada', 'cancelada']);
  });

  test('recognises only declared statuses', () => {
    expect(isActionStatus('en_curso')).toBe(true);
    expect(isActionStatus('EN_CURSO')).toBe(false);
    expect(isActionStatus('')).toBe(false);
    expect(isActionStatus(42)).toBe(false);
  });

  test('gives every declared status a human label and labels nothing else', () => {
    expect(actionStatusLabel('propuesta')).toBe('Propuesta');
    expect(actionStatusLabel('en_curso')).toBe('En curso');
    expect(actionStatusLabel('aceptada')).toBe('Aceptada');
    expect(actionStatusLabel('cancelada')).toBe('Cancelada');
    // An unknown status is shown raw rather than silently translated.
    expect(actionStatusLabel('inventado')).toBe('inventado');
  });
});

describe('prevention transitions', () => {
  test('follows the frozen chain from propuesta to aceptada', () => {
    expect(nextStatuses('propuesta')).toEqual(['asignada', 'cancelada']);
    expect(nextStatuses('asignada')).toEqual(['en_curso', 'cancelada']);
    expect(nextStatuses('en_curso')).toEqual(['bloqueada', 'enviada', 'cancelada']);
    expect(nextStatuses('bloqueada')).toEqual(['en_curso', 'cancelada']);
    expect(nextStatuses('enviada')).toEqual(['aceptada', 'devuelta', 'cancelada']);
    expect(nextStatuses('devuelta')).toEqual(['en_curso', 'cancelada']);
  });

  test('treats aceptada and cancelada as terminal', () => {
    expect(nextStatuses('aceptada')).toEqual([]);
    expect(nextStatuses('cancelada')).toEqual([]);
  });

  test('offers nothing for a status the contract does not declare', () => {
    expect(nextStatuses('inventado')).toEqual([]);
    expect(nextStatuses('')).toEqual([]);
  });

  test('rejects every transition the chain does not state', () => {
    expect(isTransitionAllowed('propuesta', 'en_curso')).toBe(false);
    expect(isTransitionAllowed('propuesta', 'aceptada')).toBe(false);
    expect(isTransitionAllowed('enviada', 'en_curso')).toBe(false);
    expect(isTransitionAllowed('aceptada', 'en_curso')).toBe(false);
    expect(isTransitionAllowed('cancelada', 'en_curso')).toBe(false);
    expect(isTransitionAllowed('propuesta', 'inventado')).toBe(false);
  });

  test('accepts the transitions the chain states, including reopen and cancellation', () => {
    expect(isTransitionAllowed('propuesta', 'asignada')).toBe(true);
    expect(isTransitionAllowed('en_curso', 'bloqueada')).toBe(true);
    expect(isTransitionAllowed('bloqueada', 'en_curso')).toBe(true);
    expect(isTransitionAllowed('devuelta', 'en_curso')).toBe(true);
    expect(isTransitionAllowed('enviada', 'devuelta')).toBe(true);
    expect(isTransitionAllowed('enviada', 'aceptada')).toBe(true);
    expect(isTransitionAllowed('bloqueada', 'cancelada')).toBe(true);
  });
});

describe('prevention role affordance', () => {
  test('reads an admin only from a declared role list', () => {
    expect(declaresAdminRole(['admin'])).toBe(true);
    expect(declaresAdminRole(['viewer', 'admin'])).toBe(true);
    expect(declaresAdminRole(['viewer'])).toBe(false);
    // A token that declares no usable role is never assumed to be admin.
    expect(declaresAdminRole(null)).toBe(false);
    expect(declaresAdminRole([])).toBe(false);
    expect(declaresAdminRole(undefined)).toBe(false);
  });

  test('enables every offered transition only for a signed-in admin', () => {
    const admin = transitionAffordances('propuesta', ['admin'], true);
    expect(admin.map(item => item.to)).toEqual(['asignada', 'cancelada']);
    expect(admin.every(item => item.enabled)).toBe(true);

    const viewer = transitionAffordances('propuesta', ['viewer'], true);
    expect(viewer.map(item => item.to)).toEqual(['asignada', 'cancelada']);
    expect(viewer.every(item => item.enabled)).toBe(false);

    const unknown = transitionAffordances('propuesta', null, true);
    expect(unknown.every(item => item.enabled)).toBe(false);

    const signedOut = transitionAffordances('propuesta', ['admin'], false);
    expect(signedOut.every(item => item.enabled)).toBe(false);
  });
});
