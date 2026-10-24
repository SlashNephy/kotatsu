import { Anchor } from '@mantine/core';

import { splitLinks } from './linkify';

// LinkifiedText は、文字の中の URL をリンクにして出す。Text などの中に置く。
// リンクは、番組の外のサイトなので、新しいタブで開き、開いた先にこの画面を触らせない。
export function LinkifiedText({ text }: { text: string }) {
  return (
    <>
      {splitLinks(text).map((segment, index) =>
        segment.kind === 'text' ? (
          segment.text
        ) : (
          <Anchor
            key={index}
            href={segment.href}
            target="_blank"
            rel="noopener noreferrer"
            inherit
            // 長い URL は、狭い画面でもはみ出さないよう、どこででも折り返す。
            style={{ overflowWrap: 'anywhere' }}
          >
            {segment.text}
          </Anchor>
        ),
      )}
    </>
  );
}
