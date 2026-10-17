import { Alert, Anchor, Stack, Title } from '@mantine/core';
import { createFileRoute, Link, useLocation } from '@tanstack/react-router';
import { Suspense } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useFragment, useLazyLoadQuery } from 'react-relay';

import { ErrorBoundary, PageLoader } from '../../futon';
import { graphQLErrorDetails, isUnauthenticated } from '../../relay/environment';
import type { ConditionsForm } from '../../conditions/form';
import { emptyRuleForm, fromRule, type RuleForm } from '../../rules/form';
import { RuleEditor, ruleEditorRuleFragment } from '../../rules/RuleEditor';
import type { RuleEditor_rule$key } from '../../rules/__generated__/RuleEditor_rule.graphql';
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
        storagePoolNames
        systemSettings {
          recordingDefaults {
            poolId
          }
          ruleDefaults {
            avoidDuplicates
            duplicateWindowDays
            followEventRelay
          }
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
            ...RuleEditor_rule @alias(as: "rule")
          }
        }
      }
    `,
    // 新しいルールでは node を引かないので、ID は何でもよい。
    { id: isNew ? '' : ruleId, skip: isNew },
    // ストアにある値ですぐに描き、裏で読み直す。読み直した値は、RuleEditor が入力を触っていなければ入れ直す (#533)。
    { fetchPolicy: 'store-and-network' },
  );
  const node = data.node;
  const rule = useFragment<RuleEditor_rule$key>(
    ruleEditorRuleFragment,
    node?.__typename === 'Rule' ? node.rule : null,
  );
  const ruleConditions = useLocation({ select: (location) => location.state.ruleConditions });
  if (!isNew && node?.__typename !== 'Rule') {
    throw new RuleNotFoundError();
  }

  return (
    <RuleEditor
      ruleId={node?.__typename === 'Rule' ? node.id : undefined}
      initial={
        rule != null
          ? fromRule(rule)
          : newRuleForm(ruleConditions, data.systemSettings.ruleDefaults)
      }
      channelTypes={data.channelTypes}
      services={data.services}
      pools={data.storagePoolNames}
      defaultPoolId={data.systemSettings.recordingDefaults.poolId ?? null}
      readOnly={data.viewer.role === 'VIEWER'}
      authorship={
        rule != null
          ? {
              createdAt: rule.createdAt,
              createdBy: rule.createdBy,
              updatedAt: rule.updatedAt,
              updatedBy: rule.updatedBy,
            }
          : undefined
      }
    />
  );
}

// newRuleForm は、新しいルールの入力の初期値。全体の設定の、ルールを作るときの既定値から始める (#470)。
// 条件を引き継いだときは、最初のキーワードをルールの名前にしておく。
function newRuleForm(
  conditions: ConditionsForm | undefined,
  defaults: {
    readonly avoidDuplicates: boolean;
    readonly duplicateWindowDays?: number | null;
    readonly followEventRelay: boolean;
  },
): RuleForm {
  const form = emptyRuleForm(conditions);
  const keyword = form.keywords.find((row) => row.text.trim() !== '');

  return {
    ...form,
    name: keyword?.text.trim() ?? '',
    avoidDuplicates: defaults.avoidDuplicates,
    duplicateWindowDays: defaults.duplicateWindowDays ?? null,
    followEventRelay: defaults.followEventRelay,
  };
}
