// skipEndMargin は、CM の区間の終わりの何秒前から、スキップするボタンを出さないか (#1300)。
// 終わりの間際に押しても、ほとんど飛ばせず、ボタンが出たり消えたりするだけなので出さない。自動で飛ばす判定には使わない (#1512)。
export const skipEndMargin = 1;

// cmMergeGap は、自動で飛ばすときに、続いた CM の区間とみなす、前の区間の終わりと次の区間の始まりの隙間 (秒、#1512)。
// tenban は、セグメントをまたいだ区間を 1 秒より近ければまとめて返すので (設計書 6.1)、それと同じ幅にする。
// 区間の時刻はミリ秒で、再生の位置に直すときの誤差はそれより十分小さい。提供の読み上げのような短い本編 (5 秒ほど) は飛ばさない。
export const cmMergeGap = 1;

// cmLookahead は、自動で飛ばすときに、いま映ったコマの時刻に足して CM に入るかを見る時間 (秒、#1512)。
// 描画の側へ渡し済みのコマと、蓋が画面に出るまでの間に映るコマの分を、先に飛ぶ。
// 合成の録画を Chromium で再生して測ると、1 コマ (約 0.033 秒) 以下では CM のコマが映り、2 コマ以上では映らなかった。
// 24p で出しているとき、コマを落としたとき、Windows の Chrome と Edge の分の余裕を足した (設計書 付録 A)。
export const cmLookahead = 0.2;

export type CmSection = { from: number; to: number };

// cmButtonSectionAt は、「CMをスキップ」のボタンを出す CM の区間 (位置 position を含むもの) を返す。なければ null を返す。
// 区間の終わりの skipEndMargin 秒の間は、含めない。
export function cmButtonSectionAt<T extends CmSection>(
  sections: readonly T[],
  position: number,
): T | null {
  return (
    sections.find((section) => section.from <= position && position < section.to - skipEndMargin) ??
    null
  );
}

// cmSectionAt は、位置 position から lookahead 秒先までの間に掛かる CM の区間を返す。なければ null を返す。
// position が区間の中か、position + lookahead が区間に入っていれば、その区間を返す。
export function cmSectionAt<T extends CmSection>(
  sections: readonly T[],
  position: number,
  lookahead = 0,
): T | null {
  return (
    sections.find((section) => section.from <= position + lookahead && position < section.to) ??
    null
  );
}

// mergeCmSections は、区間を始まりの順に並べ、重なるか、隙間が gap 秒以内の区間を 1 つにまとめる。
// 自動で飛ぶときは、続いた CM を 1 回で跨ぐ (#1512)。15 秒ずつ飛ぶと、そのたびに止まった絵と移動が起きる。
export function mergeCmSections(sections: readonly CmSection[], gap: number): CmSection[] {
  const sorted = sections
    .filter((section) => section.from < section.to)
    .toSorted((a, b) => a.from - b.from);
  const merged: CmSection[] = [];
  for (const section of sorted) {
    const last = merged.at(-1);
    if (last !== undefined && section.from - last.to <= gap) {
      last.to = Math.max(last.to, section.to);
    } else {
      merged.push({ from: section.from, to: section.to });
    }
  }

  return merged;
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
