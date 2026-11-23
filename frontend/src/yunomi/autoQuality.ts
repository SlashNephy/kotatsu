// 画質の「自動」で、デコードが追いつかない端末の段を下げる (#1513、設計書 7.1)。
// 段の表は、上から、オリジナル (ブラウザの中で変換) → kyusu の 1080p → 720p → 540p → 360p。

import { audioOnlyBitrate, audioStep } from './networkQuality';
import { decoded, original, type PlaybackRoute, type Preset } from './route';

// autoQuality は、画質のメニューの「自動」を表す名前。覚えた画質がこれか、覚えていなければ、自動で上げ下げする。
export const autoQuality = 'auto';

// decodeStepFromOriginal は、オリジナルの段から「デコードが追いつかない」で下げるときの移り先の高さ。
// 1080i の H.264 のソフトウェアデコードが重いのが原因なので、kyusu の 1080p では軽くならない見込み (推測)。
export const decodeStepFromOriginal = 720;

// originalHeight は、段の表でのオリジナルの高さ。kyusu の 1080p より上に置く。
const originalHeight = 1081;

// ladderHeight は、段の表に載る画質の高さを返す。載らない画質 (HEVC、60p、映像を変換しないもの) は null。
function ladderHeight(name: string): number | null {
  if (name === original) {
    return originalHeight;
  }
  const match = /^(\d+)p$/.exec(name);

  return match === null ? null : Number(match[1]);
}

// qualityHeight は、画質の高さを返す。段の表に載らない画質も、名前の先頭の高さで比べる。分からなければ null。
// 映像を変換しない画質 (source) は BS4K の 2160p。音声だけの画質は audioOnlyHeight。
export function qualityHeight(name: string): number | null {
  if (name === 'source') {
    return 2160;
  }
  if (name === audioStep) {
    return audioOnlyHeight;
  }
  const ladder = ladderHeight(name);
  if (ladder !== null) {
    return ladder;
  }
  const match = /^(\d+)p/.exec(name);

  return match === null ? null : Number(match[1]);
}

// Rung は、段の表の 1 段。bitrate は、映像と音声を足したビットレート (bps)。オリジナルと、プリセットのビットレートが
// 分からない段は null。
export type Rung = { name: string; height: number; bitrate: number | null };

// ladder は、選べる画質から段の表を作る。高い順に並べる。kyusu の段は H.264 のものだけを使う。
export function ladder(choices: readonly string[], presets: readonly Preset[]): Rung[] {
  const rungs: Rung[] = [];
  for (const name of choices) {
    const height = ladderHeight(name);
    if (height === null) {
      continue;
    }
    if (name === original) {
      rungs.push({ name, height, bitrate: null });
      continue;
    }
    const preset = presets.find((candidate) => candidate.name === name);
    if (preset === undefined || preset.codec !== 'H264' || preset.passthrough) {
      continue;
    }
    // プリセットの映像のビットレートに、kyusu の音声 (128 kbps) を足す。
    const bitrate =
      preset.videoBitrate === undefined ? null : preset.videoBitrate * 1_000 + audioOnlyBitrate;
    rungs.push({ name, height, bitrate });
  }

  return rungs.sort((a, b) => b.height - a.height);
}

// audioOnlyHeight は、段の表での音声だけの段の高さ。映像のある段より下に置く。
export const audioOnlyHeight = 0;

// routeQuality は、経路を表す画質の名前。再生できなければ null。
export function routeQuality(route: PlaybackRoute): string | null {
  switch (route.kind) {
    case 'convert':
      return original;
    case 'decode':
      return decoded;
    case 'kyusu':
      return route.preset;
    default:
      return null;
  }
}

// decodeStepDown は、デコードが追いつかないときに、current から 1 段下げた段を返す。下げる段が無ければ null。
// オリジナルからは、kyusu の 720p (無ければ、それより下で最も高い段) へ移す。
export function decodeStepDown(current: string, rungs: readonly Rung[]): Rung | null {
  const height = qualityHeight(current);
  if (height === null) {
    return null;
  }
  const below = height === originalHeight ? decodeStepFromOriginal + 1 : height;

  return rungs.find((rung) => rung.height < below) ?? null;
}

// capRoute は、自動で選んだ経路 base を、上限 cap (高さ) 以下の段に収める。base が上限以下か、収める段が無ければ base を返す。
// 上限が音声だけの段 (audioOnlyHeight) なら、audioOnly のときは kyusu の音声だけの画質にし、そうでなければ映像のある最も低い段にする。
export function capRoute(
  base: PlaybackRoute,
  cap: number | null,
  rungs: readonly Rung[],
  { audioOnly = false }: { audioOnly?: boolean } = {},
): PlaybackRoute {
  const name = routeQuality(base);
  if (cap === null || name === null) {
    return base;
  }
  const height = qualityHeight(name);
  if (height !== null && height <= cap) {
    return base;
  }
  if (cap <= audioOnlyHeight && audioOnly) {
    return { kind: 'kyusu', preset: audioStep };
  }
  const rung =
    rungs.find((candidate) => candidate.height <= cap) ??
    (cap <= audioOnlyHeight ? rungs.at(-1) : undefined);
  if (rung === undefined) {
    return base;
  }

  return rung.name === original ? { kind: 'convert' } : { kind: 'kyusu', preset: rung.name };
}

// networkCap は、回線のために自動で下げた段の上限 (高さ。音声だけなら audioOnlyHeight)。回線は選局し直しても変わらないので、
// デコードの上限と同じく、タブの中で持つ。上げ戻したら null にする。読み込み直すと捨てる。
let networkCap: number | null = null;

export function loadNetworkCap(): number | null {
  return networkCap;
}

// saveNetworkCap は、回線のための上限を height にする。null なら、上限を外す。
export function saveNetworkCap(height: number | null): void {
  networkCap = height;
}

// resetNetworkCapForTest は、テストの間で上限を捨てる。
export function resetNetworkCapForTest(): void {
  networkCap = null;
}

// minCap は、2 つの上限の低い方。どちらも無ければ null。
export function minCap(a: number | null, b: number | null): number | null {
  return a === null ? b : b === null ? a : Math.min(a, b);
}

// decodeCap は、デコードが追いつかなかった段の上限 (高さ)。端末の重さは選局し直しても変わらないので、
// 画面 (選局で作り直す) ではなく、タブの中で持つ。読み込み直すと捨てる。
let decodeCap: number | null = null;

export function loadDecodeCap(): number | null {
  return decodeCap;
}

// lowerDecodeCap は、上限を height に下げる。今の上限より高ければ、何もしない。
export function lowerDecodeCap(height: number): void {
  decodeCap = decodeCap === null ? height : Math.min(decodeCap, height);
}

// resetDecodeCapForTest は、テストの間で上限を捨てる。
export function resetDecodeCapForTest(): void {
  decodeCap = null;
}
