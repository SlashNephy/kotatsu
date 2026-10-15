package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
)

// maxSecondFactorAttempts は、1 回のパスワードの確認で、第二要素の確認に失敗してよい回数 (付録 A)。
// 超えたら、パスワードの確認からやり直させる。第二要素の失敗も、パスワードの失敗と同じく、
// アカウント単位と IP 単位の試行回数の制限に数えるので、やり直しても第二要素のコードを総当たりで当てられない。
const maxSecondFactorAttempts = 5

var (
	// ErrLoginChallengeInvalid は、パスワードの確認を経ていない、期限が切れた、失敗が多すぎた、のどれかを表す。
	ErrLoginChallengeInvalid = errors.New("the password must be verified again")
	// ErrSecondFactorEnrolled は、第二要素を登録済みなので、ログインの途中で登録し直せないことを表す。
	ErrSecondFactorEnrolled = errors.New("a second factor is already enrolled")
)

// LoginResult は、第二要素まで確かめたログインの結果。Token はセッションの Cookie に入れる。
type LoginResult struct {
	Token   string
	Session *Session
	// RecoveryCodes は、ログインの途中で TOTP を登録したときのリカバリーコード。ここでしか見せない。
	RecoveryCodes []string
}

// throttledChallenge は、challenge で待っているログインを引き、そのユーザーとアドレスの試行回数の制限の
// キーを返す。待ち時間の中なら ThrottledError を返す。
func (l *Logins) throttledChallenge(ctx context.Context, tx pgx.Tx, token string, client *ClientInfo) (*sqlc.LoginChallenge, *loginAttempt, error) {
	row, err := l.challenge(ctx, tx, token)
	if err != nil {
		return nil, nil, err
	}
	// 同じユーザーの第二要素の確認を、待っているログインをまたいで 1 つずつにする。
	// 同時に送った試行が、どれも前の失敗を見ずに待ち時間を素通りしないようにするためである。
	user, err := sqlc.New(tx).LockUser(ctx, row.UserID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to lock the user: %w", err)
	}
	attempt := &loginAttempt{username: user.Username, keys: throttleKeys(user.Username, client.SourceIP), now: l.clock.Now()}
	if err := checkThrottleIn(ctx, tx, attempt.keys, attempt.now); err != nil {
		return nil, nil, err
	}

	return row, attempt, nil
}

// loginAttempt は、第二要素の確認の試行。失敗を数えるキーを持つ。
type loginAttempt struct {
	username string
	keys     []string
	now      time.Time
}

// challenge は、トークンの指す、第二要素の確認を待つログインを引き、ロックする。
func (l *Logins) challenge(ctx context.Context, tx pgx.Tx, token string) (*sqlc.LoginChallenge, error) {
	row, err := sqlc.New(tx).GetLoginChallengeForUpdate(ctx, sqlc.GetLoginChallengeForUpdateParams{
		TokenHash: hashToken(token), Now: l.clock.Now(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrLoginChallengeInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the login challenge: %w", err)
	}

	return &row, nil
}

// SecondFactorMethods は、待っているログインのユーザーが使える第二要素を返す。空なら、TOTP を登録させる。
func (l *Logins) SecondFactorMethods(ctx context.Context, db sqlc.DBTX, challenge *Challenge) ([]SecondFactor, error) {
	return l.factors.Methods(ctx, db, challenge.UserID)
}

// CompleteWithTOTP は、TOTP のコードを確かめて、ログインを完了させ、セッションを作る。
func (l *Logins) CompleteWithTOTP(ctx context.Context, db Database, token, code string, client *ClientInfo) (*LoginResult, error) {
	return l.complete(ctx, db, token, SecondFactorTOTP, client, func(tx pgx.Tx, row *sqlc.LoginChallenge) error {
		return l.factors.VerifyTOTP(ctx, tx, row.UserID, code)
	})
}

// CompleteWithRecoveryCode は、リカバリーコードを確かめて、ログインを完了させ、セッションを作る。
func (l *Logins) CompleteWithRecoveryCode(ctx context.Context, db Database, token, code string, client *ClientInfo) (*LoginResult, error) {
	return l.complete(ctx, db, token, SecondFactorRecoveryCode, client, func(tx pgx.Tx, row *sqlc.LoginChallenge) error {
		return l.factors.UseRecoveryCode(ctx, tx, row.UserID, code)
	})
}

// complete は、verify で第二要素を確かめる。失敗したら回数を数え、上限に達したら、待っているログインを消す。
func (l *Logins) complete(
	ctx context.Context,
	db Database,
	token string,
	method SecondFactor,
	client *ClientInfo,
	verify func(tx pgx.Tx, row *sqlc.LoginChallenge) error,
) (*LoginResult, error) {
	var result *LoginResult
	var verifyErr error
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		row, attempt, err := l.throttledChallenge(ctx, tx, token, client)
		if err != nil {
			return err
		}
		verifyErr = verify(tx, row)
		if errors.Is(verifyErr, ErrInvalidSecondFactor) {
			// 失敗は、ロールバックせずに数えて残す。
			return l.recordSecondFactorFailure(ctx, tx, row, attempt, method, client)
		}
		if verifyErr != nil {
			return verifyErr
		}
		result, err = l.finish(ctx, tx, row, attempt, method, client)

		return err
	})
	if err != nil {
		return nil, err
	}
	if verifyErr != nil {
		return nil, verifyErr
	}

	return result, nil
}

