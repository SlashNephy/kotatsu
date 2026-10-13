import {
  Alert,
  Anchor,
  Badge,
  Button,
  Card,
  Group,
  Stack,
  Switch,
  Text,
  Title,
} from '@mantine/core';
import { Plus } from 'lucide-react';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Suspense, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useLazyLoadQuery, useMutation } from 'react-relay';

import { ErrorBoundary } from '../../futon';
import { isUnauthenticated } from '../../relay/environment';
import type { rulesEnableMutation } from './__generated__/rulesEnableMutation.graphql';
import type { rulesListQuery } from './__generated__/rulesListQuery.graphql';

export const Route = createFileRoute('/rules/')({
  component: RulesPage,
});

// RulesPage は、ルールの一覧の画面 (設計書 10.3)。ルールは家族で共有する。
function RulesPage() {
  const { t } = useTranslation();

  return (
    <Stack gap="sm">
      <Title order={2}>{t('rules.title')}</Title>
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
        <Suspense fallback={<Text c="dimmed">{t('viewer.loading')}</Text>}>
          <Rules />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

function Rules() {
  const { t } = useTranslation();
  const data = useLazyLoadQuery<rulesListQuery>(
    graphql`
      query rulesListQuery {
        viewer {
          role
        }
        rules {
          id
          name
          enabled
          memo
          conditions {
            keywords {
              text
            }
            channelTypes
          }
          updatedAt
        }
      }
    `,
    {},
    { fetchPolicy: 'store-and-network' },
  );
  const [commit] = useMutation<rulesEnableMutation>(graphql`
    mutation rulesEnableMutation($id: ID!, $enabled: Boolean!) {
      setRuleEnabled(id: $id, enabled: $enabled) {
        id
        enabled
      }
    }
  `);
  const canEdit = data.viewer.role !== 'VIEWER';
  const [toggleError, setToggleError] = useState(false);

  return (
    <Stack>
      {canEdit && (
        <Group>
          <Button
            leftSection={<Plus size={16} aria-hidden />}
            renderRoot={(props) => (
              <Link {...props} to="/rules/$ruleId" params={{ ruleId: 'new' }} />
            )}
          >
            {t('rules.create')}
          </Button>
        </Group>
      )}
      {toggleError && (
        <Alert color="red" role="alert">
          {t('reservations.toggleError')}
        </Alert>
      )}
      {data.rules.length === 0 && <Alert color="gray">{t('rules.empty')}</Alert>}
      {data.rules.map((rule) => (
        <Card key={rule.id} withBorder padding="sm" style={{ opacity: rule.enabled ? 1 : 0.6 }}>
          <Group justify="space-between" wrap="nowrap">
            <Stack gap={2} style={{ minWidth: 0 }}>
              <Anchor
                fw={700}
                renderRoot={(props) => (
                  <Link {...props} to="/rules/$ruleId" params={{ ruleId: rule.id }} />
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
                    {type}
                  </Badge>
                ))}
                {!rule.enabled && (
                  <Badge color="gray" size="sm">
                    {t('rules.disabled')}
                  </Badge>
                )}
              </Group>
              {rule.memo !== '' && (
                <Text size="xs" c="dimmed" truncate>
                  {rule.memo}
                </Text>
              )}
            </Stack>
            {canEdit && (
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
            )}
          </Group>
        </Card>
      ))}
    </Stack>
  );
}
