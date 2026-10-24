// Package mcpserver は、MCP のクライアントに、番組、予約、ルール、録画を扱うツールを公開する (#964、設計書 9.10)。
// トークンは、internal/oauth の認可サーバーが発行したものだけを受け付ける。
package mcpserver

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vektah/gqlparser/v2/ast"

	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/httpserver"
	"github.com/SlashNephy/kotatsu/internal/lib/buildinfo"
	"github.com/SlashNephy/kotatsu/internal/oauth"
	"github.com/SlashNephy/kotatsu/internal/webui"
)

const (
	// maxRequestBytes は、/mcp のリクエストの本文の大きさの上限。
	maxRequestBytes = 1 << 20
	// maxResultBytes は、ツールの応答の大きさの上限。Claude Code は、MCP の出力を既定で 25,000 トークンまでしか受け取らないので、
	// それに収まる大きさにする。超えたら、件数を減らすよう伝える。
	maxResultBytes = 60_000
	// methodToolsCall は、ツールを呼ぶ JSON-RPC のメソッド。
	methodToolsCall = "tools/call"
)

// Authenticator は、アクセストークンを確かめる。*oauth.Server が満たす。
type Authenticator interface {
	Authenticate(ctx context.Context, token, resource string) (*auth.Principal, *oauth.Grant, error)
	// Enabled は、管理者が MCP を有効にしているかどうかを返す。
	Enabled(ctx context.Context) (bool, error)
}

var _ Authenticator = new(oauth.Server)

// Handler は、/mcp のハンドラー。
type Handler struct {
	authenticator Authenticator
	// graphql は、GraphQL のハンドラー。ツールは、ログイン中のユーザーとして、ここで操作を実行する。
	graphql http.Handler
	tools   []*registeredTool
	byName  map[string]*registeredTool
	logger  *slog.Logger
	sdk     http.Handler
	// icons は、serverInfo で返すアイコン。
	icons []mcp.Icon
}

type registeredTool struct {
	*tool

	schema map[string]any
}

var _ http.Handler = new(Handler)

// NewHandler は Handler を返す。schema は GraphQL のスキーマで、ツールの入力の JSON Schema を作るのに使う。
func NewHandler(authenticator Authenticator, graphql http.Handler, schema *ast.Schema, logger *slog.Logger) (*Handler, error) {
	icons, err := serverIcons()
	if err != nil {
		return nil, err
	}
	h := &Handler{
		authenticator: authenticator, graphql: graphql, logger: logger,
		tools: make([]*registeredTool, 0), byName: make(map[string]*registeredTool), sdk: nil, icons: icons,
	}
	for _, t := range toolList() {
		registered := &registeredTool{tool: t, schema: map[string]any{keyType: schemaObject, "properties": map[string]any{}, "additionalProperties": false}}
		if t.query != "" {
			s, err := inputSchema(schema, t.query)
			if err != nil {
				return nil, fmt.Errorf("tool %s: %w", t.name, err)
			}
			registered.schema = s
		}
		h.tools = append(h.tools, registered)
		h.byName[t.name] = registered
	}
	h.sdk = mcp.NewStreamableHTTPHandler(h.server, &mcp.StreamableHTTPOptions{
		// 2026-07-28 版にはセッションがないので、どの tenban にリクエストが届いてもよい。
		Stateless: true, JSONResponse: true, Logger: logger, MaxRequestBodyBytes: maxRequestBytes,
	})

	return h, nil
}

type contextKey struct{}

