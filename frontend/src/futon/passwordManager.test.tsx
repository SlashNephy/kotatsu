import {
  MantineProvider,
  NumberInput,
  PasswordInput,
  Select,
  Textarea,
  TextInput,
} from '@mantine/core';
import { cleanup, render, screen } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, describe, expect, test } from 'vitest';

import { CredentialTextInput } from './CredentialTextInput';
import { theme } from './theme';

afterEach(cleanup);

const ignoreAttributes = ['data-1p-ignore', 'data-lpignore', 'data-bwignore'];

describe('パスワードマネージャーの抑止', () => {
  test.each<{ name: string; field: ReactNode; ignored: boolean }>([
    { name: 'TextInput', field: <TextInput label="欄" />, ignored: true },
    { name: 'Textarea', field: <Textarea label="欄" />, ignored: true },
    { name: 'NumberInput', field: <NumberInput label="欄" />, ignored: true },
    { name: 'Select', field: <Select label="欄" data={['a']} />, ignored: true },
    { name: 'PasswordInput', field: <PasswordInput label="欄" />, ignored: false },
    {
      name: 'CredentialTextInput',
      field: <CredentialTextInput label="欄" autoComplete="username" />,
      ignored: false,
    },
  ])('$name の入力欄に、抑止の属性が付くか ($ignored)', ({ field, ignored }) => {
    const { container } = render(<MantineProvider theme={theme}>{field}</MantineProvider>);
    // Select は、選択肢の一覧にも同じラベルを付けるので、文字を打つ要素を直に取る。
    const input = container.querySelector('input:not([type="hidden"]), textarea');
    if (input === null) throw new Error('入力欄が見つからない');
    for (const attribute of ignoreAttributes) {
      expect(input.hasAttribute(attribute)).toBe(ignored);
    }
  });

  test('CredentialTextInput は、渡した autoComplete をそのまま付ける', () => {
    render(
      <MantineProvider theme={theme}>
        <CredentialTextInput label="欄" autoComplete="username" />
      </MantineProvider>,
    );
    expect(screen.getByLabelText('欄').getAttribute('autocomplete')).toBe('username');
  });
});
