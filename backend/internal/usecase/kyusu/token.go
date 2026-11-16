package kyusu

import "github.com/SlashNephy/kotatsu/internal/lib/innertoken"

// derivePurpose は、アプリの秘密キーから、tenban が kyusu に送るトークンを導くときの用途。
const derivePurpose = "kotatsu/kyusu/v1"

// ErrNoKey は、アプリの秘密キーがないことを表す。
var ErrNoKey = innertoken.ErrNoKey

// Token は、tenban が kyusu に送るリクエストを認可するトークン。
// kyusu の HTTP はインスタンスのサーバーに載り、tenban の接続元の制限やセッションを通らないので、
// アプリの秘密キーを知るインスタンスからのリクエストだけを受け付ける。
type Token = innertoken.Token

// NewToken は、アプリの秘密キー secret からトークンを導く。
func NewToken(secret string) (*Token, error) {
	return innertoken.New(secret, derivePurpose)
}
