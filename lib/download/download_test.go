package download

import (
	"github.com/go-i2p/go-i2p-bt/downloader"
	"github.com/go-i2p/go-i2p-bt/metainfo"
	pp "github.com/go-i2p/go-i2p-bt/peerprotocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"os"
	"sync"
	"testing"
)

// MockWriter implements metainfo.Writer interface for testing
type MockWriter struct {
	mock.Mock
	info metainfo.Info
	data []byte
	mu   sync.Mutex
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

func setupTestDownloadManager(t *testing.T) (*DownloadManager, *MockWriter, string) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "download_test_*")
	if err != nil {
		t.Fatal(err)
	}

	// Create piece hashes - we'll create 4 pieces
	numPieces := 4
	pieceHashes := make(metainfo.Hashes, numPieces)

	// Fill with dummy hash data
	for i := 0; i < numPieces; i++ {
		var hash metainfo.Hash
		for j := 0; j < metainfo.HashSize; j++ {
			hash[j] = byte((i*metainfo.HashSize + j) % 256)
		}
		pieceHashes[i] = hash
	}

	// Create test info
	info := metainfo.Info{
		PieceLength: 32 * 1024,  // 32KB pieces
		Length:      128 * 1024, // 128KB total size
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

	err := dm.OnBlock(0, 0, blockData)
	assert.NoError(t, err)
	assert.Equal(t, int64(len(blockData)), dm.Downloaded)

	// Test receiving duplicate block
	err = dm.OnBlock(0, 0, blockData)
	assert.NoError(t, err)

	// Test invalid piece index
	err = dm.OnBlock(999, 0, blockData)
	assert.Error(t, err)
}

func TestPieceVerification(t *testing.T) {
	dm, _, tempDir := setupTestDownloadManager(t)
	defer os.RemoveAll(tempDir)

	// Create test piece data
	pieceSize := dm.Writer.Info().PieceLength
	pieceData := make([]byte, pieceSize)
	for i := range pieceData {
		pieceData[i] = byte(i % 256)
	}

	// Split into blocks
	blockSize := downloader.BlockSize
	numBlocks := (pieceSize + int64(blockSize) - 1) / int64(blockSize)

	// Add blocks to piece
	for i := 0; i < int(numBlocks); i++ {
		start := i * blockSize
		end := start + blockSize
		if end > int(pieceSize) {
			end = int(pieceSize)
		}

		blockData := pieceData[start:end]
		err := dm.OnBlock(0, uint32(start), blockData)
		assert.NoError(t, err)
	}

	// Verify the piece
	assert.True(t, dm.VerifyPiece(0))
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

func TestPeerInteractions(t *testing.T) {
	dm, _, tempDir := setupTestDownloadManager(t)
	defer os.RemoveAll(tempDir)

	// Create mock peer connection
	peerBitfield := pp.NewBitField(4)
	peerBitfield.Set(0)
	peerBitfield.Set(2)

	peer := &pp.PeerConn{
		BitField: peerBitfield,
	}

	// Test if we need pieces from the peer
	assert.True(t, dm.NeedPiecesFrom(peer))

	// Mark piece 0 as complete in our bitfield
	dm.Bitfield.Set(0)

	// We should still need pieces (piece 2)
	assert.True(t, dm.NeedPiecesFrom(peer))

	// Mark piece 2 as complete
	dm.Bitfield.Set(2)

	// Now we shouldn't need any more pieces from this peer
	assert.False(t, dm.NeedPiecesFrom(peer))
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
