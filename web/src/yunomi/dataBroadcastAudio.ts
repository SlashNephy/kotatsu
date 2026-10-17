import type { AudioNodeProvider } from 'web-bml';

// DataBroadcastAudio は、データ放送の効果音の出口 (#439)。web-bml は、鳴らすたびに getAudioDestinationNode() で出口を引く。
export type DataBroadcastAudio = AudioNodeProvider & {
  // resume は、ブラウザの自動再生の制限で止まっている AudioContext を動かす。ユーザーの操作の中で呼ぶ。
  resume: () => void;
  close: () => void;
};

// broadcastGain は、プレイヤーの音量とミュートから、効果音の音量 (0〜1) を決める。
export function broadcastGain(video: Pick<HTMLVideoElement, 'volume' | 'muted'> | null): number {
  if (video === null || video.muted) {
    return 0;
  }

  return Math.min(Math.max(video.volume, 0), 1);
}

// createDataBroadcastAudio は、効果音をプレイヤーの音量とミュートに合わせて鳴らす出口を作る。
// 効果音は短いので、鳴らすたびに、その時の音量とミュートを GainNode に写す。
// Web Audio API のない環境では null を返し、web-bml の既定の出口に任せる。
export function createDataBroadcastAudio(
  video: () => HTMLVideoElement | null,
): DataBroadcastAudio | null {
  if (typeof AudioContext === 'undefined') {
    return null;
  }
  const context = new AudioContext();
  const gain = context.createGain();
  gain.connect(context.destination);

  return {
    getAudioDestinationNode: () => {
      gain.gain.value = broadcastGain(video());

      return gain;
    },
    resume: () => {
      if (context.state === 'suspended') {
        void context.resume().catch(() => {});
      }
    },
    close: () => {
      void context.close().catch(() => {});
    },
  };
}
