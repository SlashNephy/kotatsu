package captionmodel

// SetCatalog は、テストで、モデルの一覧を手元のサーバーの URL のものに差し替える。
func SetCatalog(w *Worker, models []Model) {
	w.catalog = func() []Model { return models }
}

// AllowedHost は、テストから allowedHost を呼ぶ。
func AllowedHost(scheme, host string) bool { return allowedHost(scheme, host) }
