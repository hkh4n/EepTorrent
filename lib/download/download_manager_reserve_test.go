package download

import (
	"github.com/go-i2p/go-i2p-bt/downloader"
	"github.com/go-i2p/go-i2p-bt/metainfo"
	"sync"
	"sync/atomic"
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
	info := metainfo.Info{
		PieceLength: 256 * 1024, // 256 KB
		Pieces:      make(metainfo.Hashes, 4),
	}

	writer := NewMockWriter(info)
	dm := NewDownloadManager(writer, 1024*1024, 256*1024, 4, "/tmp/download") // 1 MB total length
	pieceIndex := uint32(1)
	offset := uint32(0) // Corrected offset within the piece

	var successCount int32
	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if dm.reserveBlock(pieceIndex, offset) {
				// Only one goroutine should succeed
				atomic.AddInt32(&successCount, 1)
			}
		}()
	}

	wg.Wait()
	assert.Equal(t, int32(1), successCount, "Only one goroutine should successfully reserve the block")

	piece := dm.Pieces[pieceIndex]
	piece.Mu.Lock()
	defer piece.Mu.Unlock()
	assert.True(t, piece.Blocks[offset/downloader.BlockSize], "Block should be reserved")
}
