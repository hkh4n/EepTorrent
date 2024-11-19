package download

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
	"fmt"
	"github.com/go-i2p/go-i2p-bt/downloader"
	"github.com/go-i2p/go-i2p-bt/metainfo"
	pp "github.com/go-i2p/go-i2p-bt/peerprotocol"
	"github.com/sirupsen/logrus"
	"sync"
	"sync/atomic"
)

var log = logrus.New()

type PieceStatus struct {
	Index       uint32
	TotalBlocks uint32
	Blocks      []bool // true if block is received
	Completed   bool
	Mu          sync.Mutex
}

type DownloadManager struct {
	Writer        metainfo.Writer
	Pieces        []*PieceStatus
	Bitfield      pp.BitField
	Downloaded    int64
	Uploaded      int64
	Left          int64
	Mu            sync.Mutex
	CurrentPiece  uint32
	CurrentOffset uint32
}

func NewDownloadManager(writer metainfo.Writer, totalLength int64, pieceLength int64, totalPieces int) *DownloadManager {
	pieces := make([]*PieceStatus, totalPieces)
	for i := 0; i < totalPieces; i++ {
		// Calculate number of blocks per piece
		remainingData := totalLength - int64(i)*pieceLength
		var blocks uint32
		if remainingData < pieceLength {
			blocks = uint32((remainingData + downloader.BlockSize - 1) / downloader.BlockSize)
		} else {
			blocks = uint32(pieceLength / downloader.BlockSize)
		}
		pieces[i] = &PieceStatus{
			Index:       uint32(i),
			TotalBlocks: blocks,
			Blocks:      make([]bool, blocks),
			Completed:   false,
		}
	}
	return &DownloadManager{
		Writer:   writer,
		Pieces:   pieces,
		Bitfield: pp.NewBitField(totalPieces),
		Left:     totalLength,
	}
}

// IsFinished checks if the download is complete
func (dm *DownloadManager) IsFinished() bool {
	totalPieces := dm.Writer.Info().CountPieces()
	completedPieces := 0

	for i := 0; i < totalPieces; i++ {
		if dm.Bitfield.IsSet(uint32(i)) {
			completedPieces++
		} else {
			// Early exit if any piece is not yet downloaded
			return false
		}
	}

	return completedPieces == totalPieces
}

func (dm *DownloadManager) OnBlock(index, offset uint32, b []byte) error {
	// Validate piece index
	if int(index) >= len(dm.Pieces) {
		log.WithFields(logrus.Fields{
			"piece_index":  index,
			"total_pieces": len(dm.Pieces),
		}).Error("Received block for invalid piece index")
		return fmt.Errorf("invalid piece index: %d", index)
	}

	piece := dm.Pieces[index]

	piece.Mu.Lock()
	defer piece.Mu.Unlock()

	// Calculate block number based on offset
	blockNum := offset / downloader.BlockSize
	if blockNum >= piece.TotalBlocks {
		log.WithFields(logrus.Fields{
			"block_num":    blockNum,
			"total_blocks": piece.TotalBlocks,
		}).Error("Received block with invalid offset")
		return fmt.Errorf("invalid block offset: %d", offset)
	}

	// Check if block is already received
	if piece.Blocks[blockNum] {
		log.WithFields(logrus.Fields{
			"piece_index": index,
			"block_num":   blockNum,
		}).Warn("Received duplicate block")
		return nil // Ignore duplicate
	}

	// Write the block
	n, err := dm.Writer.WriteBlock(index, offset, b)
	if err != nil {
		log.WithError(err).Error("Failed to write block")
		return err
	}

	// Update download progress
	atomic.AddInt64(&dm.Downloaded, int64(n))
	atomic.AddInt64(&dm.Left, -int64(n))

	// Mark block as received
	piece.Blocks[blockNum] = true

	// Check if piece is completed
	if !piece.Completed && dm.isPieceComplete(piece) {
		dm.Bitfield.Set(index)
		log.WithFields(logrus.Fields{
			"piece_index":  index,
			"total_pieces": dm.Writer.Info().CountPieces(),
			"progress":     fmt.Sprintf("%.2f%%", float64(index+1)/float64(dm.Writer.Info().CountPieces())*100),
		}).Info("Completed piece")
	}

	log.WithFields(logrus.Fields{
		"piece_index":   index,
		"block_num":     blockNum,
		"bytes_written": n,
	}).Debug("Successfully processed block")
	return nil
}

