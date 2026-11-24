import { audioStageChain, type AudioStage, type AudioStageName } from './audioStages';
import type { PlayerMedia } from './playerMedia';
import { existingAudioContext } from './sharedAudioContext';
import { maxVolume } from './volume';
import { createVirtualSurround } from './virtualSurround';

// 音量を 100% より上げる (#453)。media.volume は 1 までなので、超えた分を WebAudio の GainNode で上げる。
// WebAudio を通すと、iOS の Safari、別のウィンドウ (PiP)、バックグラウンドの再生で動きが変わるおそれがあるので、
// 初めて 100% を超えたときにだけ GainNode を挟む。一度挟んだら、その media の音は WebAudio を通り続ける。

// Booster は、media の音の WebAudio のグラフ。
// source → 入れているステップ (audioStageOrder の順) → gain → sink。切っているステップは、飛ばして繋ぐ (#812)。
// source と sink は、media の音の口 (audioPort) から受け取る。
type Booster = {
  context: BaseAudioContext;
  source: AudioNode;
  gain: GainNode;
  // stages は、作ったステップ。初めて入れたときに作り、切っても残しておく。
  stages: Map<AudioStageName, AudioStage>;
  // enabled は、入れているステップ。
  enabled: Set<AudioStageName>;
};

const boosters = new WeakMap<PlayerMedia, Booster>();
// routed は、音の口を受け取って、音が WebAudio を通るようになった media。音の口は media が保持しているので、外した後に
// 同じ media でまた上げるときは、同じ口に繋ぎ直す。
const routed = new WeakSet<PlayerMedia>();
// pending は、まだ GainNode を挟めていない media の、上げたい倍率。ユーザーが操作するまで、AudioContext を動かせないため。
const pending = new WeakMap<PlayerMedia, number>();
// wanted は、media ごとの、入れたステップ。まだ WebAudio を通せていなくても、通せたら入れる。
const wanted = new WeakMap<PlayerMedia, Set<AudioStageName>>();

// 音量の補正 (#499) のコンプレッサーの設定。大きな音を抑え、抑えた分を makeupGain で全体に足して、
// 番組と CM、局どうしの音量の差を縮める。音が割れないよう、比は控えめにする。
const compressorSettings = { threshold: -30, knee: 20, ratio: 4, attack: 0.01, release: 0.3 };
// makeupGain は、コンプレッサーで抑えた分を戻す倍率 (約 +4 dB)。
const makeupGain = 1.6;

// createNormalize は、音量の補正のステップ (compressor → makeup) を作る。
function createNormalize(ctx: BaseAudioContext): AudioStage {
  const compressor = ctx.createDynamicsCompressor();
  compressor.threshold.value = compressorSettings.threshold;
  compressor.knee.value = compressorSettings.knee;
  compressor.ratio.value = compressorSettings.ratio;
  compressor.attack.value = compressorSettings.attack;
  compressor.release.value = compressorSettings.release;
  const makeup = ctx.createGain();
  makeup.gain.value = makeupGain;
  compressor.connect(makeup);

  return { input: compressor, output: makeup, nodes: [compressor, makeup] };
}

// maxDelaySeconds は、音ずれ補正 (#818) で遅らせられる上限 (秒)。DelayNode は、作るときに上限を決める。
const maxDelaySeconds = 1;

// createDelay は、音ずれ補正のステップを作る。遅らせる長さは、setStage が setAudioDelay の値にする。
function createDelay(ctx: BaseAudioContext): AudioStage {
  const delay = ctx.createDelay(maxDelaySeconds);

  return { input: delay, output: delay, nodes: [delay] };
}

// stageFactories は、ステップごとの作り方。ステップを足すときは、audioStageOrder とここに足す。
const stageFactories: Record<AudioStageName, (ctx: BaseAudioContext) => AudioStage> = {
  delay: createDelay,
  surround: createVirtualSurround,
  normalize: createNormalize,
};

// delays は、media ごとの、遅らせる長さ (秒)。ステップをまだ作れていなくても、作ったらこの長さにする。
const delays = new WeakMap<PlayerMedia, number>();

