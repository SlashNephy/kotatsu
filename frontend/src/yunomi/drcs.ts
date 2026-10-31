import { useSyncExternalStore } from 'react';

import { md5 } from './md5';

// 字幕の外字 (DRCS) を文字に置き換える表と、置き換えられなかった外字の一覧 (#825、設計書 7.4)。
// aribb24.js は、描くたびに、描画器に渡した Map を読むので、ページの中で 1 つの Map を共有し、中身だけを入れ替える。
// こうすると、表を読み直したり外字を登録したりしても、字幕の描画器を作り直さずに、次の字幕から効く。

// drcsReplacements は、外字の画像の MD5 (小文字の 16 進) から、置き換える文字への表。字幕の描画器すべてに渡す。
export const drcsReplacements = new Map<string, string>();

// DRCSGlyph は、字幕に出た外字。画像は、1 画素あたり depth ビットで、左上から行の順に詰めてある。
export type DRCSGlyph = {
  md5: string;
  width: number;
  height: number;
  depth: number;
  binary: ArrayBuffer;
};

// maxUnknown は、覚えておく、置き換えられなかった外字の数の上限。番組をまたいで見続けても、膨らみすぎないようにする。
const maxUnknown = 100;

// seen は、字幕に出た外字。MD5 で引き、新しく出たものほど後ろに置く。
const seen = new Map<string, DRCSGlyph>();
const listeners = new Set<() => void>();
// unknownSnapshot は、seen のうち、表に無いもの。新しい順に並べる。変わるたびに作り直し、useSyncExternalStore に渡す。
let unknownSnapshot: readonly DRCSGlyph[] = [];

const notify = () => {
  unknownSnapshot = [...seen.values()]
    .filter((glyph) => !drcsReplacements.has(glyph.md5))
    .reverse();
  for (const listener of listeners) {
    listener();
  }
};

// maxDRCSText は、外字を置き換える文字の長さ (文字数) の上限。サーバーと同じ値にする。
// aribb24.js は、置き換えた文字を外字の 1 文字の枠に縮めて描くので、長いと読めない。
const maxDRCSText = 4;

// validDRCSText は、外字を置き換える文字として使えるかを返す。サーバーと同じく、コードポイントで数える。
export function validDRCSText(text: string): boolean {
  const length = Array.from(text).length;

  return length > 0 && length <= maxDRCSText;
}

type Row = { readonly md5: string; readonly text: string };

// diffDRCS は、base から rows への、外字の表の変更を返す。消した外字は、text を null にする。
export function diffDRCS(
  base: readonly Row[],
  rows: readonly Row[],
): { md5: string; text: string | null }[] {
  const before = new Map(base.map((row) => [row.md5, row.text]));
  const after = new Map(rows.map((row) => [row.md5, row.text]));
  const changes: { md5: string; text: string | null }[] = [];
  for (const [md5, text] of after) {
    if (before.get(md5) !== text) {
      changes.push({ md5, text });
    }
  }
  for (const md5 of before.keys()) {
    if (!after.has(md5)) {
      changes.push({ md5, text: null });
    }
  }

  return changes;
}

// setDRCSReplacements は、外字の置き換えの表を入れ替える。
export function setDRCSReplacements(rows: readonly { md5: string; text: string }[]) {
  drcsReplacements.clear();
  for (const { md5: key, text } of rows) {
    drcsReplacements.set(key.toLowerCase(), text);
  }
  notify();
}

// recordDRCS は、字幕の部品のうち外字を、出たものとして覚える。表に無い外字は、管理者が登録できるよう一覧に出す。
// tokens は、aribb24.js の描画器が受け取る部品の並び。外字のほかは読まない。
export function recordDRCS(tokens: readonly unknown[]) {
  let changed = false;
  for (const token of tokens) {
    if (!isDRCSToken(token)) {
      continue;
    }
    const key = md5(token.binary);
    if (seen.has(key)) {
      // 出た順を新しくする。
      const glyph = seen.get(key)!;
      seen.delete(key);
      seen.set(key, glyph);
      changed ||= !drcsReplacements.has(key);
      continue;
    }
    seen.set(key, {
      md5: key,
      width: token.width,
      height: token.height,
      depth: token.depth,
      binary: token.binary,
    });
    if (seen.size > maxUnknown) {
      seen.delete(seen.keys().next().value!);
    }
    changed = true;
  }
  if (changed) {
    notify();
  }
}

function isDRCSToken(token: unknown): token is Omit<DRCSGlyph, 'md5'> {
  return (
    typeof token === 'object' &&
    token !== null &&
    (token as { tag?: unknown }).tag === 'DRCS' &&
    (token as { binary?: unknown }).binary instanceof ArrayBuffer
  );
}

// useUnknownDRCS は、字幕に出たが表に無い外字を、新しい順に返す。
export function useUnknownDRCS(): readonly DRCSGlyph[] {
  return useSyncExternalStore(
    (listener) => {
      listeners.add(listener);

      return () => listeners.delete(listener);
    },
    () => unknownSnapshot,
  );
}

// drcsPixels は、外字の画像の各画素の濃さ (0〜1) を、左上から行の順に返す。
export function drcsPixels(
  glyph: Pick<DRCSGlyph, 'width' | 'height' | 'depth' | 'binary'>,
): number[] {
  const bytes = new Uint8Array(glyph.binary);
  const max = 2 ** glyph.depth - 1;
  const pixels: number[] = [];
  for (let i = 0; i < glyph.width * glyph.height; i++) {
    let value = 0;
    for (let bit = 0; bit < glyph.depth; bit++) {
      const index = i * glyph.depth + bit;
      value = (value << 1) | (((bytes[index >> 3] ?? 0) >> (7 - (index & 7))) & 1);
    }
    pixels.push(max === 0 ? 0 : value / max);
  }

  return pixels;
}

// drcsImage は、外字の画像を、PNG の data URL にする。画面の色に合わせて描けるよう、濃さを透明度にし、色は color で塗る。
export function drcsImage(glyph: DRCSGlyph, color: string): string {
  const canvas = document.createElement('canvas');
  canvas.width = glyph.width;
  canvas.height = glyph.height;
  const context = canvas.getContext('2d');
  if (context === null) {
    return '';
  }
  context.fillStyle = color;
  drcsPixels(glyph).forEach((alpha, i) => {
    if (alpha > 0) {
      context.globalAlpha = alpha;
      context.fillRect(i % glyph.width, Math.floor(i / glyph.width), 1, 1);
    }
  });

  return canvas.toDataURL();
}

// resetDRCSForTest は、覚えた外字と表を空にする。テストのためだけに使う。
export function resetDRCSForTest() {
  seen.clear();
  drcsReplacements.clear();
  notify();
}
