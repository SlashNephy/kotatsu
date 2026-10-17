import { useEffect, useRef } from 'react';
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
import type { CommentSettings, KeywordMuteMatch } from '../jikkyo/comments';

const settingsQuery = graphql`
  query usePlayerSettingsQuery {
    playerSettings {
      jikkyoEnabled
      captionsEnabled
      comments {
        opacity
        speed
        scale
        showAll
        mutedKeywords {
          text
          match
        }
        mutedUsers
      }
    }
  }
`;

type Settings = { jikkyoEnabled: boolean; captionsEnabled: boolean };

// CommentSettingsInput は、実況のコメントの設定の変更。まるごと置き換える (#488)。
export type CommentSettingsInput = {
  opacity: number;
  speed: number;
  scale: number;
  showAll: boolean;
  mutedKeywords: { text: string; match: KeywordMuteMatch }[];
  mutedUsers: string[];
};

// usePlayerSettings は、端末をまたいで同期するプレイヤーの設定を返す (設計書 9.1)。
// ほかの端末で変えたら、viewerSettingsChanged で知らされて読み直す。
export function usePlayerSettings(): Settings & {
  setJikkyoEnabled: (enabled: boolean) => void;
  setCaptionsEnabled: (enabled: boolean) => void;
  comments: CommentSettings;
  // setComments は、実況のコメントの設定を置き換える。失敗したら onError を呼ぶ。
  setComments: (comments: CommentSettingsInput, onError?: () => void) => void;
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
        comments {
          opacity
          speed
          scale
          showAll
          mutedKeywords {
            text
            match
          }
          mutedUsers
        }
      }
    }
  `);
  // ほかの端末で変えたら、読み直す。購読はヘッダーの DataChangesSubscriber が 1 本だけ持つ (#503)。
  // 開いたときは useLazyLoadQuery が読むので、開いた後に変わったときだけ読み直す。
  const changes = useDataChanges(['VIEWER_SETTINGS']);
  const seen = useRef(changes);
  useEffect(() => {
    if (changes === seen.current) {
      return;
    }
    seen.current = changes;
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
    const next = { ...settings, ...input, comments: settings.comments };
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

  const setComments = (comments: CommentSettingsInput, onError?: () => void) => {
    commit({
      variables: { input: { comments } },
      // 変えた設定はすぐに画面に出し、失敗したら元に戻す。
      optimisticResponse: { updatePlayerSettings: { ...settings, comments } },
      onError: () => onError?.(),
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
    comments: settings.comments,
    setComments,
  };
}
