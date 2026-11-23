import { MantineProvider } from '@mantine/core';
import { cleanup, render as renderElement } from '@testing-library/react';
import type { ReactElement } from 'react';
import { afterEach, describe, expect, it } from 'vitest';

import { nameHighlights } from '../conditions/form';
import { ProgramTitle } from './ProgramMarks';
import { theme } from './theme';

afterEach(cleanup);

// render は、マーク (Badge) を描くため、Mantine のテーマの中で描く。
const render = (element: ReactElement) =>
  renderElement(<MantineProvider theme={theme}>{element}</MantineProvider>);

describe('ProgramTitle', () => {
  it('marks the parts of the name that the keywords match (#966)', () => {
    const { container } = render(
      <ProgramTitle
        name="🈟ＮＨＫ　ニュース〜特集〜"
        highlights={[
          { text: 'nhk', regexp: false, caseSensitive: false },
          { text: '～特集～', regexp: false, caseSensitive: false },
        ]}
      />,
    );

    // 囲み文字を外し、全角を半角にした番組名に印を付ける。
    expect([...container.querySelectorAll('mark')].map((mark) => mark.textContent)).toEqual([
      'NHK',
      '〜特集〜',
    ]);
  });

  it('shows the name without marks when no keyword matches', () => {
    const { getByTestId } = render(
      <span data-testid="title">
        <ProgramTitle
          name="こたつの時間"
          highlights={[{ text: 'ねこ', regexp: false, caseSensitive: false }]}
        />
      </span>,
    );

    expect(getByTestId('title').querySelectorAll('mark')).toHaveLength(0);
    expect(getByTestId('title').textContent).toBe('こたつの時間');
  });
});

describe('nameHighlights', () => {
  it('keeps the keywords that look at the name', () => {
    expect(
      nameHighlights([
        { text: 'こたつ', regexp: false, caseSensitive: false, fields: ['NAME', 'DESCRIPTION'] },
        { text: 'あらすじ', regexp: false, caseSensitive: false, fields: ['DESCRIPTION'] },
        { text: ' ', regexp: false, caseSensitive: false, fields: ['NAME'] },
        { text: '#[0-9]+', regexp: true, caseSensitive: null, fields: ['NAME'] },
      ]),
    ).toEqual([
      { text: 'こたつ', regexp: false, caseSensitive: false },
      { text: '#[0-9]+', regexp: true, caseSensitive: false },
    ]);
  });
});
