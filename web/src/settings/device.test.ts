import { describe, expect, test } from 'vitest';

import { deviceName } from './device';

describe('deviceName', () => {
  test.each([
    {
      name: 'Linux の Chrome',
      ua: 'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36',
      want: 'Chrome（Linux）',
    },
    {
      name: 'Windows の Edge は、Chrome と名乗っていても Edge にする',
      ua: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36 Edg/140.0.0.0',
      want: 'Edge（Windows）',
    },
    {
      name: 'iPhone の Safari は、Mac OS X を名乗っていても iOS にする',
      ua: 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1',
      want: 'Safari（iOS）',
    },
    {
      name: 'Android の Firefox は、Linux を名乗っていても Android にする',
      ua: 'Mozilla/5.0 (Android 15; Mobile; rv:140.0) Gecko/140.0 Firefox/140.0',
      want: 'Firefox（Android）',
    },
    { name: 'OS だけ分かる', ua: 'Something (Macintosh)', want: 'macOS' },
    { name: '読み取れない', ua: 'kotatsu-evidence/1.0', want: null },
  ])('$name', ({ ua, want }) => {
    expect(deviceName(ua)).toBe(want);
  });
});
