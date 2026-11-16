// WebGL2 のフラグメントシェーダーで、画面を覆う三角形を描いて処理する部品 (#1655、#1654)。
// コマの補間 (interpolator.ts) と、映像の拡大 (upscaler.ts) が、yadif のコンテキストの中で使う。

// fullscreenVertexShader は、頂点の属性を使わずに、画面を覆う三角形を 1 つ描く。描く側は、空の VAO を当てて 3 頂点を描く。
export const fullscreenVertexShader = `#version 300 es
void main() {
  vec2 p = vec2((gl_VertexID << 1) & 2, gl_VertexID & 2);
  gl_Position = vec4(p * 2.0 - 1.0, 0.0, 1.0);
}
`;

// compileProgram は、fullscreenVertexShader と fragment をつないだプログラムを作る。作れなければ投げる。
export function compileProgram(gl: WebGL2RenderingContext, fragment: string): WebGLProgram {
  const program = gl.createProgram();
  for (const [type, source] of [
    [gl.VERTEX_SHADER, fullscreenVertexShader],
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

// SavedState は、処理が書き換える GL の状態。yadif のように、ほかが使っているコンテキストで動くときに戻す。
export type SavedState = {
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

// saveState は、テクスチャのユニット 0 から units 個までと、描画の状態を控える。
export function saveState(gl: WebGL2RenderingContext, units: number): SavedState {
  const activeTexture = gl.getParameter(gl.ACTIVE_TEXTURE) as number;
  const textures: (WebGLTexture | null)[] = [];
  const samplers: (WebGLSampler | null)[] = [];
  for (let unit = 0; unit < units; unit++) {
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

// restoreState は、saveState で控えた状態に戻す。
export function restoreState(gl: WebGL2RenderingContext, state: SavedState): void {
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
