import { Alert, Anchor, Stack, Title } from '@mantine/core';
import { createFileRoute, Link, useLocation } from '@tanstack/react-router';
import { Suspense } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useLazyLoadQuery } from 'react-relay';

import { ErrorBoundary, PageLoader } from '../../futon';
import { graphQLErrorDetails, isUnauthenticated } from '../../relay/environment';
import type { ConditionsForm } from '../../conditions/form';
import { emptyRuleForm, fromRule, type RuleForm } from '../../rules/form';
import { RuleEditor } from '../../rules/RuleEditor';
import type { RuleIdQuery } from './__generated__/RuleIdQuery.graphql';

declare module '@tanstack/react-router' {
  interface HistoryState {
    // ruleConditions は、新しいルールの条件の初期値。番組の検索から、条件を引き継いでルールを作るときに渡す。
    ruleConditions?: ConditionsForm;
  }
}

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
        <Suspense fallback={<PageLoader />}>
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
          logoUrl
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
              keywordMatch
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
              joinSegments
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
    { fetchPolicy: 'store-and-network' },
  );
  const node = data.node;
  const ruleConditions = useLocation({ select: (location) => location.state.ruleConditions });
  if (!isNew && node?.__typename !== 'Rule') {
    throw new RuleNotFoundError();
  }

  return (
    <RuleEditor
      ruleId={node?.__typename === 'Rule' ? node.id : undefined}
      initial={node?.__typename === 'Rule' ? fromRule(node) : newRuleForm(ruleConditions)}
      channelTypes={data.channelTypes}
      services={data.services}
      readOnly={data.viewer.role === 'VIEWER'}
    />
  );
}

// newRuleForm は、新しいルールの入力の初期値。条件を引き継いだときは、最初のキーワードをルールの名前にしておく。
function newRuleForm(conditions: ConditionsForm | undefined): RuleForm {
  const form = emptyRuleForm(conditions);
  const keyword = form.keywords.find((row) => row.text.trim() !== '');

  return { ...form, name: keyword?.text.trim() ?? '' };
}
