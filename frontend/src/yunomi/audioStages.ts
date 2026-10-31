// audioStages は、video の音の WebAudio のグラフに並べる段を決める (#812、設計書 7.5)。
// 段は、名前の付いた順に並べ、入れていない段は飛ばす。イコライザーや音ずれ補正を足すときは、ここに名前と順を足す。

// audioStageOrder は、段を並べる順。source に近いほうから並べる。
// 音ずれ補正 (#818) は、ほかの段の効き方を変えないので最初に置く。仮想サラウンドは、元のチャンネルを読むので
// その次に置き、音量の補正は、混ぜた後の音に効かせる。
export const audioStageOrder = ['delay', 'surround', 'normalize'] as const;

export type AudioStageName = (typeof audioStageOrder)[number];

// AudioStage は、段の入り口と出口と、段に含むノード。nodes は、外すときに繋がりを切るのに使う。
export type AudioStage = { input: AudioNode; output: AudioNode; nodes: AudioNode[] };

// audioStageChain は、入れている段 enabled を、並べる順にして返す。入れた順にはよらない。
export function audioStageChain(enabled: ReadonlySet<AudioStageName>): AudioStageName[] {
  return audioStageOrder.filter((name) => enabled.has(name));
}
