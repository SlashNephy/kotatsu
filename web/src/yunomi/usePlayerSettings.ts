import { useEffect } from 'react';
import {
  fetchQuery,
  graphql,
  useLazyLoadQuery,
  useMutation,
  useRelayEnvironment,
} from 'react-relay';

import { useDataChanges } from '../changes/DataChanges';
import type { usePlayerSettingsMutation } from './__generated__/usePlayerSettingsMutation.graphql';
import type { usePlayerSettingsQuery } from './__generated__/usePlayerSettingsQuery.graphql';

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
  // ほかの端末で変えたら、読み直す。購読はヘッダーの DataChangesSubscriber が 1 本だけ持つ (#503)。
  const changes = useDataChanges(['VIEWER_SETTINGS']);
  useEffect(() => {
    if (changes === 0) {
      return;
    }
    void fetchQuery<usePlayerSettingsQuery>(
      environment,
      settingsQuery,
      {},
      { fetchPolicy: 'network-only' },
    )
      .toPromise()
      .catch(() => {});
  }, [environment, changes]);

  const settings = data.playerSettings;
  const update = (input: Partial<Settings>) => {
    const next = { ...settings, ...input };
    commit({
      variables: { input },
      // 切り替えはすぐに画面に出し、失敗したら元に戻す。
      optimisticResponse: { updatePlayerSettings: next },
      // 続けて切り替えたときに、前の切り替えを上書きしないよう、変えた項目だけを書く。
      optimisticUpdater: (store) => {
        const record = store.getRoot().getLinkedRecord('playerSettings');
        if (input.jikkyoEnabled !== undefined) {
          record?.setValue(input.jikkyoEnabled, 'jikkyoEnabled');
        }
        if (input.captionsEnabled !== undefined) {
          record?.setValue(input.captionsEnabled, 'captionsEnabled');
        }
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
