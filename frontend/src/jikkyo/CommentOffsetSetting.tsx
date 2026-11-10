import { NumberInput } from '@mantine/core';
import { useTranslation } from 'react-i18next';

import { commentOffsetStepMs, maxCommentOffsetMs, useCommentOffset } from './commentOffset';

// CommentOffsetSetting は、録画の実況のコメントの位置 (#1357) を変える欄。端末ごとの設定を変える。
export function CommentOffsetSetting() {
  const { t } = useTranslation();
  const [ms, setMs] = useCommentOffset();

  return (
    <NumberInput
      size="xs"
      label={t('player.commentOffset.label')}
      description={t('player.commentOffset.description')}
      value={ms / 1_000}
      min={-maxCommentOffsetMs / 1_000}
      max={maxCommentOffsetMs / 1_000}
      step={commentOffsetStepMs / 1_000}
      decimalScale={1}
      suffix={t('player.commentOffset.suffix')}
      onChange={(value) => setMs((typeof value === 'number' ? value : Number(value) || 0) * 1_000)}
    />
  );
}
