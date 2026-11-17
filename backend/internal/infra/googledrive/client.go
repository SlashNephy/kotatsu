// Package googledrive は、Google Drive API (v3) のうち、ライブラリとして読むのに要るもの (一覧、情報、範囲を指定した読み出し) を呼ぶ
// (設計書 5.8、#1902)。スコープは drive.readonly だけを前提にし、書き込む API は呼ばない。
//
// 上限に当たったとき (429 と、403 の rateLimitExceeded、userRateLimitExceeded) と 5xx は、Google の案内どおり、倍々に伸ばした間に
// 乱数を足して待ち、決まった回数までやり直す。Retry-After があれば、それに従う。ダウンロードの上限 (downloadQuotaExceeded) は、
// 待っても解けないので、やり直さない。
package googledrive

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultURL は、Drive API のオリジン。
	DefaultURL = "https://www.googleapis.com"
	// FolderMIMEType は、フォルダの MIME タイプ。
	FolderMIMEType = "application/vnd.google-apps.folder"
	// MyDriveID は、マイドライブの根元を指す別名。
	MyDriveID = "root"
	// googleAppsPrefix は、Google ドキュメントなどの、中身をそのまま読めないファイルの MIME タイプの前置き。
	googleAppsPrefix = "application/vnd.google-apps."
	// pageSize は、一覧の 1 ページの件数。files.list の上限。
	pageSize = 1000
	// drivesPageSize は、共有ドライブの一覧の 1 ページの件数。drives.list の上限。
	drivesPageSize = 100
	// maxAttempts は、上限に当たったときなどに、やり直す回数の上限 (初めの 1 回を含む)。
	maxAttempts = 6
	// maxBackoff は、やり直すまでに待つ長さの上限。Google の案内 (32 秒か 64 秒) に合わせる。
	maxBackoff = 32 * time.Second
	// maxRetryAfter は、Retry-After に従って待つ長さの上限。
	maxRetryAfter = 30 * time.Second
	// maxTotalWait は、1 回の呼び出しで、やり直すために待つ長さの合計の上限。索引を作っている tenban が、書きかけの索引を
	// 伸ばさないまま、ほかの tenban に落ちたとみなされる時間 (1 分、設計書 付録 A) より短くする。
	maxTotalWait = 45 * time.Second
	// maxErrorBody は、エラーの応答の本文を読む大きさの上限。
	maxErrorBody = 1 << 16
	// fileFields は、ファイルの情報として引く項目。
	fileFields = "id,name,mimeType,size,modifiedTime,trashed,parents,driveId"
	// paramFields と paramAllDrives は、クエリの引数の名前。
	paramFields    = "fields"
	paramAllDrives = "supportsAllDrives"
	// paramPageSize は、1 ページの件数を渡す引数の名前。
	paramPageSize = "pageSize"
	// valueTrue は、真を表す引数の値。
	valueTrue = "true"
)

var (
	// ErrNotFound は、ファイルかフォルダがない (見えない) ことを表す。
	ErrNotFound = errors.New("not found in Google Drive")
	// ErrRateLimited は、やり直しても上限に当たったままだったことを表す。
	ErrRateLimited = errors.New("rate limited by Google Drive")
	// ErrDownloadQuota は、ファイルのダウンロードの上限に当たったことを表す。待っても解けないので、やり直さない。
	ErrDownloadQuota = errors.New("the download quota of the file was exceeded")
	// ErrUnauthorized は、トークンを作り直しても、認可されなかったことを表す。
	ErrUnauthorized = errors.New("unauthorized by Google Drive")
	// ErrNotConnected は、Google Drive を接続していないか、接続が切れていて、アクセストークンを作れないことを表す。
	// TokenSource が返す。
	ErrNotConnected = errors.New("the Google Drive is not connected")
	// ErrRangeIgnored は、範囲を指定した読み出しに、指定と違う範囲が返ったことを表す。
	ErrRangeIgnored = errors.New("the Drive API returned a different range")
)

// TokenSource は、Drive API に付けるアクセストークンを返す。
type TokenSource interface {
	// Token は、アクセストークンを返す。
	Token(ctx context.Context) (string, error)
	// Invalidate は、手元のアクセストークンを捨て、次の Token で作り直させる。401 が返ったときに呼ぶ。
	Invalidate()
}

