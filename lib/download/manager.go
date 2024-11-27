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
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"github.com/go-i2p/go-i2p-bt/downloader"
	"github.com/go-i2p/go-i2p-bt/metainfo"
	pp "github.com/go-i2p/go-i2p-bt/peerprotocol"
	"github.com/sirupsen/logrus"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

var log = logrus.StandardLogger()

type PieceStatus struct {
	Index       uint32
	TotalBlocks uint32
	Blocks      []bool   // true if block is received
	BlockData   [][]byte // Store block data until piece complete
	Completed   bool
	Mu          sync.Mutex
}

type DownloadManager struct {
	Writer              metainfo.Writer
	Pieces              []*PieceStatus
	Bitfield            pp.BitField
	Downloaded          int64
	Uploaded            int64
	Left                int64
	Mu                  sync.Mutex
	CurrentPiece        uint32
	CurrentOffset       uint32
	RequestedBlocks     map[uint32]map[uint32]bool // piece index -> offset -> requested
	Peers               []*pp.PeerConn
	DownloadDir         string
	TotalPieces         int
	UploadedThisSession int64
	LastUploadTime      time.Time
	ctx                 context.Context
	cancelFunc          context.CancelFunc
	wg                  sync.WaitGroup
}

// BlockInfo represents a specific block within a piece.
type BlockInfo struct {
	PieceIndex uint32
	Offset     uint32
	Length     uint32
}

func NewDownloadManager(writer metainfo.Writer, totalLength int64, pieceLength int64, totalPieces int, downloadDir string) *DownloadManager {
	ctx, cancel := context.WithCancel(context.Background())
	log.WithFields(logrus.Fields{
		"total_length": totalLength,
		"piece_length": pieceLength,
		"total_pieces": totalPieces,
	}).Debug("Initializing DownloadManager")

	pieces := make([]*PieceStatus, totalPieces)
	for i := 0; i < totalPieces; i++ {
		// Calculate number of blocks per piece
		remainingData := totalLength - int64(i)*pieceLength
		var pieceSize int64
		if remainingData < pieceLength {
			pieceSize = remainingData
		} else {
			pieceSize = pieceLength
		}
		blocks := uint32((pieceSize + int64(downloader.BlockSize) - 1) / int64(downloader.BlockSize))
		pieces[i] = &PieceStatus{
			Index:       uint32(i),
			TotalBlocks: blocks,
			Blocks:      make([]bool, blocks),
			Completed:   false,
		}
	}
	return &DownloadManager{
		Writer:          writer,
		Pieces:          pieces,
		Bitfield:        pp.NewBitField(totalPieces),
		TotalPieces:     totalPieces, // <-- Initialize the field
		Downloaded:      0,
		Uploaded:        0,
		Left:            totalLength,
		CurrentPiece:    0,
		CurrentOffset:   0,
		RequestedBlocks: make(map[uint32]map[uint32]bool), // Initialize RequestedBlocks
		Peers:           make([]*pp.PeerConn, 0),          // Initialize Peers
		DownloadDir:     downloadDir,
		ctx:             ctx,
		cancelFunc:      cancel,
	}
}

// Shutdown gracefully shuts down the DownloadManager.
func (dm *DownloadManager) Shutdown() {
	dm.cancelFunc()
	dm.wg.Wait()
}

// Add method to track uploads
func (dm *DownloadManager) TrackUpload(bytes int64) {
	atomic.AddInt64(&dm.Uploaded, bytes)
	atomic.AddInt64(&dm.UploadedThisSession, bytes)
	dm.LastUploadTime = time.Now()
}

func (dm *DownloadManager) IsPieceComplete(pieceIndex uint32) bool {
	if int(pieceIndex) >= len(dm.Pieces) {
		return false
	}

	piece := dm.Pieces[pieceIndex]
	piece.Mu.Lock()
	defer piece.Mu.Unlock()

	for _, blockReceived := range piece.Blocks {
		if !blockReceived {
			return false
		}
	}
	return true
}

