import { MultiSelect, type MultiSelectProps } from '@mantine/core';
import { useTranslation } from 'react-i18next';

import { decodeGenre, type Genre } from './guideView';
import { genreOptions, subGenreKey } from './genreOptions';

type Props = Omit<MultiSelectProps, 'data' | 'value' | 'onChange' | 'renderOption'> & {
  // value は、選んでいるジャンルを encodeGenre で直した値。
  value: string[];
  onChange: (value: string[]) => void;
  // keep は、名前がなくても選択肢に残すジャンル。含めるジャンルと除外するジャンルの両方を渡す。
  keep: readonly Genre[];
};

// GenreMultiSelect は、大分類と中分類を混ぜて選ぶドロップダウン。大分類ごとのグループに分けて並べる。
// 選んだジャンルは「映画／アニメ」のように大分類を添えて出し、ドロップダウンの中では、グループの見出しがあるので短く出す。
// 文字を入れると、大分類を添えた名前で絞り込むので、「アニメ」で「アニメ・特撮」と「映画／アニメ」の両方が出る。
export function GenreMultiSelect({ value, onChange, keep, ...props }: Props) {
  const { t } = useTranslation();

  return (
    <MultiSelect
      {...props}
      data={genreOptions(t, keep)}
      value={value}
      onChange={onChange}
      renderOption={({ option }) => {
        const genre = decodeGenre(option.value);
        if (genre.lv2 == null) {
          return t('guide.view.allOfGenre');
        }
        const key = subGenreKey(genre.lv1, genre.lv2);

        return key === undefined ? option.label : t(key);
      }}
      searchable
      clearable
      maxDropdownHeight={320}
    />
  );
}
