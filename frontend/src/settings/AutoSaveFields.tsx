import {
  NumberInput,
  Select,
  Switch,
  TextInput,
  type NumberInputProps,
  type SelectProps,
  type SwitchProps,
  type TextInputProps,
} from '@mantine/core';
import { useState, type KeyboardEvent } from 'react';
import { useTranslation } from 'react-i18next';

// Parsed は、欄の入力を読んだ結果。読めなければ、欄の下に出す理由 error を持つ。
type Parsed<V> = { value: V; error?: undefined } | { error: string };

// useAutoSaveField は、数と文字の欄を、欄を離れたときか Enter で保存するための入力を持つ (#1740)。
// 打っている途中の値 (100 を打つ途中の 1 や 10) は保存しない。読めない値は、欄の下に理由を出して保存しない。
// 保存できなかったときは、打った値を残す。もう一度欄を離れるか Enter を押せば、保存し直す。
// value は、保存してある値。保存してある値が変わったら (保存した、ほかの端末や管理者が変えた)、入力をそれに合わせる。
export function useAutoSaveField<V, D>({
  value,
  toDraft,
  parse,
  onSave,
  equals = Object.is,
}: {
  value: V;
  toDraft: (value: V) => D;
  parse: (draft: D) => Parsed<V>;
  onSave: (value: V) => Promise<boolean>;
  equals?: (a: V, b: V) => boolean;
}) {
  const [draft, setDraft] = useState(() => toDraft(value));
  const [base, setBase] = useState(value);
  // showError は、保存しようとして読めなかったかどうか。打っている途中に理由を出さないよう、保存しようとするまでは出さない。
  const [showError, setShowError] = useState(false);
  // committed は、保存に渡し、まだ保存してある値に届いていない値。Enter の後に欄を離れたとき (再認証の画面に
  // フォーカスを取られたときを含む) に、同じ値を重ねて送らないために持つ。保存できなかったら消し、送り直せるようにする。
  const [committed, setCommitted] = useState<{ value: V } | null>(null);
  // 描画の中で、前に受け取った値と比較して入れ直す。effect で入れ直すと、古い入力で一度描いてしまう。
  if (!equals(value, base)) {
    setBase(value);
    setDraft(toDraft(value));
    setShowError(false);
    setCommitted(null);
  }
  const parsed = parse(draft);

  // commit は、入力 (next を渡したらそれ) を読み、保存してある値と違えば保存する。
  const commit = (next: D = draft) => {
    if (next !== draft) {
      setDraft(next);
    }
    const result = parse(next);
    if (result.error !== undefined) {
      setShowError(true);

      return;
    }
    setShowError(false);
    // 読み直した形 (前後の空白を除いたものなど) を入力に出す。
    const normalized = toDraft(result.value);
    if (normalized !== next) {
      setDraft(normalized);
    }
    if (
      equals(result.value, value) ||
      (committed !== null && equals(result.value, committed.value))
    ) {
      return;
    }
    const sent = { value: result.value };
    setCommitted(sent);
    void onSave(result.value).then((ok) => {
      if (!ok) {
        setCommitted((current) => (current === sent ? null : current));
      }
    });
  };

  return {
    draft,
    setDraft,
    commit,
    error: showError ? parsed.error : undefined,
    // onKeyDown は、Enter で保存する。かな漢字変換の確定の Enter では保存しない。
    onKeyDown: (event: KeyboardEvent<HTMLInputElement>) => {
      if (event.key === 'Enter' && !event.nativeEvent.isComposing) {
        commit();
      }
    },
  };
}

// numberRangeError は、数の欄の値が範囲から外れているときに出す理由を返す。
function useNumberRangeMessage() {
  const { t } = useTranslation();

  return ({
    min,
    max,
    integer,
  }: {
    min: number | undefined;
    max: number | undefined;
    integer: boolean;
  }) => {
    const kind = integer ? 'integer' : 'number';
    if (min !== undefined && max !== undefined) {
      return t(`settings.autosave.${kind}Range`, { min, max });
    }
    if (min !== undefined) {
      return t(`settings.autosave.${kind}Min`, { min });
    }
    if (max !== undefined) {
      return t(`settings.autosave.${kind}Max`, { max });
    }

    return t(`settings.autosave.${kind}`);
  };
}

type AutoSaveNumberInputProps = Omit<
  NumberInputProps,
  'value' | 'defaultValue' | 'onChange' | 'onBlur' | 'onKeyDown' | 'error' | 'clampBehavior'
> & {
  // validate は、範囲のほかに確かめることがあれば、理由を返す。
  validate?: (value: number) => string | null;
} & (
    | {
        // nullable は、空の欄を、値がない (null) として保存してよいかどうか。
        nullable?: false;
        // value は、保存してある値。
        value: number;
        onSave: (value: number) => Promise<boolean>;
      }
    | {
        nullable: true;
        // value は、保存してある値。null は空の欄で表す。
        value: number | null;
        onSave: (value: number | null) => Promise<boolean>;
      }
  );

