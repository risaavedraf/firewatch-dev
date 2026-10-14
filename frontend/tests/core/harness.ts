// Deterministic test doubles for the transport and session units. Nothing here
// touches a network, a real browser or a real identity provider: the transport
// is exercised against a stubbed `fetch`, a stubbed `sessionStorage` and a
// stubbed `crypto`, exactly as the retired VM harness did.
import { vi } from 'vitest';

export interface StubCall {
  url: string;
  init: RequestInit;
}

interface StubResult {
  status: number;
  body: unknown;
  requestId: string;
  jsonThrows: boolean;
}

function makeResponse(result: StubResult): Response {
  return {
    ok: result.status >= 200 && result.status < 300,
    status: result.status,
    headers: {
      get: (name: string) => (name.toLowerCase() === 'x-request-id' ? result.requestId : null),
    },
    json: async () => {
      if (result.jsonThrows) throw new SyntaxError('invalid json');
      return result.body;
    },
  } as unknown as Response;
}

export function createFetchStub() {
  const calls: StubCall[] = [];
  const pending: { url: string; resolve: (result: StubResult) => void }[] = [];

  const fetch = (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    const url = String(input);
    const call: StubCall = { url, init: init ?? {} };
    calls.push(call);
    return new Promise<Response>((resolve, reject) => {
      pending.push({ url, resolve: (result) => resolve(makeResponse(result)) });
      const signal = init?.signal ?? null;
      if (signal === null) return;
      if (signal.aborted) {
        reject(new DOMException('Aborted', 'AbortError'));
        return;
      }
      signal.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')), { once: true });
    });
  };

  function take(url: string): (result: StubResult) => void {
    const index = pending.findIndex((entry) => entry.url === url);
    if (index < 0) throw new Error(`no pending request for ${url}`);
    return pending.splice(index, 1)[0].resolve;
  }

  return {
    fetch,
    calls,
    answer(url: string, body: unknown, status = 200, requestId = ''): void {
      take(url)({ status, body, requestId, jsonThrows: false });
    },
    answerMalformedJson(url: string, status = 200, requestId = ''): void {
      take(url)({ status, body: null, requestId, jsonThrows: true });
    },
    callsFor(url: string): StubCall[] {
      return calls.filter((call) => call.url === url);
    },
  };
}

export function createStorageStub() {
  const store = new Map<string, string>();
  const writes: { key: string; value: string }[] = [];
  const removals: string[] = [];

  const storage = {
    getItem: (key: string): string | null => (store.has(key) ? (store.get(key) as string) : null),
    setItem: (key: string, value: string): void => {
      store.set(key, value);
      writes.push({ key, value });
    },
    removeItem: (key: string): void => {
      store.delete(key);
      removals.push(key);
    },
    clear: (): void => store.clear(),
    key: (index: number): string | null => Array.from(store.keys())[index] ?? null,
    get length(): number {
      return store.size;
    },
  };

  return {
    storage,
    writes,
    removals,
    snapshot: (): Map<string, string> => new Map(store),
  };
}

// `subtle.digest` is the real WebCrypto so the test can recompute the expected
// challenge, while `getRandomValues` is deterministic so `state` and `verifier`
// are reproducible run to run.
export function createCryptoStub(seed = 3) {
  const bytes = new Uint8Array(64);
  for (let index = 0; index < bytes.length; index += 1) bytes[index] = (seed * 37 + index * 13) % 256;
  let offset = 0;
  const digest = globalThis.crypto.subtle.digest.bind(globalThis.crypto.subtle);
  return {
    getRandomValues: (array: Uint8Array): Uint8Array => {
      for (let index = 0; index < array.length; index += 1) array[index] = bytes[(offset + index) % bytes.length];
      offset += array.length;
      return array;
    },
    subtle: {
      digest: (algorithm: AlgorithmIdentifier, data: BufferSource): Promise<ArrayBuffer> => digest(algorithm, data),
    },
  };
}

export function createWindowStub() {
  const assign = vi.fn();
  const replaceState = vi.fn();
  const location = { href: 'http://localhost:4173/', pathname: '/', hash: '', search: '', assign };
  return { window: { location, history: { replaceState } }, location, assign, replaceState };
}

export function jwt(payload: unknown): string {
  const encode = (value: unknown): string => Buffer.from(JSON.stringify(value), 'utf8').toString('base64url');
  return `${encode({ alg: 'none', typ: 'JWT' })}.${encode(payload)}.signature`;
}

export function base64url(bytes: Uint8Array): string {
  return Buffer.from(bytes).toString('base64url');
}

export async function sha256Base64url(text: string): Promise<string> {
  const digest = await globalThis.crypto.subtle.digest('SHA-256', new TextEncoder().encode(text));
  return base64url(new Uint8Array(digest));
}

export async function flush(): Promise<void> {
  for (let index = 0; index < 12; index += 1) await Promise.resolve();
}
