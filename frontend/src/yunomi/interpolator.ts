// コマの補間 (#1655、設計書 7.5)。前後の 2 コマから、ブロックマッチングで動きを推し量り、その間の時刻のコマを作る。
// WebGL2 のフラグメントシェーダーだけで書き、canvas もコンテキストも持たない。呼ぶ側が渡す gl とテクスチャで動くので、
// kyusu の経路 (video を写したテクスチャ) でも、yadif の中 (yadif が解除したテクスチャ) でも使える。
// テクスチャは、どちらも行を下から持つ (framebuffer と同じ向き) ものとする。

// blockSize は、動きを推し量るブロックの一辺 (半分の解像度での画素)。原寸では 16 画素になる。
export const blockSize = 8;
// searchRange は、ブロックを探す範囲 (半分の解像度での画素、上下左右)。原寸では ±16 画素になる。
export const searchRange = 8;
// mismatchThreshold は、見つけたブロックの 1 画素あたりの差 (0〜1) がこれを超えたら、動きを当てにしない値。
// 場面の切り替わりや、隠れていたものが現れた所では、どこにも似た所がないので、補間せずに近い方のコマを出す。
export const mismatchThreshold = 0.06;

const vertexShader = `#version 300 es
void main() {
  vec2 p = vec2((gl_VertexID << 1) & 2, gl_VertexID & 2);
  gl_Position = vec4(p * 2.0 - 1.0, 0.0, 1.0);
}
`;

// 輝度を半分の解像度にする。元のテクスチャは 0〜1 の座標で読む。ブラウザが video を写す大きさ
// (表示の大きさか、符号化した大きさか) によらず、同じ絵を縮められる。線形に読むので、2×2 の平均になる。
const downsampleShader = `#version 300 es
precision highp float;
uniform sampler2D uSource;
uniform vec2 uOutputSize;
out vec4 color;
void main() {
  vec3 rgb = texture(uSource, gl_FragCoord.xy / uOutputSize).rgb;
  color = vec4(dot(rgb, vec3(0.299, 0.587, 0.114)), 0.0, 0.0, 1.0);
}
`;

// ブロックごとに、後のコマのブロックが前のコマのどこから来たかを総当たりで探す。
// RGBA8 に、ずれ (dx, dy) に searchRange を足したものと、1 画素あたりの差を入れる。浮動小数点の描画先に頼らない。
const estimateShader = `#version 300 es
precision highp float;
precision highp int;
uniform sampler2D uPrevious;
uniform sampler2D uNext;
out vec4 color;
const int B = ${blockSize};
const int R = ${searchRange};
void main() {
  ivec2 base = ivec2(gl_FragCoord.xy) * B;
  ivec2 last = textureSize(uPrevious, 0) - 1;
  float block[B * B];
  for (int y = 0; y < B; y++) {
    for (int x = 0; x < B; x++) {
      block[y * B + x] = texelFetch(uNext, min(base + ivec2(x, y), last), 0).r;
    }
  }
  float best = 1e9;
  ivec2 found = ivec2(0);
  for (int dy = -R; dy <= R; dy++) {
    for (int dx = -R; dx <= R; dx++) {
      float sad = 0.0;
      for (int y = 0; y < B; y++) {
        for (int x = 0; x < B; x++) {
          ivec2 q = clamp(base + ivec2(x, y) - ivec2(dx, dy), ivec2(0), last);
          sad += abs(texelFetch(uPrevious, q, 0).r - block[y * B + x]);
        }
      }
      // 差が同じくらいなら、動きの小さい方を選ぶ。平らな所で、でたらめな動きを拾わないため。
      sad += 0.002 * float(abs(dx) + abs(dy));
      if (sad < best) {
        best = sad;
        found = ivec2(dx, dy);
      }
    }
  }
  color = vec4(
    float(found.x + R) / 255.0,
    float(found.y + R) / 255.0,
    min(best / float(B * B), 1.0),
    1.0
  );
}
`;

