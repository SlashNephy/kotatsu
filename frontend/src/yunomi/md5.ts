// md5 は、bytes の MD5 を小文字の 16 進で返す (RFC 1321)。
// 字幕の外字 (DRCS) を見分ける鍵に使う (#825)。aribb24.js は、外字の画像の MD5 で置き換える文字を引くが、その関数を公開していない。
// Web Crypto には MD5 が無いので、ここで計算する。外字は数百バイトなので、速さは気にしなくてよい。
export function md5(bytes: ArrayBuffer | Uint8Array): string {
  const input = bytes instanceof Uint8Array ? bytes : new Uint8Array(bytes);
  // 1 を 1 ビット足し、長さ (ビット) の 8 バイトが末尾に入るよう、64 バイトの倍数まで 0 で埋める。
  const length = Math.floor((input.byteLength + 8) / 64 + 1) * 64;
  const padded = new Uint8Array(length);
  padded.set(input);
  padded[input.byteLength] = 0x80;
  const view = new DataView(padded.buffer);
  const bits = input.byteLength * 8;
  view.setUint32(length - 8, bits >>> 0, true);
  view.setUint32(length - 4, Math.floor(bits / 2 ** 32), true);

  let a = 0x67452301;
  let b = 0xefcdab89;
  let c = 0x98badcfe;
  let d = 0x10325476;
  for (let offset = 0; offset < length; offset += 64) {
    let [aa, bb, cc, dd] = [a, b, c, d];
    for (let i = 0; i < 64; i++) {
      let f: number;
      let g: number;
      if (i < 16) {
        f = (bb & cc) | (~bb & dd);
        g = i;
      } else if (i < 32) {
        f = (dd & bb) | (~dd & cc);
        g = (5 * i + 1) % 16;
      } else if (i < 48) {
        f = bb ^ cc ^ dd;
        g = (3 * i + 5) % 16;
      } else {
        f = cc ^ (bb | ~dd);
        g = (7 * i) % 16;
      }
      const sum = (aa + f + constants[i]! + view.getUint32(offset + g * 4, true)) | 0;
      const shift = shifts[i]!;
      [aa, dd, cc] = [dd, cc, bb];
      bb = (bb + ((sum << shift) | (sum >>> (32 - shift)))) | 0;
    }
    a = (a + aa) | 0;
    b = (b + bb) | 0;
    c = (c + cc) | 0;
    d = (d + dd) | 0;
  }

  const digest = new DataView(new ArrayBuffer(16));
  [a, b, c, d].forEach((word, i) => digest.setUint32(i * 4, word, true));

  return Array.from(new Uint8Array(digest.buffer), (byte) =>
    byte.toString(16).padStart(2, '0'),
  ).join('');
}

// shifts は、各回で左に回すビット数。
const shifts = [
  7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22, 5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14,
  20, 5, 9, 14, 20, 4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23, 6, 10, 15, 21, 6,
  10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21,
];

// constants は、各回で足す定数。floor(abs(sin(i + 1)) * 2^32) である。
const constants = Array.from(
  { length: 64 },
  (_, i) => Math.floor(Math.abs(Math.sin(i + 1)) * 2 ** 32) | 0,
);
