// notificationPath は、通知を押したときに開く画面のパス (設計書 4.8、6.2) を、このアプリの中のパスに限って返す。
// サーバーが組み立てたパスだが、ほかのオリジンへ移らないよう、/ で始まり、// や \ で始まらないものだけを通す。
export function notificationPath(url: string | null | undefined): string | null {
  if (
    typeof url !== 'string' ||
    !url.startsWith('/') ||
    url.startsWith('//') ||
    url.startsWith('/\\')
  ) {
    return null;
  }

  return url;
}
