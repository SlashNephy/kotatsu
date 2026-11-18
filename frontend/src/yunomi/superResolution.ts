import type { PictureScaler } from '@mpeg2toh264/yadif';
import { useSyncExternalStore } from 'react';

import { createHDRCopy, setHDRDrawingBuffer, type HDRCopy } from './hdr';
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

// superResolutionStrengths は、選べる鮮鋭化の強さ。1 が最も弱く、4 が最も強い。
// RTX Video Super Resolution の品質の段と同じく、映像と画面に合わせて選べるようにする。
export const superResolutionStrengths = [1, 2, 3, 4] as const;
export type SuperResolutionStrength = (typeof superResolutionStrengths)[number];

// defaultSuperResolutionStrength は、既定の強さ。FSR の例と同じ鮮鋭化 (0.2 段弱める) にする。
export const defaultSuperResolutionStrength: SuperResolutionStrength = 3;

// sharpnessStops は、強さごとの、RCAS の鮮鋭化を弱める段数。0 が最も強く、1 段ごとに半分になる。
// 1 は 1/4、2 は 1/2、3 は FSR の例と同じ 0.2 段、4 は弱めない。
export const sharpnessStops: Record<SuperResolutionStrength, number> = { 1: 2, 2: 1, 3: 0.2, 4: 0 };

// strengthKey は、鮮鋭化の強さを覚えるキー。画面と映像の見え方は端末によるので、ほかの端末とは同期しない。
const strengthKey = 'kotatsu.player.superResolutionStrength';

// loadSuperResolutionStrength は、鮮鋭化の強さを読む。覚えていないか、知らない値か、読めなければ既定の強さを返す。
export function loadSuperResolutionStrength(
  storage: Pick<Storage, 'getItem'> | null,
): SuperResolutionStrength {
  try {
    const stored = Number(storage?.getItem(strengthKey));

    return (
      superResolutionStrengths.find((strength) => strength === stored) ??
      defaultSuperResolutionStrength
    );
  } catch {
    return defaultSuperResolutionStrength;
  }
}

function getStrengthSnapshot(): SuperResolutionStrength {
  return loadSuperResolutionStrength(browserStorage());
}

