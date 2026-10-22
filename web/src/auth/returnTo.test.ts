import { beforeEach, describe, expect, it } from 'vitest';

import { consumeReturnTo, isSafePath, rememberReturnTo } from './returnTo';

describe('returnTo', () => {
  beforeEach(() => sessionStorage.clear());

  it.each([
    ['/authorize?client_id=x', true],
    ['//evil.example/', false],
    ['/\\evil.example/', false],
    ['https://evil.example/', false],
    ['authorize', false],
  ])('isSafePath(%s) は %s', (path, want) => {
    expect(isSafePath(path)).toBe(want);
  });

  it('覚えたパスを一度だけ返す', () => {
    rememberReturnTo('/authorize?state=a', 1000);
    expect(consumeReturnTo(2000)).toBe('/authorize?state=a');
    expect(consumeReturnTo(2000)).toBeNull();
  });

  it('10 分を過ぎたパスは使わない', () => {
    rememberReturnTo('/authorize', 0);
    expect(consumeReturnTo(10 * 60 * 1000 + 1)).toBeNull();
  });

  it('ほかのサイトへのパスは覚えない', () => {
    rememberReturnTo('//evil.example/', 0);
    expect(consumeReturnTo(0)).toBeNull();
  });
});
