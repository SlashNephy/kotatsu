// TOTP (RFC 6238) のコードを作る。E2E テストで、初回セットアップで作った管理者の第二要素を登録し、ログインするのに使う。
import { createHmac } from 'node:crypto';

// decodeBase32 は、RFC 4648 の Base32 (パディングなし) を読む。
function decodeBase32(text: string): Buffer {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';
  let bits = 0;
  let value = 0;
  const bytes: number[] = [];
  for (const char of text.replace(/[\s=]/g, '').toUpperCase()) {
    const index = alphabet.indexOf(char);
    if (index < 0) {
      throw new Error(`invalid base32 character: ${char}`);
    }
    value = (value << 5) | index;
    bits += 5;
    if (bits >= 8) {
      bytes.push((value >>> (bits - 8)) & 0xff);
      bits -= 8;
    }
  }

  return Buffer.from(bytes);
}

// totp は、secret (Base32) の、時刻 now (Unix ミリ秒) の 6 桁のコードを返す。周期は 30 秒、HMAC-SHA1。
export function totp(secret: string, now = Date.now()): string {
  const counter = Buffer.alloc(8);
  counter.writeBigUInt64BE(BigInt(Math.floor(now / 30_000)));
  const digest = createHmac('sha1', decodeBase32(secret)).update(counter).digest();
  const offset = digest[digest.length - 1]! & 0x0f;
  const code = (digest.readUInt32BE(offset) & 0x7fffffff) % 1_000_000;

  return String(code).padStart(6, '0');
}
