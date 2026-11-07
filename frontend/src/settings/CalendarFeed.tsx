import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, type useRelayEnvironment } from 'react-relay';

import type { CalendarFeedCreateMutation } from './__generated__/CalendarFeedCreateMutation.graphql';
import type { CalendarFeedDeleteMutation } from './__generated__/CalendarFeedDeleteMutation.graphql';
import { FeedCard, type FeedItem, type IssuedUrl } from './FeedCard';
import { mutate } from './mutate';
import type { useReauthGate } from './useReauthGate';

export type CalendarFeedItem = FeedItem;

type Props = {
  // feed は、発行した購読用の URL の記録。発行していなければ null。
  feed: CalendarFeedItem | null | undefined;
  onChanged: () => void;
  run: ReturnType<typeof useReauthGate>['run'];
  fail: (error: unknown) => void;
  environment: ReturnType<typeof useRelayEnvironment>;
};

// CalendarFeed は、予約を iCalendar で購読する URL (#1255、設計書 9.8) の発行、発行し直し、取り消し。
// URL を持つ人はそのユーザーとして予約を読めるので、発行は再認証してから行う。URL は、発行した直後にだけ出す。
export function CalendarFeed({ feed, onChanged, run, fail, environment }: Props) {
  const { t } = useTranslation();
  const [issued, setIssued] = useState<IssuedUrl[] | null>(null);

  const create = () =>
    run(async () => {
      const response = await mutate<CalendarFeedCreateMutation>(
        environment,
        graphql`
          mutation CalendarFeedCreateMutation {
            createCalendarFeed {
              path
            }
          }
        `,
        {},
      );
      setIssued([
        { url: new URL(response.createCalendarFeed.path, window.location.origin).toString() },
      ]);
      onChanged();
    }, fail);

  const revoke = () =>
    run(async () => {
      await mutate<CalendarFeedDeleteMutation>(
        environment,
        graphql`
          mutation CalendarFeedDeleteMutation {
            deleteCalendarFeed
          }
        `,
        {},
      );
      // 取り消した URL を出したままにしない。
      setIssued(null);
      onChanged();
    }, fail);

  return (
    <FeedCard
      title={t('settings.account.calendarFeed')}
      description={t('settings.account.calendarFeedDescription')}
      issuedMessage={t('settings.account.calendarFeedIssued')}
      emptyMessage={t('settings.account.noCalendarFeed')}
      createLabel={t('settings.account.createCalendarFeed')}
      feed={feed}
      issued={issued}
      onCloseIssued={() => setIssued(null)}
      onCreate={create}
      onRevoke={revoke}
    />
  );
}
