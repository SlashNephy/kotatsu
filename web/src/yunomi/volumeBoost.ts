import { maxVolume } from './volume';

// 音量を 100% より上げる (#453)。video.volume は 1 までなので、超えた分を WebAudio の GainNode で上げる。
// WebAudio を通すと、iOS の Safari、別のウィンドウ (PiP)、バックグラウンドの再生で動きが変わるおそれがあるので、
// 初めて 100% を超えたときにだけ GainNode を挟む。一度挟んだら、その video の音は WebAudio を通り続ける。

type Booster = { source: MediaElementAudioSourceNode; gain: GainNode };

// sharedContext は、すべての video で使う AudioContext。画質を変えたり選局したりするたびに video は作り直されるが、
// AudioContext は同時に作れる数に限りがあるので、1 つを使い回す。
let sharedContext: AudioContext | null = null;
const boosters = new WeakMap<HTMLMediaElement, Booster>();
// sources は、video ごとの MediaElementAudioSourceNode。1 つの video からは 1 度しか作れないので、外した後も持っておき、
// 同じ video でまた上げるときは繋ぎ直す。
const sources = new WeakMap<HTMLMediaElement, MediaElementAudioSourceNode>();
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

// activated は、ユーザーがページを操作したか。navigator.userActivation のないブラウザのために、自分でも数える。
let activated = false;
if (typeof window !== 'undefined') {
  const onActivate = () => {
    activated = true;
    window.removeEventListener('pointerdown', onActivate, true);
    window.removeEventListener('keydown', onActivate, true);
  };
  window.addEventListener('pointerdown', onActivate, true);
  window.addEventListener('keydown', onActivate, true);
}

// canStartAudio は、いま AudioContext を動かせるか。ユーザーが一度も操作していないと、AudioContext は止まったままで、
// WebAudio を通した video の音が出なくなるので、そのときは GainNode を挟まない。
function canStartAudio(): boolean {
  return (
    typeof AudioContext !== 'undefined' && (navigator.userActivation?.hasBeenActive ?? activated)
  );
}

// resumeContext は、止まっている AudioContext を動かす。ユーザーの操作の後に呼ぶ。
function resumeContext(): void {
  if (sharedContext?.state === 'suspended') {
    void sharedContext.resume().catch(() => {});
  }
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
    sharedContext ??= new AudioContext();
    const gain = sharedContext.createGain();
    const source = sources.get(video) ?? sharedContext.createMediaElementSource(video);
    sources.set(video, source);
    source.connect(gain).connect(sharedContext.destination);
    const booster = { source, gain };
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
  // 一度 WebAudio を通した video は、AudioContext に繋がないと音が出ないので、100% 以下でも繋ぐ。
  const booster = gain > 1 || sources.has(video) ? boosterOf(video) : null;
  if (booster === null) {
    if (gain > 1) {
      pending.set(video, gain);
    } else {
      pending.delete(video);
    }
  } else {
    pending.delete(video);
    booster.gain.gain.value = gain;
    resumeContext();
  }
  video.volume = element;
}

// resumePendingBoost は、ユーザーが操作した後に、保留していた倍率で GainNode を挟み、止まっている AudioContext を動かす。
// 保留するのは 100% を超えるときだけで、そのとき video.volume は 1 なので、倍率がそのまま音量になる。
export function resumePendingBoost(video: HTMLMediaElement): void {
  const gain = pending.get(video);
  if (gain !== undefined) {
    applyVolume(video, gain);
  }
  resumeContext();
}

// releaseBoost は、作り直されて使わなくなった video を、AudioContext から外す。
// 同じ video でまた上げるときは、boosterOf が繋ぎ直す。
export function releaseBoost(video: HTMLMediaElement): void {
  pending.delete(video);
  const booster = boosters.get(video);
  if (booster !== undefined) {
    booster.source.disconnect();
    booster.gain.disconnect();
    boosters.delete(video);
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
