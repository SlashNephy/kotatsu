package googledrive

import (
	"context"
	"time"
)

// SetSleep は、やり直す前に待つ処理を差し替える。テストで待たないようにする。
func SetSleep(c *Client, sleep func(ctx context.Context, d time.Duration) error) {
	c.sleep = sleep
}
