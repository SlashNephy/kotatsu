// 不具合の報告を作るときに、ブラウザで伏せる規則 (2 段目、#1521、設計書 11.3)。
// サーバーが知っている秘密の値は、画面に送る前にサーバーが伏せてある (1 段目)。ここでは、家に辿り着ける情報と、
// 形で分かる秘密を伏せる。ホスト名は、本人が自分の家の構成を見るのに要るので、画面ではそのまま出し、報告にだけかける。
// 規則は設計書の表の順に当てる。印をさらに伏せないので、何度かけても結果は変わらない。

// marks は、伏せたものの代わりに置く印。何かを伏せたことと、伏せたものの種類が、読み手に分かるようにする。
export const marks = {
  secret: '[REDACTED_SECRET]',
  host: '[REDACTED_HOST]',
  webhook: '[REDACTED_WEBHOOK]',
  email: '[REDACTED_EMAIL]',
  card: '[REDACTED_CARD]',
  ip: '[REDACTED_IP]',
  user: '[REDACTED_USER]',
} as const;

// 決めるパラメーター (設計書 付録 A)。
// minSecretLength は、伏せる秘密の値の最短の長さ。サーバーの 1 段目 (redact.MinSecretLength) とそろえる。
export const minSecretLength = 8;
// minHostLength は、サーバーから渡されたホスト名を当てる最短の長さ。
export const minHostLength = 4;
// cardDigits は、カード番号とみなす数字の桁数の範囲 (ISO/IEC 7812 の番号のうち、発行されている長さ)。
export const cardDigits = { min: 13, max: 19 } as const;
// minPortDigits は、「点の無い名前:ポート」のポートとみなす数字の最少の桁数。
export const minPortDigits = 2;
// minHostLetters は、「点の無い名前:ポート」の名前に要る英字の数。
export const minHostLetters = 2;

// publicHosts は、伏せないホスト。
export const publicHosts = ['localhost', 'github.com', 'raw.githubusercontent.com', 'ghcr.io'];

// markPattern は、伏せた後の印。設定の秘密の値は、サーバーのログで [REDACTED] になっている (config.Secret)。
const markPattern = /^\[REDACTED(?:_[A-Z]+)?\]$/;
const isMark = (value: string) => markPattern.test(value);

const escapeRegExp = (value: string) => value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

const isPublicHost = (host: string) => publicHosts.includes(host.toLowerCase());

// ipv4Shape は、IPv4 の形。
const ipv4Shape = /^\d{1,3}(?:\.\d{1,3}){3}$/;

// byLength は、長いものから順に並べ、同じ値を 1 つにまとめる。
function byLength(values: string[]): string[] {
  return [...new Set(values.map((value) => value.trim().toLowerCase()))].sort(
    (a, b) => b.length - a.length || a.localeCompare(b),
  );
}

// 1. サーバーから渡された秘密の値。大文字と小文字を区別しない部分一致。
function redactSecrets(text: string, secrets: string[]): string {
  const values = byLength(secrets).filter(
    (value) => value.length >= minSecretLength && !marks.secret.toLowerCase().includes(value),
  );
  for (const value of values) {
    text = text.replace(new RegExp(escapeRegExp(value), 'gi'), marks.secret);
  }

  return text;
}

// 2. サーバーから渡されたホスト名。前後が英数字・点・ハイフンでないところだけに当てる。
// 公開ホスト、IP アドレスの形、`:` を含むもの、`[` で始まるものは、ほかの規則に任せる。
function redactHosts(text: string, hosts: string[]): string {
  const values = byLength(hosts).filter(
    (host) =>
      host.length >= minHostLength &&
      !isPublicHost(host) &&
      !ipv4Shape.test(host) &&
      !host.includes(':') &&
      !host.startsWith('['),
  );
  for (const host of values) {
    // 文の終わりの点は、ホスト名の一部ではないので、後ろに続いてよい。
    const pattern = new RegExp(
      `(?<![A-Za-z0-9.-])${escapeRegExp(host)}(?![A-Za-z0-9-]|\\.[A-Za-z0-9])`,
      'gi',
    );
    text = text.replace(pattern, marks.host);
  }

  return text;
}

