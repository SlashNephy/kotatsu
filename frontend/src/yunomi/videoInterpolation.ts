import { createInterpolator, type Interpolator } from './interpolator';
import {
  continues,
  framePeriod,
  mediaTimeAt,
  pickFrames,
  type MediaClock,
} from './interpolationTiming';

// video の映像に、コマを補間した canvas を重ねる (#1655、設計書 7.5)。kyusu の経路で使う。
// video が新しいコマを出すたびにテクスチャへ写して前のコマとの動きを推し量り、画面を更新するたびに、
// 1 コマ遅らせた時刻のコマを作って描く。補間しないとき (一時停止、シーク、準備ができていない) は canvas を隠し、
// video をそのまま見せる。

// VideoInterpolation は、video に重ねた補間を操作する口。
export type VideoInterpolation = {
  // setEnabled は、補間するかを変える。切ると canvas を隠し、video をそのまま見せる。
  setEnabled(enabled: boolean): void;
  destroy(): void;
};

// slots は、写したコマを持つテクスチャの数。出している 2 コマのほかに、届いたばかりのコマを置く。
const slots = 3;
// periodSamples は、コマの間隔を求めるのに使う、最近のコマの数。
const periodSamples = 12;
// refreshSmoothing は、画面の更新の間隔を測るときに、新しい値を混ぜる割合。
const refreshSmoothing = 0.1;
// passthroughRatio は、コマの間隔が、画面の更新の間隔のこの倍より短ければ、補間しない割合。
// 60p の映像を 60 Hz の画面で見るときなどは、間を埋めるコマがなく、1 コマ遅れるだけになる。
const passthroughRatio = 1.5;

type Frame = { slot: number; mediaTime: number };