// Client は Drive API のクライアント。
type Client struct {
	http    *http.Client
	baseURL string
	tokens  TokenSource
	// sleep は、やり直す前に待つ。テストで差し替える。
	sleep func(ctx context.Context, d time.Duration) error
}

// NewClient は Client を返す。baseURL が空なら DefaultURL にする。httpClient には、全体のタイムアウトを付けない。
// 何 GB もあるファイルを読み通すので、読み終える前に切れるため。
func NewClient(httpClient *http.Client, baseURL string, tokens TokenSource) *Client {
	if baseURL == "" {
		baseURL = DefaultURL
	}

	return &Client{http: httpClient, baseURL: strings.TrimSuffix(baseURL, "/"), tokens: tokens, sleep: sleepContext}
}

// File は、Drive のファイルかフォルダ。
type File struct {
	ID       string
	Name     string
	MIMEType string
	// Size は、大きさ (バイト)。フォルダと、Google ドキュメントなどでは 0。
	Size         int64
	ModifiedTime time.Time
	// Parents は、親のフォルダの ID。マイドライブと共有ドライブの根元では空。
	Parents []string
	// DriveID は、ファイルのある共有ドライブの ID。マイドライブなら空。
	DriveID string
}

// Folder は、フォルダかどうかを返す。
func (f *File) Folder() bool {
	return f.MIMEType == FolderMIMEType
}

// Blob は、中身をそのまま読めるファイル (フォルダでも Google ドキュメントなどでもないもの) かどうかを返す。
func (f *File) Blob() bool {
	return !strings.HasPrefix(f.MIMEType, googleAppsPrefix)
}

// Drive は、共有ドライブ。
type Drive struct {
	ID          string
	Name        string
	CreatedTime time.Time
}

type fileJSON struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	MIMEType     string    `json:"mimeType"`
	Size         string    `json:"size"`
	ModifiedTime time.Time `json:"modifiedTime"`
	Trashed      bool      `json:"trashed"`
	Parents      []string  `json:"parents"`
	DriveID      string    `json:"driveId"`
}

func (f *fileJSON) file() *File {
	size, _ := strconv.ParseInt(f.Size, 10, 64)

	parents := f.Parents
	if parents == nil {
		parents = []string{}
	}

	return &File{ID: f.ID, Name: f.Name, MIMEType: f.MIMEType, Size: size, ModifiedTime: f.ModifiedTime, Parents: parents, DriveID: f.DriveID}
}

// About は、接続したアカウントのメールアドレス (なければ表示名) を返す。
func (c *Client) About(ctx context.Context) (string, error) {
	var body struct {
		User struct {
			DisplayName  string `json:"displayName"`
			EmailAddress string `json:"emailAddress"`
		} `json:"user"`
	}
	if err := c.getJSON(ctx, "/drive/v3/about", url.Values{paramFields: {"user(displayName,emailAddress)"}}, &body); err != nil {
		return "", err
	}
	if body.User.EmailAddress != "" {
		return body.User.EmailAddress, nil
	}

	return body.User.DisplayName, nil
}

// Drives は、見える共有ドライブを、すべてのページを読んで返す。
func (c *Client) Drives(ctx context.Context) ([]*Drive, error) {
	drives := []*Drive{}
	token := ""
	for {
		query := url.Values{
			paramPageSize: {strconv.Itoa(drivesPageSize)}, paramFields: {"nextPageToken,drives(id,name,createdTime)"},
		}
		if token != "" {
			query.Set("pageToken", token)
		}
		var body struct {
			NextPageToken string `json:"nextPageToken"`
			Drives        []struct {
				ID          string    `json:"id"`
				Name        string    `json:"name"`
				CreatedTime time.Time `json:"createdTime"`
			} `json:"drives"`
		}
		if err := c.getJSON(ctx, "/drive/v3/drives", query, &body); err != nil {
			return nil, err
		}
		for _, d := range body.Drives {
			drives = append(drives, &Drive{ID: d.ID, Name: d.Name, CreatedTime: d.CreatedTime})
		}
		if body.NextPageToken == "" {
			return drives, nil
		}
		token = body.NextPageToken
	}
}

