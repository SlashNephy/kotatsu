import { Alert, Anchor, Stack, Text, Title } from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Suspense } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useLazyLoadQuery } from 'react-relay';

import { ErrorBoundary } from '../../futon';
import { graphQLErrorDetails, isUnauthenticated } from '../../relay/environment';
import { emptyRuleForm, fromRule } from '../../rules/form';
import { RuleEditor } from '../../rules/RuleEditor';
import type { RuleIdQuery } from './__generated__/RuleIdQuery.graphql';

export const Route = createFileRoute('/rules/$ruleId')({
  component: RulePage,
});

// RuleNotFoundError は、URL のルールが見つからないことを表す。通信の失敗とは分けて示す。
class RuleNotFoundError extends Error {
  constructor() {
    super('rule not found');
    this.name = 'RuleNotFoundError';
  }
}

// newRuleId は、新しいルールを作るときの、URL のルールの ID。
const newRuleId = 'new';

// RulePage は、ルールを作るか編集する画面 (設計書 10.3)。
function RulePage() {
  const { t } = useTranslation();
  const { ruleId } = Route.useParams();

  return (
    <Stack gap="sm">
      <Anchor component={Link} to="/rules" size="sm">
        {t('rules.back')}
      </Anchor>
      <Title order={2}>
        {ruleId === newRuleId ? t('rules.createTitle') : t('rules.editTitle')}
      </Title>
      <ErrorBoundary
        fallback={(error) =>
          isUnauthenticated(error) ? (
            <Alert color="amber">
              <Anchor component={Link} to="/login">
                {t('guide.signedOut')}
              </Anchor>
            </Alert>
          ) : (
            <Alert color="amber">
              {error instanceof RuleNotFoundError || graphQLErrorDetails(error).code === 'NOT_FOUND'
                ? t('rules.notFound')
                : t('viewer.error')}
            </Alert>
          )
        }
      >
        <Suspense fallback={<Text c="dimmed">{t('viewer.loading')}</Text>}>
          <Rule key={ruleId} ruleId={ruleId} />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

function Rule({ ruleId }: { ruleId: string }) {
  const isNew = ruleId === newRuleId;
  const data = useLazyLoadQuery<RuleIdQuery>(
    graphql`
      query RuleIdQuery($id: ID!, $skip: Boolean!) {
        viewer {
          role
        }
        channelTypes {
          type
          displayName
        }
        services {
          id
          name
          serviceId
          channelType {
            displayName
          }
        }
        node(id: $id) @skip(if: $skip) {
          __typename
          ... on Rule {
            id
            name
            memo
            enabled
            conditions {
              keywords {
                text
                regexp
                caseSensitive
                fields
              }
              excludeKeywords {
                text
                regexp
                caseSensitive
                fields
              }
              channelTypes
              services {
                id
              }
              genres {
                lv1
                lv2
              }
              excludeGenres {
                lv1
                lv2
              }
              timeRanges {
                weekdays
                startMinute
                durationMinutes
              }
              minDurationSeconds
              maxDurationSeconds
              freeOnly
            }
            settings {
              poolId
              fileNameTemplate
              startMarginSeconds
              endMarginSeconds
              priority
            }
            validFrom
            validUntil
            avoidDuplicates
            duplicateWindowDays
            followEventRelay
          }
        }
      }
    `,
    // 新しいルールでは node を引かないので、ID は何でもよい。
    { id: isNew ? '' : ruleId, skip: isNew },
    { fetchPolicy: 'network-only' },
  );
  const node = data.node;
  if (!isNew && node?.__typename !== 'Rule') {
    throw new RuleNotFoundError();
  }

  return (
    <RuleEditor
      ruleId={node?.__typename === 'Rule' ? node.id : undefined}
      initial={node?.__typename === 'Rule' ? fromRule(node) : emptyRuleForm()}
      channelTypes={data.channelTypes}
      services={data.services}
      readOnly={data.viewer.role === 'VIEWER'}
    />
  );
}
