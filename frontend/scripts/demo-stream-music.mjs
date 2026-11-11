// 静的なデモ (#832、#1050) の映像の音声を合成し、WAV (48 kHz、ステレオ、32 ビット浮動小数点) で書き出す。
// scripts/build-demo-stream.sh が、AAC にして映像と合わせる。
//
//   node scripts/demo-stream-music.mjs <書き出す WAV>
//
// 朝の情報番組のライブカメラの BGM を、音源のファイルを使わずに、式だけで鳴らす (#1394)。
// - 曲: ハ長調、96 BPM、4 分の 4 拍子の 8 小節 (2.5 秒 × 8 = 20 秒)。テロップが入れ替わる 10 秒で、前半と後半に分ける。
// - 楽器: エレクトリックピアノ (FM 合成) の和音、ベース、チェレスタのような旋律、シェイカー。
// - 環境音: 山のふもとの朝らしく、ときどき小鳥がさえずる。
// - 響き: 簡単なリバーブ (くし形フィルターとオールパスフィルター) を薄く掛ける。
// 乱数は種を固定した擬似乱数を使い、同じ版の Node.js からは同じ WAV ができるようにする。
import { writeFile } from 'node:fs/promises';

const sampleRate = 48000;
const duration = 20;
const length = sampleRate * duration;
// beat は、1 拍の秒数 (96 BPM)。
const beat = 60 / 96;

const left = new Float64Array(length);
const right = new Float64Array(length);

