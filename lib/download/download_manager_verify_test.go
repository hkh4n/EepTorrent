package download

import (
	"crypto/sha1"
	"github.com/go-i2p/go-i2p-bt/downloader"
	"testing"

	"github.com/go-i2p/go-i2p-bt/metainfo"
	"github.com/stretchr/testify/assert"
)

const BlockSize = downloader.BlockSize

func TestVerifyPiece(t *testing.T) {
	// Setup
	pieceLength := int64(256 * 1024)  // 256KB
	totalLength := int64(1024 * 1024) // 1MB
	numPieces := 4

	// Generate piece hashes
	pieceHashes := make(metainfo.Hashes, numPieces)
	for i := 0; i < numPieces; i++ {
		data := make([]byte, pieceLength)
		for j := range data {
			data[j] = byte(j % 256)
		}
		hash := sha1.Sum(data)
		pieceHashes[i] = metainfo.NewHash(hash[:])
	}

	info := metainfo.Info{
		PieceLength: pieceLength,
		Length:      totalLength,
		Name:        "test_file",
		Pieces:      pieceHashes,
	}

	writer := NewMockWriter(info)
	dm := NewDownloadManager(writer, totalLength, pieceLength, numPieces, "/tmp/download")

	// Simulate receiving all blocks for piece 0
	pieceIndex := uint32(0)
	for j := 0; j < int(pieceLength)/int(BlockSize); j++ {
		offset := uint32(j) * BlockSize
		block := make([]byte, BlockSize)
		for k := range block {
			block[k] = byte(k % 256)
		}
		err := dm.OnBlock(pieceIndex, offset, block)
		assert.NoError(t, err, "OnBlock should not return error for valid block")
	}

	// Verify the piece
	valid := dm.VerifyPiece(pieceIndex)
	assert.True(t, valid, "Piece verification should succeed for correct data")

	// Introduce corruption in one block
	peerIndex := 0
	offset := uint32(peerIndex) * BlockSize
	corruptedBlock := []byte("corrupted data")
	err := dm.OnBlock(pieceIndex, offset, corruptedBlock)
	assert.NoError(t, err, "OnBlock should not return error even if data is corrupted")

	// Verify the piece again
	valid = dm.VerifyPiece(pieceIndex)
	assert.False(t, valid, "Piece verification should fail due to corrupted block")
}
