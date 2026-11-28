// 映像の拡大 (#1654、設計書 7.2)。解除したフレームを、描く矩形の端末の画素の大きさへ、縁を見て拡大し、鮮鋭化する。
// AMD FidelityFX Super Resolution 1 (FSR 1) の EASU (縁を見た拡大) と RCAS (鮮鋭化) を、WebGL2 の
// フラグメントシェーダーに書き直したもの。textureGather がないので、EASU の 12 画素は texelFetch で読む。
// canvas もコンテキストも持たず、呼ぶ側 (yadif) が渡す gl とテクスチャで動き、変えた GL の状態は戻す。
//
// EASU と RCAS のシェーダーは、ffx_fsr1.h (https://github.com/GPUOpen-Effects/FidelityFX-FSR) から書き直した。
// 元のファイルの著作権表示とライセンス (MIT) は、次のとおり。
//
// Copyright (c) 2021 Advanced Micro Devices, Inc. All rights reserved.
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files(the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and / or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions :
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

import { compileProgram, restoreState, saveState } from './glPass';
import { expandHighlightsShader } from './hdrDrawing';

// EASU。出力の画素の中心を元の画素の座標に対応させ、周りの 12 画素から縁の向きと強さを推し量り、
// 縁に沿って伸ばしたウィンドウの Lanczos に近い重みで混ぜる。近い 4 画素の最小と最大に収め、縁の周りの輪を出さない。
// 座標は、行を下から数える (framebuffer と同じ向き)。uFlip のテクスチャは行を上から持つので、読むときに返す。
const easuShader = `#version 300 es
precision highp float;
precision highp int;
uniform sampler2D uSource;
uniform bool uFlip;
// uScale は、元の大きさを出力の大きさで割ったもの。uOffset は、0.5 * uScale - 0.5。
uniform vec2 uScale;
uniform vec2 uOffset;
out vec4 color;

ivec2 sourceSize;

vec3 load(ivec2 p) {
  p = clamp(p, ivec2(0), sourceSize - 1);
  if (uFlip) {
    p.y = sourceSize.y - 1 - p.y;
  }
  return texelFetch(uSource, p, 0).rgb;
}

// luma は、輝度の 2 倍の近似 (FSR と同じ)。
float luma(vec3 c) {
  return c.b * 0.5 + (c.r * 0.5 + c.g);
}

// direction は、十字の 5 画素 (a は上、b は左、c は中心、d は右、e は下) から、縁の向きと強さを足し込む。
void direction(inout vec2 dir, inout float len, float w, float a, float b, float c, float d, float e) {
  float dc = d - c;
  float cb = c - b;
  float lenX = max(abs(dc), abs(cb));
  float dirX = d - b;
  dir.x += dirX * w;
  lenX = clamp(abs(dirX) / max(lenX, 1.0 / 65536.0), 0.0, 1.0);
  lenX *= lenX;
  len += lenX * w;
  float ec = e - c;
  float ca = c - a;
  float lenY = max(abs(ec), abs(ca));
  float dirY = e - a;
  dir.y += dirY * w;
  lenY = clamp(abs(dirY) / max(lenY, 1.0 / 65536.0), 0.0, 1.0);
  lenY *= lenY;
  len += lenY * w;
}

void tap(inout vec3 aC, inout float aW, vec2 off, vec2 dir, vec2 len, float lob, float clp, vec3 c) {
  vec2 v = vec2(off.x * dir.x + off.y * dir.y, off.x * -dir.y + off.y * dir.x);
  v *= len;
  float d2 = min(dot(v, v), clp);
  float wB = 2.0 / 5.0 * d2 - 1.0;
  float wA = lob * d2 - 1.0;
  wB *= wB;
  wA *= wA;
  wB = 25.0 / 16.0 * wB - (25.0 / 16.0 - 1.0);
  float w = wB * wA;
  aC += c * w;
  aW += w;
}

void main() {
  sourceSize = textureSize(uSource, 0);
  vec2 pp = (gl_FragCoord.xy - 0.5) * uScale + uOffset;
  vec2 fp = floor(pp);
  pp -= fp;
  ivec2 f0 = ivec2(fp);
  //    b c
  //  e f g h
  //  i j k l
  //    n o
  vec3 b = load(f0 + ivec2(0, -1));
  vec3 c = load(f0 + ivec2(1, -1));
  vec3 e = load(f0 + ivec2(-1, 0));
  vec3 f = load(f0);
  vec3 g = load(f0 + ivec2(1, 0));
  vec3 h = load(f0 + ivec2(2, 0));
  vec3 i = load(f0 + ivec2(-1, 1));
  vec3 j = load(f0 + ivec2(0, 1));
  vec3 k = load(f0 + ivec2(1, 1));
  vec3 l = load(f0 + ivec2(2, 1));
  vec3 n = load(f0 + ivec2(0, 2));
  vec3 o = load(f0 + ivec2(1, 2));
  float bL = luma(b);
  float cL = luma(c);
  float eL = luma(e);
  float fL = luma(f);
  float gL = luma(g);
  float hL = luma(h);
  float iL = luma(i);
  float jL = luma(j);
  float kL = luma(k);
  float lL = luma(l);
  float nL = luma(n);
  float oL = luma(o);
  // 中心の 4 画素 (f g j k) それぞれの十字から推し量り、双線形の重みで混ぜる。
  vec2 dir = vec2(0.0);
  float len = 0.0;
  direction(dir, len, (1.0 - pp.x) * (1.0 - pp.y), bL, eL, fL, gL, jL);
  direction(dir, len, pp.x * (1.0 - pp.y), cL, fL, gL, hL, kL);
  direction(dir, len, (1.0 - pp.x) * pp.y, fL, iL, jL, kL, nL);
  direction(dir, len, pp.x * pp.y, gL, jL, kL, lL, oL);
  vec2 dir2 = dir * dir;
  float dirR = dir2.x + dir2.y;
  bool zero = dirR < 1.0 / 32768.0;
  dirR = zero ? 1.0 : inversesqrt(dirR);
  dir.x = zero ? 1.0 : dir.x;
  dir *= dirR;
  len = len * 0.5;
  len *= len;
  float stretch = dot(dir, dir) / max(abs(dir.x), abs(dir.y));
  vec2 len2 = vec2(1.0 + (stretch - 1.0) * len, 1.0 - 0.5 * len);
  float lob = 0.5 + ((1.0 / 4.0 - 0.04) - 0.5) * len;
  float clp = 1.0 / lob;
  vec3 min4 = min(min(f, g), min(j, k));
  vec3 max4 = max(max(f, g), max(j, k));
  vec3 aC = vec3(0.0);
  float aW = 0.0;
  tap(aC, aW, vec2(0.0, -1.0) - pp, dir, len2, lob, clp, b);
  tap(aC, aW, vec2(1.0, -1.0) - pp, dir, len2, lob, clp, c);
  tap(aC, aW, vec2(-1.0, 1.0) - pp, dir, len2, lob, clp, i);
  tap(aC, aW, vec2(0.0, 1.0) - pp, dir, len2, lob, clp, j);
  tap(aC, aW, vec2(0.0, 0.0) - pp, dir, len2, lob, clp, f);
  tap(aC, aW, vec2(-1.0, 0.0) - pp, dir, len2, lob, clp, e);
  tap(aC, aW, vec2(1.0, 1.0) - pp, dir, len2, lob, clp, k);
  tap(aC, aW, vec2(2.0, 1.0) - pp, dir, len2, lob, clp, l);
  tap(aC, aW, vec2(2.0, 0.0) - pp, dir, len2, lob, clp, h);
  tap(aC, aW, vec2(1.0, 0.0) - pp, dir, len2, lob, clp, g);
  tap(aC, aW, vec2(1.0, 2.0) - pp, dir, len2, lob, clp, o);
  tap(aC, aW, vec2(0.0, 2.0) - pp, dir, len2, lob, clp, n);
  color = vec4(min(max4, max(min4, aC / aW)), 1.0);
}
`;

// RCAS。十字の 5 画素から、0〜1 を外れない範囲で最も強い鮮鋭化を求め、uSharpness を掛けて掛ける。
// 粒状のノイズらしい所 (中心だけが周りと違う所) は弱める (FSR の FSR_RCAS_DENOISE)。放送の映像は圧縮のノイズが多い。
// uExpand なら、鮮鋭化した後に、擬似 HDR の出力 (#1656) のために明るい所を伸ばす。
const rcasShader = `#version 300 es
precision highp float;
precision highp int;
uniform sampler2D uSource;
uniform float uSharpness;
uniform bool uExpand;
out vec4 color;
${expandHighlightsShader}

const float limit = 0.25 - 1.0 / 16.0;

ivec2 size;

vec3 load(ivec2 p) {
  return texelFetch(uSource, clamp(p, ivec2(0), size - 1), 0).rgb;
}

float luma(vec3 c) {
  return c.b * 0.5 + (c.r * 0.5 + c.g);
}

void main() {
  size = textureSize(uSource, 0);
  ivec2 sp = ivec2(gl_FragCoord.xy);
  vec3 b = load(sp + ivec2(0, -1));
  vec3 d = load(sp + ivec2(-1, 0));
  vec3 e = load(sp);
  vec3 f = load(sp + ivec2(1, 0));
  vec3 h = load(sp + ivec2(0, 1));
  float bL = luma(b);
  float dL = luma(d);
  float eL = luma(e);
  float fL = luma(f);
  float hL = luma(h);
  float nz = 0.25 * (bL + dL + fL + hL) - eL;
  float range = max(max(max(bL, dL), max(eL, fL)), hL) - min(min(min(bL, dL), min(eL, fL)), hL);
  nz = clamp(abs(nz) / max(range, 1.0 / 65536.0), 0.0, 1.0);
  nz = -0.5 * nz + 1.0;
  vec3 mn4 = min(min(b, d), min(f, h));
  vec3 mx4 = max(max(b, d), max(f, h));
  vec3 hitMin = min(mn4, e) / max(4.0 * mx4, vec3(1.0 / 65536.0));
  vec3 hitMax = (1.0 - max(mx4, e)) / min(4.0 * mn4 - 4.0, vec3(-1.0 / 65536.0));
  vec3 lobeRGB = max(-hitMin, hitMax);
  float lobe = max(-limit, min(max(max(lobeRGB.r, lobeRGB.g), lobeRGB.b), 0.0)) * uSharpness;
  lobe *= nz;
  vec3 sharpened = (lobe * (b + d + h + f) + e) / (4.0 * lobe + 1.0);
  color = vec4(uExpand ? expandHighlights(sharpened) : sharpened, 1.0);
}
`;

// Upscaler は、yadif のコンテキストで、テクスチャを canvas へ拡大して描く処理。
export type Upscaler = {
  // draw は、width × height の texture を、canvas (既定の framebuffer) の outputWidth × outputHeight へ拡大して描く。
  // texture は行を下から持つ。flipped なら行を上から持つ (video をコピーしたテクスチャ)。
  // sharpnessStops は、RCAS の鮮鋭化を弱めるステップ数。0 が最も強く、1 ステップごとに半分になる。
  // expand なら、擬似 HDR の出力 (#1656) のために、最後に明るい所を伸ばす。
  draw(
    texture: WebGLTexture,
    flipped: boolean,
    width: number,
    height: number,
    outputWidth: number,
    outputHeight: number,
    sharpnessStops: number,
    expand: boolean,
  ): void;
  destroy(): void;
};

// createUpscaler は、gl に拡大のシェーダーを作る。作れなければ投げる。
// EASU の結果は、出力の大きさの RGBA8 のテクスチャに置き、RCAS がそれを読んで canvas に描く。中間の絵に浮動小数点の描画先は使わない。
export function createUpscaler(gl: WebGL2RenderingContext): Upscaler {
  const easu = compileProgram(gl, easuShader);
  const rcas = compileProgram(gl, rcasShader);
  const location = {
    source: gl.getUniformLocation(easu, 'uSource'),
    flip: gl.getUniformLocation(easu, 'uFlip'),
    scale: gl.getUniformLocation(easu, 'uScale'),
    offset: gl.getUniformLocation(easu, 'uOffset'),
    rcasSource: gl.getUniformLocation(rcas, 'uSource'),
    sharpness: gl.getUniformLocation(rcas, 'uSharpness'),
    expand: gl.getUniformLocation(rcas, 'uExpand'),
  };
  const vertexArray = gl.createVertexArray();
  // middle は、EASU で拡大した絵を置く、出力の大きさのテクスチャ。
  let middle: {
    texture: WebGLTexture;
    framebuffer: WebGLFramebuffer;
    width: number;
    height: number;
  } | null = null;

  const release = () => {
    if (middle !== null) {
      gl.deleteFramebuffer(middle.framebuffer);
      gl.deleteTexture(middle.texture);
      middle = null;
    }
  };

  const ensureMiddle = (width: number, height: number) => {
    if (middle?.width === width && middle.height === height) {
      return middle;
    }
    release();
    const texture = gl.createTexture();
    gl.bindTexture(gl.TEXTURE_2D, texture);
    gl.texStorage2D(gl.TEXTURE_2D, 1, gl.RGBA8, width, height);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
    const framebuffer = gl.createFramebuffer();
    gl.bindFramebuffer(gl.FRAMEBUFFER, framebuffer);
    gl.framebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, texture, 0);
    if (gl.checkFramebufferStatus(gl.FRAMEBUFFER) !== gl.FRAMEBUFFER_COMPLETE) {
      gl.deleteFramebuffer(framebuffer);
      gl.deleteTexture(texture);
      throw new Error('the upscaler cannot render to an RGBA8 texture');
    }
    middle = { texture, framebuffer, width, height };

    return middle;
  };

  return {
    draw(texture, flipped, width, height, outputWidth, outputHeight, sharpnessStops, expand) {
      const state = saveState(gl, 1);
      try {
        gl.disable(gl.BLEND);
        gl.disable(gl.SCISSOR_TEST);
        gl.disable(gl.DEPTH_TEST);
        gl.bindVertexArray(vertexArray);
        gl.activeTexture(gl.TEXTURE0);
        // texelFetch で読むので、テクスチャの絞り方には頼らないが、ほかの sampler を当てたままにしない。
        gl.bindSampler(0, null);
        const target = ensureMiddle(outputWidth, outputHeight);
        gl.useProgram(easu);
        gl.bindTexture(gl.TEXTURE_2D, texture);
        gl.uniform1i(location.source, 0);
        gl.uniform1i(location.flip, flipped ? 1 : 0);
        gl.uniform2f(location.scale, width / outputWidth, height / outputHeight);
        gl.uniform2f(
          location.offset,
          (0.5 * width) / outputWidth - 0.5,
          (0.5 * height) / outputHeight - 0.5,
        );
        gl.bindFramebuffer(gl.FRAMEBUFFER, target.framebuffer);
        gl.viewport(0, 0, outputWidth, outputHeight);
        gl.drawArrays(gl.TRIANGLES, 0, 3);
        gl.useProgram(rcas);
        gl.bindTexture(gl.TEXTURE_2D, target.texture);
        gl.uniform1i(location.rcasSource, 0);
        gl.uniform1f(location.sharpness, 2 ** -sharpnessStops);
        gl.uniform1i(location.expand, expand ? 1 : 0);
        gl.bindFramebuffer(gl.FRAMEBUFFER, null);
        gl.viewport(0, 0, outputWidth, outputHeight);
        gl.drawArrays(gl.TRIANGLES, 0, 3);
      } finally {
        restoreState(gl, state);
      }
    },
    destroy() {
      release();
      gl.deleteVertexArray(vertexArray);
      gl.deleteProgram(easu);
      gl.deleteProgram(rcas);
    },
  };
}
