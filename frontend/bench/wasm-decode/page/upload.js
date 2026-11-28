// デコードしたコマを、製品の経路 (src/yunomi/pictureRenderer.ts の upload) と同じく、WebGL2 の R8 のテクスチャ 3 枚へ上げる (#2162)。
// 上げ方 (mode) を替えて、どれが遅いかを比較する。
//   offset    製品と同じ。WASM のメモリ全体の Uint8Array と、面の位置 (srcOffset) を渡す
//   subarray  面の範囲だけの Uint8Array (subarray) を渡す。複製はしない
//   copy      面の範囲を JS の Uint8Array に複製してから渡す
export const createUploader = (mode) => {
  const canvas = new OffscreenCanvas(16, 16);
  const gl = canvas.getContext('webgl2', { antialias: false, alpha: false });
  if (!gl) throw new Error('WebGL2 is not available in the worker');
  const debug = gl.getExtension('WEBGL_debug_renderer_info');
  const info = {
    vendor: gl.getParameter(debug ? debug.UNMASKED_VENDOR_WEBGL : gl.VENDOR),
    renderer: gl.getParameter(debug ? debug.UNMASKED_RENDERER_WEBGL : gl.RENDERER),
    version: gl.getParameter(gl.VERSION),
  };
  let planes = null;
  let size = '';

  const allocate = (width, height) => {
    for (const plane of planes ?? []) gl.deleteTexture(plane.texture);
    planes = [
      [width, height],
      [width >> 1, height >> 1],
      [width >> 1, height >> 1],
    ].map(([w, h]) => {
      const texture = gl.createTexture();
      gl.bindTexture(gl.TEXTURE_2D, texture);
      gl.texStorage2D(gl.TEXTURE_2D, 1, gl.R8, w, h);
      return { texture, width: w, height: h };
    });
  };

  return {
    info,
    upload(M, dec) {
      const width = M._frame_width(dec);
      const height = M._frame_height(dec);
      if (size !== `${width}x${height}`) {
        size = `${width}x${height}`;
        allocate(width, height);
      }
      gl.pixelStorei(gl.UNPACK_ALIGNMENT, 1);
      planes.forEach((plane, i) => {
        const pointer = M._frame_data(dec, i);
        const stride = M._frame_linesize(dec, i);
        gl.pixelStorei(gl.UNPACK_ROW_LENGTH, stride);
        gl.bindTexture(gl.TEXTURE_2D, plane.texture);
        const bytes = stride * (plane.height - 1) + plane.width;
        const args = [gl.TEXTURE_2D, 0, 0, 0, plane.width, plane.height, gl.RED, gl.UNSIGNED_BYTE];
        if (mode === 'offset') {
          gl.texSubImage2D(...args, M.HEAPU8, pointer);
        } else if (mode === 'subarray') {
          gl.texSubImage2D(...args, M.HEAPU8.subarray(pointer, pointer + bytes));
        } else if (mode === 'copy') {
          gl.texSubImage2D(...args, M.HEAPU8.slice(pointer, pointer + bytes));
        } else {
          throw new Error(`unknown upload mode: ${mode}`);
        }
      });
      gl.pixelStorei(gl.UNPACK_ROW_LENGTH, 0);
    },
    flush() {
      gl.flush();
    },
    // error は gl.getError() を返す。texSubImage2D は失敗しても例外を投げないので、計測の終わりに確認する。
    error() {
      return gl.getError();
    },
    destroy() {
      for (const plane of planes ?? []) gl.deleteTexture(plane.texture);
      gl.getExtension('WEBGL_lose_context')?.loseContext();
    },
  };
};
