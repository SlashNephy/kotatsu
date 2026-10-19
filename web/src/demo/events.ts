// ViewerEvent は、Subscription.viewerEvents で流す 1 件。
export type ViewerEvent = {
  dataChange: { channel: string; version: number } | null;
  viewerSettingsVersion: number | null;
  unreadNotificationCount: number | null;
};

// EventHub は、デモの中で起きた変更を、viewerEvents を購読している画面に届ける。
// 画面は、dataChange の版で query を取り直すので (useDataChanges)、mutation の後にここへ流す。
export type EventHub = {
  publish: (event: ViewerEvent) => void;
  // listen は、購読をやめるまで、流れた出来事を返し続ける。
  listen: () => AsyncGenerator<ViewerEvent>;
};

export function createEventHub(): EventHub {
  const listeners = new Set<(event: ViewerEvent) => void>();

  return {
    publish: (event) => {
      for (const listener of listeners) {
        listener(event);
      }
    },
    listen: async function* () {
      const queue: ViewerEvent[] = [];
      let wake: (() => void) | undefined;
      const listener = (event: ViewerEvent) => {
        queue.push(event);
        wake?.();
      };
      listeners.add(listener);
      try {
        for (;;) {
          const event = queue.shift();
          if (event !== undefined) {
            yield event;
            continue;
          }
          await new Promise<void>((resolve) => {
            wake = resolve;
          });
          wake = undefined;
        }
      } finally {
        listeners.delete(listener);
      }
    },
  };
}
