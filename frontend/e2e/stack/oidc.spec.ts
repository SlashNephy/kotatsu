// 偽の OIDC の IdP (backend/cmd/fakeidp) で、OIDC のログインを通しで確かめる (#1525、設計書 9.4、9.5、11.6)。
// scripts/e2e-stack.sh から流す。初回セットアップの管理者 (flows.spec.ts) がいる前提なので、stack の後に流す。
//
// ログインのボタンからの流れはブラウザで、断る場面は、素の HTTP のクライアントで確かめる。ログインの流れでは、
// 何を送って何が返ったかが仕様そのものなので、リダイレクトを追わずに、そのまま確かめる。
// Playwright の request は資格情報を自動で足すので、断る場面には使わない。
import { expect, test } from '@playwright/test';

import { totp } from './totp';

const baseURL = process.env['KOTATSU_E2E_URL'] ?? '';
const controlURL = process.env['KOTATSU_E2E_IDP_CONTROL_URL'] ?? '';
// 初回セットアップで作った管理者のユーザー名 (flows.spec.ts)。自動で作るユーザーと重なる場面に使う。
const localAdminUsername = 'e2e-admin';

// Identity は、偽の IdP が次に発行する ID トークンのユーザー。
type Identity = {
  subject: string;
  username: string;
  name: string;
  groups?: string[];
  // omitGroups なら、グループの claim を載せない。
  omitGroups?: boolean;
};

async function nextIdentity(identity: Identity): Promise<void> {
  const response = await fetch(`${controlURL}/next`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ groups: [], omitGroups: false, ...identity }),
  });
  expect(response.status).toBe(204);
}

async function resetIdentity(): Promise<void> {
  const response = await fetch(`${controlURL}/reset`, { method: 'POST' });
  expect(response.status).toBe(204);
}

// Client は、Kotatsu に送る Cookie を自分で持つ、素の HTTP のクライアント。リダイレクトは追わない。
class Client {
  readonly cookies = new Map<string, string>();

  async send(path: string, init: RequestInit & { origin?: string | null } = {}): Promise<Response> {
    const headers = new Headers(init.headers);
    if (this.cookies.size > 0) {
      headers.set(
        'Cookie',
        [...this.cookies].map(([name, value]) => `${name}=${value}`).join('; '),
      );
    }
    // 状態を変える POST には、同じオリジンの Origin を付ける。origin に null を渡せば付けない。
    const origin = init.origin === undefined ? baseURL : init.origin;
    if (init.method === 'POST' && origin !== null) {
      headers.set('Origin', origin);
    }
    const response = await fetch(new URL(path, baseURL), { ...init, headers, redirect: 'manual' });
    for (const header of response.headers.getSetCookie()) {
      const [pair, ...attributes] = header.split(';');
      const name = pair!.slice(0, pair!.indexOf('=')).trim();
      const value = pair!.slice(pair!.indexOf('=') + 1).trim();
      // 空の値、Max-Age=0、過去の Expires は、Cookie を消す。
      const expired = attributes.some((attribute) => {
        const [key, raw] = attribute.split('=').map((part) => part.trim());
        return (
          (key?.toLowerCase() === 'max-age' && Number(raw) <= 0) ||
          (key?.toLowerCase() === 'expires' && Date.parse(raw ?? '') <= Date.now())
        );
      });
      if (value === '' || expired) {
        this.cookies.delete(name);
      } else {
        this.cookies.set(name, value);
      }
    }
    return response;
  }

  postJSON(path: string, body: unknown, origin?: string | null): Promise<Response> {
    return this.send(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
      origin,
    });
  }

  async graphql<T>(query: string, variables: Record<string, unknown> = {}): Promise<T> {
    const response = await this.postJSON('/graphql', { query, variables });
    const result = (await response.json()) as { data?: T; errors?: unknown[] };
    expect(result.errors, JSON.stringify(result.errors)).toBeUndefined();
    return result.data!;
  }

  // viewerStatus は、ログインしているかを、GraphQL の viewer の応答の状態で返す。
  async viewerStatus(): Promise<number> {
    const response = await this.postJSON('/graphql', { query: '{ viewer { id } }' });
    const result = (await response.json()) as { errors?: { extensions?: { code?: string } }[] };
    return result.errors?.some((error) => error.extensions?.code === 'UNAUTHENTICATED')
      ? 401
      : response.status;
  }
}

