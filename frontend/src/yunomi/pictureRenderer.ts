// ブラウザ内でデコードする経路 (#1516) の、worker の OffscreenCanvas に WebGL2 でコマを描く部分。
// デコードしたコマの Y、U、V の 3 面を R8 のテクスチャ 3 枚に上げ (JS 側で複製せずに、WASM のメモリから直に)、
// インタレースのコマはフィールドごとに描く。描くフィールドの行はそのまま使い、抜けた行は、もう片方のフィールドの値 o が
// 上下の行 a、b の間からはみ出した量で、織り (o) と bob ((a+b)/2) をなだらかに混ぜる。はみ出しても、前のコマと比較して
// 止まっている画素は o を使う (縁取りのある字や細い横線が、フィールドごとに上下に揺れないように)。
// 色差 (4:2:0) も同じ規則で埋め、止まっている度合いは輝度で決めたものを使う。
// 映像を拡大するとき (#1654、#2276) は、解除したコマを、元の (符号化された) 大きさのテクスチャに描き、拡大 (pictureUpscaler) に
// 渡して canvas へ描かせる。拡大は、yadif に渡すときと同じ PictureScaler の約束で呼ぶ。拡大しないときは、今までどおり
// canvas へ直に描く。

import type { PictureScaler } from '@mpeg2toh264/yadif';

import { maxDimension, type Size } from './pictureUpscaler';

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
  // previous は、前に映したコマ。大きさが違えば比較しない。
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
    // そのときも、組の中の抜けた輝度の行で、動いているかを計測する。
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

// matrices は、YUV (Y、Cb、Cr) から RGB への変換。WebGL の mat3 はカラムごとに並べる。
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

// keepAspect は、拡大して描く大きさ output を、映す大きさ picture の縦横比に広げて返す。広げると limit (GPU が描ける一辺の上限) を
// 超えるなら、比率を保って縮める。拡大の処理は片方の向きだけ拡大するときや、横に引き伸ばす切り抜き (#824) で、映像と違う比率の
// 大きさを返す。canvas は CSS (object-fit: contain) で枠に収めるので、比率が違うと絵が縦か横に縮む。データ放送へ移すときも
// object-fit: contain で広げる (mediaFit.ts) ので、canvas を置き直すのではなく、描く大きさの比率を映像にそろえる。
export function keepAspect(output: Size, picture: Size, limit: number): Size {
  const aspect = picture.width / picture.height;
  const width = Math.max(output.width, Math.round(output.height * aspect));
  const height = Math.max(output.height, Math.round(output.width / aspect));
  const fit = Math.min(1, limit / width, limit / height);

  return { width: Math.floor(width * fit), height: Math.floor(height * fit) };
}

// Target は、拡大する前のコマを描くテクスチャと、それを描き先にする framebuffer。
type Target = {
  framebuffer: WebGLFramebuffer;
  texture: WebGLTexture;
  width: number;
  height: number;
};

// PictureRenderer は、OffscreenCanvas に WebGL2 でコマを描く。
export class PictureRenderer {
  private readonly gl: WebGL2RenderingContext;
  private readonly program: WebGLProgram;
  private readonly uniforms: Uniforms;
  // free は、使い終えたテクスチャの組。大きさが同じなら使い回す。
  private readonly free: PictureSlot[] = [];
  // picture は、映す大きさ (画素の縦横比を掛けた後)。拡大しないときは、canvas もこの大きさにする。
  private picture: Size = { width: 0, height: 0 };
  // frame は、コマの元の (符号化された) 大きさ。拡大するときはこの大きさで解除してから拡大する。
  // 地上波の 1440×1080 を、映す大きさ (1920×1080) に伸ばしてから拡大すると、1080p の画面では拡大しなくなり、
  // 大きな画面でも、最近傍で伸ばした画素を拡大することになる。
  private frame: Size = { width: 0, height: 0 };
  // scaler は、コマを拡大して描く処理。拡大しなければ null。box は、絵を映す矩形の端末の画素での大きさ。
  private scaler: PictureScaler | null = null;
  private box: Size | null = null;
  // output は、拡大して描く大きさ (canvas の大きさ)。拡大しなければ null。
  private output: Size | null = null;
  // target は、拡大する前のコマを描く先。拡大するときだけ作る。
  private target: Target | null = null;

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

  // resize は、映す大きさ picture (画素の縦横比を掛けた後) と、コマの元の大きさ frame を変え、描く先の大きさを決め直す。
  resize(picture: Size, frame: Size): void {
    this.picture = picture;
    this.frame = frame;
    this.layout();
  }

