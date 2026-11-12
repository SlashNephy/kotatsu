import { Alert, Anchor, Button, Card, Group, Stack, Text } from '@mantine/core';
import { Link } from '@tanstack/react-router';
import { CopyCheck } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useMutation } from 'react-relay';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';

import { useDataChanges } from '../changes/DataChanges';
import { EmptyState, ProgramTitle, zonedFormat } from '../futon';
import { broadcastDay, dateOf } from '../guide/layout';
import { ServiceLabel } from '../guide/ServiceLabel';
import type { SkippedDuplicateListQuery } from './__generated__/SkippedDuplicateListQuery.graphql';
import type { SkippedDuplicateListReserveMutation } from './__generated__/SkippedDuplicateListReserveMutation.graphql';

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
const dateFormat = zonedFormat({ year: 'numeric', month: 'numeric', day: 'numeric' });

// SkippedDuplicateList は、ルールに一致したが、重複の回避、同じ回の束ね、前の回を録らない設定で予約しなかった番組の一覧
// (設計書 4.4、#469、#1498、#1499)。録り直したい番組は、ここから手動で予約する。手動の予約は、これらの対象にならない。
export function SkippedDuplicateList() {
  const { t } = useTranslation();
  // 予約やルールが変わったら、読み直す。
  const changes = useDataChanges(['RESERVATIONS', 'RULES']);
  const [fetchKey, setFetchKey] = useState(0);
  const data = useLazyLoadQuery<SkippedDuplicateListQuery>(
    graphql`
      query SkippedDuplicateListQuery {
        skippedDuplicates {
          program {
            id
            name
            startAt
            endAt
            service {
              name
              logoUrl
              channelType {
                type
              }
            }
          }
          rules {
            id
            name
          }
          reason
          lastRecordedAt
          recorded {
            service {
              name
            }
            endAt
          }
          instead {
            startAt
            service {
              name
            }
          }
          laterEpisode
        }
      }
    `,
    {},
    { fetchKey: `${changes}:${fetchKey}`, fetchPolicy: 'store-and-network' },
  );
  const [reserve, reserving] = useMutation<SkippedDuplicateListReserveMutation>(graphql`
    mutation SkippedDuplicateListReserveMutation($program: ID!) {
      createProgramReservation(input: { program: $program }) {
        id
      }
    }
  `);
  const [failed, setFailed] = useState(false);
  const [reservingId, setReservingId] = useState<string | null>(null);

  const onReserve = (programId: string) => {
    setFailed(false);
    setReservingId(programId);
    reserve({
      variables: { program: programId },
      onCompleted: (_, errors) => {
        setReservingId(null);
        if (errors != null && errors.length > 0) {
          setFailed(true);

          return;
        }
        // 予約した番組は、一覧から外れるので読み直す。
        setFetchKey((key) => key + 1);
      },
      onError: () => {
        setReservingId(null);
        setFailed(true);
      },
    });
  };

  return (
    <Stack gap="xs">
      <Text size="xs" c="dimmed">
        {t('reservations.skipped.description')}
      </Text>
      {failed && (
        <Alert color="red" role="alert" withCloseButton onClose={() => setFailed(false)}>
          {t('reservations.skipped.reserveError')}
        </Alert>
      )}
      {data.skippedDuplicates.length === 0 ? (
        <EmptyState icon={CopyCheck} title={t('reservations.skipped.empty')} />
      ) : (
        data.skippedDuplicates.map(
          ({ program, rules, reason, lastRecordedAt, recorded, instead, laterEpisode }) => {
            const start = Date.parse(program.startAt);
            const ruleNames = rules.map((rule) => rule.name).join('、');
            let reasonText: string;
            if (reason === 'RECORDED_EPISODE' && recorded != null) {
              reasonText = t('reservations.skipped.reasonRecordedEpisode', {
                rules: ruleNames,
                service: recorded.service?.name ?? t('reservations.skipped.unknownService'),
                date: dateFormat.format(Date.parse(recorded.endAt)),
              });
            } else if (reason === 'EARLIER_EPISODE' && laterEpisode != null) {
              reasonText = t('reservations.skipped.reasonEarlierEpisode', {
                rules: ruleNames,
                episode: laterEpisode,
              });
            } else if (reason === 'OTHER_BROADCAST') {
              reasonText =
                instead == null
                  ? t('reservations.skipped.reasonOtherBroadcastUnknown', { rules: ruleNames })
                  : t('reservations.skipped.reasonOtherBroadcast', {
                      rules: ruleNames,
                      service: instead.service.name,
                      date: dateTimeFormat.format(Date.parse(instead.startAt)),
                    });
            } else {
              reasonText = t('reservations.skipped.reason', {
                rules: ruleNames,
                date: lastRecordedAt == null ? '' : dateFormat.format(Date.parse(lastRecordedAt)),
              });
            }

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
                        <ServiceLabel
                          name={program.service.name}
                          logoUrl={program.service.logoUrl}
                        />
                      </Text>
                    </Group>
                    <Text size="xs" c="dimmed">
                      {reasonText}
                    </Text>
                  </Stack>
                  <Button
                    size="xs"
                    variant="default"
                    flex="none"
                    loading={reserving && reservingId === program.id}
                    disabled={reserving && reservingId !== program.id}
                    onClick={() => onReserve(program.id)}
                  >
                    {t('reservations.skipped.reserve')}
                  </Button>
                </Group>
              </Card>
            );
          },
        )
      )}
    </Stack>
  );
}
