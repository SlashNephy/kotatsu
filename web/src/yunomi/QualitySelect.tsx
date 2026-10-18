import { Alert, Select, Switch } from '@mantine/core';
import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';

import type { AudioChoice, KyusuAudioChoice } from './audio';
import { cropOrigins, cropScales, type Crop } from './crop';
import { useNormalizeVolume } from './volumeNormalize';
import { useVirtualSurroundSetting } from './useVirtualSurround';
import { original, type KyusuAudio, type PlaybackRoute } from './route';

type Props = {
  choices: string[];
  quality: string | null;
  onChange: (quality: string) => void;
};

// QualitySelect は、画質のメニュー。オリジナルはブラウザの中での変換、それ以外は kyusu の画質 (設計書 7.1)。
export function QualitySelect({ choices, quality, onChange }: Props) {
  const { t } = useTranslation();
  // qualityLabel は、画質の名前を、メニューに出す文に変える。kyusu の HEVC の画質は、末尾に -hevc が付く。
  // t を引数で渡すと、i18next の型の展開が深くなりすぎるので、ここで閉じ込める。
  const qualityLabel = (name: string): string => {
    if (name === original) {
      return t('player.original');
    }
    if (name.endsWith('-hevc')) {
      return t('player.presetHevc', { name: name.slice(0, -'-hevc'.length) });
    }

    return t('player.preset', { name });
  };
  // 選べるものが 1 つしかなければ、メニューを出さない。
  if (choices.length < 2) {
    return null;
  }

  return (
    <Select
      label={t('player.quality')}
      size="xs"
      // 操作バーの設定の中に置く。別のウィンドウに移っても開けるよう、メニューをプレイヤーの中に描く。
      comboboxProps={{ withinPortal: false }}
      allowDeselect={false}
      data={choices.map((choice) => ({ value: choice, label: qualityLabel(choice) }))}
      value={quality}
      onChange={(value) => {
        if (value !== null) {
          onChange(value);
        }
      }}
    />
  );
}

// PlaybackUnavailable は、この端末では再生できないことと、その理由を示す。
export function PlaybackUnavailable({ route }: { route: PlaybackRoute }) {
  const { t } = useTranslation();
  if (route.kind !== 'unavailable') {
    return null;
  }

  return (
    <Alert color="amber" role="alert" title={t('player.unavailable.title')}>
      {route.reason === 'noTranscoder'
        ? t('player.unavailable.noTranscoder')
        : t('player.unavailable.noPlayer')}
    </Alert>
  );
}

// CaptionsToggle は、字幕と文字スーパーを出すかどうかの切り替え。
export function CaptionsToggle({
  checked,
  onChange,
}: {
  checked: boolean;
  onChange: (checked: boolean) => void;
}) {
  const { t } = useTranslation();

  return (
    <Switch
      size="sm"
      label={t('player.captions')}
      checked={checked}
      onChange={(event) => onChange(event.currentTarget.checked)}
    />
  );
}

// JikkyoToggle は、実況のコメントを出すかどうかの切り替え。
export function JikkyoToggle({
  checked,
  onChange,
}: {
  checked: boolean;
  onChange: (checked: boolean) => void;
}) {
  const { t } = useTranslation();

  return (
    <Switch
      size="sm"
      label={t('jikkyo.toggle')}
      checked={checked}
      onChange={(event) => onChange(event.currentTarget.checked)}
    />
  );
}

// DataBroadcastToggle は、データ放送を出すかどうかの切り替え。
export function DataBroadcastToggle({
  checked,
  onChange,
}: {
  checked: boolean;
  onChange: (checked: boolean) => void;
}) {
  const { t } = useTranslation();

  return (
    <Switch
      size="sm"
      label={t('player.dataBroadcast')}
      checked={checked}
      onChange={(event) => onChange(event.currentTarget.checked)}
    />
  );
}

// languages は、名前を出せる、記述子の言語のコード (ISO 639)。etc は、ARIB STD-B10 で、ほかの言語に当たらないことを表す。
const languages = ['jpn', 'eng', 'kor', 'zho', 'chi', 'etc'] as const;

// languageName は、言語のコードを、メニューに出す名前にする。知らない言語は、コードのまま出す。
function languageName(t: TFunction, code: string): string {
  const known = languages.find((language) => language === code);

  return known === undefined ? code : t(`player.languages.${known}`);
}

