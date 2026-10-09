package graph

import (
	"context"
	"errors"
	"log/slog"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// extensionCode は、エラーコードを入れる extensions のキー。
const extensionCode = "code"

// Code は、エラーの extensions.code に入れるエラーコード (設計書 9.1)。クライアントはこれで処理を分ける。
type Code string

const (
	// CodeUnauthenticated は、ログインしていないこと。
	CodeUnauthenticated Code = "UNAUTHENTICATED"
	// CodeForbidden は、ロールに許されていない操作であること。
	CodeForbidden Code = "FORBIDDEN"
	// CodeNotFound は、対象がないこと。
	CodeNotFound Code = "NOT_FOUND"
	// CodeBadUserInput は、入力が正しくないこと。
	CodeBadUserInput Code = "BAD_USER_INPUT"
	// CodeQueryTooDeep は、クエリの深さが上限を超えていること。
	// 複雑度が上限を超えたときは、gqlgen の COMPLEXITY_LIMIT_EXCEEDED を返す。
	CodeQueryTooDeep Code = "DEPTH_LIMIT_EXCEEDED"
	// CodeInternal は、サーバーの内部のエラー。詳細はログに出し、クライアントには返さない。
	CodeInternal Code = "INTERNAL"
)

var _ error = new(Error)

// Error は、エラーコードを持つエラー。リゾルバはこれを返す。
type Error struct {
	Code    Code
	Message string
}

// NewError はエラーコードを持つエラーを作る。
func NewError(code Code, message string) *Error {
	return &Error{Code: code, Message: message}
}

func (e *Error) Error() string {
	return e.Message
}

// errorPresenter は、エラーにエラーコードを付ける。Error でないエラーは内部のエラーとして扱い、詳細を隠す。
func errorPresenter(logger *slog.Logger) graphql.ErrorPresenterFunc {
	return func(ctx context.Context, err error) *gqlerror.Error {
		gqlErr := graphql.DefaultErrorPresenter(ctx, err)
		if gqlErr.Extensions != nil && gqlErr.Extensions[extensionCode] != nil {
			// 構文や検証のエラーなど、gqlgen がコードを付けたもの (GRAPHQL_PARSE_FAILED、GRAPHQL_VALIDATION_FAILED など)。
			return gqlErr
		}

		if e, ok := errors.AsType[*Error](err); ok {
			gqlErr.Message = e.Message
			gqlErr.Extensions = map[string]any{extensionCode: e.Code}

			return gqlErr
		}

		logger.ErrorContext(ctx, "GraphQL resolver failed", slog.Any("error", err), slog.Any("path", gqlErr.Path))
		gqlErr.Message = "internal error"
		gqlErr.Extensions = map[string]any{extensionCode: CodeInternal}

		return gqlErr
	}
}

// recoverFunc は、リゾルバの panic を内部のエラーにする。
func recoverFunc(logger *slog.Logger) graphql.RecoverFunc {
	return func(ctx context.Context, p any) error {
		logger.ErrorContext(ctx, "GraphQL resolver panicked", slog.Any("panic", p))

		return errors.New("panic in a resolver")
	}
}
