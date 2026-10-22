// ログインを終えたときに戻す画面のパスを、ログインの手続きの間だけ覚える。
// MCP のクライアントの同意の画面 (#965) のように、ログインしていない状態で開かれた画面へ、ログインした後に戻すのに使う。
// パスワード、パスキー、OIDC のどれでログインしても使えるよう、ブラウザのタブの sessionStorage に持つ。
// OIDC では、IdP から戻ると、サーバーがアプリの最初の画面へリダイレクトするので、最初の画面でも取り出す。

const storageKey = 'kotatsu.returnTo';
// returnToLifetime は、覚えたパスを使える長さ。ログインをやめた後に、関係のない場面で戻されないようにする。
const returnToLifetime = 10 * 60 * 1000;

type Stored = { path: string; at: number };

// isSafePath は、同じオリジンの画面のパスかどうかを返す。ほかのサイトへ移る開いたリダイレクトにしない。
export function isSafePath(path: string): boolean {
  return path.startsWith('/') && !path.startsWith('//') && !path.startsWith('/\\');
}

// rememberReturnTo は、ログインを終えたときに戻すパスを覚える。
export function rememberReturnTo(path: string, now: number = Date.now()): void {
  if (!isSafePath(path)) {
    return;
  }
  try {
    sessionStorage.setItem(storageKey, JSON.stringify({ path, at: now } satisfies Stored));
  } catch {
    // 覚えられなければ、ログインの後は最初の画面に戻る。
  }
}

// consumeReturnTo は、覚えたパスを取り出して忘れる。覚えていないか、古くなっていれば null を返す。
export function consumeReturnTo(now: number = Date.now()): string | null {
  let raw: string | null;
  try {
    raw = sessionStorage.getItem(storageKey);
    sessionStorage.removeItem(storageKey);
  } catch {
    return null;
  }
  if (raw === null) {
    return null;
  }
  try {
    const stored = JSON.parse(raw) as Partial<Stored>;
    if (
      typeof stored.path !== 'string' ||
      typeof stored.at !== 'number' ||
      now - stored.at > returnToLifetime ||
      !isSafePath(stored.path)
    ) {
      return null;
    }

    return stored.path;
  } catch {
    return null;
  }
}

// afterLoginURL は、ログインを終えたときに移る URL を返す。覚えたパスがなければ、アプリの最初の画面にする。
export function afterLoginURL(): string {
  return consumeReturnTo() ?? import.meta.env.BASE_URL;
}
