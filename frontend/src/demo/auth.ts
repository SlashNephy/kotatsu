// authResponse は、デモで /auth/* に返す応答の本文。
// デモは、いつもログインしているものとして扱う。ログアウトした後のログインは、何を入れても通す。
// 第二要素は TOTP だけを求め、どのコードでも通す。再認証も、どの入力でも通す。
export function authResponse(pathname: string): unknown {
  switch (pathname) {
    case '/internal/auth/setup':
      return { required: false };
    case '/internal/auth/oidc':
      return { enabled: false, displayName: '', linked: false };
    case '/internal/auth/login':
      return {
        next: 'second_factor',
        methods: ['totp'],
        expiresAt: new Date(Date.now() + 5 * 60 * 1000).toISOString(),
      };
    // 認証アプリの登録 (#2056)。デモの秘密鍵は、どのコードでも通すので意味を持たない。
    case '/internal/auth/login/totp/enrollment': {
      const secret = 'KOTATSUDEMOSECRET';

      return {
        secret,
        uri: `otpauth://totp/Kotatsu:demo?secret=${secret}&issuer=Kotatsu`,
        expiresAt: new Date(Date.now() + 5 * 60 * 1000).toISOString(),
      };
    }
    case '/internal/auth/login/totp':
    case '/internal/auth/login/totp/enrollment/confirm':
    case '/internal/auth/login/recovery-code':
      return { recoveryCodes: [] };
    // GET は、最近再認証したか (recent) を、POST は、再認証した時刻を返す。同じパスなので、両方を返す。
    case '/internal/auth/reauth':
      return { recent: true, authenticatedAt: new Date().toISOString() };
    case '/internal/auth/reauth/passkey/confirm':
      return { authenticatedAt: new Date().toISOString() };
    default:
      return {};
  }
}