// reserveBlock atomically reserves a block for download
func (dm *DownloadManager) reserveBlock(pieceIndex uint32, offset uint32) bool {
	piece := dm.Pieces[pieceIndex]
	piece.Mu.Lock()
	defer piece.Mu.Unlock()

	blockIndex := offset / downloader.BlockSize
	if blockIndex >= uint32(len(piece.Blocks)) {
		return false
	}

	if piece.Blocks[blockIndex] {
		return false // Block already received
	}

	// Mark block as reserved
	piece.Blocks[blockIndex] = true
	return true
}

// validatePieceSize ensures the piece length is correct
func (dm *DownloadManager) validatePieceSize(index uint32, length int64) bool {
	info := dm.Writer.Info()
	if int(index) == info.CountPieces()-1 {
		// Last piece may be shorter
		remaining := info.TotalLength() - int64(index)*info.PieceLength
		return length <= remaining
	}
	return length == info.PieceLength
}

// IsFinished checks if the download is complete
func (dm *DownloadManager) IsFinished() bool {
	log.Debug("Checking if download is finished")
	totalPieces := dm.TotalPieces
	completedPieces := 0

	for i := 0; i < totalPieces; i++ {
		if dm.Bitfield.IsSet(uint32(i)) {
			completedPieces++
		} else {
			// Early exit if any piece is not yet downloaded
			return false
		}
	}

	log.WithFields(logrus.Fields{
		"completed_pieces": completedPieces,
		"total_pieces":     totalPieces,
	}).Debug("Download completion status")

	return completedPieces == totalPieces
}

