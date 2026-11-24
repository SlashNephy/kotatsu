import { useSyncExternalStore } from 'react';

import defaultTable from '../../vendor/libaribcaption/drcs.json';
import { smoothDRCS, type DRCSShape } from './drcsSmooth';
import { md5 } from './md5';

// 字幕の外字 (DRCS) を文字に置き換えるテーブルと、置き換えられなかった外字の一覧 (#825、設計書 7.4)。
// aribb24.js は、描くたびに、レンダラーに渡した Map を読むので、ページの中で 1 つの Map を共有し、内容だけを入れ替える。
// こうすると、テーブルを読み直したり外字を登録したりしても、字幕のレンダラーを作り直さずに、次の字幕から効く。

// defaultReplacements は、同梱の既定のテーブル (vendor/libaribcaption/SOURCE の commit から作る)。
const defaultReplacements = Object.entries<string>(defaultTable.replacements);

// drcsReplacements は、外字の画像の MD5 (小文字の 16 進) から、置き換える文字へのテーブル。字幕のレンダラーすべてに渡す。
// 既定のテーブルから始めるので、管理者のテーブルを読めなくても、よく使う外字は置き換わる。
export const drcsReplacements = new Map<string, string>(defaultReplacements);

// DRCSGlyph は、字幕に出た外字。画像は、1 画素あたり depth ビットで、左上から行の順に詰めてある。
export type DRCSGlyph = {
  md5: string;
  width: number;
  height: number;
  depth: number;
  binary: ArrayBuffer;
};

// maxUnknown は、保持しておく、置き換えられなかった外字の数の上限。番組をまたいで見続けても、膨らみすぎないようにする。
const maxUnknown = 100;

// seen は、字幕に出た外字。MD5 で引き、新しく出たものほど後ろに置く。
const seen = new Map<string, DRCSGlyph>();
const listeners = new Set<() => void>();
// unknownSnapshot は、seen のうち、テーブルに無いもの。新しい順に並べる。変わるたびに作り直し、useSyncExternalStore に渡す。
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
export const maxDRCSText = 4;

// validDRCSText は、外字を置き換える文字として使えるかを返す。サーバーと同じく、コードポイントで数える。
export function validDRCSText(text: string): boolean {
  const length = Array.from(text).length;

  return length > 0 && length <= maxDRCSText;
}

// restoreDefaults は、テーブルを既定のテーブルだけに戻す。
function restoreDefaults() {
  drcsReplacements.clear();
  for (const [key, text] of defaultReplacements) {
    drcsReplacements.set(key, text);
  }
}

// setDRCSReplacements は、外字の置き換えのテーブルを、既定のテーブルに管理者の登録 (rows) を重ねたものに入れ替える。登録を優先する。
export function setDRCSReplacements(rows: readonly { md5: string; text: string }[]) {
  restoreDefaults();
  for (const { md5: key, text } of rows) {
    drcsReplacements.set(key.toLowerCase(), text);
  }
  // 登録した外字は文字で描くので、私用領域の割り当てを外す。
  for (const key of privateUse.keys()) {
    if (drcsReplacements.has(key)) {
      privateUse.delete(key);
    }
  }
  notify();
}

// 置き換えられない外字は、なめらかにした形で描く (#1518)。aribb24.js の canvas のレンダラーは、外字を文字に置き換えた後、
// 文字を path に置き換えるテーブル (replace.glyph) を引いてから描くので、外字に私用領域の 1 文字を割り当て、その文字の path を登録する。
// 割り当ては、drcsReplacements に足さず、映像に重ねるレンダラーにだけ渡す。字幕の文 (TextRenderer) では「〓」のままにし、
// 置き換えられなかった外字の一覧にも残すためである。

// GlyphPath は、aribb24.js の replace.glyph に登録する形。viewBox の広さが、字の枠 (fontsize) に縮めて描かれる。
type GlyphPath = { viewBox: [number, number, number, number]; path: string };

// privateUseFirst と privateUseLast は、割り当てる私用領域の範囲。
// aribb24.js は、usePUA を付けると ARIB の追加記号を U+E0D8〜U+E39C に置くので、そこを避けて U+E400 から使う。
const privateUseFirst = 0xe400;
const privateUseLast = 0xf8ff;
// maxSmoothed は、なめらかにした結果を保持しておく件数の上限。溢れたら、割り当てと登録した path ごと破棄して作り直す。
// 1 つの番組に出る置き換えられない外字は、多くても数十種類なので、番組をまたいでも作り直しはまれである。
// path は 1 件あたり数 KB なので、上限まで溜まっても 1 MB ほどに収まる。
const maxSmoothed = 256;

