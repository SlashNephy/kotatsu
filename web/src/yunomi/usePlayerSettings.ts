import { useMemo } from 'react';
import {
  fetchQuery,
  graphql,
  useLazyLoadQuery,
  useMutation,
  useRelayEnvironment,
  useSubscription,
} from 'react-relay';

import type { usePlayerSettingsMutation } from './__generated__/usePlayerSettingsMutation.graphql';
import type { usePlayerSettingsQuery } from './__generated__/usePlayerSettingsQuery.graphql';
import type { usePlayerSettingsSubscription } from './__generated__/usePlayerSettingsSubscription.graphql';

const settingsQuery = graphql`
  query usePlayerSettingsQuery {
    playerSettings {
      jikkyoEnabled
      captionsEnabled
    }
  }
`;

type Settings = { jikkyoEnabled: boolean; captionsEnabled: boolean };

// usePlayerSettings は、端末をまたいで同期するプレイヤーの設定を返す (設計書 9.1)。
// ほかの端末で変えたら、viewerSettingsChanged で知らされて読み直す。
export function usePlayerSettings(): Settings & {
  setJikkyoEnabled: (enabled: boolean) => void;
  setCaptionsEnabled: (enabled: boolean) => void;
} {
  const environment = useRelayEnvironment();
  const data = useLazyLoadQuery<usePlayerSettingsQuery>(
    settingsQuery,
    {},
    { fetchPolicy: 'store-or-network' },
  );
  const [commit] = useMutation<usePlayerSettingsMutation>(graphql`
    mutation usePlayerSettingsMutation($input: PlayerSettingsInput!) {
      updatePlayerSettings(input: $input) {
        jikkyoEnabled
        captionsEnabled
      }
    }
  `);
  const subscription = useMemo(
    () => ({
      subscription: graphql`
        subscription usePlayerSettingsSubscription {
          viewerSettingsChanged
        }
      `,
      variables: {},
      // 購読を始めたときにも今の版数が届くが、読み直しても同じなので、気にせず読み直す。
      onNext: () => {
        void fetchQuery<usePlayerSettingsQuery>(
          environment,
          settingsQuery,
          {},
          { fetchPolicy: 'network-only' },
        )
          .toPromise()
          .catch(() => {});
      },
    }),
    [environment],
  );
  useSubscription<usePlayerSettingsSubscription>(subscription);

  const settings = data.playerSettings;
  const update = (input: Partial<Settings>) => {
    const next = { ...settings, ...input };
    commit({
      variables: { input },
      // 切り替えはすぐに画面に出し、失敗したら元に戻す。
      optimisticResponse: { updatePlayerSettings: next },
      optimisticUpdater: (store) => {
        const record = store.getRoot().getLinkedRecord('playerSettings');
        record?.setValue(next.jikkyoEnabled, 'jikkyoEnabled');
        record?.setValue(next.captionsEnabled, 'captionsEnabled');
      },
      updater: (store) => {
        const updated = store.getRootField('updatePlayerSettings');
        if (updated !== null) {
          store.getRoot().setLinkedRecord(updated, 'playerSettings');
        }
      },
    });
  };

  return {
    jikkyoEnabled: settings.jikkyoEnabled,
    captionsEnabled: settings.captionsEnabled,
    setJikkyoEnabled: (enabled) => update({ jikkyoEnabled: enabled }),
    setCaptionsEnabled: (enabled) => update({ captionsEnabled: enabled }),
  };
}
