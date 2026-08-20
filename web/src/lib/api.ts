export type ApiError = { code: string; message: string; details?: Record<string, unknown> };

export class ApiException extends Error {
  constructor(
    public code: string,
    message: string,
    // Structured details from error envelopes — e.g. inventory_unresolved
    // carries { unresolved: string[] }. Undefined when the server sent none.
    public details?: Record<string, unknown>,
  ) {
    super(message);
    this.name = 'ApiException';
  }
}

/** A response body plus its headers — used when a caller needs response
 *  metadata (e.g. the X-Qac-Degraded fixture-data flag). */
export type ApiResult<T> = { data: T; headers: Headers };

async function request<T>(path: string, init: RequestInit): Promise<ApiResult<T>> {
  let res: Response;
  try {
    res = await fetch(`/api${path}`, init);
  } catch (e) {
    throw new ApiException('internal', e instanceof Error ? e.message : 'Network error');
  }

  let body: unknown = null;
  try {
    body = await res.json();
  } catch {
    // Response wasn't JSON; fall through to error handling.
  }

  if (!res.ok) {
    const err = (body as { error?: ApiError } | null)?.error;
    throw new ApiException(err?.code ?? 'internal', err?.message ?? 'Request failed', err?.details);
  }
  return { data: body as T, headers: res.headers };
}

/** GET that returns the parsed body. Unchanged signature — existing call
 *  sites keep working. */
export async function apiGet<T>(path: string): Promise<T> {
  const { data } = await request<T>(path, { headers: { Accept: 'application/json' } });
  return data;
}

/** GET that also exposes response headers (e.g. X-Qac-Degraded). */
export async function apiGetWithMeta<T>(path: string): Promise<ApiResult<T>> {
  return request<T>(path, { headers: { Accept: 'application/json' } });
}

/** POST that returns the parsed body. Unchanged signature. Throws
 *  ApiException (now carrying `details`) on non-2xx. */
export async function apiPost<TRes, TBody>(path: string, body: TBody): Promise<TRes> {
  const { data } = await request<TRes>(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
    body: JSON.stringify(body),
  });
  return data;
}
