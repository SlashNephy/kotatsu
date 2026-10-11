// notificationKinds は、通知の種類と、その表示名の翻訳キー (設計書 6.2)。
export const notificationKinds = {
  RECORDING_STARTED: 'notifications.kind.RECORDING_STARTED',
  RECORDING_COMPLETED: 'notifications.kind.RECORDING_COMPLETED',
  RECORDING_FAILED: 'notifications.kind.RECORDING_FAILED',
  RESERVATION_CONFLICT: 'notifications.kind.RESERVATION_CONFLICT',
  STORAGE_LOW: 'notifications.kind.STORAGE_LOW',
  TUNER_SERVER_UNREACHABLE: 'notifications.kind.TUNER_SERVER_UNREACHABLE',
} as const;

export type NotificationKindName = keyof typeof notificationKinds;
