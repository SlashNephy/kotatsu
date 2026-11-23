// ブラウザ内でデコードする経路 (#1516) の、worker の OffscreenCanvas に WebGL2 でコマを描く部分。
// デコードしたコマの Y、U、V の 3 面を R8 のテクスチャ 3 枚に上げ (JS 側で複製せずに、WASM のメモリから直に)、
// インタレースのコマはフィールドごとに描く。描くフィールドの行はそのまま使い、抜けた行は、もう片方のフィールドの値 o が
// 上下の行 a、b の間からはみ出した量で、織り (o) と bob ((a+b)/2) をなだらかに混ぜる。はみ出しても、前のコマと比べて
// 止まっている画素は o を使う (縁取りのある字や細い横線が、フィールドごとに上下に揺れないように)。
// 色差 (4:2:0) も同じ規則で埋め、止まっている度合いは輝度で決めたものを使う。

// RendererThresholds は、インタレ解除の閾値 (0〜1 の輝度)。織り < bob、静止 < 動き にする。
export type RendererThresholds = {
  weave: number;
  bob: number;
  still: number;
  motion: number;
};

// defaultRendererThresholds は、合成の映像で見て決めた既定 (#1516 の測定)。
export const defaultRendererThresholds: RendererThresholds = {
  weave: 4 / 255,
  bob: 24 / 255,
  still: 2 / 255,
  motion: 12 / 255,
};

// ColorMatrix は、YUV から RGB への変換の規格。どちらも限定レンジ。
export type ColorMatrix = 'bt709' | 'bt601';

// PictureSlot は、1 コマ分のテクスチャ。
export type PictureSlot = {
  y: WebGLTexture;
  u: WebGLTexture;
  v: WebGLTexture;
  width: number;
  height: number;
};

// PlanePointer は、WASM のメモリの中の 1 面の位置と、1 行のバイト数。
export type PlanePointer = { pointer: number; stride: number };

// DrawRequest は、描くコマとフィールド。
export type DrawRequest = {
  slot: PictureSlot;
  // previous は、前に映したコマ。大きさが違えば比べない。
  previous: PictureSlot | null;
  interlaced: boolean;
  // parity は、描くフィールドが画像の上 (0) か下 (1) か。
  parity: 0 | 1;
  matrix: ColorMatrix;
};

const vertexShader = `#version 300 es
void main() {
  // 画面全体を覆う 1 つの三角形。
  vec2 p = vec2((gl_VertexID << 1) & 2, gl_VertexID & 2);
  gl_Position = vec4(p * 2.0 - 1.0, 0.0, 1.0);
}`;

const fragmentShader = `#version 300 es
precision highp float;
precision highp int;
uniform highp sampler2D uY;
uniform highp sampler2D uU;
uniform highp sampler2D uV;
uniform highp sampler2D uPrevY;
uniform int uField;
uniform int uInterlaced;
uniform int uHasPrev;
uniform vec2 uSource;
uniform vec2 uOutput;
uniform vec4 uThresholds;
uniform mat3 uMatrix;
out vec4 fragColor;

float y(ivec2 p) { return texelFetch(uY, clamp(p, ivec2(0), ivec2(uSource) - 1), 0).r; }
float py(ivec2 p) { return texelFetch(uPrevY, clamp(p, ivec2(0), ivec2(uSource) - 1), 0).r; }
vec2 c(ivec2 p) {
  ivec2 q = clamp(p, ivec2(0), ivec2(uSource) / 2 - 1);
  return vec2(texelFetch(uU, q, 0).r, texelFetch(uV, q, 0).r);
}

// keepAt は、このフィールドにない輝度の行 m で、もう片方のフィールドの値をそのまま使う度合いを返す。
// 上下の行から飛び出していなければ (くしが出ていなければ) 1、飛び出していて、前のコマから動いていれば 0 に寄せる。
float keepAt(ivec2 m) {
  float a = y(m - ivec2(0, 1));
  float b = y(m + ivec2(0, 1));
  float o = y(m);
  float over = max(0.0, max(o - max(a, b), min(a, b) - o));
  float keep = 1.0 - smoothstep(uThresholds.x, uThresholds.y, over);
  if (over > uThresholds.x && uHasPrev == 1) {
    float motion = max(abs(o - py(m)), max(abs(a - py(m - ivec2(0, 1))), abs(b - py(m + ivec2(0, 1)))));
    keep = max(keep, 1.0 - smoothstep(uThresholds.z, uThresholds.w, motion));
  }
  return keep;
}

void main() {
  // 描く先は行を下から数え、元の画像は上から数える。
  vec2 uv = vec2(gl_FragCoord.x / uOutput.x, 1.0 - gl_FragCoord.y / uOutput.y);
  ivec2 p = ivec2(uv * uSource);
  ivec2 q = ivec2(uv * uSource * 0.5);
  float luma;
  vec2 chroma;
  if (uInterlaced == 0) {
    luma = y(p);
    chroma = c(q);
  } else {
    // m は、p と同じ 2 行の組の中で、このフィールドにない輝度の行。p がその行なら p 自身。
    // 色差の行は 2 行の輝度の組に 1 行で、輝度の行がこのフィールドにあっても、色差の行は抜けていることがある。
    // そのときも、組の中の抜けた輝度の行で、動いているかを測る。
    ivec2 m = ivec2(p.x, (p.y & ~1) | (1 - uField));
    float keep = keepAt(m);
    if ((p.y & 1) == uField) {
      luma = y(p);
    } else {
      luma = mix((y(p - ivec2(0, 1)) + y(p + ivec2(0, 1))) * 0.5, y(p), keep);
    }
    // 色差の行も、フィールドごとに交互に並ぶ。抜けた行は、輝度で決めた度合いで、上下の平均ともう片方の値を混ぜる。
    if ((q.y & 1) == uField) {
      chroma = c(q);
    } else {
      vec2 ca = c(q - ivec2(0, 1));
      vec2 cb = c(q + ivec2(0, 1));
      chroma = mix((ca + cb) * 0.5, c(q), keep);
    }
  }
  // 限定レンジ (Y は 16〜235、C は 16〜240 で中心 128) を伸ばす。
  vec3 yuv = vec3(
    (luma - 16.0 / 255.0) * (255.0 / 219.0),
    (chroma.x - 128.0 / 255.0) * (255.0 / 224.0),
    (chroma.y - 128.0 / 255.0) * (255.0 / 224.0)
  );
  fragColor = vec4(clamp(uMatrix * yuv, 0.0, 1.0), 1.0);
}`;

