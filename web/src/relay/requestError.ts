// GraphQLRequestError は、GraphQL の処理に入る前に tenban が拒否したリクエストや、
// リバースプロキシのエラーのように GraphQL の応答でないものを表す。
export class GraphQLRequestError extends Error {
  readonly status: number;
  // code は、tenban が付けたエラーコード (UNAUTHENTICATED など)。GraphQL の応答でなければ undefined。
  readonly code: string | undefined;

  constructor(message: string, status: number, code: string | undefined) {
    super(message);
    this.name = 'GraphQLRequestError';
    this.status = status;
    this.code = code;
  }
}
