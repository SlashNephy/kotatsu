// cspNonce は、tenban が index.html に埋め込んだ、このページの CSP の nonce を返す (設計書 10.4)。
// Mantine が差し込む <style> に付けて、style-src の 'unsafe-inline' を使わずに済ませる。
export function cspNonce(): string {
  return document.querySelector<HTMLMetaElement>('meta[property="csp-nonce"]')?.nonce ?? '';
}
