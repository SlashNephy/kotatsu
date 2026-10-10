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
  const [failed, setFailed] = useState(false);

  if (url == null || failed) {
    return (
      <Center bg="var(--mantine-color-default-hover)">
        <Text size="xs" c="dimmed">
          {t('recordings.noThumbnail')}
        </Text>
      </Center>
    );
  }

  return <Image src={url} alt="" loading="lazy" fit="cover" onError={() => setFailed(true)} />;
}
