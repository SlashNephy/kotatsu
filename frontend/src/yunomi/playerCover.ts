import { useRef, type RefObject } from 'react';

import { fitContain } from './capture';
import type { CoverKind } from './cmAutoSkipper';
import type { PlayerMedia } from './playerMedia';

// PlayerCover は、映像の上に重ねる蓋 (#1512)。CM を飛ぶ間、CM のフレームを隠す。
export type PlayerCover = {
  // show は、蓋を出す。still なら、いま映っているフレームをコピーして、映像が実際に出ている矩形に重ねる。
  // 写せなければ (大きさが 0、例外)、black と同じく、プレイヤーの枠全体を黒で覆う。
  show: (kind: CoverKind) => void;
  hide: () => void;
};

type Rect = { left: number; top: number; width: number; height: number };

// coverRect は、蓋を置く矩形を、蓋を入れる要素 (container) の中の CSS の画素で返す。
// 映像の要素の中で、映像が実際に出ている矩形 (レターボックスを除いた範囲) を、端末の画素の境目へ外側に丸める。
// 映像の縁が、蓋の外に覗かないようにするため。
export function coverRect(
  container: Rect,
  video: Rect,
  videoWidth: number,
  videoHeight: number,
  devicePixelRatio: number,
): Rect {
  const shown = fitContain(videoWidth, videoHeight, video.width, video.height);
  const ratio = devicePixelRatio > 0 ? devicePixelRatio : 1;
  const left = Math.floor((video.left + shown.x) * ratio) / ratio;
  const top = Math.floor((video.top + shown.y) * ratio) / ratio;
  const right = Math.ceil((video.left + shown.x + shown.width) * ratio) / ratio;
  const bottom = Math.ceil((video.top + shown.y + shown.height) * ratio) / ratio;

  return {
    left: left - container.left,
    top: top - container.top,
    width: right - left,
    height: bottom - top,
  };
}

// usePlayerCover は、蓋の要素に渡す ref と、蓋を出し入れする PlayerCover を返す。
// コピーする元は PlayerMedia の frame (<video> の経路では video の要素) にする。yadif が描く WebGL の canvas は
// preserveDrawingBuffer を使っておらず、コピーすると黒になる。frame のない経路では、黒で覆う。
// onChange は、蓋を出したとき (true) と外したとき (false) に呼ぶ。蓋を出している間は、字幕を消す (CM の字幕を残さない)。
export function usePlayerCover(
  mediaRef: RefObject<PlayerMedia | null>,
  onChange: (shown: boolean) => void,
): { ref: RefObject<HTMLDivElement | null>; cover: PlayerCover } {
  const ref = useRef<HTMLDivElement | null>(null);
  const onChangeRef = useRef(onChange);
  onChangeRef.current = onChange;
  // frame は、蓋を置き直す requestAnimationFrame の番号。蓋を出している間だけ回す。
  const frame = useRef<number | null>(null);
  const coverRef = useRef<PlayerCover | null>(null);

  // place は、映像の今の矩形に蓋を合わせる。全画面の切り替え、要素の大きさの変化、別のウィンドウへの移動でも、毎フレーム置き直す。
  const place = () => {
    const element = ref.current;
    const media = mediaRef.current;
    const container = element?.parentElement;
    if (element === null || media === null || container == null) {
      return;
    }
    // 読み直すと、video は新しい映像が届くまで大きさを失う。その間は、置いた場所のままにする。
    if (
      element.dataset['cmCover'] !== 'still' ||
      media.videoWidth === 0 ||
      media.videoHeight === 0
    ) {
      return;
    }
    const rect = coverRect(
      container.getBoundingClientRect(),
      media.element.getBoundingClientRect(),
      media.videoWidth,
      media.videoHeight,
      window.devicePixelRatio,
    );
    element.style.left = `${rect.left}px`;
    element.style.top = `${rect.top}px`;
    element.style.width = `${rect.width}px`;
    element.style.height = `${rect.height}px`;
  };
  const loop = () => {
    place();
    frame.current = requestAnimationFrame(loop);
  };

  coverRef.current ??= {
    show: (kind) => {
      const element = ref.current;
      if (element === null) {
        return;
      }
      const canvas = element.querySelector('canvas');
      let shown: CoverKind = 'black';
      const media = mediaRef.current;
      const source = media?.frame ?? null;
      if (kind === 'still' && canvas !== null && media !== null && source !== null) {
        try {
          if (media.videoWidth > 0 && media.videoHeight > 0) {
            canvas.width = media.videoWidth;
            canvas.height = media.videoHeight;
            const context = canvas.getContext('2d');
            if (context !== null) {
              context.drawImage(source, 0, 0, canvas.width, canvas.height);
              shown = 'still';
            }
          }
        } catch {
          // 写せなければ、黒で覆う。
        }
      }
      element.dataset['cmCover'] = shown;
      if (shown === 'black') {
        // 黒は、開いた直後のように映像の大きさが分からなくても隠せるよう、プレイヤーの枠全体を覆う。
        element.style.left = '0';
        element.style.top = '0';
        element.style.width = '100%';
        element.style.height = '100%';
      } else {
        place();
      }
      element.hidden = false;
      if (frame.current === null) {
        frame.current = requestAnimationFrame(loop);
      }
      onChangeRef.current(true);
    },
    hide: () => {
      const element = ref.current;
      if (frame.current !== null) {
        cancelAnimationFrame(frame.current);
        frame.current = null;
      }
      if (element === null || element.hidden) {
        return;
      }
      element.hidden = true;
      delete element.dataset['cmCover'];
      onChangeRef.current(false);
    },
  };

  return { ref, cover: coverRef.current };
}
