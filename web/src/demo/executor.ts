import {
  buildSchema,
  execute,
  parse,
  subscribe,
  type ExecutionResult,
  type GraphQLSchema,
} from 'graphql';

import sdl from '../../schema.graphql?raw';

// Variables は、GraphQL の変数。
export type Variables = Record<string, unknown>;

// Executor は、tenban の代わりに、ブラウザの中で GraphQL を実行する (#832)。
// スキーマは tenban と同じ schema.graphql を使うので、画面が送る query をそのまま検証できる。
// 値は、rootValue のフィールド (関数なら引数を受け取る) から、graphql-js の既定のリゾルバーで引く。
export type Executor = {
  execute: (query: string, variables: Variables) => Promise<ExecutionResult>;
  subscribe: (query: string, variables: Variables) => Promise<AsyncIterable<ExecutionResult>>;
};

let schema: GraphQLSchema | undefined;

export function createExecutor(rootValue: object): Executor {
  schema ??= buildSchema(sdl);
  const built = schema;

  return {
    execute: async (query, variables) =>
      execute({ schema: built, document: parse(query), rootValue, variableValues: variables }),
    subscribe: async (query, variables) => {
      const result = await subscribe({
        schema: built,
        document: parse(query),
        rootValue,
        variableValues: variables,
      });
      if (Symbol.asyncIterator in result) {
        return result;
      }
      // 始める前に失敗したもの (検証のエラーなど) は、そのエラーだけを 1 つ流す。
      return (async function* () {
        yield result;
      })();
    },
  };
}
