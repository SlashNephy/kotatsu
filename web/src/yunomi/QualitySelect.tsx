import { Alert, Select, Switch } from '@mantine/core';
import { useTranslation } from 'react-i18next';

import { original, type PlaybackRoute } from './route';

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
