// ViewerEvent は、Subscription.viewerEvents で流す 1 件。
export type ViewerEvent = {
  dataChange: { channel: string; version: number } | null;
  viewerSettingsVersion: number | null;
  unreadNotificationCount: number | null;
  // streamsDraining は、tenban を止める前の知らせ。デモは止まらないので、いつも null。
  streamsDraining: null;
};

// EventHub は、デモの中で起きた変更を、viewerEvents を購読している画面に届ける。
// 画面は、dataChange の版で query を取り直すので (useDataChanges)、mutation の後にここへ流す。
export type EventHub = {
  publish: (event: ViewerEvent) => void;
  // listen は、return を呼ぶまで、流れた出来事を map で変えて返し続ける。
  listen: <T>(map: (event: ViewerEvent) => T) => AsyncIterableIterator<T>;
};

export function createEventHub(): EventHub {
  const listeners = new Set<(event: ViewerEvent) => void>();

  return {
    publish: (event) => {
      for (const listener of listeners) {
        listener(event);
      }
    },
    // ジェネレーターで書くと、次の出来事を待っている間に return を呼ばれても、待ちが終わるまで finally に進めず、
    // リスナーが残る。そこで、待っている next をその場で終わらせられるよう、イテレーターを直に組む。
    listen: <T>(map: (event: ViewerEvent) => T) => {
      const queue: T[] = [];
      let waiting: ((result: IteratorResult<T>) => void) | undefined;
      const listener = (event: ViewerEvent) => {
        const value = map(event);
        if (waiting === undefined) {
          queue.push(value);
          return;
        }
        const resolve = waiting;
        waiting = undefined;
        resolve({ done: false, value });
      };
      listeners.add(listener);
      const done = (): IteratorResult<T> => ({ done: true, value: undefined });

      const iterator: AsyncIterableIterator<T> = {
        next: async () => {
          if (!listeners.has(listener)) {
            return done();
          }
          const value = queue.shift();
          if (value !== undefined) {
            return { done: false, value };
          }
          return new Promise<IteratorResult<T>>((resolve) => {
            waiting = resolve;
          });
        },
        return: async () => {
          listeners.delete(listener);
          queue.length = 0;
          waiting?.(done());
          waiting = undefined;

          return done();
        },
        [Symbol.asyncIterator]: () => iterator,
      };

      return iterator;
    },
  };
}
