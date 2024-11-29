package download

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"github.com/go-i2p/go-i2p-bt/downloader"
	"github.com/go-i2p/go-i2p-bt/metainfo"
	pp "github.com/go-i2p/go-i2p-bt/peerprotocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"io"
	"net"
	"os"
	"sync"
	"testing"
)

// MockWriter implements metainfo.Writer interface for testing
type MockWriter struct {
	mock.Mock
	info    metainfo.Info
	data    []byte
	mu      sync.Mutex
	written map[uint32]map[uint32][]byte
}

func NewMockWriter(info metainfo.Info) *MockWriter {
	return &MockWriter{
		info: info,
		data: make([]byte, info.Length),
	}
}

func (m *MockWriter) Write(piece int, data []byte) error {
	args := m.Called(piece, data)
	return args.Error(0)
}

func (m *MockWriter) WriteAt(p []byte, off int64) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if off < 0 || off > int64(len(m.data)) {
		return 0, os.ErrInvalid
	}

	n = copy(m.data[off:], p)
	args := m.Called(p, off)
	return n, args.Error(1)
}

func (m *MockWriter) WriteBlock(pieceIndex uint32, pieceOffset uint32, p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	offset := int64(pieceIndex)*int64(m.info.PieceLength) + int64(pieceOffset)
	if offset < 0 || offset > int64(len(m.data)) {
		return 0, os.ErrInvalid
	}

	n := copy(m.data[offset:], p)
	args := m.Called(pieceIndex, pieceOffset, p)
	return n, args.Error(1)
}

func (m *MockWriter) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockWriter) Info() metainfo.Info {
	return m.info
}

// blockKey generates a unique key for a block based on pieceIndex and offset.
func (mw *MockWriter) blockKey(pieceIndex, offset uint32) string {
	return fmt.Sprintf("piece_%d_offset_%d", pieceIndex, offset)
}

// ReadBlock simulates reading data from a block.
// Returns data if the block has been written; otherwise, returns an error.
// ReadBlock reads data from a specific block identified by pieceIndex and pieceOffset.
func (mw *MockWriter) ReadBlock(pieceIndex, pieceOffset uint32) ([]byte, error) {
	mw.mu.Lock()
	defer mw.mu.Unlock()

	data, exists := mw.written[pieceIndex][pieceOffset]
	if !exists {
		return nil, errors.New("block not found")
	}
	return data, nil
}

// ReadAt implements the io.ReaderAt interface.
func (mw *MockWriter) ReadAt(p []byte, offset int64) (n int, err error) {
	mw.mu.Lock()
	defer mw.mu.Unlock()

	pieceLength := int64(mw.info.PieceLength)
	pieceIndex := uint32(offset / pieceLength)
	pieceOffset := uint32(offset % pieceLength)

	data, exists := mw.written[pieceIndex][pieceOffset]
	if !exists {
		return 0, errors.New("block not found")
	}

	copyLen := len(p)
	if len(data) < copyLen {
		copyLen = len(data)
	}
	copy(p, data[:copyLen])

	if copyLen < len(p) {
		return copyLen, io.EOF
	}
	return copyLen, nil
}

// TotalWrittenBlocks returns the total number of written blocks.
func (mw *MockWriter) TotalWrittenBlocks() int {
	mw.mu.Lock()
	defer mw.mu.Unlock()

	total := 0
	for _, blocks := range mw.written {
		total += len(blocks)
	}
	return total
}

func setupTestDownloadManager(t *testing.T) (*DownloadManager, *MockWriter, string) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "download_test_*")
	if err != nil {
		t.Fatal(err)
	}

	numPieces := 4
	pieceLength := int64(32 * 1024)  // 32KB pieces
	totalLength := int64(128 * 1024) // 128KB total

	// Pre-calculate the actual piece hashes that will match our test data
	pieceHashes := make(metainfo.Hashes, numPieces)
	for pieceIndex := 0; pieceIndex < numPieces; pieceIndex++ {
		// Create the same piece data that the test will write
		pieceData := make([]byte, pieceLength)
		for offset := 0; offset < int(pieceLength); offset += downloader.BlockSize {
			remainingBytes := int(pieceLength) - offset
			currentBlockSize := downloader.BlockSize
			if remainingBytes < downloader.BlockSize {
				currentBlockSize = remainingBytes
			}

			// Fill block with same pattern as test
			for i := 0; i < currentBlockSize; i++ {
				pieceData[offset+i] = byte(i % 256)
			}
		}

		// Calculate SHA1 hash of the piece data
		hash := sha1.Sum(pieceData)
		pieceHashes[pieceIndex] = metainfo.NewHash(hash[:])
	}

	// Create test info
	info := metainfo.Info{
		PieceLength: pieceLength,
		Length:      totalLength,
		Name:        "test_file",
		Pieces:      pieceHashes,
	}

	// Create mock writer
	mockWriter := &MockWriter{info: info}
	mockWriter.On("Write", mock.Anything, mock.Anything).Return(nil)
	mockWriter.On("Close").Return(nil)

	// Create download manager
	dm := NewDownloadManager(mockWriter, info.Length, info.PieceLength, numPieces, tempDir)

	return dm, mockWriter, tempDir
}

