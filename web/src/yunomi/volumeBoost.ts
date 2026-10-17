import { maxVolume } from './volume';

// 音量を 100% より上げる (#453)。video.volume は 1 までなので、超えた分を WebAudio の GainNode で上げる。
// WebAudio を通すと、iOS の Safari、別のウィンドウ (PiP)、バックグラウンドの再生で動きが変わるおそれがあるので、
// 初めて 100% を超えたときにだけ GainNode を挟む。一度挟んだら、その video の音は WebAudio を通り続ける。

type Booster = { context: AudioContext; gain: GainNode };

const boosters = new WeakMap<HTMLMediaElement, Booster>();
// pending は、まだ GainNode を挟めていない video の、上げたい倍率。ユーザーが操作するまで、AudioContext を動かせないため。
const pending = new WeakMap<HTMLMediaElement, number>();

// splitVolume は、0 から maxVolume の音量を、video.volume (0〜1) と GainNode の倍率 (1 以上) に分ける。
export function splitVolume(volume: number): { element: number; gain: number } {
  const clamped = Math.min(Math.max(volume, 0), maxVolume);

  return { element: Math.min(clamped, 1), gain: Math.max(clamped, 1) };
}

// boostOf は、video の音を上げている倍率を返す。上げていなければ 1 を返す。
export function boostOf(video: HTMLMediaElement): number {
  return boosters.get(video)?.gain.gain.value ?? pending.get(video) ?? 1;
}

// currentVolume は、video の今の音量 (0 から maxVolume) を返す。
export function currentVolume(video: HTMLMediaElement): number {
  return video.volume < 1 ? video.volume : video.volume * boostOf(video);
}

// canStartAudio は、いま AudioContext を動かせるか。ユーザーが一度も操作していないと、AudioContext は止まったままで、
// WebAudio を通した video の音が出なくなるので、そのときは GainNode を挟まない。
function canStartAudio(): boolean {
  return typeof AudioContext !== 'undefined' && (navigator.userActivation?.hasBeenActive ?? true);
}

function boosterOf(video: HTMLMediaElement): Booster | null {
  const existing = boosters.get(video);
  if (existing !== undefined) {
    return existing;
  }
  if (!canStartAudio()) {
    return null;
  }
  try {
    const context = new AudioContext();
    const gain = context.createGain();
    context.createMediaElementSource(video).connect(gain).connect(context.destination);
    const booster = { context, gain };
    boosters.set(video, booster);

    return booster;
  } catch {
    // 別の AudioContext に繋いだ video などで、挟めなければ、100% までで鳴らす。
    return null;
  }
}

// applyVolume は、video に音量 (0 から maxVolume) を当てる。100% を超える分は GainNode で上げる。
// まだ GainNode を挟めないときは、100% で鳴らし、ユーザーが操作したら挟む (resumePendingBoost)。
export function applyVolume(video: HTMLMediaElement, volume: number): void {
  const { element, gain } = splitVolume(volume);
  const booster = gain > 1 || boosters.has(video) ? boosterOf(video) : null;
  if (booster === null) {
    if (gain > 1) {
      pending.set(video, gain);
    } else {
      pending.delete(video);
    }
  } else {
    pending.delete(video);
    booster.gain.gain.value = gain;
    if (booster.context.state === 'suspended') {
      void booster.context.resume().catch(() => {});
    }
  }
  video.volume = element;
}

// resumePendingBoost は、ユーザーが操作した後に、保留していた倍率で GainNode を挟む。
export function resumePendingBoost(video: HTMLMediaElement): void {
  const gain = pending.get(video);
  if (gain !== undefined) {
    applyVolume(video, video.volume * gain);
  }
}

// lowerBoost は、ほかの経路 (OS のメディア操作など) で video.volume を 1 より下げたときに、上げていた倍率を戻す。
export function lowerBoost(video: HTMLMediaElement): void {
  if (video.volume < 1) {
    pending.delete(video);
    const booster = boosters.get(video);
    if (booster !== undefined) {
      booster.gain.gain.value = 1;
    }
  }
}