// isPieceComplete checks if all blocks in a piece are received
func (dm *DownloadManager) isPieceComplete(piece *PieceStatus) bool {
	for _, received := range piece.Blocks {
		if !received {
			return false
		}
	}
	piece.Completed = true
	return true
}

func (dm *DownloadManager) NeedPiecesFrom(pc *pp.PeerConn) bool {
	log := log.WithField("peer", pc.RemoteAddr().String())

	dm.Mu.Lock()
	defer dm.Mu.Unlock()

	for i := 0; i < len(dm.Bitfield); i++ { //dm.Bitfield.Length() -> len(dm.Bitfield)
		if !dm.Bitfield.IsSet(uint32(i)) && pc.BitField.IsSet(uint32(i)) {
			log.WithFields(logrus.Fields{
				"piece_index":    i,
				"have_piece":     dm.Bitfield.IsSet(uint32(i)),
				"peer_has_piece": pc.BitField.IsSet(uint32(i)),
			}).Debug("Found needed piece from peer")
			return true
		}
	}
	log.Debug("No needed pieces from this peer")
	return false
}

// LogProgress logs the current download progress
func (dm *DownloadManager) LogProgress() {
	progress := dm.Progress()
	downloaded := atomic.LoadInt64(&dm.Downloaded)
	left := atomic.LoadInt64(&dm.Left)
	log.WithFields(logrus.Fields{
		"progress":         fmt.Sprintf("%.2f%%", progress),
		"downloaded_bytes": downloaded,
		"total_bytes":      dm.Writer.Info().TotalLength(),
		"remaining_bytes":  left,
	}).Info("Download progress update")
}

// Progress calculates the current download progress percentage
func (dm *DownloadManager) Progress() float64 {
	totalPieces := dm.Writer.Info().CountPieces()
	completedPieces := 0
	for i := 0; i < totalPieces; i++ {
		if dm.Bitfield.IsSet(uint32(i)) {
			completedPieces++
		}
	}
	return (float64(completedPieces) / float64(totalPieces)) * 100
}

// GetNextBlock retrieves the next block to request from a peer
func (dm *DownloadManager) GetNextBlock() (uint32, uint32, error) {
	dm.Mu.Lock()
	defer dm.Mu.Unlock()

	for i := 0; i < len(dm.Pieces); i++ {
		piece := dm.Pieces[i]
		piece.Mu.Lock()
		if !dm.Bitfield.IsSet(piece.Index) && !piece.Completed {
			for j, received := range piece.Blocks {
				if !received {
					offset := j * downloader.BlockSize
					piece.Mu.Unlock()
					return piece.Index, uint32(offset), nil
				}
			}
		}
		piece.Mu.Unlock()
	}

	return 0, 0, fmt.Errorf("no blocks to request")
}

// PieceInfo holds information about a specific piece
type PieceInfo struct {
	Index       uint32
	PieceLength int32
}

// GetPiece retrieves information about a specific piece
func (dm *DownloadManager) GetPiece(index uint32) (*PieceInfo, error) {
	dm.Mu.Lock()
	defer dm.Mu.Unlock()

	if int(index) >= len(dm.Pieces) {
		return nil, fmt.Errorf("invalid piece index: %d", index)
	}

	piece := dm.Pieces[index]

	piece.Mu.Lock()
	defer piece.Mu.Unlock()

	if piece.Completed {
		return nil, fmt.Errorf("piece %d already completed", index)
	}

	return &PieceInfo{
		Index:       piece.Index,
		PieceLength: int32(dm.Writer.Info().PieceLength),
	}, nil
}
