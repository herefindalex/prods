import { normalizeAdminLocale, type AdminLocale } from "./locales";

export const csrf = document.querySelector<HTMLMetaElement>('meta[name="csrf-token"]')?.content ?? "";

export type APILocale = AdminLocale;

let requestLocale: APILocale = normalizeAdminLocale(document.documentElement.lang);
let generation = 0;
let sessionExpired = false;
let sessionScope: string | undefined;
const pending = new Set<AbortController>();
const sessionListeners = new Set<() => void>();

export function bindSessionScope(scope: string): void {
  if (sessionScope && sessionScope !== scope) {
    expireSession();
    throw new APIError("Session changed. Sign in again.", 401);
  }
  sessionScope = scope;
}

export function onSessionExpired(listener: () => void): () => void {
  sessionListeners.add(listener);
  return () => { sessionListeners.delete(listener); };
}

export function expireSession(): void {
  if (sessionExpired) return;
  sessionExpired = true;
  generation++;
  for (const controller of pending) controller.abort();
  pending.clear();
  for (const listener of sessionListeners) listener();
}

export function setAPILocale(locale: APILocale): void {
  requestLocale = locale;
}

export class APIError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code?: string,
  ) {
    super(message);
  }

  get statusCode(): number { return this.status; }
}

function errorFromResponse(body: string, response: Response): APIError {
  let message = body.trim() || `${response.status} ${response.statusText}`;
  let code: string | undefined;
  try {
    const parsed = JSON.parse(body) as { code?: string; error?: string };
    if (parsed.error) message = parsed.error;
    if (parsed.code) code = parsed.code;
  } catch {
    // Legacy plain-text errors remain readable during the contract migration.
  }
  return new APIError(message, response.status, code);
}

async function request<T>(path: string, init: RequestInit, read: (response: Response) => Promise<T>): Promise<T> {
  if (!path.startsWith("/admin/") || path.includes("\\")) throw new APIError("Unsupported Admin endpoint", 400);
  if (sessionExpired) throw new APIError("Session expired. Sign in again.", 401);
  const requestGeneration = generation;
  const controller = new AbortController();
  pending.add(controller);
  const headers = new Headers(init.headers);
  if (!headers.has("Accept")) headers.set("Accept", "application/json");
  if (sessionScope) headers.set("X-Prods-Session-Scope", sessionScope);
  headers.set("Accept-Language", requestLocale);
  if (init.body && !(init.body instanceof FormData) && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  if (init.method && init.method !== "GET" && init.method !== "HEAD") {
    headers.set("X-CSRF-Token", csrf);
  }
  try {
    const signal = init.signal ? AbortSignal.any([init.signal, controller.signal]) : controller.signal;
    const response = await fetch(path, { ...init, headers, signal, credentials: "same-origin", cache: "no-store" });
    if (requestGeneration !== generation) throw new APIError("Session changed", 401);
    if (!response.ok) {
      const body = await response.text();
      if (requestGeneration !== generation) throw new APIError("Session changed", 401);
      if (response.status === 401) expireSession();
      throw errorFromResponse(body, response);
    }
    const value = await read(response);
    if (requestGeneration !== generation) throw new APIError("Session changed", 401);
    return value;
  } catch (error) {
    if (error instanceof APIError) throw error;
    if (controller.signal.aborted || init.signal?.aborted) throw error;
    const writing = init.method && !["GET", "HEAD"].includes(init.method.toUpperCase());
    throw new APIError(writing
      ? "The operation result is unknown. Check the saved record or operation receipt before retrying."
      : "Unable to load data. Try refreshing.", 0, writing ? "outcome_unknown" : "network_error");
  } finally {
    pending.delete(controller);
  }
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  return request(path, init, async (response) => response.status === 204 ? undefined as T : await response.json() as T);
}

export async function apiText(path: string): Promise<string> {
  return request(path, { headers: { Accept: "text/plain" } }, (response) => response.text());
}

export function postJSON<T>(path: string, body: unknown): Promise<T> {
  return api<T>(path, { method: "POST", body: JSON.stringify(body) });
}

export function putJSON<T>(path: string, body: unknown): Promise<T> {
  return api<T>(path, { method: "PUT", body: JSON.stringify(body) });
}

export async function downloadFile(path: string, filename: string): Promise<void> {
  const blob = await request(path, { method: "POST" }, (response) => response.blob());
  const url = URL.createObjectURL(blob);
  try {
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = filename;
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
  } finally {
    URL.revokeObjectURL(url);
  }
}

export function clientID(prefix: string): string {
  return `${prefix}_${crypto.randomUUID().replaceAll("-", "")}`;
}
