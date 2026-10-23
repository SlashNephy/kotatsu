// 認証の API (/auth/*) を呼ぶ。GraphQL の外にあるのは、ログインする前に使うからである (設計書 9.2、9.3)。
// tenban は、CSRF を防ぐため、POST の JSON だけを受け付け、Origin を確かめる。fetch は同じオリジンの Origin を付ける。

import type {
  AuthenticationResponseJSON,
  PublicKeyCredentialCreationOptionsJSON,
  PublicKeyCredentialRequestOptionsJSON,
  RegistrationResponseJSON,
} from '@simplewebauthn/browser';

export type SecondFactor = 'totp' | 'passkey' | 'recovery_code';

// AuthError は、tenban が返したエラー。code で画面の案内を出し分ける。
export class AuthError extends Error {
  readonly status: number;
  readonly code: string;
  // reason は、パスワードの規定に当たったときの理由 (too_short など)。
  readonly reason: string | undefined;
  // retryAfter は、試行回数の制限で待たせる秒数。
  readonly retryAfter: number | undefined;

  constructor(
    status: number,
    code: string,
    reason: string | undefined,
    retryAfter: number | undefined,
  ) {
    super(code);
    this.name = 'AuthError';
    this.status = status;
    this.code = code;
    this.reason = reason;
    this.retryAfter = retryAfter;
  }
}

type ErrorBody = { error?: { code?: string; reason?: string } };

async function get<T>(path: string): Promise<T> {
  return handle<T>(
    await fetch(path, { credentials: 'same-origin', headers: { Accept: 'application/json' } }),
  );
}

async function post<T>(path: string, body: unknown): Promise<T> {
  return handle<T>(
    await fetch(path, {
      method: 'POST',
      credentials: 'same-origin',
      headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    }),
  );
}

async function handle<T>(response: Response): Promise<T> {
  const json: unknown = (response.headers.get('Content-Type') ?? '').startsWith('application/json')
    ? await response.json()
    : {};
  if (!response.ok) {
    const error = (json as ErrorBody).error;
    const retryAfter = Number.parseInt(response.headers.get('Retry-After') ?? '', 10);
    throw new AuthError(
      response.status,
      error?.code ?? `HTTP_${response.status}`,
      error?.reason,
      Number.isNaN(retryAfter) ? undefined : retryAfter,
    );
  }

  return json as T;
}

export type LoginResult = {
  // next は、次に求めるもの。second_factor なら登録済みの第二要素、enroll_second_factor なら TOTP の登録。
  next: 'second_factor' | 'enroll_second_factor';
  methods: SecondFactor[];
  expiresAt: string;
};

export type TOTPEnrollment = { secret: string; uri: string; expiresAt: string };

export type Completed = { recoveryCodes: string[] };

// Reauthenticated は、再認証を終えたときの応答。authenticatedAt から 10 分の間、重要な操作ができる。
export type Reauthenticated = { authenticatedAt: string };

// ReauthenticationStatus は、ログイン中のセッションの認証の日時と、直近 10 分以内に認証していて重要な操作ができるか。
export type ReauthenticationStatus = { authenticatedAt: string; recent: boolean };

// OIDCStatus は、OIDC でのログインを受け付けるかと、ボタンに出す IdP の名前、ログイン中のユーザーが IdP と紐付いているか。
export type OIDCStatus = { enabled: boolean; displayName: string; linked: boolean };

// PasskeyAssertion は、パスキーでの確認の開始。options を、そのまま navigator.credentials.get に渡す。
export type PasskeyAssertion = {
  options: PublicKeyCredentialRequestOptionsJSON;
  expiresAt: string;
};

// PasskeyCreation は、パスキーの登録を始めた応答。options を navigator.credentials.create に渡す。
export type PasskeyCreation = {
  options: PublicKeyCredentialCreationOptionsJSON;
  expiresAt: string;
};

