import { describe, expect, test } from 'vitest';

import en from './en.json';
import ja from './ja.json';

// leaves は、入れ子の文言を「a.b.c」のキーと文の組にする。
function leaves(tree: object, prefix = ''): [string, string][] {
  return Object.entries(tree).flatMap(([key, value]: [string, unknown]) => {
    const path = prefix === '' ? key : `${prefix}.${key}`;

    return typeof value === 'string' ? [[path, value]] : leaves(value as object, path);
  });
}

const placeholders = (text: string) => [...text.matchAll(/\{\{[^}]+\}\}/g)].map((m) => m[0]).sort();

// 英語の文言 (#831) は、日本語の文言を足したときに訳し忘れると、日本語で出てしまう。
describe('en.json', () => {
  const english = new Map(leaves(en));

  test.each(leaves(ja))('%s', (key, text) => {
    const translated = english.get(key);
    expect(translated, `${key} is not translated`).toBeDefined();
    expect(placeholders(translated ?? '')).toEqual(placeholders(text));
  });
});