// begin は、OIDC でのログイン (url の既定は /auth/oidc) を始め、IdP の認可のエンドポイントの URL を返す。
async function begin(client: Client, path = '/auth/oidc', body: unknown = {}): Promise<URL> {
  const response = await client.postJSON(path, body);
  expect(response.status, await response.clone().text()).toBe(200);
  const { authorizationUrl } = (await response.json()) as { authorizationUrl: string };
  return new URL(authorizationUrl);
}

// authorize は、IdP の認可のエンドポイントを開き、Kotatsu に戻るリダイレクト先を返す。
async function authorize(authorizationURL: URL): Promise<URL> {
  const response = await fetch(authorizationURL, { redirect: 'manual' });
  expect(response.status).toBe(302);
  return new URL(response.headers.get('Location')!);
}

// callback は、IdP から Kotatsu に戻り、Kotatsu が返したリダイレクト先 (同じオリジンのパスとクエリ) を返す。
async function callback(client: Client, redirect: URL): Promise<string> {
  const response = await client.send(redirect.pathname + redirect.search);
  expect(response.status).toBe(303);
  return response.headers.get('Location')!;
}

// login は、identity で、始めから終わりまで OIDC でログインし、Kotatsu が返したリダイレクト先を返す。
async function login(client: Client, identity: Identity): Promise<string> {
  await nextIdentity(identity);
  return callback(client, await authorize(await begin(client)));
}

test.describe.configure({ mode: 'serial' });

test.afterEach(async () => {
  // 振る舞いを変えたまま終えると、後のテストが落ちる。
  await resetIdentity();
});

test('ログインのボタンから、IdP を通ってログインする', async ({ page }) => {
  await nextIdentity({
    subject: 'e2e-oidc-browser',
    username: 'e2e-oidc-browser',
    name: 'ブラウザの OIDC ユーザー',
    groups: ['kotatsu-members'],
  });
  await page.goto('/login');
  await page.getByRole('button', { name: 'E2E の IdPでログイン' }).click();
  await page.waitForURL((url) => url.pathname === '/');
  await expect(page.getByRole('button', { name: 'ブラウザの OIDC ユーザー' })).toBeVisible();
});

test('ログインの始まりで、PKCE と state と nonce を付けて IdP へ送り、途中の控えを Cookie に入れる', async () => {
  const client = new Client();
  const url = await begin(client);
  expect(url.searchParams.get('client_id')).toBe('kotatsu');
  expect(url.searchParams.get('redirect_uri')).toBe(`${baseURL}/auth/oidc/callback`);
  expect(url.searchParams.get('response_type')).toBe('code');
  expect(url.searchParams.get('code_challenge_method')).toBe('S256');
  expect(url.searchParams.get('code_challenge')).toBeTruthy();
  expect(url.searchParams.get('state')).toBeTruthy();
  expect(url.searchParams.get('nonce')).toBeTruthy();
  expect(client.cookies.has('kotatsu_oidc')).toBe(true);
});

test('IdP から戻ると、セッションの Cookie を付けて / へ戻し、途中の控えを消す', async () => {
  const client = new Client();
  const location = await login(client, {
    subject: 'e2e-oidc-member',
    username: 'e2e-oidc-member',
    name: 'OIDC のメンバー',
    groups: ['kotatsu-members'],
  });
  expect(location).toBe('/');
  expect(client.cookies.has('kotatsu_session')).toBe(true);
  expect(client.cookies.has('kotatsu_oidc')).toBe(false);
  const { viewer } = await client.graphql<{ viewer: { username: string; role: string } }>(
    '{ viewer { username role } }',
  );
  expect(viewer).toEqual({ username: 'e2e-oidc-member', role: 'MEMBER' });
});

test('state が違えば断り、セッションを作らずに途中の控えを消す', async () => {
  const client = new Client();
  await nextIdentity({
    subject: 'e2e-oidc-state',
    username: 'e2e-oidc-state',
    name: 'state',
    groups: ['kotatsu-members'],
  });
  const redirect = await authorize(await begin(client));
  redirect.searchParams.set('state', 'tampered');
  expect(await callback(client, redirect)).toBe('/login?error=oidc_invalid');
  expect(client.cookies.has('kotatsu_session')).toBe(false);
  expect(client.cookies.has('kotatsu_oidc')).toBe(false);
});

