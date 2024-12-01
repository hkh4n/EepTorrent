package download

import (
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTrackUpload(t *testing.T) {
	dm, _, tempDir := setupTestDownloadManager(t)
	defer os.RemoveAll(tempDir)

	initialUploaded := atomic.LoadInt64(&dm.Uploaded)
	initialSessionUploaded := atomic.LoadInt64(&dm.UploadedThisSession)
	initialLastUpload := dm.LastUploadTime

	time.Sleep(10 * time.Millisecond) // Ensure timestamp difference

	dm.TrackUpload(1024)

	assert.Equal(t, initialUploaded+1024, atomic.LoadInt64(&dm.Uploaded), "Uploaded bytes should be incremented by 1024")
	assert.Equal(t, initialSessionUploaded+1024, atomic.LoadInt64(&dm.UploadedThisSession), "UploadedThisSession should be incremented by 1024")
	assert.True(t, dm.LastUploadTime.After(initialLastUpload), "LastUploadTime should be updated")
}
