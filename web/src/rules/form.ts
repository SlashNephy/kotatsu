// ルールの編集の画面の入力と、tenban のルール (Rule と RuleInput) を行き来させる。

import {
  emptyConditions,
  fromConditions,
  toConditionsInput,
  type ConditionsData,
  type ConditionsForm,
} from '../conditions/form';

type Settings = {
  poolId: string | null;
  fileNameTemplate: string | null;
  startMarginSeconds: number | null;
  endMarginSeconds: number | null;
  priority: number | null;
  // joinSegments は、分かれた録画のファイルを1つにつなぐかどうか。null なら全体の既定値に従う。
  joinSegments: boolean | null;
};

// RuleForm は、ルールの編集の画面の入力。番組の条件と、ルールだけの項目を並べる。
export type RuleForm = ConditionsForm & {
  name: string;
  memo: string;
  enabled: boolean;
  avoidDuplicates: boolean;
  duplicateWindowDays: number | null;
  followEventRelay: boolean;
  settings: Settings;
  validFrom: string | null;
  validUntil: string | null;
};

// RuleData は、tenban が返すルールのうち、編集に使う項目。
export type RuleData = {
  readonly name: string;
  readonly memo: string;
  readonly enabled: boolean;
  readonly conditions: ConditionsData;
  readonly settings: {
    readonly poolId?: string | null;
    readonly fileNameTemplate?: string | null;
    readonly startMarginSeconds?: number | null;
    readonly endMarginSeconds?: number | null;
    readonly priority?: number | null;
    readonly joinSegments?: boolean | null;
  };
  readonly validFrom?: string | null;
  readonly validUntil?: string | null;
  readonly avoidDuplicates: boolean;
  readonly duplicateWindowDays?: number | null;
  readonly followEventRelay: boolean;
};

// emptyRuleForm は、新しいルールの入力の初期値。conditions を渡すと、その条件から始める (番組の検索から作るときなど)。
export function emptyRuleForm(conditions: ConditionsForm = emptyConditions()): RuleForm {
  return {
    ...conditions,
    name: '',
    memo: '',
    enabled: true,
    avoidDuplicates: false,
    duplicateWindowDays: null,
    followEventRelay: true,
    settings: {
      poolId: null,
      fileNameTemplate: null,
      startMarginSeconds: null,
      endMarginSeconds: null,
      priority: null,
      joinSegments: null,
    },
    validFrom: null,
    validUntil: null,
  };
}

// fromRule は、tenban のルールを、編集の画面の入力にする。
export function fromRule(rule: RuleData): RuleForm {
  return {
    ...fromConditions(rule.conditions),
    name: rule.name,
    memo: rule.memo,
    enabled: rule.enabled,
    avoidDuplicates: rule.avoidDuplicates,
    duplicateWindowDays: rule.duplicateWindowDays ?? null,
    followEventRelay: rule.followEventRelay,
    settings: {
      poolId: rule.settings.poolId ?? null,
      fileNameTemplate: rule.settings.fileNameTemplate ?? null,
      startMarginSeconds: rule.settings.startMarginSeconds ?? null,
      endMarginSeconds: rule.settings.endMarginSeconds ?? null,
      priority: rule.settings.priority ?? null,
      joinSegments: rule.settings.joinSegments ?? null,
    },
    validFrom: rule.validFrom ?? null,
    validUntil: rule.validUntil ?? null,
  };
}

// toInput は、編集の画面の入力を、tenban の RuleInput にする。
export function toInput(form: RuleForm) {
  return {
    name: form.name.trim(),
    memo: form.memo,
    enabled: form.enabled,
    conditions: toConditionsInput(form),
    settings: form.settings,
    validFrom: form.validFrom,
    validUntil: form.validUntil,
    avoidDuplicates: form.avoidDuplicates,
    duplicateWindowDays: form.avoidDuplicates ? form.duplicateWindowDays : null,
    followEventRelay: form.followEventRelay,
  };
}
