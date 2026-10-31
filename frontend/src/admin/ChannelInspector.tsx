import { Alert, Badge, Button, Group, Select, Stack, Table, Text } from '@mantine/core';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';

import { kindKey } from '../recordings/StreamQualityButton';
import { streamTypeName } from '../recordings/streamQuality';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';
import { mutate } from '../settings/mutate';
import { formatBitrate } from '../yunomi/streamMeter';
import type { ChannelInspectorMutation } from './__generated__/ChannelInspectorMutation.graphql';
import type { ChannelInspectorQuery } from './__generated__/ChannelInspectorQuery.graphql';
import { useAction } from './useAction';

const query = graphql`
  query ChannelInspectorQuery {
    services(includeHidden: true) {
      id
      channel
      channelType {
        type
        displayName
      }
    }
  }
`;

const inspectMutation = graphql`
  mutation ChannelInspectorMutation($channelType: String!, $channel: String!) {
    inspectChannel(channelType: $channelType, channel: $channel) {
      transportStreamId
      seconds
      totalBps
      programs {
        serviceId
        name
        bps
        streams {
          pid
          kind
          streamType
          bps
          drops
          errors
          scrambled
        }
      }
      others {
        pid
        kind
        bps
      }
    }
  }
`;

type Inspection = ChannelInspectorMutation['response']['inspectChannel'];

const hex = (n: number, digits: number) =>
  `0x${n.toString(16).toUpperCase().padStart(digits, '0')}`;

// ChannelInspector は、物理チャンネルの TS をまるごと数秒受け取り、TSID、サービスの一覧、サービスごとの ES とビットレートを
// 出す (#830、設計書 10.3)。マルチ編成で帯域が分かれる様子を見るのに使う。調べる間、チューナーを 1 本使うので、押したときだけ調べる。
export function ChannelInspector() {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const data = useLazyLoadQuery<ChannelInspectorQuery>(query, {});
  // チャンネルは、サービスの物理チャンネルを、チャンネル種別とチャンネルの組で重ねずに並べる。
  const channels = [
    ...new Map(
      data.services.map((service) => [
        `${service.channelType.type}/${service.channel}`,
        {
          value: `${service.channelType.type}/${service.channel}`,
          label: `${service.channelType.displayName} ${service.channel}`,
        },
      ]),
    ).values(),
  ];
  const [selected, setSelected] = useState<string | null>(channels[0]?.value ?? null);
  const [result, setResult] = useState<Inspection | null>(null);
  const [failed, setFailed] = useState(false);
  const { run, busy } = useAction();

  const inspect = () => {
    if (selected === null) {
      return;
    }
    const [channelType = '', channel = ''] = selected.split('/');
    setFailed(false);
    run(
      async () => {
        const response = await mutate<ChannelInspectorMutation>(environment, inspectMutation, {
          channelType,
          channel,
        });
        setResult(response.inspectChannel);
      },
      () => setFailed(true),
    );
  };

  return (
    <Stack gap="sm">
      <Text size="sm" c="dimmed">
        {t('admin.system.inspect.description')}
      </Text>
      <Group align="flex-end">
        <Select
          size="xs"
          label={t('admin.system.inspect.channel')}
          data={channels}
          value={selected}
          onChange={setSelected}
          allowDeselect={false}
          searchable
          maw={240}
        />
        <Button size="xs" onClick={inspect} loading={busy} disabled={selected === null}>
          {t('admin.system.inspect.run')}
        </Button>
      </Group>
      {failed && (
        <Alert color="red" role="alert">
          {t('admin.system.inspect.failed')}
        </Alert>
      )}
      {result !== null && <InspectionResult result={result} />}
    </Stack>
  );
}

function InspectionResult({ result }: { result: Inspection }) {
  const { t } = useTranslation();

  return (
    <Stack gap="xs">
      <Text size="sm">
        {t('admin.system.inspect.summary', {
          tsid: result.transportStreamId == null ? '-' : hex(result.transportStreamId, 4),
          bitrate: formatBitrate(result.totalBps),
          seconds: result.seconds.toFixed(1),
        })}
      </Text>
      <Table.ScrollContainer minWidth={640}>
        <Table
          fz="sm"
          verticalSpacing={4}
          styles={{ td: { whiteSpace: 'nowrap' }, th: { whiteSpace: 'nowrap' } }}
        >
          <Table.Thead>
            <Table.Tr>
              <Table.Th>PID</Table.Th>
              <Table.Th>{t('recordings.streamQuality.kindHeader')}</Table.Th>
              <Table.Th>{t('recordings.streamQuality.codec')}</Table.Th>
              <Table.Th ta="right">{t('recordings.streamQuality.bitrate')}</Table.Th>
              <Table.Th ta="right">{t('recordings.streamQuality.drops')}</Table.Th>
              <Table.Th ta="right">{t('recordings.streamQuality.scrambled')}</Table.Th>
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {result.programs.map((program) => [
              <Table.Tr key={`p${program.serviceId}`} fw={600}>
                <Table.Td colSpan={3}>
                  {t('admin.system.inspect.service', {
                    id: program.serviceId,
                    name: program.name ?? t('admin.system.inspect.unknownService'),
                  })}
                </Table.Td>
                <Table.Td ta="right">{formatBitrate(program.bps)}</Table.Td>
                <Table.Td colSpan={2} />
              </Table.Tr>,
              ...program.streams.map((stream) => (
                <Table.Tr key={`${program.serviceId}-${stream.pid}`}>
                  <Table.Td ff="monospace" pl="lg">
                    {hex(stream.pid, 4)}
                  </Table.Td>
                  <Table.Td>
                    <Badge size="sm" variant="light" color="gray" tt="none">
                      {t(kindKey(stream.kind))}
                    </Badge>
                  </Table.Td>
                  <Table.Td>{streamTypeName(stream.streamType ?? null) ?? '-'}</Table.Td>
                  <Table.Td ta="right">{formatBitrate(stream.bps)}</Table.Td>
                  <Table.Td ta="right" c={Number(stream.drops) > 0 ? 'amber.8' : undefined}>
                    {Number(stream.drops).toLocaleString('ja-JP')}
                  </Table.Td>
                  <Table.Td ta="right" c={Number(stream.scrambled) > 0 ? 'amber.8' : undefined}>
                    {Number(stream.scrambled).toLocaleString('ja-JP')}
                  </Table.Td>
                </Table.Tr>
              )),
            ])}
            <Table.Tr fw={600}>
              <Table.Td colSpan={3}>{t('admin.system.inspect.others')}</Table.Td>
              <Table.Td ta="right">
                {formatBitrate(result.others.reduce((sum, stream) => sum + stream.bps, 0))}
              </Table.Td>
              <Table.Td colSpan={2} />
            </Table.Tr>
          </Table.Tbody>
        </Table>
      </Table.ScrollContainer>
    </Stack>
  );
}
