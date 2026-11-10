import { useSyncExternalStore } from 'react';

import type { PlayerCommands } from '../yunomi/commands';
import { browserStorage } from '../yunomi/volume';

// 実況のコメントの位置 (#1357、設計書 7.4)。録画の再生で、コメントを流す位置を前後にずらす。
// 正の値で遅らせ、負の値で早める。投稿者がコメントを打つまでの時間があるので、コメントは映像より遅れて見えることが多い。端末ごとに 1 つ覚える。

const offsetKey = 'kotatsu.player.commentOffsetMs';
// changeEvent は、同じタブの中で変えたことを知らせるイベント。storage イベントは、ほかのタブにしか届かない。
const changeEvent = 'kotatsu:comment-offset';

// maxCommentOffsetMs は、前後にずらせる上限 (ミリ秒)。commentOffsetStepMs は、キーとメニューで変える幅。
export const maxCommentOffsetMs = 30_000;
export const commentOffsetStepMs = 500;

// clampCommentOffset は、ずらす量を、上限の範囲の、刻みにそろえた値にする。
function clampCommentOffset(ms: number): number {
  const stepped = Math.round(ms / commentOffsetStepMs) * commentOffsetStepMs;

  // 0 を足して、-0 を 0 にする。
  return Math.min(Math.max(stepped, -maxCommentOffsetMs), maxCommentOffsetMs) + 0;
}

// loadCommentOffset は、覚えたずらす量 (ミリ秒) を読む。読めなければ 0 を返す。
export function loadCommentOffset(storage: Pick<Storage, 'getItem'> | null): number {
  try {
    const ms = Number(storage?.getItem(offsetKey) ?? 0);

    return Number.isFinite(ms) ? clampCommentOffset(ms) : 0;
  } catch {
    return 0;
  }
}

// stepCommentOffset は、ずらす量 ms を、direction の向きに 1 刻み変えた値を返す。
export function stepCommentOffset(ms: number, direction: 1 | -1): number {
  return clampCommentOffset(ms + direction * commentOffsetStepMs);
}

// formatCommentOffset は、ずらす量を、符号つきの秒の文字にする。
export function formatCommentOffset(ms: number): string {
  const seconds = (ms / 1_000).toFixed(1);

  return ms > 0 ? `+${seconds}` : seconds;
}

function subscribe(onChange: () => void): () => void {
  window.addEventListener('storage', onChange);
  window.addEventListener(changeEvent, onChange);

  return () => {
    window.removeEventListener('storage', onChange);
    window.removeEventListener(changeEvent, onChange);
  };
}

// useCommentOffset は、ずらす量 (ミリ秒) と、それを変える関数を返す。ほかのタブで変えても追う。
export function useCommentOffset(): [number, (ms: number) => void] {
  const ms = useSyncExternalStore(subscribe, () => loadCommentOffset(browserStorage()));

  return [
    ms,
    (next) => {
      try {
        browserStorage()?.setItem(offsetKey, String(clampCommentOffset(next)));
      } catch {
        // 書けなければ、次に開いたときは既定に戻るだけ。
      }
      window.dispatchEvent(new Event(changeEvent));
    },
  ];
}

// useCommentOffsetCommands は、コメントの位置の操作を返す。変えたら、onChange に新しいずらす量 (ミリ秒) を渡す。
export function useCommentOffsetCommands(
  onChange: (ms: number) => void,
): Pick<PlayerCommands, 'commentOffsetEarlier' | 'commentOffsetLater' | 'commentOffsetReset'> {
  const [ms, setMs] = useCommentOffset();
  const change = (next: number) => {
    setMs(next);
    onChange(next);
  };

  return {
    commentOffsetEarlier: () => change(stepCommentOffset(ms, -1)),
    commentOffsetLater: () => change(stepCommentOffset(ms, 1)),
    commentOffsetReset: () => change(0),
  };
}
