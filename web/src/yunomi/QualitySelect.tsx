import { Alert, Select, Switch } from '@mantine/core';
import { useTranslation } from 'react-i18next';

import type { AudioChoice, KyusuAudioChoice } from './audio';
import { original, type KyusuAudio, type PlaybackRoute } from './route';

type Props = {
  choices: string[];
  quality: string | null;
  onChange: (quality: string) => void;
};

// QualitySelect は、画質のメニュー。オリジナルはブラウザの中での変換、それ以外は kyusu の画質 (設計書 7.1)。
export function QualitySelect({ choices, quality, onChange }: Props) {
  const { t } = useTranslation();
  // 選べるものが 1 つしかなければ、メニューを出さない。
  if (choices.length < 2) {
    return null;
  }

  return (
    <Select
      aria-label={t('player.quality')}
      size="xs"
      w={200}
      allowDeselect={false}
      data={choices.map((choice) => ({ value: choice, label: qualityLabel(choice, t) }))}
      value={quality}
      onChange={(value) => {
        if (value !== null) {
          onChange(value);
        }
      }}
    />
  );
}

// qualityLabel は、画質の名前を、メニューに出す文に変える。kyusu の HEVC の画質は、末尾に -hevc が付く。
function qualityLabel(name: string, t: ReturnType<typeof useTranslation>['t']): string {
  if (name === original) {
    return t('player.original');
  }
  const hevc = name.endsWith('-hevc');

  return t(hevc ? 'player.presetHevc' : 'player.preset', {
    name: hevc ? name.slice(0, -'-hevc'.length) : name,
  });
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

// languageNames は、記述子の言語のコード (ISO 639) を、メニューに出す名前にする。知らない言語は、コードのまま出す。
const languageNames: Record<string, string> = {
  jpn: '日本語',
  eng: '英語',
  kor: '韓国語',
  zho: '中国語',
  chi: '中国語',
  // ARIB STD-B10 で、上の言語に当たらないことを表す。
  etc: 'その他の言語',
};

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

    return choice.language === null
      ? base
      : `${base} (${languageNames[choice.language] ?? choice.language})`;
  };

  return (
    <Select
      aria-label={t('player.audio')}
      size="xs"
      w={180}
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

    return choice.language === null
      ? base
      : `${base} (${languageNames[choice.language] ?? choice.language})`;
  };

  return (
    <Select
      aria-label={t('player.audio')}
      size="xs"
      w={180}
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
      aria-label={t('player.rate')}
      size="xs"
      w={90}
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
