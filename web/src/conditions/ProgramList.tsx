import { Badge, Stack, Text, UnstyledButton } from '@mantine/core';
import { useTranslation } from 'react-i18next';

import { ProgramTitle } from '../futon';
import { ServiceLabel } from '../guide/ServiceLabel';
import classes from './ProgramList.module.css';

const dateTimeFormat = new Intl.DateTimeFormat('ja-JP', {
  month: 'numeric',
  day: 'numeric',
  weekday: 'short',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
  timeZone: 'Asia/Tokyo',
});

export type ProgramListItem = {
  readonly id: string;
  readonly name: string;
  readonly startAt: string;
  readonly archived?: boolean;
  readonly service: { readonly name: string; readonly logoUrl: string | null | undefined };
};

type Props = {
  programs: readonly ProgramListItem[];
  // onSelect は、番組を押したときに呼ぶ。番組の詳細を開くのに使う。
  onSelect: (programId: string) => void;
};

// ProgramList は、条件に一致した番組の一覧。番組の検索の結果と、ルールのプレビューで使う。
// 日時は、番組名の頭が揃うよう、いちばん長い日時 (12/28(月) 00:00) が収まる幅に固定する。
// 狭い画面では、番組名が読めるよう、日時とチャンネルの下の段に回す。
export function ProgramList({ programs, onSelect }: Props) {
  const { t } = useTranslation();

  return (
    <Stack gap={0}>
      {programs.map((program) => (
        <UnstyledButton
          key={program.id}
          className={classes.row}
          onClick={() => onSelect(program.id)}
        >
          <div className={classes.line}>
            <Text size="sm" c="dimmed" w="8.5em" style={{ flexShrink: 0 }}>
              {dateTimeFormat.format(Date.parse(program.startAt))}
            </Text>
            <Text size="xs" c="dimmed" truncate className={classes.service}>
              <ServiceLabel name={program.service.name} logoUrl={program.service.logoUrl} />
            </Text>
            <Text size="sm" truncate className={classes.title}>
              <ProgramTitle name={program.name} />
            </Text>
            {program.archived === true && (
              <Badge color="gray" variant="light" style={{ flexShrink: 0 }}>
                {t('search.archived')}
              </Badge>
            )}
          </div>
        </UnstyledButton>
      ))}
    </Stack>
  );
}
