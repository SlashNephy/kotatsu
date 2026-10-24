// 5.1ch の音声を、ヘッドホン向けに仮想サラウンドにする (#454、設計書 7.5)。
// 5 つのチャンネルを、聞く人のまわりの決まった方向に、頭部伝達関数 (HRTF) の PannerNode で置き、2ch に混ぜる。
// LFE は、方向を持たないので、そのまま両耳に足す。外部のデータは使わず、ブラウザの HRTF だけで作る。

// speakers は、5.1ch の並び (AAC の L, R, C, LFE, Ls, Rs の順ではなく、WebAudio が 5.1 を分けたときの
// L, R, C, LFE, SL, SR の順) ごとの、置く方向 (度。正面が 0、右が正) と、混ぜるときの倍率。LFE は方向を持たない。
const speakers: readonly { azimuth: number | null; gain: number }[] = [
  { azimuth: -30, gain: 1 },
  { azimuth: 30, gain: 1 },
  { azimuth: 0, gain: Math.SQRT1_2 },
  { azimuth: null, gain: 0.5 },
  { azimuth: -110, gain: Math.SQRT1_2 },
  { azimuth: 110, gain: Math.SQRT1_2 },
];

// position は、方向 (度) を、聞く人から 1 だけ離れた点にする。WebAudio は、右が +x、前が -z。
export function position(azimuth: number): { x: number; z: number } {
  const radians = (azimuth * Math.PI) / 180;

  return { x: Math.sin(radians), z: -Math.cos(radians) };
}

// VirtualSurround は、5.1ch を入れ、2ch を出すノードの組。
export type VirtualSurround = { input: AudioNode; output: AudioNode; nodes: AudioNode[] };

// createVirtualSurround は、ctx の上に、仮想サラウンドのノードを作る。
export function createVirtualSurround(ctx: BaseAudioContext): VirtualSurround {
  const splitter = ctx.createChannelSplitter(6);
  // 6ch のまま分けるよう、チャンネルを増減させない。
  splitter.channelCount = 6;
  splitter.channelCountMode = 'explicit';
  splitter.channelInterpretation = 'discrete';
  const output = ctx.createGain();
  output.channelCount = 2;
  output.channelCountMode = 'explicit';
  const nodes: AudioNode[] = [splitter, output];
  speakers.forEach(({ azimuth, gain }, channel) => {
    const level = ctx.createGain();
    level.gain.value = gain;
    nodes.push(level);
    splitter.connect(level, channel);
    if (azimuth === null) {
      level.connect(output);

      return;
    }
    const panner = new PannerNode(ctx, {
      panningModel: 'HRTF',
      distanceModel: 'linear',
      refDistance: 1,
    });
    const { x, z } = position(azimuth);
    panner.positionX.value = x;
    panner.positionY.value = 0;
    panner.positionZ.value = z;
    nodes.push(panner);
    level.connect(panner).connect(output);
  });

  return { input: splitter, output, nodes };
}
