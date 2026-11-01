import { Badge, Button, Modal, Table, Text } from '@mantine/core';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { formatBitrate } from '../yunomi/streamMeter';
import { packetBitrate, streamTypeName, totalQuality } from './streamQuality';

// StreamQuality は、PID ごとの品質の集計 (#473)。
export type StreamQuality = {
  readonly pid: number;
  readonly kind: string;
  readonly packets: number;
  readonly drops: number;
  readonly errors: number;
  readonly scrambled: number;
  // streamType は PMT の stream_type、componentTag はストリーム識別記述子の component_tag (#827)。分からなければ null。
  readonly streamType?: number | null;
  readonly componentTag?: number | null;
};

const kindKeys = {
  VIDEO: 'recordings.streamQuality.kind.VIDEO',
  AUDIO: 'recordings.streamQuality.kind.AUDIO',
  OTHER: 'recordings.streamQuality.kind.OTHER',
  PSI: 'recordings.streamQuality.kind.PSI',
  UNKNOWN: 'recordings.streamQuality.kind.UNKNOWN',
} as const;

export const kindKey = (kind: string) =>
  Object.hasOwn(kindKeys, kind) ? kindKeys[kind as keyof typeof kindKeys] : kindKeys.UNKNOWN;

const count = (n: number) => n.toLocaleString('ja-JP');

// bitrate は、パケットの数を録画の長さで割った平均のビットレートを書く。長さが分からなければ - にする。
const bitrate = (packets: number, seconds: number) => {
  const bps = packetBitrate(packets, seconds);

  return bps === null ? '-' : formatBitrate(bps);
};

// hex は、値を 0x10 のように 16 進で書く。
const hex = (n: number, digits: number) =>
  `0x${n.toString(16).toUpperCase().padStart(digits, '0')}`;

// StreamQualityButton は、PID ごとのドロップ、エラー、スクランブルの表を開くボタン (#473)。
// 映像のドロップか、データ放送のような見なくてよい PID のドロップかを見分けるのに使う。集計がなければ出さない。
// コーデックと component_tag、録画の長さ seconds (秒) で割った平均のビットレート、合計の行も出す (#827)。
export function StreamQualityButton({
  streams,
  seconds,
}: {
  streams: readonly StreamQuality[];
  seconds: number;
}) {
  const { t } = useTranslation();
  const [opened, setOpened] = useState(false);
  if (streams.length === 0) {
    return null;
  }
  const total = totalQuality(streams);

  return (
    <>
      <Button
        size="compact-xs"
        variant="subtle"
        onClick={() => setOpened(true)}
        w="fit-content"
        px={0}
      >
        {t('recordings.streamQuality.open')}
      </Button>
      <Modal
        opened={opened}
        onClose={() => setOpened(false)}
        title={t('recordings.streamQuality.title')}
        // 列を折り返さずに並べられる幅にする。狭い画面では、表を横へスクロールさせる。
        size={960}
      >
        <Text size="xs" c="dimmed" mb="xs">
          {t('recordings.streamQuality.description')}
        </Text>
        <Table.ScrollContainer minWidth={900}>
          {/* 列が多いので、折り返さずに横へスクロールさせる。 */}
          <Table fz="sm" verticalSpacing={4} striped style={{ whiteSpace: 'nowrap' }}>
            <Table.Thead>
              <Table.Tr>
                <Table.Th>PID</Table.Th>
                <Table.Th>{t('recordings.streamQuality.kindHeader')}</Table.Th>
                <Table.Th>{t('recordings.streamQuality.codec')}</Table.Th>
                <Table.Th>{t('recordings.streamQuality.componentTag')}</Table.Th>
                <Table.Th ta="right">{t('recordings.streamQuality.packets')}</Table.Th>
                <Table.Th ta="right">{t('recordings.streamQuality.bitrate')}</Table.Th>
                <Table.Th ta="right">{t('recordings.streamQuality.drops')}</Table.Th>
                <Table.Th ta="right">{t('recordings.streamQuality.errors')}</Table.Th>
                <Table.Th ta="right">{t('recordings.streamQuality.scrambled')}</Table.Th>
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {streams.map((stream) => (
                <Table.Tr key={stream.pid}>
                  <Table.Td ff="monospace">{hex(stream.pid, 4)}</Table.Td>
                  <Table.Td>
                    <Badge size="sm" variant="light" color="gray" tt="none">
                      {t(kindKey(stream.kind))}
                    </Badge>
                  </Table.Td>
                  <Table.Td>{streamTypeName(stream.streamType ?? null) ?? '-'}</Table.Td>
                  <Table.Td ff="monospace">
                    {stream.componentTag == null ? '-' : hex(stream.componentTag, 2)}
                  </Table.Td>
                  <Table.Td ta="right">{count(stream.packets)}</Table.Td>
                  <Table.Td ta="right">{bitrate(stream.packets, seconds)}</Table.Td>
                  <Table.Td ta="right" c={stream.drops > 0 ? 'amber.8' : undefined}>
                    {count(stream.drops)}
                  </Table.Td>
                  <Table.Td ta="right" c={stream.errors > 0 ? 'amber.8' : undefined}>
                    {count(stream.errors)}
                  </Table.Td>
                  <Table.Td ta="right" c={stream.scrambled > 0 ? 'amber.8' : undefined}>
                    {count(stream.scrambled)}
                  </Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
            <Table.Tfoot>
              <Table.Tr fw={600}>
                <Table.Td colSpan={4}>{t('recordings.streamQuality.total')}</Table.Td>
                <Table.Td ta="right">{count(total.packets)}</Table.Td>
                <Table.Td ta="right">{bitrate(total.packets, seconds)}</Table.Td>
                <Table.Td ta="right" c={total.drops > 0 ? 'amber.8' : undefined}>
                  {count(total.drops)}
                </Table.Td>
                <Table.Td ta="right" c={total.errors > 0 ? 'amber.8' : undefined}>
                  {count(total.errors)}
                </Table.Td>
                <Table.Td ta="right" c={total.scrambled > 0 ? 'amber.8' : undefined}>
                  {count(total.scrambled)}
                </Table.Td>
              </Table.Tr>
            </Table.Tfoot>
          </Table>
        </Table.ScrollContainer>
      </Modal>
    </>
  );
}
