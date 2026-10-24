import { useContext } from 'react';
import { useLazyLoadQuery as useRelayLazyLoadQuery, type GraphQLTaggedNode } from 'react-relay';
import type { OperationType, VariablesOf } from 'relay-runtime';

import { ErrorBoundaryAttemptContext } from '../futon';

type Options = Parameters<typeof useRelayLazyLoadQuery>[2];

// useLazyLoadQuery は、react-relay の useLazyLoadQuery に、いちばん近い ErrorBoundary が戻した回数を、読み込みのキー (fetchKey) として混ぜる。
// サーバーが戻って ErrorBoundary が子を描き直したとき、失敗を覚えている Relay に、送り直させるため。
// 回数は境界ごとなので、ほかの境界の中で読み込み済みのクエリは、読み直さない。
export function useLazyLoadQuery<TQuery extends OperationType>(
  query: GraphQLTaggedNode,
  variables: VariablesOf<TQuery>,
  options?: Options,
): TQuery['response'] {
  const attempt = useContext(ErrorBoundaryAttemptContext);
  // 戻していなければ、渡された options をそのまま使い、react-relay の useLazyLoadQuery と同じに振る舞う。
  const resolved =
    attempt === 0
      ? options
      : { ...options, fetchKey: `${options?.fetchKey ?? ''}:retry${attempt}` };

  return useRelayLazyLoadQuery<TQuery>(query, variables, resolved);
}
