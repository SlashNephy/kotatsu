// PNG に、文字の情報 (iTXt のチャンク) を書き込む (#458)。キャプチャの画像だけを見ても、どの番組のいつの場面か分かるようにする。

// PNGText は、書き込む文字の情報。keyword は、PNG の仕様で決まった名前 (Title、Source など)。
export type PNGText = { keyword: string; text: string };

const signature = [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a];

// crcTable は、PNG のチャンクの CRC-32 の表。
const crcTable = (() => {
  const table = new Uint32Array(256);
  for (let n = 0; n < 256; n++) {
    let c = n;
    for (let k = 0; k < 8; k++) {
      c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
    }
    table[n] = c >>> 0;
  }

  return table;
})();

// crc32 は、PNG のチャンクの種類とデータから CRC-32 を計算する。
export function crc32(bytes: Uint8Array): number {
  let c = 0xffffffff;
  for (const byte of bytes) {
    c = crcTable[(c ^ byte) & 0xff]! ^ (c >>> 8);
  }

  return (c ^ 0xffffffff) >>> 0;
}

// itxtChunk は、圧縮しない iTXt のチャンク (長さ、種類、データ、CRC) を作る。
// データは、キーワード、0、圧縮の有無 (0)、圧縮の方式 (0)、言語 (空)、0、訳したキーワード (空)、0、UTF-8 の文。
function itxtChunk({ keyword, text }: PNGText): Uint8Array {
  const encoder = new TextEncoder();
  const keywordBytes = encoder.encode(keyword);
  const textBytes = encoder.encode(text);
  const data = new Uint8Array(keywordBytes.length + 5 + textBytes.length);
  data.set(keywordBytes, 0);
  // キーワードの後ろの 5 バイト (区切り、圧縮の有無と方式、空の言語と訳の区切り) は、どれも 0 のままにする。
  data.set(textBytes, keywordBytes.length + 5);
  const typeAndData = new Uint8Array(4 + data.length);
  typeAndData.set(encoder.encode('iTXt'), 0);
  typeAndData.set(data, 4);
  const chunk = new Uint8Array(8 + data.length + 4);
  const view = new DataView(chunk.buffer);
  view.setUint32(0, data.length);
  chunk.set(typeAndData, 4);
  view.setUint32(8 + data.length, crc32(typeAndData));

  return chunk;
}

// withPNGText は、PNG の IHDR の直後に、texts の iTXt のチャンクを足した PNG を返す。
// 空の文は書かない。PNG でなければ、そのまま返す。
export function withPNGText(
  png: Uint8Array<ArrayBuffer>,
  texts: readonly PNGText[],
): Uint8Array<ArrayBuffer> {
  if (png.length < 33 || signature.some((byte, i) => png[i] !== byte)) {
    return png;
  }
  // IHDR は、署名の直後に必ず置かれ、長さは 13 バイト。長さ、種類、データ、CRC で 25 バイト。
  const ihdrEnd = signature.length + 25;
  const chunks = texts.filter(({ text }) => text !== '').map(itxtChunk);
  const size = chunks.reduce((sum, chunk) => sum + chunk.length, 0);
  const out = new Uint8Array(png.length + size);
  out.set(png.subarray(0, ihdrEnd), 0);
  let offset = ihdrEnd;
  for (const chunk of chunks) {
    out.set(chunk, offset);
    offset += chunk.length;
  }
  out.set(png.subarray(ihdrEnd), offset);

  return out;
}