// 時刻 uT (0 なら前のコマ、1 なら後のコマ) のコマを作る。動きは、ブロックの間を線形に補う。
// 動きを当てにできない所は、近い方のコマをそのまま出す。境目がくっきりしないよう、閾値の前後でなだらかに混ぜる。
const composeShader = `#version 300 es
precision highp float;
uniform sampler2D uPrevious;
uniform sampler2D uNext;
uniform sampler2D uVectors;
uniform float uT;
uniform vec2 uOutputSize;
uniform vec2 uLumaSize;
out vec4 color;
const float R = ${searchRange}.0;
const float threshold = ${mismatchThreshold};
void main() {
  vec2 uv = gl_FragCoord.xy / uOutputSize;
  vec4 m = texture(uVectors, uv);
  // ずれは、動きを推し量った輝度の画素なので、その大きさで割って、テクスチャの座標にする。
  vec2 motion = (m.rg * 255.0 - R) / uLumaSize;
  vec4 interpolated = mix(
    texture(uPrevious, uv - uT * motion),
    texture(uNext, uv + (1.0 - uT) * motion),
    uT
  );
  vec4 nearest = uT < 0.5 ? texture(uPrevious, uv) : texture(uNext, uv);
  float unreliable = smoothstep(threshold * 0.7, threshold * 1.3, m.b);
  color = vec4(mix(interpolated.rgb, nearest.rgb, unreliable), 1.0);
}
`;

// Interpolator は、補間に使う GL の資源をまとめたもの。
export type Interpolator = {
  // estimate は、previous から next への動きを推し量り、次の compose で使う。2 コマが変わったときに 1 回だけ呼ぶ。
  // width と height は、コマの大きさ。動きは、その半分の解像度で推し量る。
  estimate(previous: WebGLTexture, next: WebGLTexture, width: number, height: number): void;
  // compose は、最後に estimate した 2 コマの間の、時刻 t (0〜1) のコマを、framebuffer (null なら canvas) に、
  // width × height で描く。コマと大きさが違えば、引き伸ばして描く。
  compose(t: number, framebuffer: WebGLFramebuffer | null, width: number, height: number): void;
  destroy(): void;
};

type Programs = {
  downsample: WebGLProgram;
  estimate: WebGLProgram;
  compose: WebGLProgram;
};

function compile(gl: WebGL2RenderingContext, fragment: string): WebGLProgram {
  const program = gl.createProgram();
  for (const [type, source] of [
    [gl.VERTEX_SHADER, vertexShader],
    [gl.FRAGMENT_SHADER, fragment],
  ] as const) {
    const shader = gl.createShader(type);
    if (shader === null) {
      throw new Error('failed to create a shader');
    }
    gl.shaderSource(shader, source);
    gl.compileShader(shader);
    if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
      throw new Error(`failed to compile a shader: ${gl.getShaderInfoLog(shader) ?? ''}`);
    }
    gl.attachShader(program, shader);
    gl.deleteShader(shader);
  }
  gl.linkProgram(program);
  if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
    throw new Error(`failed to link a program: ${gl.getProgramInfoLog(program) ?? ''}`);
  }

  return program;
}

// SavedState は、補間が書き換える GL の状態。yadif のように、ほかが使っているコンテキストで動くときに戻す。
type SavedState = {
  framebuffer: WebGLFramebuffer | null;
  program: WebGLProgram | null;
  viewport: Int32Array;
  activeTexture: number;
  textures: (WebGLTexture | null)[];
  samplers: (WebGLSampler | null)[];
  vertexArray: WebGLVertexArrayObject | null;
  blend: boolean;
  scissor: boolean;
  depth: boolean;
};

// textureUnits は、補間が使うテクスチャのユニットの数。
const textureUnits = 3;

