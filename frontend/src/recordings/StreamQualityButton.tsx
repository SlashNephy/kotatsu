import { Badge, Button, Modal, Table, Text } from '@mantine/core';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

// StreamQuality は、PID ごとの品質の集計 (#473)。
export type StreamQuality = {
  readonly pid: number;
  readonly kind: string;
  readonly packets: number;
  readonly drops: number;
  readonly errors: number;
  readonly scrambled: number;
};

const kindKeys = {
  VIDEO: 'recordings.streamQuality.kind.VIDEO',
  AUDIO: 'recordings.streamQuality.kind.AUDIO',
  OTHER: 'recordings.streamQuality.kind.OTHER',
  PSI: 'recordings.streamQuality.kind.PSI',
  UNKNOWN: 'recordings.streamQuality.kind.UNKNOWN',
} as const;

const kindKey = (kind: string) =>
  Object.hasOwn(kindKeys, kind) ? kindKeys[kind as keyof typeof kindKeys] : kindKeys.UNKNOWN;

const count = (n: number) => n.toLocaleString('ja-JP');

// StreamQualityButton は、PID ごとのドロップ、エラー、スクランブルの表を開くボタン (#473)。
// 映像のドロップか、データ放送のような見なくてよい PID のドロップかを見分けるのに使う。集計がなければ出さない。
export function StreamQualityButton({ streams }: { streams: readonly StreamQuality[] }) {
  const { t } = useTranslation();
  const [opened, setOpened] = useState(false);
  if (streams.length === 0) {
    return null;
  }

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
        size="lg"
      >
        <Text size="xs" c="dimmed" mb="xs">
          {t('recordings.streamQuality.description')}
        </Text>
        <Table.ScrollContainer minWidth={480}>
          <Table fz="sm" verticalSpacing={4} striped>
            <Table.Thead>
              <Table.Tr>
                <Table.Th>PID</Table.Th>
                <Table.Th>{t('recordings.streamQuality.kindHeader')}</Table.Th>
                <Table.Th ta="right">{t('recordings.streamQuality.packets')}</Table.Th>
                <Table.Th ta="right">{t('recordings.streamQuality.drops')}</Table.Th>
                <Table.Th ta="right">{t('recordings.streamQuality.errors')}</Table.Th>
                <Table.Th ta="right">{t('recordings.streamQuality.scrambled')}</Table.Th>
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {streams.map((stream) => (
                <Table.Tr key={stream.pid}>
                  <Table.Td ff="monospace">
                    0x{stream.pid.toString(16).toUpperCase().padStart(4, '0')}
                  </Table.Td>
                  <Table.Td>
                    <Badge size="sm" variant="light" color="gray" tt="none">
                      {t(kindKey(stream.kind))}
                    </Badge>
                  </Table.Td>
                  <Table.Td ta="right">{count(stream.packets)}</Table.Td>
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
          </Table>
        </Table.ScrollContainer>
      </Modal>
    </>
  );
}
