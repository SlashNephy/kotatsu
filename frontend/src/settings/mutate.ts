import {
  commitMutation,
  type Environment,
  type GraphQLTaggedNode,
  type MutationParameters,
} from 'relay-runtime';

// mutate は、mutation を送り、結果を Promise で返す。
// Relay は、GraphQL のエラーも onCompleted に渡すので、エラーがあれば、graphQLErrorDetails で読める形の Error で失敗させる。
export function mutate<T extends MutationParameters>(
  environment: Environment,
  mutation: GraphQLTaggedNode,
  variables: T['variables'],
): Promise<T['response']> {
  return new Promise((resolve, reject) => {
    commitMutation<T>(environment, {
      mutation,
      variables,
      onCompleted: (response, errors) => {
        if (errors != null && errors.length > 0) {
          reject(
            Object.assign(new Error(errors[0]?.message ?? 'GraphQL error'), { source: { errors } }),
          );

          return;
        }
        resolve(response);
      },
      onError: reject,
    });
  });
}