// attachVideoInterpolation は、video に補間の canvas を重ねる。canvas は video のすぐ後ろに置き、
// video と同じ className を付けて、同じ矩形に映す。WebGL2 を使えなければ onUnavailable を呼び、何もしない。
export function attachVideoInterpolation(
  video: HTMLVideoElement,
  {
    className,
    onUnavailable,
  }: {
    className: string;
    onUnavailable?: (error: unknown) => void;
  },
): VideoInterpolation {
  let enabled = false;
  let destroyed = false;
  let unavailable = false;
  let canvas: HTMLCanvasElement | null = null;
  let gl: WebGL2RenderingContext | null = null;
  let interpolator: Interpolator | null = null;
  let textures: WebGLTexture[] = [];
  // size は、映すときの大きさ (video の表示の大きさ)。frameSize は、届いたコマの大きさ。
  // 横に引き伸ばして映す映像 (1440×1080 を 16:9 で映すなど) では、2 つが違う。
  let size: { width: number; height: number } | null = null;
  let frameSize: { width: number; height: number } | null = null;
  // frames は、写したコマ。時刻の早い順に、最大 2 コマ持つ。
  let frames: Frame[] = [];
  // estimated は、動きを推し量った 2 コマ (frames の添字でなく、時刻で覚える)。
  let estimated: { previous: number; next: number } | null = null;
  let times: number[] = [];
  let clock: MediaClock | null = null;
  let refreshMs = 1_000 / 60;
  let lastLoopAt = 0;
  let frameHandle: number | null = null;
  let loopHandle: number | null = null;

  const hide = () => {
    if (canvas !== null) {
      canvas.style.visibility = 'hidden';
    }
  };

  const fail = (error: unknown) => {
    unavailable = true;
    stop();
    hide();
    onUnavailable?.(error);
  };

  const setup = (): boolean => {
    if (gl !== null) {
      return true;
    }
    if (unavailable) {
      return false;
    }
    try {
      const element = document.createElement('canvas');
      element.className = className;
      element.style.position = 'absolute';
      element.style.inset = '0';
      element.style.pointerEvents = 'none';
      element.style.visibility = 'hidden';
      const context = element.getContext('webgl2', {
        alpha: false,
        antialias: false,
        depth: false,
        stencil: false,
        preserveDrawingBuffer: false,
      });
      if (context === null) {
        throw new Error('WebGL2 is unavailable');
      }
      interpolator = createInterpolator(context);
      element.addEventListener('webglcontextlost', (event) => {
        event.preventDefault();
        fail(new Error('the WebGL context was lost'));
      });
      // 字幕の canvas より下に置くよう、video のすぐ後ろに入れる。
      video.after(element);
      canvas = element;
      gl = context;

      return true;
    } catch (error) {
      fail(error);

      return false;
    }
  };

  // resize は、映す大きさと届くコマの大きさが変わったら、canvas とテクスチャを作り直す。
  // テクスチャの大きさは、写すときにブラウザが決める (texImage2D) ので、ここでは作るだけにする。
  const resize = (width: number, height: number, frameWidth: number, frameHeight: number) => {
    if (
      gl === null ||
      canvas === null ||
      (size?.width === width &&
        size.height === height &&
        frameSize?.width === frameWidth &&
        frameSize.height === frameHeight)
    ) {
      return;
    }
    for (const texture of textures) {
      gl.deleteTexture(texture);
    }
    textures = [];
    for (let i = 0; i < slots; i++) {
      const texture = gl.createTexture();
      gl.bindTexture(gl.TEXTURE_2D, texture);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
      textures.push(texture);
    }
    canvas.width = width;
    canvas.height = height;
    size = { width, height };
    frameSize = { width: frameWidth, height: frameHeight };
    frames = [];
    estimated = null;
  };

  // onFrame は、video が新しいコマを出したときに、そのコマを写し、前のコマとの動きを推し量る。
  const onFrame = (_now: DOMHighResTimeStamp, metadata: VideoFrameCallbackMetadata) => {
    frameHandle = null;
    if (!enabled || destroyed || gl === null || interpolator === null) {
      return;
    }
    try {
      const width = video.videoWidth;
      const height = video.videoHeight;
      // rVFC の大きさは、届いたコマの大きさ。知らせないブラウザでは、映す大きさと同じとみなす。
      const frameWidth = metadata.width > 0 ? metadata.width : width;
      const frameHeight = metadata.height > 0 ? metadata.height : height;
      if (width > 0 && height > 0) {
        resize(width, height, frameWidth, frameHeight);
        const previous = frames.at(-1);
        if (previous !== undefined && !continues(previous.mediaTime, metadata.mediaTime)) {
          frames = [];
          estimated = null;
          times = [];
        }
        // 出している 2 コマに使っていないテクスチャへ写す。
        const used = new Set(frames.map((frame) => frame.slot));
        const slot = [...Array(slots).keys()].find((i) => !used.has(i)) ?? 0;
        const texture = textures[slot];
        if (texture !== undefined) {
          gl.bindTexture(gl.TEXTURE_2D, texture);
          gl.pixelStorei(gl.UNPACK_FLIP_Y_WEBGL, true);
          gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, gl.RGBA, gl.UNSIGNED_BYTE, video);
          gl.pixelStorei(gl.UNPACK_FLIP_Y_WEBGL, false);
          frames = [...frames, { slot, mediaTime: metadata.mediaTime }].slice(-2);
          const [from, to] = frames;
          const fromTexture = from === undefined ? undefined : textures[from.slot];
          const toTexture = to === undefined ? undefined : textures[to.slot];
          if (
            from !== undefined &&
            to !== undefined &&
            fromTexture !== undefined &&
            toTexture !== undefined
          ) {
            interpolator.estimate(fromTexture, toTexture, frameWidth, frameHeight);
            estimated = { previous: from.mediaTime, next: to.mediaTime };
          }
        }
        times = [...times, metadata.mediaTime].slice(-periodSamples);
        clock = {
          mediaTime: metadata.mediaTime,
          displayTime: metadata.expectedDisplayTime,
          rate: video.playbackRate,
        };
      }
    } catch (error) {
      fail(error);

      return;
    }
    frameHandle = video.requestVideoFrameCallback(onFrame);
  };

  // draw は、画面を更新するたびに、1 コマ遅らせた時刻のコマを描く。描けなければ canvas を隠す。
  const draw = (now: DOMHighResTimeStamp): boolean => {
    if (
      gl === null ||
      canvas === null ||
      interpolator === null ||
      size === null ||
      clock === null ||
      estimated === null ||
      video.paused ||
      video.ended ||
      video.seeking
    ) {
      return false;
    }
    const period = framePeriod(times);
    if (period === null || period * 1_000 < refreshMs * passthroughRatio) {
      return false;
    }
    // 今描いたものは、次に画面を更新したときに映る。
    const shown = mediaTimeAt(clock, now + refreshMs) - period;
    const pick = pickFrames(
      frames.map(({ mediaTime }) => ({ mediaTime })),
      shown,
    );
    const from = pick === null ? undefined : frames[pick.from];
    const to = pick === null ? undefined : frames[pick.to];
    if (pick === null || from === undefined || to === undefined) {
      return false;
    }
    let t: number;
    if (from.mediaTime === estimated.previous && to.mediaTime === estimated.next) {
      t = pick.t;
    } else if (from === to && from.mediaTime === estimated.previous) {
      t = 0;
    } else if (from === to && from.mediaTime === estimated.next) {
      t = 1;
    } else {
      return false;
    }
    interpolator.compose(t, null, size.width, size.height);

    return true;
  };

  const loop = (now: DOMHighResTimeStamp) => {
    loopHandle = null;
    if (!enabled || destroyed) {
      return;
    }
    const step = now - lastLoopAt;
    lastLoopAt = now;
    // 画面の更新の間隔は、タブを裏に回したときなどの長い間を除いて、なだらかに追う。
    if (step > 0 && step < 100) {
      refreshMs += (step - refreshMs) * refreshSmoothing;
    }
    try {
      const drawn = draw(now);
      if (canvas !== null) {
        canvas.style.visibility = drawn ? 'visible' : 'hidden';
      }
    } catch (error) {
      fail(error);

      return;
    }
    loopHandle = requestAnimationFrame(loop);
  };

  const start = () => {
    if (!setup()) {
      return;
    }
    frameHandle ??= video.requestVideoFrameCallback(onFrame);
    loopHandle ??= requestAnimationFrame(loop);
  };

  function stop() {
    if (frameHandle !== null) {
      video.cancelVideoFrameCallback(frameHandle);
      frameHandle = null;
    }
    if (loopHandle !== null) {
      cancelAnimationFrame(loopHandle);
      loopHandle = null;
    }
    frames = [];
    estimated = null;
    times = [];
    clock = null;
  }

  return {
    setEnabled(next) {
      if (destroyed || next === enabled) {
        return;
      }
      enabled = next;
      if (enabled) {
        start();
      } else {
        stop();
        hide();
      }
    },
    destroy() {
      destroyed = true;
      enabled = false;
      stop();
      interpolator?.destroy();
      if (gl !== null) {
        for (const texture of textures) {
          gl.deleteTexture(texture);
        }
      }
      canvas?.remove();
      interpolator = null;
      gl = null;
      canvas = null;
      textures = [];
    },
  };
}
