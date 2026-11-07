// deinterlacerCanvas は、yadif が解除した映像を描く canvas に付ける data 属性の名前 (dataset のキー)。
// yadif は、この canvas の位置と大きさを video に合わせて自分で決めるので、外から大きさを変えない。
export const deinterlacerCanvas = 'kotatsuDeinterlacer';

// enableFilm は、デインタレーサーの 2:3 プルダウンの検出を入れる (#1284)。GPU が対応していなければ、入れずに続ける。
export function enableFilm(deinterlacer: { film: boolean }): void {
  try {
    deinterlacer.film = true;
  } catch {
    // EXT_color_buffer_float を持たない GPU では、プルダウンを検出できない。ふつうの解除のまま出す。
  }
}