// useSuperResolutionStrength は、鮮鋭化の強さと、それを変える関数を返す。ほかのタブで変えても追う。
export function useSuperResolutionStrength(): [
  SuperResolutionStrength,
  (strength: SuperResolutionStrength) => void,
] {
  const strength = useSyncExternalStore(subscribe, getStrengthSnapshot);

  return [
    strength,
    (next) => {
      try {
        browserStorage()?.setItem(strengthKey, String(next));
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
// コンテキストを失っていると、getParameter は null を返すので、0 (描けない) とする。
function maxDimension(gl: WebGL2RenderingContext): number {
  const texture = gl.getParameter(gl.MAX_TEXTURE_SIZE) as number | null;
  const viewport = gl.getParameter(gl.MAX_VIEWPORT_DIMS) as Int32Array | null;
  if (texture === null || viewport === null) {
    return 0;
  }

  return Math.min(texture, viewport[0] ?? texture, viewport[1] ?? texture);
}

// YadifUpscaler は、yadif に渡す拡大と、今拡大している大きさを読む口と、片付ける口。
// 擬似 HDR の出力 (#1656) も、yadif が canvas へ描く最後の段なので、ここで描く。拡大しないときは、元の大きさのまま写す。
export type YadifUpscaler = PictureScaler & {
  // output は、最後に拡大して描いた大きさ。拡大していなければ null。
  output(): Size | null;
  // shown は、最後に大きさを決めたときの、映す大きさ (端末の画素、切り抜きの拡大率を掛けたもの)。
  // 元の大きさ以下に映していて拡大しないときに、その理由として見せる。決めていなければ null。
  shown(): Size | null;
  // hdr は、最後に描いたコマを、擬似 HDR で描いたか。
  hdr(): boolean;
  destroy(): void;
};

// createYadifUpscaler は、yadif に渡す拡大を作る。拡大の処理は、yadif が最初に描かせるときに、そのコンテキストで作る。
// 作れないか、途中で失敗したら、onUnavailable を呼び、以後は拡大しない (yadif はコマをそのまま描く)。
// yadif の描画の中で投げると、yadif のループが止まるので、ここで受け止める。
// zoom は、切り抜きの拡大率を、大きさを決めるたびに読む。strength は、鮮鋭化の強さを、描くたびに読む。
// upscale は、拡大するかを、大きさを決めるたびに読む。hdr は、擬似 HDR で描くかを、大きさを決めるときと描くたびに読む。
// 擬似 HDR で描けない (浮動小数点の描画先を作れないなど) ときは、onHDRUnavailable を呼び、以後は SDR で描く。
// create と createCopy と setDrawingBuffer は、テストで処理を差し替える口。
export function createYadifUpscaler({
  zoom = () => ({ x: 1, y: 1 }),
  strength = () => defaultSuperResolutionStrength,
  upscale = () => true,
  hdr = () => false,
  onUnavailable,
  onHDRUnavailable,
  create = createUpscaler,
  createCopy = createHDRCopy,
  setDrawingBuffer = setHDRDrawingBuffer,
}: {
  zoom?: () => { x: number; y: number };
  strength?: () => SuperResolutionStrength;
  upscale?: () => boolean;
  hdr?: () => boolean;
  onUnavailable?: (error: unknown) => void;
  onHDRUnavailable?: (error: unknown) => void;
  create?: (gl: WebGL2RenderingContext) => Upscaler;
  createCopy?: (gl: WebGL2RenderingContext) => HDRCopy;
  setDrawingBuffer?: typeof setHDRDrawingBuffer;
} = {}): YadifUpscaler {
  let context: WebGL2RenderingContext | null = null;
  let upscaler: Upscaler | null = null;
  let copy: HDRCopy | null = null;
  let broken = false;
  let output: Size | null = null;
  let shown: Size | null = null;
  // hdrBroken は、擬似 HDR で描けなかったこと。hdrDrawn は、描画先を擬似 HDR の形にしていること。
  let hdrBroken = false;
  let hdrDrawn = false;
  const wantsHDR = () => !hdrBroken && hdr();

  const release = () => {
    try {
      upscaler?.destroy();
      copy?.destroy();
    } catch {
      // コンテキストを失っていれば、片付けられなくてもよい。
    }
    upscaler = null;
    copy = null;
  };

  // prepareDrawingBuffer は、描く直前に、描画先を擬似 HDR で描くかに合わせ、伸ばして描くかを返す。
  // 描画先を変えられなければ、擬似 HDR をやめ、SDR の描画先に戻して描く。
  const prepareDrawingBuffer = (gl: WebGL2RenderingContext, width: number, height: number) => {
    const wanted = wantsHDR();
    if (!wanted && !hdrDrawn) {
      return false;
    }
    try {
      setDrawingBuffer(gl, wanted, width, height);
      hdrDrawn = wanted;
    } catch (error) {
      hdrBroken = true;
      try {
        setDrawingBuffer(gl, false, width, height);
      } catch {
        // 戻せなくても、伸ばさずに描けば SDR と同じ見え方になる。
      }
      hdrDrawn = false;
      onHDRUnavailable?.(error);
    }

    return hdrDrawn;
  };

  const fail = (error: unknown) => {
    broken = true;
    output = null;
    release();
    onUnavailable?.(error);
  };

  return {
    size(gl, width, height, boxWidth, boxHeight) {
      if (broken) {
        return null;
      }
      try {
        let size: Size | null = null;
        if (upscale()) {
          const scale = zoom();
          shown = {
            width: Math.round(boxWidth * scale.x),
            height: Math.round(boxHeight * scale.y),
          };
          size = upscaleSize(
            { width, height },
            { width: boxWidth, height: boxHeight },
            scale,
            maxDimension(gl),
          );
        } else {
          shown = null;
        }
        if (size === null) {
          output = null;
        }
        // 拡大しなくても、擬似 HDR で描くなら、元の大きさのまま描く (yadif にそのまま写させない)。
        if (size === null && wantsHDR()) {
          return { width, height };
        }

        return size;
      } catch (error) {
        fail(error);

        return null;
      }
    },
    draw(gl, texture, flipped, width, height, outputWidth, outputHeight) {
      if (broken) {
        return false;
      }
      try {
        if (context !== gl) {
          release();
          context = gl;
        }
        const expand = prepareDrawingBuffer(gl, outputWidth, outputHeight);
        if (outputWidth === width && outputHeight === height) {
          // 拡大しない。擬似 HDR のために、元の大きさのまま写す。
          copy ??= createCopy(gl);
          copy.draw(texture, flipped, width, height, expand);
          output = null;

          return true;
        }
        upscaler ??= create(gl);
        upscaler.draw(
          texture,
          flipped,
          width,
          height,
          outputWidth,
          outputHeight,
          sharpnessStops[strength()],
          expand,
        );
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
    shown() {
      return shown;
    },
    hdr() {
      return hdrDrawn;
    },
    destroy() {
      release();
      context = null;
      hdrDrawn = false;
      output = null;
      shown = null;
    },
  };
}