func TestNewDownloadManager(t *testing.T) {
	dm, _, tempDir := setupTestDownloadManager(t)
	defer os.RemoveAll(tempDir)

	assert.NotNil(t, dm)
	assert.Equal(t, int64(128*1024), dm.Left)
	assert.Equal(t, 4, len(dm.Pieces))
	assert.Equal(t, tempDir, dm.DownloadDir)
}

func TestOnBlock(t *testing.T) {
	dm, _, tempDir := setupTestDownloadManager(t)
	defer os.RemoveAll(tempDir)

	// Test receiving a valid block
	blockData := make([]byte, downloader.BlockSize)
	for i := range blockData {
		blockData[i] = byte(i % 256)
	}

	t.Run("Valid block", func(t *testing.T) {
		err := dm.OnBlock(0, 0, blockData)
		assert.NoError(t, err)
		assert.Equal(t, int64(len(blockData)), dm.Downloaded)
	})

	t.Run("Duplicate block", func(t *testing.T) {
		err := dm.OnBlock(0, 0, blockData)
		assert.NoError(t, err) // Duplicate blocks should be ignored gracefully
	})

	t.Run("Invalid piece index - too large", func(t *testing.T) {
		err := dm.OnBlock(999, 0, blockData)
		assert.Error(t, err)
	})

	t.Run("Invalid piece index - at boundary", func(t *testing.T) {
		// Test the last valid piece index
		lastValidIndex := uint32(dm.TotalPieces - 1)
		err := dm.OnBlock(lastValidIndex, 0, blockData)
		assert.NoError(t, err, "Last valid piece index should work")

		// Test the first invalid piece index
		firstInvalidIndex := uint32(dm.TotalPieces)
		err = dm.OnBlock(firstInvalidIndex, 0, blockData)
		assert.Error(t, err, "First invalid piece index should fail")
	})

	t.Run("Invalid block number", func(t *testing.T) {
		invalidOffset := uint32(dm.Writer.Info().PieceLength)
		err := dm.OnBlock(0, invalidOffset, blockData)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid block number")
	})

	t.Run("Nil block data", func(t *testing.T) {
		err := dm.OnBlock(0, 0, nil)
		assert.Error(t, err)
	})

	t.Run("Block too large", func(t *testing.T) {
		largeBlock := make([]byte, dm.Writer.Info().PieceLength+1)
		err := dm.OnBlock(0, 0, largeBlock)
		assert.Error(t, err)
	})
}

func TestPieceVerification(t *testing.T) {
	dm, _, tempDir := setupTestDownloadManager(t)
	defer os.RemoveAll(tempDir)

	t.Run("Complete valid piece", func(t *testing.T) {
		// Fill up a piece with blocks
		pieceIndex := uint32(0)
		blockSize := uint32(downloader.BlockSize)
		pieceLength := dm.Writer.Info().PieceLength

		for offset := uint32(0); offset < uint32(pieceLength); offset += blockSize {
			remainingBytes := uint32(pieceLength) - offset
			currentBlockSize := blockSize
			if remainingBytes < blockSize {
				currentBlockSize = remainingBytes
			}

			blockData := make([]byte, currentBlockSize)
			for i := range blockData {
				blockData[i] = byte(i % 256)
			}

			err := dm.OnBlock(pieceIndex, offset, blockData)
			assert.NoError(t, err)
		}

		// Verify the piece
		piece := dm.Pieces[pieceIndex]
		piece.Mu.Lock()
		isComplete := dm.isPieceComplete(piece)
		piece.Mu.Unlock()

		assert.True(t, isComplete)
	})

	t.Run("Incomplete piece", func(t *testing.T) {
		pieceIndex := uint32(1)

		// Add just one block, leaving the piece incomplete
		blockData := make([]byte, downloader.BlockSize)
		err := dm.OnBlock(pieceIndex, 0, blockData)
		assert.NoError(t, err)

		piece := dm.Pieces[pieceIndex]
		piece.Mu.Lock()
		isComplete := dm.isPieceComplete(piece)
		piece.Mu.Unlock()

		assert.False(t, isComplete)
	})
}

