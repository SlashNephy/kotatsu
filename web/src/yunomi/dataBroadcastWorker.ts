// データ放送の TS を読み、web-bml の decodeTS で解いたメッセージを、画面のスレッドに送る Web Worker (設計書 7.4)。
// TS を解くのは重いので、画面のスレッドから外す。
import { decodeTS } from 'web-bml/ts';

// DataBroadcastRequest は、画面のスレッドから受け取る、読み始めの指示。
export type DataBroadcastRequest = {
  // url は、tenban のデータ放送用のエンドポイント。
  url: string;
  // serviceId は、TS の program_number。
  serviceId: number;
};

// DataBroadcastEnd は、TS を読み終えたか、読めなかったことを、画面のスレッドに知らせるメッセージ。
export type DataBroadcastEnd = { type: 'kotatsu:end'; error: boolean };

self.addEventListener('message', (event: MessageEvent<DataBroadcastRequest>) => {
  void read(event.data);
});

async function read({ url, serviceId }: DataBroadcastRequest): Promise<void> {
  const decoder = decodeTS({
    sendCallback: (message) => self.postMessage(message),
    serviceId,
    parsePES: false,
  });
  const end = (error: boolean) =>
    self.postMessage({ type: 'kotatsu:end', error } satisfies DataBroadcastEnd);
  try {
    const response = await fetch(url, { credentials: 'same-origin' });
    if (!response.ok || response.body === null) {
      end(true);

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
