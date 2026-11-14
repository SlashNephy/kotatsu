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

// dismissed は、システムの状態の画面で既読にした知らせ。読み直しを待たずに、画面の上のお知らせから外す。
const dismissed = new Set<string>();
const listeners = new Set<() => void>();
let version = 0;

// dismissLineupAlerts は、知らせ ids を既読にしたことを覚え、お知らせから外す。
export function dismissLineupAlerts(ids: readonly string[]) {
  for (const id of ids) {
    dismissed.add(id);
  }
  version += 1;
  for (const listener of listeners) {
    listener();
  }
}

// isLineupAlertDismissed は、知らせ id を、システムの状態の画面で既読にしたか。
export function isLineupAlertDismissed(id: string): boolean {
  return dismissed.has(id);
}

// subscribeLineupDismissals は、既読にした知らせが増えたら listener を呼ぶ。useSyncExternalStore に渡す。
export function subscribeLineupDismissals(listener: () => void): () => void {
  listeners.add(listener);

  return () => {
    listeners.delete(listener);
  };
}

// lineupDismissalsVersion は、既読にした知らせが増えるたびに変わる値。useSyncExternalStore に渡す。
export function lineupDismissalsVersion(): number {
  return version;
}
