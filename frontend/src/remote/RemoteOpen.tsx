import { Button, Loader, SegmentedControl, Select, Stack, Text, TextInput } from '@mantine/core';
import { Search } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { fetchQuery, graphql, useRelayEnvironment } from 'react-relay';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';

import { ProgramTitle, toHalfWidth, zonedFormat } from '../futon';
import type { RemoteOpenRecordingsQuery } from './__generated__/RemoteOpenRecordingsQuery.graphql';
import type { RemoteOpenServicesQuery } from './__generated__/RemoteOpenServicesQuery.graphql';

// RemoteOpenTarget は、ほかの端末のプレイヤーに開かせるもの。PlayerCommandInput の OPEN に渡す。
export type RemoteOpenTarget = { serviceId: string } | { recordingId: string };

// maxRecordings は、録画を探したときに出す数。
const maxRecordings = 10;

const startFormat = zonedFormat({
  month: 'numeric',
  day: 'numeric',
  weekday: 'short',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

type Recordings = RemoteOpenRecordingsQuery['response']['recordings']['edges'];

// RemoteOpen は、ほかの端末のプレイヤーに、別のチャンネルか録画を開かせる欄 (キャスト、#802、設計書 7.7)。
// チャンネルは一覧から選び、録画は番組名で探して選ぶ。
export function RemoteOpen({ onOpen }: { onOpen: (target: RemoteOpenTarget) => void }) {
  const { t } = useTranslation();
  const [kind, setKind] = useState<'live' | 'recording'>('live');

  return (
    <Stack gap="xs">
      <Text size="sm" fw={600}>
        {t('remote.open.title')}
      </Text>
      <SegmentedControl
        fullWidth
        size="xs"
        value={kind}
        onChange={(value) => setKind(value as 'live' | 'recording')}
        data={[
          { value: 'live', label: t('remote.open.live') },
          { value: 'recording', label: t('remote.open.recording') },
        ]}
      />
      {kind === 'live' ? <ServicePicker onOpen={onOpen} /> : <RecordingPicker onOpen={onOpen} />}
    </Stack>
  );
}

// ServicePicker は、チャンネルを選んで開かせる。チャンネル種別ごとにまとめる。
function ServicePicker({ onOpen }: { onOpen: (target: RemoteOpenTarget) => void }) {
  const { t } = useTranslation();
  const data = useLazyLoadQuery<RemoteOpenServicesQuery>(
    graphql`
      query RemoteOpenServicesQuery {
        services {
          id
          name
          channelType {
            displayName
          }
        }
      }
    `,
    {},
  );
  const groups = new Map<string, { value: string; label: string }[]>();
  for (const service of data.services) {
    const group = groups.get(service.channelType.displayName) ?? [];
    group.push({ value: service.id, label: toHalfWidth(service.name) });
    groups.set(service.channelType.displayName, group);
  }

  return (
    <Select
      searchable
      placeholder={t('remote.open.servicePlaceholder')}
      aria-label={t('remote.open.live')}
      value={null}
      data={[...groups].map(([group, items]) => ({ group, items }))}
      onChange={(serviceId) => {
        if (serviceId !== null) {
          onOpen({ serviceId });
        }
      }}
    />
  );
}

// RecordingPicker は、録画を番組名や概要で探し、選んで開かせる。
function RecordingPicker({ onOpen }: { onOpen: (target: RemoteOpenTarget) => void }) {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const [keyword, setKeyword] = useState('');
  const [results, setResults] = useState<Recordings | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const search = () => {
    setLoading(true);
    setError(false);
    fetchQuery<RemoteOpenRecordingsQuery>(
      environment,
      graphql`
        query RemoteOpenRecordingsQuery($keyword: String, $first: Int!) {
          recordings(first: $first, filter: { keyword: $keyword }) {
            edges {
              node {
                id
                name
                startAt
                service {
                  name
                }
              }
            }
          }
        }
      `,
      { keyword: keyword.trim() === '' ? null : keyword.trim(), first: maxRecordings },
      { fetchPolicy: 'network-only' },
    )
      .toPromise()
      .then(
        (data) => setResults(data?.recordings.edges ?? []),
        () => setError(true),
      )
      .finally(() => setLoading(false));
  };

  return (
    <Stack gap="xs">
      <form
        onSubmit={(event) => {
          event.preventDefault();
          search();
        }}
      >
        <TextInput
          value={keyword}
          onChange={(event) => setKeyword(event.currentTarget.value)}
          placeholder={t('remote.open.recordingPlaceholder')}
          aria-label={t('remote.open.recordingPlaceholder')}
          rightSection={loading ? <Loader size="xs" /> : <Search size={16} aria-hidden />}
        />
      </form>
      {error && (
        <Text size="sm" c="red" role="alert">
          {t('remote.open.searchError')}
        </Text>
      )}
      {results !== null && results.length === 0 && (
        <Text size="sm" c="dimmed">
          {t('remote.open.noRecordings')}
        </Text>
      )}
      {results?.map(({ node }) => (
        <Button
          key={node.id}
          variant="default"
          justify="flex-start"
          h="auto"
          py={6}
          onClick={() => onOpen({ recordingId: node.id })}
        >
          <Stack gap={0} align="flex-start">
            <Text size="sm" fw={600} style={{ whiteSpace: 'normal', textAlign: 'left' }}>
              <ProgramTitle name={node.name} />
            </Text>
            <Text size="xs" c="dimmed">
              {startFormat.format(Date.parse(node.startAt))} {toHalfWidth(node.service.name)}
            </Text>
          </Stack>
        </Button>
      ))}
    </Stack>
  );
}
