import { createInterpolator, type Interpolator } from './interpolator';
import {
  continues,
  createDrawPacer,
  framePeriod,
  mediaTimeAt,
  pickFrames,
  type MediaClock,
} from './interpolationTiming';
import type { Size, YadifUpscaler } from './superResolution';

// video の映像に、コマの補間 (#1655) と拡大 (#2113) をした canvas を重ねる (設計書 7.5)。kyusu の経路で使う。
// video が新しいコマを出すたびにテクスチャへコピーし、補間するなら前のコマとの動きを推し量る。画面を更新するたびに、
// 補間するなら 1 コマ遅らせた時刻のコマを作り、拡大するなら映す大きさへ拡大して描く。
// 補間と拡大は、同じ WebGL2 のコンテキストの中で続けて行う。別の canvas に分けると、拡大した絵が補間に渡らないからである。
// どちらもしないとき (一時停止やシークで補間せず、拡大もしない、準備ができていない) は canvas を隠し、video をそのまま見せる。

// VideoOverlay は、video に重ねた補間と拡大を操作する口。
export type VideoOverlay = {
  // setInterpolate は、補間するかを変える。
  setInterpolate(enabled: boolean): void;
  // setUpscale は、拡大するかを変える。
  setUpscale(enabled: boolean): void;
  // refresh は、拡大の大きさと、止めている間のコマを描き直す。切り抜きや鮮鋭化の強さ、拡大の方式を変えたときに呼ぶ。
  refresh(): void;
  destroy(): void;
};

// slots は、コピーしたコマを持つテクスチャの数。出している 2 コマのほかに、届いたばかりのコマを置く。
const slots = 3;
// periodSamples は、コマの間隔を求めるのに使う、最近のコマの数。
const periodSamples = 12;
// refreshSmoothing は、画面の更新の間隔を計測するときに、新しい値を混ぜる割合。
const refreshSmoothing = 0.1;
// passthroughRatio は、コマの実時間の間隔が、画面の更新の間隔のこの倍より短ければ、補間しない割合。
// 60p の映像を 60 Hz の画面で見るときや、30p を倍速で見るときなどは、間を埋めるコマがなく、1 コマ遅れるだけになる。
const passthroughRatio = 1.5;
// maxUpscaleHeight は、拡大するコマの高さの上限 (含まない)。4K のコマ (変換しない画質) は、HDR のことがあり、
// RGBA8 の canvas に描くと SDR になるので、拡大しない。
const maxUpscaleHeight = 2160;

type Frame = { slot: number; mediaTime: number };

// containSize は、picture の比率の絵を、box に収めて (object-fit: contain) 映すときの大きさを返す。
// 補間と拡大の canvas は video と同じく枠いっぱいに置き、絵は CSS で枠の中に収めるので、
// 絵を映す大きさは、枠の大きさではなく、比率を保って収めた大きさになる。
export function containSize(picture: Size, box: Size): Size | null {
  if (picture.width <= 0 || picture.height <= 0 || box.width <= 0 || box.height <= 0) {
    return null;
  }
  const scale = Math.min(box.width / picture.width, box.height / picture.height);

  return {
    width: Math.round(picture.width * scale),
    height: Math.round(picture.height * scale),
  };
}

// deviceBox は、ResizeObserver が通知した canvas の大きさを、端末の画素で返す。
// devicePixelContentBoxSize は正確だが、端末の画素の比率を差し替えたとき (開発者ツールや自動操作) は CSS の大きさのままになるので、
// CSS の大きさに比率を掛けた値と 1 画素より大きく食い違えば、掛けた値を使う (yadif のフックと同じ)。
export function deviceBox(
  css: Size,
  device: { inlineSize: number; blockSize: number } | undefined,
  ratio: number,
): Size {
  const scaled = { width: Math.round(css.width * ratio), height: Math.round(css.height * ratio) };
  if (
    device !== undefined &&
    Math.abs(device.inlineSize - scaled.width) <= 1 &&
    Math.abs(device.blockSize - scaled.height) <= 1
  ) {
    return { width: device.inlineSize, height: device.blockSize };
  }

  return scaled;
}

