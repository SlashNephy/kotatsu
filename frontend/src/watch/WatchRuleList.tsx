import { Alert, Anchor, Badge, Button, Card, Group, Stack, Switch, Text } from '@mantine/core';
import { Link } from '@tanstack/react-router';
import { BellPlus, Plus } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useMutation } from 'react-relay';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';

import { useDataChanges } from '../changes/DataChanges';
import { EmptyState } from '../futon';
import type { WatchRuleListEnableMutation } from './__generated__/WatchRuleListEnableMutation.graphql';
import type { WatchRuleListQuery } from './__generated__/WatchRuleListQuery.graphql';

// maxWatchRules は、1 ユーザーが持てる視聴ルールの数の上限 (設計書 付録 A)。tenban も同じ数で拒む。
export const maxWatchRules = 50;

// WatchRuleList は、ログイン中のユーザーの視聴ルールの一覧 (設計書 4.8、10.3)。
// 視聴ルールは自分だけのものなので、閲覧だけのユーザーも作り、変えられる。
export function WatchRuleList() {
  const { t } = useTranslation();
  // ほかの端末で視聴ルールを変えたら、ユーザーの設定の変更として知らされるので、読み直す。
  const viewerChanges = useDataChanges(['VIEWER_SETTINGS']);
  const data = useLazyLoadQuery<WatchRuleListQuery>(
    graphql`
      query WatchRuleListQuery {
        channelTypes {
          type
          displayName
        }
        watchRules {
          id
          name
          enabled
          conditions {
            keywords {
              text
            }
            channelTypes
          }
        }
      }
    `,
    {},
    { fetchKey: viewerChanges, fetchPolicy: 'store-and-network' },
  );
  const [commit] = useMutation<WatchRuleListEnableMutation>(graphql`
    mutation WatchRuleListEnableMutation($id: ID!, $enabled: Boolean!) {
      setWatchRuleEnabled(id: $id, enabled: $enabled) {
        id
        enabled
      }
    }
  `);
  const [toggleError, setToggleError] = useState(false);
  const full = data.watchRules.length >= maxWatchRules;

  return (
    <Stack>
      <Text size="sm" c="dimmed">
        {t('watchRules.description')}
      </Text>
      <Group>
        {/* 上限まで作ったら、作る画面に移れないようにする。tenban も上限を超える作成を拒む。 */}
        {full ? (
          <>
            <Button leftSection={<Plus size={16} aria-hidden />} disabled>
              {t('watchRules.create')}
            </Button>
            <Text size="xs" c="dimmed">
              {t('watchRules.limit', { count: maxWatchRules })}
            </Text>
          </>
        ) : (
          <Button
            leftSection={<Plus size={16} aria-hidden />}
            renderRoot={(props) => (
              <Link {...props} to="/rules/watch/$watchRuleId" params={{ watchRuleId: 'new' }} />
            )}
          >
            {t('watchRules.create')}
          </Button>
        )}
      </Group>
      {toggleError && (
        <Alert color="red" role="alert">
          {t('watchRules.toggleError')}
        </Alert>
      )}
      {data.watchRules.length === 0 && (
        <EmptyState
          icon={BellPlus}
          title={t('watchRules.empty')}
          hint={t('watchRules.emptyHint')}
        />
      )}
      {data.watchRules.map((rule) => (
        <Card key={rule.id} withBorder padding="sm" style={{ opacity: rule.enabled ? 1 : 0.6 }}>
          <Group justify="space-between" wrap="nowrap">
            <Stack gap={2} style={{ minWidth: 0, flex: 1 }}>
              <Anchor
                fw={700}
                renderRoot={(props) => (
                  <Link
                    {...props}
                    to="/rules/watch/$watchRuleId"
                    params={{ watchRuleId: rule.id }}
                  />
                )}
              >
                {rule.name}
              </Anchor>
              <Group gap={4}>
                {rule.conditions.keywords.map((keyword, index) => (
                  <Badge key={index} variant="light" color="gray" size="sm">
                    {keyword.text}
                  </Badge>
                ))}
                {rule.conditions.channelTypes.map((type) => (
                  <Badge key={type} variant="outline" color="gray" size="sm">
                    {data.channelTypes.find((channelType) => channelType.type === type)
                      ?.displayName ?? type}
                  </Badge>
                ))}
                {!rule.enabled && (
                  <Badge color="gray" size="sm">
                    {t('rules.disabled')}
                  </Badge>
                )}
              </Group>
            </Stack>
            <Switch
              aria-label={t('rules.enabled')}
              checked={rule.enabled}
              onChange={(e) =>
                commit({
                  variables: { id: rule.id, enabled: e.currentTarget.checked },
                  onCompleted: (_response, errors) =>
                    setToggleError(errors != null && errors.length > 0),
                  onError: () => setToggleError(true),
                })
              }
            />
          </Group>
        </Card>
      ))}
    </Stack>
  );
}
