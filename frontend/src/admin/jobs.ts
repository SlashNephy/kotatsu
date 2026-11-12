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
