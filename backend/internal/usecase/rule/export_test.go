package rule

// SetMatchBatchSize は、照合の対象の番組をまとめて照合する件数を変える。
func SetMatchBatchSize(e *Engine, n int) {
	e.batchSize = n
}
