// failureKeys は、録画の失敗の理由 (設計書 5.5) を、表示名の翻訳のキーにする。録画の詳細と、システムの状態の画面で使う。
const failureKeys = {
  TUNER_UNAVAILABLE: 'recordings.failure.TUNER_UNAVAILABLE',
  TUNER_SERVER_UNREACHABLE: 'recordings.failure.TUNER_SERVER_UNREACHABLE',
  STREAM_INTERRUPTED: 'recordings.failure.STREAM_INTERRUPTED',
  STORAGE_UNAVAILABLE: 'recordings.failure.STORAGE_UNAVAILABLE',
  STORAGE_FULL: 'recordings.failure.STORAGE_FULL',
  PROGRAM_NOT_FOUND: 'recordings.failure.PROGRAM_NOT_FOUND',
  CANCELLED: 'recordings.failure.CANCELLED',
  RECORDER_UNAVAILABLE: 'recordings.failure.RECORDER_UNAVAILABLE',
} as const;

// failureKey は、失敗の理由の表示名の翻訳のキーを返す。知らない理由 (新しいサーバー) には、汎用の文を返す。
export function failureKey(
  reason: string,
): (typeof failureKeys)[keyof typeof failureKeys] | 'recordings.failure.unknown' {
  return failureKeys[reason as keyof typeof failureKeys] ?? 'recordings.failure.unknown';
}
