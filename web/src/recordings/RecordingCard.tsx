import { AspectRatio, Box, Card, Group, Progress, Stack, Text, Tooltip } from '@mantine/core';
import { TriangleAlert } from 'lucide-react';
import { Link } from '@tanstack/react-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { ProgramTitle } from '../futon';
import { ServiceLabel } from '../guide/ServiceLabel';
import { RecordingThumbnail } from './RecordingThumbnail';
import { usePreviewSlideshow } from './usePreviewSlideshow';
import { watchProgress } from './watchProgress';
import { RecordingStateBadge } from './RecordingStateBadge';

export type RecordingCardData = {
  id: string;
  name: string;
  startAt: string;
  endAt: string;
  state: string;
  thumbnailUrl?: string | null;
  // previewUrl は、シークバー用のプレビュー画像。サムネイルにホバーしている間、そのコマを順に見せる。
  previewUrl?: string | null;
  // recordedFrom、recordedUntil、watchPositionMs は、どこまで見たかを出すのに使う。録画中は recordedUntil が null。
  recordedFrom: string;
  recordedUntil?: string | null;
  // Int64 は Relay の型では any になるので、受け取ってから数に直す。
  watchPositionMs?: unknown;
  service: { name: string; logoUrl?: string | null };
  // drops、errors、scrambled は、受信の傷の数。録画の一覧でだけ渡す。Int64 なので、数に直して使う。
  drops?: unknown;
  errors?: unknown;
  scrambled?: unknown;
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
  const [hovered, setHovered] = useState(false);
  const slide = usePreviewSlideshow(recording.previewUrl, hovered);
  const startAt = Date.parse(recording.startAt);
  const sameYear = yearFormat.format(startAt) === yearFormat.format(Date.now());
  const tags = recording.tags ?? [];
  const quality = {
    drops: Number(recording.drops ?? 0),
    errors: Number(recording.errors ?? 0),
    scrambled: Number(recording.scrambled ?? 0),
  };
  const damaged = quality.drops > 0 || quality.errors > 0 || quality.scrambled > 0;
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
      <Box
        pos="relative"
        onPointerEnter={(event) => setHovered(event.pointerType === 'mouse')}
        onPointerLeave={() => setHovered(false)}
      >
        <AspectRatio ratio={16 / 9}>
          <RecordingThumbnail url={recording.thumbnailUrl} />
        </AspectRatio>
        {/* ホバーしている間は、プレビュー画像のコマを、代表画像の上に重ねて順に見せる。 */}
        {slide !== null && recording.previewUrl != null && (
          <Box
            aria-hidden
            pos="absolute"
            inset={0}
            style={{
              backgroundImage: `url(${JSON.stringify(recording.previewUrl)})`,
              backgroundRepeat: 'no-repeat',
              backgroundSize: slide.backgroundSize,
              backgroundPosition: slide.backgroundPosition,
            }}
          />
        )}
        {/* 再生したことがあれば、どこまで見たかを、サムネイルの下端に細いバーで示す。 */}
        {/* 見た目の手がかりなので、読み上げない。読ませると、カードのリンクの名前に割合の数字が混ざる。 */}
        {progress !== null && (
          <Progress
            value={progress * 100}
            size={4}
            radius={0}
            color="mikan"
            aria-hidden
            pos="absolute"
            bottom={0}
            left={0}
            right={0}
            styles={{ root: { background: 'rgb(0 0 0 / 0.4)' } }}
          />
        )}
      </Box>
      <Stack gap={0} p="xs">
        {/* 番組名は 2 行までにし、1 行でも 2 行分の高さを取って、並んだカードの日時やサービスの行を揃える。 */}
        <Text size="sm" lineClamp={2} mih="2lh">
          <ProgramTitle name={recording.name} />
        </Text>
        <Group gap={6} pt={4}>
          <Text size="xs" c="dimmed">
            {(sameYear ? dateTimeFormat : yearDateTimeFormat).format(startAt)} 〜{' '}
            {timeFormat.format(Date.parse(recording.endAt))}
          </Text>
          {/* 受信に傷があれば、警告の印を、日時のすぐ右に出す。数は、ツールチップと読み上げで示す。 */}
          {damaged && (
            <Tooltip label={t('recordings.qualityCounts', quality)}>
              <TriangleAlert
                size={14}
                color="var(--mantine-color-amber-6)"
                role="img"
                aria-label={t('recordings.damagedWith', quality)}
              />
            </Tooltip>
          )}
          {/* 完了は当たり前なので出さず、録画中や失敗のように目を留めたい状態だけを出す。 */}
          {/* ドロップありは、警告の印を出すときは、印で分かるので出さない。 */}
          {recording.state !== 'COMPLETED' &&
            !(recording.state === 'COMPLETED_WITH_ERRORS' && damaged) && (
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
