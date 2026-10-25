// Package redact は、エラーの文言から、秘密を含みうる値を取り除く。
package redact

import (
	"errors"
	"net/url"
)

// URLError は、err の中の *url.Error の URL を、スキームとホストだけにして返す。
// net/http と net/url のエラーは、文言に URL の全文を含む。Discord や Slack の Webhook、Web Push の送り先のように、
// URL のパスやクエリ、ユーザー情報が秘密のトークンになるものは、ログやジョブの記録に残さない。
// URL を読めなければ、URL を空にする。*url.Error がなければ、err をそのまま返す。
// 原因 (url.Error.Err) は残すので、errors.Is や errors.As で原因を確かめられる。
// fmt.Errorf は包むときに文言を作るので、ほかのエラーで包む前に呼ぶ。
func URLError(err error) error {
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		return err
	}
	urlErr.URL = origin(urlErr.URL)

	return err
}

// origin は、URL の文字列 raw を、スキームとホストだけにする。読めなければ空にする。
func origin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}

	return (&url.URL{Scheme: u.Scheme, Host: u.Host}).String()
}