// RegisteredPasskey は、登録したパスキー。
export type RegisteredPasskey = { id: string; name: string };

export const authApi = {
  setupStatus: () => get<{ required: boolean }>('/auth/setup'),
  setup: (input: { token: string; username: string; displayName: string; password: string }) =>
    post<{ username: string; next: 'login' }>('/auth/setup', input),
  login: (username: string, password: string) =>
    post<LoginResult>('/auth/login', { username, password }),
  totp: (code: string) => post<Completed>('/auth/login/totp', { code }),
  recoveryCode: (code: string) => post<Completed>('/auth/login/recovery-code', { code }),
  beginEnrollment: () => post<TOTPEnrollment>('/auth/login/totp/enrollment', {}),
  confirmEnrollment: (code: string) =>
    post<Completed>('/auth/login/totp/enrollment/confirm', { code }),
  // パスキーだけでログインする。どのユーザーかは、認証器が答える。
  beginPasskeyLogin: () => post<PasskeyAssertion>('/auth/passkey/login', {}),
  confirmPasskeyLogin: (credential: AuthenticationResponseJSON) =>
    post<Completed>('/auth/passkey/login/confirm', { credential }),
  // パスワードの後の第二要素として、パスキーで確かめる。
  beginPasskeySecondFactor: () => post<PasskeyAssertion>('/auth/login/passkey', {}),
  confirmPasskeySecondFactor: (credential: AuthenticationResponseJSON) =>
    post<Completed>('/auth/login/passkey/confirm', { credential }),
  // OIDC でのログイン (設計書 9.4)。始めると、IdP の認可のエンドポイントを返すので、ブラウザーを送り出す。
  oidcStatus: () => get<OIDCStatus>('/auth/oidc'),
  beginOIDC: () => post<{ authorizationUrl: string }>('/auth/oidc', {}),
  // ログイン中のセッションのまま、もう一度確かめる (設計書 9.3)。パスワードの変更などの前に求められる。
  reauthenticate: (password: string, method: 'totp' | 'recovery_code', code: string) =>
    post<Reauthenticated>('/auth/reauth', { password, method, code }),
  beginPasskeyReauthentication: () => post<PasskeyAssertion>('/auth/reauth/passkey', {}),
  // ログイン中のユーザーに、パスキーを登録する。直前に再認証していなければ、REAUTHENTICATION_REQUIRED を返す。
  beginPasskeyRegistration: () => post<PasskeyCreation>('/auth/passkey/registration', {}),
  confirmPasskeyRegistration: (credential: RegistrationResponseJSON, name: string) =>
    post<RegisteredPasskey>('/auth/passkey/registration/confirm', { credential, name }),
  confirmPasskeyReauthentication: (credential: AuthenticationResponseJSON) =>
    post<Reauthenticated>('/auth/reauth/passkey/confirm', { credential }),
  // 重要な操作ができるか。入力のある重要な操作は、入力を始める前に確かめ、必要なら先に再認証させる (#916)。
  reauthenticationStatus: () => get<ReauthenticationStatus>('/auth/reauth'),
  // IdP で再認証する。終えると、returnTo のパスに reauthenticated=oidc を付けて戻る。失敗したときは、returnTo に error=<理由> が付く。
  beginOIDCReauthentication: (returnTo: string) =>
    post<{ authorizationUrl: string }>('/auth/reauth/oidc', { returnTo }),
  // ログイン中のユーザーを IdP のユーザーと紐付ける (設計書 9.4)。終えると、returnTo のパスに linked=oidc を付けて戻る。
  // 失敗したときは、returnTo に error=<理由> が付く。直前に再認証していなければ、REAUTHENTICATION_REQUIRED を返す。
  beginOIDCLink: (returnTo: string) =>
    post<{ authorizationUrl: string }>('/auth/oidc/link', { returnTo }),
  logout: async () => {
    await fetch('/auth/logout', { method: 'POST', credentials: 'same-origin' });
  },
};