// GetDrive は、共有ドライブ id を返す。
func (c *Client) GetDrive(ctx context.Context, id string) (*Drive, error) {
	var body struct {
		ID          string    `json:"id"`
		Name        string    `json:"name"`
		CreatedTime time.Time `json:"createdTime"`
	}
	if err := c.getJSON(ctx, "/drive/v3/drives/"+url.PathEscape(id), url.Values{paramFields: {"id,name,createdTime"}}, &body); err != nil {
		return nil, err
	}

	return &Drive{ID: body.ID, Name: body.Name, CreatedTime: body.CreatedTime}, nil
}

// Children は、フォルダ parentID の中の、ゴミ箱にないファイルとフォルダを、すべてのページを読んで返す。
// driveID は、フォルダのある共有ドライブ。マイドライブなら空にする。
func (c *Client) Children(ctx context.Context, parentID, driveID string) ([]*File, error) {
	files := []*File{}
	seen := map[string]bool{}
	token := ""
	for {
		query := url.Values{
			"q":           {fmt.Sprintf("'%s' in parents and trashed = false", escapeQuery(parentID))},
			paramFields:   {"nextPageToken,files(" + fileFields + ")"},
			paramPageSize: {strconv.Itoa(pageSize)}, "orderBy": {"folder,name_natural,createdTime"},
			paramAllDrives: {valueTrue}, "includeItemsFromAllDrives": {valueTrue},
		}
		if driveID == "" {
			query.Set("corpora", "user")
		} else {
			query.Set("corpora", "drive")
			query.Set("driveId", driveID)
		}
		if token != "" {
			query.Set("pageToken", token)
		}
		var body struct {
			NextPageToken string     `json:"nextPageToken"`
			Files         []fileJSON `json:"files"`
		}
		if err := c.getJSON(ctx, "/drive/v3/files", query, &body); err != nil {
			return nil, err
		}
		for i := range body.Files {
			// 同じ名前が並ぶと、ページをまたいで同じものが返ることがあるので、ID で重ねない。
			if !body.Files[i].Trashed && !seen[body.Files[i].ID] {
				seen[body.Files[i].ID] = true
				files = append(files, body.Files[i].file())
			}
		}
		if body.NextPageToken == "" {
			return files, nil
		}
		token = body.NextPageToken
	}
}

// SearchFiles は、名前に name を含む、ゴミ箱にないファイル (フォルダを除く) を、最大 maxPages ページ読んで返す。
// driveID は、探す共有ドライブ。マイドライブなら空にする。Drive の q では、あるフォルダの子孫だけには絞れないので、
// 呼ぶ側で、親をたどって絞る。2 つ目の戻り値は、ページが残っていて、読み切っていないかどうか。
func (c *Client) SearchFiles(ctx context.Context, name, driveID string, maxPages int) ([]*File, bool, error) {
	files := []*File{}
	token := ""
	for range maxPages {
		query := url.Values{
			"q":           {fmt.Sprintf("name contains '%s' and mimeType != '%s' and trashed = false", escapeQuery(name), FolderMIMEType)},
			paramFields:   {"nextPageToken,files(" + fileFields + ")"},
			paramPageSize: {strconv.Itoa(pageSize)}, paramAllDrives: {valueTrue}, "includeItemsFromAllDrives": {valueTrue},
		}
		if driveID == "" {
			query.Set("corpora", "user")
		} else {
			query.Set("corpora", "drive")
			query.Set("driveId", driveID)
		}
		if token != "" {
			query.Set("pageToken", token)
		}
		var body struct {
			NextPageToken string     `json:"nextPageToken"`
			Files         []fileJSON `json:"files"`
		}
		if err := c.getJSON(ctx, "/drive/v3/files", query, &body); err != nil {
			return nil, false, err
		}
		for i := range body.Files {
			if !body.Files[i].Trashed {
				files = append(files, body.Files[i].file())
			}
		}
		if body.NextPageToken == "" {
			return files, false, nil
		}
		token = body.NextPageToken
	}

	return files, true, nil
}

