// 番組名に入る囲み文字 (🈑 や 🅊 など、ARIB の追加記号) を、番組名から取り出す。
// 囲み文字は、フォントによっては潰れて読めないので、画面では番組名と分けて、自前の印として描く。

// enclosedMark は、囲み文字の範囲。英字の囲み文字 (🄰〜🅏、HV や SD などを含む) と、仮名や漢字の囲み文字 (🈀〜)。
const enclosedMark = /[\u{1F130}-\u{1F14F}\u{1F200}-\u{1F2FF}]/gu;
// enclosedMarks は、続いた囲み文字と、その前後の空白。
const enclosedMarks = /\s*(?:[\u{1F130}-\u{1F14F}\u{1F200}-\u{1F2FF}]\s*)+/gu;

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
    // NFKC で、囲みを外した中身 (🈑 → 字、🅊 → HV) にする。
    marks.add(mark.normalize('NFKC'));
  }

  return {
    // 囲み文字とその前後の空白を 1 つの空白にする。番組名の中の空白 (全角の空白など) は、そのまま残す。
    name: name.replace(enclosedMarks, ' ').trim(),
    marks: [...marks],
  };
}
