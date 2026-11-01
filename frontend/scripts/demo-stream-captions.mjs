// 静的なデモ (#832、#1050) の映像に、ARIB 字幕 (STD-B24) のストリームを足す。scripts/build-demo-stream.sh から呼ぶ。
//
//   node scripts/demo-stream-captions.mjs <ffmpeg で作った TS> <書き出す TS>
//
// ffmpeg は ARIB 字幕を書けないので、字幕の PES を自前で作り、TS に差し込む。
// 字幕の文は、aribb24.js (画面で字幕を描くのと同じライブラリ) の符号化器で、8 単位符号のデータグループにする。
// 画面の変換 (mpeg2toh264) は、PMT で component_tag が 0x30 のストリームを字幕として読むので、PMT にこのストリームを足す。
import { readFile, writeFile } from 'node:fs/promises';

import {
  ARIBB24ActivePositionSetToken,
  ARIBB24CharacterCompositionDotDesignationToken,
  ARIBB24CharacterToken,
  ARIBB24ClearScreenToken,
  ARIBB24JapaneseJIS8Encoder,
  ARIBB24NormalSizeToken,
  ARIBB24SetDisplayFormatToken,
  ARIBB24SetDisplayPositionToken,
  ARIBB24SetHorizontalSpacingToken,
  ARIBB24SetVerticalSpacingToken,
  ARIBB24SetWritingFormatToken,
  ARIBB24WhiteForegroundToken,
  muxDatagroup,
  muxIndependentPES,
} from 'aribb24.js';

// captions は、字幕の文と、出す時間 (映像の頭からの秒)。テロップ (scripts/demo-stream-scene.mjs) の上に出す。
const captions = [
  { from: 1.0, to: 4.8, lines: ['こたつ山のふもとから', 'けさの様子をお伝えします'] },
  { from: 5.2, to: 9.4, lines: ['空気は冷たく', '畑には霜が降りました'] },
  { from: 10.6, to: 14.6, lines: ['きょうは一日', 'よく晴れる見込みです'] },
  { from: 15.0, to: 19.0, lines: ['こたつでぬくぬくしながら', 'お出かけの準備をどうぞ'] },
];

const packetSize = 188;
const captionPID = 0x0130;
// leadSeconds は、字幕の PES を、表示する時刻 (PTS) のどれだけ前に差し込むか。PCR を基準に数える。
const leadSeconds = 0.5;

// 字幕の画面は、960x540 の横書き (SWF 7) で、1 文字を 36x36 ドット、字間 4、行間 24 にする。1 行に 24 文字、9 行入る。
const columns = 24;
const firstRow = 3;

// statement は、字幕の文を出す (lines が空なら、消すだけの) 字幕文のデータグループを返す。
function statement(lines) {
  const tokens = [
    ARIBB24ClearScreenToken.from(),
    ARIBB24SetWritingFormatToken.from(7),
    ARIBB24SetDisplayFormatToken.from(960, 540),
    ARIBB24SetDisplayPositionToken.from(0, 0),
    ARIBB24CharacterCompositionDotDesignationToken.from(36, 36),
    ARIBB24SetHorizontalSpacingToken.from(4),
    ARIBB24SetVerticalSpacingToken.from(24),
    ARIBB24NormalSizeToken.from(),
    ARIBB24WhiteForegroundToken.from(),
  ];
  lines.forEach((line, index) => {
    const characters = [...line];
    // 行の中ほどに寄せる。APS の行は、文字の下端の位置を指す。
    tokens.push(
      ARIBB24ActivePositionSetToken.from(
        Math.floor((columns - characters.length) / 2),
        firstRow + index + 1,
      ),
    );
    tokens.push(...characters.map((character) => ARIBB24CharacterToken.from(character)));
  });
  const units = new ARIBB24JapaneseJIS8Encoder().encode(tokens);

  return muxDatagroup({ tag: 'CaptionStatement', group: 0, lang: 0, timeControlMode: 0, units });
}

// management は、字幕管理データ (日本語の 1 言語、受信時に自動で出す、8 単位符号) のデータグループを返す。
function management() {
  return muxDatagroup({
    tag: 'CaptionManagement',
    group: 0,
    timeControlMode: 0,
    languages: [
      { lang: 0, displayMode: 0, iso_639_language_code: 'jpn', format: 0, TCS: 0, rollup: 0 },
    ],
    units: [],
  });
}