test('途中の控えの Cookie がないまま戻ると、断る', async () => {
  const client = new Client();
  await nextIdentity({
    subject: 'e2e-oidc-nocookie',
    username: 'e2e-oidc-nocookie',
    name: 'no cookie',
    groups: ['kotatsu-members'],
  });
  const redirect = await authorize(await begin(client));
  client.cookies.delete('kotatsu_oidc');
  expect(await callback(client, redirect)).toBe('/login?error=oidc_expired');
  expect(client.cookies.has('kotatsu_session')).toBe(false);
});

test('自動で作るユーザーには、当たったグループのうち強い方のロールを割り当てる', async () => {
  const client = new Client();
  expect(
    await login(client, {
      subject: 'e2e-oidc-admin',
      username: 'e2e-oidc-admin',
      name: 'OIDC の管理者',
      groups: ['kotatsu-viewers', 'kotatsu-admins', 'kotatsu-members'],
    }),
  ).toBe('/');
  const { viewer } = await client.graphql<{ viewer: { role: string } }>('{ viewer { role } }');
  expect(viewer.role).toBe('ADMIN');
});

test('どのグループにも当たらなければ、自動で作らずに断る', async () => {
  const client = new Client();
  expect(
    await login(client, {
      subject: 'e2e-oidc-outsider',
      username: 'e2e-oidc-outsider',
      name: '外の人',
      groups: ['someone-else'],
    }),
  ).toBe('/login?error=oidc_denied');
  expect(client.cookies.has('kotatsu_session')).toBe(false);
});

test('グループの claim がなければ、自動で作らずに断る', async () => {
  const client = new Client();
  expect(
    await login(client, {
      subject: 'e2e-oidc-nogroups',
      username: 'e2e-oidc-nogroups',
      name: 'グループなし',
      omitGroups: true,
    }),
  ).toBe('/login?error=oidc_denied');
});

test('自動で作るユーザーが、既存のユーザー名と重なれば、作らずに断る', async () => {
  const client = new Client();
  expect(
    await login(client, {
      subject: 'e2e-oidc-collision',
      username: localAdminUsername,
      name: '重なる人',
      groups: ['kotatsu-members'],
    }),
  ).toBe('/login?error=oidc_username_taken');
});

test('無効にしたユーザーは、IdP で通っても断る', async () => {
  const target = {
    subject: 'e2e-oidc-disabled',
    username: 'e2e-oidc-disabled',
    name: '無効にする人',
    groups: ['kotatsu-members'],
  };
  const user = new Client();
  expect(await login(user, target)).toBe('/');
  const { viewer } = await user.graphql<{ viewer: { id: string } }>('{ viewer { id } }');

  // 自動で作った管理者で、ログインした直後 (直近 10 分以内の認証) に無効にする。
  const admin = new Client();
  await login(admin, {
    subject: 'e2e-oidc-admin',
    username: 'e2e-oidc-admin',
    name: 'OIDC の管理者',
    groups: ['kotatsu-admins'],
  });
  await admin.graphql('mutation($id: ID!) { disableUser(id: $id) { id disabled } }', {
    id: viewer.id,
  });

  // 無効にした時点から、今のセッションも使えない。
  expect(await user.viewerStatus()).toBe(401);
  expect(await login(new Client(), target)).toBe('/login?error=oidc_denied');
});

