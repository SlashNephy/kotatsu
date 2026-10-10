import type { FormattedCommentInput } from '@xpadev-net/niconicomments';

// JikkyoComment は、描画する実況のコメント。time は、描画の時計 (秒) での、コメントを流し始める時刻。
export type JikkyoComment = {
  time: number;
  content: string;
  // mail は、色や位置などのコマンドを空白で区切ったもの (184 shita big yellow など)。
  mail: string;
};

// shouldDraw は、コメントを流すかどうかを返す。空のコメントと、/ で始まる運営のコマンド (/nicoad など) は流さない。
export function shouldDraw(comment: JikkyoComment): boolean {
  const content = comment.content.trim();

  return content !== '' && !content.startsWith('/');
}

// toFormatted は、コメントを niconicomments の形にする。vpos は 1/100 秒の単位。
export function toFormatted(comment: JikkyoComment, id: number): FormattedCommentInput {
  return {
    id,
    vpos: Math.round(comment.time * 100),
    content: comment.content,
    mail: comment.mail.split(/\s+/).filter((command) => command !== ''),
  };
}
