import type { JikkyoComment } from './comments';
import type { PartMarker } from './partMarkers';

// 上映会のコメント (NICOLOG) を、録画の位置に合わせる (#1363、設計書 7.4)。
// 上映会には CM がないので、録画の CM の区間を除いた時間軸で、両方の `OP` の位置を合わせる。`A` と `B` は CM が明けたパートを指すので、上映会には書き込まれない。

// CmSection は、録画の CM の区間 (録画の始まりからの秒)。
export type CmSection = { from: number; to: number };

// withoutCm は、録画の位置 position から、それより前の CM の長さを除いた時刻を返す。CM の中なら null を返す。
function withoutCm(position: number, sections: readonly CmSection[]): number | null {
  let removed = 0;
  for (const section of sections) {
    if (position >= section.to) {
      removed += section.to - section.from;
    } else if (position > section.from) {
      return null;
    }
  }

  return position - removed;
}

// withCm は、CM を除いた時刻 main を、録画の位置に直す。
function withCm(main: number, sections: readonly CmSection[]): number {
  let position = main;
  for (const section of sections) {
    if (position >= section.from) {
      position += section.to - section.from;
    }
  }

  return position;
}

// firstOp は、最初の `OP` の時刻を返す。なければ null を返す。
const firstOp = (markers: readonly PartMarker[]): number | null =>
  markers.find((marker) => marker.part === 'OP')?.time ?? null;

// alignNicolog は、上映会のコメントを録画の位置に直して返す。どちらかに `OP` がないか、CM の区間がなければ空を返す。
// `OP` より前のコメントは、録画と上映会でずれることがあるので捨てる。
export function alignNicolog(
  comments: readonly JikkyoComment[],
  nicologMarkers: readonly PartMarker[],
  ownMarkers: readonly PartMarker[],
  cmSections: readonly CmSection[],
): JikkyoComment[] {
  const sections = [...cmSections].sort((a, b) => a.from - b.from);
  const nicologOp = firstOp(nicologMarkers);
  const ownOp = firstOp(ownMarkers);
  const ownOpMain = ownOp === null ? null : withoutCm(ownOp, sections);
  if (sections.length === 0 || nicologOp === null || ownOpMain === null) {
    return [];
  }
  const shift = ownOpMain - nicologOp;

  return comments.flatMap((comment) =>
    comment.time < nicologOp ? [] : [{ ...comment, time: withCm(comment.time + shift, sections) }],
  );
}
