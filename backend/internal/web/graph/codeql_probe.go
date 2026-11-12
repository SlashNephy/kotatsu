package graph

// CodeQL が gqlgen のリゾルバーの引数を外からの入力として追えるかを確かめるための、わざと脆弱なコード。
// マージしない。

import (
	"context"
	"os"
	"os/exec"
)

func codeQLProbeGraphQL(ctx context.Context, query string) {
	_, _ = exec.CommandContext(ctx, "sh", "-c", query).Output()
	_, _ = os.ReadFile(query)
}
