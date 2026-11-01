// 視聴ルールの編集の画面の入力と、tenban の視聴ルール (WatchRule と WatchRuleInput) を行き来させる (設計書 4.8)。
// 番組の条件は、録画のルールと同じ入力を使う。

import {
  emptyConditions,
  fromConditions,
  toConditionsInput,
  type ConditionsData,
  type ConditionsForm,
} from '../conditions/form';

// WatchRuleForm は、視聴ルールの編集の画面の入力。番組の条件と、視聴ルールだけの項目を並べる。
export type WatchRuleForm = ConditionsForm & {
  name: string;
  enabled: boolean;
};

// WatchRuleData は、tenban が返す視聴ルールのうち、編集に使う項目。
export type WatchRuleData = {
  readonly name: string;
  readonly enabled: boolean;
  readonly conditions: ConditionsData;
};

// emptyWatchRuleForm は、新しい視聴ルールの入力の初期値。
export function emptyWatchRuleForm(): WatchRuleForm {
  return { ...emptyConditions(), name: '', enabled: true };
}

// fromWatchRule は、tenban の視聴ルールを、編集の画面の入力にする。
export function fromWatchRule(rule: WatchRuleData): WatchRuleForm {
  return { ...fromConditions(rule.conditions), name: rule.name, enabled: rule.enabled };
}

// toWatchRuleInput は、編集の画面の入力を、tenban の WatchRuleInput にする。
export function toWatchRuleInput(form: WatchRuleForm) {
  return { name: form.name.trim(), enabled: form.enabled, conditions: toConditionsInput(form) };
}

// toPreviewInput は、編集の画面の入力を、ルールのプレビュー (previewRule) に渡す RuleInput にする。
// 視聴ルールは、有効期間も重複の回避も持たないので、どちらも区切らない録画のルールとして照合する。
export function toPreviewInput(form: WatchRuleForm) {
  const input = toWatchRuleInput(form);

  return {
    name: input.name,
    enabled: input.enabled,
    conditions: input.conditions,
    validFrom: null,
    validUntil: null,
    avoidDuplicates: false,
  };
}
