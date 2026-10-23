import {
  Alert,
  Button,
  Card,
  Code,
  CopyButton,
  Group,
  Stack,
  Text,
  TextInput,
  Title,
} from '@mantine/core';
import { Check, Copy } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, type useRelayEnvironment } from 'react-relay';

import type { ApiTokensCreateMutation } from './__generated__/ApiTokensCreateMutation.graphql';
import type { ApiTokensDeleteMutation } from './__generated__/ApiTokensDeleteMutation.graphql';
import { zonedFormat } from '../futon';
import { mutate } from './mutate';
import { useReauthBeforeInput, type useReauthGate } from './useReauthGate';

const dateTimeFormat = zonedFormat({
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

export type ApiTokenItem = {
  id: string;
  name: string;
  createdAt: string;
  lastUsedAt: string | null | undefined;
};

type Props = {
  tokens: readonly ApiTokenItem[];
  onChanged: () => void;
  run: ReturnType<typeof useReauthGate>['run'];
  prepare: ReturnType<typeof useReauthGate>['prepare'];
  fail: (error: unknown) => void;
  environment: ReturnType<typeof useRelayEnvironment>;
};

// ApiTokens は、読み取り専用の API トークン (#431、設計書 9.8) の一覧と、発行、取り消し。
// 発行は、そのユーザーとして読むことを許すので、再認証してから行う。トークンそのものは、発行した直後にだけ出す。
export function ApiTokens({ tokens, onChanged, run, prepare, fail, environment }: Props) {
  const { t } = useTranslation();
  const [name, setName] = useState('');
  const [issued, setIssued] = useState<string | null>(null);
  // 名前を入れ始める前に再認証させる。IdP で再認証すると、ページを離れ、入れた名前が消えるため (#916)。
  const nameInput = useReauthBeforeInput(prepare, 'apiToken.name');

  const create = () =>
    run(async () => {
      const response = await mutate<ApiTokensCreateMutation>(
        environment,
        graphql`
          mutation ApiTokensCreateMutation($name: String!) {
            createApiToken(name: $name) {
              token
            }
          }
        `,
        { name: name.trim() },
      );
      setIssued(response.createApiToken.token);
      setName('');
      onChanged();
    }, fail);

  return (
    <Card withBorder>
      <Stack gap="xs">
        <Title order={3} size="h5">
          {t('settings.account.apiTokens')}
        </Title>
        <Text size="xs" c="dimmed">
          {t('settings.account.apiTokensDescription')}
        </Text>
        {issued !== null && (
          <Alert color="green" role="status" withCloseButton onClose={() => setIssued(null)}>
            <Stack gap={4}>
              <Text size="sm">{t('settings.account.apiTokenIssued')}</Text>
              <Group gap="xs" wrap="nowrap">
                <Code miw={0} style={{ overflowWrap: 'anywhere' }}>
                  {issued}
                </Code>
                <CopyButton value={issued}>
                  {({ copied, copy }) => (
                    <Button
                      size="compact-sm"
                      variant="default"
                      flex="none"
                      leftSection={
                        copied ? <Check size={14} aria-hidden /> : <Copy size={14} aria-hidden />
                      }
                      onClick={copy}
                    >
                      {copied ? t('settings.account.copied') : t('settings.account.copy')}
                    </Button>
                  )}
                </CopyButton>
              </Group>
            </Stack>
          </Alert>
        )}
        {tokens.length === 0 && (
          <Text size="sm" c="dimmed">
            {t('settings.account.noApiTokens')}
          </Text>
        )}
        {tokens.map((token) => (
          <Group key={token.id} justify="space-between" wrap="nowrap">
            <Stack gap={0} miw={0} flex={1}>
              <Text size="sm" truncate>
                {token.name}
              </Text>
              <Text size="xs" c="dimmed">
                {t('settings.account.apiTokenTimes', {
                  created: dateTimeFormat.format(Date.parse(token.createdAt)),
                  lastUsed:
                    token.lastUsedAt == null
                      ? '-'
                      : dateTimeFormat.format(Date.parse(token.lastUsedAt)),
                })}
              </Text>
            </Stack>
            <Button
              size="compact-sm"
              variant="default"
              color="red"
              flex="none"
              onClick={() =>
                run(async () => {
                  await mutate<ApiTokensDeleteMutation>(
                    environment,
                    graphql`
                      mutation ApiTokensDeleteMutation($id: ID!) {
                        deleteApiToken(id: $id)
                      }
                    `,
                    { id: token.id },
                  );
                  // 取り消したトークンを出したままにしない。
                  setIssued(null);
                  onChanged();
                }, fail)
              }
            >
              {t('settings.account.revokeApiToken')}
            </Button>
          </Group>
        ))}
        <Group gap="xs" align="flex-end">
          <TextInput
            size="xs"
            label={t('settings.account.newApiTokenName')}
            placeholder={t('settings.account.newApiTokenPlaceholder')}
            maxLength={64}
            ref={nameInput.ref}
            onFocus={nameInput.onFocus}
            value={name}
            onChange={(event) => setName(event.currentTarget.value)}
          />
          <Button size="compact-sm" disabled={name.trim() === ''} onClick={create}>
            {t('settings.account.createApiToken')}
          </Button>
        </Group>
      </Stack>
    </Card>
  );
}
