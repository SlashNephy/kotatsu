import { Badge, Button, Card, Group, Stack, Text, Title } from '@mantine/core';
import { useTranslation } from 'react-i18next';
import { graphql, type useRelayEnvironment } from 'react-relay';

import type { OAuthGrantsDeleteMutation } from './__generated__/OAuthGrantsDeleteMutation.graphql';
import { zonedFormat } from '../futon';
import { mutate } from './mutate';

const dateTimeFormat = zonedFormat({
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

export type OAuthGrantItem = {
  id: string;
  clientId: string;
  clientName: string;
  scopes: readonly string[];
  createdAt: string;
  lastUsedAt: string | null | undefined;
};

type Props = {
  grants: readonly OAuthGrantItem[];
  onChanged: () => void;
  // run は、操作を行い、失敗したら onError を呼ぶ。取り消しは再認証を求めないので、再認証の画面は要らない。
  run: (operation: () => Promise<void>, onError: (error: unknown) => void) => void;
  fail: (error: unknown) => void;
  environment: ReturnType<typeof useRelayEnvironment>;
};

// clientHost は、クライアントの ID (メタデータの URL) のホストを返す。名前はクライアントが自分で名乗るので、ホストも並べて出す。
export function clientHost(clientId: string): string {
  try {
    return new URL(clientId).host;
  } catch {
    return clientId;
  }
}

// OAuthGrants は、連携中の MCP のクライアント (#965、設計書 9.9) の一覧と、取り消し。
// 取り消しは、権限を広げず、乗っ取られた後の被害を抑える操作なので、再認証を求めない。
export function OAuthGrants({ grants, onChanged, run, fail, environment }: Props) {
  const { t } = useTranslation();

  return (
    <Card withBorder>
      <Stack gap="xs">
        <Title order={3} size="h5">
          {t('settings.account.oauthGrants')}
        </Title>
        <Text size="xs" c="dimmed">
          {t('settings.account.oauthGrantsDescription')}
        </Text>
        {grants.length === 0 && (
          <Text size="sm" c="dimmed">
            {t('settings.account.noOAuthGrants')}
          </Text>
        )}
        {grants.map((grant) => (
          <Group key={grant.id} justify="space-between" wrap="nowrap">
            <Stack gap={2} miw={0} flex={1}>
              <Group gap={6} wrap="nowrap" miw={0}>
                <Text size="sm" truncate>
                  {grant.clientName}
                </Text>
                <Text size="xs" c="dimmed" truncate>
                  {clientHost(grant.clientId)}
                </Text>
              </Group>
              <Group gap={4}>
                {grant.scopes.map((scope) => (
                  <Badge key={scope} size="xs" variant="light">
                    {t(scope === 'mcp:write' ? 'oauth.scopeWrite' : 'oauth.scopeRead')}
                  </Badge>
                ))}
              </Group>
              <Text size="xs" c="dimmed">
                {t('settings.account.oauthGrantTimes', {
                  created: dateTimeFormat.format(Date.parse(grant.createdAt)),
                  lastUsed:
                    grant.lastUsedAt == null
                      ? '-'
                      : dateTimeFormat.format(Date.parse(grant.lastUsedAt)),
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
                  await mutate<OAuthGrantsDeleteMutation>(
                    environment,
                    graphql`
                      mutation OAuthGrantsDeleteMutation($id: ID!) {
                        deleteOAuthGrant(id: $id)
                      }
                    `,
                    { id: grant.id },
                  );
                  onChanged();
                }, fail)
              }
            >
              {t('settings.account.revokeOAuthGrant')}
            </Button>
          </Group>
        ))}
      </Stack>
    </Card>
  );
}
