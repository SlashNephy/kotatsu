import { useSyncExternalStore } from 'react';

import { compileProgram, restoreState, saveState } from './glPass';
import { browserStorage } from './volume';

// 擬似 HDR の出力 (#1656、設計書 7.2、7.5)。yadif が解除したコマの明るい所を、SDR の白より明るく伸ばして描く。
// yadif の WebGL2 の描画先を RGBA16F にし、トーンマッピングを extended にすると、1 を超える値が白より明るく出る
// (WebGL の drawingBufferToneMapping。https://github.com/KhronosGroup/WebGL/pull/3668)。
// 持たないブラウザ (Chromium 158 でも既定では無効) と、HDR でない画面では、今までどおり SDR で描く。

// storageKey は、擬似 HDR で出すかを覚えるキー。画面は端末によるので、ほかの端末とは同期しない。
const storageKey = 'kotatsu.player.hdrOutput';
// changeEvent は、同じタブの中で変えたことを知らせるイベント。storage イベントは、ほかのタブにしか届かない。
const changeEvent = 'kotatsu:hdr-output';
// hdrDisplayQuery は、HDR を出せる画面かを問うメディアクエリ。
const hdrDisplayQuery = '(dynamic-range: high)';

// ToneMappedContext は、型の定義にまだない、描画先の形式とトーンマッピングを変える口を持つコンテキスト。
type ToneMappedContext = WebGL2RenderingContext & {
  readonly drawingBufferFormat: GLenum;
  drawingBufferStorage(format: GLenum, width: number, height: number): void;
  drawingBufferToneMapping(toneMapping: { mode: 'standard' | 'extended' }): void;
};

// hdrOutputSupported は、ブラウザが、WebGL2 の描画先を浮動小数点にし、白より明るく出せるか。
export function hdrOutputSupported(): boolean {
  if (typeof WebGL2RenderingContext === 'undefined') {
    return false;
  }
  const prototype = WebGL2RenderingContext.prototype;

  return 'drawingBufferToneMapping' in prototype && 'drawingBufferStorage' in prototype;
}

// loadHDROutput は、擬似 HDR で出すかを読む。見え方が元の映像と変わるので、既定では出さない。
// 読めなければ、既定の false を返す。
export function loadHDROutput(storage: Pick<Storage, 'getItem'> | null): boolean {
  try {
    return storage?.getItem(storageKey) === 'true';
  } catch {
    return false;
  }
}

function subscribeSetting(onChange: () => void): () => void {
  window.addEventListener('storage', onChange);
  window.addEventListener(changeEvent, onChange);

  return () => {
    window.removeEventListener('storage', onChange);
    window.removeEventListener(changeEvent, onChange);
  };
}

function getSettingSnapshot(): boolean {
  return loadHDROutput(browserStorage());
}

