import { Center, Image, Text } from '@mantine/core';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

type Props = {
  // url は、tenban が返した代表画像の URL のパス。画像をまだ作っていなければ null。
  url: string | null | undefined;
};

// RecordingThumbnail は、録画の代表画像。画像をまだ作っていないか、読めなければ、代わりの表示を出す。
export function RecordingThumbnail({ url }: Props) {
  const { t } = useTranslation();
  // failedURL は、読み込めなかった画像の URL。URL が変われば、読み直す。
  const [failedURL, setFailedURL] = useState<string | null>(null);

  if (url == null || failedURL === url) {
    return (
      <Center bg="var(--mantine-color-default-hover)">
        <Text size="xs" c="dimmed">
          {t('recordings.noThumbnail')}
        </Text>
      </Center>
    );
  }

  return <Image src={url} alt="" loading="lazy" fit="cover" onError={() => setFailedURL(url)} />;
}
