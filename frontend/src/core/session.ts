// Session and identity for the browser client.
//
// This module owns exactly three things and nothing else:
//   1. the OIDC Authorization Code + PKCE S256 round trip against the issuer;
//   2. the access and ID tokens, which live in module memory only;
//   3. the role the access token DECLARES, for display only.
//
// It never talks to the gateway (that is `api.ts`), it never persists a token,
// and it never decides a permission: the server verifies the signature and is
// the only authority. The single value written to `sessionStorage` is the PKCE
// transaction (`state` + `code_verifier`), which is only useful for the length
// of the redirect and is removed at the callback and again on logout.
import { clientID, issuer, pkceTransactionKey, redirectURI } from '../config';

export interface SessionState {
  status: 'signed-out' | 'signing-in' | 'signed-in';
  // The roles the access token declares, or `null` when the token is absent,
  // unreadable or declares none. Never a guess and never a default.
  roles: string[] | null;
}

let accessToken = '';
let idToken = '';
let snapshot: SessionState = { status: 'signed-out', roles: null };
const listeners = new Set<() => void>();

function publish(next: SessionState): void {
  if (snapshot.status === next.status && sameRoles(snapshot.roles, next.roles)) return;
  snapshot = next;
  for (const listener of listeners) listener();
}

function sameRoles(left: string[] | null, right: string[] | null): boolean {
  if (left === null || right === null) return left === right;
  return left.length === right.length && left.every((role, index) => role === right[index]);
}

// A stable snapshot for `useSyncExternalStore`: the reference only changes when
// the session actually changes.
export function getSessionSnapshot(): SessionState {
  return snapshot;
}

