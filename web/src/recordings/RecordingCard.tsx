import { AspectRatio, Box, Card, Group, Progress, Stack, Text } from '@mantine/core';
import { Link } from '@tanstack/react-router';
import { useTranslation } from 'react-i18next';

import { ProgramTitle } from '../futon';
import { ServiceLabel } from '../guide/ServiceLabel';
import { RecordingThumbnail } from './RecordingThumbnail';
import { watchProgress } from './watchProgress';
import { RecordingStateBadge } from './RecordingStateBadge';

export type RecordingCardData = {
  id: string;
  name: string;
  startAt: string;
  endAt: string;
  state: string;
  thumbnailUrl?: string | null;
  // recordedFrom、recordedUntil、watchPositionMs は、どこまで見たかを出すのに使う。録画中は recordedUntil が null。
  recordedFrom: string;
  recordedUntil?: string | null;
  // Int64 は Relay の型では any になるので、受け取ってから数に直す。
  watchPositionMs?: unknown;
  service: { name: string; logoUrl?: string | null };
  // tags は、録画の一覧でだけ渡す。
  tags?: readonly { id: string; name: string }[];
};

const dateTimeFormat = new Intl.DateTimeFormat('ja-JP', {
  month: 'numeric',
  day: 'numeric',
  weekday: 'short',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
  timeZone: 'Asia/Tokyo',
});
// 今年でない録画は、年も書く。
const yearDateTimeFormat = new Intl.DateTimeFormat('ja-JP', {
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
  weekday: 'short',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
  timeZone: 'Asia/Tokyo',
});
const yearFormat = new Intl.DateTimeFormat('ja-JP', { year: 'numeric', timeZone: 'Asia/Tokyo' });
const timeFormat = new Intl.DateTimeFormat('ja-JP', {
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
  timeZone: 'Asia/Tokyo',
});

// RecordingCard は、サムネイル付きの録画のカード。押すと、録画の詳細を開く。ホームと録画の一覧で使う。
// 行の間は、放送中の番組のカードと揃える (番組名と放送時間は 4px、サービスとの間は xs)。
export function RecordingCard({ recording }: { recording: RecordingCardData }) {
  const { t } = useTranslation();
  const startAt = Date.parse(recording.startAt);
  const sameYear = yearFormat.format(startAt) === yearFormat.format(Date.now());
  const tags = recording.tags ?? [];
  const duration =
    ((recording.recordedUntil == null ? Date.now() : Date.parse(recording.recordedUntil)) -
      Date.parse(recording.recordedFrom)) /
    1000;
  const progress = watchProgress(
    recording.watchPositionMs == null ? null : Number(recording.watchPositionMs),
    duration,
  );

  return (
    <Card
      withBorder
      padding={0}
      renderRoot={(props) => (
        <Link {...props} to="/recordings/$recordingId" params={{ recordingId: recording.id }} />
      )}
    >
      <Box pos="relative">
        <AspectRatio ratio={16 / 9}>
          <RecordingThumbnail url={recording.thumbnailUrl} />
        </AspectRatio>
        {/* 再生したことがあれば、どこまで見たかを、サムネイルの下端に細いバーで示す。 */}
        {progress !== null && (
          <Progress
            value={progress * 100}
            size={4}
            radius={0}
            color="mikan"
            aria-label={t('recordings.watched', { percent: Math.round(progress * 100) })}
            pos="absolute"
            bottom={0}
            left={0}
            right={0}
            styles={{ root: { background: 'rgb(0 0 0 / 0.4)' } }}
          />
        )}
      </Box>
      <Stack gap={0} p="xs">
        <Text size="sm" lineClamp={2}>
          <ProgramTitle name={recording.name} />
        </Text>
        <Group gap={6} pt={4}>
          <Text size="xs" c="dimmed">
            {(sameYear ? dateTimeFormat : yearDateTimeFormat).format(startAt)} 〜{' '}
            {timeFormat.format(Date.parse(recording.endAt))}
          </Text>
          {/* 完了は当たり前なので出さず、録画中や失敗のように目を留めたい状態だけを出す。 */}
          {recording.state !== 'COMPLETED' && (
            <RecordingStateBadge state={recording.state} size="xs" />
          )}
        </Group>
        <Text size="xs" c="dimmed" mt="xs">
          <ServiceLabel name={recording.service.name} logoUrl={recording.service.logoUrl} />
        </Text>
        {/* タグは、ハッシュタグの形で並べる。 */}
        {tags.length > 0 && (
          <Group gap={8} mt="xs">
            {tags.map((tag) => (
              <Text key={tag.id} span size="xs" c="dimmed">
                #{tag.name}
              </Text>
            ))}
          </Group>
        )}
      </Stack>
    </Card>
  );
}
