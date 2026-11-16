import { Anchor } from '@mantine/core';
import { useTranslation } from 'react-i18next';

import { streamingSearch } from './streaming';

type StreamingLinkProps = {
  service: { readonly name: string; readonly channelType: { readonly type: string } };
  // workTitle は、tenban が番組名から読んだ作品名 (Program.workTitle、Recording.workTitle)。
  workTitle: string;
};

// StreamingLink は、番組と録画に、見逃し配信のサービス (TVer、NHK ONE) で作品名を探すリンクを出す (#1354)。当たらなければ何も出さない。
export function StreamingLink({ service, workTitle }: StreamingLinkProps) {
  const { t } = useTranslation();
  const search = streamingSearch(service, workTitle);
  if (search === null) {
    return null;
  }

  return (
    <Anchor size="sm" href={search.url} target="_blank" rel="noopener noreferrer">
      {t(search.service === 'nhk' ? 'guide.detail.searchNhkOne' : 'guide.detail.searchTver')}
    </Anchor>
  );
}