func TestProgress(t *testing.T) {
	dm, _, tempDir := setupTestDownloadManager(t)
	defer os.RemoveAll(tempDir)

	// Initially 0%
	assert.Equal(t, float64(0), dm.Progress())

	// Download half the pieces
	pieceSize := dm.Writer.Info().PieceLength
	halfData := make([]byte, pieceSize)
	err := dm.OnBlock(0, 0, halfData)
	assert.NoError(t, err)

	// Should be 25% (1 out of 4 pieces)
	expectedProgress := float64(pieceSize) / float64(dm.Writer.Info().Length) * 100
	assert.InEpsilon(t, expectedProgress, dm.Progress(), 0.1)
}

func TestConcurrentAccess(t *testing.T) {
	dm, _, tempDir := setupTestDownloadManager(t)
	defer os.RemoveAll(tempDir)

	var wg sync.WaitGroup
	numGoroutines := 10

	// Simulate multiple goroutines accessing the download manager
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			// Perform various concurrent operations
			dm.Progress()
			dm.IsFinished()
			dm.GetPieceLength(0)

			// Add some data
			blockData := make([]byte, 1024)
			dm.OnBlock(uint32(index%4), 0, blockData)
		}(i)
	}

	wg.Wait()
	// No race conditions should occur
}

type MockConn struct {
	net.Conn
	addr net.Addr
}

func (m *MockConn) RemoteAddr() net.Addr {
	return m.addr
}

func TestPeerInteractions(t *testing.T) {
	dm, _, tempDir := setupTestDownloadManager(t)
	defer os.RemoveAll(tempDir)

	// Calculate the correct number of bytes needed (should round up)
	numBytes := (4 + 7) / 8 // (bits + 7) / 8 rounds up to nearest byte

	// Create mock peer connection with correct size bitfield
	peerBitfield := pp.NewBitField(numBytes * 8) // Size in bits
	peerBitfield.Set(0)
	peerBitfield.Set(2)

	mockAddr := &net.TCPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 6881,
	}

	mockConn := &MockConn{
		addr: mockAddr,
	}

	peer := &pp.PeerConn{
		BitField: peerBitfield,
		Conn:     mockConn,
	}

	// Initial state verification
	t.Logf("Initial state:")
	for i := 0; i < dm.TotalPieces; i++ {
		t.Logf("Piece %d - We have: %v, Peer has: %v",
			i, dm.Bitfield.IsSet(uint32(i)), peer.BitField.IsSet(uint32(i)))
	}

	// Initial check
	result := dm.NeedPiecesFrom(peer)
	t.Logf("Initial check (should be true): %v", result)
	assert.True(t, result, "Should need pieces from peer (has pieces 0,2, we have none)")

	// Mark piece 0 as complete
	dm.Bitfield.Set(0)
	t.Logf("\nAfter setting piece 0:")
	for i := 0; i < dm.TotalPieces; i++ {
		t.Logf("Piece %d - We have: %v, Peer has: %v",
			i, dm.Bitfield.IsSet(uint32(i)), peer.BitField.IsSet(uint32(i)))
	}

	result = dm.NeedPiecesFrom(peer)
	t.Logf("After setting piece 0 (should be true): %v", result)
	assert.True(t, result, "Should still need piece 2 from peer")

	// Mark piece 2 as complete
	dm.Bitfield.Set(2)
	t.Logf("\nAfter setting piece 2:")
	for i := 0; i < dm.TotalPieces; i++ {
		t.Logf("Piece %d - We have: %v, Peer has: %v",
			i, dm.Bitfield.IsSet(uint32(i)), peer.BitField.IsSet(uint32(i)))
	}

	result = dm.NeedPiecesFrom(peer)
	t.Logf("After setting piece 2 (should be false): %v", result)
	assert.False(t, result, "Should not need any more pieces from peer")
}
func TestIsFinished(t *testing.T) {
	dm, _, tempDir := setupTestDownloadManager(t)
	defer os.RemoveAll(tempDir)

	// Initially not finished
	assert.False(t, dm.IsFinished())

	// Mark all pieces as complete
	for i := 0; i < dm.TotalPieces; i++ {
		dm.Bitfield.Set(uint32(i))
	}

	// Should now be finished
	assert.True(t, dm.IsFinished())
}
