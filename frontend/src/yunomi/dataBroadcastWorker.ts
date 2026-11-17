// データ放送の TS を読み、web-bml の decodeTS で解いたメッセージを、画面のスレッドに送る Web Worker (設計書 7.4)。
// TS を解くのは重いので、画面のスレッドから外す。
import { decodeTS } from 'web-bml/ts';

import { dataBroadcastEndpoint } from './route';

// DataBroadcastRequest は、画面のスレッドから受け取る、読み始めの指示。
export type DataBroadcastRequest = {
  // url は、tenban のデータ放送用のエンドポイント。
  url: string;
  // serviceId は、TS の program_number。
  serviceId: number;
};

// DataBroadcastEnd は、TS を読み終えたか、読めなかったことを、画面のスレッドに知らせるメッセージ。
// status は、tenban が成功でない応答を返したときの HTTP のステータス。
export type DataBroadcastEnd = { type: 'kotatsu:end'; error: boolean; status?: number };

self.addEventListener('message', (event: MessageEvent<DataBroadcastRequest>) => {
  // 専用の Worker に送れるのは、作った画面のスレッドだけで、そのメッセージの origin は空になる (Chromium と Firefox)。
  // 空にしないブラウザでも通るよう、同じ origin も受け、ほかの origin のものは読まない。
  if (event.origin !== '' && event.origin !== self.location.origin) {
    return;
  }
  void read(event.data);
});

async function read({ url, serviceId }: DataBroadcastRequest): Promise<void> {
  const end = (error: boolean, status?: number) =>
    self.postMessage({ type: 'kotatsu:end', error, status } satisfies DataBroadcastEnd);
  const endpoint = dataBroadcastEndpoint(url);
  if (endpoint === null) {
    end(true);

    return;
  }
  const decoder = decodeTS({
    sendCallback: (message) => self.postMessage(message),
    serviceId,
    parsePES: false,
  });
  try {
    const response = await fetch(endpoint, { credentials: 'same-origin' });
    if (!response.ok || response.body === null) {
      end(true, response.status);

      return;
    }
    const reader = response.body.getReader();
    for (;;) {
      const { done, value } = await reader.read();
      if (done) {
        break;
      }
      decoder.push(value);
    }
    end(false);
  } catch {
    end(true);
  }
}
