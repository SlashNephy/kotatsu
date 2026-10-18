import type { FormattedCommentInput } from '@xpadev-net/niconicomments';

// JikkyoComment は、描画する実況のコメント。time は、描画の時計 (秒) での、コメントを流し始める時刻。
export type JikkyoComment = {
  time: number;
  content: string;
  // mail は、色や位置などのコマンドを空白で区切ったもの (184 shita big yellow など)。
  mail: string;
  // userId は、投稿したユーザーの匿名の ID。ミュートに使う (#488)。分からなければ省く。
  userId?: string;
};

// KeywordMuteMatch は、ミュートするキーワードの照らし方。
export type KeywordMuteMatch = 'PARTIAL' | 'PREFIX' | 'SUFFIX' | 'EXACT' | 'REGEX';

// keywordMuteMatches は、照らし方の一覧。設定の画面の選択肢の順。
export const keywordMuteMatches: readonly KeywordMuteMatch[] = [
  'PARTIAL',
  'PREFIX',
  'SUFFIX',
  'EXACT',
  'REGEX',
];

// isKeywordMuteMatch は、サーバーから届いた照らし方が、この画面の知っているものかを返す。知らないものは保存し直さない。
export const isKeywordMuteMatch = (value: string): value is KeywordMuteMatch =>
  (keywordMuteMatches as readonly string[]).includes(value);

// CommentSettings は、実況のコメントの表示とミュートの設定 (#488)。ユーザーごとに持ち、端末をまたいで同期する。
export type CommentSettings = {
  // opacity は、不透明度 (0.1〜1)。
  readonly opacity: number;
  // speed は、流れる速さの倍率 (0.5〜2)。
  readonly speed: number;
  // scale は、文字の大きさの倍率 (0.5〜1.5)。
  readonly scale: number;
  // showAll は、重なっても、すべてのコメントを出すかどうか。
  readonly showAll: boolean;
  readonly mutedKeywords: readonly { readonly text: string; readonly match: string }[];
  readonly mutedUsers: readonly string[];
};

export const defaultCommentSettings: CommentSettings = {
  opacity: 1,
  speed: 1,
  scale: 1,
  showAll: true,
  mutedKeywords: [],
  mutedUsers: [],
};

// defaultCommentSeconds は、niconicomments が流れるコメントを出しておく秒数。速さの倍率は、これを縮めて変える。
const defaultCommentSeconds = 3;
// limitedComments は、すべては出さないときに、画面に同時に出すコメントの数の上限。
export const limitedComments = 40;

// normalizeForMute は、キーワードと照らすために、全角と半角、大文字と小文字をそろえる。
const normalizeForMute = (text: string) => text.normalize('NFKC').toLowerCase();

// compileMute は、ミュートの設定から、コメントをミュートするかどうかを返す関数を作る。正しくない正規表現は無視する。
export function compileMute(settings: CommentSettings): (comment: JikkyoComment) => boolean {
  const users = new Set(settings.mutedUsers);
  const matchers: ((content: string, normalized: string) => boolean)[] = [];
  for (const keyword of settings.mutedKeywords) {
    if (keyword.match === 'REGEX') {
      try {
        const pattern = new RegExp(keyword.text, 'i');
        matchers.push((content) => pattern.test(content));
      } catch {
        // 書きかけの正規表現などは、ミュートに使わない。
      }
      continue;
    }
    const text = normalizeForMute(keyword.text);
    if (text === '') {
      continue;
    }
    switch (keyword.match) {
      case 'PREFIX':
        matchers.push((_, normalized) => normalized.startsWith(text));
        break;
      case 'SUFFIX':
        matchers.push((_, normalized) => normalized.endsWith(text));
        break;
      case 'EXACT':
        matchers.push((_, normalized) => normalized === text);
        break;
      default:
        matchers.push((_, normalized) => normalized.includes(text));
    }
  }

  return (comment) => {
    if (comment.userId !== undefined && users.has(comment.userId)) {
      return true;
    }
    if (matchers.length === 0) {
      return false;
    }
    const content = comment.content.trim();
    const normalized = normalizeForMute(content);

    return matchers.some((matches) => matches(content, normalized));
  };
}

// shouldDraw は、コメントを流すかどうかを返す。空のコメントと、/ で始まる運営のコマンド (/nicoad など) は流さない。
export function shouldDraw(comment: JikkyoComment): boolean {
  const content = comment.content.trim();

  return content !== '' && !content.startsWith('/');
}

// toFormatted は、コメントを niconicomments の形にする。vpos は 1/100 秒の単位。
// 速さの倍率 speed が 1 でなければ、出しておく秒数のコマンド (@秒) を足す。コメント自身が秒数を指定していれば、それに従う。
export function toFormatted(comment: JikkyoComment, id: number, speed = 1): FormattedCommentInput {
  const mail = comment.mail.split(/\s+/).filter((command) => command !== '');
  if (speed !== 1 && !mail.some((command) => /^[@＠][0-9.]/.test(command))) {
    mail.push(`@${(defaultCommentSeconds / speed).toFixed(2)}`);
  }

  return { id, vpos: Math.round(comment.time * 100), content: comment.content, mail };
}
