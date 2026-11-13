import { Group, Spoiler, Stack, Text, Title } from '@mantine/core';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

import { LinkifiedText, toHalfWidth, zonedFormat } from '../futon';
import { ServiceLabel } from '../guide/ServiceLabel';
import classes from './WatchLayout.module.css';

const dateTimeFormat = zonedFormat({
  month: 'numeric',
  day: 'numeric',
  weekday: 'short',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});
// 今年でない番組は、年も書く。
const yearDateTimeFormat = zonedFormat({
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
  weekday: 'short',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});
const yearFormat = zonedFormat({ year: 'numeric' });
const timeFormat = zonedFormat({
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

// BroadcastTime は、番組の放送の時間。endUndecided なら、終了時刻の代わりに未定と書く。
export type BroadcastTime = { startAt: string; endAt: string; endUndecided?: boolean };

type Props = {
  player: ReactNode;
  // info は、映像の下に置く番組の情報と、映像に付く操作 (字幕の履歴など)。
  info: ReactNode;
  // side は、右の列の中身 (実況や、ほかの局の一覧)。出すものがなければ null にし、映像を本文の幅いっぱいに広げる。
  side: ReactNode | null;
};

// WatchLayout は、ライブ視聴と録画の再生の画面の並び。YouTube と同じく、映像の下に番組の情報を、右の列に実況と一覧を置く。
// 狭い画面では、右の列の中身を、番組の情報の下に並べる。
export function WatchLayout({ player, info, side }: Props) {
  return (
    <div className={classes.layout} data-side={side === null ? undefined : true}>
      <div className={classes.player}>{player}</div>
      <Stack gap="xs" className={classes.info}>
        {info}
      </Stack>
      {side !== null && (
        <Stack gap="md" className={classes.side}>
          {side}
        </Stack>
      )}
    </div>
  );
}

type HeadingProps = {
  // title は、番組名。見出しにする。
  title: ReactNode;
  serviceName: string;
  serviceLogoUrl: string | null | undefined;
  // time は、放送の時間。サービスの名前に続けて出す。番組が分からなければ null。
  time: BroadcastTime | null;
  // actions は、サービスの行の右端に並べる操作。
  actions?: ReactNode;
};

// WatchHeading は、映像のすぐ下に置く、番組名とサービスの行。
export function WatchHeading({ title, serviceName, serviceLogoUrl, time, actions }: HeadingProps) {
  const { t } = useTranslation();
  const range = (time: BroadcastTime) => {
    const startAt = Date.parse(time.startAt);
    const sameYear = yearFormat.format(startAt) === yearFormat.format(Date.now());

    return t('range', {
      from: (sameYear ? dateTimeFormat : yearDateTimeFormat).format(startAt),
      to: time.endUndecided
        ? t('live.overlay.endUndecided')
        : timeFormat.format(Date.parse(time.endAt)),
    });
  };

  return (
    <Stack gap={4}>
      <Title order={2} size="h3">
        {title}
      </Title>
      <Group justify="space-between" wrap="nowrap" gap="xs">
        <Group gap="xs" wrap="wrap" style={{ minWidth: 0 }}>
          <Text size="sm" fw={500}>
            <ServiceLabel name={serviceName} logoUrl={serviceLogoUrl} />
          </Text>
          {time !== null && (
            <Text size="sm" c="dimmed">
              {range(time)}
            </Text>
          )}
        </Group>
        {actions !== undefined && (
          <Group gap="xs" wrap="nowrap">
            {actions}
          </Group>
        )}
      </Group>
    </Stack>
  );
}

// watchCommentsHeight は、右の列に置く実況のコメントの一覧の高さ。画面の幅で変わる。
export const watchCommentsHeight = 'var(--watch-comments-height)';

// descriptionMaxHeight は、番組の説明を畳んでおく高さ (px)。3 行ほど出し、長ければ「もっと見る」で開く。
const descriptionMaxHeight = 66;

// WatchDescription は、番組名とサービスの行の下に置く、番組の説明 (#1266)。空なら何も出さない。
// extended (番組の詳細。番組表の詳細と同じ見出しと本文の組) があれば、説明の後に続ける (#1646)。畳んだ高さは説明と共有する。
export function WatchDescription({
  text,
  extended = [],
}: {
  text: string;
  extended?: ReadonlyArray<{ readonly heading: string; readonly text: string }>;
}) {
  const { t } = useTranslation();
  const items = extended.filter((item) => item.heading.trim() !== '' || item.text.trim() !== '');
  if (text.trim() === '' && items.length === 0) {
    return null;
  }

  return (
    <Spoiler
      maxHeight={descriptionMaxHeight}
      showLabel={t('player.description.show')}
      hideLabel={t('player.description.hide')}
    >
      <Stack gap="xs">
        {text.trim() !== '' && (
          <Text size="sm" c="dimmed" style={{ whiteSpace: 'pre-wrap' }}>
            <LinkifiedText text={toHalfWidth(text)} />
          </Text>
        )}
        {items.map((item, index) => (
          <Stack key={index} gap={2}>
            {item.heading.trim() !== '' && (
              <Text size="sm" c="dimmed" fw={700}>
                {toHalfWidth(item.heading)}
              </Text>
            )}
            <Text size="sm" c="dimmed" style={{ whiteSpace: 'pre-wrap' }}>
              <LinkifiedText text={toHalfWidth(item.text)} />
            </Text>
          </Stack>
        ))}
      </Stack>
    </Spoiler>
  );
}
