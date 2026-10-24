package password

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
)

// blocklistData は、よく使われるパスワードの一覧 (SecLists の上位 10 万件のうち 8 文字以上のもの、小文字)。
// scripts/update-password-blocklist.sh で作り直す。出典は blocklist.SOURCE、ライセンスは blocklist.LICENSE にある。
//
//go:embed blocklist.txt.gz
var blocklistData []byte

// loadBlocklist は、ブロックリストを読み込む。
func loadBlocklist() (map[string]struct{}, error) {
	r, err := gzip.NewReader(bytes.NewReader(blocklistData))
	if err != nil {
		return nil, fmt.Errorf("failed to open the password blocklist: %w", err)
	}
	words := map[string]struct{}{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		words[scanner.Text()] = struct{}{}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read the password blocklist: %w", err)
	}

	return words, nil
}
