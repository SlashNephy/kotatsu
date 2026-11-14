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
  // attach は、描いている視聴の画面を包む要素を覚える。再生中かを、その中の映像で調べる。
  attach: (container: HTMLElement | null) => void;
  subscribe: (listener: () => void) => () => void;
};

function createWatchStore(): WatchStore {
  let state = emptyState;
  let container: HTMLElement | null = null;
  const listeners = new Set<() => void>();
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
        isPlaying(container) &&
        loadMiniPlayer(browserStorage())
      ) {
        update({ ...state, mini: true });

        return;
      }
      update(emptyState);
    },
    close: () => update(emptyState),
    attach: (element) => {
      container = element;
    },
    subscribe: (listener) => {
      listeners.add(listener);

      return () => {
        listeners.delete(listener);
      };
    },
  };
}

// isPlaying は、container の中の映像を再生しているかを返す。止めているか、映像がなければ false。
// 裏で読み込んでいる組の映像ではなく、今映っている映像を見る。
function isPlaying(container: HTMLElement | null): boolean {
  const video = container === null ? null : visibleVideo(container);

  return video !== null && !video.paused && !video.ended;
}

const WatchStoreContext = createContext<WatchStore | null>(null);

// WatchMiniContext は、視聴の画面を、小さなプレイヤーで描いているか。
const WatchMiniContext = createContext(false);

// useWatchMini は、視聴の画面を、小さなプレイヤーで描いているかを返す。
// 小さく描いている間は、番組の情報、操作バー、キーボードの操作、実況のコメント、データ放送を出さずに、映像だけにする。
export function useWatchMini(): boolean {
  return useContext(WatchMiniContext);
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
      <div
        ref={store.attach}
        className={mini ? classes.mini : undefined}
        data-mini={mini || undefined}
      >
        <div className={mini ? classes.miniFrame : undefined}>{element}</div>
        {mini && <MiniPlayerBar href={href} onClose={store.close} />}
      </div>
    </WatchMiniContext>
  );
}

// MiniPlayerBar は、小さなプレイヤーの操作。映像を押すと視聴の画面へ戻り、再生と一時停止と、閉じる操作を置く。
function MiniPlayerBar({ href, onClose }: { href: string; onClose: () => void }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  // frame は、視聴の画面を包む要素。プレイヤーは、つなぎ直すたびに video を作り直すので、その中の今映っている video を見張り続ける (#1730)。
  const [frame, setFrame] = useState<HTMLElement | null>(null);
  const [video, setVideo] = useState<HTMLVideoElement | null>(null);
  const [paused, setPaused] = useState(false);
  const open = () => void navigate({ href });

  useEffect(() => {
    if (frame === null) {
      return;
    }
    const pick = () => setVideo(visibleVideo(frame));
    pick();
    const observer = new MutationObserver(pick);
    observer.observe(frame, {
      childList: true,
      subtree: true,
      attributes: true,
      attributeFilter: ['style'],
    });

    return () => observer.disconnect();
  }, [frame]);

  useEffect(() => {
    if (video === null) {
      return;
    }
    const sync = () => setPaused(video.paused);
    sync();
    video.addEventListener('play', sync);
    video.addEventListener('pause', sync);

    return () => {
      video.removeEventListener('play', sync);
      video.removeEventListener('pause', sync);
    };
  }, [video]);

  return (
    <div
      ref={(bar) => {
        setFrame(bar?.parentElement ?? null);
      }}
      className={classes.miniControls}
    >
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
              if (video === null) {
                return;
              }
              if (video.paused) {
                void video.play().catch(() => {});
              } else {
                video.pause();
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

function noSubscribe() {
  return () => {};
}

function noState(): WatchState {
  return emptyState;
}

// visibleVideo は、frame の中の、今映っている video を返す。裏で読み込んでいる組 (隠してある) の video は選ばない。
function visibleVideo(frame: HTMLElement): HTMLVideoElement | null {
  return (
    [...frame.querySelectorAll('video')].find(
      (v) => v.closest('[style*="visibility: hidden"]') === null,
    ) ?? null
  );
}