// pes は、PTS を付けた同期型 PES (private_stream_1) を返す。
function pes(dataGroup, pts) {
  const payload = new Uint8Array(muxIndependentPES({ tag: 'Caption', data: dataGroup }));
  const header = new Uint8Array(14);
  header.set([0x00, 0x00, 0x01, 0xbd]);
  const length = 3 + 5 + payload.length;
  header[4] = length >> 8;
  header[5] = length & 0xff;
  header[6] = 0x80;
  // PTS だけを持つ。
  header[7] = 0x80;
  header[8] = 5;
  // PTS は 33 ビットあるので、ビット演算でなく割り算で切り出す。
  const bits = (shift, mask) => Math.floor(pts / 2 ** shift) & mask;
  header[9] = 0x21 | (bits(30, 0x07) << 1);
  header[10] = bits(22, 0xff);
  header[11] = (bits(15, 0x7f) << 1) | 1;
  header[12] = bits(7, 0xff);
  header[13] = (bits(0, 0x7f) << 1) | 1;

  return Buffer.concat([header, payload]);
}

// continuity は、字幕の PID の continuity_counter。
let continuity = 0;

// packetize は、PES を TS のパケットに分ける。最後のパケットは、アダプテーションフィールドで埋める。
function packetize(data) {
  const packets = [];
  for (let offset = 0; offset < data.length; offset += 184) {
    const chunk = data.subarray(offset, offset + 184);
    const packet = Buffer.alloc(packetSize, 0xff);
    packet[0] = 0x47;
    packet[1] = (offset === 0 ? 0x40 : 0x00) | (captionPID >> 8);
    packet[2] = captionPID & 0xff;
    const stuffing = 184 - chunk.length;
    if (stuffing === 0) {
      packet[3] = 0x10 | continuity;
      chunk.copy(packet, 4);
    } else {
      packet[3] = 0x30 | continuity;
      packet[4] = stuffing - 1;
      if (stuffing > 1) {
        packet[5] = 0x00;
      }
      chunk.copy(packet, 4 + stuffing);
    }
    continuity = (continuity + 1) & 0x0f;
    packets.push(packet);
  }

  return packets;
}

// crc32 は、PSI のセクションの CRC (MPEG-2) を返す。
function crc32(data) {
  let crc = 0xffffffff;
  for (const byte of data) {
    crc ^= byte << 24;
    for (let bit = 0; bit < 8; bit++) {
      crc = crc & 0x80000000 ? (crc << 1) ^ 0x04c11db7 : crc << 1;
    }
  }

  return crc >>> 0;
}

function pid(packet) {
  return ((packet[1] & 0x1f) << 8) | packet[2];
}

// payloadOffset は、パケットのペイロードの始まりを返す。ペイロードがなければ null。
function payloadOffset(packet) {
  const control = (packet[3] >> 4) & 0x03;
  if ((control & 0x01) === 0) {
    return null;
  }

  return control === 0x03 ? 5 + packet[4] : 4;
}

// pcr は、パケットの PCR (90 kHz の部分) を返す。なければ null。
function pcr(packet) {
  const control = (packet[3] >> 4) & 0x03;
  if ((control & 0x02) === 0 || packet[4] === 0 || (packet[5] & 0x10) === 0) {
    return null;
  }

  return (
    packet[6] * 2 ** 25 +
    packet[7] * 2 ** 17 +
    packet[8] * 2 ** 9 +
    packet[9] * 2 +
    (packet[10] >> 7)
  );
}

// pesPTS は、PES の頭のパケットの PTS を返す。なければ null。
function pesPTS(packet) {
  const offset = payloadOffset(packet);
  if (offset === null || (packet[1] & 0x40) === 0) {
    return null;
  }
  const p = packet.subarray(offset);
  if (p[0] !== 0 || p[1] !== 0 || p[2] !== 1 || (p[7] & 0x80) === 0) {
    return null;
  }

  return (
    ((p[9] >> 1) & 0x07) * 2 ** 30 +
    p[10] * 2 ** 22 +
    (p[11] >> 1) * 2 ** 15 +
    p[12] * 2 ** 7 +
    (p[13] >> 1)
  );
}