export function subscribeSession(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function getAccessToken(): string {
  return accessToken;
}

// --- Self-contained base64url / UTF-8 decoding -----------------------------
//
// The payload decoder deliberately does NOT use `atob`, `Buffer` or
// `TextDecoder`: it is dependency-free, it rejects a non-base64url character
// instead of guessing, and it rejects a truncated or overlong UTF-8 sequence
// instead of turning it into replacement characters that would misrepresent
// what the token declares. It is the ONLY reader of the token payload in the
// app; no other module decodes `realm_access`.
const BASE64URL_ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_';

function encodeBase64URL(bytes: Uint8Array): string {
  let output = '';
  for (let index = 0; index < bytes.length; index += 3) {
    const first = bytes[index];
    const second = index + 1 < bytes.length ? bytes[index + 1] : 0;
    const third = index + 2 < bytes.length ? bytes[index + 2] : 0;
    output += BASE64URL_ALPHABET[first >> 2];
    output += BASE64URL_ALPHABET[((first & 0x03) << 4) | (second >> 4)];
    if (index + 1 < bytes.length) output += BASE64URL_ALPHABET[((second & 0x0f) << 2) | (third >> 6)];
    if (index + 2 < bytes.length) output += BASE64URL_ALPHABET[third & 0x3f];
  }
  return output;
}

function decodeBase64URL(value: string): number[] | null {
  if (typeof value !== 'string') return null;
  // Trailing padding is tolerated; an interior or non-URL character is not.
  const text = value.replace(/=+$/, '');
  const bytes: number[] = [];
  let accumulator = 0;
  let bits = 0;
  for (const character of text) {
    const index = BASE64URL_ALPHABET.indexOf(character);
    if (index < 0) return null;
    accumulator = (accumulator << 6) | index;
    bits += 6;
    if (bits >= 8) {
      bits -= 8;
      bytes.push((accumulator >> bits) & 0xff);
      accumulator &= (1 << bits) - 1;
    }
  }
  return bytes;
}

function decodeUTF8(bytes: number[]): string | null {
  let text = '';
  for (let index = 0; index < bytes.length; index += 1) {
    const byte = bytes[index];
    let code: number;
    if (byte < 0x80) code = byte;
    else if (byte >= 0xc2 && byte <= 0xdf && index + 1 < bytes.length && (bytes[index + 1] & 0xc0) === 0x80) {
      code = ((byte & 0x1f) << 6) | (bytes[index + 1] & 0x3f);
      index += 1;
    } else if (
      byte >= 0xe0 && byte <= 0xef && index + 2 < bytes.length &&
      (bytes[index + 1] & 0xc0) === 0x80 && (bytes[index + 2] & 0xc0) === 0x80
    ) {
      code = ((byte & 0x0f) << 12) | ((bytes[index + 1] & 0x3f) << 6) | (bytes[index + 2] & 0x3f);
      index += 2;
    } else if (
      byte >= 0xf0 && byte <= 0xf4 && index + 3 < bytes.length &&
      (bytes[index + 1] & 0xc0) === 0x80 && (bytes[index + 2] & 0xc0) === 0x80 && (bytes[index + 3] & 0xc0) === 0x80
    ) {
      code = ((byte & 0x07) << 18) | ((bytes[index + 1] & 0x3f) << 12) | ((bytes[index + 2] & 0x3f) << 6) | (bytes[index + 3] & 0x3f);
      index += 3;
    } else return null;
    if (code > 0x10ffff || (code >= 0xd800 && code <= 0xdfff)) return null;
    if (code > 0xffff) {
      const shifted = code - 0x10000;
      text += String.fromCharCode(0xd800 + (shifted >> 10), 0xdc00 + (shifted & 0x3ff));
    } else text += String.fromCharCode(code);
  }
  return text;
}

function decodeJWTPayload(token: string): unknown {
  if (typeof token !== 'string') return null;
  const parts = token.split('.');
  if (parts.length !== 3) return null;
  const bytes = decodeBase64URL(parts[1]);
  if (bytes === null) return null;
  const json = decodeUTF8(bytes);
  if (json === null) return null;
  try {
    return JSON.parse(json);
  } catch {
    return null;
  }
}

function record(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function nonempty(value: unknown): value is string {
  return typeof value === 'string' && value.trim() !== '';
}

// The roles the token declares, or `null` when it declares none usable. No
// default, no fallback and no inference: an absent, non-array, empty or
// non-string list is "not informed", never "viewer". The signature is NOT
// verified here — the server, which does verify it, remains the only authority.
export function decodeRoles(token: string): string[] | null {
  const payload = decodeJWTPayload(token);
  if (!record(payload)) return null;
  const realmAccess = record(payload.realm_access) ? payload.realm_access : null;
  if (!realmAccess || !Array.isArray(realmAccess.roles)) return null;
  const roles: string[] = [];
  for (const role of realmAccess.roles) {
    if (!nonempty(role)) continue;
    const name = role.trim();
    if (!roles.includes(name)) roles.push(name);
  }
  return roles.length > 0 ? roles : null;
}

// --- PKCE ------------------------------------------------------------------

function randomBase64URL(byteLength: number): string {
  const bytes = new Uint8Array(byteLength);
  crypto.getRandomValues(bytes);
  return encodeBase64URL(bytes);
}

function parseTransaction(stored: string | null): { state: string; verifier: string } | null {
  if (stored === null) return null;
  try {
    const parsed: unknown = JSON.parse(stored);
    if (!record(parsed) || !nonempty(parsed.state) || !nonempty(parsed.verifier)) return null;
    return { state: parsed.state, verifier: parsed.verifier };
  } catch {
    return null;
  }
}

export function startLogin(): void {
  publish({ status: 'signing-in', roles: null });
  void beginAuthorizationCodeFlow();
}

async function beginAuthorizationCodeFlow(): Promise<void> {
  try {
    const state = randomBase64URL(32);
    const verifier = randomBase64URL(32);
    const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(verifier));
    const challenge = encodeBase64URL(new Uint8Array(digest));
    // Written once, read and removed at the callback.
    sessionStorage.setItem(pkceTransactionKey, JSON.stringify({ state, verifier }));
    const authorization = new URL(`${issuer}/protocol/openid-connect/auth`);
    authorization.search = new URLSearchParams({
      client_id: clientID,
      redirect_uri: redirectURI,
      response_type: 'code',
      scope: 'openid',
      state,
      code_challenge: challenge,
      code_challenge_method: 'S256',
    }).toString();
    window.location.assign(authorization.href);
  } catch {
    // A browser that cannot build the challenge never reaches the provider:
    // the session stays closed instead of sending a weak or absent challenge.
    publish({ status: 'signed-out', roles: null });
  }
}

export function hasPendingCallback(search: string): boolean {
  const callback = new URLSearchParams(search);
  return ['code', 'state', 'error'].some((key) => (callback.get(key) ?? '') !== '');
}

export async function completeLogin(search: string): Promise<void> {
  const callback = new URLSearchParams(search);
  const code = callback.get('code') ?? '';
  const returnedState = callback.get('state') ?? '';
  const oauthError = callback.get('error') ?? '';
  try {
    const stored = sessionStorage.getItem(pkceTransactionKey);
    sessionStorage.removeItem(pkceTransactionKey);
    const transaction = parseTransaction(stored);
    if (oauthError !== '') throw new Error('login_rejected');
    if (code === '' || returnedState === '' || transaction === null || returnedState !== transaction.state) {
      throw new Error('login_state_invalid');
    }
    const response = await fetch(`${issuer}/protocol/openid-connect/token`, {
      method: 'POST',
      cache: 'no-store',
      credentials: 'omit',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({
        grant_type: 'authorization_code',
        client_id: clientID,
        code,
        redirect_uri: redirectURI,
        code_verifier: transaction.verifier,
      }),
    });
    if (!response.ok) throw new Error('login_token_exchange_failed');
    const tokenResponse: unknown = await response.json();
    const token = record(tokenResponse) && typeof tokenResponse.access_token === 'string' ? tokenResponse.access_token : '';
    if (token === '') throw new Error('login_token_missing');
    const returnedIDToken = record(tokenResponse) && typeof tokenResponse.id_token === 'string' ? tokenResponse.id_token : '';
    connectSession(token, returnedIDToken);
  } catch (error) {
    resetSession();
    throw error instanceof Error ? error : new Error('login_failed');
  } finally {
    // The callback query is consumed on every path: success, refusal and
    // provider error alike. The hash survives so the opened section is kept.
    window.history.replaceState(null, '', `${window.location.pathname}${window.location.hash}`);
  }
}

function connectSession(token: string, returnedIDToken: string): void {
  accessToken = token;
  idToken = returnedIDToken;
  publish({ status: 'signed-in', roles: decodeRoles(token) });
}

export function signOut(): void {
  const idTokenHint = idToken;
  sessionStorage.removeItem(pkceTransactionKey);
  // The memory copy is dropped before the browser leaves the page, so a failed
  // redirect cannot leave a usable token behind.
  resetSession();
  try {
    const logout = new URL(`${issuer}/protocol/openid-connect/logout`);
    logout.search = new URLSearchParams({
      id_token_hint: idTokenHint,
      client_id: clientID,
      post_logout_redirect_uri: redirectURI,
    }).toString();
    window.location.assign(logout.href);
  } catch {
    // Session already closed locally; there is nothing else to state.
  }
}

export function resetSession(): void {
  accessToken = '';
  idToken = '';
  publish({ status: 'signed-out', roles: null });
}
