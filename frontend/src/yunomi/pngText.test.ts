import { describe, expect, test } from 'vitest';

import { crc32, withPNGText } from './pngText';

// minimalPNG は、1x1 の PNG (署名、IHDR、IDAT、IEND)。
const minimalPNG = Uint8Array.from(
  atob(
    'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==',
  ),
  (c) => c.charCodeAt(0),
);

type Chunk = { type: string; data: Uint8Array; crcOK: boolean };

// chunks は、PNG をチャンクに分け、CRC が合っているかを添える。
function chunks(png: Uint8Array): Chunk[] {
  const view = new DataView(png.buffer, png.byteOffset, png.byteLength);
  const list: Chunk[] = [];
  let offset = 8;
  while (offset < png.length) {
    const length = view.getUint32(offset);
    const typeAndData = png.subarray(offset + 4, offset + 8 + length);
    list.push({
      type: new TextDecoder().decode(typeAndData.subarray(0, 4)),
      data: typeAndData.subarray(4),
      crcOK: view.getUint32(offset + 8 + length) === crc32(typeAndData),
    });
    offset += 12 + length;
  }

  return list;
}

// itxt は、圧縮しない iTXt のデータから、キーワードと文を取り出す。
function itxt(data: Uint8Array) {
  const keywordEnd = data.indexOf(0);
  const languageEnd = data.indexOf(0, keywordEnd + 3);
  const translatedEnd = data.indexOf(0, languageEnd + 1);

  return {
    keyword: new TextDecoder().decode(data.subarray(0, keywordEnd)),
    compressed: data[keywordEnd + 1],
    text: new TextDecoder().decode(data.subarray(translatedEnd + 1)),
  };
}

describe('crc32', () => {
  // IEND のチャンクの CRC は、どの PNG でも同じ値 (AE 42 60 82) になる。
  test('IEND の種類の CRC は 0xAE426082', () => {
    expect(crc32(new TextEncoder().encode('IEND'))).toBe(0xae426082);
  });
});

describe('withPNGText', () => {
  test('IHDR の直後に iTXt を足し、ほかのチャンクはそのまま残す', () => {
    const out = withPNGText(minimalPNG, [
      { keyword: 'Title', text: 'ニュース７' },
      { keyword: 'Source', text: 'NHK総合1・東京' },
    ]);

    const list = chunks(out);
    expect(list.map((chunk) => chunk.type)).toEqual(['IHDR', 'iTXt', 'iTXt', 'IDAT', 'IEND']);
    expect(list.every((chunk) => chunk.crcOK)).toBe(true);
    expect(list.filter((chunk) => chunk.type === 'iTXt').map((chunk) => itxt(chunk.data))).toEqual([
      { keyword: 'Title', compressed: 0, text: 'ニュース７' },
      { keyword: 'Source', compressed: 0, text: 'NHK総合1・東京' },
    ]);
    // 元の画像のチャンクは、中身を変えない。
    expect(list.filter((chunk) => chunk.type !== 'iTXt')).toEqual(chunks(minimalPNG));
  });

  test('空の文は書かない', () => {
    const out = withPNGText(minimalPNG, [
      { keyword: 'Title', text: '' },
      { keyword: 'Source', text: 'BS11' },
    ]);

    expect(chunks(out).map((chunk) => chunk.type)).toEqual(['IHDR', 'iTXt', 'IDAT', 'IEND']);
  });

  test('PNG でなければ、そのまま返す', () => {
    const notPNG = new TextEncoder().encode('GIF89a not a png at all, padding to be long enough');

    expect(withPNGText(notPNG, [{ keyword: 'Title', text: 'x' }])).toBe(notPNG);
  });
});
