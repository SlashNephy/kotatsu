// ブラウザ内でデコードする経路 (#1516) で、鳴らす音声を選ぶ部分 (#2165)。
// 番組の音声の ES から鳴らすものを選び、デュアルモノの ES は、主音声、副音声、主と副の両方のどれかに振り分ける。

// DualMonoChannel は、デュアルモノの ES のうち鳴らすもの。both は、主音声を左、副音声を右に鳴らす。
export type DualMonoChannel = 'main' | 'sub' | 'both';

// chooseAudioPid は、番組の音声の ES (PMT の順) から、鳴らす ES の PID を返す。
// 選んだ ES が番組にあればそれを、なければ (番組が変わって構成が変わったときなど) 最初の ES を鳴らす。
export function chooseAudioPid(
  pids: ReadonlyArray<number>,
  preferred: number | null,
): number | null {
  if (preferred !== null && pids.includes(preferred)) {
    return preferred;
  }

  return pids[0] ?? null;
}

// routeDualMono は、デュアルモノの ES をデコードした 2ch (左が主音声、右が副音声) を、channel に振り分けた左右を返す。
// 主音声か副音声だけを鳴らすときは、その音を左右の両方から鳴らす。
export function routeDualMono(
  left: Float32Array,
  right: Float32Array,
  channel: DualMonoChannel,
): { left: Float32Array; right: Float32Array } {
  switch (channel) {
    case 'main':
      return { left, right: left.slice() };
    case 'sub':
      return { left: right.slice(), right };
    case 'both':
      return { left, right };
  }
}

// adtsPceElementId は、raw_data_block の要素のうち、program_config_element の id_syn_ele (ISO/IEC 14496-3)。
const adtsPceElementId = 5;

// startsWithPce は、ADTS のフレームの最初の要素が PCE かどうかを返す。
// デュアルモノの ES は channel_configuration が 0 で、チャンネルの構成を PCE で送る。PCE は数十フレームに 1 つしかなく、
// デコーダは PCE を読むまで、そのフレームをデコードできない。
export function startsWithPce(frame: Uint8Array): boolean {
  if (frame.length < 7) {
    return false;
  }
  // protection_absent が 0 なら、ヘッダーの後に 2 バイトの CRC が付く。
  const headerLength = (frame[1]! & 0x01) === 1 ? 7 : 9;
  const first = frame[headerLength];

  return first !== undefined && first >> 5 === adtsPceElementId;
}
