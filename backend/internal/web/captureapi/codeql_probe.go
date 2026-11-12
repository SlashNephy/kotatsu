package captureapi

// 対照: net/http のハンドラーから同じシンクに渡す。こちらは CodeQL が検出するはず。マージしない。

import (
	"net/http"
	"os"
	"os/exec"
)

func CodeQLProbeHTTP(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("probe")
	_, _ = exec.CommandContext(r.Context(), "sh", "-c", query).Output()
	_, _ = os.ReadFile(query)
}
