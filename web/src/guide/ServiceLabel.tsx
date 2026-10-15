import { Tv } from 'lucide-react';
import { useState, type CSSProperties } from 'react';

import { toHalfWidth } from '../futon/halfWidth';
import classes from './ServiceLabel.module.css';

type Props = {
  name: string;
  // logoUrl は、tenban が返したロゴの URL のパス。放送がロゴを送っていなければ null。
  logoUrl: string | null | undefined;
  // logoHeight は、ロゴの高さ。省略すると、文字の大きさに合わせる。
  logoHeight?: string;
  // logoOnlyNarrow は、狭い画面でロゴだけを見せるか。ロゴがないときは、見分けが付かないので名前も見せる。
  logoOnlyNarrow?: boolean;
};

// ServiceLabel は、ロゴを添えたサービス名。Text などの中に置き、文字と同じ行に並べる。
// ロゴがないか、読めなければ、ロゴと同じ大きさの枠にアイコンを入れて代わりに出し、名前の位置を揃える。
// 名前の全角の英数字は半角にする。
export function ServiceLabel({
  name: rawName,
  logoUrl,
  logoHeight = '1.4em',
  logoOnlyNarrow = false,
}: Props) {
  const name = toHalfWidth(rawName);
  // failedURL は、読み込めなかったロゴの URL。URL が変われば、読み直す。
  const [failedURL, setFailedURL] = useState<string | null>(null);
  const frame: CSSProperties = {
    height: logoHeight,
    // 放送のロゴは 16:9 で送られる。読み込む前から幅を取っておき、名前がずれないようにする。
    aspectRatio: '16 / 9',
    // middle は x の高さの中央に揃え、和文の字面より下がるので、字面の中央 (ベースラインから約 0.38em) に揃える。
    verticalAlign: `calc(0.38em - ${logoHeight} / 2)`,
    marginInlineEnd: 'var(--service-label-gap, 0.4em)',
    borderRadius: 2,
  };

  if (logoUrl == null || failedURL === logoUrl) {
    return (
      <>
        {/* 名前を隣に書くので、読み上げでは飛ばす。 */}
        <span
          aria-hidden
          data-logo-placeholder
          style={{
            ...frame,
            // ロゴの img と同じく、箱の下端をベースラインにして揃える。
            // inline-flex では、中のアイコンの下端がベースラインになり、ロゴより下がる。
            display: 'inline-block',
            overflow: 'hidden',
            position: 'relative',
            background: 'var(--mantine-color-default-hover)',
            color: 'var(--mantine-color-dimmed)',
          }}
        >
          <Tv
            style={{ position: 'absolute', inset: 0, margin: 'auto', height: '70%', width: 'auto' }}
          />
        </span>
        {name}
      </>
    );
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
        className={logoOnlyNarrow ? classes.narrowLogo : undefined}
        style={{ ...frame, display: 'inline-block', objectFit: 'contain' }}
      />
      {logoOnlyNarrow ? <span className={classes.narrowHiddenName}>{name}</span> : name}
    </>
  );
}
