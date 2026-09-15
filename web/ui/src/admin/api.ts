export const csrf = document.querySelector<HTMLMetaElement>('meta[name="csrf-token"]')?.content ?? "";

export type APILocale = "en-US" | "zh-TW";

let requestLocale: APILocale = document.documentElement.lang.toLowerCase().startsWith("zh")
  ? "zh-TW"
  : "en-US";

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

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set("Accept-Language", requestLocale);
  if (init.body && !(init.body instanceof FormData) && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  if (init.method && init.method !== "GET" && init.method !== "HEAD") {
    headers.set("X-CSRF-Token", csrf);
  }
  const response = await fetch(path, { ...init, headers });
  if (!response.ok) {
    const body = await response.text();
    throw errorFromResponse(body, response);
  }
  if (response.status === 204) return undefined as T;
  return (await response.json()) as T;
}

export function postJSON<T>(path: string, body: unknown): Promise<T> {
  return api<T>(path, { method: "POST", body: JSON.stringify(body) });
}

export function putJSON<T>(path: string, body: unknown): Promise<T> {
  return api<T>(path, { method: "PUT", body: JSON.stringify(body) });
}

export async function downloadFile(path: string, filename: string): Promise<void> {
  const response = await fetch(path, {
    method: "POST",
    headers: { "Accept-Language": requestLocale, "X-CSRF-Token": csrf },
  });
  if (!response.ok) {
    const body = await response.text();
    throw errorFromResponse(body, response);
  }

  const url = URL.createObjectURL(await response.blob());
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
