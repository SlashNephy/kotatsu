package graph

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// cursorVersion は、カーソルの形のバージョン。形を変えたら上げ、古いカーソルを拒否する。
const cursorVersion = 1

var (
	// ErrInvalidCursor は、カーソルが正しくないことを表す。
	ErrInvalidCursor = errors.New("invalid cursor")
	// ErrInvalidID は、Node の ID が正しくないことを表す。
	ErrInvalidID = errors.New("invalid global ID")
)

// EncodeCursor は、一覧の位置を、クライアントが中身を解釈しないカーソル (Relay の Cursor Connections) にする。
// position には、並べ替えのキー (日時と ID など) を JSON にできる形で渡す。
func EncodeCursor(position any) (string, error) {
	b, err := json.Marshal(struct {
		Version  int `json:"v"`
		Position any `json:"p"`
	}{Version: cursorVersion, Position: position})
	if err != nil {
		return "", fmt.Errorf("failed to encode a cursor: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

// DecodeCursor は、EncodeCursor で作ったカーソルを、position に読み込む。
func DecodeCursor(cursor string, position any) error {
	b, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidCursor, err)
	}
	var decoded struct {
		Version  int             `json:"v"`
		Position json.RawMessage `json:"p"`
	}
	if err := json.Unmarshal(b, &decoded); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidCursor, err)
	}
	if decoded.Version != cursorVersion {
		return fmt.Errorf("%w: unsupported version %d", ErrInvalidCursor, decoded.Version)
	}
	if err := json.Unmarshal(decoded.Position, position); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidCursor, err)
	}

	return nil
}

// GlobalID は、型の名前と ID から、Node の ID (Relay の Global Object Identification) を作る。
func GlobalID(typeName, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(typeName + ":" + id))
}

// ParseGlobalID は、GlobalID で作った ID を、型の名前と ID に分ける。
func ParseGlobalID(globalID string) (string, string, error) {
	b, err := base64.RawURLEncoding.DecodeString(globalID)
	if err != nil {
		return "", "", fmt.Errorf("%w: %w", ErrInvalidID, err)
	}
	typeName, id, ok := strings.Cut(string(b), ":")
	if !ok || typeName == "" || id == "" {
		return "", "", fmt.Errorf("%w: missing the type name or the ID", ErrInvalidID)
	}

	return typeName, id, nil
}
