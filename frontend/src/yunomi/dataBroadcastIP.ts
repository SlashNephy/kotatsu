import type { IP } from 'web-bml';

// requestPath と connectivityPath は、データ放送の文書の通信を中継する tenban のエンドポイント (#501、設計書 7.4)。
// CSP (設計書 10.4) で、ブラウザからは外部へ接続させないので、文書の通信はどれも tenban に送る。
export const requestPath = '/media/data-broadcast/requests';
export const connectivityPath = '/media/data-broadcast/connectivity';

// connectionTypeEthernetDHCP は、getConnectionType で返す接続の種類 (ARIB STD-B24 第二分冊 第二編 付属 3 の表 5-12)。
const connectionTypeEthernetDHCP = 403;

// RelayedResponse は、tenban が中継した応答。本文は Base64 で届く。
type RelayedResponse = {
  status: number;
  contentType: string;
  cacheControl: string;
  body: string | null;
};

type Connectivity = {
  success: boolean;
  ipAddress: string | null;
  responseTimeMillis: number | null;
};

function encodeBase64(bytes: Uint8Array): string {
  let binary = '';
  for (const byte of bytes) {
    binary += String.fromCharCode(byte);
  }

  return btoa(binary);
}

function decodeBase64(base64: string | null): Uint8Array<ArrayBuffer> {
  const binary = atob(base64 ?? '');
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i);
  }

  return bytes;
}

// postJSON は、tenban に JSON を送り、成功すれば応答の JSON を返す。中継が無効、宛先を断った、届かなかったなどでは null を返す。
async function postJSON<T>(fetcher: typeof fetch, path: string, body: unknown): Promise<T | null> {
  try {
    const response = await fetcher(path, {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!response.ok) {
      return null;
    }

    return (await response.json()) as T;
  } catch {
    return null;
  }
}

// createDataBroadcastIP は、web-bml に渡す通信の機能を返す。管理者が中継を有効にしたときだけ渡す (#501)。
// 文書の GET (インターネットの文書や画像を読む) と、transmitTextDataOverIP の POST (投票やプレゼントの応募など) と、
// confirmIPNetwork (インターネットへ接続できるかの確かめ) を、tenban に中継させる。
export function createDataBroadcastIP(fetcher: typeof fetch = fetch): IP {
  const relay = (method: 'GET' | 'POST', url: string, body: Uint8Array | null) =>
    postJSON<RelayedResponse>(fetcher, requestPath, {
      method,
      url,
      body: body === null ? null : encodeBase64(body),
    });

  return {
    // 中継を有効にしていれば、受信機はいつもインターネットにつながっているものとして答える (表 5-14 の 1)。
    isIPConnected: () => 1,
    getConnectionType: () => connectionTypeEthernetDHCP,
    get: async (uri) => {
      const response = await relay('GET', uri, null);
      if (response === null) {
        return {};
      }
      const headers = new Headers();
      if (response.contentType !== '') {
        headers.set('Content-Type', response.contentType);
      }
      if (response.cacheControl !== '') {
        headers.set('Cache-Control', response.cacheControl);
      }

      return { statusCode: response.status, headers, response: decodeBase64(response.body) };
    },
    transmitTextDataOverIP: async (uri, body) => {
      const response = await relay('POST', uri, body);
      if (response === null) {
        // 送れなかったことを、web-bml が通信の機能を持たないときと同じ値で知らせる。
        return { resultCode: NaN, statusCode: '', response: new Uint8Array() };
      }

      return {
        resultCode: 1,
        statusCode: String(response.status),
        response: decodeBase64(response.body),
      };
    },
    confirmIPNetwork: async (destination, _isICMP, timeoutMillis) => {
      // ICMP は tenban で送れないので、どちらを求められても、tenban が TCP で接続できるかを確かめる。
      const result = await postJSON<Connectivity>(fetcher, connectivityPath, {
        destination,
        timeoutMillis,
      });
      if (result === null) {
        return null;
      }

      return {
        success: result.success,
        ipAddress: result.ipAddress,
        responseTimeMillis: result.responseTimeMillis,
      };
    },
  };
}
