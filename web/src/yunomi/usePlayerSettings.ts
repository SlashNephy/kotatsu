import { useEffect, useRef } from 'react';
import {
  fetchQuery,
  graphql,
  useLazyLoadQuery,
  useMutation,
  useRelayEnvironment,
} from 'react-relay';
import { isKeywordMuteMatch } from '../jikkyo/comments';

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
      captions {
        superimposeLive
        superimposeRecorded
        stroke
        backgroundOpacity
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

// CaptionSettings は、字幕の見た目と、文字スーパーを出すかどうかの設定 (#491)。
export type CaptionSettings = {
  superimposeLive: boolean;
  superimposeRecorded: boolean;
  stroke: boolean;
  // backgroundOpacity は、背景の不透明度 (0〜1)。null なら、放送の指定どおり。
  backgroundOpacity: number | null;
};

// usePlayerSettings は、端末をまたいで同期するプレイヤーの設定を返す (設計書 9.1)。
// ほかの端末で変えたら、viewerSettingsChanged で知らされて読み直す。
export function usePlayerSettings(): Settings & {
  setJikkyoEnabled: (enabled: boolean) => void;
  setCaptionsEnabled: (enabled: boolean) => void;
  comments: CommentSettings;
  // setComments は、実況のコメントの設定を置き換える。失敗したら onError を呼ぶ。
  setComments: (comments: CommentSettingsInput, onError?: () => void) => void;
  // muteUser は、ユーザー ID をミュートの一覧に足す (#767)。足してあれば何もしない。
  muteUser: (userId: string) => void;
  captions: CaptionSettings;
  // setCaptions は、字幕の設定を置き換える。失敗したら onError を呼ぶ。
  setCaptions: (captions: CaptionSettings, onError?: () => void) => void;
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
        captions {
          superimposeLive
          superimposeRecorded
          stroke
          backgroundOpacity
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
    const next = {
      ...settings,
      ...input,
      comments: settings.comments,
      captions: settings.captions,
    };
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

  const setCaptions = (captions: CaptionSettings, onError?: () => void) => {
    commit({
      variables: { input: { captions } },
      // 変えた設定はすぐに画面に出し、失敗したら元に戻す。
      optimisticResponse: { updatePlayerSettings: { ...settings, captions } },
      onError: () => onError?.(),
      updater: (store) => {
        const updated = store.getRootField('updatePlayerSettings');
        if (updated !== null) {
          store.getRoot().setLinkedRecord(updated, 'playerSettings');
        }
      },
    });
  };

  const muteUser = (userId: string) => {
    const comments = settings.comments;
    if (comments.mutedUsers.includes(userId)) {
      return;
    }
    setComments({
      opacity: comments.opacity,
      speed: comments.speed,
      scale: comments.scale,
      showAll: comments.showAll,
      mutedKeywords: comments.mutedKeywords.flatMap(({ text, match }) =>
        isKeywordMuteMatch(match) ? [{ text, match }] : [],
      ),
      mutedUsers: [...comments.mutedUsers, userId],
    });
  };

  return {
    jikkyoEnabled: settings.jikkyoEnabled,
    captionsEnabled: settings.captionsEnabled,
    setJikkyoEnabled: (enabled) => update({ jikkyoEnabled: enabled }),
    setCaptionsEnabled: (enabled) => update({ captionsEnabled: enabled }),
    comments: settings.comments,
    setComments,
    muteUser,
    captions: {
      ...settings.captions,
      backgroundOpacity: settings.captions.backgroundOpacity ?? null,
    },
    setCaptions,
  };
}