// OnBlock handles the reception of a block from a peer.
func (dm *DownloadManager) OnBlock(index, offset uint32, b []byte) error {
	dm.Mu.Lock()
	defer dm.Mu.Unlock()

	log.WithFields(logrus.Fields{
		"piece_index":  index,
		"offset":       offset,
		"block_length": len(b),
	}).Debug("Received block")
	// Early check: If the piece is already completed, ignore the block.
	if dm.Bitfield.IsSet(index) {
		log.WithFields(logrus.Fields{
			"piece_index": index,
			"offset":      offset,
		}).Debug("Received block for already completed piece, ignoring")
		return nil
	}

	// Validate piece index.
	if int(index) >= len(dm.Pieces) {
		log.WithFields(logrus.Fields{
			"piece_index":  index,
			"total_pieces": len(dm.Pieces),
		}).Error("Received block for invalid piece index")
		return fmt.Errorf("invalid piece index: %d", index)
	}

	piece := dm.Pieces[index]

	// Acquire piece-specific lock.
	piece.Mu.Lock()
	defer piece.Mu.Unlock()

	log.WithFields(logrus.Fields{
		"index":  index,
		"offset": offset,
		"length": len(b),
	}).Debug("OnBlock called")

	// Validate block size
	info := dm.Writer.Info()
	// Get actual piece length
	pieceLength := dm.GetPieceLength(index)

	// Calculate expected block size
	expectedBlockSize := downloader.BlockSize
	if int64(offset)+int64(expectedBlockSize) > pieceLength {
		expectedBlockSize = int(pieceLength - int64(offset))
	}

	// For small pieces, adjust expected block size
	if pieceLength < int64(expectedBlockSize) {
		expectedBlockSize = int(pieceLength)
	}

	if len(b) != expectedBlockSize {
		log.WithFields(logrus.Fields{
			"received_size": len(b),
			"expected_size": expectedBlockSize,
			"piece_length":  pieceLength,
			"offset":        offset,
		}).Error("Invalid block size")
		return fmt.Errorf("invalid block size: got %d, expected %d", len(b), expectedBlockSize)
	}
	// Calculate block number based on offset.
	blockNum := offset / downloader.BlockSize
	if blockNum >= piece.TotalBlocks {
		log.WithFields(logrus.Fields{
			"piece_index":  index,
			"offset":       offset,
			"block_num":    blockNum,
			"total_blocks": piece.TotalBlocks,
		}).Error("Invalid block number")
		return fmt.Errorf("invalid block offset: %d", offset)
	}

	// Check if block is already received.
	if piece.Blocks[blockNum] {
		log.WithFields(logrus.Fields{
			"piece_index": index,
			"block_num":   blockNum,
		}).Warn("Received duplicate block")
		return nil // Ignore duplicate.
	}

	// Initialize BlockData if needed
	if piece.BlockData == nil {
		piece.BlockData = make([][]byte, piece.TotalBlocks)
	}

	// Store block data in memory and verify it
	blockData := make([]byte, len(b))
	copy(blockData, b)
	piece.BlockData[blockNum] = blockData

	// Log the block data
	log.WithFields(logrus.Fields{
		"piece_index": index,
		"block_num":   blockNum,
		"block_data":  fmt.Sprintf("%x", blockData),
	}).Debug("Stored block data")

	// Write the block to disk.
	n, err := dm.Writer.WriteBlock(index, offset, b)
	if err != nil {
		log.WithError(err).Error("Failed to write block")
		return err
	}

	// Update download progress.
	atomic.AddInt64(&dm.Downloaded, int64(n))
	atomic.AddInt64(&dm.Left, -int64(n))

	log.WithFields(logrus.Fields{
		"downloaded": atomic.LoadInt64(&dm.Downloaded),
		"left":       atomic.LoadInt64(&dm.Left),
	}).Debug("Updated download progress")

	// Mark block as received.
	piece.Blocks[blockNum] = true

	// Check if the piece is completed.
	if !piece.Completed && dm.isPieceComplete(piece) {
		log.WithField("piece_index", index).Debug("Piece is complete, verifying hash")
		// Double-check piece completion before verification
		allBlocksPresent := true
		for _, received := range piece.Blocks {
			if !received {
				allBlocksPresent = false
				break
			}
		}

		if !allBlocksPresent {
			log.WithField("piece_index", index).Warn("Piece appeared complete but some blocks missing")
			return nil
		}
		// Double-check all blocks are present
		for i, blockData := range piece.BlockData {
			if blockData == nil {
				log.WithFields(logrus.Fields{
					"piece_index": index,
					"block_index": i,
				}).Error("Block data is nil during verification")
				return fmt.Errorf("missing block data at index %d for piece %d", i, index)
			}
		}
		// Combine all blocks and verify the complete piece
		completeData := make([]byte, 0, info.PieceLength)
		for _, blockData := range piece.BlockData {
			completeData = append(completeData, blockData...)
		}

		// Log the complete piece data
		log.WithFields(logrus.Fields{
			"piece_index":   index,
			"complete_data": fmt.Sprintf("%x", completeData),
			"data_length":   len(completeData),
		}).Debug("Assembled complete piece data")

		rawExpectedHash := info.Pieces[index]
		expectedHash, err := hex.DecodeString(rawExpectedHash.String())
		if err != nil {
			log.WithError(err).Panic("Failed to decode expected hash")
		}
		actualHash := sha1.Sum(completeData)

		log.WithFields(logrus.Fields{
			"piece_index":   index,
			"expected_hash": fmt.Sprintf("%x", expectedHash),
			"actual_hash":   fmt.Sprintf("%x", actualHash),
		}).Debug("Computed hashes for verification")

		if bytes.Equal(expectedHash[:], actualHash[:]) {
			// Verified successfully
			dm.Bitfield.Set(index)
			piece.Completed = true

			// Clear block data to free memory
			piece.BlockData = nil

			log.WithFields(logrus.Fields{
				"piece_index":  index,
				"total_pieces": len(dm.Pieces),
				"progress":     fmt.Sprintf("%.2f%%", float64(index+1)/float64(len(dm.Pieces))*100),
			}).Info("Completed piece")

			// Advertise the newly completed piece to all connected peers.
			for _, peerConn := range dm.Peers {
				err := peerConn.SendHave(index)
				if err != nil {
					log.WithFields(logrus.Fields{
						"peer":        peerConn.RemoteAddr().String(),
						"piece_index": index,
					}).WithError(err).Error("Failed to send Have message to peer")
				} else {
					log.WithFields(logrus.Fields{
						"peer":        peerConn.RemoteAddr().String(),
						"piece_index": index,
					}).Info("Sent Have message to peer")
				}
			}
		} else {
			log.WithFields(logrus.Fields{
				"piece_index":   index,
				"expected_hash": fmt.Sprintf("%x", expectedHash),
				"actual_hash":   fmt.Sprintf("%x", actualHash),
			}).Error("Piece hash mismatch during verification")

			// Reset piece state
			for i := range piece.Blocks {
				piece.Blocks[i] = false
			}
			piece.BlockData = nil

			// Reset progress counters for this piece
			pieceLength := int64(len(completeData))
			atomic.AddInt64(&dm.Downloaded, -pieceLength)
			atomic.AddInt64(&dm.Left, pieceLength)

			return fmt.Errorf("piece verification failed for index %d", index)
		}
	}

	// Handle endgame scenarios.
	if dm.IsEndgame() && atomic.LoadInt64(&dm.Left) <= 0 {
		log.Info("Endgame completed, finalizing download")
		dm.RequestAllRemainingBlocks(dm.GetAllPeers())
		if dm.IsFinished() {
			err := dm.FinalizeDownload()
			if err != nil {
				log.WithError(err).Error("Failed to finalize download")
				return err
			}
		}
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
	log.WithField("piece_index", piece.Index).Debug("Checking if piece is complete")
	for _, received := range piece.Blocks {
		if !received {
			return false
		}
	}
	log.WithField("piece_index", piece.Index).Debug("Piece is complete")
	return true
}

func (dm *DownloadManager) NeedPiecesFrom(pc *pp.PeerConn) bool {
	dm.Mu.Lock()
	defer dm.Mu.Unlock()
	log := log.WithField("peer", pc.RemoteAddr().String())
	log.Debug("Checking if we need pieces from this peer")

	peerPieces := len(pc.BitField)
	totalPieces := len(dm.Bitfield)
	log.WithFields(logrus.Fields{
		"peerPieces":  peerPieces,
		"totalPieces": totalPieces,
	}).Debug("Piece details")

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
	/*
		totalPieces := dm.Writer.Info().CountPieces()
		completedPieces := 0
		for i := 0; i < totalPieces; i++ {
			if dm.Bitfield.IsSet(uint32(i)) {
				completedPieces++
			}
		}
		progress := (float64(completedPieces) / float64(totalPieces)) * 100
		log.WithFields(logrus.Fields{
			"completed_pieces": completedPieces,
			"total_pieces":     totalPieces,
			"progress":         fmt.Sprintf("%.2f%%", progress),
		}).Debug("Progress calculated")
		return progress

	*/
	total := float64(dm.Writer.Info().TotalLength())
	downloaded := float64(atomic.LoadInt64(&dm.Downloaded))
	return (downloaded / total) * 100
}

// GetNextBlock retrieves the next block to request from a peer
func (dm *DownloadManager) GetNextBlock() (uint32, uint32, error) {
	dm.Mu.Lock()
	defer dm.Mu.Unlock()
	log.Debug("Getting next block to request")

	for i := 0; i < len(dm.Pieces); i++ {
		piece := dm.Pieces[i]
		piece.Mu.Lock()
		if !dm.Bitfield.IsSet(piece.Index) && !piece.Completed {
			for j, received := range piece.Blocks {
				if !received {
					offset := j * downloader.BlockSize
					piece.Mu.Unlock()
					log.WithFields(logrus.Fields{
						"piece_index": piece.Index,
						"offset":      offset,
					}).Debug("Found next block to request")
					return piece.Index, uint32(offset), nil
				}
			}
		}
		piece.Mu.Unlock()
	}
	log.Debug("No blocks to request")
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
	log.WithField("piece_index", index).Debug("Retrieving piece information")

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

const EndgameThreshold = 10

func (dm *DownloadManager) IsEndgame() bool {
	remainingBlocks := atomic.LoadInt64(&dm.Left) / int64(downloader.BlockSize)
	isEndgame := remainingBlocks <= EndgameThreshold
	log.WithField("is_endgame", isEndgame).Debug("Endgame status")
	return isEndgame
}

func (dm *DownloadManager) GetAllRemainingBlocks() []BlockInfo {
	var blocks []BlockInfo
	dm.Mu.Lock()
	defer dm.Mu.Unlock()
	log.Debug("Getting all remaining blocks")

	for _, piece := range dm.Pieces {
		if piece.Completed {
			continue
		}
		piece.Mu.Lock()
		for i, received := range piece.Blocks {
			if !received {
				offset := uint32(i) * downloader.BlockSize
				length := downloader.BlockSize

				// Adjust length for the last block in the piece
				pieceLength := dm.Writer.Info().PieceLength
				if (offset + uint32(length)) > uint32(pieceLength) { // Will converting cause issues?
					length = int(pieceLength - int64(offset))
				}

				blocks = append(blocks, BlockInfo{
					PieceIndex: piece.Index,
					Offset:     offset,
					Length:     uint32(length),
				})
				log.WithFields(logrus.Fields{
					"piece_index": piece.Index,
					"offset":      offset,
					"length":      length,
				}).Debug("Added remaining block")
			}
		}
		piece.Mu.Unlock()
	}
	log.WithField("total_remaining_blocks", len(blocks)).Debug("Collected remaining blocks")
	return blocks
}

func (dm *DownloadManager) RequestAllRemainingBlocks(peers []*pp.PeerConn) {
	log.WithField("num_peers", len(peers)).Debug("Requesting all remaining blocks from peers")
	remainingBlocks := dm.GetAllRemainingBlocks()
	for _, peer := range peers {
		if peer.PeerChoked {
			log.WithField("peer", peer.RemoteAddr().String()).Debug("Peer is choked, skipping")
			continue
		}
		for _, block := range remainingBlocks {
			// Check if the peer has the piece
			if peer.BitField.IsSet(block.PieceIndex) {
				err := peer.SendRequest(block.PieceIndex, block.Offset, block.Length)
				if err != nil {
					log.WithFields(logrus.Fields{
						"peer":        peer.RemoteAddr().String(),
						"piece_index": block.PieceIndex,
						"offset":      block.Offset,
					}).WithError(err).Error("Failed to send endgame request")
					continue
				}
				log.WithFields(logrus.Fields{
					"peer":        peer.RemoteAddr().String(),
					"piece_index": block.PieceIndex,
					"offset":      block.Offset,
				}).Debug("Sent endgame block request")
			}
		}
	}
}
func (dm *DownloadManager) IsBlockRequested(pieceIndex, offset uint32) bool {
	dm.Mu.Lock()
	defer dm.Mu.Unlock()
	log.WithFields(logrus.Fields{
		"piece_index": pieceIndex,
		"offset":      offset,
	}).Debug("Checking if block is already requested")
	if blocks, exists := dm.RequestedBlocks[pieceIndex]; exists {
		return blocks[offset]
	}
	return false
}

func (dm *DownloadManager) MarkBlockRequested(pieceIndex, offset uint32) {
	dm.Mu.Lock()
	defer dm.Mu.Unlock()
	log.WithFields(logrus.Fields{
		"piece_index": pieceIndex,
		"offset":      offset,
	}).Debug("Marking block as requested")
	if dm.RequestedBlocks[pieceIndex] == nil {
		dm.RequestedBlocks[pieceIndex] = make(map[uint32]bool)
	}
	dm.RequestedBlocks[pieceIndex][offset] = true
}

func (dm *DownloadManager) AddPeer(peer *pp.PeerConn) {
	dm.Mu.Lock()
	defer dm.Mu.Unlock()
	log.WithField("peer", peer.RemoteAddr().String()).Debug("Adding peer to DownloadManager")
	dm.Peers = append(dm.Peers, peer)
}

func (dm *DownloadManager) RemovePeer(peer *pp.PeerConn) {
	dm.Mu.Lock()
	defer dm.Mu.Unlock()
	log.WithField("peer", peer.RemoteAddr().String()).Debug("Removing peer from DownloadManager")
	for i, p := range dm.Peers {
		if p == peer {
			dm.Peers = append(dm.Peers[:i], dm.Peers[i+1:]...)
			break
		}
	}
}

func (dm *DownloadManager) GetAllPeers() []*pp.PeerConn {
	dm.Mu.Lock()
	defer dm.Mu.Unlock()
	log.Debug("Retrieving all peers from DownloadManager")
	return dm.Peers
}

func (dm *DownloadManager) FinalizeDownload() error {
	log.Info("Finalizing download")
	// Verify each piece
	for _, piece := range dm.Pieces {
		if !dm.VerifyPiece(piece.Index) { // Implement VerifyPiece
			log.Errorf("Piece %d verification failed", piece.Index)
			return fmt.Errorf("piece %d verification failed", piece.Index)
		}
	}

	// Close writer and perform cleanup
	err := dm.Writer.Close()
	if err != nil {
		log.Errorf("Failed to close writer: %v\n", err)
		return fmt.Errorf("failed to close writer: %v", err)
	}

	log.Info("Download completed and verified successfully")
	return nil
}

// VerifyPiece verifies the integrity of a downloaded piece.
func (dm *DownloadManager) VerifyPiece(index uint32) bool {
	log.WithField("piece_index", index).Debug("Verifying piece")

	info := dm.Writer.Info()
	totalPieces := info.CountPieces()

	// Validate the piece index
	if int(index) >= totalPieces {
		log.WithFields(logrus.Fields{
			"piece_index":  index,
			"total_pieces": totalPieces,
		}).Error("Invalid piece index for verification")
		return false
	}

	piece := dm.Pieces[index]
	piece.Mu.Lock()
	defer piece.Mu.Unlock()

	// If we have block data in memory, verify from that instead of disk
	if piece.BlockData != nil {
		completeData := make([]byte, 0, info.PieceLength)
		for _, blockData := range piece.BlockData {
			completeData = append(completeData, blockData...)
		}

		expectedHash := info.Pieces[index]

		fmt.Printf("%s\n", expectedHash.String())
		fmt.Printf("%s\n", expectedHash.BytesString())
		fmt.Printf("%s\n", expectedHash.HexString())
		decodedExpectedHash, err := hex.DecodeString(expectedHash.String())
		if err != nil {
			log.WithError(err).Error("Failed to decode expected hash")
			return false
		}

		actualHash := sha1.Sum(completeData)

		if !bytes.Equal(expectedHash[:], actualHash[:]) {
			log.WithFields(logrus.Fields{
				"piece_index":         index,
				"decodedExpectedHash": fmt.Sprintf("%x", decodedExpectedHash),
				"expected_hash":       fmt.Sprintf("%x", expectedHash),
				"actual_hash":         fmt.Sprintf("%x", actualHash),
			}).Panic("Piece hash mismatch (memory verification)")
			return false
		}
		log.Info("===\nFOUND CORRECT HASH!\n===")
		return true
	}

	// Fall back to disk verification if no in-memory data
	pieceData, err := dm.ReadPiece(index)
	if err != nil {
		log.WithError(err).Error("Failed to read piece for verification")
		return false
	}

	expectedHash := info.Pieces[index]
	actualHash := sha1.Sum(pieceData)

	if !bytes.Equal(expectedHash[:], actualHash[:]) {
		log.WithFields(logrus.Fields{
			"piece_index":   index,
			"expected_hash": fmt.Sprintf("%x", expectedHash),
			"actual_hash":   fmt.Sprintf("%x", actualHash),
		}).Panic("Piece hash mismatch (disk verification)")
		return false
	}

	return true
}

// ReadPiece reads the data of a specific piece from disk.
func (dm *DownloadManager) ReadPiece(index uint32) ([]byte, error) {
	log.WithField("piece_index", index).Debug("Reading piece from disk")
	info := dm.Writer.Info()
	pieceLength := info.PieceLength
	totalPieces := info.CountPieces()

	if int(index) >= totalPieces {
		return nil, fmt.Errorf("invalid piece index: %d", index)
	}

	// Determine the actual length of the piece (handle the last piece)
	var actualLength int64
	if int(index) == totalPieces-1 {
		remaining := info.TotalLength() - int64(index)*pieceLength
		if remaining < pieceLength {
			actualLength = remaining
		} else {
			actualLength = pieceLength
		}
	} else {
		actualLength = pieceLength
	}

	pieceData := make([]byte, actualLength)

	log.WithFields(logrus.Fields{
		"piece_index":  index,
		"piece_size":   actualLength,
		"download_dir": dm.DownloadDir,
		"multi_file":   len(info.Files) > 0,
	}).Debug("Reading piece data")

	if len(info.Files) == 0 {
		// Single-file torrent
		filePath := filepath.Join(dm.DownloadDir, info.Name)
		log.WithField("file_path", filePath).Warn("Attempting to stat file") // Add this line
		// Check if the path is a directory
		fi, err := os.Stat(filePath)
		if err != nil {
			log.WithFields(logrus.Fields{
				"file_path": filePath,
				"error":     err,
			}).Error("Failed to stat file")
			return nil, fmt.Errorf("failed to stat file %s: %v", filePath, err)
		}
		if fi.IsDir() {
			return nil, fmt.Errorf("expected file at %s but found a directory", filePath)
		}

		file, err := os.Open(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to open file %s: %v", filePath, err)
		}
		defer file.Close()

		_, err = file.ReadAt(pieceData, int64(index)*pieceLength)
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("failed to read piece %d: %v", index, err)
		}
	} else {
		// Multi-file torrent
		var currentOffset int64 = 0
		var bytesRead int64 = 0
		for _, fileInfo := range info.Files {
			filePath := filepath.Join(dm.DownloadDir, fileInfo.Path(info))
			fileSize := fileInfo.Length

			// Check if the piece starts within this file
			if currentOffset+fileSize <= int64(index)*pieceLength {
				currentOffset += fileSize
				continue
			}

			file, err := os.Open(filePath)
			if err != nil {
				return nil, fmt.Errorf("failed to open file %s: %v", filePath, err)
			}

			defer file.Close()

			// Calculate the start position within the file
			pieceStart := int64(index)*pieceLength - currentOffset
			if pieceStart < 0 {
				pieceStart = 0
			}

			// Calculate how much to read from this file
			remaining := actualLength - bytesRead
			toRead := pieceStart + remaining
			if toRead > fileSize {
				toRead = fileSize
			}

			readBytes, err := file.ReadAt(pieceData[bytesRead:], pieceStart)
			if err != nil && err != io.EOF {
				return nil, fmt.Errorf("failed to read piece %d from file %s: %v", index, filePath, err)
			}

			bytesRead += int64(readBytes)
			currentOffset += fileSize

			if bytesRead >= actualLength {
				break
			}
		}

		if bytesRead < actualLength {
			return nil, fmt.Errorf("incomplete read for piece %d", index)
		}
	}

	log.WithField("piece_index", index).Debug("Successfully read piece from disk")
	return pieceData, nil
}

// GetBlock retrieves a specific block of data from the downloaded files.
// pieceIndex: The index of the piece.
// offset: The byte offset within the piece where the block starts.
// length: The length of the block in bytes.
func (dm *DownloadManager) GetBlock(pieceIndex, offset, length uint32) ([]byte, error) {
	dm.Mu.Lock()
	defer dm.Mu.Unlock()

	log := log.WithFields(logrus.Fields{
		"piece_index": pieceIndex,
		"offset":      offset,
		"length":      length,
	})

	// Validate pieceIndex
	if int(pieceIndex) >= len(dm.Pieces) {
		log.Error("Invalid piece index")
		return nil, fmt.Errorf("invalid piece index: %d", pieceIndex)
	}

	piece := dm.Pieces[pieceIndex]

	// Example: Check if the block has already been received
	if piece.Blocks[offset/downloader.BlockSize] {
		log.Warn("Block already received")
		return nil, fmt.Errorf("block already received")
	}

	// Validate offset and length
	info := dm.Writer.Info()
	pieceLength := info.PieceLength
	totalPieces := info.CountPieces()

	if int(pieceIndex) == totalPieces-1 {
		// Last piece may have a different length
		/* // Copy over?
		info := dm.Writer.Info()
		if int64(offset)+int64(length) > info.PieceLength {
		    length = int(info.PieceLength - int64(offset))
		}

		*/
		remaining := info.TotalLength() - int64(pieceIndex)*pieceLength
		if int64(offset)+int64(length) > remaining {
			log.Error("Requested block exceeds piece boundaries")
			return nil, fmt.Errorf("requested block exceeds piece boundaries")
		}
	} else {
		if int64(offset)+int64(length) > int64(pieceLength) {
			log.Error("Requested block exceeds piece boundaries")
			return nil, fmt.Errorf("requested block exceeds piece boundaries")
		}
	}

	// Determine if it's a single-file or multi-file torrent
	var filePath string
	var fileOffset int64

	if len(info.Files) == 0 {
		// Single-file torrent
		filePath = filepath.Join(dm.DownloadDir, info.Name)
		fileOffset = int64(pieceIndex)*int64(pieceLength) + int64(offset)
	} else {
		// Multi-file torrent
		// Iterate through the files to find which file contains the block
		cumulative := int64(0)
		for _, fileInfo := range info.Files {
			if cumulative+fileInfo.Length > int64(pieceIndex)*int64(pieceLength)+int64(offset) {
				filePath = filepath.Join(dm.DownloadDir, fileInfo.Path(info))
				fileOffset = int64(pieceIndex)*int64(pieceLength) + int64(offset) - cumulative
				break
			}
			cumulative += fileInfo.Length
		}

		// If filePath is still empty, it means the block spans multiple files
		if filePath == "" {
			log.Error("Block spans multiple files, which is not supported")
			return nil, fmt.Errorf("block spans multiple files, which is not supported")
		}
	}

	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		log.WithError(err).Error("Failed to open file for reading block")
		return nil, fmt.Errorf("failed to open file %s: %v", filePath, err)
	}
	defer file.Close()

	// Seek to the fileOffset
	_, err = file.Seek(fileOffset, io.SeekStart)
	if err != nil {
		log.WithError(err).Error("Failed to seek to block offset")
		return nil, fmt.Errorf("failed to seek to offset %d in file %s: %v", fileOffset, filePath, err)
	}

	// Read the block
	blockData := make([]byte, length)
	n, err := io.ReadFull(file, blockData)
	if err != nil {
		log.WithError(err).Error("Failed to read block data")
		return nil, fmt.Errorf("failed to read block data: %v", err)
	}
	if uint32(n) != length {
		log.WithFields(logrus.Fields{
			"expected_length": length,
			"read_length":     n,
		}).Error("Incomplete block read")
		return nil, fmt.Errorf("incomplete block read: expected %d bytes, got %d bytes", length, n)
	}

	// Example: Mark the block as received
	piece.Blocks[offset/downloader.BlockSize] = true

	// Example: Check if the piece is complete
	if dm.isPieceComplete(piece) {
		log.Info("Piece completed")
		// Additional logic for completed piece
	}

	log.Debug("Successfully retrieved block data")
	return blockData, nil
}

