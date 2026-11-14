import { Anchor } from '@mantine/core';
import { useTranslation } from 'react-i18next';

import { filmarksSearchURL } from './filmarks';

type FilmarksLinkProps = {
  genres: readonly { readonly lv1: number; readonly lv2: number }[];
  // workTitle は、tenban が番組名から読んだ作品名 (Program.workTitle、Recording.workTitle)。
  workTitle: string;
  // anime は、しょぼいカレンダーの話に結びついているかどうか。アニメの記録先へのリンクを出すので、Filmarks のリンクは出さない。
  anime: boolean;
};

// FilmarksLink は、ドラマと映画の番組と録画に、Filmarks で作品名を探すリンクを出す (#1650)。当たらなければ何も出さない。
export function FilmarksLink({ genres, workTitle, anime }: FilmarksLinkProps) {
  const { t } = useTranslation();
  const href = filmarksSearchURL(genres, workTitle, anime);
  if (href == null) {
    return null;
  }

  return (
    <Anchor size="sm" href={href} target="_blank" rel="noopener noreferrer">
      {t('guide.detail.searchFilmarks')}
    </Anchor>
  );
}
