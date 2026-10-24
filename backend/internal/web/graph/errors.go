package graph

import (
	"context"
	"errors"
	"log/slog"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/domain/match"
	"github.com/SlashNephy/kotatsu/internal/lib/password"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/reserve"
	"github.com/SlashNephy/kotatsu/internal/usecase/rule"
)

const (
	// extensionCode は、エラーコードを入れる extensions のキー。
	extensionCode = "code"
	// extensionReason は、パスワードの規定に当たった理由を入れる extensions のキー。
	extensionReason = "reason"
)

// Code は、エラーの extensions.code に入れるエラーコード (設計書 9.1)。クライアントはこれで処理を分ける。
type Code string

const (
	// CodeUnauthenticated は、ログインしていないこと。
	CodeUnauthenticated Code = "UNAUTHENTICATED"
	// CodeReauthenticationRequired は、重要な操作の前に、もう一度認証する必要があること (設計書 9.5)。
	CodeReauthenticationRequired Code = "REAUTHENTICATION_REQUIRED"
	// CodeForbidden は、ロールに許されていない操作であること。
	CodeForbidden Code = "FORBIDDEN"
	// CodeNotFound は、対象がないこと。
	CodeNotFound Code = "NOT_FOUND"
	// CodeBadUserInput は、入力が正しくないこと。
	CodeBadUserInput Code = "BAD_USER_INPUT"
	// CodeConflict は、対象の今の状態では、その操作ができないこと (保護した録画を消すなど)。
	CodeConflict Code = "CONFLICT"
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

		switch {
		case errors.Is(err, auth.ErrReauthenticationRequired):
			gqlErr.Message = "reauthentication required"
			gqlErr.Extensions = map[string]any{extensionCode: CodeReauthenticationRequired}

			return gqlErr
		case errors.Is(err, auth.ErrUnauthenticated):
			gqlErr.Message = "login required"
			gqlErr.Extensions = map[string]any{extensionCode: CodeUnauthenticated}

			return gqlErr
		}
		// パスワードの規定に当たったときは、画面が理由を示せるよう、理由を extensions.reason に入れる。
		if rejected, ok := errors.AsType[*password.RejectedError](err); ok {
			gqlErr.Message = "the password was rejected"
			gqlErr.Extensions = map[string]any{extensionCode: CodeBadUserInput, extensionReason: rejected.Reason}

			return gqlErr
		}
		if e, ok := errors.AsType[*Error](err); ok || domainError(err, &e) {
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

// domainError は、ドメインのパッケージが返すエラーを、エラーコードを持つエラーにする。当てはまらなければ false を返す。
// メッセージは、ドメインのエラーのものをそのまま使う。入力のどこが正しくないかを伝えるためである。
func domainError(err error, target **Error) bool {
	var code Code
	switch {
	case errors.Is(err, rule.ErrNotFound), errors.Is(err, domain.ErrReservationNotFound), errors.Is(err, reserve.ErrProgramNotFound),
		errors.Is(err, auth.ErrUserNotFound), errors.Is(err, auth.ErrSessionNotFound):
		code = CodeNotFound
	case errors.Is(err, rule.ErrInvalidRule), errors.Is(err, domain.ErrInvalidReservation),
		errors.Is(err, reserve.ErrInvalidSettings), errors.Is(err, match.ErrInvalidConditions),
		errors.Is(err, auth.ErrInvalidUser), errors.Is(err, auth.ErrUsernameTaken), errors.Is(err, auth.ErrInvalidRole),
		errors.Is(err, auth.ErrLastAdmin), errors.Is(err, auth.ErrCannotDisableSelf):
		code = CodeBadUserInput
	default:
		return false
	}
	// gqlgen は、リゾルバのエラーを、位置とパスを付けた *gqlerror.Error に包む。メッセージには、元のエラーの文だけを使う。
	message := err.Error()
	if wrapped, ok := errors.AsType[*gqlerror.Error](err); ok && wrapped.Err != nil {
		message = wrapped.Err.Error()
	}
	*target = NewError(code, message)

	return true
}
