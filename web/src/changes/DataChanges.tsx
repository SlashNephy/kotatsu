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
import type { DataChangesViewerSettingsSubscription } from './__generated__/DataChangesViewerSettingsSubscription.graphql';
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
};

const DataChangesContext = createContext<Context>({
  state: initialChangeState,
  dispatch: () => {},
});

// DataChangesProvider は、データの変更を、画面に配る (#503)。購読は DataChangesSubscriber が行う。
// ログインしてから購読するので、購読とは分けて、画面全体を包む。
export function DataChangesProvider({ children }: { children: ReactNode }) {
  const [state, dispatch] = useReducer(applyChange, initialChangeState);
  const value = useMemo(() => ({ state, dispatch }), [state]);

  return <DataChangesContext.Provider value={value}>{children}</DataChangesContext.Provider>;
}

// useDataChangeDispatch は、届いた変更を DataChangesProvider に渡す関数を返す。
export function useDataChangeDispatch(): Context['dispatch'] {
  return useContext(DataChangesContext).dispatch;
}

// DataChangesSubscriber は、dataChanged を購読し、届いた変更を DataChangesProvider に渡す。ログインしている間だけ置く。
export function DataChangesSubscriber() {
  const dispatch = useDataChangeDispatch();
  const config = useMemo(
    () => ({
      subscription: graphql`
        subscription DataChangesSubscription {
          dataChanged {
            channel
            version
          }
        }
      `,
      variables: {},
      onNext: (response: DataChangesSubscription['response'] | null | undefined) => {
        const change = response?.dataChanged;
        // 新しい種類がサーバーに足されても、知らない種類は無視する。
        if (change && change.channel !== '%future added value') {
          dispatch({ channel: change.channel, version: change.version });
        }
      },
      // 切れても、画面は今の内容のまま使える。つなぎ直したときに、切れている間の変更を数える。
      onError: () => {},
    }),
    [dispatch],
  );
  useSubscription<DataChangesSubscription>(config);
  // ログイン中のユーザーの設定 (通知の設定、番組表ビュー、プレイヤーの設定、第二要素、パスキー、セッション) も、
  // ほかの端末で変えたら読み直す。
  const viewerConfig = useMemo(
    () => ({
      subscription: graphql`
        subscription DataChangesViewerSettingsSubscription {
          viewerSettingsChanged
        }
      `,
      variables: {},
      onNext: (response: DataChangesViewerSettingsSubscription['response'] | null | undefined) => {
        const version = response?.viewerSettingsChanged;
        if (version !== undefined) {
          dispatch({ channel: 'VIEWER_SETTINGS', version });
        }
      },
      onError: () => {},
    }),
    [dispatch],
  );
  useSubscription<DataChangesViewerSettingsSubscription>(viewerConfig);

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