// caller は、/mcp にリクエストを送ってきたユーザーと連携。
type caller struct {
	principal *auth.Principal
	grant     *oauth.Grant
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 無効なときは、MCP サーバーがないものとして振る舞う。
	enabled, err := h.authenticator.Enabled(r.Context())
	if err != nil {
		h.logger.ErrorContext(r.Context(), "failed to load the MCP setting", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)

		return
	}
	if !enabled {
		http.NotFound(w, r)

		return
	}
	origin := requestOrigin(r)
	resourceMetadata := origin + oauth.ProtectedResourceMetadataPath
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || strings.TrimSpace(token) == "" {
		unauthorized(w, resourceMetadata, "")

		return
	}
	principal, grant, err := h.authenticator.Authenticate(r.Context(), strings.TrimSpace(token), oauth.ResourceURL(origin))
	switch {
	case err == nil:
	case errors.Is(err, auth.ErrUnauthenticated), errors.Is(err, oauth.ErrWrongAudience):
		// ほかの MCP サーバー宛てのトークンも 401 にする。403 にすると、クライアントが同じトークンにスコープを足そうとする。
		unauthorized(w, resourceMetadata, "invalid_token")

		return
	default:
		h.logger.ErrorContext(r.Context(), "failed to authenticate the MCP request", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)

		return
	}
	// ツールに要るスコープが足りなければ、SDK に渡す前に 403 で知らせ、クライアントに足りないスコープで認可し直してもらう。
	// ツールを実行するときにも、もう一度確かめる。こちらの読み方と SDK の読み方が食い違っても、すり抜けられないようにする。
	if missing := h.missingScope(w, r, grant); missing != "" {
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer error="insufficient_scope", scope=%q, resource_metadata=%q`, missing, resourceMetadata))
		http.Error(w, "insufficient scope", http.StatusForbidden)

		return
	}
	ctx := context.WithValue(auth.WithPrincipal(r.Context(), principal), contextKey{}, &caller{principal: principal, grant: grant})
	h.sdk.ServeHTTP(w, r.WithContext(ctx))
}

// missingScope は、ツールを呼ぶリクエストで、連携に足りないスコープを返す。足りていれば空を返す。
// 本文は、SDK と同じ jsonrpc.DecodeMessage で読み、SDK のために元に戻す。読めなければ、SDK に断らせる。
func (h *Handler) missingScope(w http.ResponseWriter, r *http.Request, grant *oauth.Grant) string {
	body, err := readBody(w, r)
	if err != nil {
		return ""
	}
	msg, err := jsonrpc.DecodeMessage(body)
	if err != nil {
		return ""
	}
	req, ok := msg.(*jsonrpc.Request)
	if !ok || req.Method != methodToolsCall {
		return ""
	}
	var params mcp.CallToolParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return ""
	}
	t, ok := h.byName[params.Name]
	if !ok || oauth.HasScope(grant.Scopes, t.scope) {
		return ""
	}

	return t.scope
}

// readBody は、本文を読み切り、後ろのハンドラーのために元に戻す。大きすぎる本文は読まない。
func readBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(http.MaxBytesReader(w, r.Body, maxRequestBytes)); err != nil {
		return nil, err
	}
	r.Body = readCloser{Reader: bytes.NewReader(buf.Bytes())}

	return buf.Bytes(), nil
}

type readCloser struct{ *bytes.Reader }

func (readCloser) Close() error { return nil }

// unauthorized は、401 を返す。WWW-Authenticate に、保護リソースのメタデータとスコープを載せる (RFC 9728 5.1)。
func unauthorized(w http.ResponseWriter, resourceMetadata, code string) {
	value := fmt.Sprintf(`Bearer resource_metadata=%q, scope=%q`, resourceMetadata, oauth.ScopeRead+" "+oauth.ScopeWrite)
	if code != "" {
		value = fmt.Sprintf(`Bearer error=%q, resource_metadata=%q`, code, resourceMetadata)
	}
	w.Header().Set("WWW-Authenticate", value)
	http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
}

// requestOrigin は、リクエストが接続したオリジンを返す。
func requestOrigin(r *http.Request) string {
	if client := httpserver.ClientFrom(r.Context()); client != nil {
		return client.Origin()
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	return scheme + "://" + r.Host
}

// server は、リクエストごとに MCP のサーバーを作る。stateless なので、リクエストごとに作ってよい。
func (h *Handler) server(*http.Request) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "kotatsu", Title: "Kotatsu", Version: buildinfo.Get().Version, Icons: h.icons}, &mcp.ServerOptions{
		Instructions: "Kotatsu は、テレビ番組を録画するサーバー。番組を探して予約し、録画のルールを作り、録画を探せる。" +
			"サービスやジャンルを指定するときは、先に list_services と list_genres で ID と番号を確かめる。時刻は RFC 3339 で渡す。",
	})
	for _, t := range h.tools {
		server.AddTool(&mcp.Tool{
			Name: t.name, Title: t.title, Description: t.description + "\n要るスコープ: " + t.scope,
			InputSchema: t.schema,
			Annotations: &mcp.ToolAnnotations{
				Title: t.title, ReadOnlyHint: t.readOnly, DestructiveHint: new(t.destructive), IdempotentHint: t.readOnly,
				OpenWorldHint: new(false),
			},
		}, h.toolHandler(t))
	}

	return server
}

// serverIcons は、serverInfo で返すアイコンを、data URI にして返す。
// 前段に認証のプロキシを置く構成では、Kotatsu の URL のアイコンをクライアントが取れないので、URL にはしない。
// 背景を塗ったアイコンなので、テーマで出し分けない。
func serverIcons() ([]mcp.Icon, error) {
	files := []struct {
		name, mimeType, size string
	}{
		{name: "icon.svg", mimeType: "image/svg+xml", size: "any"},
		// SVG を描けないクライアントのために、PNG も返す。
		{name: "icon-192.png", mimeType: "image/png", size: "192x192"},
	}
	icons := make([]mcp.Icon, 0, len(files))
	for _, f := range files {
		data, err := fs.ReadFile(webui.Icons(), f.name)
		if err != nil {
			return nil, fmt.Errorf("read the icon %s: %w", f.name, err)
		}
		icons = append(icons, mcp.Icon{
			Source:   "data:" + f.mimeType + ";base64," + base64.StdEncoding.EncodeToString(data),
			MIMEType: f.mimeType, Sizes: []string{f.size}, Theme: "",
		})
	}

	return icons, nil
}

// toolHandler は、ツールの処理を返す。
func (h *Handler) toolHandler(t *registeredTool) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		c, _ := ctx.Value(contextKey{}).(*caller)
		if c == nil {
			return nil, errors.New("the MCP request is not authenticated")
		}
		if !oauth.HasScope(c.grant.Scopes, t.scope) {
			return toolError(fmt.Sprintf("this tool requires the %s scope", t.scope)), nil
		}
		var result any
		var err error
		if t.handle != nil {
			result, err = t.handle()
		} else {
			result, err = h.execute(ctx, t.query, req.Params.Arguments)
		}
		if err != nil {
			if toolErr, ok := errors.AsType[*graphqlError](err); ok {
				return toolError(toolErr.Error()), nil
			}

			return nil, err
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("failed to encode the tool result: %w", err)
		}
		if len(encoded) > maxResultBytes {
			return toolError(fmt.Sprintf("the result is too large (%d bytes); narrow the conditions or pass a smaller first", len(encoded))), nil
		}

		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(encoded)}}, StructuredContent: json.RawMessage(encoded)}, nil
	}
}

func toolError(message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: message}}, IsError: true}
}

// graphqlError は、GraphQL が返したエラー。ツールのエラーとして、クライアントに伝える。
type graphqlError struct {
	messages []string
}

func (e *graphqlError) Error() string {
	return strings.Join(e.messages, "; ")
}

// execute は、ログイン中のユーザーとして、GraphQL の操作を実行し、data を返す。
// 画面と同じハンドラーを通すので、入力の確かめ、権限、複雑度の上限、同時に照合する検索の数の上限も、画面と同じに効く。
func (h *Handler) execute(ctx context.Context, query string, arguments json.RawMessage) (any, error) {
	variables := json.RawMessage("{}")
	if len(bytes.TrimSpace(arguments)) > 0 && !bytes.Equal(bytes.TrimSpace(arguments), []byte("null")) {
		variables = arguments
	}
	body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return nil, fmt.Errorf("failed to encode the GraphQL request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/graphql", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to build the GraphQL request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.graphql.ServeHTTP(rec, req)
	var resp struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message    string         `json:"message"`
			Extensions map[string]any `json:"extensions"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("failed to decode the GraphQL response (status %d): %w", rec.Code, err)
	}
	if len(resp.Errors) > 0 {
		messages := make([]string, 0, len(resp.Errors))
		for _, e := range resp.Errors {
			if code, ok := e.Extensions["code"].(string); ok {
				messages = append(messages, code+": "+e.Message)
			} else {
				messages = append(messages, e.Message)
			}
		}

		return nil, &graphqlError{messages: messages}
	}

	return resp.Data, nil
}
