import { useEffect } from 'react';
import { graphql, useMutation } from 'react-relay';

import { useDataChanges } from '../changes/DataChanges';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';
import type { PreferencesSyncMutation } from './__generated__/PreferencesSyncMutation.graphql';
import type { PreferencesSyncQuery } from './__generated__/PreferencesSyncQuery.graphql';
import { applyServerPreferences, registerPreferenceUploader } from './syncedPreferences';

// PreferencesSync は、端末をまたいで同期する設定を、サーバーと揃える (#1862)。ログインしている間に 1 つだけ置く。何も描かない。
// ほかの端末で変えたら、viewerSettingsChanged で知らされて読み直す。
export function PreferencesSync() {
  const changes = useDataChanges(['VIEWER_SETTINGS']);
  const data = useLazyLoadQuery<PreferencesSyncQuery>(
    graphql`
      query PreferencesSyncQuery {
        playerSettings {
          preferences {
            key
            value
          }
        }
      }
    `,
    {},
    { fetchKey: changes, fetchPolicy: 'store-and-network' },
  );
  const [commit] = useMutation<PreferencesSyncMutation>(graphql`
    mutation PreferencesSyncMutation($input: PlayerSettingsInput!) {
      updatePlayerSettings(input: $input) {
        preferences {
          key
          value
        }
      }
    }
  `);

  useEffect(
    () =>
      registerPreferenceUploader((preferences, done) => {
        commit({
          variables: { input: { preferences } },
          // 送れなくても、この端末の値はそのまま使う。次に変えたときか、次に開いたときに、もう一度揃える。
          onCompleted: done,
          onError: done,
        });
      }),
    [commit],
  );

  const preferences = data.playerSettings.preferences;
  useEffect(() => {
    let storage: Storage | null = null;
    try {
      storage = window.localStorage;
    } catch {
      return;
    }
    applyServerPreferences(preferences, storage);
  }, [preferences]);

  return null;
}
