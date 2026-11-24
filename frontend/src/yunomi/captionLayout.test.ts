import {
  ARIBB24ActiveCoordinatePositionSetToken,
  ARIBB24ActivePositionBackwardToken,
  ARIBB24ActivePositionSetToken,
  ARIBB24CharacterCompositionDotDesignationToken,
  ARIBB24CharacterToken,
  ARIBB24JapaneseInitialParserState,
  ARIBB24MiddleSizeToken,
  ARIBB24NormalSizeToken,
  ARIBB24Parser,
  ARIBB24SetDisplayFormatToken,
  ARIBB24SetDisplayPositionToken,
  ARIBB24SetHorizontalSpacingToken,
  ARIBB24SetVerticalSpacingToken,
  type ARIBB24Token,
} from 'aribb24.js';
import { describe, expect, test } from 'vitest';

// aribb24.js のパーサーに当てたパッチ (patches/aribb24.js@2.0.34.patch) の振る舞いを確認する。
// 表示位置 (SDP) を変えても、書き込む位置は画面の上で動かない。折り返すのは、進む向きの端を越えたときだけにする。

const chars = (text: string): ARIBB24Token[] =>
  Array.from(text, (character) => ARIBB24CharacterToken.from(character));

// placed は、解析した字幕の文字と、画面の上の左端の x、基準線の y を、書いた順に返す。
function placed(tokens: ARIBB24Token[], magnification: number) {
  const parser = new ARIBB24Parser(ARIBB24JapaneseInitialParserState, { magnification });

  return parser
    .parse(tokens)
    .filter((token) => token.tag === 'Character')
    .map((token) => ({
      character: token.character,
      x: (token.state.margin[0] + token.state.position[0]) / magnification,
      y: (token.state.margin[1] + token.state.position[1]) / magnification,
    }));
}

// header は、全角の字の枠を 40×60 (字 36、字間 4、行間 24) にする。
const header: ARIBB24Token[] = [
  ARIBB24SetDisplayFormatToken.from(840, 480),
  ARIBB24SetDisplayPositionToken.from(60, 30),
  ARIBB24CharacterCompositionDotDesignationToken.from(36, 36),
  ARIBB24SetHorizontalSpacingToken.from(4),
  ARIBB24SetVerticalSpacingToken.from(24),
];

const at = (character: string, x: number, y: number) => ({ character, x, y });

describe('ARIBB24Parser の表示位置 (SDP)', () => {
  test.each([
    {
      // 放送の字幕からコピーした並び。カッコだけを広い領域に置き、本文の領域を狭めてから、カッコの続きに書く。
      name: '書き込む位置を決めた後に変えても、文字は続きに並ぶ',
      tokens: [
        ...header,
        ARIBB24ActiveCoordinatePositionSetToken.from(180, 449),
        ARIBB24MiddleSizeToken.from(),
        ...chars('《'),
        ARIBB24SetDisplayFormatToken.from(620, 480),
        ARIBB24SetDisplayPositionToken.from(170, 30),
        ARIBB24NormalSizeToken.from(),
        ...chars('俺は'),
        ARIBB24ActiveCoordinatePositionSetToken.from(290, 509),
        ...chars('一緒に'),
        ARIBB24SetDisplayFormatToken.from(840, 480),
        ARIBB24SetDisplayPositionToken.from(60, 30),
        ARIBB24MiddleSizeToken.from(),
        ...chars('》'),
      ],
      expected: [
        at('《', 180, 449),
        // 半角の《 (幅 20) の右に続く。
        at('俺', 200, 449),
        at('は', 240, 449),
        at('一', 290, 509),
        at('緒', 330, 509),
        at('に', 370, 509),
        at('》', 410, 509),
      ],
    },
    {
      // 新しい領域の左端より左に書いた後も、反対の端へ折り返さずに右へ進む。
      name: '領域の左にはみ出した位置から書いても、続きに並ぶ',
      tokens: [
        ...header,
        ARIBB24ActiveCoordinatePositionSetToken.from(100, 449),
        ARIBB24MiddleSizeToken.from(),
        ...chars('《'),
        ARIBB24SetDisplayFormatToken.from(620, 480),
        ARIBB24SetDisplayPositionToken.from(170, 30),
        ARIBB24NormalSizeToken.from(),
        ...chars('俺は君'),
      ],
      expected: [at('《', 100, 449), at('俺', 120, 449), at('は', 160, 449), at('君', 200, 449)],
    },
    {
      name: '領域の上にはみ出した位置から書いても、同じ行に並ぶ',
      tokens: [
        ...header,
        ARIBB24ActiveCoordinatePositionSetToken.from(180, 89),
        ARIBB24SetDisplayPositionToken.from(60, 100),
        ...chars('あい'),
      ],
      expected: [at('あ', 180, 89), at('い', 220, 89)],
    },
    {
      name: '文字の位置 (APS) で決めた後に変えても、文字は動かない',
      tokens: [
        ...header,
        ARIBB24ActivePositionSetToken.from(2, 0),
        ARIBB24SetDisplayPositionToken.from(100, 30),
        ...chars('あい'),
      ],
      expected: [at('あ', 140, 89), at('い', 180, 89)],
    },
    {
      name: '書き込む位置を決める前に変えたら、新しい領域の左上から書く',
      tokens: [...header, ARIBB24SetDisplayPositionToken.from(170, 90), ...chars('俺')],
      expected: [at('俺', 170, 90 + 59)],
    },
    {
      // 領域の幅は全角 2 文字分。進む向きの端を越えたら、今までどおり折り返す。
      name: '右端を越えたら次の行へ、左端を越えて戻ったら前の行の右へ折り返す',
      tokens: [
        ...header,
        ARIBB24SetDisplayFormatToken.from(80, 480),
        ARIBB24ActiveCoordinatePositionSetToken.from(60, 89),
        ...chars('あいう'),
        ARIBB24ActiveCoordinatePositionSetToken.from(60, 149),
        ARIBB24ActivePositionBackwardToken.from(),
        ...chars('え'),
      ],
      expected: [at('あ', 60, 89), at('い', 100, 89), at('う', 60, 149), at('え', 100, 89)],
    },
  ])('$name', ({ tokens, expected }) => {
    // 拡大率は、ARIBB24Parser の既定 (2) と、放送の座標そのまま (1) の両方で確認する。
    for (const magnification of [1, 2]) {
      expect(placed(tokens, magnification)).toEqual(expected);
    }
  });
});
