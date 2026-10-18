import { maxVolume } from './volume';

// 音量を 100% より上げる (#453)。video.volume は 1 までなので、超えた分を WebAudio の GainNode で上げる。
// WebAudio を通すと、iOS の Safari、別のウィンドウ (PiP)、バックグラウンドの再生で動きが変わるおそれがあるので、
// 初めて 100% を超えたときにだけ GainNode を挟む。一度挟んだら、その video の音は WebAudio を通り続ける。

// Booster は、video の音の WebAudio のグラフ。source → (音量の補正: compressor → makeup) → gain → 出力。
// 音量の補正 (#499) を切っているときは、source を直に gain へ繋ぐ。
type Booster = {
  source: MediaElementAudioSourceNode;
  compressor: DynamicsCompressorNode;
  makeup: GainNode;
  gain: GainNode;
  normalized: boolean;
};

// sharedContext は、すべての video で使う AudioContext。画質を変えたり選局したりするたびに video は作り直されるが、
// AudioContext は同時に作れる数に限りがあるので、1 つを使い回す。
let sharedContext: AudioContext | null = null;
const boosters = new WeakMap<HTMLMediaElement, Booster>();
// sources は、video ごとの MediaElementAudioSourceNode。1 つの video からは 1 度しか作れないので、外した後も持っておき、
// 同じ video でまた上げるときは繋ぎ直す。
const sources = new WeakMap<HTMLMediaElement, MediaElementAudioSourceNode>();
// pending は、まだ GainNode を挟めていない video の、上げたい倍率。ユーザーが操作するまで、AudioContext を動かせないため。
const pending = new WeakMap<HTMLMediaElement, number>();
// normalizing は、音量の補正を入れた video。まだ WebAudio を通せていなくても、通せたら補正する。
const normalizing = new WeakSet<HTMLMediaElement>();

// 音量の補正 (#499) のコンプレッサーの設定。大きな音を抑え、抑えた分を makeupGain で全体に足して、
// 番組と CM、局どうしの音量の差を縮める。音が割れないよう、比は控えめにする。
const compressorSettings = { threshold: -30, knee: 20, ratio: 4, attack: 0.01, release: 0.3 };
// makeupGain は、コンプレッサーで抑えた分を戻す倍率 (約 +4 dB)。
const makeupGain = 1.6;

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
    const compressor = sharedContext.createDynamicsCompressor();
    compressor.threshold.value = compressorSettings.threshold;
    compressor.knee.value = compressorSettings.knee;
    compressor.ratio.value = compressorSettings.ratio;
    compressor.attack.value = compressorSettings.attack;
    compressor.release.value = compressorSettings.release;
    const makeup = sharedContext.createGain();
    makeup.gain.value = makeupGain;
    compressor.connect(makeup).connect(gain);
    const source = sources.get(video) ?? sharedContext.createMediaElementSource(video);
    sources.set(video, source);
    gain.connect(sharedContext.destination);
    const booster = { source, compressor, makeup, gain, normalized: false };
    connectSource(booster, normalizing.has(video));
    boosters.set(video, booster);

    return booster;
  } catch {
    // 別の AudioContext に繋いだ video などで、挟めなければ、100% までで鳴らす。
    return null;
  }
}

// connectSource は、source を、音量の補正を通すか、直に gain へ繋ぐ。
function connectSource(booster: Booster, normalized: boolean): void {
  booster.source.disconnect();
  booster.source.connect(normalized ? booster.compressor : booster.gain);
  booster.normalized = normalized;
}

// setNormalize は、video の音量の補正 (#499) を入れるか切る。入れるときは、WebAudio を通す。
// ユーザーがまだ操作しておらず AudioContext を動かせなければ、覚えておき、操作した後 (resumePendingBoost) に入れる。
export function setNormalize(video: HTMLMediaElement, enabled: boolean): void {
  if (enabled) {
    normalizing.add(video);
  } else {
    normalizing.delete(video);
  }
  const booster = enabled || sources.has(video) ? boosterOf(video) : null;
  if (booster !== null && booster.normalized !== enabled) {
    connectSource(booster, enabled);
    resumeContext();
  }
}

// isNormalized は、video の音を、今、補正しているかを返す。
export function isNormalized(video: HTMLMediaElement): boolean {
  return boosters.get(video)?.normalized ?? false;
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
  if (normalizing.has(video) && !isNormalized(video)) {
    setNormalize(video, true);
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
    booster.compressor.disconnect();
    booster.makeup.disconnect();
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
