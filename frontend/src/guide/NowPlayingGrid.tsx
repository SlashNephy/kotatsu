import { SimpleGrid } from '@mantine/core';

import { NowPlayingCard } from './NowPlayingCard';
import { PinnableCard } from './PinnableCard';
import type { NowPlayingRow } from './useNowPlayingTabs';

type Props = {
  rows: readonly NowPlayingRow[];
  // pinned は、ピン留めしたサービスの ID。
  pinned: readonly string[];
};

// NowPlayingGrid は、放送中の番組のカードの一覧。ホームとライブ視聴の一覧で使う。
export function NowPlayingGrid({ rows, pinned }: Props) {
  const pinnedSet = new Set(pinned);

  return (
    <SimpleGrid cols={{ base: 1, xs: 2, md: 3, lg: 4 }} spacing="xs">
      {rows.map((row) => (
        <PinnableCard
          key={row.service.id}
          serviceId={row.service.id}
          serviceName={row.service.name}
          pinned={pinnedSet.has(row.service.id)}
        >
          <NowPlayingCard
            serviceId={row.service.id}
            serviceName={row.service.name}
            serviceLogoUrl={row.service.logoUrl ?? null}
            program={row.programs[0] ?? null}
            force={row.service.jikkyo?.force ?? null}
          />
        </PinnableCard>
      ))}
    </SimpleGrid>
  );
}
