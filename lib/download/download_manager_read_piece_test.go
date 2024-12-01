// download_manager_read_piece_test.go
package download

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadPiece(t *testing.T) {
	dm, _, tempDir := setupTestDownloadManager(t)
	defer os.RemoveAll(tempDir)

	pieceIndex := uint32(0)
	pieceLength := dm.Writer.Info().PieceLength

	// Prepare piece data and write to disk
	pieceData := make([]byte, pieceLength)
	for i := range pieceData {
		pieceData[i] = byte(i % 256)
	}

	filePath := filepath.Join(tempDir, dm.Writer.Info().Name)
	err := ioutil.WriteFile(filePath, pieceData, 0644)
	assert.NoError(t, err)

	// Read the piece
	readData, err := dm.ReadPiece(pieceIndex)
	assert.NoError(t, err)
	assert.Equal(t, pieceData, readData, "Read data should match written data")
}

func TestReadPiece_InvalidIndex(t *testing.T) {
	dm, _, tempDir := setupTestDownloadManager(t)
	defer os.RemoveAll(tempDir)

	invalidIndex := uint32(dm.TotalPieces) // Out of bounds
	_, err := dm.ReadPiece(invalidIndex)
	assert.Error(t, err, "Should return error for invalid piece index")
}