// random は、種を固定した擬似乱数 (mulberry32) を返す。[0, 1) の数を返す。
function random(seed) {
  let state = seed >>> 0;

  return () => {
    state = (state + 0x6d2b79f5) >>> 0;
    let t = state;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);

    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

const humanize = random(1394);

// frequency は、MIDI のノート番号を、平均律の周波数 (Hz) にする。
function frequency(note) {
  return 440 * 2 ** ((note - 69) / 12);
}

// add は、start 秒から seconds 秒の音を、pan (-1 が左、1 が右) の位置に足す。voice は、音の頭からの秒を受け取り、標本を返す。
function add(start, seconds, pan, voice) {
  const from = Math.round(start * sampleRate);
  const to = Math.min(length, from + Math.round(seconds * sampleRate));
  // 定パワーで振り分ける。
  const angle = ((pan + 1) * Math.PI) / 4;
  const [gainLeft, gainRight] = [Math.cos(angle), Math.sin(angle)];
  for (let i = Math.max(0, from); i < to; i++) {
    const sample = voice((i - from) / sampleRate);
    left[i] += sample * gainLeft;
    right[i] += sample * gainRight;
  }
}

// release は、鍵を離した (held 秒を過ぎた) あとに、音を tail 秒ほどで消していく係数を返す。
function release(t, held, tail) {
  return t < held ? 1 : Math.exp(-(t - held) / tail);
}

// electricPiano は、エレクトリックピアノの音を足す。叩いた直後ほど倍音が多い FM 合成で、柔らかい鐘のような音にする。
function electricPiano(start, beats, note, velocity) {
  const f = frequency(note);
  const held = beats * beat;
  add(start, held + 1.2, -0.2, (t) => {
    const index = 1.1 * Math.exp(-t / 0.5) + 0.2;
    const envelope = Math.min(1, t / 0.004) * Math.exp(-t / 1.6) * release(t, held, 0.25);

    return (
      velocity * envelope * Math.sin(2 * Math.PI * f * t + index * Math.sin(2 * Math.PI * f * t))
    );
  });
}

// bass は、ベースの音を足す。小さなスピーカーでも聞こえるよう、2 倍音を混ぜる。
function bass(start, beats, note, velocity) {
  const f = frequency(note);
  const held = beats * beat;
  add(start, held + 0.4, 0, (t) => {
    const envelope = Math.min(1, t / 0.01) * Math.exp(-t / 1.2) * release(t, held, 0.08);

    return (
      velocity * envelope * (Math.sin(2 * Math.PI * f * t) + 0.35 * Math.sin(4 * Math.PI * f * t))
    );
  });
}

// celesta は、旋律の音を足す。倍音ごとに消える速さを変え、叩いたときだけ金属っぽく響かせる。
const celestaPartials = [
  { ratio: 1, amplitude: 1, decay: 1.4 },
  { ratio: 2, amplitude: 0.3, decay: 0.6 },
  { ratio: 3, amplitude: 0.1, decay: 0.3 },
  { ratio: 4.2, amplitude: 0.06, decay: 0.08 },
];
function celesta(start, beats, note, velocity) {
  const f = frequency(note);
  const held = beats * beat;
  add(start, held + 1.5, 0.25, (t) => {
    const envelope = Math.min(1, t / 0.003) * release(t, held, 0.5);
    let sample = 0;
    for (const { ratio, amplitude, decay } of celestaPartials) {
      sample += amplitude * Math.exp(-t / decay) * Math.sin(2 * Math.PI * f * ratio * t);
    }

    return velocity * envelope * sample;
  });
}

// shaker は、シェイカーの音を足す。白色雑音の差分 (高い音だけを通す) を、短く鳴らす。
const noise = random(832);
function shaker(start, velocity) {
  let previous = 0;
  add(start, 0.12, 0.35, (t) => {
    const current = noise() * 2 - 1;
    const sample = current - previous;
    previous = current;

    return velocity * Math.min(1, t / 0.006) * Math.exp(-t / 0.035) * sample;
  });
}

// chirp は、小鳥のさえずりの 1 声を足す。高い音から、さらに高い音へ素早く滑らせる。
function chirp(start, from, to, seconds, pan, velocity) {
  let phase = 0;
  add(start, seconds, pan, (t) => {
    const progress = t / seconds;
    phase += (2 * Math.PI * (from + (to - from) * progress * progress)) / sampleRate;

    return velocity * Math.sin(Math.PI * progress) ** 2 * Math.sin(phase);
  });
}

// chords は、小節ごとの和音。拍は小節の頭からの位置。前半は C、Am、F、G、後半は C、Em-Am、Dm-G7、C。
const chords = [
  [{ at: 0, beats: 4, notes: [60, 64, 67, 71], root: 36 }],
  [{ at: 0, beats: 4, notes: [57, 60, 64, 67], root: 45 }],
  [{ at: 0, beats: 4, notes: [53, 57, 60, 64], root: 41 }],
  [{ at: 0, beats: 4, notes: [55, 59, 62, 67], root: 43 }],
  [{ at: 0, beats: 4, notes: [60, 64, 67, 71], root: 36 }],
  [
    { at: 0, beats: 2, notes: [55, 59, 62, 64], root: 40 },
    { at: 2, beats: 2, notes: [57, 60, 64, 67], root: 45 },
  ],
  [
    { at: 0, beats: 2, notes: [53, 57, 60, 62], root: 38 },
    { at: 2, beats: 2, notes: [53, 55, 59, 62], root: 43 },
  ],
  [{ at: 0, beats: 4, notes: [60, 64, 67, 72], root: 36 }],
];

// melody は、旋律 (曲の頭からの拍、MIDI のノート番号、長さの拍)。ハ長調の五音音階を中心にする。
const melody = [
  [0, 76, 1],
  [1, 79, 0.5],
  [1.5, 81, 0.5],
  [2, 79, 1.5],
  [3.5, 76, 0.5],
  [4, 81, 1],
  [5, 79, 0.5],
  [5.5, 76, 0.5],
  [6, 74, 1.5],
  [7.5, 72, 0.5],
  [8, 72, 0.5],
  [8.5, 74, 0.5],
  [9, 76, 1],
  [10, 81, 1],
  [11, 79, 1],
  [12, 74, 1.5],
  [13.5, 76, 0.5],
  [14, 74, 2],
  [16, 76, 1],
  [17, 79, 0.5],
  [17.5, 81, 0.5],
  [18, 84, 1.5],
  [19.5, 81, 0.5],
  [20, 83, 1],
  [21, 79, 1],
  [22, 81, 1],
  [23, 76, 1],
  [24, 77, 1],
  [25, 76, 0.5],
  [25.5, 74, 0.5],
  [26, 74, 1],
  [27, 71, 1],
  [28, 72, 3],
];

// variation は、音の強さを少し揺らし、打ち込みらしい硬さを和らげる。
function variation(velocity) {
  return velocity * (0.9 + 0.2 * humanize());
}

chords.forEach((segments, bar) => {
  for (const { at, beats, notes, root } of segments) {
    const start = (bar * 4 + at) * beat;
    // 和音は、頭と、半拍ずらした裏で刻む。最後の小節は、伸ばして終える。
    const hits =
      bar === chords.length - 1
        ? [{ at: 0, beats: 4 }]
        : beats === 4
          ? [
              { at: 0, beats: 1.5 },
              { at: 1.5, beats: 1 },
              { at: 3, beats: 1 },
            ]
          : [
              { at: 0, beats: 1 },
              { at: 1, beats: 1 },
            ];
    for (const hit of hits) {
      for (const note of notes) {
        electricPiano(start + hit.at * beat, hit.beats, note, variation(0.045));
      }
    }
    const last = bar === chords.length - 1;
    bass(start, last ? 4 : 1.5, root, variation(0.16));
    if (beats === 4 && !last) {
      bass(start + 2 * beat, 1.5, root, variation(0.13));
    }
  }
});

for (const [at, note, beats] of melody) {
  celesta(at * beat, beats, note, variation(0.07));
}

// シェイカーは 8 分音符で刻み、裏拍を強くする。最後の小節では鳴らさない。
for (let step = 0; step < 7 * 8; step++) {
  shaker((step * beat) / 2, variation(step % 2 === 1 ? 0.035 : 0.018));
}

// 小鳥は、曲の邪魔をしないよう、旋律の休みのあたりで、左右に分けて鳴かせる。
const birds = random(1050);
for (const [start, pan] of [
  [2.2, -0.7],
  [8.4, 0.7],
  [13.1, -0.5],
  [18.6, 0.6],
]) {
  const calls = 2 + Math.floor(birds() * 2);
  for (let call = 0; call < calls; call++) {
    const from = 3400 + birds() * 600;
    chirp(start + call * 0.14, from, from + 1200 + birds() * 600, 0.07, pan, 0.012);
  }
}

// reverb は、Schroeder のリバーブ (並べたくし形フィルターと、直列のオールパスフィルター) を、片方のチャンネルに掛けた響きを返す。
// 左右で遅れを少しずらし、広がりを出す。
function reverb(input, spread) {
  const scale = sampleRate / 44100;
  const output = new Float64Array(length);
  for (const delay of [1557, 1617, 1491, 1422]) {
    const size = Math.round((delay + spread) * scale);
    const buffer = new Float64Array(size);
    let filtered = 0;
    for (let i = 0; i < length; i++) {
      const delayed = buffer[i % size];
      // 高い音ほど早く消えるよう、戻す音をなだらかにする。
      filtered = delayed * 0.75 + filtered * 0.25;
      buffer[i % size] = input[i] + filtered * 0.8;
      output[i] += delayed / 4;
    }
  }
  for (const delay of [556, 441]) {
    const size = Math.round((delay + spread) * scale);
    const buffer = new Float64Array(size);
    for (let i = 0; i < length; i++) {
      const delayed = buffer[i % size];
      const value = output[i] + delayed * 0.5;
      buffer[i % size] = value;
      output[i] = delayed - value * 0.5;
    }
  }

  return output;
}

// master は、全体の音量。放送の音量の基準 (ARIB TR-B32 の -24 LKFS) に合わせる。
// 足す音を変えたら、ffmpeg の ebur128 フィルターで測り直して合わせる。
const master = 0.74;

const wetLeft = reverb(left, 0);
const wetRight = reverb(right, 23);
for (let i = 0; i < length; i++) {
  left[i] = (left[i] + wetLeft[i] * 0.25) * master;
  right[i] = (right[i] + wetRight[i] * 0.25) * master;
}

// WAV (WAVE_FORMAT_IEEE_FLOAT) で書く。
const data = Buffer.alloc(length * 2 * 4);
for (let i = 0; i < length; i++) {
  data.writeFloatLE(left[i], i * 8);
  data.writeFloatLE(right[i], i * 8 + 4);
}
const header = Buffer.alloc(44);
header.write('RIFF', 0);
header.writeUInt32LE(36 + data.length, 4);
header.write('WAVE', 8);
header.write('fmt ', 12);
header.writeUInt32LE(16, 16);
header.writeUInt16LE(3, 20);
header.writeUInt16LE(2, 22);
header.writeUInt32LE(sampleRate, 24);
header.writeUInt32LE(sampleRate * 2 * 4, 28);
header.writeUInt16LE(2 * 4, 32);
header.writeUInt16LE(32, 34);
header.write('data', 36);
header.writeUInt32LE(data.length, 40);

const [, , output] = process.argv;
if (output === undefined) {
  console.error('usage: node scripts/demo-stream-music.mjs <output.wav>');
  process.exit(1);
}
await writeFile(output, Buffer.concat([header, data]));
