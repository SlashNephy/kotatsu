// Anime4K の CNN (x2、M) で、フレームを縦横 2 倍に拡大する (#1653、設計書 7.2)。
// 畳み込みのステップは、Anime4K の mpv 向けのシェーダー (MIT) を scripts/convert-anime4k.mjs で WebGL2 に直したもの (anime4kShaders.ts)。
// 最後の Depth-to-Space のステップ (畳み込みの結果を 2×2 の画素に並べ、元のフレームを線形に拡大したものに足す) は、ここに書く。
// 畳み込みの途中の値は負にもなるので、RGBA16F のテクスチャに置く。描けない (EXT_color_buffer_float がない) 端末では作れない。
import { anime4kPasses } from './anime4kShaders';
import { compileProgram, restoreState, saveState } from './glPass';

// depthToSpaceShader は、畳み込みの最後のステップの 4 つの値を、2×2 の画素に 1 つずつ割り当て、元のフレームを線形に拡大した色に足す。
// 元のシェーダーと同じく、同じ値を RGB のどれにも足す (明るさの差分)。並べる順は、画像の左上、右上、左下、右下。
const depthToSpaceShader = `#version 300 es
precision highp float;
precision highp int;
uniform highp sampler2D uMain;
uniform highp sampler2D uResidual;
uniform int uDown;
uniform vec2 uOutputSize;
out vec4 fragColor;
void main() {
  ivec2 pixel = ivec2(gl_FragCoord.xy);
  ivec2 sub = pixel % 2;
  // テクスチャの行を下から持つなら、2×2 の中の上下も入れ替わる。
  int row = uDown == 1 ? sub.y : 1 - sub.y;
  float residual = texelFetch(uResidual, pixel / 2, 0)[row * 2 + sub.x];
  vec3 base = texture(uMain, gl_FragCoord.xy / uOutputSize).rgb;
  fragColor = vec4(clamp(base + residual, 0.0, 1.0), 1.0);
}
`;

// Anime4K は、yadif のコンテキストで、フレームを 2 倍に拡大する処理。
export type Anime4K = {
  // upscale は、width × height の texture を、縦横 2 倍に拡大したテクスチャを返す。
  // flipped なら texture は行を上から持ち、そうでなければ下から持つ。返すテクスチャも、同じ向きで行を持つ。
  // 返すテクスチャは、次に upscale を呼ぶか destroy するまで使える。
  upscale(texture: WebGLTexture, flipped: boolean, width: number, height: number): WebGLTexture;
  destroy(): void;
};

type Target = { texture: WebGLTexture; framebuffer: WebGLFramebuffer };

