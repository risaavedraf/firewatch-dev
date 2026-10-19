import { expect, test } from 'vitest';

import {
  evidenceDecisionLabel,
  isEvidenceDecision,
  isEvidenceReference,
  parseEvidenceReferences,
  REASSESSMENT_DISCLAIMER,
  sufficiencyCopy,
} from '../../src/domain/prevention';

// The S2 domain helpers are a transcription of the server rules for the page's
// own wording and affordances. The server remains the authority: these tests pin
// the transcription so the page never offers a value the contract lacks.

test('only the two declared decisions are recognised', () => {
  expect(isEvidenceDecision('suficiente')).toBe(true);
  expect(isEvidenceDecision('insuficiente')).toBe(true);
  for (const value of ['', 'quizas', 'SUFICIENTE', null, undefined, 3]) {
    expect(isEvidenceDecision(value)).toBe(false);
  }
  expect(evidenceDecisionLabel('suficiente')).toBe('Suficiente');
  expect(evidenceDecisionLabel('insuficiente')).toBe('Insuficiente');
  // An unknown decision is shown raw, never silently translated.
  expect(evidenceDecisionLabel('quizas')).toBe('quizas');
});

test('sufficiency copy states the server fact and never invents a decision', () => {
  const sufficient = sufficiencyCopy(true);
  const insufficient = sufficiencyCopy(false);
  expect(sufficient).not.toBe(insufficient);
  expect(sufficient).toMatch(/suficiente/i);
  expect(insufficient).toMatch(/insuficient/i);
  // The copy names the server as the authority and the acceptance consequence.
  expect(insufficient).toMatch(/aceptar|aceptaci/i);
});

test('references are one per line, trimmed and without blanks', () => {
  expect(parseEvidenceReferences('https://a.example/x\n\n  http://b.example/y  \n')).toEqual([
    'https://a.example/x',
    'http://b.example/y',
  ]);
  expect(parseEvidenceReferences('')).toEqual([]);
  expect(parseEvidenceReferences('   \n  \n')).toEqual([]);
});

test('a reference mirrors the server shape: http(s), host, no credentials, bounded', () => {
  expect(isEvidenceReference('https://example.org/acta')).toBe(true);
  expect(isEvidenceReference('http://example.org/acta?x=1#y')).toBe(true);
  for (const value of [
    '',
    '   ',
    '/acta',
    'ftp://example.org/acta',
    'javascript:alert(1)',
    'https:///acta',
    'https://user:secret@example.org/acta',
    `https://example.org/${'a'.repeat(2048)}`,
  ]) {
    expect(isEvidenceReference(value), value).toBe(false);
  }
});

test('the reassessment disclaimer says synthetic and not causal proof', () => {
  expect(REASSESSMENT_DISCLAIMER).toMatch(/sintétic/i);
  expect(REASSESSMENT_DISCLAIMER).toMatch(/no/i);
  expect(REASSESSMENT_DISCLAIMER).toMatch(/riesgo/i);
});