// matrices は、YUV (Y、Cb、Cr) から RGB への変換。WebGL の mat3 は列ごとに並べる。
const matrices: Record<ColorMatrix, Float32Array> = {
  bt709: new Float32Array([1, 1, 1, 0, -0.1873, 1.8556, 1.5748, -0.4681, 0]),
  bt601: new Float32Array([1, 1, 1, 0, -0.3441, 1.772, 1.402, -0.7141, 0]),
};

const uniformNames = [
  'uY',
  'uU',
  'uV',
  'uPrevY',
  'uField',
  'uInterlaced',
  'uHasPrev',
  'uSource',
  'uOutput',
  'uThresholds',
  'uMatrix',
] as const;

type Uniforms = Record<(typeof uniformNames)[number], WebGLUniformLocation | null>;

// PictureRenderer は、OffscreenCanvas に WebGL2 でコマを描く。
export class PictureRenderer {
  private readonly gl: WebGL2RenderingContext;
  private readonly program: WebGLProgram;
  private readonly uniforms: Uniforms;
  // free は、使い終えたテクスチャの組。大きさが同じなら使い回す。
  private readonly free: PictureSlot[] = [];

  constructor(
    private readonly canvas: OffscreenCanvas,
    private readonly thresholds: RendererThresholds = defaultRendererThresholds,
  ) {
    const gl = canvas.getContext('webgl2', {
      alpha: false,
      antialias: false,
      depth: false,
      stencil: false,
      preserveDrawingBuffer: false,
      // ソフトウェアの描画 (SwiftShader) では、1 コマの描画がコマの持ち時間を超えるおそれがあるので使わない。
      failIfMajorPerformanceCaveat: true,
    });
    if (gl === null) {
      throw new Error('WebGL2 is not available');
    }
    this.gl = gl;
    this.program = compile(gl);
    this.uniforms = Object.fromEntries(
      uniformNames.map((name) => [name, gl.getUniformLocation(this.program, name)]),
    ) as Uniforms;
  }

  // resize は、描く先の大きさを、映す大きさ (画素の縦横比を掛けた後) にする。変えたら true を返す。
  resize(width: number, height: number): boolean {
    if (this.canvas.width === width && this.canvas.height === height) {
      return false;
    }
    this.canvas.width = width;
    this.canvas.height = height;

    return true;
  }

  // acquire は、width × height のコマを上げるテクスチャの組を返す。使い終えたら release に返す。
  acquire(width: number, height: number): PictureSlot {
    const index = this.free.findIndex((slot) => slot.width === width && slot.height === height);
    if (index >= 0) {
      return this.free.splice(index, 1)[0]!;
    }
    const { gl } = this;

    return {
      y: texture(gl, width, height),
      u: texture(gl, width >> 1, height >> 1),
      v: texture(gl, width >> 1, height >> 1),
      width,
      height,
    };
  }

  // release は、使い終えたテクスチャの組を返す。大きさの違うものが溜まらないよう、違う大きさの組は消す。
  release(slot: PictureSlot): void {
    for (const other of this.free.filter(
      (free) => free.width !== slot.width || free.height !== slot.height,
    )) {
      this.destroy(other);
    }
    const kept = this.free.filter(
      (free) => free.width === slot.width && free.height === slot.height,
    );
    this.free.length = 0;
    this.free.push(...kept, slot);
  }

