import { ActionIcon, Menu } from '@mantine/core';
import { EllipsisVertical, Search } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { filmarksSearchURL } from './filmarks';
import { streamingSearch } from './streaming';

type SearchElsewhereMenuProps = {
  genres: readonly { readonly lv1: number; readonly lv2: number }[];
  // workTitle は、tenban が番組名から読んだ作品名 (Program.workTitle、Recording.workTitle)。
  workTitle: string;
  // anime は、しょぼいカレンダーの話に結びついているかどうか。アニメの記録先へのリンクがあるので、Filmarks では探さない。
  anime: boolean;
  service: { readonly name: string; readonly channelType: { readonly type: string } };
};

// SearchElsewhereMenu は、番組と録画の詳細の番組名の横に、作品名でほかのサイトを探すリンクを、三点のメニューにまとめて置く。
// ドラマと映画は Filmarks (#1650)、地上波の局の番組は見逃し配信のサービス (TVer、NHK ONE。#1354) で探す。
// どれも外部のサイトへ移る補助の操作なので、作品の欄に並べず、メニューにしまう。探すサイトがなければ、何も出さない。
export function SearchElsewhereMenu({
  genres,
  workTitle,
  anime,
  service,
}: SearchElsewhereMenuProps) {
  const { t } = useTranslation();
  const filmarks = filmarksSearchURL(genres, workTitle, anime);
  const streaming = streamingSearch(service, workTitle);
  const links = [
    ...(filmarks != null ? [{ label: t('guide.detail.searchFilmarks'), href: filmarks }] : []),
    ...(streaming !== null
      ? [
          {
            label: t(
              streaming.service === 'nhk' ? 'guide.detail.searchNhkOne' : 'guide.detail.searchTver',
            ),
            href: streaming.url,
          },
        ]
      : []),
  ];
  if (links.length === 0) {
    return null;
  }

  return (
    <Menu position="bottom-end" withinPortal>
      <Menu.Target>
        <ActionIcon variant="subtle" color="gray" aria-label={t('guide.detail.searchElsewhere')}>
          <EllipsisVertical size={16} aria-hidden />
        </ActionIcon>
      </Menu.Target>
      <Menu.Dropdown>
        {links.map((link) => (
          <Menu.Item
            key={link.href}
            component="a"
            href={link.href}
            target="_blank"
            rel="noopener noreferrer"
            leftSection={<Search size={14} aria-hidden />}
          >
            {link.label}
          </Menu.Item>
        ))}
      </Menu.Dropdown>
    </Menu>
  );
}