// Get は、ファイルかフォルダ id の情報を返す。ゴミ箱にあれば ErrNotFound を返す。
func (c *Client) Get(ctx context.Context, id string) (*File, error) {
	var body fileJSON
	query := url.Values{paramFields: {fileFields}, paramAllDrives: {valueTrue}}
	if err := c.getJSON(ctx, "/drive/v3/files/"+url.PathEscape(id), query, &body); err != nil {
		return nil, err
	}
	if body.Trashed {
		return nil, fmt.Errorf("%w: %s is in the trash", ErrNotFound, id)
	}

	return body.file(), nil
}

// Download は、ファイル id の中身を、offset バイト目から終わりまで読む本文を返す (alt=media と Range)。
// 呼ぶ側は、読み終えたら閉じる。途中で切れたら、読めた所から呼び直す。
func (c *Client) Download(ctx context.Context, id string, offset int64) (io.ReadCloser, error) {
	query := url.Values{"alt": {"media"}, paramAllDrives: {valueTrue}}
	res, err := c.do(ctx, "/drive/v3/files/"+url.PathEscape(id), query, func(req *http.Request) {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(offset, 10)+"-")
	})
	if err != nil {
		return nil, err
	}
	switch res.StatusCode {
	case http.StatusPartialContent:
		start, ok := contentRangeStart(res.Header.Get("Content-Range"))
		if !ok || start != offset {
			_ = res.Body.Close()

			return nil, fmt.Errorf("%w: asked for %d, got %q", ErrRangeIgnored, offset, res.Header.Get("Content-Range"))
		}
	case http.StatusOK:
		// 範囲を無視して頭から返された。頭から読むのなら、そのまま使える。
		if offset != 0 {
			_ = res.Body.Close()

			return nil, fmt.Errorf("%w: asked for %d, got the whole file", ErrRangeIgnored, offset)
		}
	default:
		_ = res.Body.Close()

		return nil, fmt.Errorf("unexpected status %d from the Drive API", res.StatusCode)
	}

	return res.Body, nil
}

// contentRangeStart は、Content-Range (bytes 100-199/200) の始まりを返す。
func contentRangeStart(header string) (int64, bool) {
	rest, ok := strings.CutPrefix(header, "bytes ")
	if !ok {
		return 0, false
	}
	first, _, ok := strings.Cut(rest, "-")
	if !ok {
		return 0, false
	}
	start, err := strconv.ParseInt(first, 10, 64)

	return start, err == nil
}

func (c *Client) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	res, err := c.do(ctx, path, query, nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d from the Drive API", res.StatusCode)
	}
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		return fmt.Errorf("failed to decode the response from the Drive API: %w", err)
	}

	return nil
}

// do は、GET を送り、成功 (2xx) の応答を返す。上限と 5xx はやり直し、401 はトークンを作り直して 1 度だけやり直す。
// 404 は ErrNotFound を返す。
func (c *Client) do(ctx context.Context, path string, query url.Values, prepare func(*http.Request)) (*http.Response, error) {
	refreshed, skipWait := false, false
	var lastErr error
	var waited time.Duration
	// トークンを作り直した後の 1 回は、試す回数に数えない。
	limit := maxAttempts
	for attempt := 0; attempt < limit; attempt++ {
		// トークンを作り直した直後は、待たずにやり直す。
		if attempt > 0 && !skipWait {
			wait := backoff(attempt, lastErr)
			if waited+wait > maxTotalWait {
				break
			}
			waited += wait
			if err := c.sleep(ctx, wait); err != nil {
				return nil, err
			}
		}
		skipWait = false
		token, err := c.tokens.Token(ctx)
		if err != nil {
			// トークンを作れないのは、接続していないか、外れたときなので、やり直さない。
			return nil, err
		}
		res, err := c.send(ctx, token, path, query, prepare)
		if err != nil {
			if ctx.Err() != nil {
				return nil, err
			}
			// 接続の失敗は、読むだけの GET なので、やり直してよい。
			lastErr = err

			continue
		}
		if res.StatusCode < http.StatusMultipleChoices {
			return res, nil
		}
		apiErr := readError(res)
		switch {
		case res.StatusCode == http.StatusUnauthorized && !refreshed:
			c.tokens.Invalidate()
			refreshed, skipWait = true, true
			limit++
			lastErr = apiErr

			continue
		case res.StatusCode == http.StatusUnauthorized:
			return nil, fmt.Errorf("%w: %w", ErrUnauthorized, apiErr)
		case res.StatusCode == http.StatusNotFound:
			return nil, fmt.Errorf("%w: %w", ErrNotFound, apiErr)
		case apiErr.hasReason("downloadQuotaExceeded"):
			return nil, fmt.Errorf("%w: %w", ErrDownloadQuota, apiErr)
		case apiErr.retryable():
			lastErr = apiErr

			continue
		default:
			return nil, apiErr
		}
	}
	var apiErr *APIError
	if errors.As(lastErr, &apiErr) && apiErr.rateLimited() {
		return nil, fmt.Errorf("%w: %w", ErrRateLimited, lastErr)
	}

	return nil, fmt.Errorf("failed to call the Drive API after retrying: %w", lastErr)
}