// recordSecondFactorFailure は、第二要素の失敗を、待っているログインと、アカウント単位と IP 単位の試行回数の制限に数える。
func (l *Logins) recordSecondFactorFailure(
	ctx context.Context,
	tx pgx.Tx,
	row *sqlc.LoginChallenge,
	attempt *loginAttempt,
	method SecondFactor,
	client *ClientInfo,
) error {
	q := sqlc.New(tx)
	failures, err := q.IncrementLoginChallengeFailures(ctx, row.ID)
	if err != nil {
		return fmt.Errorf("failed to count the second factor failure: %w", err)
	}
	if failures >= maxSecondFactorAttempts {
		if err := q.DeleteLoginChallenge(ctx, row.ID); err != nil {
			return fmt.Errorf("failed to delete the login challenge: %w", err)
		}
	}

	return l.recordFailure(ctx, tx, attempt.keys, &row.UserID, attempt.username, client, attempt.now, actionLoginFailed, string(method))
}

// finish は、待っているログインを消し、アカウント単位の失敗の回数を消して、セッションを作る。
func (l *Logins) finish(
	ctx context.Context,
	tx pgx.Tx,
	row *sqlc.LoginChallenge,
	attempt *loginAttempt,
	method SecondFactor,
	client *ClientInfo,
) (*LoginResult, error) {
	q := sqlc.New(tx)
	if err := q.DeleteLoginChallenge(ctx, row.ID); err != nil {
		return nil, fmt.Errorf("failed to delete the login challenge: %w", err)
	}
	if err := q.ResetLoginThrottle(ctx, attempt.keys[0]); err != nil {
		return nil, fmt.Errorf("failed to reset the login throttle: %w", err)
	}

	return l.startSession(ctx, tx, row.UserID, method, client)
}

// startSession は、セッションを作り、ログインを監査ログに残す。
func (l *Logins) startSession(ctx context.Context, tx pgx.Tx, userID uuid.UUID, method SecondFactor, client *ClientInfo) (*LoginResult, error) {
	return startSession(ctx, tx, l.sessions, l.recorder, userID, method, client)
}

// startSession は、sessions でセッションを作り、method でのログインを recorder の監査ログに残す。
func startSession(
	ctx context.Context,
	tx pgx.Tx,
	sessions *Sessions,
	recorder audit.Recorder,
	userID uuid.UUID,
	method SecondFactor,
	client *ClientInfo,
) (*LoginResult, error) {
	token, session, err := sessions.Create(ctx, tx, userID, client)
	if err != nil {
		return nil, err
	}
	err = recorder.Record(ctx, tx, &audit.Event{
		Actor:      audit.User(userID),
		Action:     "user.login",
		TargetType: loginAuditTarget,
		TargetID:   userID.String(),
		Summary:    map[string]any{summaryMethod: string(method), "sessionId": session.ID.String()},
		SourceIP:   client.SourceIP,
		UserAgent:  client.UserAgent,
	})
	if err != nil {
		return nil, err
	}

	return &LoginResult{Token: token, Session: session, RecoveryCodes: []string{}}, nil
}

// BeginEnrollment は、第二要素を 1 つも登録していないユーザーに、ログインの途中で TOTP の登録を始めさせる (設計書 9.3)。
// パスワードの確認は済んでいる。第二要素を登録済みなら、ErrSecondFactorEnrolled を返す。
func (l *Logins) BeginEnrollment(ctx context.Context, db Database, token string) (*TOTPEnrollment, error) {
	var enrollment *TOTPEnrollment
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		row, err := l.challenge(ctx, tx, token)
		if err != nil {
			return err
		}
		user, err := sqlc.New(tx).LockUser(ctx, row.UserID)
		if err != nil {
			return fmt.Errorf("failed to lock the user: %w", err)
		}
		if err := l.requireNoSecondFactor(ctx, tx, row); err != nil {
			return err
		}
		enrollment, err = l.factors.BeginTOTP(ctx, tx, row.UserID, user.Username)

		return err
	})
	if err != nil {
		return nil, err
	}

	return enrollment, nil
}

// CompleteEnrollment は、ログインの途中で登録を始めた TOTP のコードを確かめて有効にし、ログインを完了させる。
// リカバリーコードは、ここでしか見せない。
func (l *Logins) CompleteEnrollment(ctx context.Context, db Database, token, code string, client *ClientInfo) (*LoginResult, error) {
	var result *LoginResult
	var verifyErr error
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		row, attempt, err := l.throttledChallenge(ctx, tx, token, client)
		if err != nil {
			return err
		}
		// 同じユーザーの登録を同時に完了させない。
		if _, err := sqlc.New(tx).LockUser(ctx, row.UserID); err != nil {
			return fmt.Errorf("failed to lock the user: %w", err)
		}
		if err := l.requireNoSecondFactor(ctx, tx, row); err != nil {
			return err
		}
		codes, err := l.factors.ConfirmTOTP(ctx, tx, audit.UserFrom(row.UserID, client.SourceIP, client.UserAgent), row.UserID, code)
		verifyErr = err
		if errors.Is(err, ErrInvalidSecondFactor) {
			return l.recordSecondFactorFailure(ctx, tx, row, attempt, SecondFactorTOTP, client)
		}
		if err != nil {
			return err
		}
		result, err = l.finish(ctx, tx, row, attempt, SecondFactorTOTP, client)
		if err != nil {
			return err
		}
		result.RecoveryCodes = codes

		return nil
	})
	if err != nil {
		return nil, err
	}
	if verifyErr != nil {
		return nil, verifyErr
	}

	return result, nil
}

func (l *Logins) requireNoSecondFactor(ctx context.Context, tx pgx.Tx, row *sqlc.LoginChallenge) error {
	methods, err := l.factors.Methods(ctx, tx, row.UserID)
	if err != nil {
		return err
	}
	// どの種類でも、第二要素があれば、パスワードを知っているだけで登録させない。
	if len(methods) > 0 {
		return ErrSecondFactorEnrolled
	}

	return nil
}
