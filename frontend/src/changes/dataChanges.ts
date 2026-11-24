// サーバーが dataChanged で通知するデータの変更を、画面が読み直す合図にする (#503、設計書 9.1)。

// DataChannel は、変更を通知するデータの種類。GraphQL の DataChannel と同じ。
export type DataChannel =
  | 'EPG'
  | 'RULES'
  | 'RESERVATIONS'
  | 'ASSIGNMENTS'
  | 'RESERVATION_CONFLICTS'
  | 'TUNERS'
  | 'RECORDINGS'
  | 'ANIME_FOLLOWING'
  | 'DRAMA_WATCHLIST'
  | 'ADMIN';

// ChangeChannel は、画面が読み直す合図にする変更の種類。dataChanged で知らされる DataChannel と、
// viewerSettingsChanged で知らされる、ログイン中のユーザーの設定 (VIEWER_SETTINGS)。
export type ChangeChannel = DataChannel | 'VIEWER_SETTINGS';

// ChangeState は、種類ごとの、届いたバージョン (known) と、購読を始めてから変わった回数 (bumps)。
// 画面は bumps を読み直しの合図に使う。最初に届くバージョンは、今のバージョンを通知するだけなので、数えない。
export type ChangeState = {
  known: Partial<Record<ChangeChannel, number>>;
  bumps: Partial<Record<ChangeChannel, number>>;
};

export const initialChangeState: ChangeState = { known: {}, bumps: {} };

// applyChange は、届いた変更を state に反映した state を返す。バージョンが上がっていなければ、同じ state を返す。
// 購読をつなぎ直すと、すべての種類の今のバージョンがまた届く。切れている間に変わっていれば、バージョンが上がっているので数える。
export function applyChange(
  state: ChangeState,
  change: { channel: ChangeChannel; version: number },
): ChangeState {
  const known = state.known[change.channel];
  if (known !== undefined && change.version <= known) {
    return state;
  }
  const bumps =
    known === undefined
      ? state.bumps
      : { ...state.bumps, [change.channel]: (state.bumps[change.channel] ?? 0) + 1 };

  return { known: { ...state.known, [change.channel]: change.version }, bumps };
}

// changeKey は、channels のどれかが変わるたびに変わる値を返す。useLazyLoadQuery の fetchKey に混ぜて使う。
export function changeKey(state: ChangeState, channels: readonly ChangeChannel[]): number {
  return channels.reduce((sum, channel) => sum + (state.bumps[channel] ?? 0), 0);
}
