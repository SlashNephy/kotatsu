import { NumberInput } from '@mantine/core';
import { useTranslation } from 'react-i18next';

import { AutoSaveNumberInput } from '../settings/AutoSaveFields';
import { commentOffsetStepMs, maxCommentOffsetMs, useCommentOffset } from './commentOffset';

// CommentOffsetSetting は、録画の実況のコメントの位置 (#1357) を変える欄。端末ごとの設定を変える。
// プレイヤーの設定のメニューでは、変えたらすぐ映像に合わせる。設定の画面では onSaved を渡し、欄を離れたときか Enter で
// 保存して、保存の通知を出す (#1740)。
export function CommentOffsetSetting({ onSaved }: { onSaved?: () => void }) {
  const { t } = useTranslation();
  const [ms, setMs] = useCommentOffset();
  const props = {
    size: 'xs',
    label: t('player.commentOffset.label'),
    description: t('player.commentOffset.description'),
    min: -maxCommentOffsetMs / 1_000,
    max: maxCommentOffsetMs / 1_000,
    step: commentOffsetStepMs / 1_000,
    decimalScale: 1,
    suffix: t('player.commentOffset.suffix'),
  } as const;

  if (onSaved !== undefined) {
    return (
      <AutoSaveNumberInput
        {...props}
        value={ms / 1_000}
        onSave={(seconds) => {
          setMs(seconds * 1_000);
          onSaved();

          return Promise.resolve(true);
        }}
      />
    );
  }

  return (
    <NumberInput
      {...props}
      value={ms / 1_000}
      onChange={(value) => setMs((typeof value === 'number' ? value : Number(value) || 0) * 1_000)}
    />
  );
}
