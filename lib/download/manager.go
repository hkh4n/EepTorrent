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
	"crypto/sha1"
	"fmt"
	"github.com/go-i2p/go-i2p-bt/downloader"
	"github.com/go-i2p/go-i2p-bt/metainfo"
	pp "github.com/go-i2p/go-i2p-bt/peerprotocol"
	"github.com/sirupsen/logrus"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

var log = logrus.StandardLogger()

type PieceStatus struct {
	Index       uint32
	TotalBlocks uint32
	Blocks      []bool // true if block is received
	Completed   bool
	Mu          sync.Mutex
}

type DownloadManager struct {
	Writer          metainfo.Writer
	Pieces          []*PieceStatus
	Bitfield        pp.BitField
	Downloaded      int64
	Uploaded        int64
	Left            int64
	Mu              sync.Mutex
	CurrentPiece    uint32
	CurrentOffset   uint32
	RequestedBlocks map[uint32]map[uint32]bool // piece index -> offset -> requested
	Peers           []*pp.PeerConn
	DownloadDir     string
}

// BlockInfo represents a specific block within a piece.
type BlockInfo struct {
	PieceIndex uint32
	Offset     uint32
	Length     uint32
}

func NewDownloadManager(writer metainfo.Writer, totalLength int64, pieceLength int64, totalPieces int) *DownloadManager {
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
		Downloaded:      0,
		Uploaded:        0,
		Left:            totalLength,
		CurrentPiece:    0,
		CurrentOffset:   0,
		RequestedBlocks: make(map[uint32]map[uint32]bool), // Initialize RequestedBlocks
		Peers:           make([]*pp.PeerConn, 0),          // Initialize Peers
	}
}

// IsFinished checks if the download is complete
func (dm *DownloadManager) IsFinished() bool {
	log.Debug("Checking if download is finished")
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

	// Calculate block number based on offset.
	blockNum := offset / downloader.BlockSize
	if blockNum >= piece.TotalBlocks {
		log.WithFields(logrus.Fields{
			"block_num":    blockNum,
			"total_blocks": piece.TotalBlocks,
		}).Error("Received block with invalid offset")
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
		// Verify the piece's integrity.
		if dm.VerifyPiece(index) {
			dm.Bitfield.Set(index)
			piece.Completed = true
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
					// Optionally handle the error, e.g., remove the unresponsive peer.
				} else {
					log.WithFields(logrus.Fields{
						"peer":        peerConn.RemoteAddr().String(),
						"piece_index": index,
					}).Info("Sent Have message to peer")
				}
			}
		} else {
			log.WithFields(logrus.Fields{
				"piece_index": index,
			}).Error("Piece verification failed")
			// Verification
			for i := range piece.Blocks {
				piece.Blocks[i] = false
			}
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
			// Notify UI here
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
	piece.Completed = true
	log.WithField("piece_index", piece.Index).Info("Piece marked as completed")
	return true
}

func (dm *DownloadManager) NeedPiecesFrom(pc *pp.PeerConn) bool {
	dm.Mu.Lock()
	defer dm.Mu.Unlock()
	log := log.WithField("peer", pc.RemoteAddr().String())
	log.Debug("Checking if we need pieces from this peer")

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
	progress := (float64(completedPieces) / float64(totalPieces)) * 100
	log.WithFields(logrus.Fields{
		"completed_pieces": completedPieces,
		"total_pieces":     totalPieces,
		"progress":         fmt.Sprintf("%.2f%%", progress),
	}).Debug("Progress calculated")
	return progress
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
	pieceData, err := dm.ReadPiece(index)
	if err != nil {
		log.WithError(err).Error("Failed to read piece for verification")
		return false
	}

	expectedHash := dm.Writer.Info().Pieces[index]
	actualHash := sha1.Sum(pieceData)

	if !bytes.Equal(expectedHash[:], actualHash[:]) {
		log.WithFields(logrus.Fields{
			"piece_index":   index,
			"expected_hash": fmt.Sprintf("%x", expectedHash),
			"actual_hash":   fmt.Sprintf("%x", actualHash),
		}).Error("Piece hash mismatch")
		return false
	}

	dm.Mu.Lock()
	dm.Bitfield.Set(index)
	dm.Mu.Unlock()

	log.WithFields(logrus.Fields{
		"piece_index": index,
	}).Info("Piece verified successfully")
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
		for _, fileInfo := range info.Files {
			filePath := filepath.Join(dm.DownloadDir, fileInfo.Path(info))
			fileSize := fileInfo.Length

			if currentOffset+fileSize < int64(index)*pieceLength {
				currentOffset += fileSize
				continue
			}

			file, err := os.Open(filePath)
			if err != nil {
				return nil, fmt.Errorf("failed to open file %s: %v", filePath, err)
			}

			defer file.Close()

			pieceStart := int64(index)*pieceLength - currentOffset
			toRead := actualLength
			if pieceStart+toRead > fileSize {
				toRead = fileSize - pieceStart
			}

			n, err := file.ReadAt(pieceData[:toRead], pieceStart)
			if err != nil && err != io.EOF {
				return nil, fmt.Errorf("failed to read piece %d from file %s: %v", index, filePath, err)
			}

			if int64(n) != toRead {
				return nil, fmt.Errorf("incomplete read for piece %d from file %s: expected %d bytes, got %d bytes", index, filePath, toRead, n)
			}

			currentOffset += fileSize

			if toRead >= actualLength {
				break
			}
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
