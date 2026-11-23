import { ActionIcon, Group, Tooltip } from '@mantine/core';
import { useNavigate } from '@tanstack/react-router';
import { Maximize2, Pause, Play, X } from 'lucide-react';
import {
  createContext,
  type ReactNode,
  useContext,
  useEffect,
  useLayoutEffect,
  useState,
  useSyncExternalStore,
} from 'react';
import { useTranslation } from 'react-i18next';

import { loadMiniPlayer } from './miniPlayer';
import type { PlayerMedia } from './playerMedia';
import { browserStorage } from './volume';
import classes from './WatchHost.module.css';

// 視聴の画面 (ライブ視聴、録画とタイムシフトの再生、端末に保存した録画の再生) を、各ルートの下ではなく、
// アプリの根元に置いた WatchHost の下で描く (#1602)。ルートは、何を描くかを useHostedWatch で渡すだけにする。
// 再生中にルートを離れたら、画面を閉じずに、画面の隅の小さなプレイヤーで再生を続ける。止めていたら閉じる。

// WatchState は、WatchHost で描く視聴の画面。
type WatchState = {
  element: ReactNode;
  // href は、視聴の画面の URL。小さなプレイヤーを押したら、ここへ戻る。
  href: string;
  // mini は、小さなプレイヤーで描いているか。
  mini: boolean;
};

const emptyState: WatchState = { element: null, href: '', mini: false };

// WatchStore は、WatchHost で描く視聴の画面を持つ。
type WatchStore = {
  get: () => WatchState;
  // show は、ルートの視聴の画面 element を、大きく描く。
  show: (element: ReactNode, href: string) => void;
  // leave は、ルートを離れたときに呼ぶ。再生中で、小さなプレイヤーを使う設定なら小さくし、そうでなければ閉じる。
  leave: () => void;
  // close は、視聴の画面を閉じる。
  close: () => void;
  subscribe: (listener: () => void) => () => void;
  // media は、視聴の画面が今映している経路。再生中かを、これで調べる。映していなければ null。
  media: () => PlayerMedia | null;
  // setMedia は、視聴の画面が今映している経路を覚える。
  setMedia: (media: PlayerMedia | null) => void;
  subscribeMedia: (listener: () => void) => () => void;
};

function createWatchStore(): WatchStore {
  let state = emptyState;
  let media: PlayerMedia | null = null;
  const listeners = new Set<() => void>();
  const mediaListeners = new Set<() => void>();
  const update = (next: WatchState) => {
    state = next;
    for (const listener of listeners) {
      listener();
    }
  };

  return {
    get: () => state,
    show: (element, href) => update({ element, href, mini: false }),
    leave: () => {
      if (
        state.element !== null &&
        !state.mini &&
        isPlaying(media) &&
        loadMiniPlayer(browserStorage())
      ) {
        update({ ...state, mini: true });

        return;
      }
      update(emptyState);
    },
    close: () => update(emptyState),
    subscribe: (listener) => {
      listeners.add(listener);

      return () => {
        listeners.delete(listener);
      };
    },
    media: () => media,
    setMedia: (next) => {
      if (media === next) {
        return;
      }
      media = next;
      for (const listener of mediaListeners) {
        listener();
      }
    },
    subscribeMedia: (listener) => {
      mediaListeners.add(listener);

      return () => {
        mediaListeners.delete(listener);
      };
    },
  };
}

// isPlaying は、media を再生しているかを返す。止めているか、映像がなければ false。
function isPlaying(media: PlayerMedia | null): boolean {
  return media !== null && !media.paused && !media.ended;
}

const WatchStoreContext = createContext<WatchStore | null>(null);

// WatchMiniContext は、視聴の画面を、小さなプレイヤーで描いているか。
const WatchMiniContext = createContext(false);

// useWatchMini は、視聴の画面を、小さなプレイヤーで描いているかを返す。
// 小さく描いている間は、番組の情報、操作バー、キーボードの操作、実況のコメント、データ放送を出さずに、映像だけにする。
export function useWatchMini(): boolean {
  return useContext(WatchMiniContext);
}

// useCloseWatch は、視聴の画面 (小さなプレイヤーを含む) を閉じる関数を返す。置き場の外では何もしない。
// 「この端末で試す」(#1771) が、ほかの再生と重ならないように、試し始める前に閉じる。
export function useCloseWatch(): () => void {
  const store = useContext(WatchStoreContext);

  return () => store?.close();
}

// WatchHostProvider は、視聴の画面の置き場を用意する。ルートの Outlet と WatchHost の両方を包む。
export function WatchHostProvider({ children }: { children: ReactNode }) {
  const [store] = useState(createWatchStore);

  return <WatchStoreContext value={store}>{children}</WatchStoreContext>;
}