// AudioSelect は、音声のメニュー。二か国語放送の音声や、デュアルモノの主音声と副音声を選ぶ (設計書 7.5)。
export function AudioSelect({
  choices,
  value,
  onChange,
}: {
  choices: AudioChoice[];
  value: string | null;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  if (choices.length < 2) {
    return null;
  }
  const label = (choice: AudioChoice) => {
    const base =
      choice.kind === 'dualMono'
        ? t(choice.sub ? 'player.audioSub' : 'player.audioMain')
        : t('player.audioStream', { number: choice.index + 1 });

    return choice.language === null ? base : `${base}（${languageName(t, choice.language)}）`;
  };

  return (
    <Select
      label={t('player.audio')}
      size="xs"
      // 操作バーの設定の中に置く。別のウィンドウに移っても開けるよう、メニューをプレイヤーの中に描く。
      comboboxProps={{ withinPortal: false }}
      allowDeselect={false}
      data={choices.map((choice) => ({ value: choice.value, label: label(choice) }))}
      value={value}
      onChange={(next) => {
        if (next !== null) {
          onChange(next);
        }
      }}
    />
  );
}

// KyusuAudioSelect は、kyusu の経路の音声のメニュー。番組の音声の構成から作った選択肢を出し、第 2 音声がなければ出さない。
// 構成が分からないときは、主音声と第 2 音声を出す。第 2 音声がない番組では、主音声と同じものになる。
export function KyusuAudioSelect({
  choices,
  value,
  onChange,
}: {
  choices: KyusuAudioChoice[];
  value: KyusuAudio;
  onChange: (value: KyusuAudio) => void;
}) {
  const { t } = useTranslation();
  if (choices.length < 2) {
    return null;
  }
  const label = (choice: KyusuAudioChoice) => {
    let base: string;
    if (choice.kind === 'dualMono') {
      base = t(choice.slot === 1 ? 'player.audioMain' : 'player.audioSub');
    } else if (choice.kind === 'stream') {
      base = t('player.audioStream', { number: choice.slot });
    } else {
      base = t(choice.slot === 1 ? 'player.audioMain' : 'player.audioSecond');
    }

    return choice.language === null ? base : `${base}（${languageName(t, choice.language)}）`;
  };

  return (
    <Select
      label={t('player.audio')}
      size="xs"
      // 操作バーの設定の中に置く。別のウィンドウに移っても開けるよう、メニューをプレイヤーの中に描く。
      comboboxProps={{ withinPortal: false }}
      allowDeselect={false}
      data={choices.map((choice) => ({ value: String(choice.slot), label: label(choice) }))}
      value={String(value)}
      onChange={(next) => {
        if (next === '1' || next === '2') {
          onChange(next === '1' ? 1 : 2);
        }
      }}
    />
  );
}

// rates は、選べる再生速度。
const rates = ['0.75', '1', '1.25', '1.5', '2'];

// RateSelect は、録画の再生速度のメニュー (設計書 7.5)。
export function RateSelect({
  value,
  onChange,
}: {
  value: number;
  onChange: (rate: number) => void;
}) {
  const { t } = useTranslation();

  return (
    <Select
      label={t('player.rate')}
      size="xs"
      // 操作バーの設定の中に置く。別のウィンドウに移っても開けるよう、メニューをプレイヤーの中に描く。
      comboboxProps={{ withinPortal: false }}
      allowDeselect={false}
      data={rates.map((rate) => ({ value: rate, label: `${rate}x` }))}
      value={String(value)}
      onChange={(next) => {
        if (next !== null) {
          onChange(Number(next));
        }
      }}
    />
  );
}

// CropSelect は、L 字放送の切り抜き (#496) を選ぶ。拡大率と、拡大の起点を選ぶ。切り抜きは、その場限りで覚えない。
export function CropSelect({ value, onChange }: { value: Crop; onChange: (crop: Crop) => void }) {
  const { t } = useTranslation();

  return (
    <>
      <Select
        label={t('player.crop.scale')}
        size="xs"
        comboboxProps={{ withinPortal: false }}
        allowDeselect={false}
        data={cropScales.map((scale) => ({
          value: String(scale),
          label: scale === 1 ? t('player.crop.none') : `${scale}x`,
        }))}
        value={String(value.scale)}
        onChange={(next) => {
          if (next !== null) {
            onChange({ ...value, scale: Number(next) });
          }
        }}
      />
      {value.scale !== 1 && (
        <Select
          label={t('player.crop.origin')}
          size="xs"
          comboboxProps={{ withinPortal: false }}
          allowDeselect={false}
          data={cropOrigins.map((origin) => ({
            value: origin,
            label: t(`player.crop.origins.${origin}`),
          }))}
          value={value.origin}
          onChange={(next) => {
            const origin = cropOrigins.find((candidate) => candidate === next);
            if (origin !== undefined) {
              onChange({ ...value, origin });
            }
          }}
        />
      )}
    </>
  );
}

// NormalizeVolumeToggle は、設定のメニューに置く、音量の補正 (#499) の切り替え。端末ごとの設定を変える。
export function NormalizeVolumeToggle() {
  const { t } = useTranslation();
  const [enabled, setEnabled] = useNormalizeVolume();

  return (
    <Switch
      size="xs"
      label={t('player.normalizeVolume')}
      checked={enabled}
      onChange={(event) => setEnabled(event.currentTarget.checked)}
    />
  );
}

// VirtualSurroundToggle は、設定のメニューに置く、仮想サラウンド (#454) の切り替え。5.1ch の番組でだけ出す。
export function VirtualSurroundToggle() {
  const { t } = useTranslation();
  const [enabled, setEnabled] = useVirtualSurroundSetting();

  return (
    <Switch
      size="xs"
      label={t('player.virtualSurround')}
      checked={enabled}
      onChange={(event) => setEnabled(event.currentTarget.checked)}
    />
  );
}
