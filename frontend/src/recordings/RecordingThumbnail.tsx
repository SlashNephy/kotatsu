import { Center, Image } from '@mantine/core';
import { Tv } from 'lucide-react';
import { useState } from 'react';

type Props = {
  // url は、tenban が返した代表画像の URL のパス。画像をまだ作っていなければ null。
  url: string | null | undefined;
};

// RecordingThumbnail は、録画の代表画像。画像をまだ作っていないか、読めなければ、代わりにテレビのアイコンを出す。
// 置き場所の大きさに広がるので、AspectRatio などで 16:9 の枠を与えて使う。
export function RecordingThumbnail({ url }: Props) {
  // failedURL は、読み込めなかった画像の URL。URL が変われば、読み直す。
  const [failedURL, setFailedURL] = useState<string | null>(null);

  if (url == null || failedURL === url) {
    return (
      <Center bg="var(--mantine-color-default-hover)">
        <Tv size={28} aria-hidden color="var(--mantine-color-dimmed)" />
      </Center>
    );
  }

  return <Image src={url} alt="" loading="lazy" fit="cover" onError={() => setFailedURL(url)} />;
}
