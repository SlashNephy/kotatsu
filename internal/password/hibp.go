package password

import (
	"bufio"
	"context"
	"crypto"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultHIBPURL は、Have I Been Pwned の Pwned Passwords の範囲検索の API。
	DefaultHIBPURL = "https://api.pwnedpasswords.com"
	// hibpTimeout は、Have I Been Pwned への照会の時間の上限。
	hibpTimeout = 5 * time.Second
)

// HIBP は、Have I Been Pwned の Pwned Passwords に、パスワードが漏洩済みかを照会する (設計書 9.3)。
//
// 照会は k-匿名性の範囲検索で行い、パスワードの SHA-1 の先頭 5 文字だけを送る。
// 応答の件数からも推測されないよう、Add-Padding で件数を水増しした応答を求める。
// SHA-1 は API が定めるハッシュで、パスワードの保存には使わない。
type HIBP struct {
	url    string
	client *http.Client
}

// NewHIBP は、url の Pwned Passwords に照会する HIBP を返す。url が空なら DefaultHIBPURL にする。
func NewHIBP(url string) *HIBP {
	if url == "" {
		url = DefaultHIBPURL
	}

	return &HIBP{url: strings.TrimSuffix(url, "/"), client: &http.Client{Timeout: hibpTimeout}}
}

// ErrHIBPUnavailable は、Have I Been Pwned に照会できなかったことを表す。
var ErrHIBPUnavailable = errors.New("have i been pwned is unavailable")

// Breached は、パスワードが漏洩済みのパスワードに含まれるかを返す。
func (h *HIBP) Breached(ctx context.Context, password string) (bool, error) {
	// API が SHA-1 を定めている。照会に使う net/http が crypto/tls を通じて crypto/sha1 を読み込むので、
	// crypto.SHA1 は登録済みになる。登録されていなければ、照会できなかったものとして扱う。
	if !crypto.SHA1.Available() {
		return false, fmt.Errorf("%w: SHA-1 is not available", ErrHIBPUnavailable)
	}
	hasher := crypto.SHA1.New()
	hasher.Write([]byte(password))
	digest := strings.ToUpper(hex.EncodeToString(hasher.Sum(nil)))
	prefix, suffix := digest[:5], digest[5:]

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.url+"/range/"+prefix, nil)
	if err != nil {
		return false, fmt.Errorf("%w: %w", ErrHIBPUnavailable, err)
	}
	req.Header.Set("Add-Padding", "true")
	resp, err := h.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("%w: %w", ErrHIBPUnavailable, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("%w: HTTP %d", ErrHIBPUnavailable, resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		candidate, count, ok := strings.Cut(strings.TrimSpace(scanner.Text()), ":")
		if !ok || !strings.EqualFold(candidate, suffix) {
			continue
		}
		// 水増しの行は、件数が 0 になっている。
		n, err := strconv.Atoi(count)

		return err == nil && n > 0, nil
	}
	if err := scanner.Err(); err != nil {
		return false, fmt.Errorf("%w: %w", ErrHIBPUnavailable, err)
	}

	return false, nil
}
