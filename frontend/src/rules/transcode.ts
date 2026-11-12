import type { TFunction } from 'i18next';
import { graphql } from 'react-relay';

import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';
import type { transcodePresetsQuery } from './__generated__/transcodePresetsQuery.graphql';

// noTranscode は、「変換しない」を表す、録画設定の transcodePreset の値 (#1253)。
export const noTranscode = '';

// useTranscodePresets は、録画を終えた後の変換 (#1253) で選べる画質の名前を返す。
export function useTranscodePresets(): readonly string[] {
  const data = useLazyLoadQuery<transcodePresetsQuery>(
    graphql`
      query transcodePresetsQuery {
        transcodePresets
      }
    `,
    {},
    { fetchPolicy: 'store-or-network' },
  );

  return data.transcodePresets;
}

// transcodeLabel は、変換の画質の名前を、「H.264 720p」のように書く。空なら「変換しない」にする。
export function transcodeLabel(t: TFunction, preset: string): string {
  if (preset === noTranscode) {
    return t('rules.transcodeOff');
  }
  const [height, codec] = preset.split('-');

  return t('rules.transcodePreset', { codec: codec === 'hevc' ? 'HEVC' : 'H.264', height });
}

// transcodeOptions は、変換の画質を選ぶ Select の選択肢を、「変換しない」を先頭にして返す。
export function transcodeOptions(t: TFunction, presets: readonly string[]) {
  return [noTranscode, ...presets].map((preset) => ({
    value: preset === noTranscode ? 'off' : preset,
    label: transcodeLabel(t, preset),
  }));
}

// presetOfOption は、transcodeOptions の値を、録画設定の transcodePreset の値にする。
export function presetOfOption(value: string): string {
  return value === 'off' ? noTranscode : value;
}

// optionOfPreset は、録画設定の transcodePreset の値を、transcodeOptions の値にする。
export function optionOfPreset(preset: string): string {
  return preset === noTranscode ? 'off' : preset;
}

// transcodeFrameRates は、録画を変換するときに選べる、出すコマ数 (#1509)。auto は、映像から測って 30 か 60 を選ぶ。
export const transcodeFrameRates = ['auto', '30', '60'] as const;

// frameRateLabel は、出すコマ数を、「自動」や「60 コマ/秒」のように書く。
export function frameRateLabel(t: TFunction, frameRate: string): string {
  return frameRate === 'auto'
    ? t('rules.transcodeFrameRateAuto')
    : t('rules.transcodeFrameRateFixed', { rate: frameRate });
}

// frameRateOptions は、出すコマ数を選ぶ Select の選択肢を返す。
export function frameRateOptions(t: TFunction) {
  return transcodeFrameRates.map((frameRate) => ({
    value: frameRate,
    label: frameRateLabel(t, frameRate),
  }));
}
