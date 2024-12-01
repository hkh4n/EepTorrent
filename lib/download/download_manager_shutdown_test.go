// manager_shutdown_test.go
package download

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDownloadManagerShutdown(t *testing.T) {
	dm, _, tempDir := setupTestDownloadManager(t)
	defer os.RemoveAll(tempDir)

	// Start a dummy goroutine to simulate ongoing work
	dm.wg.Add(1)
	go func() {
		defer dm.wg.Done()
		select {
		case <-dm.ctx.Done():
			return
		case <-time.After(5 * time.Second):
			return
		}
	}()

	// Shutdown should cancel the context and wait for the goroutine
	start := time.Now()
	dm.Shutdown()
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 1*time.Second, "Shutdown should return promptly")
}
