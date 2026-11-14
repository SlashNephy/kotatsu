// lineupAlertWindowMs は、衛星の局の顔ぶれの変化 (チャンネルの変更、#1519) のお知らせを、画面の上に出しておく最長の長さ (設計書 付録 A)。
// 管理者がシステムの状態の画面を開くまで出し、開いたら消す。
export const lineupAlertWindowMs = 7 * 24 * 60 * 60 * 1000;

// recentLineupAlerts は、notifications のうち、now から lineupAlertWindowMs より前に起きたものを除く。
export function recentLineupAlerts<T extends { readonly occurredAt: string }>(
  notifications: readonly T[],
  now: number,
): T[] {
  return notifications.filter(
    (notification) => now - Date.parse(notification.occurredAt) <= lineupAlertWindowMs,
  );
}