// urlChars は、URL の中に続く字。空白、引用符、山括弧、バックスラッシュ (JSON の \" の頭) で終わる。
const urlChars = String.raw`[^\s"'<>\\` + '`' + ']';

// 3. Webhook の URL (Discord、Slack、パスに hook か hooks の語を含む http(s) の URL)。URL の全体を伏せる。
const webhookPatterns = [
  new RegExp(
    String.raw`https?://(?:[a-z0-9-]+\.)*discord(?:app)?\.com/api/webhooks/${urlChars}*`,
    'gi',
  ),
  new RegExp(String.raw`https?://hooks\.slack\.com/${urlChars}*`, 'gi'),
  new RegExp(
    String.raw`https?://${urlChars}*?/(?:web)?hooks?(?=[/?#\s"'<>\\]|$)${urlChars}*`,
    'gi',
  ),
];

// 4. URL の中の資格情報 (scheme://ユーザー:パスワード@)。資格の部分だけを伏せる。
const credentialsPattern = /\b([a-z][a-z0-9+.-]*:\/\/)[^\s/@:"'<>\\]+:[^\s/@"'<>\\]*@/gi;

// 5. Authorization、Proxy-Authorization、Cookie、Set-Cookie の値。行の終わりまで伏せる。
const headerPattern =
  /\b(proxy-authorization|authorization|set-cookie|cookie)(["']?\s*[:=]\s*)(?!\[REDACTED_SECRET\]$)\S.*$/gim;

// 6. Bearer に続く値。「Bearer scheme」のような語と見分けるため、秘密の最短の長さ以上で、数字か記号を含むものに限る。
const bearerPattern = /\b(Bearer)\s+([A-Za-z0-9._~+/=-]+)/gi;
const looksLikeToken = (value: string) =>
  value.length >= minSecretLength && /[0-9._~+/=-]/.test(value);

// 7. Kotatsu が発行するトークン。接頭辞の後ろに、32 バイトの乱数を base64url にした 43 字が続く。
// メトリクスの名前 (kotatsu_storage_pool_last_measured_timestamp_seconds など) も同じ接頭辞で、後ろが 43 字以上のものもある。
// メトリクスの名前は小文字と _ だけなので、大文字か数字を含むものだけをトークンとみなす。
const kotatsuTokenPattern =
  /(?<![A-Za-z0-9_-])kotatsu_(?:(?:at|rt|ac|cal|iptv)_)?[A-Za-z0-9_-]{43}(?![A-Za-z0-9_-])/g;
const looksRandom = (token: string) => /[A-Z0-9]/.test(token.replace(/^kotatsu_/, ''));

// 8. 名前に秘密の語を含む「名前=値」「名前: 値」の値。
const secretName = String.raw`[\w.-]*(?:secret|token|password|passwd|pwd|api[ _-]?key|private[ _-]?key|credential|session[ _-]?id)[\w.-]*`;
// JSON の形 ("名前":"値")。
const jsonSecretPattern = new RegExp(String.raw`("${secretName}"\s*:\s*)"((?:[^"\\]|\\.)*)"`, 'gi');
// key=value と key: value の形。値が引用符で囲まれていれば、中に空白があっても、閉じる引用符まで伏せる。
const plainSecretPattern = new RegExp(
  String.raw`(?<![\w.-])(${secretName})(\s*[=:]\s*)(\[REDACTED(?:_[A-Z]+)?\]|"(?:[^"\\]|\\.)*"|'[^']*'|[^\s,;&"'}\])]+)`,
  'gi',
);

function redactNamedSecrets(text: string): string {
  text = text.replace(jsonSecretPattern, (whole, head: string, value: string) =>
    value === '' || isMark(value) ? whole : `${head}"${marks.secret}"`,
  );

  return text.replace(
    plainSecretPattern,
    (whole, name: string, separator: string, value: string) => {
      const quote = value.startsWith('"') || value.startsWith("'") ? value[0] : '';
      const inner = quote === '' ? value : value.slice(1, -1);
      if (inner === '' || isMark(inner)) {
        return whole;
      }

      return `${name}${separator}${quote}${marks.secret}${quote}`;
    },
  );
}

// 9. URL のクエリで、値そのものが鍵として働くもの。? か & の直後の名前だけに当てる。
const queryPattern =
  /([?&](?:code|state|nonce|token|access_token|id_token|refresh_token|key|sig|signature)=)([^&#\s"'<>\\]+)/gi;

// 10. メールアドレス。
const emailPattern =
  /(?<![A-Za-z0-9._%+-])[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}(?![A-Za-z0-9-])/g;

// 11. カード番号。
// 4 桁ずつ空白かハイフンで区切ったもの (と、American Express などの 4-6-5、4-6-4) は、区切りの形と Luhn の検査で見分ける。
// 4 桁の数字を空白で並べたもの (Go の %v で出したサービス ID の並びなど) の 9 割は、Luhn の検査で外れる。
// UUID (8-4-4-4-12) の途中に当たらないよう、前後に英数字や「英数字-」が続くものは除く。
const separatedCardPattern =
  /(?<![0-9A-Za-z.]|[0-9A-Za-z]-)\d{4}([ -])(?:\d{4}\1\d{4}\1\d{1,4}(?:\1\d{1,3})?|\d{6}\1\d{4,5})(?![0-9A-Za-z.]|-[0-9A-Za-z])/g;
// 区切りの無いものは、桁数のほか、Luhn の検査、発行者の番号 (IIN) と長さの組、番組の ID の形でないことを見る。
const plainCardPattern = /(?<![0-9A-Za-z.])\d{13,19}(?![0-9A-Za-z.])/g;

// luhn は、Luhn の検査を通るかどうかを返す。
export function luhn(digits: string): boolean {
  let sum = 0;
  for (let i = 0; i < digits.length; i++) {
    let digit = Number(digits[digits.length - 1 - i]);
    if (i % 2 === 1) {
      digit *= 2;
      if (digit > 9) {
        digit -= 9;
      }
    }
    sum += digit;
  }

  return sum % 10 === 0;
}

// issuers は、主な国際ブランドの発行者の番号の範囲と、番号の長さ。
const issuers: { from: number; to: number; prefixLength: number; lengths: number[] }[] = [
  // Visa
  { from: 4, to: 4, prefixLength: 1, lengths: [13, 16, 19] },
  // Mastercard
  { from: 51, to: 55, prefixLength: 2, lengths: [16] },
  { from: 2221, to: 2720, prefixLength: 4, lengths: [16] },
  // American Express
  { from: 34, to: 34, prefixLength: 2, lengths: [15] },
  { from: 37, to: 37, prefixLength: 2, lengths: [15] },
  // JCB
  { from: 3528, to: 3589, prefixLength: 4, lengths: [16, 17, 18, 19] },
  // Diners Club
  { from: 300, to: 305, prefixLength: 3, lengths: [14, 15, 16, 17, 18, 19] },
  { from: 36, to: 36, prefixLength: 2, lengths: [14, 15, 16, 17, 18, 19] },
  { from: 38, to: 39, prefixLength: 2, lengths: [16, 17, 18, 19] },
  // Discover
  { from: 6011, to: 6011, prefixLength: 4, lengths: [16, 17, 18, 19] },
  { from: 644, to: 649, prefixLength: 3, lengths: [16, 17, 18, 19] },
  { from: 65, to: 65, prefixLength: 2, lengths: [16, 17, 18, 19] },
  // UnionPay
  { from: 62, to: 62, prefixLength: 2, lengths: [16, 17, 18, 19] },
];

const hasIssuer = (digits: string) =>
  issuers.some(({ from, to, prefixLength, lengths }) => {
    const prefix = Number(digits.slice(0, prefixLength));

    return lengths.includes(digits.length) && prefix >= from && prefix <= to;
  });

// maxSubID は、番組の ID の中のネットワーク ID、サービス ID、イベント ID の最大 (16 ビット)。
const maxSubID = 0xffff;

// isProgramID は、Kotatsu (Mirakurun) の番組の ID の形かどうかを返す。
// 番組の ID は (networkId × 100000 + serviceId) × 100000 + eventId なので、下の 5 桁ずつが、それぞれ 16 ビットに収まる。
export function isProgramID(digits: string): boolean {
  if (digits.length < 11 || digits.length > 15) {
    return false;
  }
  const eventID = Number(digits.slice(-5));
  const serviceID = Number(digits.slice(-10, -5));
  const networkID = Number(digits.slice(0, -10));

  return networkID >= 1 && networkID <= maxSubID && serviceID <= maxSubID && eventID <= maxSubID;
}

// isCardNumber は、区切りの無い数字の並びを、カード番号とみなすかどうかを返す。
export function isCardNumber(digits: string): boolean {
  return (
    digits.length >= cardDigits.min &&
    digits.length <= cardDigits.max &&
    luhn(digits) &&
    hasIssuer(digits) &&
    !isProgramID(digits)
  );
}

function redactCards(text: string): string {
  text = text.replace(separatedCardPattern, (whole) => {
    const digits = whole.replace(/\D/g, '');

    return digits.length >= cardDigits.min && digits.length <= cardDigits.max && luhn(digits)
      ? marks.card
      : whole;
  });

  return text.replace(plainCardPattern, (whole) => (isCardNumber(whole) ? marks.card : whole));
}

// 12. IPv6 (:: を含む短い形と 8 組の形。%ゾーンを含めて)。C++ の名前 (a::b) と見分けるため、数字を含むときだけ伏せる。
const hex = '[0-9A-Fa-f]{1,4}';
const ipv6Pattern = new RegExp(
  String.raw`(?<![0-9A-Za-z:.])(?:(?:${hex}:){7}${hex}|(?:${hex}(?::${hex})*)?::(?:${hex}(?::${hex})*)?(?::\d{1,3}(?:\.\d{1,3}){3})?)(?:%[0-9A-Za-z._-]+)?(?![0-9A-Za-z:])`,
  'g',
);

// 13. IPv4。ポートと CIDR の長さは、当てる範囲の外なので残る。
// 英字＋/ の直後 (Chrome/131.0.0.0 のような製品の版) と、直後に -英字 が続くもの (カーネルの版) は除く。
const octet = String.raw`(?:25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)`;
const ipv4Pattern = new RegExp(
  String.raw`(?<![\d.])(?<![A-Za-z]/)${octet}(?:\.${octet}){3}(?!\.?\d|-[A-Za-z])`,
  'g',
);

// 14. URL のホスト部。
const urlHostPattern = /\b([a-z][a-z0-9+.-]*:\/\/(?:[^\s/@"'<>\\]*@)?)([^/\s:?#"'<>\\[\]@]+)/gi;

// 15. 「点の無い名前:ポート」のホスト名。後ろにさらに :数字 が続くもの (時刻、行と列) と、ホストでない語は除く。
const hostPortPattern = new RegExp(
  String.raw`(?<![A-Za-z0-9_.\-/:@\[])([A-Za-z0-9][A-Za-z0-9_-]*):(\d{${minPortDigits},5})(?!\d|:\d|\.\d)`,
  'g',
);
// notHostWords は、「名前:数字」の名前が、ホストでない語。
const notHostWords = new Set([
  'line',
  'lines',
  'col',
  'column',
  'code',
  'status',
  'port',
  'exit',
  'error',
  'err',
  'pid',
  'id',
  'row',
  'step',
  'retry',
  'retries',
  'attempt',
  'http',
  'https',
  'tcp',
  'udp',
  'ws',
  'wss',
  'count',
  'size',
  'len',
  'offset',
  'index',
  'page',
  'level',
  'version',
  'priority',
  'channel',
  'service',
  'event',
  'segment',
  'frame',
  'job',
  'tuner',
  'pos',
  'at',
]);

function redactHostPorts(text: string): string {
  return text.replace(hostPortPattern, (whole, name: string, port: string) => {
    const letters = name.replace(/[^A-Za-z]/g, '').length;
    if (letters < minHostLetters || notHostWords.has(name.toLowerCase()) || Number(port) > 65535) {
      return whole;
    }

    return `${marks.host}:${port}`;
  });
}

// 16. 裸のドメイン名のうち、末尾がよくあるもの。.ts や .app は、ファイル名と見分けるために入れない。
const commonTLDs = [
  'local',
  'lan',
  'home',
  'internal',
  'localdomain',
  'arpa',
  'intranet',
  'corp',
  'private',
  'test',
  'example',
  'io',
  'com',
  'net',
  'org',
  'jp',
  'dev',
  'cloud',
  'info',
  'xyz',
];
const domainPattern = new RegExp(
  String.raw`(?<![A-Za-z0-9_.@-])(?:[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?\.)+(?:${commonTLDs.join('|')})(?![A-Za-z0-9-]|\.[A-Za-z0-9])`,
  'gi',
);

// 17. ホームフォルダのユーザー名 (/Users/名前、/home/名前、C:\Users\名前)。JSON の \\ も含める。
const homePattern = /(\/Users\/|\/home\/|\b[A-Za-z]:(?:\\\\|\\)Users(?:\\\\|\\))([^/\\\s"'<>:]+)/g;

// RedactOptions は、サーバーから渡されたもの。
export type RedactOptions = {
  // secrets は、伏せる秘密の値。サーバーは、秘密の値をブラウザに渡さないので、ふつうは空。
  secrets: readonly string[];
  // hosts は、伏せるホスト名 (リクエストが来たホスト名と、チューナーサーバーなどのホスト名)。
  hosts: readonly string[];
};

// redact は、text の秘密の値と、家に辿り着ける情報を、設計書の表の順に伏せる。
export function redact(text: string, { secrets, hosts }: RedactOptions): string {
  text = redactSecrets(text, [...secrets]);
  text = redactHosts(text, [...hosts]);
  for (const pattern of webhookPatterns) {
    text = text.replace(pattern, marks.webhook);
  }
  text = text.replace(credentialsPattern, `$1${marks.secret}@`);
  text = text.replace(headerPattern, `$1$2${marks.secret}`);
  text = text.replace(bearerPattern, (whole, bearer: string, value: string) =>
    looksLikeToken(value) ? `${bearer} ${marks.secret}` : whole,
  );
  text = text.replace(kotatsuTokenPattern, (token) => (looksRandom(token) ? marks.secret : token));
  text = redactNamedSecrets(text);
  text = text.replace(queryPattern, (whole, head: string, value: string) =>
    isMark(value) ? whole : `${head}${marks.secret}`,
  );
  text = text.replace(emailPattern, marks.email);
  text = redactCards(text);
  text = text.replace(ipv6Pattern, (address) => (/\d/.test(address) ? marks.ip : address));
  text = text.replace(ipv4Pattern, marks.ip);
  text = text.replace(urlHostPattern, (whole, scheme: string, host: string) =>
    isPublicHost(host) ? whole : `${scheme}${marks.host}`,
  );
  text = redactHostPorts(text);
  text = text.replace(domainPattern, (domain) => (isPublicHost(domain) ? domain : marks.host));
  text = text.replace(homePattern, (whole, home: string, name: string) =>
    isMark(name) ? whole : `${home}${marks.user}`,
  );

  return text;
}
