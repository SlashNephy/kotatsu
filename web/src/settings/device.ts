// deviceName は、ログイン中の端末の一覧に出す、User-Agent から読み取ったブラウザと OS の名前。
// 読み取れなければ null を返し、呼ぶ側で User-Agent のまま出すか「不明な端末」とする。
// 並べる順に意味がある。Edge と Opera は Chrome を、Chrome は Safari を名乗るので、先に調べる。
const browsers: [RegExp, string][] = [
  [/Edg(e|A|iOS)?\//, 'Edge'],
  [/OPR\/|Opera/, 'Opera'],
  [/Firefox\/|FxiOS\//, 'Firefox'],
  [/Chrome\/|CriOS\//, 'Chrome'],
  [/Safari\//, 'Safari'],
];

const systems: [RegExp, string][] = [
  [/iPhone|iPad|iPod/, 'iOS'],
  [/Android/, 'Android'],
  [/Windows/, 'Windows'],
  [/Mac OS X|Macintosh/, 'macOS'],
  [/CrOS/, 'ChromeOS'],
  [/Linux/, 'Linux'],
];

export function deviceName(userAgent: string): string | null {
  const browser = browsers.find(([pattern]) => pattern.test(userAgent))?.[1];
  const system = systems.find(([pattern]) => pattern.test(userAgent))?.[1];
  if (browser === undefined && system === undefined) {
    return null;
  }
  if (browser === undefined || system === undefined) {
    return browser ?? system ?? null;
  }

  return `${browser}（${system}）`;
}
