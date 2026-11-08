import { describe, expect, test } from 'vitest';

import { enableFilm } from './deinterlacer';

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
