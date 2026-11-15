// 画質の「自動」で、デコードが追いつかない端末の段を下げる (#1513、設計書 7.1)。
// 段の表は、上から、オリジナル (ブラウザの中で変換) → kyusu の 1080p → 720p → 540p → 360p。

import { original, type PlaybackRoute, type Preset } from './route';

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
// 映像を変換しない画質 (source) は BS4K の 2160p。
export function qualityHeight(name: string): number | null {
  if (name === 'source') {
    return 2160;
  }
  const ladder = ladderHeight(name);
  if (ladder !== null) {
    return ladder;
  }
  const match = /^(\d+)p/.exec(name);

  return match === null ? null : Number(match[1]);
}

// Rung は、段の表の 1 段。
export type Rung = { name: string; height: number };

// ladder は、選べる画質から段の表を作る。高い順に並べる。kyusu の段は H.264 のものだけを使う。
export function ladder(choices: readonly string[], presets: readonly Preset[]): Rung[] {
  const rungs: Rung[] = [];
  for (const name of choices) {
    const height = ladderHeight(name);
    if (height === null) {
      continue;
    }
    if (name !== original) {
      const preset = presets.find((candidate) => candidate.name === name);
      if (preset === undefined || preset.codec !== 'H264' || preset.passthrough) {
        continue;
      }
    }
    rungs.push({ name, height });
  }

  return rungs.sort((a, b) => b.height - a.height);
}

// routeQuality は、経路を表す画質の名前。再生できなければ null。
export function routeQuality(route: PlaybackRoute): string | null {
  return route.kind === 'convert' ? original : route.kind === 'kyusu' ? route.preset : null;
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
export function capRoute(
  base: PlaybackRoute,
  cap: number | null,
  rungs: readonly Rung[],
): PlaybackRoute {
  const name = routeQuality(base);
  if (cap === null || name === null) {
    return base;
  }
  const height = qualityHeight(name);
  if (height !== null && height <= cap) {
    return base;
  }
  const rung = rungs.find((candidate) => candidate.height <= cap);
  if (rung === undefined) {
    return base;
  }

  return rung.name === original ? { kind: 'convert' } : { kind: 'kyusu', preset: rung.name };
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
