import { Card, Text } from '@mantine/core';
import { Link } from '@tanstack/react-router';
import { useTranslation } from 'react-i18next';

import { ProgramTitle } from '../futon';
import { ServiceLabel } from './ServiceLabel';
import { zonedFormat } from '../futon';

const timeFormat = zonedFormat({
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

type Props = {
  serviceId: string;
  serviceName: string;
  serviceLogoUrl: string | null;
  // program は、放送中の番組。番組がなければ null。
  program: { name: string; startAt: string; endAt: string } | null;
  // force は、実況の勢い (1 分あたりのコメントの数。#490)。出さないか、分からなければ null。
  force?: number | null;
};

// NowPlayingCard は、サービスと放送中の番組のカード。押すと、そのサービスのライブ視聴を開く。ホームとライブ視聴の一覧で使う。
export function NowPlayingCard({
  serviceId,
  serviceName,
  serviceLogoUrl,
  program,
  force = null,
}: Props) {
  const { t } = useTranslation();

  return (
    <Card
      withBorder
      padding="xs"
      renderRoot={(props) => <Link {...props} to="/live/$serviceId" params={{ serviceId }} />}
    >
      <Text size="xs" c="dimmed">
        <ServiceLabel name={serviceName} logoUrl={serviceLogoUrl} />
      </Text>
      <Text size="sm" lineClamp={2} mt="xs" c={program === null ? 'dimmed' : undefined}>
        {program === null ? t('live.noProgram') : <ProgramTitle name={program.name} />}
      </Text>
      {/* 番組名が 1 行のカードでも、放送時間をカードの下端に揃える。 */}
      {program !== null && (
        <Text size="xs" c="dimmed" mt="auto" pt={4}>
          {timeFormat.format(Date.parse(program.startAt))} 〜{' '}
          {timeFormat.format(Date.parse(program.endAt))}{' '}
          {t('home.duration', {
            minutes: Math.round((Date.parse(program.endAt) - Date.parse(program.startAt)) / 60_000),
          })}
          {force !== null && <> · {t('jikkyo.force', { count: force })}</>}
        </Text>
      )}
    </Card>
  );
}
