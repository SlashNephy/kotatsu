import { InputBase, type InputBaseProps } from '@mantine/core';
import type { ComponentPropsWithoutRef } from 'react';

type Props = InputBaseProps & Omit<ComponentPropsWithoutRef<'input'>, keyof InputBaseProps>;

// パスワードマネージャーに埋めさせる、ユーザー名の欄 (#1709)。
// テーマは TextInput の既定に、パスワードマネージャーを止める属性を付ける (theme.ts)。
// Mantine は undefined の props を既定で埋めるので、TextInput のままでは属性を外せない。
// そのため、既定を付けていない InputBase で描く。見た目は TextInput と同じ。
export function CredentialTextInput(props: Props) {
  return <InputBase component="input" {...props} />;
}
