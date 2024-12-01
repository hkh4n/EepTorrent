package peer

/*
A cross-platform I2P-only BitTorrent client.
Copyright (C) 2024 Haris Khan

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPeerState(t *testing.T) {
	ps := NewPeerState()

	// Test initial state
	assert.False(t, ps.IsBlockRequested(1, 0), "Block should not be requested initially")

	// Mark a block as requested
	ps.MarkBlockRequested(1, 0)
	assert.True(t, ps.IsBlockRequested(1, 0), "Block should be marked as requested")

	// Mark another block
	ps.MarkBlockRequested(2, 16384)
	assert.True(t, ps.IsBlockRequested(2, 16384), "Another block should be marked as requested")

	// Test concurrent access
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(piece, offset uint32) {
			defer wg.Done()
			ps.MarkBlockRequested(piece, offset)
			assert.True(t, ps.IsBlockRequested(piece, offset), "Block should be marked as requested")
		}(uint32(i%5), uint32(i*4096))
	}
	wg.Wait()

	// Verify total requested blocks
	expected := 102
	actual := 0
	for _, blocks := range ps.RequestedBlocks {
		actual += len(blocks)
	}
	assert.Equal(t, expected, actual, "Total requested blocks should match")
}
