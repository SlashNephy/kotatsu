import type { PlayerDeinterlacer } from '@mpeg2toh264/player';

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

// createDeinterlacer は、create でデインタレーサーを作る。作れなければ (WebGL2 のないブラウザで、yadif は作るときに投げる)、
// 解除せずに映像をそのまま見せるものを返し、available を false にする (#1617)。
// 投げたままにすると、プレイヤーがエラーとして知らせ、画面はつなぎ直しを繰り返すだけで再生できない。
// 返すものは、放送の映像の大きさを読むために、プレイヤーが入れる映像の表 (videoTimeline) を持つ。
export function createDeinterlacer(create: () => PlayerDeinterlacer): {
  deinterlacer: PlayerDeinterlacer;
  available: boolean;
} {
  try {
    return { deinterlacer: create(), available: true };
  } catch (error) {
    console.warn('deinterlacing is unavailable; showing the picture as it is', error);

    return {
      deinterlacer: {
        running: false,
        scan: null,
        enabled: false,
        videoTimeline: [],
        destroy() {},
      },
      available: false,
    };
  }
}