test('設定の画面からの紐付けは、ほかのユーザーと紐付いた subject を断り、紐付けたら linked=oidc を付けて戻す', async () => {
  // OIDC と紐付いていないローカルのユーザーを、自動で作った管理者で作る。
  const admin = new Client();
  await login(admin, {
    subject: 'e2e-oidc-admin',
    username: 'e2e-oidc-admin',
    name: 'OIDC の管理者',
    groups: ['kotatsu-admins'],
  });
  const password = 'kotatsu-e2e-cedar-harbor-violet-17';
  await admin.graphql('mutation($input: CreateUserInput!) { createUser(input: $input) { id } }', {
    input: {
      username: 'e2e-local',
      displayName: 'E2E のローカルユーザー',
      role: 'MEMBER',
      password,
    },
  });

  // パスワードでログインし、その場で認証アプリを登録する (第二要素は必須)。ログインした直後なので、紐付けに要る再認証を満たす。
  const local = new Client();
  expect((await local.postJSON('/auth/login', { username: 'e2e-local', password })).status).toBe(
    200,
  );
  const enrollment = await local.postJSON('/auth/login/totp/enrollment', {});
  const { secret } = (await enrollment.json()) as { secret: string };
  const confirmed = await local.postJSON('/auth/login/totp/enrollment/confirm', {
    code: totp(secret),
  });
  expect(confirmed.status, await confirmed.clone().text()).toBe(200);
  expect(local.cookies.has('kotatsu_session')).toBe(true);

  // 戻すパスは、/ で始まり // で始まらないものだけを受け付ける。外のホストへは出さない。
  expect((await local.postJSON('/auth/oidc/link', { returnTo: '//evil.example/' })).status).toBe(
    400,
  );
  expect(
    (await local.postJSON('/auth/oidc/link', { returnTo: 'https://evil.example/' })).status,
  ).toBe(400);

  // 既にほかのユーザー (自動で作った管理者) と紐付いた subject は断る。
  await nextIdentity({
    subject: 'e2e-oidc-admin',
    username: 'e2e-oidc-admin',
    name: 'OIDC の管理者',
    groups: ['kotatsu-admins'],
  });
  let redirect = await authorize(await begin(local, '/auth/oidc/link', { returnTo: '/settings' }));
  expect(await callback(local, redirect)).toBe('/settings?error=oidc_link_taken');

  await nextIdentity({
    subject: 'e2e-local-idp',
    username: 'e2e-local-idp',
    name: 'ローカルの IdP',
    groups: [],
  });
  redirect = await authorize(await begin(local, '/auth/oidc/link', { returnTo: '/settings' }));
  expect(await callback(local, redirect)).toBe('/settings?linked=oidc');

  // 紐付けた後は、IdP でそのユーザーとしてログインできる。グループに当たらなくても、作るわけではないので通る。
  const viaIdP = new Client();
  expect(
    await login(viaIdP, {
      subject: 'e2e-local-idp',
      username: 'e2e-local-idp',
      name: 'ローカルの IdP',
      groups: [],
    }),
  ).toBe('/');
  const { viewer } = await viaIdP.graphql<{ viewer: { username: string } }>(
    '{ viewer { username } }',
  );
  expect(viewer.username).toBe('e2e-local');
});

test('状態を変える /auth/* の POST は、ほかのオリジンと Origin のないものを断る', async () => {
  const client = new Client();
  expect((await client.postJSON('/auth/oidc', {}, 'http://evil.example')).status).toBe(403);
  expect((await client.postJSON('/auth/oidc', {}, null)).status).toBe(403);
  expect(client.cookies.has('kotatsu_oidc')).toBe(false);
});

test('ログアウトすると、セッションが消えて、再びログインが要る。GET ではログアウトしない', async () => {
  const client = new Client();
  await login(client, {
    subject: 'e2e-oidc-member',
    username: 'e2e-oidc-member',
    name: 'OIDC のメンバー',
    groups: ['kotatsu-members'],
  });
  // よそのページの画像のタグなどから、ログアウトさせられないようにする。
  expect((await client.send('/auth/logout')).status).toBe(405);
  expect(await client.viewerStatus()).toBe(200);

  expect((await client.send('/auth/logout', { method: 'POST' })).status).toBeLessThan(400);
  expect(client.cookies.has('kotatsu_session')).toBe(false);
  expect(await client.viewerStatus()).toBe(401);
});

test('今のセッションのほかをまとめて失効させると、ほかのセッションだけが入り直しになる', async () => {
  const identity = {
    subject: 'e2e-oidc-sessions',
    username: 'e2e-oidc-sessions',
    name: '2 つの端末',
    groups: ['kotatsu-members'],
  };
  const first = new Client();
  const second = new Client();
  await login(first, identity);
  await login(second, identity);

  const { revokeOtherSessions } = await first.graphql<{ revokeOtherSessions: number }>(
    'mutation { revokeOtherSessions }',
  );
  // 一式を共有するので、件数は決め打ちしない。
  expect(revokeOtherSessions).toBeGreaterThanOrEqual(1);
  expect(await first.viewerStatus()).toBe(200);
  expect(await second.viewerStatus()).toBe(401);
});
