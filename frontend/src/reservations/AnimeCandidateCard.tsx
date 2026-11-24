import { Anchor, Badge, Button, Card, Group, Stack, Text, Title } from '@mantine/core';
import { Link } from '@tanstack/react-router';
import { CalendarCheck, ListPlus, Star } from 'lucide-react';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

import { ProgramTitle, zonedFormat } from '../futon';
import { broadcastDay, dateOf } from '../guide/layout';
import { ServiceLabel } from '../guide/ServiceLabel';

const dateTimeFormat = zonedFormat({
  month: 'numeric',
  day: 'numeric',
  weekday: 'short',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});
const timeFormat = zonedFormat({
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

// AnimeCandidateWork は、録画の候補の作品。見たいリスト (#928) と、フォロー中の人が見ている作品 (#1968) が、同じ形で返す。
export type AnimeCandidateWork = {
  readonly tid: number;
  readonly title: string;
  readonly reservedBroadcasts: number;
  readonly covered: boolean;
  readonly broadcasts: ReadonlyArray<{
    readonly preferred: boolean;
    readonly program: {
      readonly id: string;
      readonly name: string;
      readonly startAt: string;
      readonly endAt: string;
      readonly service: {
        readonly name: string;
        readonly logoUrl: string | null | undefined;
        readonly channelType: { readonly type: string };
      };
    };
  }>;
};

// AnimeCandidateCard は、録画の候補の作品の 1 枚 (#928、#1968)。作品名と、予約していない放送を並べ、
// 予約できるユーザーには、作品からルールを作るボタンと、放送ごとの予約のボタンを出す。
// 作品名の下には、候補にした理由 (見たいリストに入れた記録先、見ている人) を、children で受け取って出す。
export function AnimeCandidateCard({
  work,
  canEdit,
  busy,
  pending,
  onCreateRule,
  onReserve,
  children,
}: {
  work: AnimeCandidateWork;
  canEdit: boolean;
  // busy は、押して返事を待っている操作 (`rule:<tid>` か、番組の ID)。
  busy: string | null;
  // pending は、どれかの操作の返事を待っているかどうか。
  pending: boolean;
  onCreateRule: (tid: number) => void;
  onReserve: (programId: string) => void;
  children?: ReactNode;
}) {
  const { t } = useTranslation();

  return (
    <Card component="section" withBorder padding="xs" aria-label={work.title}>
      <Stack gap="xs">
        <Group gap="sm" wrap="nowrap" justify="space-between" align="flex-start">
          <Stack gap={4} style={{ minWidth: 0 }}>
            <Group gap="xs">
              <Title order={3} size="h5">
                {work.title}
              </Title>
              {/* 予約のある作品は、録る放送が決まっているので、予約済みと示し、ルールを作らせない (#928)。 */}
              {work.covered && (
                <Badge
                  size="sm"
                  color="teal"
                  variant="light"
                  tt="none"
                  leftSection={<CalendarCheck size={12} aria-hidden />}
                >
                  {t('reservations.wishlist.covered')}
                </Badge>
              )}
            </Group>
            {children}
            {work.reservedBroadcasts > 0 && (
              <Text size="xs" c="dimmed">
                {t('reservations.wishlist.reserved', { count: work.reservedBroadcasts })}
              </Text>
            )}
          </Stack>
          {canEdit && !work.covered && (
            <Button
              size="xs"
              flex="none"
              leftSection={<ListPlus size={14} aria-hidden />}
              loading={busy === `rule:${work.tid}`}
              disabled={pending && busy !== `rule:${work.tid}`}
              onClick={() => onCreateRule(work.tid)}
            >
              {t('reservations.wishlist.createRule')}
            </Button>
          )}
        </Group>
        <Stack gap={4}>
          {work.broadcasts.map(({ preferred, program }) => {
            const start = Date.parse(program.startAt);

            return (
              <Group key={program.id} gap="sm" wrap="nowrap" justify="space-between">
                <Stack gap={0} style={{ minWidth: 0 }}>
                  <Text size="sm" truncate>
                    <Anchor
                      inherit
                      renderRoot={(props) => (
                        <Link
                          {...props}
                          to="/guide"
                          search={{
                            date: dateOf(broadcastDay(start).from),
                            type: program.service.channelType.type,
                            program: program.id,
                          }}
                        />
                      )}
                    >
                      <ProgramTitle name={program.name} />
                    </Anchor>
                  </Text>
                  <Group gap="xs">
                    <Text size="xs" c="dimmed">
                      {t('range', {
                        from: dateTimeFormat.format(start),
                        to: timeFormat.format(Date.parse(program.endAt)),
                      })}
                    </Text>
                    <Text size="xs">
                      <ServiceLabel name={program.service.name} logoUrl={program.service.logoUrl} />
                    </Text>
                    {preferred && (
                      <Badge
                        size="xs"
                        color="teal"
                        variant="light"
                        tt="none"
                        leftSection={<Star size={10} aria-hidden />}
                        title={t('reservations.wishlist.preferredHelp')}
                      >
                        {t('reservations.wishlist.preferred')}
                      </Badge>
                    )}
                  </Group>
                </Stack>
                {canEdit && (
                  <Button
                    size="xs"
                    variant="default"
                    flex="none"
                    loading={busy === program.id}
                    disabled={pending && busy !== program.id}
                    onClick={() => onReserve(program.id)}
                  >
                    {t('reservations.wishlist.reserve')}
                  </Button>
                )}
              </Group>
            );
          })}
        </Stack>
      </Stack>
    </Card>
  );
}