// WatchHost は、ルートが useHostedWatch で渡した視聴の画面を描く。ルートの Outlet の隣に置く。
// 大きく描くときも小さく描くときも、視聴の画面を同じ位置に置き、プレイヤーを作り直さない。
export function WatchHost() {
  const store = useContext(WatchStoreContext);
  const { element, href, mini } = useSyncExternalStore(
    store?.subscribe ?? noSubscribe,
    store?.get ?? noState,
    store?.get ?? noState,
  );
  if (store === null || element === null) {
    return null;
  }

  return (
    <WatchMiniContext value={mini}>
      <div className={mini ? classes.mini : undefined} data-mini={mini || undefined}>
        <div className={mini ? classes.miniFrame : undefined}>{element}</div>
        {mini && <MiniPlayerBar store={store} href={href} onClose={store.close} />}
      </div>
    </WatchMiniContext>
  );
}

// MiniPlayerBar は、小さなプレイヤーの操作。映像を押すと視聴の画面へ戻り、再生と一時停止と、閉じる操作を置く。
function MiniPlayerBar({
  store,
  href,
  onClose,
}: {
  store: WatchStore;
  href: string;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  // media は、視聴の画面が今映している経路。プレイヤーは、つなぎ直すたびに作り直すので、変わるたびに追う (#1730)。
  const media = useSyncExternalStore(store.subscribeMedia, store.media, store.media);
  const [paused, setPaused] = useState(false);
  const open = () => void navigate({ href });

  useEffect(() => {
    if (media === null) {
      return;
    }
    const sync = () => setPaused(media.paused);
    sync();
    media.addEventListener('play', sync);
    media.addEventListener('pause', sync);

    return () => {
      media.removeEventListener('play', sync);
      media.removeEventListener('pause', sync);
    };
  }, [media]);

  return (
    <div className={classes.miniControls}>
      {/* 映像を押すと、視聴の画面へ戻る。 */}
      <button
        type="button"
        className={classes.miniOpen}
        onClick={open}
        aria-label={t('player.mini.open')}
      />
      <Group gap={4} className={classes.miniButtons}>
        <Tooltip label={t('player.mini.open')}>
          <ActionIcon
            variant="filled"
            color="dark"
            onClick={open}
            aria-label={t('player.mini.open')}
          >
            <Maximize2 size={16} aria-hidden />
          </ActionIcon>
        </Tooltip>
        <Tooltip label={paused ? t('player.mini.play') : t('player.mini.pause')}>
          <ActionIcon
            variant="filled"
            color="dark"
            onClick={() => {
              if (media === null) {
                return;
              }
              if (media.paused) {
                void media.play().catch(() => {});
              } else {
                media.pause();
              }
            }}
            aria-label={paused ? t('player.mini.play') : t('player.mini.pause')}
          >
            {paused ? <Play size={16} aria-hidden /> : <Pause size={16} aria-hidden />}
          </ActionIcon>
        </Tooltip>
        <Tooltip label={t('player.mini.close')}>
          <ActionIcon
            variant="filled"
            color="dark"
            onClick={onClose}
            aria-label={t('player.mini.close')}
          >
            <X size={16} aria-hidden />
          </ActionIcon>
        </Tooltip>
      </Group>
    </div>
  );
}

// useHostedWatch は、ルートの視聴の画面 element を、WatchHost で描かせる。ルートは、戻り値をそのまま返す。
// 描くたびに渡し直す。ルートを離れたら、再生中なら小さなプレイヤーにし、そうでなければ閉じる。
// 置き場がなければ (テストで WatchHostProvider を置かないときなど)、その場で描くよう element を返す。
//
// ルートとは別の木で描くので、element の中では、ルートの Route.useParams のような、そのルートの文脈に頼る hook を使わない。
// ルートで読んだ値を props で渡す。
export function useHostedWatch(element: ReactNode): ReactNode {
  const store = useContext(WatchStoreContext);
  useLayoutEffect(() => {
    store?.show(element, window.location.pathname + window.location.search + window.location.hash);
  });
  useLayoutEffect(
    () => () => {
      store?.leave();
    },
    [store],
  );

  return store === null ? element : null;
}

// useWatchMedia は、視聴の画面が今映している経路を、置き場に覚えさせる。小さなプレイヤーにするかを決めるときと、
// 小さなプレイヤーの再生と一時停止に使う。視聴の画面の中 (useHostedWatch で渡した element の中) で呼ぶ。
export function useWatchMedia(media: PlayerMedia | null): void {
  const store = useContext(WatchStoreContext);
  useEffect(() => {
    store?.setMedia(media);

    return () => {
      // 次の経路を覚えさせた後に、前の経路の片付けで消さないようにする。
      if (store?.media() === media) {
        store.setMedia(null);
      }
    };
  }, [store, media]);
}

function noSubscribe() {
  return () => {};
}

function noState(): WatchState {
  return emptyState;
}
