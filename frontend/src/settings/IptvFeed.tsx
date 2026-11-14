import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, type useRelayEnvironment } from 'react-relay';

import type { IptvFeedCreateMutation } from './__generated__/IptvFeedCreateMutation.graphql';
import type { IptvFeedDeleteMutation } from './__generated__/IptvFeedDeleteMutation.graphql';
import { FeedCard, type FeedItem, type IssuedUrl } from './FeedCard';
import { mutate } from './mutate';
import type { useReauthGate } from './useReauthGate';

type Props = {
  // feed は、発行した IPTV の配信の URL の記録。発行していなければ null。
  feed: FeedItem | null | undefined;
  // enabled は、管理者が IPTV のクライアント用の URL を有効にしているかどうか (#1755)。
  enabled: boolean;
  onChanged: () => void;
  run: ReturnType<typeof useReauthGate>['run'];
  fail: (error: unknown) => void;
  environment: ReturnType<typeof useRelayEnvironment>;
};

// IptvFeed は、ライブ視聴の一覧 (M3U) と番組表 (XMLTV) を IPTV のクライアントに配信する URL (#1324、設計書 9.8) の
// 発行、発行し直し、取り消し。予約のカレンダーとは別の URL にする。URL を持つ人はそのユーザーとしてライブ視聴を見られるので、
// 発行は再認証してから行う。URL は、発行した直後にだけ出す。
// 管理者が無効にしている間は発行させない (#1755)。発行済みの URL があれば、使えないことを示し、取り消しだけを許す。
export function IptvFeed({ feed, enabled, onChanged, run, fail, environment }: Props) {
  const { t } = useTranslation();
  const [issued, setIssued] = useState<IssuedUrl[] | null>(null);

  const create = () =>
    run(async () => {
      const response = await mutate<IptvFeedCreateMutation>(
        environment,
        graphql`
          mutation IptvFeedCreateMutation {
            createIptvFeed {
              playlistPath
              guidePath
            }
          }
        `,
        {},
      );
      const origin = window.location.origin;
      setIssued([
        {
          label: t('settings.account.iptvPlaylist'),
          url: new URL(response.createIptvFeed.playlistPath, origin).toString(),
        },
        {
          label: t('settings.account.iptvGuide'),
          url: new URL(response.createIptvFeed.guidePath, origin).toString(),
        },
      ]);
      onChanged();
    }, fail);

  const revoke = () =>
    run(async () => {
      await mutate<IptvFeedDeleteMutation>(
        environment,
        graphql`
          mutation IptvFeedDeleteMutation {
            deleteIptvFeed
          }
        `,
        {},
      );
      // 取り消した URL を出したままにしない。
      setIssued(null);
      onChanged();
    }, fail);

  if (!enabled && feed == null && issued === null) {
    return null;
  }

  return (
    <FeedCard
      title={t('settings.account.iptvFeed')}
      description={t('settings.account.iptvFeedDescription')}
      issuedMessage={t('settings.account.iptvFeedIssued')}
      emptyMessage={t('settings.account.noIptvFeed')}
      createLabel={t('settings.account.createIptvFeed')}
      feed={feed}
      issued={issued}
      disabledMessage={enabled ? undefined : t('settings.account.iptvFeedDisabled')}
      onCloseIssued={() => setIssued(null)}
      onCreate={create}
      onRevoke={revoke}
    />
  );
}
