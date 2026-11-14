// notificationKinds は、通知の種類と、その表示名の翻訳キー (設計書 6.2)。
export const notificationKinds = {
  RECORDING_STARTED: 'notifications.kind.RECORDING_STARTED',
  RECORDING_COMPLETED: 'notifications.kind.RECORDING_COMPLETED',
  RECORDING_FAILED: 'notifications.kind.RECORDING_FAILED',
  RESERVATION_CONFLICT: 'notifications.kind.RESERVATION_CONFLICT',
  STORAGE_LOW: 'notifications.kind.STORAGE_LOW',
  TUNER_SERVER_UNREACHABLE: 'notifications.kind.TUNER_SERVER_UNREACHABLE',
  WATCH_REMINDER: 'notifications.kind.WATCH_REMINDER',
  ANIME_RECORDED: 'notifications.kind.ANIME_RECORDED',
  ANIME_RECORD_FAILED: 'notifications.kind.ANIME_RECORD_FAILED',
  SCRAMBLED_RECORDINGS: 'notifications.kind.SCRAMBLED_RECORDINGS',
  LINEUP_CHANGED: 'notifications.kind.LINEUP_CHANGED',
} as const;

// personalKinds は、受け取るユーザーだけに届く、個人の通知の種類 (視聴予約と、見たアニメの記録)。全体向けの Webhook では送らない。
export const personalKinds: readonly NotificationKindName[] = [
  'WATCH_REMINDER',
  'ANIME_RECORDED',
  'ANIME_RECORD_FAILED',
];

// adminKinds は、管理者にだけ届く通知の種類 (#1517、#1519)。ほかのユーザーの通知設定には出さない。
export const adminKinds: readonly NotificationKindName[] = [
  'SCRAMBLED_RECORDINGS',
  'LINEUP_CHANGED',
];

export type NotificationKindName = keyof typeof notificationKinds;
