import {
  Alert,
  Anchor,
  Box,
  Group,
  SegmentedControl,
  SimpleGrid,
  Stack,
  Title,
} from '@mantine/core';
import { Tv } from 'lucide-react';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Suspense, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useLazyLoadQuery } from 'react-relay';

import { EmptyState, ErrorBoundary, PageLoader } from '../../futon';
import { isUnauthenticated } from '../../relay/environment';
import type { liveServicesQuery } from './__generated__/liveServicesQuery.graphql';
import { NowPlayingCard } from '../../guide/NowPlayingCard';
import { PinButton } from '../../guide/PinButton';
import { sharesWithListed } from '../../guide/sharedEvent';

export const Route = createFileRoute('/live/')({
  component: LivePage,
});

// LivePage は、ライブ視聴のチャンネルの一覧 (設計書 10.3)。
function LivePage() {
  const { t } = useTranslation();
  // 放送中の番組を出すので、画面を開いた時刻から 1 分間の時間帯を引く。
  // 読み込みの間に子は描き直されて state を失うので、時間帯は、待たない親で決める。
  const [range] = useState(() => {
    const now = Date.now();

    return { from: new Date(now).toISOString(), to: new Date(now + 60_000).toISOString() };
  });

  return (
    <Stack gap="sm">
      <Title order={2}>{t('live.title')}</Title>
      <ErrorBoundary
        fallback={(error) =>
          isUnauthenticated(error) ? (
            <Alert color="amber">
              <Anchor component={Link} to="/login">
                {t('guide.signedOut')}
              </Anchor>
            </Alert>
          ) : (
            <Alert color="amber">{t('viewer.error')}</Alert>
          )
        }
      >
        <Suspense fallback={<PageLoader />}>
          <Services range={range} />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

// pinnedTab は、ピン留めしたサービスを出すタブの値。チャンネル種別とぶつからないよう、小文字にする。
const pinnedTab = 'pinned';

function Services({ range }: { range: { from: string; to: string } }) {
  const { t } = useTranslation();
  const data = useLazyLoadQuery<liveServicesQuery>(
    graphql`
      query liveServicesQuery($from: Time!, $to: Time!) {
        channelTypes {
          type
          displayName
        }
        pinnedServices {
          id
        }
        guide(from: $from, to: $to) {
          service {
            id
            name
            logoUrl
            channelType {
              type
            }
            jikkyo {
              force
            }
          }
          programs {
            name
            startAt
            endAt
            sharedFrom {
              id
            }
          }
        }
      }
    `,
    range,
  );
  const types = data.channelTypes.filter((type) =>
    data.guide.some((row) => row.service.channelType.type === type.type),
  );
  // pinned は、ピン留めしたサービスの ID を、ピン留めした順に並べたもの (#485)。
  const pinned = data.pinnedServices.map((service) => service.id);
  const pinnedSet = new Set(pinned);
  // ピン留めしたサービスがあれば、ピン留めのタブから出す。
  const [selectedType, setType] = useState(pinned.length > 0 ? pinnedTab : (types[0]?.type ?? ''));
  // ピン留めをすべて外したら、ピン留めのタブはなくなるので、最初の種別に戻す。後でまたピン留めしても、ピン留めのタブへは戻さない。
  const fallBack = selectedType === pinnedTab && pinned.length === 0;
  useEffect(() => {
    if (fallBack) {
      setType(types[0]?.type ?? '');
    }
  }, [fallBack, types]);
  const type = fallBack ? (types[0]?.type ?? '') : selectedType;
  // order は、並べる順。勢いの順では、実況の勢いが強い局から並べる (#490)。勢いの分からない局は後ろに回す。
  const [order, setOrder] = useState<'guide' | 'force'>('guide');
  // サブチャンネルがメインチャンネルと同じ番組を流しているときは、メインチャンネルだけを出す (ホームの放送中の番組と同じ)。
  // ピン留めのタブでは、ピン留めしたサービスをそのまま出す。
  const listed = new Set(data.guide.map((row) => row.service.id));
  const filtered =
    type === pinnedTab
      ? pinned.flatMap((id) => data.guide.filter((row) => row.service.id === id))
      : data.guide.filter(
          (row) =>
            row.service.channelType.type === type && !sharesWithListed(row.programs[0], listed),
        );
  const shown =
    order === 'force'
      ? [...filtered].sort(
          (a, b) => (b.service.jikkyo?.force ?? -1) - (a.service.jikkyo?.force ?? -1),
        )
      : filtered;

  return (
    <Stack>
      <Group justify="space-between" gap="xs">
        {(types.length > 1 || pinned.length > 0) && (
          <SegmentedControl
            size="xs"
            value={type}
            onChange={setType}
            data={[
              ...(pinned.length > 0 ? [{ value: pinnedTab, label: t('pin.tab') }] : []),
              ...types.map((channelType) => ({
                value: channelType.type,
                label: channelType.displayName,
              })),
            ]}
          />
        )}
        {/* 並べる局がなければ、並べる順も出さない。 */}
        {types.length > 0 && (
          <SegmentedControl
            size="xs"
            value={order}
            onChange={(value) => setOrder(value === 'force' ? 'force' : 'guide')}
            aria-label={t('live.order')}
            data={[
              { value: 'guide', label: t('live.orderGuide') },
              { value: 'force', label: t('live.orderForce') },
            ]}
          />
        )}
      </Group>
      {types.length === 0 && (
        <EmptyState icon={Tv} title={t('live.empty')} hint={t('live.emptyHint')} />
      )}
      <SimpleGrid cols={{ base: 1, xs: 2, md: 3, lg: 4 }} spacing="xs">
        {shown.map((row) => (
          // カードはリンクなので、ピン留めのボタンは、カードの中ではなく上に重ねる。
          <Box key={row.service.id} pos="relative">
            <NowPlayingCard
              serviceId={row.service.id}
              serviceName={row.service.name}
              serviceLogoUrl={row.service.logoUrl ?? null}
              program={row.programs[0] ?? null}
              force={row.service.jikkyo?.force ?? null}
            />
            <Box pos="absolute" top={6} right={6}>
              <PinButton
                serviceId={row.service.id}
                serviceName={row.service.name}
                pinned={pinnedSet.has(row.service.id)}
                size="xs"
              />
            </Box>
          </Box>
        ))}
      </SimpleGrid>
    </Stack>
  );
}
