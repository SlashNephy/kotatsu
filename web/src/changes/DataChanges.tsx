import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useReducer,
  useState,
  type ReactNode,
} from 'react';
import { graphql, useSubscription } from 'react-relay';

import type { DataChangesSubscription } from './__generated__/DataChangesSubscription.graphql';
import {
  applyChange,
  changeKey,
  initialChangeState,
  type ChangeChannel,
  type ChangeState,
} from './dataChanges';

type Context = {
  state: ChangeState;
  dispatch: (change: { channel: ChangeChannel; version: number }) => void;
  // unread は、未読の通知の数 (設計書 6.2)。分からない間 (つなぐ前や切れたとき) は null。
  unread: number | null;
  setUnread: (unread: number | null) => void;
};

const DataChangesContext = createContext<Context>({
  state: initialChangeState,
  dispatch: () => {},
  unread: null,
  setUnread: () => {},
});

// DataChangesProvider は、データの変更を、画面に配る (#503)。購読は DataChangesSubscriber が行う。
// ログインしてから購読するので、購読とは分けて、画面全体を包む。
export function DataChangesProvider({ children }: { children: ReactNode }) {
  const [state, dispatch] = useReducer(applyChange, initialChangeState);
  const [unread, setUnread] = useState<number | null>(null);
  const value = useMemo(() => ({ state, dispatch, unread, setUnread }), [state, unread]);

  return <DataChangesContext.Provider value={value}>{children}</DataChangesContext.Provider>;
}

// useDataChangeDispatch は、届いた変更を DataChangesProvider に渡す関数を返す。
export function useDataChangeDispatch(): Context['dispatch'] {
  return useContext(DataChangesContext).dispatch;
}

// useUnreadNotificationCount は、未読の通知の数を返す。分からない間は null。
export function useUnreadNotificationCount(): number | null {
  return useContext(DataChangesContext).unread;
}

// DataChangesSubscriber は、ログイン中のユーザーへの知らせを購読し、DataChangesProvider に渡す。ログインしている間だけ置く。
// データの変更、ログイン中のユーザーの設定 (通知の設定、番組表ビュー、プレイヤーの設定、第二要素、パスキー、セッション) の版数、
// 未読の通知の数を、1 つの接続でまとめて受け取る。HTTP/1.1 では、ブラウザが同じホストに同時に開く接続は 6 本までなので、
// 映像やデータ放送の接続のために、購読の接続の数を抑える。
export function DataChangesSubscriber() {
  const { dispatch, setUnread } = useContext(DataChangesContext);
  const config = useMemo(
    () => ({
      subscription: graphql`
        subscription DataChangesSubscription {
          viewerEvents {
            dataChange {
              channel
              version
            }
            viewerSettingsVersion
            unreadNotificationCount
          }
        }
      `,
      variables: {},
      onNext: (response: DataChangesSubscription['response'] | null | undefined) => {
        const event = response?.viewerEvents;
        if (!event) {
          return;
        }
        const change = event.dataChange;
        // 新しい種類がサーバーに足されても、知らない種類は無視する。
        if (change && change.channel !== '%future added value') {
          dispatch({ channel: change.channel, version: change.version });
        }
        if (event.viewerSettingsVersion != null) {
          dispatch({ channel: 'VIEWER_SETTINGS', version: event.viewerSettingsVersion });
        }
        if (event.unreadNotificationCount != null) {
          setUnread(event.unreadNotificationCount);
        }
      },
      // 切れても、画面は今の内容のまま使える。つなぎ直したときに、切れている間の変更を数える。未読の数は分からなくなる。
      onError: () => setUnread(null),
    }),
    [dispatch, setUnread],
  );
  useSubscription<DataChangesSubscription>(config);

  return null;
}

// settleMs は、変更が続けて届いたときに、読み直すのを待つ時間。EPG の同期などで続けて届いても、まとめて 1 回だけ読み直す。
export const settleMs = 2000;

// useDataChanges は、channels のどれかが変わるたびに変わる値を返す。useLazyLoadQuery の fetchKey に混ぜて、画面を読み直す。
// 変わってから settleMs の間、次の変更が届かなくなってから値を変える。
export function useDataChanges(channels: readonly ChangeChannel[]): number {
  const { state } = useContext(DataChangesContext);
  const key = changeKey(state, channels);
  const [settled, setSettled] = useState(key);
  useEffect(() => {
    if (key === settled) {
      return;
    }
    const timer = setTimeout(() => setSettled(key), settleMs);

    return () => clearTimeout(timer);
  }, [key, settled]);

  return settled;
}
