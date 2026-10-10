// 録画の状態 (設計書 5.5) の、画面での表し方。

// recordingStates は、絞り込みに出す録画の状態。
export const recordingStates = [
  'RECORDING',
  'COMPLETED',
  'COMPLETED_WITH_ERRORS',
  'PARTIAL',
  'FAILED',
] as const;

const stateKeys = {
  RECORDING: 'recordings.state.RECORDING',
  COMPLETED: 'recordings.state.COMPLETED',
  COMPLETED_WITH_ERRORS: 'recordings.state.COMPLETED_WITH_ERRORS',
  PARTIAL: 'recordings.state.PARTIAL',
  FAILED: 'recordings.state.FAILED',
} as const;

// stateKey は、状態の表示名の翻訳キーを返す。知らない状態は、完了として扱わず、失敗の表示にする。
export function stateKey(state: string): (typeof stateKeys)[keyof typeof stateKeys] {
  return stateKeys[state as keyof typeof stateKeys] ?? stateKeys.FAILED;
}

// stateColor は、状態の色を返す。赤は、録画の失敗のように問題が起きたときだけ使う (設計書 10.2)。
// 一部が欠けた録画は、警告の琥珀色にする。
export function stateColor(state: string): string {
  switch (state) {
    case 'RECORDING':
      return 'mikan';
    case 'COMPLETED':
      return 'gray';
    case 'COMPLETED_WITH_ERRORS':
    case 'PARTIAL':
      return 'amber';
    default:
      return 'red';
  }
}