  // upload は、WASM のメモリ heap にある 3 面を、slot のテクスチャへ上げる。JS 側では複製しない。
  upload(
    slot: PictureSlot,
    heap: Uint8Array,
    planes: readonly [PlanePointer, PlanePointer, PlanePointer],
  ): void {
    const { gl } = this;
    gl.pixelStorei(gl.UNPACK_ALIGNMENT, 1);
    const targets = [
      [slot.y, slot.width, slot.height],
      [slot.u, slot.width >> 1, slot.height >> 1],
      [slot.v, slot.width >> 1, slot.height >> 1],
    ] as const;
    targets.forEach(([target, width, height], i) => {
      const plane = planes[i]!;
      // 行の右の詰め物は、1 行のバイト数を渡して飛ばす。
      gl.pixelStorei(gl.UNPACK_ROW_LENGTH, plane.stride);
      gl.bindTexture(gl.TEXTURE_2D, target);
      gl.texSubImage2D(
        gl.TEXTURE_2D,
        0,
        0,
        0,
        width,
        height,
        gl.RED,
        gl.UNSIGNED_BYTE,
        heap,
        plane.pointer,
      );
    });
    gl.pixelStorei(gl.UNPACK_ROW_LENGTH, 0);
  }

  // draw は、request のコマとフィールドを描く。
  draw(request: DrawRequest): void {
    const { gl, uniforms } = this;
    const { slot, previous } = request;
    gl.viewport(0, 0, this.canvas.width, this.canvas.height);
    gl.useProgram(this.program);
    const bind = (unit: number, target: WebGLTexture, location: WebGLUniformLocation | null) => {
      gl.activeTexture(gl.TEXTURE0 + unit);
      gl.bindTexture(gl.TEXTURE_2D, target);
      gl.uniform1i(location, unit);
    };
    bind(0, slot.y, uniforms.uY);
    bind(1, slot.u, uniforms.uU);
    bind(2, slot.v, uniforms.uV);
    // 前のコマは、大きさが同じときだけ比べる (選局した直後と、大きさが変わった直後は、コマの中だけで決める)。
    const hasPrevious =
      previous !== null && previous.width === slot.width && previous.height === slot.height;
    bind(3, hasPrevious ? previous.y : slot.y, uniforms.uPrevY);
    gl.uniform1i(uniforms.uField, request.parity);
    gl.uniform1i(uniforms.uInterlaced, request.interlaced ? 1 : 0);
    gl.uniform1i(uniforms.uHasPrev, hasPrevious ? 1 : 0);
    gl.uniform2f(uniforms.uSource, slot.width, slot.height);
    gl.uniform2f(uniforms.uOutput, this.canvas.width, this.canvas.height);
    const { weave, bob, still, motion } = this.thresholds;
    gl.uniform4f(uniforms.uThresholds, weave, bob, still, motion);
    gl.uniformMatrix3fv(uniforms.uMatrix, false, matrices[request.matrix]);
    gl.drawArrays(gl.TRIANGLES, 0, 3);
  }

  // dispose は、持っているテクスチャと program を消す。
  dispose(): void {
    for (const slot of this.free.splice(0)) {
      this.destroy(slot);
    }
    this.gl.deleteProgram(this.program);
  }

  private destroy(slot: PictureSlot): void {
    this.gl.deleteTexture(slot.y);
    this.gl.deleteTexture(slot.u);
    this.gl.deleteTexture(slot.v);
  }
}

function texture(gl: WebGL2RenderingContext, width: number, height: number): WebGLTexture {
  const target = gl.createTexture();
  gl.bindTexture(gl.TEXTURE_2D, target);
  gl.texStorage2D(gl.TEXTURE_2D, 1, gl.R8, width, height);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);

  return target;
}

function compile(gl: WebGL2RenderingContext): WebGLProgram {
  const shader = (type: number, source: string) => {
    const created = gl.createShader(type);
    if (created === null) {
      throw new Error('failed to create a shader');
    }
    gl.shaderSource(created, source);
    gl.compileShader(created);
    if (!gl.getShaderParameter(created, gl.COMPILE_STATUS)) {
      throw new Error(`failed to compile a shader: ${gl.getShaderInfoLog(created) ?? ''}`);
    }

    return created;
  };
  const program = gl.createProgram();
  gl.attachShader(program, shader(gl.VERTEX_SHADER, vertexShader));
  gl.attachShader(program, shader(gl.FRAGMENT_SHADER, fragmentShader));
  gl.linkProgram(program);
  if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
    throw new Error(`failed to link the program: ${gl.getProgramInfoLog(program) ?? ''}`);
  }

  return program;
}
