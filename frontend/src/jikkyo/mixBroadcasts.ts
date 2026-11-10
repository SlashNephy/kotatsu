import type { JikkyoComment } from './comments';
import { findPartMarkers, type PartMarker } from './partMarkers';

// 同じ話の別の放送の実況の過去ログを、録画の位置に合わせる (#1361、設計書 7.4)。
// 局ごとに CM の位置と長さが違うので、両方の放送の区切りを対応づけ、区間ごとにずらす。

// MatchedMarker は、対応づけた区切り。source は別の放送での時刻、target は録画での時刻 (どちらも秒)。
export type MatchedMarker = { source: number; target: number };

// minMatched は、混ぜるのに要る、共通の区切りの数の下限。maxShiftChange は、となりの区切りの間で許すずれの変化 (秒)。
const minMatched = 2;
const maxShiftChange = 180;
// shiftPenalty は、ずれの変化 1 秒あたりの減点。誤った区切りが混ざっていても、ずれの近い組を選ぶ。
// 上限まで変化しても、区切りが 1 つ増える分の加点を超えないようにする。
const shiftPenalty = 0.5 / maxShiftChange;

type Candidate = { source: number; target: number; sourceIndex: number; targetIndex: number };

// matchMarkers は、2 つの放送の区切りを、順番を保って対応づける。共通の区切りが足りなければ空を返す。
export function matchMarkers(
  source: readonly PartMarker[],
  target: readonly PartMarker[],
): MatchedMarker[] {
  const candidates: Candidate[] = [];
  source.forEach((s, sourceIndex) => {
    target.forEach((t, targetIndex) => {
      if (s.part === t.part) {
        candidates.push({ source: s.time, target: t.time, sourceIndex, targetIndex });
      }
    });
  });
  const shift = (candidate: Candidate) => candidate.target - candidate.source;
  // scores[k] は、candidates[k] で終わる対応づけの点。previous[k] は、その 1 つ前の候補。
  const scores = candidates.map(() => 1);
  const previous = candidates.map(() => -1);
  candidates.forEach((candidate, k) => {
    for (let l = 0; l < k; l++) {
      const before = candidates[l]!;
      const change = Math.abs(shift(candidate) - shift(before));
      if (
        before.sourceIndex >= candidate.sourceIndex ||
        before.targetIndex >= candidate.targetIndex ||
        change > maxShiftChange
      ) {
        continue;
      }
      const score = scores[l]! + 1 - change * shiftPenalty;
      if (score > scores[k]!) {
        scores[k] = score;
        previous[k] = l;
      }
    }
  });
  let best = -1;
  candidates.forEach((_, k) => {
    if (best === -1 || scores[k]! > scores[best]!) {
      best = k;
    }
  });
  const matched: MatchedMarker[] = [];
  for (let k = best; k !== -1; k = previous[k]!) {
    const { source: sourceTime, target: targetTime } = candidates[k]!;
    matched.unshift({ source: sourceTime, target: targetTime });
  }

  return matched.length >= minMatched ? matched : [];
}

// mapComments は、別の放送のコメントを、録画の位置に直す。どの区間にも入らないコメント (CM の間など) は捨てる。
export function mapComments(
  comments: readonly JikkyoComment[],
  matched: readonly MatchedMarker[],
): JikkyoComment[] {
  const mapped: JikkyoComment[] = [];
  for (const comment of comments) {
    // comment.time を含む区間を探す。区切りの数は少ないので、順に見る。
    let index = -1;
    while (index + 1 < matched.length && matched[index + 1]!.source <= comment.time) {
      index++;
    }
    if (index === -1) {
      continue;
    }
    const current = matched[index]!;
    const next = matched[index + 1];
    const elapsed = comment.time - current.source;
    // 区間の長さは、短いほうに合わせる。長いほうのはみ出した分は、CM などの片方にしかない部分である。
    if (
      next !== undefined &&
      elapsed >= Math.min(next.source - current.source, next.target - current.target)
    ) {
      continue;
    }
    mapped.push({ ...comment, time: current.target + elapsed });
  }

  return mapped;
}

// mixBroadcast は、別の放送のコメント other を、録画自身のコメント own の区切りに合わせて返す。区切りが足りなければ空を返す。
export function mixBroadcast(
  own: readonly JikkyoComment[],
  other: readonly JikkyoComment[],
): JikkyoComment[] {
  return mapComments(other, matchMarkers(findPartMarkers(other), findPartMarkers(own)));
}
