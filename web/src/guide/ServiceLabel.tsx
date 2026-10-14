import { useState } from 'react';

import { toHalfWidth } from '../futon/halfWidth';

type Props = {
  name: string;
  // logoUrl は、tenban が返したロゴの URL のパス。放送がロゴを送っていなければ null。
  logoUrl: string | null | undefined;
  // logoHeight は、ロゴの高さ。省略すると、文字の大きさに合わせる。
  logoHeight?: string;
};

// ServiceLabel は、ロゴを添えたサービス名。Text などの中に置き、文字と同じ行に並べる。
// ロゴがないか、読めなければ、名前だけを出す。名前の全角の英数字は半角にする。
export function ServiceLabel({ name: rawName, logoUrl, logoHeight = '1.4em' }: Props) {
  const name = toHalfWidth(rawName);
  // failedURL は、読み込めなかったロゴの URL。URL が変われば、読み直す。
  const [failedURL, setFailedURL] = useState<string | null>(null);

  if (logoUrl == null || failedURL === logoUrl) {
    return <>{name}</>;
  }

  return (
    <>
      <img
        src={logoUrl}
        // 名前を隣に書くので、読み上げでは飛ばす。
        alt=""
        loading="lazy"
        decoding="async"
        onError={() => setFailedURL(logoUrl)}
        style={{
          display: 'inline-block',
          height: logoHeight,
          // 放送のロゴは 16:9 で送られる。読み込む前から幅を取っておき、名前がずれないようにする。
          aspectRatio: '16 / 9',
          objectFit: 'contain',
          // middle は x の高さの中央に揃え、和文の字面より下がるので、字面の中央 (ベースラインから約 0.38em) に揃える。
          verticalAlign: `calc(0.38em - ${logoHeight} / 2)`,
          marginInlineEnd: '0.4em',
          borderRadius: 2,
        }}
      />
      {name}
    </>
  );
}
