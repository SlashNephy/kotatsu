import type { PictureScaler } from '@mpeg2toh264/yadif';
import { useSyncExternalStore } from 'react';

import { createUpscaler, type Upscaler } from './upscaler';
import { browserStorage } from './volume';

// 映像の拡大 (#1654、設計書 7.2、7.5)。yadif が解除したコマを、映像を描く矩形の端末の画素の大きさへ、
// 縁を見て拡大し、鮮鋭化してから描く。ブラウザに引き伸ばさせるより、縁がぼやけない。

// storageKey は、映像を拡大するかを覚えるキー。GPU の負荷は端末によるので、ほかの端末とは同期しない。
const storageKey = 'kotatsu.player.superResolution';
// changeEvent は、同じタブの中で変えたことを知らせるイベント。storage イベントは、ほかのタブにしか届かない。
const changeEvent = 'kotatsu:super-resolution';

// loadSuperResolution は、映像を拡大するかを読む。GPU の負荷が増え、鮮鋭化はノイズも目立たせるので、既定では拡大しない。
// 読めなければ、既定の false を返す。
export function loadSuperResolution(storage: Pick<Storage, 'getItem'> | null): boolean {
  try {
    return storage?.getItem(storageKey) === 'true';
  } catch {
    return false;
  }
}

function subscribe(onChange: () => void): () => void {
  window.addEventListener('storage', onChange);
  window.addEventListener(changeEvent, onChange);

  return () => {
    window.removeEventListener('storage', onChange);
    window.removeEventListener(changeEvent, onChange);
  };
}

function getSnapshot(): boolean {
  return loadSuperResolution(browserStorage());
}

// useSuperResolution は、映像を拡大するかと、それを変える関数を返す。ほかのタブで変えても追う。
export function useSuperResolution(): [boolean, (enabled: boolean) => void] {
  const enabled = useSyncExternalStore(subscribe, getSnapshot);

  return [
    enabled,
    (next) => {
      try {
        browserStorage()?.setItem(storageKey, String(next));
      } catch {
        // 書けなければ、次に開いたときは既定に戻るだけ。
      }
      window.dispatchEvent(new Event(changeEvent));
    },
  ];
}

// Size は、画素の幅と高さ。
export type Size = { width: number; height: number };

// upscaleSize は、source の大きさのコマを、端末の画素で box の大きさに描くときに、拡大して描く大きさを返す。
// zoom は、切り抜き (#496) で掛けている CSS の拡大率。box は拡大する前の大きさなので、掛けて実際に映る大きさにする。
// 縦横のどちらも元の大きさ以下なら、拡大しないので null を返す (小さなプレイヤーや、縮めて映すとき)。
// 片方だけが大きいとき (1440×1080 を 1920×1080 に映すなど) は、小さい方を元の大きさのままにし、その向きだけ拡大する。
// maxDimension (GPU が描ける一辺の上限) を超えるなら、縦横の比を保って縮める。
export function upscaleSize(
  source: Size,
  box: Size,
  zoom: { x: number; y: number },
  maxDimension: number,
): Size | null {
  const values = [source.width, source.height, box.width, box.height, zoom.x, zoom.y, maxDimension];
  if (values.some((value) => !Number.isFinite(value) || value <= 0)) {
    return null;
  }
  const width = Math.max(Math.round(box.width * zoom.x), source.width);
  const height = Math.max(Math.round(box.height * zoom.y), source.height);
  const fit = Math.min(1, maxDimension / width, maxDimension / height);
  // 縮めても、元の大きさより小さくはしない (EASU は縮めるためのものではない)。
  const output = {
    width: Math.max(Math.floor(width * fit), source.width),
    height: Math.max(Math.floor(height * fit), source.height),
  };
  if (output.width <= source.width && output.height <= source.height) {
    return null;
  }

  return output;
}

// maxDimension は、gl が描ける一辺の上限 (テクスチャと viewport の小さい方)。
function maxDimension(gl: WebGL2RenderingContext): number {
  const texture = gl.getParameter(gl.MAX_TEXTURE_SIZE) as number;
  const viewport = gl.getParameter(gl.MAX_VIEWPORT_DIMS) as Int32Array;

  return Math.min(texture, viewport[0] ?? texture, viewport[1] ?? texture);
}

// YadifUpscaler は、yadif に渡す拡大と、今拡大している大きさを読む口と、片付ける口。
export type YadifUpscaler = PictureScaler & {
  // output は、最後に拡大して描いた大きさ。拡大していなければ null。
  output(): Size | null;
  destroy(): void;
};

// createYadifUpscaler は、yadif に渡す拡大を作る。拡大の処理は、yadif が最初に描かせるときに、そのコンテキストで作る。
// 作れないか、途中で失敗したら、onUnavailable を呼び、以後は拡大しない (yadif はコマをそのまま描く)。
// yadif の描画の中で投げると、yadif のループが止まるので、ここで受け止める。
// zoom は、切り抜きの拡大率を、大きさを決めるたびに読む。create は、テストで拡大の処理を差し替える口。
export function createYadifUpscaler({
  zoom = () => ({ x: 1, y: 1 }),
  onUnavailable,
  create = createUpscaler,
}: {
  zoom?: () => { x: number; y: number };
  onUnavailable?: (error: unknown) => void;
  create?: (gl: WebGL2RenderingContext) => Upscaler;
} = {}): YadifUpscaler {
  let context: WebGL2RenderingContext | null = null;
  let upscaler: Upscaler | null = null;
  let broken = false;
  let output: Size | null = null;

  const fail = (error: unknown) => {
    broken = true;
    output = null;
    try {
      upscaler?.destroy();
    } catch {
      // コンテキストを失っていれば、片付けられなくてもよい。
    }
    upscaler = null;
    onUnavailable?.(error);
  };

  return {
    size(gl, width, height, boxWidth, boxHeight) {
      if (broken) {
        return null;
      }
      const size = upscaleSize(
        { width, height },
        { width: boxWidth, height: boxHeight },
        zoom(),
        maxDimension(gl),
      );
      if (size === null) {
        output = null;
      }

      return size;
    },
    draw(gl, texture, flipped, width, height, outputWidth, outputHeight) {
      if (broken) {
        return false;
      }
      try {
        if (context !== gl || upscaler === null) {
          upscaler?.destroy();
          upscaler = create(gl);
          context = gl;
        }
        upscaler.draw(texture, flipped, width, height, outputWidth, outputHeight);
        output = { width: outputWidth, height: outputHeight };

        return true;
      } catch (error) {
        fail(error);

        return false;
      }
    },
    output() {
      return output;
    },
    destroy() {
      try {
        upscaler?.destroy();
      } catch {
        // コンテキストを失っていれば、片付けられなくてもよい。
      }
      upscaler = null;
      context = null;
      output = null;
    },
  };
}
