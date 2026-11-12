import type { TFunction } from 'i18next';

// jobStates は、ジョブの画面 (#1279) のタブに並べる状態。River UI と同じく、動いているものから並べる。
export const jobStates = [
  'RUNNING',
  'AVAILABLE',
  'SCHEDULED',
  'RETRYABLE',
  'PENDING',
  'COMPLETED',
  'CANCELLED',
  'DISCARDED',
] as const;

export type JobStateName = (typeof jobStates)[number];

// kindKeys は、名前を付けたジョブの種類 (River の kind) と、設計書 6.1 の表の名前の翻訳のキー。
const kindKeys = {
  anime_import: 'admin.jobs.kinds.anime_import',
  anime_match: 'admin.jobs.kinds.anime_match',
  anime_record: 'admin.jobs.kinds.anime_record',
  anime_watch_progress_prune: 'admin.jobs.kinds.anime_watch_progress_prune',
  anime_wishlist_import: 'admin.jobs.kinds.anime_wishlist_import',
  audit_prune: 'admin.jobs.kinds.audit_prune',
  notification_dispatch: 'admin.jobs.kinds.notification_dispatch',
  notification_prune: 'admin.jobs.kinds.notification_prune',
  oauth_prune: 'admin.jobs.kinds.oauth_prune',
  recording_analyze: 'admin.jobs.kinds.recording_analyze',
  recording_chapters: 'admin.jobs.kinds.recording_chapters',
  recording_cm_detect: 'admin.jobs.kinds.recording_cm_detect',
  recording_join: 'admin.jobs.kinds.recording_join',
  recording_join_finalize: 'admin.jobs.kinds.recording_join_finalize',
  recording_transcode: 'admin.jobs.kinds.recording_transcode',
  session_prune: 'admin.jobs.kinds.session_prune',
  storage_capacity_check: 'admin.jobs.kinds.storage_capacity_check',
  thumbnail_generate: 'admin.jobs.kinds.thumbnail_generate',
  thumbnail_live: 'admin.jobs.kinds.thumbnail_live',
  timeshift_promote: 'admin.jobs.kinds.timeshift_promote',
  timeshift_promote_reap: 'admin.jobs.kinds.timeshift_promote_reap',
  timeshift_prune: 'admin.jobs.kinds.timeshift_prune',
  watch_notify: 'admin.jobs.kinds.watch_notify',
  web_push_deliver: 'admin.jobs.kinds.web_push_deliver',
  webhook_deliver: 'admin.jobs.kinds.webhook_deliver',
} as const;

// jobKindLabel は、ジョブの種類を、画面に出す名前にする。知らない種類は、そのまま返す。
export function jobKindLabel(t: TFunction, kind: string): string {
  return kind in kindKeys ? t(kindKeys[kind as keyof typeof kindKeys]) : kind;
}

// progressPercent は、ジョブの進み具合の割合 (0 から 1) を、百分率の整数にする (#1511)。
// 切り捨てて、書き終える前に 100% と出さない。
export function progressPercent(ratio: number): number {
  return Math.min(100, Math.max(0, Math.floor(ratio * 100)));
}

// remainingLabel は、ジョブの残り時間 (秒) を、画面に出す文にする (#1511)。null なら、まだ計算しているところとする。
// 分は切り上げ、1 時間を超えたら時間と分に分ける。
export function remainingLabel(t: TFunction, seconds: number | null | undefined): string {
  if (seconds == null) {
    return t('admin.jobs.remainingUnknown');
  }
  if (seconds < 60) {
    return t('admin.jobs.remainingUnderMinute');
  }
  const minutes = Math.ceil(seconds / 60);
  if (minutes < 60) {
    return t('admin.jobs.remainingMinutes', { minutes });
  }

  return t('admin.jobs.remainingHours', { hours: Math.floor(minutes / 60), minutes: minutes % 60 });
}
