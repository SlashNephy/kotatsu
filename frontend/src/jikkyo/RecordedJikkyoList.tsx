import type { TFunction } from 'i18next';

import { broadcastTime, formatClock, formatDuration, type Gap } from '../yunomi/position';
import { watchCommentsHeight } from '../yunomi/WatchLayout';
import { CommentList } from './CommentList';
import { countUntil, listedUntil } from './commentList';
import type { JikkyoComment } from './comments';
import { findPartMarkers } from './partMarkers';
import type { RecordedJikkyoState } from './useRecordedJikkyo';

// 録画とライブラリのファイルの再生で、実況の過去ログ (設計書 7.4) を出すための部品 (#2244)。

// recordedJikkyoLabel は、実況のボタンのツールチップ。過去ログの読み込み具合を添える。
export function recordedJikkyoLabel(t: TFunction, state: RecordedJikkyoState | null): string {
  if (state === null) {
    return t('jikkyo.toggle');
  }

  return t('jikkyo.toggleWith', {
    state:
      state.kind === 'loading'
        ? t('jikkyo.kakologLoading')
        : state.kind === 'error'
          ? t('jikkyo.kakologError')
          : state.count === 0
            ? t('jikkyo.kakologEmpty')
            : t('jikkyo.kakologLoaded', { count: state.count }),
  });
}

// jikkyoPartBands は、実況のコメントから見つけた番組の区切りのマーク (#1358)。コメントの位置をずらしたら (offset)、
// マークも同じだけ動かす。
export function jikkyoPartBands(
  t: TFunction,
  ownComments: readonly JikkyoComment[] | null,
  offset: number,
): { from: number; to: number; label: string; kind: 'part' }[] {
  return findPartMarkers(ownComments ?? []).map((marker) => {
    const at = Math.max(marker.time + offset, 0);
    const part = t(`player.partMarker.${marker.part}`);

    return {
      from: at,
      to: at,
      label: marker.noCmBefore ? t('player.partMarkerNoCm', { part }) : part,
      kind: 'part' as const,
    };
  });
}

// jikkyoListedCount は、再生位置 position までに流れた過去ログの数。一覧は、この数が変わったときだけ作り直す。
export function jikkyoListedCount(
  comments: readonly JikkyoComment[] | null,
  position: number,
  offset: number,
): number {
  return comments === null ? 0 : countUntil(comments, position - offset);
}

// labelCache は、コメントごとに作った時刻の文字。一覧は、コメントが流れるたびに作り直すので、作り直すたびに全部の時刻を
// 書き直さないよう、コメントごとに一度だけ作る。録り始めや途切れた区間や位置のずれが変わったら、作り直す。
const labelCache = new WeakMap<
  JikkyoComment,
  { recordedFrom: string | null; gaps: readonly Gap[]; offset: number; label: string }
>();

function commentLabel(
  comment: JikkyoComment,
  offset: number,
  recordedFrom: string | null,
  gaps: readonly Gap[],
): string {
  const cached = labelCache.get(comment);
  if (
    cached !== undefined &&
    cached.recordedFrom === recordedFrom &&
    cached.gaps === gaps &&
    cached.offset === offset
  ) {
    return cached.label;
  }
  const seconds = comment.time + offset;
  const label =
    recordedFrom === null
      ? formatDuration(seconds)
      : formatClock(broadcastTime(recordedFrom, gaps, seconds));
  labelCache.set(comment, { recordedFrom, gaps, offset, label });

  return label;
}

// RecordedJikkyoList は、実況の過去ログのうち、再生位置までに流れた count 件の一覧 (#489)。時刻は、録り始め (recordedFrom) からの
// 放送時刻で出す。録り始めが分からなければ、再生の経過時間で出す。再生位置は 1 秒に何度も変わるので、受け取るのは流れた数にし、
// 数が変わったときだけ作り直す。
export function RecordedJikkyoList({
  comments,
  count,
  offset,
  recordedFrom,
  gaps,
  frozen,
  onMuteUser,
}: {
  comments: readonly JikkyoComment[];
  count: number;
  offset: number;
  recordedFrom: string | null;
  gaps: readonly Gap[];
  // frozen は、全画面の間のように一覧が見えないときに、コメントが届いても描き直さないかどうか。
  frozen: boolean;
  // onMuteUser は、行から、そのコメントを書いたユーザーをミュートする (#767)。渡さなければ、メニューに出さない。
  onMuteUser?: (userId: string) => void;
}) {
  // 全画面の間は、一覧が描き直さないので、作らない。
  const rows = frozen
    ? []
    : listedUntil(comments, count).map(({ index, comment }) => ({
        key: index,
        label: commentLabel(comment, offset, recordedFrom, gaps),
        content: comment.content,
        ...(comment.userId === undefined ? {} : { userId: comment.userId }),
      }));

  return (
    <CommentList
      rows={rows}
      frozen={frozen}
      {...(onMuteUser === undefined ? {} : { onMuteUser })}
      height={watchCommentsHeight}
    />
  );
}
