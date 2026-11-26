import { createMemoryHistory, type ParsedLocation } from '@tanstack/react-router';
import { describe, expect, test } from 'vitest';

import { createScrollRestoration } from './scrollRestoration';

// at は、pathname だけを持つ location。
function at(pathname: string): { location: ParsedLocation } {
  return { location: { pathname } as ParsedLocation };
}

describe('createScrollRestoration (#2146)', () => {
  test('ほかの画面へ移ったときは働かせ、同じ画面のままクエリを変えたときは働かせない', () => {
    const history = createMemoryHistory({ initialEntries: ['/recordings'] });
    const scroll = createScrollRestoration();
    scroll.track(history);
    expect(scroll.shouldRestore(at('/recordings'))).toBe(true);

    history.replace('/recordings?order=oldest');
    expect(scroll.shouldRestore(at('/recordings'))).toBe(false);

    history.push('/recordings?program=a');
    expect(scroll.shouldRestore(at('/recordings'))).toBe(false);

    history.push('/rules');
    expect(scroll.shouldRestore(at('/rules'))).toBe(true);
  });

  test('ブラウザで戻ったときと進んだときは、同じ画面でも働かせる', () => {
    const history = createMemoryHistory({ initialEntries: ['/search'] });
    const scroll = createScrollRestoration();
    scroll.track(history);
    scroll.shouldRestore(at('/search'));
    history.push('/search?program=a');
    scroll.shouldRestore(at('/search'));

    history.back();
    expect(scroll.shouldRestore(at('/search'))).toBe(true);

    history.forward();
    expect(scroll.shouldRestore(at('/search'))).toBe(true);
  });
});
