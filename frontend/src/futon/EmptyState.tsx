import { Alert, Text } from '@mantine/core';
import type { LucideIcon } from 'lucide-react';
import type { ReactNode } from 'react';

type Props = {
  icon: LucideIcon;
  // title は、何もないことを伝える見出し。
  title: ReactNode;
  // hint は、次にどうすればよいかの手がかり。手がかりがなければ省く。
  hint?: ReactNode;
};

// EmptyState は、一覧が空のときに出す表示。見出しと手がかりの 2 段にし、何の一覧かが分かるアイコンを添える。
export function EmptyState({ icon: Icon, title, hint }: Props) {
  return (
    <Alert color="gray" icon={<Icon size={20} aria-hidden />} title={title}>
      {hint !== undefined && (
        <Text size="sm" c="dimmed">
          {hint}
        </Text>
      )}
    </Alert>
  );
}
