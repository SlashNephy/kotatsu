import { notifications, notificationsStore, type NotificationData } from '@mantine/notifications';
import { useTranslation } from 'react-i18next';

import { settingsErrorMessage } from './errors';

// saveNoticeId は、設定を保存したことを伝える通知の ID (#1740)。どの設定の画面でも同じ ID を使い、続けて変えたときは
// 新しい通知を積まずに、出ている通知を差し替える。
export const saveNoticeId = 'settings-save';

// savedAutoClose は、保存した通知を閉じるまでの時間 (ミリ秒)。失敗の通知は、読めるよう、閉じるまで残す。
const savedAutoClose = 3_000;

// sequence は、差し替えるたびに、閉じるまでの時間を 1 ミリ秒ずらすための数。Mantine の通知は、内容を差し替えても
// 閉じるまでの時間を数え直さず、時間の値が変わったときだけ数え直すので、ずらして、差し替えた時から数え直させる。
let sequence = 0;

// showSaveNotice は、保存の通知を出すか、出ている通知を差し替える。
// Mantine の notifications.show は、同じ ID の通知が出ていると何もしないので、出ていれば update で差し替える。
function showSaveNotice(notice: { title?: string; message: string; ok: boolean }) {
  sequence += 1;
  const data: NotificationData = {
    id: saveNoticeId,
    title: notice.title,
    message: notice.message,
    color: notice.ok ? 'teal' : 'red',
    autoClose: notice.ok ? savedAutoClose + (sequence % 2) : false,
    // 保存した通知は読み上げを割り込ませず、失敗はすぐ読み上げる。
    role: notice.ok ? 'status' : 'alert',
  };
  const state = notificationsStore.getState();
  if ([...state.notifications, ...state.queue].some((item) => item.id === saveNoticeId)) {
    notifications.update(data);
  } else {
    notifications.show(data);
  }
}

// notifySaved は、設定を保存したと通知する。
export function notifySaved(message: string) {
  showSaveNotice({ message, ok: true });
}

// notifySaveFailed は、設定を保存できなかったと、理由 reason とともに通知する。
export function notifySaveFailed(title: string, reason: string) {
  showSaveNotice({ title, message: reason, ok: false });
}

// useSaveNotice は、設定の画面で、保存した通知と、保存できなかった通知を出す関数を返す。
// 通知は、利用者が設定の画面で変えたときにだけ出す。端末の設定のフックや、プレイヤーと共有するフックの中では出さない。
// プレイヤーの画面での変更や、ほかのタブでの変更 (storage イベント) にまで通知しないため。
export function useSaveNotice() {
  const { t } = useTranslation();

  return {
    saved: () => notifySaved(t('settings.autosave.saved')),
    failed: (error: unknown) =>
      notifySaveFailed(t('settings.autosave.failed'), settingsErrorMessage(t, error)),
  };
}
