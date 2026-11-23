// sharedContext は、プレイヤーの音を通す AudioContext。画質を変えたり選局したりするたびに再生の要素は作り直されるが、
// AudioContext は同時に作れる数に限りがあるので、1 つを使い回す。
// 音量を 100% より上げる段 (volumeBoost) と、再生の経路 (PlayerMedia の audioPort) は、同じ AudioContext を使う。
// 別の AudioContext のノードどうしは繋げないため。
let sharedContext: AudioContext | null = null;

// sharedAudioContext は、プレイヤーの音を通す AudioContext を返す。初めて呼んだときに作る。
// ユーザーがページを操作する前に作ると、止まった (suspended) まま作られる。動かすのは呼んだ側の役目。
export function sharedAudioContext(): AudioContext {
  sharedContext ??= new AudioContext();

  return sharedContext;
}

// existingAudioContext は、もう作った AudioContext を返す。まだ作っていなければ null を返し、作らない。
export function existingAudioContext(): AudioContext | null {
  return sharedContext;
}
