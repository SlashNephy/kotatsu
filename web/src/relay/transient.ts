import { GraphQLRequestError } from './requestError';

// fetchFailureMessages は、サーバーに届かなかったときに fetch が投げる TypeError の文言。
// ブラウザごとに違い、TypeError より詳しい型がないので、文言で見分けるしかない。
const fetchFailureMessages = new Set([
  // Chromium
  'Failed to fetch',
  // Firefox
  'NetworkError when attempting to fetch resource',
  // WebKit
  'Load failed',
]);

// isTransientStatus は、送り直せば通る見込みのある HTTP のステータスかどうかを返す。
// 502、503、504 は、tenban の Pod が入れ替わる間に、リバースプロキシが返す。
// 401 や 404 のように、送り直しても変わらないものは含めない。
export function isTransientStatus(status: number): boolean {
  return status >= 500 || status === 408 || status === 429;
}

// isTransientError は、サーバーの入れ替わりやネットワークの瞬断のように、時間をおいて送り直せば通る見込みのある失敗かどうかを返す。
export function isTransientError(error: unknown): boolean {
  if (error instanceof GraphQLRequestError) {
    return isTransientStatus(error.status);
  }

  // 末尾のピリオドは、ブラウザによって付いたり付かなかったりする。
  return error instanceof TypeError && fetchFailureMessages.has(error.message.replace(/\.$/, ''));
}