// rewritePMT は、PMT のセクションに、字幕のストリームを足す。PMT は 1 つのパケットに収まっているものとする。
function rewritePMT(packet) {
  const offset = payloadOffset(packet);
  if (offset === null || (packet[1] & 0x40) === 0) {
    throw new Error('PMT does not start in this packet');
  }
  const start = offset + 1 + packet[offset];
  const sectionLength = ((packet[start + 1] & 0x0f) << 8) | packet[start + 2];
  // CRC を除いたセクション。
  const body = packet.subarray(start, start + 3 + sectionLength - 4);
  // stream_identifier_descriptor (component_tag 0x30) と、data_component_descriptor (ARIB 字幕、0x0008)。
  const descriptors = [0x52, 0x01, 0x30, 0xfd, 0x03, 0x00, 0x08, 0x3d];
  const stream = [
    0x06,
    0xe0 | (captionPID >> 8),
    captionPID & 0xff,
    0xf0,
    descriptors.length,
    ...descriptors,
  ];
  const section = Buffer.concat([body, Buffer.from(stream)]);
  const newLength = section.length - 3 + 4;
  section[1] = (section[1] & 0xf0) | (newLength >> 8);
  section[2] = newLength & 0xff;
  const crc = Buffer.alloc(4);
  crc.writeUInt32BE(crc32(section));
  const rewritten = Buffer.concat([section, crc]);
  if (start + rewritten.length > packetSize) {
    throw new Error('PMT does not fit in one packet');
  }
  const out = Buffer.alloc(packetSize, 0xff);
  packet.copy(out, 0, 0, start);
  rewritten.copy(out, start);

  return out;
}

const [input, output] = process.argv.slice(2);
if (input === undefined || output === undefined) {
  throw new Error('usage: node scripts/demo-stream-captions.mjs <input.ts> <output.ts>');
}
const source = await readFile(input);
if (source.length % packetSize !== 0) {
  throw new Error('input is not a 188-byte TS');
}
const packets = [];
for (let offset = 0; offset < source.length; offset += packetSize) {
  packets.push(source.subarray(offset, offset + packetSize));
}

// PAT から PMT の PID と、PMT から映像の PID を読む。
const pat = packets.find((packet) => pid(packet) === 0 && (packet[1] & 0x40) !== 0);
if (pat === undefined) {
  throw new Error('PAT is not found');
}
const patSection = pat.subarray(payloadOffset(pat) + 1 + pat[payloadOffset(pat)]);
const pmtPID = ((patSection[10] & 0x1f) << 8) | patSection[11];
const pmt = packets.find((packet) => pid(packet) === pmtPID && (packet[1] & 0x40) !== 0);
const pmtSection = pmt.subarray(payloadOffset(pmt) + 1 + pmt[payloadOffset(pmt)]);
const pcrPID = ((pmtSection[8] & 0x1f) << 8) | pmtSection[9];
const videoPID = (() => {
  const programInfoLength = ((pmtSection[10] & 0x0f) << 8) | pmtSection[11];
  const sectionLength = ((pmtSection[1] & 0x0f) << 8) | pmtSection[2];
  for (let i = 12 + programInfoLength; i < 3 + sectionLength - 4;) {
    const esInfoLength = ((pmtSection[i + 3] & 0x0f) << 8) | pmtSection[i + 4];
    if (pmtSection[i] === 0x02) {
      return ((pmtSection[i + 1] & 0x1f) << 8) | pmtSection[i + 2];
    }
    i += 5 + esInfoLength;
  }
  throw new Error('MPEG-2 video is not found in PMT');
})();

// 字幕の時刻の基準は、映像の最初の PTS。
const firstVideoPTS = packets
  .filter((packet) => pid(packet) === videoPID)
  .map(pesPTS)
  .find((pts) => pts !== null);
if (firstVideoPTS === undefined) {
  throw new Error('video PTS is not found');
}

// 差し込む PES。字幕管理データは、途中から見始めても字幕が出るよう、5 秒ごとに送る。
const units = [];
for (let at = 0; at < 20; at += 5) {
  units.push({ at, data: management() });
}
for (const caption of captions) {
  units.push({ at: caption.from, data: statement(caption.lines) });
  units.push({ at: caption.to, data: statement([]) });
}
units.sort((a, b) => a.at - b.at);
const pending = units.map(({ at, data }) => {
  const pts = firstVideoPTS + Math.round(at * 90000);

  return { pts, packets: packetize(pes(data, pts)) };
});

// PCR が、PES の PTS の leadSeconds 前を過ぎたところで差し込む。
const out = [];
let lastPCR = null;
for (const packet of packets) {
  if (pid(packet) === pcrPID) {
    lastPCR = pcr(packet) ?? lastPCR;
  }
  while (
    pending.length > 0 &&
    lastPCR !== null &&
    lastPCR >= pending[0].pts - leadSeconds * 90000
  ) {
    out.push(...pending.shift().packets);
  }
  out.push(pid(packet) === pmtPID && (packet[1] & 0x40) !== 0 ? rewritePMT(packet) : packet);
}
for (const rest of pending) {
  out.push(...rest.packets);
}

await writeFile(output, Buffer.concat(out));
