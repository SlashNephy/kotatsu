import { describe, expect, test } from 'vitest';

import { isCardNumber, isProgramID, luhn, marks, redact } from './redact';

const noOptions = { secrets: [], hosts: [] };

// cases は、設計書 11.3 の 2 段目の表の行ごとの、伏せるべき例と伏せてはいけない例。どの例も、2 回かけても変わらないことを確かめる。
const cases: { rule: string; input: string; want: string; secrets?: string[]; hosts?: string[] }[] =
  [
    // 1. サーバーから渡された秘密の値
    {
      rule: '1 秘密の値は大文字と小文字を区別せずに伏せる',
      input: 'token=ABCDEFGHIJ and abcdefghij',
      secrets: ['abcdefghij'],
      want: `token=${marks.secret} and ${marks.secret}`,
    },
    {
      rule: '1 長い秘密から順に伏せる',
      input: 'x password1-extra y password1',
      secrets: ['password1', 'password1-extra'],
      want: `x ${marks.secret} y ${marks.secret}`,
    },
    {
      rule: '1 最短の長さに満たない値は使わない',
      input: 'news is good',
      secrets: ['news'],
      want: 'news is good',
    },
    // 2. 渡されたホスト名
    {
      rule: '2 渡されたホスト名を伏せる',
      input: 'connect to Tuner-Box failed; tuner-box.',
      hosts: ['tuner-box'],
      want: `connect to ${marks.host} failed; ${marks.host}.`,
    },
    {
      rule: '2 語の一部や、点やハイフンでつながったものには当てない',
      input: 'mytuner-box tuner-box2 tuner-box.example x-tuner-box',
      hosts: ['tuner-box'],
      want: `mytuner-box tuner-box2 ${marks.host} x-tuner-box`,
    },
    {
      rule: '2 公開ホスト、短すぎる名前、IP の形は、渡されても使わない',
      input: 'localhost nas 192.0.2.1',
      hosts: ['localhost', 'nas', '192.0.2.1', '[::1]', 'host:8080'],
      want: `localhost nas ${marks.ip}`,
    },
    // 3. Webhook の URL
    {
      rule: '3 Discord の Webhook の URL を全体で伏せる',
      input: 'POST https://discord.com/api/webhooks/123/abc-DEF failed',
      want: `POST ${marks.webhook} failed`,
    },
    {
      rule: '3 Slack の Webhook と、パスに hooks の語を含む URL',
      input:
        'https://hooks.slack.com/services/T0/B0/xyz and https://ntfy.example.test/hooks/abc?x=1',
      want: `${marks.webhook} and ${marks.webhook}`,
    },
    {
      rule: '3 hooks を語の一部に含むだけの URL は Webhook とみなさない',
      input: 'https://github.com/SlashNephy/kotatsu/blob/main/frontend/src/hooksample.ts',
      want: 'https://github.com/SlashNephy/kotatsu/blob/main/frontend/src/hooksample.ts',
    },
    // 4. URL の中の資格情報
    {
      rule: '4 URL の資格だけを伏せる',
      input: 'dial postgres://kotatsu:pa55@db.home.arpa:5432/kotatsu',
      want: `dial postgres://${marks.secret}@${marks.host}:5432/kotatsu`,
    },
    {
      rule: '4 資格の無い URL は、ホストだけを伏せる',
      input: 'open https://github.com/SlashNephy/kotatsu',
      want: 'open https://github.com/SlashNephy/kotatsu',
    },
    // 5. Authorization などのヘッダー
    {
      rule: '5 ヘッダーの値を行の終わりまで伏せる',
      input: 'Authorization: Basic dXNlcjpwYXNz\nCookie: kotatsu_session=abc; other=1\nAccept: */*',
      want: `Authorization: ${marks.secret}\nCookie: ${marks.secret}\nAccept: */*`,
    },
    {
      rule: '5 JSON のヘッダー',
      input: '{"set-cookie":"a=b; Path=/"}',
      want: `{"set-cookie":${marks.secret}`,
    },
    // 6. Bearer
    {
      rule: '6 Bearer に続く値を伏せる',
      input: 'sent with bearer eyJhbGciOi.payload.sig',
      want: `sent with bearer ${marks.secret}`,
    },
    {
      rule: '6 Bearer の語だけなら触らない',
      input: 'the Bearer scheme',
      want: 'the Bearer scheme',
    },
    // 7. Kotatsu のトークン
    {
      rule: '7 Kotatsu のトークンを伏せる',
      input:
        'got kotatsu_at_Ab3dEf6hIj9lMn2pQr5tUv8xYz1bCd4fGh7jKl0nOp3 and kotatsu_Ab3dEf6hIj9lMn2pQr5tUv8xYz1bCd4fGh7jKl0nOp3',
      want: `got ${marks.secret} and ${marks.secret}`,
    },
    {
      rule: '7 メトリクスの名前は伏せない (43 字以上の名前も)',
      input:
        'kotatsu_recording_failures_total kotatsu_storage_pool_last_measured_timestamp_seconds kotatsu_recording_last_failure_timestamp_seconds',
      want: 'kotatsu_recording_failures_total kotatsu_storage_pool_last_measured_timestamp_seconds kotatsu_recording_last_failure_timestamp_seconds',
    },
    // 8. 名前に秘密の語を含む値
    {
      rule: '8 key=value と key: value の値を伏せる',
      input: 'client_secret=abc123 password: "two words" api key: k-1 session_id=s1',
      want: `client_secret=${marks.secret} password: "${marks.secret}" api key: ${marks.secret} session_id=${marks.secret}`,
    },
    {
      rule: '8 JSON の値を伏せ、伏せてある値 ([REDACTED]) には触らない',
      input: '{"access_token":"a b\\"c","secret_key":"[REDACTED]","token_count":"3"}',
      want: `{"access_token":"${marks.secret}","secret_key":"[REDACTED]","token_count":"${marks.secret}"}`,
    },
    {
      rule: '8 秘密の語を含まない名前は触らない',
      input: 'state=recording code=500 key=main title="ニュース7"',
      want: 'state=recording code=500 key=main title="ニュース7"',
    },
    {
      rule: '8 既に印の値は触らない',
      input: 'password=[REDACTED] token: [REDACTED_SECRET]',
      want: 'password=[REDACTED] token: [REDACTED_SECRET]',
    },
    // 9. URL のクエリの鍵
    {
      rule: '9 クエリの鍵の値を伏せる',
      input: '/auth/oidc/callback?code=xyz&state=abc&foo=1 and https://github.com/x?sig=deadbeef',
      want: `/auth/oidc/callback?code=${marks.secret}&state=${marks.secret}&foo=1 and https://github.com/x?sig=${marks.secret}`,
    },
    {
      rule: '9 ? と & の直後でなければ触らない',
      input: 'job state=running mycode=1',
      want: 'job state=running mycode=1',
    },
    // 10. メールアドレス
    {
      rule: '10 メールアドレスを伏せる',
      input: 'subject mailto:Alice.Smith+tv@example.co.jp sent',
      want: `subject mailto:${marks.email} sent`,
    },
    {
      rule: '10 @ を含むだけのものは触らない',
      input: '@types/node v@1 user@localhost',
      want: '@types/node v@1 user@localhost',
    },
    // 11. カード番号
    {
      rule: '11 区切りのあるカード番号を伏せる',
      input: 'card 4111 1111 1111 1111 or 3782-822463-10005',
      want: `card ${marks.card} or ${marks.card}`,
    },
    {
      // Go の %v で出した 4 桁のサービス ID の並び。区切りの形は同じだが、Luhn の検査で外れる。
      rule: '11 4 桁の数字を空白で並べただけのものは伏せない',
      input: 'services [1024 1025 1032 1033] and 4111 1111 1111 1112',
      want: 'services [1024 1025 1032 1033] and 4111 1111 1111 1112',
    },
    {
      rule: '11 区切りの無いカード番号を伏せる (Visa、Mastercard、JCB)',
      input: 'n=4111111111111111 m=5555555555554444 j=3530111333300000',
      want: `n=${marks.card} m=${marks.card} j=${marks.card}`,
    },
    {
      rule: '11 番組の ID、epoch ミリ秒、日付、UUID は伏せない',
      input:
        'program_id=327360102400123 bs=400101000012 at=1794491737330 2026-11-12 123e4567-1234-5678-9012-123456789abc v1.2.3.4111111111111111',
      want: 'program_id=327360102400123 bs=400101000012 at=1794491737330 2026-11-12 123e4567-1234-5678-9012-123456789abc v1.2.3.4111111111111111',
    },
    // 12. IPv6
    {
      rule: '12 IPv6 を伏せる (短い形、8 組の形、ゾーン付き)',
      input: 'from [2001:db8::1]:443 and fe80::1%eth0 and 2001:0db8:0000:0000:0000:ff00:0042:8329',
      want: `from [${marks.ip}]:443 and ${marks.ip} and ${marks.ip}`,
    },
    {
      rule: '12 数字の無いもの (C++ の名前) や時刻は伏せない',
      input: 'std::vector a::b listening on [::] at 12:30:00',
      want: 'std::vector a::b listening on [::] at 12:30:00',
    },
    // 13. IPv4
    {
      rule: '13 IPv4 を伏せ、ポートと CIDR は残す',
      input: 'from 192.168.1.20:51724 allow 10.0.0.0/8',
      want: `from ${marks.ip}:51724 allow ${marks.ip}/8`,
    },
    {
      rule: '13 製品とカーネルの版は伏せない',
      input: 'Chrome/131.0.0.0 Safari/537.36 kernel 6.1.0.12-amd64 v1.2.3.4.5',
      want: 'Chrome/131.0.0.0 Safari/537.36 kernel 6.1.0.12-amd64 v1.2.3.4.5',
    },
    // 14. URL のホスト部
    {
      rule: '14 URL のホストを伏せ、公開ホストは残す',
      input: 'GET http://mirakurun:40772/api/version and https://ghcr.io/slashnephy/kotatsu',
      want: `GET http://${marks.host}:40772/api/version and https://ghcr.io/slashnephy/kotatsu`,
    },
    // 15. 点の無い名前:ポート
    {
      rule: '15 点の無い名前:ポートを伏せる',
      input: 'dial tcp mirakurun:40772: refused; db:5432',
      want: `dial tcp ${marks.host}:40772: refused; ${marks.host}:5432`,
    },
    {
      rule: '15 ファイル:行、時刻、行と列、ホストでない語は伏せない',
      input: 'main.go:42 at 12:30:00 parser:12:5 line:120 status:200 x:80 retry:10',
      want: 'main.go:42 at 12:30:00 parser:12:5 line:120 status:200 x:80 retry:10',
    },
    {
      rule: '15 1 桁の数字はポートとみなさない',
      input: 'step two:1',
      want: 'step two:1',
    },
    // 16. 裸のドメイン名
    {
      rule: '16 よくある末尾のドメイン名を伏せる',
      input: 'resolve nas.local and tuner.home.arpa and kotatsu.example.com',
      want: `resolve ${marks.host} and ${marks.host} and ${marks.host}`,
    },
    {
      rule: '16 ファイル名と公開ホストは伏せない',
      input: 'main.ts index.app.tsx config.yaml raw.githubusercontent.com',
      want: 'main.ts index.app.tsx config.yaml raw.githubusercontent.com',
    },
    // 17. ホームフォルダのユーザー名
    {
      rule: '17 ホームフォルダのユーザー名を伏せる',
      input:
        'open /home/alice/rec.ts and /Users/bob/x and C:\\Users\\carol\\x and "C:\\\\Users\\\\dave\\\\x"',
      want: `open /home/${marks.user}/rec.ts and /Users/${marks.user}/x and C:\\Users\\${marks.user}\\x and "C:\\\\Users\\\\${marks.user}\\\\x"`,
    },
    {
      rule: '17 ホームフォルダでないパスは触らない',
      input: '/var/lib/kotatsu/recordings/ニュース7.m2ts',
      want: '/var/lib/kotatsu/recordings/ニュース7.m2ts',
    },
    // 残すもの
    {
      rule: '番組名、局名、ファイル名、番組の ID、epoch ミリ秒、日付は残す',
      input:
        '{"msg":"recording finished","title":"【新】アニメ 第1話","service":"ＮＨＫ総合１・東京","file":"20261112-ニュース.m2ts","program_id":327360102400123,"at":1794491737330,"date":"2026-11-12"}',
      want: '{"msg":"recording finished","title":"【新】アニメ 第1話","service":"ＮＨＫ総合１・東京","file":"20261112-ニュース.m2ts","program_id":327360102400123,"at":1794491737330,"date":"2026-11-12"}',
    },
  ];