function save(gl: WebGL2RenderingContext): SavedState {
  const activeTexture = gl.getParameter(gl.ACTIVE_TEXTURE) as number;
  const textures: (WebGLTexture | null)[] = [];
  const samplers: (WebGLSampler | null)[] = [];
  for (let unit = 0; unit < textureUnits; unit++) {
    gl.activeTexture(gl.TEXTURE0 + unit);
    textures.push(gl.getParameter(gl.TEXTURE_BINDING_2D) as WebGLTexture | null);
    samplers.push(gl.getParameter(gl.SAMPLER_BINDING) as WebGLSampler | null);
  }
  gl.activeTexture(activeTexture);

  return {
    framebuffer: gl.getParameter(gl.FRAMEBUFFER_BINDING) as WebGLFramebuffer | null,
    program: gl.getParameter(gl.CURRENT_PROGRAM) as WebGLProgram | null,
    viewport: gl.getParameter(gl.VIEWPORT) as Int32Array,
    activeTexture,
    textures,
    samplers,
    vertexArray: gl.getParameter(gl.VERTEX_ARRAY_BINDING) as WebGLVertexArrayObject | null,
    blend: gl.isEnabled(gl.BLEND),
    scissor: gl.isEnabled(gl.SCISSOR_TEST),
    depth: gl.isEnabled(gl.DEPTH_TEST),
  };
}

function restore(gl: WebGL2RenderingContext, state: SavedState): void {
  for (const [capability, enabled] of [
    [gl.BLEND, state.blend],
    [gl.SCISSOR_TEST, state.scissor],
    [gl.DEPTH_TEST, state.depth],
  ] as const) {
    if (enabled) {
      gl.enable(capability);
    } else {
      gl.disable(capability);
    }
  }
  state.textures.forEach((texture, unit) => {
    gl.activeTexture(gl.TEXTURE0 + unit);
    gl.bindTexture(gl.TEXTURE_2D, texture);
    gl.bindSampler(unit, state.samplers[unit] ?? null);
  });
  gl.activeTexture(state.activeTexture);
  gl.bindVertexArray(state.vertexArray);
  gl.useProgram(state.program);
  gl.bindFramebuffer(gl.FRAMEBUFFER, state.framebuffer);
  const [x = 0, y = 0, width = 0, height = 0] = state.viewport;
  gl.viewport(x, y, width, height);
}

// Target は、描画先のテクスチャと framebuffer の組。
type Target = {
  texture: WebGLTexture;
  framebuffer: WebGLFramebuffer;
  width: number;
  height: number;
};

function createTarget(
  gl: WebGL2RenderingContext,
  width: number,
  height: number,
  internalFormat: number,
  filter: number,
): Target {
  const texture = gl.createTexture();
  gl.bindTexture(gl.TEXTURE_2D, texture);
  gl.texStorage2D(gl.TEXTURE_2D, 1, internalFormat, width, height);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, filter);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, filter);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
  const framebuffer = gl.createFramebuffer();
  gl.bindFramebuffer(gl.FRAMEBUFFER, framebuffer);
  gl.framebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, texture, 0);

  return { texture, framebuffer, width, height };
}

function deleteTarget(gl: WebGL2RenderingContext, target: Target): void {
  gl.deleteFramebuffer(target.framebuffer);
  gl.deleteTexture(target.texture);
}

