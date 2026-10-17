import { beforeEach, describe, expect, test } from 'vitest';

import { clearDataBroadcastStorage } from './dataBroadcastStorage';
import { writeZipcodeNVRAM, zipcodeNVRAMKey } from './dataBroadcastZipcode';

describe('clearDataBroadcastStorage', () => {
  beforeEach(() => localStorage.clear());

  test('web-bml の保存データだけを消し、郵便番号と Kotatsu の設定は残す', () => {
    localStorage.setItem('kotatsu.bml.nvram_prefix=~%2FX%2FNVRAM_00', 'AAAA');
    localStorage.setItem('kotatsu.bml.nvram_prefix=~%2FY%2FNVRAM_01', 'BBBB');
    localStorage.setItem('kotatsu.bml.lastTimeLoaded', '1');
    writeZipcodeNVRAM(localStorage, '1000001');
    localStorage.setItem('kotatsu.player.volume', '0.5');

    expect(clearDataBroadcastStorage(localStorage)).toBe(3);
    expect(Object.keys(localStorage).sort()).toEqual(
      [zipcodeNVRAMKey, 'kotatsu.player.volume'].sort(),
    );
  });
});
