import { describe, expect, test } from 'vitest';

import { connectivityPath, createDataBroadcastIP, requestPath } from './dataBroadcastIP';

type Sent = { path: string; init: RequestInit | undefined; body: unknown };

// fakeFetch は、送った先と本文を sent に残し、respond の応答を返す fetch。
function fakeFetch(respond: () => Response | Promise<Response>): {
  fetcher: typeof fetch;
  sent: Sent[];
} {
  const sent: Sent[] = [];
  const fetcher = (async (input: RequestInfo | URL, init?: RequestInit) => {
    sent.push({
      path: typeof input === 'string' ? input : input instanceof URL ? input.href : input.url,
      init,
      body: typeof init?.body === 'string' ? JSON.parse(init.body) : null,
    });

    return respond();
  }) as typeof fetch;

  return { fetcher, sent };
}

// text は、応答の本文を文字列にする。jsdom と Node.js で Uint8Array の realm が違うので、バイト列のまま比べない。
function text(bytes: Uint8Array | undefined): string {
  return new TextDecoder().decode(bytes);
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

describe('createDataBroadcastIP', () => {
  test('文書の POST を、本文を Base64 にして tenban に中継させ、上流の状態コードと本文を返す', async () => {
    const { fetcher, sent } = fakeFetch(() =>
      json({ status: 200, contentType: 'text/plain', cacheControl: '', body: btoa('OK') }),
    );
    const ip = createDataBroadcastIP(fetcher);
    const body = new TextEncoder().encode('Denbun=%a4%a2');

    const result = await ip.transmitTextDataOverIP?.('https://example.com/vote', body);

    expect(result?.resultCode).toBe(1);
    expect(result?.statusCode).toBe('200');
    expect(text(result?.response)).toBe('OK');
    expect(sent).toHaveLength(1);
    expect(sent[0]?.path).toBe(requestPath);
    expect(sent[0]?.init?.method).toBe('POST');
    expect(sent[0]?.init?.credentials).toBe('same-origin');
    expect(sent[0]?.body).toEqual({
      method: 'POST',
      url: 'https://example.com/vote',
      body: btoa('Denbun=%a4%a2'),
    });
  });

  test.each([
    { name: '中継が無効な', respond: () => new Response('disabled', { status: 404 }) },
    { name: 'LAN の中の宛先を断った', respond: () => new Response('refused', { status: 403 }) },
    {
      name: 'tenban に届かない',
      respond: (): Response => {
        throw new TypeError('network error');
      },
    },
  ])('$nameときは、送れなかったことを知らせる', async ({ respond }) => {
    const { fetcher } = fakeFetch(respond);
    const ip = createDataBroadcastIP(fetcher);

    const result = await ip.transmitTextDataOverIP?.('https://example.com/vote', new Uint8Array());

    expect(result?.resultCode).toBeNaN();
    expect(result?.statusCode).toBe('');
    expect(result?.response).toHaveLength(0);
  });

  test('文書の GET を中継させ、web-bml が読む Content-Type と Cache-Control を見出しで返す', async () => {
    const { fetcher, sent } = fakeFetch(() =>
      json({
        status: 200,
        contentType: 'text/X-arib-bml; charset=EUC-JP',
        cacheControl: 'no-store',
        body: btoa('<bml/>'),
      }),
    );
    const ip = createDataBroadcastIP(fetcher);

    const result = await ip.get?.('https://example.com/index.bml');

    expect(sent[0]?.body).toEqual({
      method: 'GET',
      url: 'https://example.com/index.bml',
      body: null,
    });
    expect(result?.statusCode).toBe(200);
    expect(result?.headers?.get('content-type')).toBe('text/X-arib-bml; charset=EUC-JP');
    expect(result?.headers?.get('cache-control')).toBe('no-store');
    expect(text(result?.response)).toBe('<bml/>');
  });

  test('GET を中継できなければ、何も返さない', async () => {
    const { fetcher } = fakeFetch(() => new Response('refused', { status: 403 }));
    const ip = createDataBroadcastIP(fetcher);

    expect(await ip.get?.('http://192.168.1.1/')).toEqual({});
  });

  test('接続の確かめを tenban に任せ、結果を返す。確かめられなければ null を返す', async () => {
    const ok = fakeFetch(() =>
      json({ success: true, ipAddress: '192.0.2.1', responseTimeMillis: 12 }),
    );
    expect(
      await createDataBroadcastIP(ok.fetcher).confirmIPNetwork?.('example.com', true, 3000),
    ).toEqual({ success: true, ipAddress: '192.0.2.1', responseTimeMillis: 12 });
    expect(ok.sent[0]?.path).toBe(connectivityPath);
    expect(ok.sent[0]?.body).toEqual({ destination: 'example.com', timeoutMillis: 3000 });

    const disabled = fakeFetch(() => new Response('disabled', { status: 404 }));
    expect(
      await createDataBroadcastIP(disabled.fetcher).confirmIPNetwork?.('example.com', false, 3000),
    ).toBeNull();
  });
});
