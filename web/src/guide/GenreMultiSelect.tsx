import { MultiSelect, type MultiSelectProps } from '@mantine/core';
import { useTranslation } from 'react-i18next';

import type { Genre } from './guideView';
import { genreOptions } from './genreOptions';

type Props = Omit<MultiSelectProps, 'data' | 'value' | 'onChange'> & {
  // value は、選んでいるジャンルを encodeGenre で直した値。
  value: string[];
  onChange: (value: string[]) => void;
  // keep は、名前がなくても選択肢に残すジャンル。含めるジャンルと除外するジャンルの両方を渡す。
  keep: readonly Genre[];
};

// GenreMultiSelect は、大分類と中分類を混ぜて選ぶドロップダウン。大分類ごとのグループに分けて並べる。
// 選択肢も選んだジャンルも、アプリのほかの場所と同じく「映画／アニメ」のように「大分類／中分類」で出す。大分類のすべては、大分類の名前だけにする。
// 文字を入れると、この名前で絞り込むので、「アニメ」で「アニメ・特撮」と「映画／アニメ」の両方が出る。
export function GenreMultiSelect({ value, onChange, keep, ...props }: Props) {
  const { t } = useTranslation();

  return (
    <MultiSelect
      {...props}
      data={genreOptions(t, keep)}
      value={value}
      onChange={onChange}
      searchable
      clearable
      maxDropdownHeight={320}
    />
  );
}
