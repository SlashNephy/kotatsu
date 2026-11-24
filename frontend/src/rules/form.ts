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
  // detectCm は、録画が終わったら CM の区間を見分けるかどうか (#1209)。null なら全体の設定に従う。
  detectCm: boolean | null;
  // generateCaptions は、録画が終わったら音声から字幕を自動生成するかどうか (#1682)。null なら全体の設定に従う。
  generateCaptions: boolean | null;
  // transcodePreset は、録画を終えた後に変換するときの画質 (#1253)。空なら変換しない。null なら全体の設定に従う。
  transcodePreset: string | null;
  // transcodeFrameRate は、変換で出すコマ数 (#1509。auto、30、60)。null なら全体の設定に従う。
  transcodeFrameRate: string | null;
  // cmDetector は、CM の区間の見分け方 (#1505)。null なら全体の設定に従う。
  cmDetector: CMDetector | null;
};

// CMDetector は、CM の区間の見分け方 (#1505)。
export type CMDetector = 'FFMPEG' | 'LOGO';

// RuleForm は、ルールの編集の画面の入力。番組の条件と、ルールだけの項目を並べる。
export type RuleForm = ConditionsForm & {
  name: string;
  memo: string;
  enabled: boolean;
  avoidDuplicates: boolean;
  duplicateWindowDays: number | null;
  followEventRelay: boolean;
  // bundleSameEpisodes は、局をまたいで同じ回をまとめ、最初の放送だけを予約するかどうか (#1498)。
  bundleSameEpisodes: boolean;
  // skipEarlierEpisodes は、追っているシリーズの前の回を予約しないかどうか (#1499)。
  skipEarlierEpisodes: boolean;
  settings: Settings;
  validFrom: string | null;
  validUntil: string | null;
  // groupId は、ルールを入れるグループの ID (#1493)。null ならどのグループにも入れない (一覧の「その他」)。
  groupId: string | null;
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
    readonly detectCm?: boolean | null;
    readonly generateCaptions?: boolean | null;
    readonly transcodePreset?: string | null;
    readonly transcodeFrameRate?: string | null;
    readonly cmDetector?: string | null;
  };
  readonly validFrom?: string | null;
  readonly validUntil?: string | null;
  readonly avoidDuplicates: boolean;
  readonly duplicateWindowDays?: number | null;
  readonly followEventRelay: boolean;
  readonly bundleSameEpisodes: boolean;
  readonly skipEarlierEpisodes: boolean;
  readonly group?: { readonly id: string } | null;
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
    bundleSameEpisodes: true,
    // 番組名を変えずに 1 話から数え直すシリーズを黙って飛ばさないよう、新しいルールでも切から始める (#1499)。
    skipEarlierEpisodes: false,
    settings: {
      poolId: null,
      fileNameTemplate: null,
      startMarginSeconds: null,
      endMarginSeconds: null,
      priority: null,
      joinSegments: null,
      detectCm: null,
      generateCaptions: null,
      transcodePreset: null,
      transcodeFrameRate: null,
      cmDetector: null,
    },
    validFrom: null,
    validUntil: null,
    groupId: null,
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
    bundleSameEpisodes: rule.bundleSameEpisodes,
    skipEarlierEpisodes: rule.skipEarlierEpisodes,
    settings: {
      poolId: rule.settings.poolId ?? null,
      fileNameTemplate: rule.settings.fileNameTemplate ?? null,
      startMarginSeconds: rule.settings.startMarginSeconds ?? null,
      endMarginSeconds: rule.settings.endMarginSeconds ?? null,
      priority: rule.settings.priority ?? null,
      joinSegments: rule.settings.joinSegments ?? null,
      detectCm: rule.settings.detectCm ?? null,
      generateCaptions: rule.settings.generateCaptions ?? null,
      transcodePreset: rule.settings.transcodePreset ?? null,
      transcodeFrameRate: rule.settings.transcodeFrameRate ?? null,
      cmDetector: cmDetectorOf(rule.settings.cmDetector),
    },
    validFrom: rule.validFrom ?? null,
    validUntil: rule.validUntil ?? null,
    groupId: rule.group?.id ?? null,
  };
}

// cmDetectorOf は、tenban が返す見分け方を、入力の値にする。知らない値と null は、全体の設定に従う (null) にする。
export function cmDetectorOf(value: string | null | undefined): CMDetector | null {
  return value === 'FFMPEG' || value === 'LOGO' ? value : null;
}

// toInput は、編集の画面の入力を、tenban の RuleInput にする。グループは含めない。
// プレビューと照合を重くする書き方の確認は、グループに関わらないので、グループを変えても問い合わせ直さないためである。
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
    bundleSameEpisodes: form.bundleSameEpisodes,
    skipEarlierEpisodes: form.skipEarlierEpisodes,
  };
}

// toSaveInput は、ルールを保存するときの RuleInput。toInput に、ルールを入れるグループを足す (#1493)。
export function toSaveInput(form: RuleForm) {
  return { ...toInput(form), groupId: form.groupId };
}