// AutoSaveNumberInput は、欄を離れたときか Enter で保存する数の欄 (#1740)。min、max、allowDecimal から外れた値は、
// 理由を出して保存しない。Mantine は、欄を離れたときに範囲へ丸めるので、丸めずに理由を出すよう、丸めを切る。
export function AutoSaveNumberInput(allProps: AutoSaveNumberInputProps) {
  // onSave は、nullable で絞り込んでから呼ぶので、allProps から使う。props には渡さない。
  const {
    value,
    nullable,
    validate,
    min,
    max,
    allowDecimal = true,
    onSave: _onSave,
    ...props
  } = allProps;
  const { t } = useTranslation();
  const rangeMessage = useNumberRangeMessage();
  const field = useAutoSaveField<number | null, number | string>({
    value,
    toDraft: (saved) => saved ?? '',
    parse: (draft) => {
      if (draft === '') {
        return nullable === true ? { value: null } : { error: t('settings.autosave.required') };
      }
      const number = typeof draft === 'number' ? draft : Number.NaN;
      const outOfRange =
        !Number.isFinite(number) ||
        (allowDecimal === false && !Number.isInteger(number)) ||
        (min !== undefined && number < min) ||
        (max !== undefined && number > max);
      if (outOfRange) {
        return { error: rangeMessage({ min, max, integer: allowDecimal === false }) };
      }
      const invalid = validate?.(number) ?? null;

      return invalid === null ? { value: number } : { error: invalid };
    },
    // 空の欄を null として読むのは nullable のときだけなので、そうでなければ null は届かない。
    onSave: (next) =>
      allProps.nullable === true
        ? allProps.onSave(next)
        : next === null
          ? Promise.resolve(false)
          : allProps.onSave(next),
  });

  return (
    <NumberInput
      {...props}
      min={min}
      max={max}
      allowDecimal={allowDecimal}
      clampBehavior="none"
      value={field.draft}
      onChange={field.setDraft}
      onBlur={() => field.commit()}
      onKeyDown={field.onKeyDown}
      error={field.error}
    />
  );
}

type AutoSaveTextInputProps = Omit<
  TextInputProps,
  'value' | 'defaultValue' | 'onChange' | 'onBlur' | 'onKeyDown' | 'error'
> & {
  value: string;
  onSave: (value: string) => Promise<boolean>;
  // required は、空の欄を保存させないかどうか。
  required?: boolean;
  // validate は、ほかに確かめることがあれば、理由を返す。
  validate?: (value: string) => string | null;
  // trim は、前後の空白を除いて保存するかどうか。
  trim?: boolean;
  // normalize は、確かめた値を、保存する形 (郵便番号の数字だけなど) にする。保存してある値とは、この形で比べる。
  normalize?: (value: string) => string;
};

// AutoSaveTextInput は、欄を離れたときか Enter で保存する文字の欄 (#1740)。
export function AutoSaveTextInput({
  value,
  onSave,
  required = false,
  validate,
  trim = true,
  normalize = (text) => text,
  ...props
}: AutoSaveTextInputProps) {
  const { t } = useTranslation();
  const field = useAutoSaveField<string, string>({
    value,
    toDraft: (saved) => saved,
    parse: (draft) => {
      const text = trim ? draft.trim() : draft;
      if (required && text === '') {
        return { error: t('settings.autosave.required') };
      }
      const invalid = validate?.(text) ?? null;

      return invalid === null ? { value: normalize(text) } : { error: invalid };
    },
    onSave,
  });

  return (
    <TextInput
      {...props}
      value={field.draft}
      onChange={(event) => field.setDraft(event.currentTarget.value)}
      onBlur={() => field.commit()}
      onKeyDown={field.onKeyDown}
      error={field.error}
    />
  );
}

// useAutoSavedValue は、切り替えと選ぶ項目を、変えたらすぐ保存するための値を持つ (#1740)。
// 変えた値は、保存して読み直した値が届くまで出す。保存できなかったら、変える前の値 (保存してある値) に戻す。
// 保存してある値が変わったら (保存した、ほかの端末や管理者が変えた)、変えた値を破棄して、保存してある値を出す。
export function useAutoSavedValue<V>(
  saved: V,
  onSave: (value: V) => Promise<boolean>,
  equals: (a: V, b: V) => boolean = Object.is,
): [V, (value: V) => void] {
  const [pending, setPending] = useState<{ value: V; base: V } | null>(null);
  if (pending !== null && !equals(pending.base, saved)) {
    setPending(null);
  }
  const current = pending !== null && equals(pending.base, saved) ? pending.value : saved;

  const change = (value: V) => {
    const next = { value, base: saved };
    setPending(next);
    void onSave(value).then((ok) => {
      if (!ok) {
        // 後から別の値に変えていれば、そちらを残す。
        setPending((p) => (p === next ? null : p));
      }
    });
  };

  return [current, change];
}

type AutoSaveSwitchProps = Omit<SwitchProps, 'checked' | 'defaultChecked' | 'onChange'> & {
  // checked は、保存してある値。
  checked: boolean;
  onSave: (checked: boolean) => Promise<boolean>;
};

// AutoSaveSwitch は、切り替えたらすぐ保存する切り替え (#1740)。保存できなかったら、切り替える前に戻す。
export function AutoSaveSwitch({ checked, onSave, ...props }: AutoSaveSwitchProps) {
  const [current, change] = useAutoSavedValue(checked, onSave);

  return (
    <Switch
      {...props}
      checked={current}
      onChange={(event) => change(event.currentTarget.checked)}
    />
  );
}

type AutoSaveSelectProps = Omit<
  SelectProps,
  'value' | 'defaultValue' | 'onChange' | 'allowDeselect'
> & {
  // value は、保存してある値。
  value: string;
  onSave: (value: string) => Promise<boolean>;
};

// AutoSaveSelect は、選んだらすぐ保存する選ぶ項目 (#1740)。保存できなかったら、選ぶ前に戻す。選んだ項目は外せない。
export function AutoSaveSelect({ value, onSave, ...props }: AutoSaveSelectProps) {
  const [current, change] = useAutoSavedValue(value, onSave);

  return (
    <Select
      {...props}
      allowDeselect={false}
      value={current}
      onChange={(next) => {
        if (next !== null && next !== current) {
          change(next);
        }
      }}
    />
  );
}
