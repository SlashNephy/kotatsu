import { Stack, Text } from '@mantine/core';
import { useTranslation } from 'react-i18next';
import { zonedFormat } from './timeZone';

const dateTimeFormat = zonedFormat({
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

type Props = {
  createdAt: string;
  createdBy: { readonly displayName: string } | null | undefined;
  updatedAt: string;
  updatedBy: { readonly displayName: string } | null | undefined;
  // automatic は、ユーザーがいないとき、neko が作ったり変えたりしたものとみなすかどうか (ルール予約)。
  // そうでなければ、ユーザーが消えたものとみなす。
  automatic?: boolean;
};

// Authorship は、ルールや予約を、誰がいつ作り、誰がいつ変えたかを出す (設計書 8.1)。
export function Authorship({ createdAt, createdBy, updatedAt, updatedBy, automatic }: Props) {
  const { t } = useTranslation();
  const name = (user: Props['createdBy']) =>
    user?.displayName ?? t(automatic === true ? 'authorship.automatic' : 'authorship.deleted');

  return (
    <Stack gap={0}>
      <Text size="xs" c="dimmed">
        {t('authorship.created', {
          name: name(createdBy),
          at: dateTimeFormat.format(Date.parse(createdAt)),
        })}
      </Text>
      <Text size="xs" c="dimmed">
        {t('authorship.updated', {
          name: name(updatedBy),
          at: dateTimeFormat.format(Date.parse(updatedAt)),
        })}
      </Text>
    </Stack>
  );
}