describe('redact', () => {
  test.each(cases)('$rule', ({ input, want, secrets = [], hosts = [] }) => {
    const options = { secrets, hosts };
    const once = redact(input, options);
    expect(once).toBe(want);
    // 印をさらに伏せないので、何度かけても変わらない。
    expect(redact(once, options)).toBe(once);
  });

  test('ログの 1 行に、規則がまとめて当たる', () => {
    const line =
      'webhook failed {"url":"https://discord.com/api/webhooks/1/x","remote":"192.168.1.5:40772","host":"tuner.lan","mail":"a@b.jp"}';
    expect(redact(line, noOptions)).toBe(
      `webhook failed {"url":"${marks.webhook}","remote":"${marks.ip}:40772","host":"${marks.host}","mail":"${marks.email}"}`,
    );
  });
});

describe('luhn', () => {
  test.each([
    ['4111111111111111', true],
    ['4111111111111112', false],
    ['79927398713', true],
    ['0', true],
  ])('%s', (digits, want) => {
    expect(luhn(digits)).toBe(want);
  });
});

// テストのカード番号 (各ブランドが公開している番号)。
const testCards = [
  '4111111111111111',
  '4012888888881881',
  '4222222222222',
  '5555555555554444',
  '2223003122003222',
  '378282246310005',
  '371449635398431',
  '3530111333300000',
  '3566002020360505',
  '30569309025904',
  '6011111111111117',
  '6200000000000005',
];

