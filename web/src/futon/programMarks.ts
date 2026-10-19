// 番組名に入る囲み文字 (🈑 や 🅊 など、ARIB の追加記号) を、番組名から取り出す。
// 囲み文字は、フォントによっては潰れて読めないので、画面では番組名と分けて、自前の印として描く。

// enclosedRanges は、囲み文字の範囲 (#434)。
// - 英字の囲み文字 (🄰〜🅏、HV や SD などを含む)
// - 4K や 5.1 などの囲み文字 (🆛〜🆬)
// - 仮名や漢字の囲み文字 (🈁〜)。🈀 (ほか) は、番組名の一部 (「〜ほか」) なので、番組名に残す。
// - 鍵の囲み文字 (⚿)
const enclosedRanges = '\\u{1F130}-\\u{1F14F}\\u{1F19B}-\\u{1F1AC}\\u{1F201}-\\u{1F2FF}\\u{26BF}';
const enclosedMark = new RegExp(`[${enclosedRanges}]`, 'gu');
// enclosedMarks は、続いた囲み文字と、その前後の空白。
const enclosedMarks = new RegExp(`\\s*(?:[${enclosedRanges}]\\s*)+`, 'gu');

// markLabels は、NFKC で中身を取り出せない囲み文字の中身。Unicode の文字名 (SQUARED FOUR K など) から決めた。
const markLabels: Readonly<Record<string, string>> = {
  '\u{1F19B}': '3D',
  '\u{1F19C}': '2ndScr',
  '\u{1F19D}': '2K',
  '\u{1F19E}': '4K',
  '\u{1F19F}': '8K',
  '\u{1F1A0}': '5.1',
  '\u{1F1A1}': '7.1',
  '\u{1F1A2}': '22.2',
  '\u{1F1A3}': '60P',
  '\u{1F1A4}': '120P',
  '\u{1F1A5}': 'd',
  '\u{1F1A6}': 'HC',
  '\u{1F1A7}': 'HDR',
  '\u{1F1A8}': 'Hi-Res',
  '\u{1F1A9}': 'Lossless',
  '\u{1F1AA}': 'SHV',
  '\u{1F1AB}': 'UHD',
  '\u{1F1AC}': 'VOD',
  '\u26BF': '鍵',
};

// markOptions は、番組の条件で選べる囲み文字の中身 (設計書 4.2)。番組名に付く ARIB の囲み文字を、よく使うものから並べる。
// 括弧で書いたもの ([新] など) も、バックエンドで同じ囲み文字とみなす。
export const markOptions = {
  // 漢字と仮名の囲み文字
  ideographic: [
    '新',
    '終',
    '再',
    '初',
    '生',
    '前',
    '後',
    '字',
    '二',
    '多',
    '解',
    '双',
    'デ',
    '手',
    '吹',
    '声',
    '無',
    '料',
    '映',
    '天',
    '交',
    '販',
    '鍵',
  ],
  // 英数字の囲み文字
  alphanumeric: [
    'S',
    'SS',
    'B',
    'N',
    'P',
    'W',
    'HV',
    'MV',
    'SD',
    'PPV',
    '2K',
    '4K',
    '8K',
    '3D',
    'HDR',
    'SHV',
    'UHD',
    '60P',
    '120P',
    'HC',
    '5.1',
    '7.1',
    '22.2',
    'Hi-Res',
    'Lossless',
    'VOD',
    '2ndScr',
    'd',
  ],
} as const;

export type ProgramMarks = {
  // name は、囲み文字を除き、前後の空白を詰めた番組名。
  name: string;
  // marks は、囲み文字の中身 (🈑 なら「字」)。番組名に出てきた順に、重複を除いて並べる。
  marks: string[];
};

// splitProgramMarks は、番組名を、囲み文字を除いた番組名と、囲み文字の中身に分ける。
export function splitProgramMarks(name: string): ProgramMarks {
  const marks = new Set<string>();
  for (const [mark] of name.matchAll(enclosedMark)) {
    // NFKC で、囲みを外した中身 (🈑 → 字、🅊 → HV) にする。NFKC で分解されない囲み文字は、表で引く。
    marks.add(markLabels[mark] ?? mark.normalize('NFKC'));
  }

  return {
    // 囲み文字とその前後の空白を 1 つの空白にする。番組名の中の空白 (全角の空白など) は、そのまま残す。
    // 🈀 (ほか) は、フォントによっては潰れるので、番組名の中で「ほか」と書く。
    name: name.replace(enclosedMarks, ' ').replaceAll('\u{1F200}', 'ほか').trim(),
    marks: [...marks],
  };
}
