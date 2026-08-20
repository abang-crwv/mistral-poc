import { describe, it, expect } from 'vitest';
import { http, HttpResponse } from 'msw';
import { server } from '@/mocks/server';
import { apiGet, apiPost, ApiException } from './api';

describe('apiGet', () => {
  it('returns the parsed body on success', async () => {
    server.use(http.get('/api/test', () => HttpResponse.json({ value: 42 })));
    const result = await apiGet<{ value: number }>('/test');
    expect(result.value).toBe(42);
  });

  it('throws ApiException with code and message on 404 envelope', async () => {
    server.use(
      http.get('/api/missing', () =>
        HttpResponse.json(
          { error: { code: 'not_found', message: 'Run not found' } },
          { status: 404 },
        ),
      ),
    );
    await expect(apiGet('/missing')).rejects.toMatchObject({
      code: 'not_found',
      message: 'Run not found',
    });
    await expect(apiGet('/missing')).rejects.toBeInstanceOf(ApiException);
  });

  it('throws ApiException with internal code on 500 without envelope', async () => {
    server.use(http.get('/api/broken', () => new HttpResponse(null, { status: 500 })));
    await expect(apiGet('/broken')).rejects.toMatchObject({
      code: 'internal',
    });
  });
});

describe('apiPost', () => {
  it('returns the parsed body on success', async () => {
    server.use(
      http.post('/api/echo', async ({ request }) => {
        const body = (await request.json()) as { hello: string };
        return HttpResponse.json({ echoed: body.hello }, { status: 201 });
      }),
    );
    const result = await apiPost<{ echoed: string }, { hello: string }>('/echo', {
      hello: 'world',
    });
    expect(result.echoed).toBe('world');
  });

  it('throws ApiException with code and message on 400 envelope', async () => {
    server.use(
      http.post('/api/bad', () =>
        HttpResponse.json(
          { error: { code: 'invalid_request', message: 'Bundle is required' } },
          { status: 400 },
        ),
      ),
    );
    await expect(apiPost('/bad', {})).rejects.toMatchObject({
      code: 'invalid_request',
      message: 'Bundle is required',
    });
    await expect(apiPost('/bad', {})).rejects.toBeInstanceOf(ApiException);
  });
});
