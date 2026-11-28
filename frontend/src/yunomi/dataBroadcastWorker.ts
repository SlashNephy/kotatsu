// データ放送の TS を読み、web-bml の decodeTS で解いたメッセージを、画面のスレッドに送る Web Worker (設計書 7.4)。
// TS を解くのは重いので、画面のスレッドから外す。
import { decodeTS } from 'web-bml/ts';

import { dataBroadcastEndpoint } from './route';
import { TsPacer } from './tsPacer';

// DataBroadcastRequest は、画面のスレッドから受け取る、読み始めの指示。
export type DataBroadcastRequest = {
  type: 'kotatsu:read';
  // url は、tenban のデータ放送用のエンドポイント。
  url: string;
  // serviceId は、TS の program_number。null なら、PAT の最初の番組を選ぶ。
  serviceId: number | null;
  // paced は、録画型の再生元の配信か。届いた TS を、kotatsu:elapsed で受け取った経過に届くまで渡さない (#2270)。
  paced: boolean;
};

// DataBroadcastElapsed は、録画型の再生元で、読み始めた位置からの放送時刻の経過 (秒) を通知するメッセージ。
export type DataBroadcastElapsed = { type: 'kotatsu:elapsed'; elapsed: number };

// DataBroadcastEnd は、TS を読み終えたか、読めなかったことを、画面のスレッドに通知するメッセージ。
// status は、tenban が成功でない応答を返したときの HTTP のステータス。finished は、録画型の再生元の配信を終わりまで
// 渡し終えたこと。つなぎ直しても同じなので、つなぎ直さない。
export type DataBroadcastEnd = {
  type: 'kotatsu:end';
  error: boolean;
  status?: number;
  finished?: boolean;
};

// maxPendingBytes は、録画型の再生元で、渡さずにためておくバイトの上限。地上波のデータ放送 (約 1.8 Mbps) の 30 秒ほど。
// 超えたら、経過が進んで減るまで、配信を読むのを待つ。
const maxPendingBytes = 8 * 1024 * 1024;

// pacer と elapsed は、録画型の再生元で、いま読んでいる配信の、ためている TS と経過。
let pacer: TsPacer | null = null;
let elapsed = 0;
// drained は、ためた TS が上限を超えて、読むのを待っている間の、待ちを解く関数。
let drained: (() => void) | null = null;

self.addEventListener(
  'message',
  (event: MessageEvent<DataBroadcastRequest | DataBroadcastElapsed>) => {
    // 専用の Worker に送れるのは、作った画面のスレッドだけで、そのメッセージの origin は空になる (Chromium と Firefox)。
    // 空にしないブラウザでも通るよう、同じ origin も受け、ほかの origin のものは読まない。
    if (event.origin !== '' && event.origin !== self.location.origin) {
      return;
    }
    const message = event.data;
    if (message.type === 'kotatsu:elapsed') {
      elapsed = message.elapsed;
      pacer?.release(elapsed);
      // 上限を超えて待っている読み込みと、読み終えて渡し終わるのを待っている読み込みに、確認し直させる。
      if (drained !== null && (pacer?.pendingBytes ?? 0) <= maxPendingBytes) {
        const wake = drained;
        drained = null;
        wake();
      }

      return;
    }
    void read(message);
  },
);

async function read({ url, serviceId, paced }: DataBroadcastRequest): Promise<void> {
  const end = (error: boolean, status?: number) =>
    self.postMessage({ type: 'kotatsu:end', error, status } satisfies DataBroadcastEnd);
  const endpoint = dataBroadcastEndpoint(url);
  if (endpoint === null) {
    end(true);

    return;
  }
  const decoder = decodeTS({
    sendCallback: (message) => self.postMessage(message),
    ...(serviceId === null ? {} : { serviceId }),
    parsePES: false,
  });
  const current = paced ? new TsPacer((bytes) => decoder.push(bytes)) : null;
  pacer = current;
  try {
    const response = await fetch(endpoint, { credentials: 'same-origin' });
    if (!response.ok || response.body === null) {
      end(true, response.status);

      return;
    }
    const reader = response.body.getReader();
    for (;;) {
      // ためた TS が上限を超えたら、経過が進んで減るまで読まない。
      while (current !== null && current.pendingBytes > maxPendingBytes) {
        await new Promise<void>((resolve) => {
          drained = resolve;
        });
      }
      const { done, value } = await reader.read();
      if (done) {
        break;
      }
      if (current === null) {
        decoder.push(value);
      } else {
        current.feed(value);
        current.release(elapsed);
      }
    }
    if (current === null) {
      end(false);

      return;
    }
    // 録画型の再生元は、読み終えても、ためた分を経過に合わせて渡し続け、渡し終えたら終わりを通知する。
    while (current.pendingBytes > 0) {
      await new Promise<void>((resolve) => {
        drained = resolve;
      });
    }
    self.postMessage({
      type: 'kotatsu:end',
      error: false,
      finished: true,
    } satisfies DataBroadcastEnd);
  } catch {
    end(true);
  }
}