describe('isCardNumber', () => {
  test.each(testCards.filter((card) => !isProgramID(card)))('テストの番号 %s を当てる', (card) => {
    expect(isCardNumber(card)).toBe(true);
  });

  // 番組の ID の形と重なるカード番号 (13 桁の Visa と 15 桁の American Express の一部) は、番組の ID を残すために当てない。
  test('番組の ID の形と重なる番号は当てない', () => {
    expect(testCards.filter((card) => isProgramID(card))).toEqual([
      '4222222222222',
      '378282246310005',
    ]);
    expect(isCardNumber('4222222222222')).toBe(false);
    expect(isCardNumber('378282246310005')).toBe(false);
  });

  // 番組の ID は (networkId × 100000 + serviceId) × 100000 + eventId。地上波、BS、CS、ケーブルの範囲で、
  // Luhn を通るものも含めて、どれもカード番号とみなさない。
  test('番組の ID を当てない', () => {
    const networks = [4, 6, 7, 1, 10, 11, 30848, 32391, 32736, 32744, 65530, 65534, 65535];
    let checked = 0;
    for (const networkID of networks) {
      for (let serviceID = 0; serviceID <= 65535; serviceID += 257) {
        for (let eventID = 0; eventID <= 65535; eventID += 4099) {
          const id = String((networkID * 100000 + serviceID) * 100000 + eventID);
          expect(isCardNumber(id), id).toBe(false);
          checked++;
        }
      }
    }
    expect(checked).toBeGreaterThan(30000);
  });

  // epoch ミリ秒 (13 桁)、マイクロ秒 (16 桁)、ナノ秒 (19 桁) を、13 桁になる 2001 年 9 月から 2039 年まで。
  // 2040 年からのマイクロ秒は、Mastercard の番号 (2221〜2720 で始まる 16 桁) と重なる。
  test('epoch の時刻を当てない', () => {
    const start = Date.UTC(2001, 8, 9);
    const end = Date.UTC(2040, 0, 1);
    for (let ms = start; ms < end; ms += 86_400_000 * 3 + 12_345) {
      for (const digits of [String(ms), `${ms}123`, `${ms}123456`]) {
        expect(isCardNumber(digits), digits).toBe(false);
      }
    }
  });
});
