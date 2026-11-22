import {
  buildSchema,
  defaultFieldResolver,
  execute,
  getNullableType,
  isAbstractType,
  isEnumType,
  isListType,
  isNonNullType,
  isObjectType,
  isScalarType,
  parse,
  subscribe,
  type ExecutionResult,
  type GraphQLFieldResolver,
  type GraphQLOutputType,
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

// ExecutorOptions は、createExecutor の設定。fillMissing なら、デモのデータにない non-null のフィールドを、型の既定の値で埋める。
// デモが応えない画面 (管理の画面など) も、Storybook (#2056) で枠だけは描けるようにするため。静的なデモでは使わない。
export type ExecutorOptions = { fillMissing?: boolean };

// placeholder は、型 type の既定の値。文字列は空、数は 0、真偽は false、一覧は空、列挙は最初の値、オブジェクトは中身を埋める空のもの。
// 64 ビットの整数 (Int64) も 0、時刻 (Time) は今の時刻にする。抽象型は、取りうる型の最初のもの (__typename だけを持つ空のもの) にする。
// 取りうる型がない抽象型と、知らないスカラーは、決められないので null を返す (non-null ならエラーになる)。
function placeholder(type: GraphQLOutputType, schemaOf: GraphQLSchema): unknown {
  const inner = getNullableType(type);
  if (isListType(inner)) {
    return [];
  }
  if (isEnumType(inner)) {
    return inner.getValues()[0]?.value ?? null;
  }
  if (isObjectType(inner)) {
    return {};
  }
  if (isAbstractType(inner)) {
    const possible = schemaOf.getPossibleTypes(inner)[0];

    return possible === undefined ? null : { __typename: possible.name };
  }
  if (isScalarType(inner)) {
    switch (inner.name) {
      case 'String':
      case 'ID':
        return '';
      case 'Int':
      case 'Int64':
      case 'Float':
        return 0;
      case 'Boolean':
        return false;
      case 'Time':
        return new Date().toISOString();
      default:
        return null;
    }
  }

  return null;
}

// fillingResolver は、値がなく non-null のフィールドを、型の既定の値で埋めるリゾルバー。
const fillingResolver: GraphQLFieldResolver<unknown, unknown> = (source, args, context, info) => {
  const value: unknown = defaultFieldResolver(source, args, context, info);
  if (value === undefined && isNonNullType(info.returnType)) {
    return placeholder(info.returnType, info.schema);
  }

  return value;
};

export function createExecutor(
  rootValue: object,
  { fillMissing = false }: ExecutorOptions = {},
): Executor {
  schema ??= buildSchema(sdl);
  const built = schema;
  const fieldResolver = fillMissing ? fillingResolver : undefined;

  return {
    execute: async (query, variables) =>
      execute({
        schema: built,
        document: parse(query),
        rootValue,
        variableValues: variables,
        fieldResolver,
      }),
    subscribe: async (query, variables) => {
      const result = await subscribe({
        schema: built,
        document: parse(query),
        rootValue,
        variableValues: variables,
        fieldResolver,
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
