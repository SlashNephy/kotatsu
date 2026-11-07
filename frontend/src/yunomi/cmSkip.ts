// skipEndMargin は、CM の区間の終わりの何秒前から、スキップするボタンを出さないか (#1300)。
// 終わりの間際に押しても、ほとんど飛ばせず、ボタンが出たり消えたりするだけなので出さない。
export const skipEndMargin = 1;

export type CmSection = { from: number; to: number };

// cmSectionAt は、再生位置 position (秒) を含む CM の区間を返す。なければ null を返す。
// 区間の終わりの skipEndMargin 秒の間は、含めない。
export function cmSectionAt<T extends CmSection>(
  sections: readonly T[],
  position: number,
): T | null {
  return (
    sections.find((section) => section.from <= position && position < section.to - skipEndMargin) ??
    null
  );
}

// shouldAutoSkip は、CM を自動で飛ばす設定 (#1282) のとき、いま再生している CM の区間 section を飛ばすかを返す。
// 再生していない間と、利用者が自分でその区間へシークしたとき (lastSeek が区間の中) は、見たいものとして飛ばさない。
export function shouldAutoSkip(
  section: CmSection | null,
  enabled: boolean,
  playing: boolean,
  lastSeek: number | null,
): boolean {
  if (!enabled || !playing || section === null) {
    return false;
  }

  return lastSeek === null || lastSeek < section.from || section.to <= lastSeek;
}
