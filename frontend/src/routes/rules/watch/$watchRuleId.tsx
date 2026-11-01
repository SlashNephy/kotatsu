import { Alert, Anchor, Stack, Title } from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Suspense } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useFragment } from 'react-relay';
import { useLazyLoadQuery } from '../../../relay/useLazyLoadQuery';

import { useDataChanges } from '../../../changes/DataChanges';
import { ErrorBoundary, PageLoader } from '../../../futon';
import { graphQLErrorDetails, isUnauthenticated } from '../../../relay/environment';
import { WatchRuleEditor, watchRuleEditorFragment } from '../../../watch/WatchRuleEditor';
import type { WatchRuleEditor_watchRule$key } from '../../../watch/__generated__/WatchRuleEditor_watchRule.graphql';
import { emptyWatchRuleForm, fromWatchRule } from '../../../watch/watchRuleForm';
import type { WatchRuleIdQuery } from './__generated__/WatchRuleIdQuery.graphql';

export const Route = createFileRoute('/rules/watch/$watchRuleId')({
  component: WatchRulePage,
});

// WatchRuleNotFoundError は、URL の視聴ルールが見つからないことを表す。通信の失敗とは分けて示す。
class WatchRuleNotFoundError extends Error {
  constructor() {
    super('watch rule not found');
    this.name = 'WatchRuleNotFoundError';
  }
}

// newWatchRuleId は、新しい視聴ルールを作るときの、URL の視聴ルールの ID。
const newWatchRuleId = 'new';

// WatchRulePage は、視聴ルールを作るか編集する画面 (設計書 4.8、10.3)。
function WatchRulePage() {
  const { t } = useTranslation();
  const { watchRuleId } = Route.useParams();

  return (
    <Stack gap="sm">
      <Anchor
        size="sm"
        renderRoot={(props) => <Link {...props} to="/rules" search={{ watch: true }} />}
      >
        {t('watchRules.back')}
      </Anchor>
      <Title order={2}>
        {watchRuleId === newWatchRuleId ? t('watchRules.createTitle') : t('watchRules.editTitle')}
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
              {error instanceof WatchRuleNotFoundError ||
              graphQLErrorDetails(error).code === 'NOT_FOUND'
                ? t('watchRules.notFound')
                : t('viewer.error')}
            </Alert>
          )
        }
      >
        <Suspense fallback={<PageLoader />}>
          <WatchRule key={watchRuleId} watchRuleId={watchRuleId} />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

function WatchRule({ watchRuleId }: { watchRuleId: string }) {
  const isNew = watchRuleId === newWatchRuleId;
  // ほかの端末で視聴ルールが変わったら、読み直す。入力を触っていれば、WatchRuleEditor は入れ直さない。
  const changes = useDataChanges(['VIEWER_SETTINGS']);
  const data = useLazyLoadQuery<WatchRuleIdQuery>(
    graphql`
      query WatchRuleIdQuery($id: ID!, $skip: Boolean!) {
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
          ... on WatchRule {
            id
            ...WatchRuleEditor_watchRule @alias(as: "watchRule")
          }
        }
      }
    `,
    // 新しい視聴ルールでは node を引かないので、ID は何でもよい。
    { id: isNew ? '' : watchRuleId, skip: isNew },
    { fetchKey: changes, fetchPolicy: 'store-and-network' },
  );
  const node = data.node;
  const watchRule = useFragment<WatchRuleEditor_watchRule$key>(
    watchRuleEditorFragment,
    node?.__typename === 'WatchRule' ? node.watchRule : null,
  );
  if (!isNew && node?.__typename !== 'WatchRule') {
    throw new WatchRuleNotFoundError();
  }

  return (
    <WatchRuleEditor
      watchRuleId={node?.__typename === 'WatchRule' ? node.id : undefined}
      initial={watchRule != null ? fromWatchRule(watchRule) : emptyWatchRuleForm()}
      channelTypes={data.channelTypes}
      services={data.services}
    />
  );
}
