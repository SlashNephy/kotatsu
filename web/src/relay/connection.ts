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

// ConnectionTracker は、subscription の接続のうち、つなぎ直している最中のものがあるかを数える (#503)。
// graphql-sse は subscription ごとに SSE の接続を張るので、1 本ずつ数える。
export type ConnectionTracker = {
  track: () => SubscriptionConnection;
  // isReconnecting は、つなぎ直している最中の subscription が 1 本でもあれば true を返す。
  isReconnecting: () => boolean;
  subscribe: (listener: () => void) => () => void;
};

export function createConnectionTracker(): ConnectionTracker {
  const reconnecting = new Set<symbol>();
  const listeners = new Set<() => void>();
  const update = (id: symbol, value: boolean) => {
    const before = reconnecting.size > 0;
    if (value) {
      reconnecting.add(id);
    } else {
      reconnecting.delete(id);
    }
    if (before !== reconnecting.size > 0) {
      for (const listener of listeners) {
        listener();
      }
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
          if (!released) {
            update(id, false);
          }
        },
        release: () => {
          released = true;
          update(id, false);
        },
      };
    },
    isReconnecting: () => reconnecting.size > 0,
    subscribe: (listener) => {
      listeners.add(listener);

      return () => {
        listeners.delete(listener);
      };
    },
  };
}

// subscriptionConnections は、画面全体の subscription の接続の状態。
export const subscriptionConnections = createConnectionTracker();

// useSubscriptionsReconnecting は、つなぎ直している最中の subscription があるかを返す。
export function useSubscriptionsReconnecting(): boolean {
  return useSyncExternalStore(
    subscriptionConnections.subscribe,
    subscriptionConnections.isReconnecting,
  );
}

// maxRetryDelayMs は、つなぎ直すのを待つ時間の上限。サーバーの再起動などで長く切れても、この間隔で試し続ける。
export const maxRetryDelayMs = 30_000;

// retryDelayMs は、retries 回目のつなぎ直しの前に待つ時間。1 秒から倍々に延ばし、maxRetryDelayMs で止める。
// 多くの画面が同時につなぎ直さないよう、0.3〜3 秒をずらす (random は 0 以上 1 未満)。
export function retryDelayMs(retries: number, random: number): number {
  const base = Math.min(1000 * 2 ** retries, maxRetryDelayMs);

  return base + Math.floor(300 + random * 2700);
}
