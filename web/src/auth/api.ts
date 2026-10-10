// 認証の API (/auth/*) を呼ぶ。GraphQL の外にあるのは、ログインする前に使うからである (設計書 9.2、9.3)。
// tenban は、CSRF を防ぐため、POST の JSON だけを受け付け、Origin を確かめる。fetch は同じオリジンの Origin を付ける。

export type SecondFactor = 'totp' | 'recovery_code';

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
    const retryAfter = response.headers.get('Retry-After');
    throw new AuthError(
      response.status,
      error?.code ?? `HTTP_${response.status}`,
      error?.reason,
      retryAfter === null ? undefined : Number(retryAfter),
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
  logout: async () => {
    await fetch('/auth/logout', { method: 'POST', credentials: 'same-origin' });
  },
};
