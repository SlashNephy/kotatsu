import { ptsTicksPerSecond } from './ptsTimeline';

// ブラウザ内でデコードする経路 (#1516) の、AAC の音声の PES を、ADTS のフレームに分ける部分。
// 1 つの PES には、ADTS のフレームがいくつか入り、PES の境目でフレームが切れることもある。
// PES の PTS は、その PES で始まる最初のフレームの時刻なので、続くフレームの時刻は、フレームの長さを足して求める。
// ヘッダーの中身 (プロファイルなど) の解釈はデコーダに任せ、ここでは区切りと時刻だけを付ける。

// AdtsFrame は、ADTS のフレーム 1 つ。data はヘッダーを含む。
export type AdtsFrame = {
  // pts は、伸ばした PTS (ptsTimeline.ts)。
  pts: number;
  // duration は、フレームの長さ (PTS の単位)。
  duration: number;
  sampleRate: number;
  // channels は、channel_configuration。0 は、構成をデコーダが読む (PCE)。
  channels: number;
  data: Uint8Array;
};

// adtsSampleRates は、sampling_frequency_index から周波数を引く表 (ISO/IEC 14496-3)。
const adtsSampleRates = [
  96_000, 88_200, 64_000, 48_000, 44_100, 32_000, 24_000, 22_050, 16_000, 12_000, 11_025, 8_000,
  7_350,
];

// samplesPerRawBlock は、AAC の 1 ブロックのサンプル数。
const samplesPerRawBlock = 1024;

// AdtsSplitter は、音声の PES を、ADTS のフレームに分ける。PES の境目で切れたフレームは、次の PES とつなぐ。
export class AdtsSplitter {
  private carry = new Uint8Array(0);
  // next は、carry の先頭のフレームの時刻。PTS のない PES が続くときも、前のフレームから数える。
  private next: number | null = null;

  // push は、PES のペイロードと、その PTS (伸ばしたもの、なければ null) を受け取り、組み上がったフレームを返す。
  push(payload: Uint8Array, pts: number | null): AdtsFrame[] {
    const frames: AdtsFrame[] = [];
    let data = payload;
    let time: number | null;
    if (this.carry.length > 0) {
      data = new Uint8Array(this.carry.length + payload.length);
      data.set(this.carry);
      data.set(payload, this.carry.length);
      // 切れたフレームの続きの後で、この PES の PTS のフレームが始まる。続きの分は、前の時刻から数える。
      time = this.next;
    } else {
      time = pts ?? this.next;
    }
    let offset = 0;
    let pesStart: number | null = this.carry.length > 0 ? null : 0;
    const resumeAt = this.carry.length;
    while (data.length - offset >= 7) {
      if (data[offset] !== 0xff || (data[offset + 1]! & 0xf6) !== 0xf0) {
        // 同期がずれたら、次の同期語を探す。
        offset += 1;
        continue;
      }
      const length =
        ((data[offset + 3]! & 0x03) << 11) | (data[offset + 4]! << 3) | (data[offset + 5]! >> 5);
      const rateIndex = (data[offset + 2]! >> 2) & 0x0f;
      const sampleRate = adtsSampleRates[rateIndex];
      if (length < 7 || sampleRate === undefined) {
        offset += 1;
        continue;
      }
      if (data.length - offset < length) {
        break;
      }
      // この PES の中で始まる最初のフレームには、この PES の PTS を当てる。
      if (pesStart === null && offset >= resumeAt && pts !== null) {
        time = pts;
        pesStart = offset;
      }
      const blocks = (data[offset + 6]! & 0x03) + 1;
      const duration = (samplesPerRawBlock * blocks * ptsTicksPerSecond) / sampleRate;
      const channels = ((data[offset + 2]! & 0x01) << 2) | (data[offset + 3]! >> 6);
      if (time !== null) {
        frames.push({
          pts: time,
          duration,
          sampleRate,
          channels,
          data: data.slice(offset, offset + length),
        });
        time += duration;
      }
      offset += length;
    }
    this.carry = data.slice(offset);
    this.next = time;

    return frames;
  }

  // reset は、つなぎ待ちのフレームと時刻を忘れる。選局したときと、飛んだときに呼ぶ。
  reset(): void {
    this.carry = new Uint8Array(0);
    this.next = null;
  }
}
