import { compileProgram, restoreState, saveState } from './glPass';

// 擬似 HDR の出力 (#1656、設計書 7.2、7.5) の、WebGL2 で描く部分。worker (デコードの経路の描画) からも使うので、
// DOM と React に触れない。設定と画面の判定は hdr.ts にある。
// 描画先を RGBA16F にし、トーンマッピングを extended にすると、1 を超える値が白より明るく出る
// (WebGL の drawingBufferToneMapping。https://github.com/KhronosGroup/WebGL/pull/3668)。

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

// expandHighlightsShader は、sRGB で符号化した色の明るい所を、白より明るく伸ばす GLSL の関数。
// 線形の輝度が hdrKnee までは変えず、そこから 1 (SDR の白) へ向けて、滑らかに hdrPeak 倍まで伸ばす。
// 色相を変えないよう、RGB に同じ倍率を掛ける。extended の描画先も、値を sRGB の符号化のまま読む (0.5 が RGBA8 と
// 同じ明るさで出る) ので、伸ばした線形の値を、sRGB の曲線をそのまま 1 の先へ延ばして符号化し直す。
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

// copyShader は、テクスチャを、同じ大きさの canvas へコピーする。uExpand なら、明るい所を伸ばす。
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

// HDRCopy は、拡大せずに、明るい所を伸ばして canvas へコピーする処理。擬似 HDR をやめた直後のコマは、伸ばさずにコピーする。
export type HDRCopy = {
  // draw は、width × height の texture を、同じ大きさの canvas (既定の framebuffer) へコピーする。
  // texture は行を下から持つ。flipped なら行を上から持つ (video をコピーしたテクスチャ)。expand なら、明るい所を伸ばす。
  draw(
    texture: WebGLTexture,
    flipped: boolean,
    width: number,
    height: number,
    expand: boolean,
  ): void;
  destroy(): void;
};

// createHDRCopy は、gl にコピーするシェーダーを作る。作れなければ投げる。変えた GL の状態は戻す。
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
// そうでなければ RGBA8 と standard に変える。既にその形なら何もしない。変えると描画先の内容は消えるので、
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
  // alpha のないコンテキストなどでは、形式を変えられず、GL のエラーになるだけで投げない。変わったかを確認してから、
  // トーンマッピングを変える。
  context.drawingBufferStorage(hdr ? gl.RGBA16F : gl.RGBA8, width, height);
  if (context.drawingBufferFormat !== (hdr ? gl.RGBA16F : gl.RGBA8)) {
    throw new Error('failed to change the drawing buffer format');
  }
  context.drawingBufferToneMapping({ mode: hdr ? 'extended' : 'standard' });
}