func (c *Client) send(ctx context.Context, token, path string, query url.Values, prepare func(*http.Request)) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path+"?"+query.Encode(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to build a request to the Drive API: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if prepare != nil {
		prepare(req)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call the Drive API: %w", err)
	}

	return res, nil
}

// APIError は、Drive API が返したエラー。
type APIError struct {
	Status  int
	Message string
	Reasons []string
	// RetryAfter は、Retry-After の長さ。なければ 0。
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	return fmt.Sprintf("the Drive API returned %d (%s): %s", e.Status, strings.Join(e.Reasons, ","), e.Message)
}

func (e *APIError) hasReason(reason string) bool {
	return slices.Contains(e.Reasons, reason)
}

// rateLimited は、上限に当たったことを表すエラーかどうかを返す。
func (e *APIError) rateLimited() bool {
	return e.Status == http.StatusTooManyRequests ||
		(e.Status == http.StatusForbidden && (e.hasReason("rateLimitExceeded") || e.hasReason("userRateLimitExceeded")))
}

// retryable は、待ってやり直せばよいエラーかどうかを返す。
func (e *APIError) retryable() bool {
	return e.rateLimited() || e.Status >= http.StatusInternalServerError
}

// readError は、エラーの応答を読んで閉じる。
func readError(res *http.Response) *APIError {
	defer res.Body.Close()
	apiErr := &APIError{Status: res.StatusCode, Message: http.StatusText(res.StatusCode), Reasons: []string{}, RetryAfter: 0}
	if seconds, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil && seconds > 0 {
		apiErr.RetryAfter = min(time.Duration(seconds)*time.Second, maxRetryAfter)
	}
	var body struct {
		Error struct {
			Message string `json:"message"`
			Errors  []struct {
				Reason string `json:"reason"`
			} `json:"errors"`
		} `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, maxErrorBody)).Decode(&body); err != nil {
		return apiErr
	}
	if body.Error.Message != "" {
		apiErr.Message = body.Error.Message
	}
	for _, e := range body.Error.Errors {
		apiErr.Reasons = append(apiErr.Reasons, e.Reason)
	}

	return apiErr
}

// backoff は、attempt 回目 (1 から) のやり直しの前に待つ長さを返す。Retry-After があればそれに従い、なければ
// 倍々に伸ばした長さに 1 秒までの乱数を足す (Google の案内)。
func backoff(attempt int, lastErr error) time.Duration {
	var apiErr *APIError
	if errors.As(lastErr, &apiErr) && apiErr.RetryAfter > 0 {
		return apiErr.RetryAfter
	}
	wait := min(time.Duration(1<<(attempt-1))*time.Second, maxBackoff)

	return wait + jitter()
}

// jitter は、1 秒までの乱数の長さを返す。
func jitter() time.Duration {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(time.Second)))
	if err != nil {
		return 0
	}

	return time.Duration(n.Int64())
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("canceled while waiting to retry: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

// escapeQuery は、files.list の q に入れる文字列の \ と ' を逃がす。
func escapeQuery(s string) string {
	return strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s)
}