// createInterpolator は、gl に補間のシェーダーを作る。作れなければ投げる。
export function createInterpolator(gl: WebGL2RenderingContext): Interpolator {
  const programs: Programs = {
    downsample: compile(gl, downsampleShader),
    estimate: compile(gl, estimateShader),
    compose: compile(gl, composeShader),
  };
  // 描くときは頂点の属性を使わないので、空の VAO を当てて、ほかの VAO の状態に触れない。
  const vertexArray = gl.createVertexArray();
  // 渡されるテクスチャは、最近傍で読む設定のこともある (yadif の出力)。縮めるときも、ずらして読むときも
  // 線形に読みたいので、テクスチャの設定を変えずに、線形で読む sampler を当てる。
  const linear = gl.createSampler();
  gl.samplerParameteri(linear, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
  gl.samplerParameteri(linear, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
  gl.samplerParameteri(linear, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
  gl.samplerParameteri(linear, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
  // sized は、今の資源を作った、元のコマの大きさ。
  let sized: { width: number; height: number } | null = null;
  let previousLuma: Target | null = null;
  let nextLuma: Target | null = null;
  let vectors: Target | null = null;
  // pair は、最後に estimate した 2 コマ。
  let pair: { previous: WebGLTexture; next: WebGLTexture } | null = null;

  const release = () => {
    for (const target of [previousLuma, nextLuma, vectors]) {
      if (target !== null) {
        deleteTarget(gl, target);
      }
    }
    previousLuma = null;
    nextLuma = null;
    vectors = null;
    sized = null;
  };

  const resize = (width: number, height: number) => {
    if (sized?.width === width && sized.height === height) {
      return;
    }
    release();
    const halfWidth = Math.ceil(width / 2);
    const halfHeight = Math.ceil(height / 2);
    previousLuma = createTarget(gl, halfWidth, halfHeight, gl.R8, gl.NEAREST);
    nextLuma = createTarget(gl, halfWidth, halfHeight, gl.R8, gl.NEAREST);
    vectors = createTarget(
      gl,
      Math.ceil(halfWidth / blockSize),
      Math.ceil(halfHeight / blockSize),
      gl.RGBA8,
      gl.LINEAR,
    );
    sized = { width, height };
  };

  const bind = (program: WebGLProgram, textures: [string, WebGLTexture][]) => {
    gl.useProgram(program);
    textures.forEach(([name, texture], unit) => {
      gl.activeTexture(gl.TEXTURE0 + unit);
      gl.bindTexture(gl.TEXTURE_2D, texture);
      gl.bindSampler(unit, linear);
      gl.uniform1i(gl.getUniformLocation(program, name), unit);
    });
  };

  const drawTo = (target: Target | null, width: number, height: number) => {
    gl.bindFramebuffer(gl.FRAMEBUFFER, target?.framebuffer ?? null);
    gl.viewport(0, 0, width, height);
    gl.drawArrays(gl.TRIANGLES, 0, 3);
  };

  const prepare = () => {
    gl.disable(gl.BLEND);
    gl.disable(gl.SCISSOR_TEST);
    gl.disable(gl.DEPTH_TEST);
    gl.bindVertexArray(vertexArray);
  };

  return {
    estimate(previous, next, width, height) {
      const state = save(gl);
      try {
        prepare();
        resize(width, height);
        if (previousLuma === null || nextLuma === null || vectors === null) {
          return;
        }
        for (const [source, target] of [
          [previous, previousLuma],
          [next, nextLuma],
        ] as const) {
          bind(programs.downsample, [['uSource', source]]);
          gl.uniform2f(
            gl.getUniformLocation(programs.downsample, 'uOutputSize'),
            target.width,
            target.height,
          );
          drawTo(target, target.width, target.height);
        }
        bind(programs.estimate, [
          ['uPrevious', previousLuma.texture],
          ['uNext', nextLuma.texture],
        ]);
        drawTo(vectors, vectors.width, vectors.height);
        pair = { previous, next };
      } finally {
        restore(gl, state);
      }
    },
    compose(t, framebuffer, width, height) {
      if (pair === null || vectors === null || previousLuma === null) {
        return;
      }
      const luma = previousLuma;
      const state = save(gl);
      try {
        prepare();
        bind(programs.compose, [
          ['uPrevious', pair.previous],
          ['uNext', pair.next],
          ['uVectors', vectors.texture],
        ]);
        gl.uniform1f(gl.getUniformLocation(programs.compose, 'uT'), t);
        gl.uniform2f(gl.getUniformLocation(programs.compose, 'uOutputSize'), width, height);
        gl.uniform2f(gl.getUniformLocation(programs.compose, 'uLumaSize'), luma.width, luma.height);
        gl.bindFramebuffer(gl.FRAMEBUFFER, framebuffer);
        gl.viewport(0, 0, width, height);
        gl.drawArrays(gl.TRIANGLES, 0, 3);
      } finally {
        restore(gl, state);
      }
    },
    destroy() {
      release();
      pair = null;
      gl.deleteVertexArray(vertexArray);
      gl.deleteSampler(linear);
      gl.deleteProgram(programs.downsample);
      gl.deleteProgram(programs.estimate);
      gl.deleteProgram(programs.compose);
    },
  };
}
