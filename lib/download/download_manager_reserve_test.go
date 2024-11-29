package download

import (
	"github.com/go-i2p/go-i2p-bt/metainfo"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReserveBlock(t *testing.T) {
	// Setup
	writer := NewMockWriter(metainfo.Info{})
	dm := NewDownloadManager(writer, 1024*1024, 256*1024, 4, "/tmp/download")
	pieceIndex := uint32(0)
	offset := uint32(0)

	// Ensure the block is initially not reserved by reserving it
	reserved := dm.reserveBlock(pieceIndex, offset)
	assert.True(t, reserved, "Block should be successfully reserved initially")

	// Attempt to reserve the same block again
	reservedAgain := dm.reserveBlock(pieceIndex, offset)
	assert.False(t, reservedAgain, "Block should already be reserved")
}

func TestReserveBlockConcurrency(t *testing.T) {
	// Setup
	writer := NewMockWriter(metainfo.Info{})
	dm := NewDownloadManager(writer, 1024*1024, 256*1024, 4, "/tmp/download")
	pieceIndex := uint32(1)
	offset := uint32(512 * 1024) // Assume BlockSize is 256KB

	var successCount int
	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if dm.reserveBlock(pieceIndex, offset) {
				// Only one goroutine should succeed
				successCount++
			}
		}()
	}

	wg.Wait()
	assert.Equal(t, 1, successCount, "Only one goroutine should successfully reserve the block")
}