// useHDROutput は、擬似 HDR で出すかと、それを変える関数を返す。ほかのタブで変えても追う。
export function useHDROutput(): [boolean, (enabled: boolean) => void] {
  const enabled = useSyncExternalStore(subscribeSetting, getSettingSnapshot);

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

function subscribeDisplay(onChange: () => void): () => void {
  const query = window.matchMedia(hdrDisplayQuery);
  query.addEventListener('change', onChange);

  return () => {
    query.removeEventListener('change', onChange);
  };
}

function getDisplaySnapshot(): boolean {
  return window.matchMedia(hdrDisplayQuery).matches;
}

// useHDRDisplay は、今の画面が HDR を出せるかを返す。ウィンドウを別の画面へ移したときも追う。
export function useHDRDisplay(): boolean {
  return useSyncExternalStore(subscribeDisplay, getDisplaySnapshot, () => false);
}

// expandHighlightsShader は、sRGB で符号化した色の明るい所を、白より明るく伸ばす GLSL の関数。
// 線形の輝度が hdrKnee までは変えず、そこから 1 (SDR の白) へ向けて、滑らかに hdrPeak 倍まで伸ばす。
// 色相を変えないよう、RGB に同じ倍率を掛ける。extended の描画先は、1 を超える値も sRGB の符号化のまま読むので、
// 伸ばした線形の値を、sRGB の曲線をそのまま 1 の先へ延ばして符号化し直す。
export const expandHighlightsShader = `
const float hdrKnee = 0.5;
const float hdrPeak = 2.0;

vec3 srgbToLinear(vec3 c) {
  return mix(c / 12.92, pow((c + 0.055) / 1.055, vec3(2.4)), step(vec3(0.04045), c));
}

vec3 linearToSRGB(vec3 c) {
  return mix(c * 12.92, 1.055 * pow(c, vec3(1.0 / 2.4)) - 0.055, step(vec3(0.0031308), c));
}

vec3 expandHighlights(vec3 c) {
  vec3 l = srgbToLinear(clamp(c, 0.0, 1.0));
  float y = dot(l, vec3(0.2126, 0.7152, 0.0722));
  float s = smoothstep(hdrKnee, 1.0, y);
  return linearToSRGB(l * (1.0 + (hdrPeak - 1.0) * s * s));
}
`;

// copyShader は、テクスチャを、同じ大きさの canvas へ写す。uExpand なら、明るい所を伸ばす。
// 行は下から数える (framebuffer と同じ向き)。uFlip のテクスチャは行を上から持つので、読むときに返す。
const copyShader = `#version 300 es
precision highp float;
precision highp int;
uniform sampler2D uSource;
uniform bool uFlip;
uniform bool uExpand;
out vec4 color;
${expandHighlightsShader}
void main() {
  ivec2 size = textureSize(uSource, 0);
  ivec2 p = ivec2(gl_FragCoord.xy);
  if (uFlip) {
    p.y = size.y - 1 - p.y;
  }
  vec3 c = texelFetch(uSource, p, 0).rgb;
  color = vec4(uExpand ? expandHighlights(c) : c, 1.0);
}
`;

// HDRCopy は、拡大せずに、明るい所を伸ばして canvas へ写す処理。擬似 HDR をやめた直後のコマは、伸ばさずに写す。
export type HDRCopy = {
  // draw は、width × height の texture を、同じ大きさの canvas (既定の framebuffer) へ写す。
  // texture は行を下から持つ。flipped なら行を上から持つ (video を写したテクスチャ)。expand なら、明るい所を伸ばす。
  draw(
    texture: WebGLTexture,
    flipped: boolean,
    width: number,
    height: number,
    expand: boolean,
  ): void;
  destroy(): void;
};

// createHDRCopy は、gl に写すシェーダーを作る。作れなければ投げる。変えた GL の状態は戻す。
export function createHDRCopy(gl: WebGL2RenderingContext): HDRCopy {
  const program = compileProgram(gl, copyShader);
  const location = {
    source: gl.getUniformLocation(program, 'uSource'),
    flip: gl.getUniformLocation(program, 'uFlip'),
    expand: gl.getUniformLocation(program, 'uExpand'),
  };
  const vertexArray = gl.createVertexArray();

  return {
    draw(texture, flipped, width, height, expand) {
      const state = saveState(gl, 1);
      try {
        gl.disable(gl.BLEND);
        gl.disable(gl.SCISSOR_TEST);
        gl.disable(gl.DEPTH_TEST);
        gl.bindVertexArray(vertexArray);
        gl.activeTexture(gl.TEXTURE0);
        gl.bindSampler(0, null);
        gl.useProgram(program);
        gl.bindTexture(gl.TEXTURE_2D, texture);
        gl.uniform1i(location.source, 0);
        gl.uniform1i(location.flip, flipped ? 1 : 0);
        gl.uniform1i(location.expand, expand ? 1 : 0);
        gl.bindFramebuffer(gl.FRAMEBUFFER, null);
        gl.viewport(0, 0, width, height);
        gl.drawArrays(gl.TRIANGLES, 0, 3);
      } finally {
        restoreState(gl, state);
      }
    },
    destroy() {
      gl.deleteVertexArray(vertexArray);
      gl.deleteProgram(program);
    },
  };
}

// setHDRDrawingBuffer は、gl の描画先 (canvas) を、hdr なら RGBA16F と extended のトーンマッピングに、
// そうでなければ RGBA8 と standard に変える。既にその形なら何もしない。変えると描画先の中身は消えるので、
// 描く直前に呼ぶ。width と height は、canvas の大きさ。
// 浮動小数点の描画先を作れない (EXT_color_buffer_float がない) か、口を持たなければ投げる。
export function setHDRDrawingBuffer(
  gl: WebGL2RenderingContext,
  hdr: boolean,
  width: number,
  height: number,
): void {
  const context = gl as ToneMappedContext;
  // 描画先の形式は、コンテキストを失って戻ったときにも RGBA8 に戻るので、トーンマッピングも形式で見分ける。
  const current = context.drawingBufferFormat === gl.RGBA16F;
  if (current === hdr) {
    return;
  }
  if (hdr && gl.getExtension('EXT_color_buffer_float') === null) {
    throw new Error('the drawing buffer cannot hold floating point colors');
  }
  // alpha のないコンテキストなどでは、形式を変えられず、GL のエラーになるだけで投げない。変わったかを確かめてから、
  // トーンマッピングを変える。
  context.drawingBufferStorage(hdr ? gl.RGBA16F : gl.RGBA8, width, height);
  if (context.drawingBufferFormat !== (hdr ? gl.RGBA16F : gl.RGBA8)) {
    throw new Error('failed to change the drawing buffer format');
  }
  context.drawingBufferToneMapping({ mode: hdr ? 'extended' : 'standard' });
}
