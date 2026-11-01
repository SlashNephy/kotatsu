import { Anchor, Table, Text } from '@mantine/core';
import { Link } from '@tanstack/react-router';
import { startTransition, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql } from 'react-relay';

import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';
import { formatBitrate } from '../yunomi/streamMeter';
import type { ActiveRecordingsQuery } from './__generated__/ActiveRecordingsQuery.graphql';

// refreshMs は、読み直す間隔。mikan は、録画中の様子を 10 秒ごとに書く (#829)。
const refreshMs = 10_000;

const query = graphql`
  query ActiveRecordingsQuery {
    recordings(first: 50, filter: { states: [RECORDING] }) {
      edges {
        node {
          id
          name
          drops
          errors
          scrambled
          service {
            name
          }
          liveStats {
            bitrate
            drops
            errors
            scrambled
          }
        }
      }
    }
  }
`;

const count = (n: number) => n.toLocaleString('ja-JP');

// ActiveRecordings は、システムの状態の画面の、録画中の録画の一覧 (#829、設計書 5.3)。直近のビットレートと、
// ドロップ、エラー、スクランブルの数を、10 秒ごとに読み直して出す。数は、閉じたセグメントと書いているセグメントの合計。
export function ActiveRecordings() {
  const { t } = useTranslation();
  const [fetchKey, setFetchKey] = useState(0);
  useEffect(() => {
    const timer = setInterval(
      () => startTransition(() => setFetchKey((key) => key + 1)),
      refreshMs,
    );

    return () => clearInterval(timer);
  }, []);
  const data = useLazyLoadQuery<ActiveRecordingsQuery>(
    query,
    {},
    { fetchKey, fetchPolicy: 'store-and-network' },
  );
  const recordings = data.recordings.edges.map((edge) => edge.node);
  if (recordings.length === 0) {
    return (
      <Text size="sm" c="dimmed">
        {t('admin.system.noActiveRecordings')}
      </Text>
    );
  }

  return (
    <Table.ScrollContainer minWidth={560}>
      <Table
        fz="sm"
        verticalSpacing={4}
        styles={{ td: { whiteSpace: 'nowrap' }, th: { whiteSpace: 'nowrap' } }}
      >
        <Table.Thead>
          <Table.Tr>
            <Table.Th>{t('admin.system.activeRecordingName')}</Table.Th>
            <Table.Th>{t('admin.system.activeRecordingService')}</Table.Th>
            <Table.Th ta="right">{t('admin.system.activeRecordingBitrate')}</Table.Th>
            <Table.Th ta="right">{t('recordings.streamQuality.drops')}</Table.Th>
            <Table.Th ta="right">{t('recordings.streamQuality.errors')}</Table.Th>
            <Table.Th ta="right">{t('recordings.streamQuality.scrambled')}</Table.Th>
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          {recordings.map((recording) => {
            const live = recording.liveStats;
            // Int64 は Relay の型では any になるので、数に直す。
            const drops = Number(recording.drops) + Number(live?.drops ?? 0);
            const errors = Number(recording.errors) + Number(live?.errors ?? 0);
            const scrambled = Number(recording.scrambled) + Number(live?.scrambled ?? 0);

            return (
              <Table.Tr key={recording.id}>
                <Table.Td>
                  <Anchor
                    size="sm"
                    renderRoot={(props) => (
                      <Link
                        {...props}
                        to="/recordings/$recordingId"
                        params={{ recordingId: recording.id }}
                      />
                    )}
                  >
                    {recording.name}
                  </Anchor>
                </Table.Td>
                <Table.Td>{recording.service?.name ?? '-'}</Table.Td>
                <Table.Td ta="right">
                  {live == null ? '-' : formatBitrate(Number(live.bitrate))}
                </Table.Td>
                <Table.Td ta="right" c={drops > 0 ? 'amber.8' : undefined}>
                  {count(drops)}
                </Table.Td>
                <Table.Td ta="right" c={errors > 0 ? 'amber.8' : undefined}>
                  {count(errors)}
                </Table.Td>
                <Table.Td ta="right" c={scrambled > 0 ? 'amber.8' : undefined}>
                  {count(scrambled)}
                </Table.Td>
              </Table.Tr>
            );
          })}
        </Table.Tbody>
      </Table>
    </Table.ScrollContainer>
  );
}