// smoothed は、なめらかにした結果。キーは、元の画像の MD5、幅と高さ、1 画素のビット数の組。MD5 は画素の並びしか見ないので、大きさも含める。
// 読めない画像は null を保持し、手を加えずに描く。
const smoothed = new Map<string, DRCSShape | null>();
// privateUse は、外字の画像の MD5 から、割り当てた私用領域の文字へのテーブル。
const privateUse = new Map<string, string>();
let nextPrivateUse = privateUseFirst;

// drcsGlyphs は、私用領域の文字から、なめらかにした外字の path へのテーブル。映像に重ねるレンダラーの replace.glyph に渡す。
export const drcsGlyphs = new Map<string, GlyphPath>();

// RenderReplacements は、drcsReplacements と私用領域の割り当ての両方を検索するテーブル。aribb24.js は get だけを呼ぶので、
// get と has だけを重ねる。内容を並べる操作 (size や反復) は、空のテーブルとして振る舞う。
class RenderReplacements extends Map<string, string> {
  override get(key: string): string | undefined {
    return drcsReplacements.get(key) ?? privateUse.get(key);
  }

  override has(key: string): boolean {
    return drcsReplacements.has(key) || privateUse.has(key);
  }
}

// drcsRenderReplacements は、映像に重ねるレンダラーに渡す外字のテーブル。管理者の登録と既定のテーブルを、割り当てより優先する。
export const drcsRenderReplacements: Map<string, string> = new RenderReplacements();

function forgetSmoothed() {
  smoothed.clear();
  privateUse.clear();
  drcsGlyphs.clear();
  nextPrivateUse = privateUseFirst;
}

// prepareDRCS は、レンダラーが描く直前に呼び、置き換えられない外字に私用領域の文字を割り当て、なめらかにした path を登録する。
// fontsize は、描き始めの字の大きさ (aribb24.js の initialState の fontsize)。tokens は、レンダラーが受け取る部品の並び。
// aribb24.js は、path を、元の画像の大きさではなく字の大きさに縮めて描くので、viewBox を広げて、元の外字と同じ大きさにする。
export function prepareDRCS(fontsize: readonly [number, number], tokens: readonly unknown[]) {
  // 描いている途中で破棄すると、その字幕の外字が引けなくなるので、破棄するのは描き始める前にする。
  if (smoothed.size >= maxSmoothed || nextPrivateUse > privateUseLast) {
    forgetSmoothed();
  }
  let size = fontsize;
  for (const token of tokens) {
    if (isCompositionToken(token)) {
      // 字の大きさを変える指定 (SSM)。aribb24.js も、ここで字の大きさを変える。
      size = [token.horizontal, token.vertical];
      continue;
    }
    if (!isDRCSToken(token)) {
      continue;
    }
    // 置き換えテーブルは、元の画像の MD5 で検索する。
    const key = md5(token.binary);
    if (drcsReplacements.has(key)) {
      continue;
    }
    const cacheKey = `${key}:${token.width}x${token.height}:${token.depth}`;
    let shape = smoothed.get(cacheKey);
    if (shape === undefined) {
      shape = smoothDRCS(token);
      smoothed.set(cacheKey, shape);
    }
    if (shape === null) {
      continue;
    }
    let character = privateUse.get(key);
    if (character === undefined) {
      if (nextPrivateUse > privateUseLast) {
        continue;
      }
      character = String.fromCodePoint(nextPrivateUse++);
      privateUse.set(key, character);
    }
    const glyph: GlyphPath = {
      viewBox: [
        0,
        0,
        (shape.width * size[0]) / token.width,
        (shape.height * size[1]) / token.height,
      ],
      path: shape.path,
    };
    drcsGlyphs.set(character, glyph);
    // 結合する文字 (combining) があると、aribb24.js は、それを足した文字列で path を検索する。
    // 外字の描画でも結合する文字は描かないので、同じ path を登録する。
    const combining = (token as { combining?: unknown }).combining;
    if (typeof combining === 'string' && combining !== '') {
      drcsGlyphs.set(character + combining, glyph);
    }
  }
}

function isCompositionToken(
  token: unknown,
): token is { tag: 'CharacterCompositionDotDesignation'; horizontal: number; vertical: number } {
  return (
    typeof token === 'object' &&
    token !== null &&
    (token as { tag?: unknown }).tag === 'CharacterCompositionDotDesignation'
  );
}

// recordDRCS は、字幕の部品のうち外字を、出たものとして保持する。テーブルに無い外字は、管理者が登録できるよう一覧に出す。
// tokens は、aribb24.js のレンダラーが受け取る部品の並び。外字のほかは読まない。
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

// useUnknownDRCS は、字幕に出たがテーブルに無い外字を、新しい順に返す。
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

// resetDRCSForTest は、保持した外字を忘れ、テーブルを既定のテーブルに戻す。テストのためだけに使う。
export function resetDRCSForTest() {
  seen.clear();
  restoreDefaults();
  forgetSmoothed();
  notify();
}
