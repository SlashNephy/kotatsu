import { useSyncExternalStore } from 'react';

// SubscriptionConnection は、1 本の subscription の接続の状態を、ConnectionTracker に知らせる。
export type SubscriptionConnection = {
  // reconnecting は、切れた接続をつなぎ直し始めたときに呼ぶ。
  reconnecting: () => void;
  // connected は、つながったときに呼ぶ。
  connected: () => void;
  // release は、subscription をやめたときに呼ぶ。
  release: () => void;
};

// ConnectionTracker は、サーバーとの接続のうち、つなぎ直している最中のものがあるかを数える (#503)。
// graphql-sse は subscription ごとに SSE の接続を張るので、1 本ずつ数える。
// 一時的な障害で送り直している query も、1 本の接続として数える (withTransientRetry)。
export type ConnectionTracker = {
  track: () => SubscriptionConnection;
  // isReconnecting は、つなぎ直している最中の接続が 1 本でもあれば true を返す。
  isReconnecting: () => boolean;
  // recoveries は、つなぎ直していた接続が、つながった回数。サーバーが戻ったことの知らせに使う。
  // つなぎ直している接続の数が 0 に戻るだけでは、諦めてやめたのと区別できないので、別に数える。
  recoveries: () => number;
  subscribe: (listener: () => void) => () => void;
};

export function createConnectionTracker(): ConnectionTracker {
  const reconnecting = new Set<symbol>();
  const listeners = new Set<() => void>();
  let recoveries = 0;
  const notify = () => {
    for (const listener of listeners) {
      listener();
    }
  };
  const update = (id: symbol, value: boolean) => {
    const before = reconnecting.size > 0;
    if (value) {
      reconnecting.add(id);
    } else {
      reconnecting.delete(id);
    }
    if (before !== reconnecting.size > 0) {
      notify();
    }
  };

  return {
    track: () => {
      const id = Symbol('subscription');
      // released は、subscription をやめたかどうか。やめた後に届いた知らせで、つなぎ直している状態に戻さない。
      let released = false;

      return {
        reconnecting: () => {
          if (!released) {
            update(id, true);
          }
        },
        connected: () => {
          if (released) {
            return;
          }
          if (reconnecting.has(id)) {
            recoveries++;
            notify();
          }
          update(id, false);
        },
        release: () => {
          released = true;
          update(id, false);
        },
      };
    },
    isReconnecting: () => reconnecting.size > 0,
    recoveries: () => recoveries,
    subscribe: (listener) => {
      listeners.add(listener);

      return () => {
        listeners.delete(listener);
      };
    },
  };
}

// serverConnections は、画面全体のサーバーとの接続 (subscription と、送り直している query) の状態。
export const serverConnections = createConnectionTracker();

// useServerReconnecting は、つなぎ直している最中の接続があるかを返す。
export function useServerReconnecting(): boolean {
  return useSyncExternalStore(serverConnections.subscribe, serverConnections.isReconnecting);
}

// maxRetryDelayMs は、つなぎ直すのを待つ時間の上限。サーバーの再起動などで長く切れても、この間隔で試し続ける。
export const maxRetryDelayMs = 30_000;

// retryDelayMs は、retries 回目のつなぎ直しの前に待つ時間。1 秒から倍々に延ばし、maxRetryDelayMs で止める。
// 多くの画面が同時につなぎ直さないよう、0.3〜3 秒をずらす (random は 0 以上 1 未満)。
export function retryDelayMs(retries: number, random: number): number {
  const base = Math.min(1000 * 2 ** retries, maxRetryDelayMs);

  return base + Math.floor(300 + random * 2700);
}
