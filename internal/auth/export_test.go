package auth

import "context"

// CompleteAs は、IdP から戻ってきたリクエストのログイン中のユーザーを principal として、ログインか再認証を完了させる。
// CallbackHandler と同じ処理を、HTTP を通さずに確かめるのに使う。再認証に失敗したときも、戻すパスを返す。
func (o *OIDC) CompleteAs(
	ctx context.Context, db Database, token string, callback *OIDCCallback, client *ClientInfo, principal *Principal,
) (*OIDCReauthentication, string, error) {
	outcome, err := o.complete(ctx, db, token, callback, client, principal)
	if outcome == nil {
		return nil, "", err
	}

	return outcome.reauth, outcome.returnTo, err
}
