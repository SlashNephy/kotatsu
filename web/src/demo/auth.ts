// authResponse は、デモで /auth/* に返す応答の本文。
// デモは、いつもログインしているものとして扱う。ログアウトした後のログインは、何を入れても通す。
// 第二要素は TOTP だけを求め、どのコードでも通す。再認証も、どの入力でも通す。
export function authResponse(pathname: string): unknown {
  switch (pathname) {
    case '/auth/setup':
      return { required: false };
    case '/auth/oidc':
      return { enabled: false, displayName: '', linked: false };
    case '/auth/login':
      return {
        next: 'second_factor',
        methods: ['totp'],
        expiresAt: new Date(Date.now() + 5 * 60 * 1000).toISOString(),
      };
    case '/auth/login/totp':
    case '/auth/login/recovery-code':
      return { recoveryCodes: [] };
    case '/auth/reauth':
    case '/auth/reauth/passkey/confirm':
      return { authenticatedAt: new Date().toISOString() };
    default:
      return {};
  }
}
