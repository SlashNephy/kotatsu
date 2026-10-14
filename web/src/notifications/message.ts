// MessageParts は、通知の文を、番組名の前、番組名、番組名の後に分けたもの。
export type MessageParts = {
  before: string;
  // name は、文の中の番組名。番組名がなければ (文に含まれていなければ) undefined。
  name: string | undefined;
  after: string;
};

// splitMessage は、通知の文 (message) から、材料 (payload) の番組名を探して、前後に分ける。
// 文はサーバーが作るので、画面では番組名の部分だけを差し替えて、囲み文字を印として描く。
// payload は JSON の文字列。読めないときや、番組名が文になければ、文をそのまま返す。
export function splitMessage(message: string, payload: string): MessageParts {
  let name: unknown;
  try {
    name = (JSON.parse(payload) as Record<string, unknown> | null)?.name;
  } catch {
    name = undefined;
  }
  const index = typeof name === 'string' && name !== '' ? message.indexOf(name) : -1;
  if (typeof name !== 'string' || index < 0) {
    return { before: message, name: undefined, after: '' };
  }

  return {
    before: message.slice(0, index),
    name,
    after: message.slice(index + name.length),
  };
}
