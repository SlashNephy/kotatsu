// emscripten が作る dist/decoder.js の型。関数は vendor/ffmpeg/glue.c のもの。
// ポインタは WASM のメモリの位置 (number)。Dec は glue.c の Dec のポインタ。

export type DecoderModule = {
  HEAPU8: Uint8Array;
  HEAP32: Int32Array;
  HEAPF32: Float32Array;
  _malloc(size: number): number;
  _free(pointer: number): void;
  _dec_open(kind: number): number;
  _dec_close(dec: number): void;
  _dec_skip_b(dec: number, on: number): void;
  _dec_parse(dec: number, buf: number, size: number, pts: number, out: number): number;
  _dec_send_parsed(dec: number): number;
  _dec_send(dec: number, buf: number, size: number, pts: number): number;
  _dec_receive(dec: number): number;
  _dec_flush(dec: number): number;
  _frame_data(dec: number, plane: number): number;
  _frame_linesize(dec: number, plane: number): number;
  _frame_width(dec: number): number;
  _frame_height(dec: number): number;
  _frame_format(dec: number): number;
  _frame_flags(dec: number): number;
  _frame_pict_type(dec: number): number;
  _frame_has_pts(dec: number): number;
  _frame_pts(dec: number): number;
  _frame_sar_num(dec: number): number;
  _frame_sar_den(dec: number): number;
  _frame_colorspace(dec: number): number;
  _frame_color_range(dec: number): number;
  _frame_nb_samples(dec: number): number;
  _frame_sample_rate(dec: number): number;
  _frame_channels(dec: number): number;
  _buf_alloc(size: number): number;
  _buf_free(pointer: number): void;
};

export type DecoderModuleOptions = {
  // locateFile は、decoder.wasm の URL を返す。明示的な URL で配信するので、必ず渡す。
  locateFile: (path: string) => string;
};

export default function createDecoderModule(options: DecoderModuleOptions): Promise<DecoderModule>;
