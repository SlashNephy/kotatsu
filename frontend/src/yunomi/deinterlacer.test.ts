import { describe, expect, test } from 'vitest';

import { createDeinterlacer, enableFilm } from './deinterlacer';

describe('enableFilm', () => {
  test('GPU が対応していれば、プルダウンの検出を入れる', () => {
    const deinterlacer = { film: false };
    enableFilm(deinterlacer);
    expect(deinterlacer.film).toBe(true);
  });

  test('GPU が対応していなければ、入れずに続ける', () => {
    // yadif は、EXT_color_buffer_float がなければ、film を入れるときに投げ、film を切ったままにする。
    let film = false;
    const deinterlacer = {
      get film() {
        return film;
      },
      set film(next: boolean) {
        if (next) {
          throw new Error('film needs EXT_color_buffer_float');
        }
        film = next;
      },
    };
    expect(() => enableFilm(deinterlacer)).not.toThrow();
    expect(deinterlacer.film).toBe(false);
  });
});

describe('createDeinterlacer', () => {
  const timeline = [{ start: 0, codedSize: { width: 1440, height: 1080 } }];

  test('作れれば、作ったデインタレーサーをそのまま返す', () => {
    const made = { running: true, scan: null, enabled: true, videoTimeline: [], destroy() {} };
    const { deinterlacer, available } = createDeinterlacer(() => made);
    expect(deinterlacer).toBe(made);
    expect(available).toBe(true);
  });

  test('WebGL2 がなく作れなければ、解除せずに映像の表だけを持つものを返す', () => {
    const { deinterlacer, available } = createDeinterlacer(() => {
      throw new Error('this browser has no WebGL2');
    });
    expect(available).toBe(false);
    // プレイヤーは、作った後に映像の表と有効かどうかを入れる。入れても解除は動かない。
    deinterlacer.videoTimeline = timeline;
    deinterlacer.enabled = true;
    expect(deinterlacer.running).toBe(false);
    expect(deinterlacer.scan).toBeNull();
    // 放送の映像の大きさは、この表から読むので、入れたものを返す。
    expect(deinterlacer.videoTimeline).toBe(timeline);
    expect(() => deinterlacer.destroy()).not.toThrow();
  });
});
