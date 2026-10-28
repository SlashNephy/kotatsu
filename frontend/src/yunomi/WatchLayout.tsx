import { Group, Stack, Text, Title } from '@mantine/core';
import type { ReactNode } from 'react';

import { ServiceLabel } from '../guide/ServiceLabel';
import classes from './WatchLayout.module.css';

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
  // actions は、サービスの行の右端に並べる操作。
  actions?: ReactNode;
};

// WatchHeading は、映像のすぐ下に置く、番組名とサービスの行。
export function WatchHeading({ title, serviceName, serviceLogoUrl, actions }: HeadingProps) {
  return (
    <Stack gap={4}>
      <Title order={2} size="h3">
        {title}
      </Title>
      <Group justify="space-between" wrap="nowrap" gap="xs">
        <Text size="sm" fw={500} style={{ minWidth: 0 }}>
          <ServiceLabel name={serviceName} logoUrl={serviceLogoUrl} />
        </Text>
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