  // setScaler は、拡大の処理と、絵を映す矩形の端末の画素での大きさを変え、描く先の大きさを決め直す。
  // 大きさの決め方は拡大の設定にもよるので、設定を変えたときにも呼ぶ。描き直す必要があれば true を返す。
  // canvas の大きさが変わると描いたものは消え、拡大している間は鮮鋭化の強さなどを描くときに読むためである。
  setScaler(scaler: PictureScaler | null, box: Size | null): boolean {
    this.scaler = scaler;
    this.box = box;

    return this.layout() || this.output !== null;
  }

  // layout は、拡大して描く大きさを拡大の処理に決めさせ、canvas をその大きさ (拡大しなければ映す大きさ) にする。
  // 拡大しないなら、拡大する前のコマを描く先を片付ける。canvas の大きさか拡大して描く大きさを変えたら true を返す。
  private layout(): boolean {
    const { picture, frame, scaler, box, gl } = this;
    const before = this.output;
    const size =
      scaler === null || box === null || picture.width <= 0 || frame.width <= 0
        ? null
        : scaler.size(gl, frame.width, frame.height, box.width, box.height);
    this.output = size === null ? null : keepAspect(size, picture, maxDimension(gl));
    if (this.output === null) {
      this.releaseTarget();
    }
    const canvas = this.output ?? picture;
    const resized = this.canvas.width !== canvas.width || this.canvas.height !== canvas.height;
    if (resized) {
      this.canvas.width = canvas.width;
      this.canvas.height = canvas.height;
    }

    return (
      resized || before?.width !== this.output?.width || before?.height !== this.output?.height
    );
  }

  // releaseTarget は、拡大する前のコマを描く先を消す。
  private releaseTarget(): void {
    if (this.target !== null) {
      this.gl.deleteFramebuffer(this.target.framebuffer);
      this.gl.deleteTexture(this.target.texture);
      this.target = null;
    }
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

  // draw は、request のコマとフィールドを描く。拡大するなら、元の大きさのテクスチャに描いてから、拡大して canvas へ描かせる。
  // 拡大できなければ (作れない、途中で失敗した)、PictureScaler の約束のとおり、以後は拡大せずに映す大きさで描く。
  draw(request: DrawRequest): void {
    const { gl, scaler, output } = this;
    const { width, height } = request.slot;
    if (scaler === null || output === null) {
      this.drawPicture(request, null, this.canvas.width, this.canvas.height);

      return;
    }
    const target = this.ensureTarget(width, height);
    this.drawPicture(request, target.framebuffer, width, height);
    gl.bindFramebuffer(gl.FRAMEBUFFER, null);
    if (scaler.draw(gl, target.texture, false, width, height, output.width, output.height)) {
      return;
    }
    this.scaler = null;
    this.layout();
    this.drawPicture(request, null, this.canvas.width, this.canvas.height);
  }

  // ensureTarget は、width × height の、拡大する前のコマを描く先を返す。大きさが変われば作り直す。
  private ensureTarget(width: number, height: number): Target {
    const current = this.target;
    if (current !== null && current.width === width && current.height === height) {
      return current;
    }
    const { gl } = this;
    this.releaseTarget();
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
    gl.bindFramebuffer(gl.FRAMEBUFFER, null);
    this.target = { framebuffer, texture, width, height };

    return this.target;
  }

  // drawPicture は、request のコマとフィールドを、framebuffer (null なら canvas) へ width × height で描く。
  // 描く先の行は下から数える (framebuffer の向き) ので、拡大に渡すテクスチャも行を下から持つ。
  private drawPicture(
    request: DrawRequest,
    framebuffer: WebGLFramebuffer | null,
    width: number,
    height: number,
  ): void {
    const { gl, uniforms } = this;
    const { slot, previous } = request;
    gl.bindFramebuffer(gl.FRAMEBUFFER, framebuffer);
    gl.viewport(0, 0, width, height);
    gl.useProgram(this.program);
    const bind = (unit: number, target: WebGLTexture, location: WebGLUniformLocation | null) => {
      gl.activeTexture(gl.TEXTURE0 + unit);
      gl.bindTexture(gl.TEXTURE_2D, target);
      gl.uniform1i(location, unit);
    };
    bind(0, slot.y, uniforms.uY);
    bind(1, slot.u, uniforms.uU);
    bind(2, slot.v, uniforms.uV);
    // 前のコマは、大きさが同じときだけ比較する (選局した直後と、大きさが変わった直後は、コマの中だけで決める)。
    const hasPrevious =
      previous !== null && previous.width === slot.width && previous.height === slot.height;
    bind(3, hasPrevious ? previous.y : slot.y, uniforms.uPrevY);
    gl.uniform1i(uniforms.uField, request.parity);
    gl.uniform1i(uniforms.uInterlaced, request.interlaced ? 1 : 0);
    gl.uniform1i(uniforms.uHasPrev, hasPrevious ? 1 : 0);
    gl.uniform2f(uniforms.uSource, slot.width, slot.height);
    gl.uniform2f(uniforms.uOutput, width, height);
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
    this.releaseTarget();
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