func (dm *DownloadManager) BroadcastHave(pieceIndex uint32) {
	dm.Mu.Lock()
	defer dm.Mu.Unlock()

	for _, peerConn := range dm.Peers {
		go func(pc *pp.PeerConn) {
			err := pc.SendHave(pieceIndex)
			if err != nil {
				logrus.WithFields(logrus.Fields{
					"peer":        pc.RemoteAddr().String(),
					"piece_index": pieceIndex,
				}).WithError(err).Error("Failed to send 'Have' message to peer")
			} else {
				logrus.WithFields(logrus.Fields{
					"peer":        pc.RemoteAddr().String(),
					"piece_index": pieceIndex,
				}).Info("Sent 'Have' message to peer")
			}
		}(peerConn)
	}
}

func (dm *DownloadManager) CalculateNumberOfBlocks(pieceIndex int) int {
	info := dm.Writer.Info()
	if pieceIndex < dm.TotalPieces-1 {
		return int(info.PieceLength / downloader.BlockSize)
	}
	// Handle the last piece which might be smaller
	remaining := info.TotalLength() - info.PieceLength*(int64(dm.TotalPieces-1))
	return int(math.Ceil(float64(remaining) / float64(downloader.BlockSize)))
}

func (dm *DownloadManager) GetPieceLength(index uint32) int64 {
	info := dm.Writer.Info()
	if int(index) == len(dm.Pieces)-1 {
		// Last piece
		return info.TotalLength() - int64(index)*info.PieceLength
	}
	return info.PieceLength
}
