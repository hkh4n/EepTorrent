package peer

import (
	"github.com/stretchr/testify/assert"
	"sync"
	"testing"
)

func TestNewPeerState(t *testing.T) {
	ps := NewPeerState()

	assert.NotNil(t, ps)
	assert.NotNil(t, ps.RequestedBlocks)
	assert.False(t, ps.RequestPending)
	assert.Equal(t, int32(0), ps.PendingRequests)
}

func TestBlockRequestTracking(t *testing.T) {
	ps := NewPeerState()

	// Test marking a block as requested
	pieceIndex := uint32(1)
	offset := uint32(16384) // Standard block size

	// Initially should not be requested
	assert.False(t, ps.IsBlockRequested(pieceIndex, offset))

	// Mark as requested
	ps.MarkBlockRequested(pieceIndex, offset)
	assert.True(t, ps.IsBlockRequested(pieceIndex, offset))

	// Different offset should still be unrequested
	assert.False(t, ps.IsBlockRequested(pieceIndex, offset+16384))

	// Different piece should be unrequested
	assert.False(t, ps.IsBlockRequested(pieceIndex+1, offset))
}

func TestConcurrentBlockRequests(t *testing.T) {
	ps := NewPeerState()
	var wg sync.WaitGroup
	numGoroutines := 100

	// Test concurrent access to block request tracking
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			pieceIndex := uint32(index % 10)
			offset := uint32((index % 5) * 16384)

			// Perform concurrent operations
			ps.MarkBlockRequested(pieceIndex, offset)
			ps.IsBlockRequested(pieceIndex, offset)
		}(i)
	}

	wg.Wait()
	// No race conditions should occur
}

func TestPendingRequestsCounter(t *testing.T) {
	ps := NewPeerState()

	// Test incrementing pending requests
	for i := 0; i < 5; i++ {
		ps.MarkBlockRequested(uint32(i), 0)
		ps.PendingRequests++
	}

	assert.Equal(t, int32(5), ps.PendingRequests)

	// Test decrementing pending requests
	for i := 0; i < 3; i++ {
		ps.PendingRequests--
	}

	assert.Equal(t, int32(2), ps.PendingRequests)
}

func TestMultiplePiecesTracking(t *testing.T) {
	ps := NewPeerState()

	// Request multiple blocks from different pieces
	testCases := []struct {
		pieceIndex uint32
		offset     uint32
	}{
		{0, 0},
		{0, 16384},
		{1, 0},
		{2, 32768},
	}

	for _, tc := range testCases {
		ps.MarkBlockRequested(tc.pieceIndex, tc.offset)
	}

	// Verify all blocks are tracked correctly
	for _, tc := range testCases {
		assert.True(t, ps.IsBlockRequested(tc.pieceIndex, tc.offset))
	}

	// Verify structure of RequestedBlocks map
	assert.Equal(t, 3, len(ps.RequestedBlocks))    // Should have 3 pieces
	assert.Equal(t, 2, len(ps.RequestedBlocks[0])) // Piece 0 should have 2 blocks
	assert.Equal(t, 1, len(ps.RequestedBlocks[1])) // Piece 1 should have 1 block
	assert.Equal(t, 1, len(ps.RequestedBlocks[2])) // Piece 2 should have 1 block
}

func TestRequestPendingFlag(t *testing.T) {
	ps := NewPeerState()

	// Initially should not be pending
	assert.False(t, ps.RequestPending)

	// Simulate request cycle
	ps.RequestPending = true
	assert.True(t, ps.RequestPending)

	ps.RequestPending = false
	assert.False(t, ps.RequestPending)
}

func TestLockingBehavior(t *testing.T) {
	ps := NewPeerState()
	var wg sync.WaitGroup
	numGoroutines := 10

	// Test concurrent access with explicit locking
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			ps.Lock()
			defer ps.Unlock()

			// Modify state under lock
			pieceIndex := uint32(index)
			if ps.RequestedBlocks[pieceIndex] == nil {
				ps.RequestedBlocks[pieceIndex] = make(map[uint32]bool)
			}
			ps.RequestedBlocks[pieceIndex][0] = true
		}(i)
	}

	wg.Wait()
	// Check final state
	assert.Equal(t, numGoroutines, len(ps.RequestedBlocks))
}
