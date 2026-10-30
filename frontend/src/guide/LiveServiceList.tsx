import { Chip, Group, NavLink, Stack, Text } from '@mantine/core';
import { Link } from '@tanstack/react-router';
import { startTransition, Suspense, useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql } from 'react-relay';

import { useDataChanges } from '../changes/DataChanges';
import type { ChangeChannel } from '../changes/dataChanges';
import { nextWakeAt, ProgramTitle, useWakeAt, zonedFormat } from '../futon';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';
import { loadRecentChannels, otherRecentChannels } from '../yunomi/recentChannels';
import { browserStorage } from '../yunomi/volume';
import type { LiveServiceListQuery } from './__generated__/LiveServiceListQuery.graphql';
import { ServiceLabel } from './ServiceLabel';
import { sharesWithListed } from './sharedEvent';

const timeFormat = zonedFormat({
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

// liveChannels は、一覧に関わるデータの種類。変わったら、読み直す (#866)。
const liveChannels: readonly ChangeChannel[] = ['EPG'];

// maxWaitMs は、時刻で読み直すまでに待つ最大の時間。放送中の番組がないサービスで、番組が始まるのに気づくのに使う。
const maxWaitMs = 10 * 60_000;

// recentType は、種別のチップのうち、最近見た局を並べるチップの値。チャンネル種別と重ならない値にする。
const recentType = '#recent';

type Range = { now: number; from: string; to: string };

function liveRange(now: number): Range {
  return { now, from: new Date(now).toISOString(), to: new Date(now + 60_000).toISOString() };
}

// LiveServiceList は、ライブ視聴の画面の右の列に置く、サービスと放送中の番組の一覧 (設計書 10.3)。
// 押すと、そのサービスへ移る。YouTube の「次の動画」の代わりに、一覧に戻らずに選局できるようにする (#493)。
// 読み込みの間に子は描き直されて state を失うので、時間帯は、待たない親で決める。一覧は読み込みを待たず、映像を先に出す。
export function LiveServiceList({ serviceId }: { serviceId: string }) {
  const [range, setRange] = useState(() => liveRange(Date.now()));
  const refresh = useCallback(() => startTransition(() => setRange(liveRange(Date.now()))), []);

  return (
    <Suspense fallback={null}>
      <ServiceList serviceId={serviceId} range={range} onExpire={refresh} />
    </Suspense>
  );
}

function ServiceList({
  serviceId,
  range,
  onExpire,
}: {
  serviceId: string;
  range: Range;
  onExpire: () => void;
}) {
  const { t } = useTranslation();
  const changes = useDataChanges(liveChannels);
  const data = useLazyLoadQuery<LiveServiceListQuery>(
    graphql`
      query LiveServiceListQuery($from: Time!, $to: Time!) {
        channelTypes {
          type
          displayName
        }
        guide(from: $from, to: $to) {
          service {
            id
            name
            logoUrl
            channelType {
              type
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
    { from: range.from, to: range.to },
    { fetchKey: changes, fetchPolicy: 'store-and-network' },
  );
  // 放送中の番組のうち、最も早く終わる時刻に読み直す。読み直す間も、一覧を出したままにする。
  useWakeAt(
    nextWakeAt(
      data.guide.flatMap((row) => row.programs.map((program) => program.endAt)),
      range.now,
      maxWaitMs,
    ),
    onExpire,
  );
  const types = data.channelTypes.filter((type) =>
    data.guide.some((row) => row.service.channelType.type === type.type),
  );
  // 最初は、見ているサービスと同じ種別を出す。
  const currentType = data.guide.find((row) => row.service.id === serviceId)?.service.channelType
    .type;
  const [selectedType, setType] = useState<string | null>(null);
  const type = selectedType ?? currentType ?? types[0]?.type ?? '';
  const listed = new Set(data.guide.map((row) => row.service.id));
  // recent は、この端末で最近見た局のうち、今見ている局を除いたもの (#816)。新しい順に並べる。
  const [recentIds] = useState(() => loadRecentChannels(browserStorage()));
  const recent = otherRecentChannels(recentIds, serviceId, listed).flatMap(
    (id) => data.guide.find((row) => row.service.id === id) ?? [],
  );
  // サブチャンネルがメインチャンネルと同じ番組を流しているときは、メインチャンネルだけを出す (ライブの一覧と同じ)。
  const shown =
    type === recentType
      ? recent
      : data.guide.filter(
          (row) =>
            row.service.channelType.type === type &&
            (row.service.id === serviceId || !sharesWithListed(row.programs[0], listed)),
        );

  return (
    <Stack gap="xs">
      {/* 種別は、YouTube の絞り込みのチップと同じく、折り返して並べる。種別が多い (EXT など) と、横並びの切り替えには収まらない。 */}
      {types.length + (recent.length > 0 ? 1 : 0) > 1 && (
        <Chip.Group value={type} onChange={(value) => setType(value)}>
          <Group gap={6}>
            {recent.length > 0 && (
              <Chip value={recentType} size="xs">
                {t('live.recentChannels')}
              </Chip>
            )}
            {types.map((channelType) => (
              <Chip key={channelType.type} value={channelType.type} size="xs">
                {channelType.displayName}
              </Chip>
            ))}
          </Group>
        </Chip.Group>
      )}
      <nav aria-label={t('live.switchChannel')}>
        {shown.map((row) => {
          const program = row.programs[0] ?? null;

          return (
            <NavLink
              key={row.service.id}
              renderRoot={(props) => (
                <Link {...props} to="/live/$serviceId" params={{ serviceId: row.service.id }} />
              )}
              active={row.service.id === serviceId}
              aria-current={row.service.id === serviceId ? 'page' : undefined}
              label={
                <Text size="sm" lineClamp={2} c={program === null ? 'dimmed' : undefined}>
                  {program === null ? t('live.noProgram') : <ProgramTitle name={program.name} />}
                </Text>
              }
              description={
                <>
                  <ServiceLabel name={row.service.name} logoUrl={row.service.logoUrl ?? null} />
                  {program !== null && (
                    <>
                      {' · '}
                      {t('range', {
                        from: timeFormat.format(Date.parse(program.startAt)),
                        to: timeFormat.format(Date.parse(program.endAt)),
                      })}
                    </>
                  )}
                </>
              }
            />
          );
        })}
      </nav>
    </Stack>
  );
}