// wantedOf は、media に入れたステップを返す。
function wantedOf(media: PlayerMedia): Set<AudioStageName> {
  let stages = wanted.get(media);
  if (stages === undefined) {
    stages = new Set();
    wanted.set(media, stages);
  }

  return stages;
}

// splitVolume は、0 から maxVolume の音量を、media.volume (0〜1) と GainNode の倍率 (1 以上) に分ける。
export function splitVolume(volume: number): { element: number; gain: number } {
  const clamped = Math.min(Math.max(volume, 0), maxVolume);

  return { element: Math.min(clamped, 1), gain: Math.max(clamped, 1) };
}

// boostOf は、media の音を上げている倍率を返す。上げていなければ 1 を返す。
export function boostOf(media: PlayerMedia): number {
  return boosters.get(media)?.gain.gain.value ?? pending.get(media) ?? 1;
}

// currentVolume は、media の今の音量 (0 から maxVolume) を返す。
export function currentVolume(media: PlayerMedia): number {
  return media.volume < 1 ? media.volume : media.volume * boostOf(media);
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
// WebAudio を通した media の音が出なくなるので、そのときは GainNode を挟まない。
function canStartAudio(): boolean {
  return (
    typeof AudioContext !== 'undefined' && (navigator.userActivation?.hasBeenActive ?? activated)
  );
}

// resumeContext は、止まっている AudioContext を動かす。ユーザーの操作の後に呼ぶ。
function resumeContext(): void {
  const context = existingAudioContext();
  if (context?.state === 'suspended') {
    void context.resume().catch(() => {});
  }
}

function boosterOf(media: PlayerMedia): Booster | null {
  const existing = boosters.get(media);
  if (existing !== undefined) {
    return existing;
  }
  if (!canStartAudio()) {
    return null;
  }
  try {
    const port = media.audioPort();
    routed.add(media);
    const { context } = port;
    const gain = context.createGain();
    gain.connect(port.sink);
    const booster: Booster = {
      context,
      source: port.source,
      gain,
      stages: new Map(),
      enabled: new Set(wanted.get(media)),
    };
    wire(context, booster);
    applyDelay(media, booster);
    boosters.set(media, booster);

    return booster;
  } catch {
    // 別の AudioContext に繋いだ media などで、挟めなければ、100% までで鳴らす。
    return null;
  }
}

// wire は、booster の入れているステップに合わせて、source から gain までを繋ぎ直す。ステップの中の繋がりは、そのまま残す。
function wire(ctx: BaseAudioContext, booster: Booster): void {
  booster.source.disconnect();
  for (const stage of booster.stages.values()) {
    stage.output.disconnect();
  }
  let previous: AudioNode = booster.source;
  for (const name of audioStageChain(booster.enabled)) {
    let stage = booster.stages.get(name);
    if (stage === undefined) {
      stage = stageFactories[name](ctx);
      booster.stages.set(name, stage);
    }
    previous.connect(stage.input);
    previous = stage.output;
  }
  previous.connect(booster.gain);
}

// setStage は、media の音のステップ name を入れるか切る。入れるときは、WebAudio を通す。
// ユーザーがまだ操作しておらず AudioContext を動かせなければ、保持しておき、操作した後 (resumePendingBoost) に入れる。
function setStage(media: PlayerMedia, name: AudioStageName, enabled: boolean): void {
  if (enabled) {
    wantedOf(media).add(name);
  } else {
    wanted.get(media)?.delete(name);
  }
  const booster = enabled || routed.has(media) ? boosterOf(media) : null;
  if (booster === null) {
    return;
  }
  if (booster.enabled.has(name) !== enabled) {
    if (enabled) {
      booster.enabled.add(name);
    } else {
      booster.enabled.delete(name);
    }
    wire(booster.context, booster);
  }
  applyDelay(media, booster);
  // 作ったばかりの AudioContext は止まっていることがあるので、動かす。動かさないと、WebAudio を通した音が出ない。
  resumeContext();
}

// applyDelay は、音ずれ補正のステップに、保持した長さを当てる。ステップを作ったとき (setStage でも、音量を上げて booster を
// 作ったときでも) と、長さだけを変えたときに呼ぶ。
function applyDelay(media: PlayerMedia, booster: Booster): void {
  const delay = booster.stages.get('delay')?.input as DelayNode | undefined;
  if (delay !== undefined) {
    delay.delayTime.value = delays.get(media) ?? 0;
  }
}

// hasStage は、media の音に、今、ステップ name を入れているかを返す。
function hasStage(media: PlayerMedia, name: AudioStageName): boolean {
  return boosters.get(media)?.enabled.has(name) ?? false;
}

// setSurround は、media の仮想サラウンド (#454) を入れるか切る。
export function setSurround(media: PlayerMedia, enabled: boolean): void {
  setStage(media, 'surround', enabled);
}

// isSurrounded は、media の音を、今、仮想サラウンドにしているかを返す。
export function isSurrounded(media: PlayerMedia): boolean {
  return hasStage(media, 'surround');
}

// setAudioDelay は、media の音を seconds 秒遅らせる (#818)。0 なら、音ずれ補正のステップを外す。
export function setAudioDelay(media: PlayerMedia, seconds: number): void {
  const clamped = Math.min(Math.max(seconds, 0), maxDelaySeconds);
  delays.set(media, clamped);
  setStage(media, 'delay', clamped > 0);
}

// setNormalize は、media の音量の補正 (#499) を入れるか切る。
export function setNormalize(media: PlayerMedia, enabled: boolean): void {
  setStage(media, 'normalize', enabled);
}

// isNormalized は、media の音を、今、補正しているかを返す。
export function isNormalized(media: PlayerMedia): boolean {
  return hasStage(media, 'normalize');
}

// applyVolume は、media に音量 (0 から maxVolume) を当てる。100% を超える分は GainNode で上げる。
// まだ GainNode を挟めないときは、100% で鳴らし、ユーザーが操作したら挟む (resumePendingBoost)。
export function applyVolume(media: PlayerMedia, volume: number): void {
  const { element, gain } = splitVolume(volume);
  // 一度 WebAudio を通した media は、AudioContext に繋がないと音が出ないので、100% 以下でも繋ぐ。
  const booster = gain > 1 || routed.has(media) ? boosterOf(media) : null;
  if (booster === null) {
    if (gain > 1) {
      pending.set(media, gain);
    } else {
      pending.delete(media);
    }
  } else {
    pending.delete(media);
    booster.gain.gain.value = gain;
    resumeContext();
  }
  media.volume = element;
}

// resumePendingBoost は、ユーザーが操作した後に、保留していた倍率で GainNode を挟み、止まっている AudioContext を動かす。
// 保留するのは 100% を超えるときだけで、そのとき media.volume は 1 なので、倍率がそのまま音量になる。
export function resumePendingBoost(media: PlayerMedia): void {
  const gain = pending.get(media);
  if (gain !== undefined) {
    applyVolume(media, gain);
  }
  for (const name of wanted.get(media) ?? []) {
    if (!hasStage(media, name)) {
      setStage(media, name, true);
    }
  }
  resumeContext();
}

// releaseBoost は、作り直されて使わなくなった media を、AudioContext から外す。
// 同じ media でまた上げるときは、boosterOf が繋ぎ直す。
export function releaseBoost(media: PlayerMedia): void {
  pending.delete(media);
  const booster = boosters.get(media);
  if (booster !== undefined) {
    booster.source.disconnect();
    for (const stage of booster.stages.values()) {
      for (const node of stage.nodes) {
        node.disconnect();
      }
    }
    booster.gain.disconnect();
    boosters.delete(media);
  }
}

// lowerBoost は、ほかの経路 (OS のメディア操作など) で media.volume を 1 より下げたときに、上げていた倍率を戻す。
export function lowerBoost(media: PlayerMedia): void {
  if (media.volume < 1) {
    pending.delete(media);
    const booster = boosters.get(media);
    if (booster !== undefined) {
      booster.gain.gain.value = 1;
    }
  }
}
