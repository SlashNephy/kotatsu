import {
  createContext,
  type ReactNode,
  useContext,
  useLayoutEffect,
  useState,
  useSyncExternalStore,
} from 'react';

// 視聴の画面 (ライブ視聴、録画とタイムシフトの再生、端末に保存した録画の再生) を、各ルートの下ではなく、
// アプリの根元に置いた WatchHost の下で描く (#1602)。ルートは、何を描くかを useHostedWatch で渡すだけにする。
// 画面を移ってもプレイヤーを閉じずに、小さなプレイヤーで再生を続けるための置き場で、今はルートを離れたら閉じる。

// WatchStore は、WatchHost で描く視聴の画面を持つ。
type WatchStore = {
  get: () => ReactNode;
  set: (element: ReactNode) => void;
  subscribe: (listener: () => void) => () => void;
};

function createWatchStore(): WatchStore {
  let current: ReactNode = null;
  const listeners = new Set<() => void>();

  return {
    get: () => current,
    set: (element) => {
      current = element;
      for (const listener of listeners) {
        listener();
      }
    },
    subscribe: (listener) => {
      listeners.add(listener);

      return () => {
        listeners.delete(listener);
      };
    },
  };
}

const WatchStoreContext = createContext<WatchStore | null>(null);

// WatchHostProvider は、視聴の画面の置き場を用意する。ルートの Outlet と WatchHost の両方を包む。
export function WatchHostProvider({ children }: { children: ReactNode }) {
  const [store] = useState(createWatchStore);

  return <WatchStoreContext value={store}>{children}</WatchStoreContext>;
}

// WatchHost は、ルートが useHostedWatch で渡した視聴の画面を描く。ルートの Outlet の隣に置く。
export function WatchHost() {
  const store = useContext(WatchStoreContext);
  const element = useSyncExternalStore(
    store?.subscribe ?? noSubscribe,
    store?.get ?? noElement,
    store?.get ?? noElement,
  );

  return element;
}

// useHostedWatch は、ルートの視聴の画面 element を、WatchHost で描かせる。ルートは、戻り値をそのまま返す。
// 描くたびに渡し直し、ルートを離れたら下ろす。置き場がなければ (テストで WatchHostProvider を置かないときなど)、
// その場で描くよう element を返す。
//
// ルートとは別の木で描くので、element の中では、ルートの Route.useParams のような、そのルートの文脈に頼る hook を使わない。
// ルートで読んだ値を props で渡す。
export function useHostedWatch(element: ReactNode): ReactNode {
  const store = useContext(WatchStoreContext);
  useLayoutEffect(() => {
    store?.set(element);
  });
  useLayoutEffect(
    () => () => {
      store?.set(null);
    },
    [store],
  );

  return store === null ? element : null;
}

function noSubscribe() {
  return () => {};
}

function noElement(): ReactNode {
  return null;
}
