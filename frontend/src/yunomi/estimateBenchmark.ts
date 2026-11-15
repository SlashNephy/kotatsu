import type { EstimateTiming } from './benchmarkResults';
import { createInterpolator } from './interpolator';

// 「この端末で試す」(#1771) で、コマの補間が動きを推し量る処理の時間を測る。
// 再生とは別の WebGL2 のコンテキストで、合成の 2 コマ (粗い模様と、それをずらしたもの) を推し量る。測り終えたら捨てる。

// width と height は、測るコマの大きさ。放送の 1080i と同じ。
const width = 1440;
const height = 1080;
// repeats は、測る回数。GPU の時間は中央値を、読み戻しまでの時間は平均を使う。
const repeats = 20;
// shift は、後のコマをずらす画素の数 (原寸)。
const shift = { x: 6, y: 4 };

// pattern は、合成のコマの画素 (RGBA)。8 画素ごとの粗い模様を、dx と dy だけずらす。
function pattern(dx: number, dy: number): Uint8Array<ArrayBuffer> {
  const pixels = new Uint8Array(width * height * 4);
  for (let y = 0; y < height; y++) {
    for (let x = 0; x < width; x++) {
      const u = Math.floor((x - dx) / 8);
      const v = Math.floor((y - dy) / 8);
      // 整数の掛け算で、ブロックごとに決まった明るさを作る。
      const value = (Math.imul(u, 73_856_093) ^ Math.imul(v, 19_349_663)) & 0xff;
      const offset = (y * width + x) * 4;
      pixels[offset] = value;
      pixels[offset + 1] = 255 - value;
      pixels[offset + 2] = (value * 7) & 0xff;
      pixels[offset + 3] = 255;
    }
  }

  return pixels;
}

function upload(gl: WebGL2RenderingContext, pixels: Uint8Array<ArrayBuffer>): WebGLTexture {
  const texture = gl.createTexture();
  gl.bindTexture(gl.TEXTURE_2D, texture);
  gl.texStorage2D(gl.TEXTURE_2D, 1, gl.RGBA8, width, height);
  gl.texSubImage2D(gl.TEXTURE_2D, 0, 0, 0, width, height, gl.RGBA, gl.UNSIGNED_BYTE, pixels);

  return texture;
}

const nextFrame = () => new Promise((resolve) => setTimeout(resolve, 0));

// measureEstimate は、推し量る処理の時間を測る。WebGL2 がないか、測れなければ null。signal で止めると投げる。
export async function measureEstimate(signal?: AbortSignal): Promise<EstimateTiming | null> {
  try {
    return await measure(signal);
  } catch (error) {
    if (signal?.aborted === true) {
      throw error;
    }
    // シェーダーを作れないなど、推し量れない端末では、再生の結果だけを出す。
    console.warn('failed to measure the interpolation estimate', error);

    return null;
  }
}

async function measure(signal?: AbortSignal): Promise<EstimateTiming | null> {
  const canvas = document.createElement('canvas');
  canvas.width = 1;
  canvas.height = 1;
  const gl = canvas.getContext('webgl2');
  if (gl === null) {
    return null;
  }
  const textures = [upload(gl, pattern(0, 0)), upload(gl, pattern(shift.x, shift.y))] as const;
  const interpolator = createInterpolator(gl);
  try {
    const timer = gl.getExtension('EXT_disjoint_timer_query_webgl2') as {
      TIME_ELAPSED_EXT: number;
      GPU_DISJOINT_EXT: number;
    } | null;
    // wait は、それまでの推し量りを使って 1 画素を描き、読み戻して、GPU が描き終えるのを待つ。
    const pixel = new Uint8Array(4);
    const wait = () => {
      interpolator.compose(0.5, null, 1, 1);
      gl.readPixels(0, 0, 1, 1, gl.RGBA, gl.UNSIGNED_BYTE, pixel);
    };
    // 最初の 1 回は、シェーダーのコンパイルや資源の用意を含むので数えない。
    interpolator.estimate(textures[0], textures[1], width, height);
    wait();
    if (timer !== null) {
      const times: number[] = [];
      for (let i = 0; i < repeats; i++) {
        const query = gl.createQuery();
        gl.beginQuery(timer.TIME_ELAPSED_EXT, query);
        interpolator.estimate(textures[0], textures[1], width, height);
        gl.endQuery(timer.TIME_ELAPSED_EXT);
        // コンテキストを失うと、問い合わせは答えを返さなくなるので、待つのをやめる。
        while (!(gl.getQueryParameter(query, gl.QUERY_RESULT_AVAILABLE) as boolean | null)) {
          signal?.throwIfAborted();
          if (gl.isContextLost()) {
            return null;
          }
          await nextFrame();
        }
        const disjoint = gl.getParameter(timer.GPU_DISJOINT_EXT) as boolean;
        if (!disjoint) {
          times.push((gl.getQueryParameter(query, gl.QUERY_RESULT) as number) / 1e6);
        }
        gl.deleteQuery(query);
      }
      times.sort((a, b) => a - b);
      const middle = times[Math.floor(times.length / 2)];
      if (middle !== undefined) {
        return { ms: middle, method: 'gpu' };
      }
    }
    // GPU の時間を測れない端末 (多くのスマートフォン) では、まとめて回し、1 画素を読み戻して終わりを待つまでの時間で測る。
    const start = performance.now();
    for (let i = 0; i < repeats; i++) {
      interpolator.estimate(textures[0], textures[1], width, height);
    }
    wait();

    return { ms: (performance.now() - start) / repeats, method: 'readback' };
  } finally {
    interpolator.destroy();
    for (const texture of textures) {
      gl.deleteTexture(texture);
    }
    gl.getExtension('WEBGL_lose_context')?.loseContext();
  }
}
