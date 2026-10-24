import { afterEach, describe, expect, test } from 'vitest';

import { cspNonce } from './nonce';

afterEach(() => {
  document.head.innerHTML = '';
});

describe('cspNonce', () => {
  test('tenban が埋め込んだ nonce を返す', () => {
    const meta = document.createElement('meta');
    meta.setAttribute('property', 'csp-nonce');
    meta.nonce = 'abc123';
    document.head.append(meta);

    expect(cspNonce()).toBe('abc123');
  });

  test('nonce がなければ空文字列を返す', () => {
    expect(cspNonce()).toBe('');
  });
});