// createAnime4K は、gl に Anime4K のシェーダーを作る。作れないか、RGBA16F に描けなければ投げる。
export function createAnime4K(gl: WebGL2RenderingContext): Anime4K {
  if (gl.getExtension('EXT_color_buffer_float') === null) {
    throw new Error('Anime4K needs EXT_color_buffer_float');
  }
  const passes = anime4kPasses.map((pass) => {
    const program = compileProgram(gl, pass.fragment);

    return {
      ...pass,
      program,
      down: gl.getUniformLocation(program, 'uDown'),
      samplers: pass.inputs.map((input) => gl.getUniformLocation(program, input)),
    };
  });
  const last = compileProgram(gl, depthToSpaceShader);
  const lastLocation = {
    main: gl.getUniformLocation(last, 'uMain'),
    residual: gl.getUniformLocation(last, 'uResidual'),
    down: gl.getUniformLocation(last, 'uDown'),
    outputSize: gl.getUniformLocation(last, 'uOutputSize'),
  };
  const vertexArray = gl.createVertexArray();
  // 渡されるフレームは、最近傍で読む設定のこともある (yadif の出力)。拡大のもとは線形に読みたいので、線形の sampler を当てる。
  const linear = gl.createSampler();
  gl.samplerParameteri(linear, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
  gl.samplerParameteri(linear, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
  gl.samplerParameteri(linear, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
  gl.samplerParameteri(linear, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
  const units = Math.max(...passes.map((pass) => pass.inputs.length), 2);

  // sized は、今の資源を作った、元のフレームの大きさ。
  let sized: { width: number; height: number } | null = null;
  let middles = new Map<string, Target>();
  let output: Target | null = null;

  const createTarget = (format: number, width: number, height: number): Target => {
    const texture = gl.createTexture();
    gl.bindTexture(gl.TEXTURE_2D, texture);
    gl.texStorage2D(gl.TEXTURE_2D, 1, format, width, height);
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
      throw new Error('Anime4K cannot render to its textures');
    }

    return { texture, framebuffer };
  };

  const release = () => {
    for (const target of [...middles.values(), ...(output === null ? [] : [output])]) {
      gl.deleteFramebuffer(target.framebuffer);
      gl.deleteTexture(target.texture);
    }
    middles = new Map();
    output = null;
    sized = null;
  };

  const ensure = (width: number, height: number) => {
    if (sized?.width === width && sized.height === height && output !== null) {
      return output;
    }
    release();
    for (const pass of passes) {
      middles.set(pass.output, createTarget(gl.RGBA16F, width, height));
    }
    output = createTarget(gl.RGBA8, width * 2, height * 2);
    sized = { width, height };

    return output;
  };

  return {
    upscale(texture, flipped, width, height) {
      const state = saveState(gl, units);
      try {
        gl.disable(gl.BLEND);
        gl.disable(gl.SCISSOR_TEST);
        gl.disable(gl.DEPTH_TEST);
        gl.bindVertexArray(vertexArray);
        // テクスチャを作るときに当てるユニットを、控えたユニットの中にする。
        gl.activeTexture(gl.TEXTURE0);
        const target = ensure(width, height);
        const down = flipped ? 1 : -1;
        const textureOf = (name: string) =>
          name === 'MAIN' ? texture : (middles.get(name)?.texture ?? null);
        gl.viewport(0, 0, width, height);
        for (const pass of passes) {
          gl.useProgram(pass.program);
          pass.inputs.forEach((input, unit) => {
            gl.activeTexture(gl.TEXTURE0 + unit);
            gl.bindTexture(gl.TEXTURE_2D, textureOf(input));
            // texelFetch で読むので、テクスチャの絞り方には頼らないが、ほかの sampler を当てたままにしない。
            gl.bindSampler(unit, null);
            gl.uniform1i(pass.samplers[unit] ?? null, unit);
          });
          gl.uniform1i(pass.down, down);
          gl.bindFramebuffer(gl.FRAMEBUFFER, middles.get(pass.output)?.framebuffer ?? null);
          gl.drawArrays(gl.TRIANGLES, 0, 3);
        }
        gl.useProgram(last);
        gl.activeTexture(gl.TEXTURE0);
        gl.bindTexture(gl.TEXTURE_2D, texture);
        gl.bindSampler(0, linear);
        gl.uniform1i(lastLocation.main, 0);
        gl.activeTexture(gl.TEXTURE1);
        gl.bindTexture(gl.TEXTURE_2D, middles.get('conv2d_last_tf')?.texture ?? null);
        gl.bindSampler(1, null);
        gl.uniform1i(lastLocation.residual, 1);
        gl.uniform1i(lastLocation.down, down);
        gl.uniform2f(lastLocation.outputSize, width * 2, height * 2);
        gl.bindFramebuffer(gl.FRAMEBUFFER, target.framebuffer);
        gl.viewport(0, 0, width * 2, height * 2);
        gl.drawArrays(gl.TRIANGLES, 0, 3);

        return target.texture;
      } finally {
        restoreState(gl, state);
      }
    },
    destroy() {
      release();
      gl.deleteSampler(linear);
      gl.deleteVertexArray(vertexArray);
      for (const pass of passes) {
        gl.deleteProgram(pass.program);
      }
      gl.deleteProgram(last);
    },
  };
}