// attachVideoOverlay は、video に補間と拡大の canvas を重ねる。canvas は video のすぐ後ろに置き、
// video と同じ className を付けて、同じ矩形に映す。WebGL2 を使えなければ onUnavailable を呼び、何もしない。
// upscaler は、拡大に使う処理 (変換の経路と同じもの)。大きさを決めるのも描くのも、これに任せる。
export function attachVideoOverlay(
  video: HTMLVideoElement,
  {
    className,
    onUnavailable,
    maxRate = () => null,
    upscaler = null,
  }: {
    className: string;
    onUnavailable?: (error: unknown) => void;
    // maxRate は、補間したコマを出す回数の上限 (1 秒あたり)。null なら、画面の書き換えごとに描く。
    maxRate?: () => number | null;
    upscaler?: YadifUpscaler | null;
  },
): VideoOverlay {
  let interpolate = false;
  let upscale = false;
  let destroyed = false;
  let unavailable = false;
  let canvas: HTMLCanvasElement | null = null;
  let gl: WebGL2RenderingContext | null = null;
  let interpolator: Interpolator | null = null;
  let resizes: ResizeObserver | null = null;
  let textures: WebGLTexture[] = [];
  // size は、映すときの大きさ (video の表示の大きさ)。frameSize は、届いたコマの大きさ。
  // 横に引き伸ばして映す映像 (1440×1080 を 16:9 で映すなど) では、2 つが違う。
  let size: Size | null = null;
  let frameSize: Size | null = null;
  // box は、canvas の矩形の、端末の画素での大きさ。output は、拡大して描く大きさ。拡大しなければ null。
  let box: Size | null = null;
  let output: Size | null = null;
  // composed は、拡大するときに、補間したコマを描いておくテクスチャ (コマの大きさ)。
  let composed: { texture: WebGLTexture; framebuffer: WebGLFramebuffer; size: Size } | null = null;
  // stale は、拡大して描いたコマが古いこと。新しいコマが届いたか、canvas を作り直したか、補間したコマを描いた後に立てる。
  let stale = true;
  // frames は、コピーしたコマ。時刻の早い順に、最大 2 コマ持つ。
  let frames: Frame[] = [];
  // estimated は、動きを推し量った 2 コマ (frames の添字でなく、時刻で保持する)。
  let estimated: { previous: number; next: number } | null = null;
  let times: number[] = [];
  let clock: MediaClock | null = null;
  let refreshMs = 1_000 / 60;
  let lastLoopAt = 0;
  const pacer = createDrawPacer();
  let frameHandle: number | null = null;
  let loopHandle: number | null = null;

  const active = () => interpolate || upscale;

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
      // コマが出るたびに通知する口がなければ、コピーする時機が分からない。
      if (typeof video.requestVideoFrameCallback !== 'function') {
        throw new Error('requestVideoFrameCallback is unavailable');
      }
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
      // 拡大する大きさは、canvas を映す大きさ (端末の画素) で決める。ブラウザの拡大率や画面を変えても追う。
      resizes = new ResizeObserver(([entry]) => {
        if (entry === undefined) {
          return;
        }
        box = deviceBox(
          { width: entry.contentRect.width, height: entry.contentRect.height },
          entry.devicePixelContentBoxSize?.[0],
          window.devicePixelRatio || 1,
        );
        fit();
      });
      resizes.observe(element, { box: 'device-pixel-content-box' });
      video.addEventListener('seeking', onSeeking);
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

  const freeComposed = () => {
    if (composed !== null && gl !== null) {
      gl.deleteTexture(composed.texture);
      gl.deleteFramebuffer(composed.framebuffer);
    }
    composed = null;
  };

  // fit は、拡大して描く大きさを決め、canvas の大きさを合わせる。canvas の大きさを変えると描画が消えるので、描き直させる。
  const fit = () => {
    if (gl === null || canvas === null || size === null || frameSize === null) {
      return;
    }
    output = null;
    if (upscale && upscaler !== null && box !== null && frameSize.height < maxUpscaleHeight) {
      const shown = containSize(size, box);
      if (shown !== null) {
        output = upscaler.size(gl, frameSize.width, frameSize.height, shown.width, shown.height);
      }
    }
    if (output === null) {
      freeComposed();
    }
    const target = output ?? size;
    if (canvas.width !== target.width || canvas.height !== target.height) {
      canvas.width = target.width;
      canvas.height = target.height;
    }
    stale = true;
  };

  // resize は、映す大きさと届くコマの大きさが変わったら、テクスチャを作り直す。
  // テクスチャの大きさは、コピーするときにブラウザが決める (texImage2D) ので、ここでは作るだけにする。
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
    freeComposed();
    size = { width, height };
    frameSize = { width: frameWidth, height: frameHeight };
    frames = [];
    estimated = null;
    fit();
  };

  // composeTarget は、拡大するときに補間したコマを描く、コマの大きさのテクスチャを返す。
  const composeTarget = (context: WebGL2RenderingContext, frame: Size) => {
    if (
      composed !== null &&
      composed.size.width === frame.width &&
      composed.size.height === frame.height
    ) {
      return composed;
    }
    freeComposed();
    const texture = context.createTexture();
    context.bindTexture(context.TEXTURE_2D, texture);
    context.texStorage2D(context.TEXTURE_2D, 1, context.RGBA8, frame.width, frame.height);
    context.texParameteri(context.TEXTURE_2D, context.TEXTURE_MIN_FILTER, context.LINEAR);
    context.texParameteri(context.TEXTURE_2D, context.TEXTURE_MAG_FILTER, context.LINEAR);
    context.texParameteri(context.TEXTURE_2D, context.TEXTURE_WRAP_S, context.CLAMP_TO_EDGE);
    context.texParameteri(context.TEXTURE_2D, context.TEXTURE_WRAP_T, context.CLAMP_TO_EDGE);
    const framebuffer = context.createFramebuffer();
    context.bindFramebuffer(context.FRAMEBUFFER, framebuffer);
    context.framebufferTexture2D(
      context.FRAMEBUFFER,
      context.COLOR_ATTACHMENT0,
      context.TEXTURE_2D,
      texture,
      0,
    );
    context.bindFramebuffer(context.FRAMEBUFFER, null);
    composed = { texture, framebuffer, size: frame };

    return composed;
  };

  // upscaleTo は、テクスチャ (行を下から持つ、コマの大きさ) を、canvas へ拡大して描く。描けなければ、拡大をやめる。
  const upscaleTo = (
    context: WebGL2RenderingContext,
    texture: WebGLTexture,
    frame: Size,
  ): boolean => {
    if (upscaler === null || output === null) {
      return false;
    }
    context.bindFramebuffer(context.FRAMEBUFFER, null);
    if (
      upscaler.draw(context, texture, false, frame.width, frame.height, output.width, output.height)
    ) {
      return true;
    }
    // 拡大の処理は、失敗したら以後は大きさを返さない。元の大きさの canvas に戻す。
    fit();

    return false;
  };

  // onSeeking は、シークを始めたら、前の位置のコマを破棄する。シークした先のコマが届くまで、canvas を隠して video を見せる。
  const onSeeking = () => {
    frames = [];
    estimated = null;
    times = [];
  };

  // onFrame は、video が新しいコマを出したときに、そのコマをコピーし、補間するなら前のコマとの動きを推し量る。
  const onFrame = (_now: DOMHighResTimeStamp, metadata: VideoFrameCallbackMetadata) => {
    frameHandle = null;
    if (!active() || destroyed || gl === null || interpolator === null) {
      return;
    }
    try {
      const width = video.videoWidth;
      const height = video.videoHeight;
      // rVFC の大きさは、届いたコマの大きさ。通知しないブラウザでは、映す大きさと同じとみなす。
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
        // 出している 2 コマに使っていないテクスチャへコピーする。
        const used = new Set(frames.map((frame) => frame.slot));
        const slot = [...Array(slots).keys()].find((i) => !used.has(i)) ?? 0;
        const texture = textures[slot];
        if (texture !== undefined) {
          gl.bindTexture(gl.TEXTURE_2D, texture);
          gl.pixelStorei(gl.UNPACK_FLIP_Y_WEBGL, true);
          gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, gl.RGBA, gl.UNSIGNED_BYTE, video);
          gl.pixelStorei(gl.UNPACK_FLIP_Y_WEBGL, false);
          frames = [...frames, { slot, mediaTime: metadata.mediaTime }].slice(-2);
          stale = true;
          const [from, to] = frames;
          const fromTexture = from === undefined ? undefined : textures[from.slot];
          const toTexture = to === undefined ? undefined : textures[to.slot];
          if (
            interpolate &&
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

  // drawInterpolated は、画面を更新するたびに、1 コマ遅らせた時刻のコマを描く。補間しないなら false を返す。
  const drawInterpolated = (now: DOMHighResTimeStamp): boolean => {
    if (
      !interpolate ||
      gl === null ||
      interpolator === null ||
      size === null ||
      frameSize === null ||
      clock === null ||
      estimated === null ||
      video.paused ||
      video.ended
    ) {
      return false;
    }
    // コマの間隔は映像の時刻で計測するので、再生の速さで割って、実時間の間隔にして比較する。
    const period = framePeriod(times);
    if (
      period === null ||
      clock.rate <= 0 ||
      (period / clock.rate) * 1_000 < refreshMs * passthroughRatio
    ) {
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
    // 上限より速く描かない。描かない書き換えでは、前に描いたコマがそのまま映る。
    if (!pacer.shouldDraw(now, maxRate(), refreshMs)) {
      return true;
    }
    if (output === null) {
      interpolator.compose(t, null, size.width, size.height);

      return true;
    }
    // 拡大するときは、補間したコマをコマの大きさのテクスチャに描いてから、拡大して描く。
    const target = composeTarget(gl, frameSize);
    interpolator.compose(t, target.framebuffer, frameSize.width, frameSize.height);
    if (!upscaleTo(gl, target.texture, frameSize)) {
      interpolator.compose(t, null, size.width, size.height);
    }
    // 補間をやめたら、最後のコマを拡大して描き直す。
    stale = true;

    return true;
  };

  // drawUpscaled は、補間しないときに、最後に届いたコマを拡大して描く。拡大しないなら false を返す。
  // 届いたコマと大きさが変わらなければ、描き直さない (canvas には前に描いたコマが残る)。
  const drawUpscaled = (): boolean => {
    if (gl === null || frameSize === null || output === null) {
      return false;
    }
    const last = frames.at(-1);
    const texture = last === undefined ? undefined : textures[last.slot];
    if (texture === undefined) {
      return false;
    }
    if (!stale) {
      return true;
    }
    if (!upscaleTo(gl, texture, frameSize)) {
      return false;
    }
    stale = false;

    return true;
  };

  const loop = (now: DOMHighResTimeStamp) => {
    loopHandle = null;
    if (!active() || destroyed) {
      return;
    }
    const step = now - lastLoopAt;
    lastLoopAt = now;
    // 画面の更新の間隔は、タブを裏に回したときなどの長い間を除いて、なだらかに追う。
    if (step > 0 && step < 100) {
      refreshMs += (step - refreshMs) * refreshSmoothing;
    }
    try {
      // シークの間は、video が出す新しいコマを待つ。
      const drawn = !video.seeking && (drawInterpolated(now) || drawUpscaled());
      if (!drawn) {
        pacer.reset();
      }
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
    fit();
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

  const update = () => {
    if (destroyed) {
      return;
    }
    if (!active()) {
      stop();
      hide();

      return;
    }
    start();
  };

  return {
    setInterpolate(next) {
      if (next === interpolate) {
        return;
      }
      interpolate = next;
      estimated = null;
      update();
    },
    setUpscale(next) {
      if (next === upscale) {
        return;
      }
      upscale = next;
      update();
      fit();
    },
    refresh() {
      fit();
    },
    destroy() {
      destroyed = true;
      interpolate = false;
      upscale = false;
      stop();
      resizes?.disconnect();
      video.removeEventListener('seeking', onSeeking);
      upscaler?.destroy();
      interpolator?.destroy();
      freeComposed();
      if (gl !== null) {
        for (const texture of textures) {
          gl.deleteTexture(texture);
        }
      }
      canvas?.remove();
      resizes = null;
      interpolator = null;
      gl = null;
      canvas = null;
      textures = [];
    },
  };
}
