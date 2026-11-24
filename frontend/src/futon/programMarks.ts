// 番組名に入る囲み文字 (🈑 や 🅊 など、ARIB の追加記号) を、番組名から取り出す。
// 囲み文字は、フォントによっては潰れて読めないので、画面では番組名と分けて、自前のマークとして描く。

import { enclosedCharacters, enclosedGroups, type EnclosedGroup } from './enclosedCharacters.gen';

// 取り出す囲み文字は、バックエンドと同じテーブル (設計書 4.2) で決める。テーブルにない囲み文字 (🈀 や ㊙ など) は、番組名の一部として残す。
const enclosedClass = enclosedCharacters.map((c) => c.character).join('');
const enclosedMark = new RegExp(`[${enclosedClass}]`, 'gu');
// enclosedMarks は、続いた囲み文字と、その前後の空白。
const enclosedMarks = new RegExp(`\\s*(?:[${enclosedClass}]\\s*)+`, 'gu');
const enclosedLabels = new Map(enclosedCharacters.map((c) => [c.character, c.label]));
const enclosedDescriptions = new Map(enclosedCharacters.map((c) => [c.label, c.description]));

// markDescription は、囲み文字の内容の説明 (新なら新番組) を返す。テーブルにない内容なら undefined。
export function markDescription(label: string): string | undefined {
  return enclosedDescriptions.get(label);
}

// markOptions は、番組の条件で選べる囲み文字の内容を、まとまりごとにテーブルの順に並べる (設計書 4.2)。
// 括弧で書いたもの ([新] など) も、バックエンドで同じ囲み文字とみなす。
export const markOptions: readonly { group: EnclosedGroup; labels: readonly string[] }[] =
  enclosedGroups.map((group) => ({
    group,
    labels: enclosedCharacters.filter((c) => c.group === group).map((c) => c.label),
  }));

export type ProgramMarks = {
  // name は、囲み文字を除き、前後の空白を詰めた番組名。
  name: string;
  // marks は、囲み文字の内容 (🈑 なら「字」)。番組名に出てきた順に、重複を除いて並べる。
  marks: string[];
};

// splitProgramMarks は、番組名を、囲み文字を除いた番組名と、囲み文字の内容に分ける。
export function splitProgramMarks(name: string): ProgramMarks {
  const marks = new Set<string>();
  for (const [mark] of name.matchAll(enclosedMark)) {
    // テーブルで、囲みを外した内容 (🈑 → 字、🅊 → HV) にする。
    marks.add(enclosedLabels.get(mark) ?? mark);
  }

  return {
    // 囲み文字とその前後の空白を 1 つの空白にする。番組名の中の空白 (全角の空白など) は、そのまま残す。
    // 🈀 (ほか) は、フォントによっては潰れるので、番組名の中で「ほか」と書く。
    name: name.replace(enclosedMarks, ' ').replaceAll('\u{1F200}', 'ほか').trim(),
    marks: [...marks],
  };
}
