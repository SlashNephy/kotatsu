import { Anchor, Card, Group, Stack, Text } from '@mantine/core';
import { Link } from '@tanstack/react-router';
import { BellOff } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { graphql } from 'react-relay';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';

import { useDataChanges } from '../changes/DataChanges';
import { EmptyState, ProgramTitle, zonedFormat } from '../futon';
import { broadcastDay, dateOf } from '../guide/layout';
import { ServiceLabel } from '../guide/ServiceLabel';
import { WatchReserveButton } from './WatchReserveButton';
import type { WatchReservationListQuery } from './__generated__/WatchReservationListQuery.graphql';

const dateTimeFormat = zonedFormat({
  month: 'numeric',
  day: 'numeric',
  weekday: 'short',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});
const timeFormat = zonedFormat({
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

// WatchReservationList は、ログイン中のユーザーの視聴予約の一覧 (設計書 4.8、10.3)。放送の早い順に並べ、ベルのボタンで外せる。
export function WatchReservationList() {
  const { t } = useTranslation();
  // ほかの端末で付け外ししたら、読み直す。
  const viewerChanges = useDataChanges(['VIEWER_SETTINGS']);
  const data = useLazyLoadQuery<WatchReservationListQuery>(
    graphql`
      query WatchReservationListQuery {
        watchReservations {
          notifyAt
          program {
            id
            name
            startAt
            endAt
            watchReserved
            service {
              name
              logoUrl
              channelType {
                type
              }
            }
          }
        }
      }
    `,
    {},
    { fetchKey: viewerChanges, fetchPolicy: 'store-and-network' },
  );
  // この画面で外した番組は、読み直すまで待たずに消す。
  const reservations = data.watchReservations.filter(({ program }) => program.watchReserved);

  return (
    <Stack gap="xs">
      {reservations.length === 0 ? (
        <EmptyState icon={BellOff} title={t('watch.empty')} hint={t('watch.emptyHint')} />
      ) : (
        reservations.map(({ program, notifyAt }) => {
          const start = Date.parse(program.startAt);

          return (
            <Card key={program.id} withBorder padding="xs">
              <Group gap="sm" wrap="nowrap" justify="space-between">
                <Stack gap={2} style={{ minWidth: 0 }}>
                  <Text size="sm" truncate>
                    <Anchor
                      inherit
                      renderRoot={(props) => (
                        <Link
                          {...props}
                          to="/guide"
                          search={{
                            date: dateOf(broadcastDay(start).from),
                            type: program.service.channelType.type,
                            program: program.id,
                          }}
                        />
                      )}
                    >
                      <ProgramTitle name={program.name} />
                    </Anchor>
                  </Text>
                  <Group gap="xs">
                    <Text size="xs" c="dimmed">
                      {t('range', {
                        from: dateTimeFormat.format(start),
                        to: timeFormat.format(Date.parse(program.endAt)),
                      })}
                    </Text>
                    <Text size="xs">
                      <ServiceLabel name={program.service.name} logoUrl={program.service.logoUrl} />
                    </Text>
                    <Text size="xs" c="dimmed">
                      {t('watch.notifyAt', { time: timeFormat.format(Date.parse(notifyAt)) })}
                    </Text>
                  </Group>
                </Stack>
                <WatchReserveButton programId={program.id} reserved={program.watchReserved} />
              </Group>
            </Card>
          );
        })
      )}
      <Anchor component={Link} to="/settings/notifications" size="xs">
        {t('watch.settingsLink')}
      </Anchor>
    </Stack>
  );
}
