package upload

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
	"fmt"
	"github.com/go-i2p/go-i2p-bt/metainfo"
	pp "github.com/go-i2p/go-i2p-bt/peerprotocol"
	"github.com/sirupsen/logrus"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

var log = logrus.StandardLogger()

type UploadManager struct {
	Writer          metainfo.Writer
	Info            metainfo.Info
	Bitfield        pp.BitField
	DownloadDir     string
	Peers           []*pp.PeerConn
	Mu              sync.Mutex
	Uploaded        int64
	UploadedSession int64
	LastUploadTime  time.Time
	ctx             context.Context
	cancelFunc      context.CancelFunc
	wg              sync.WaitGroup
}

func NewUploadManager(writer metainfo.Writer, info metainfo.Info, downloadDir string) *UploadManager {
	ctx, cancel := context.WithCancel(context.Background())

	totalPieces := info.CountPieces()
	bitfield := pp.NewBitField(totalPieces)
	for i := 0; i < totalPieces; i++ {
		bitfield.Set(uint32(i))
	}

	return &UploadManager{
		Writer:      writer,
		Info:        info,
		Bitfield:    bitfield,
		DownloadDir: downloadDir,
		Peers:       make([]*pp.PeerConn, 0),
		ctx:         ctx,
		cancelFunc:  cancel,
	}
}

func (um *UploadManager) Shutdown() {
	um.cancelFunc()
	um.wg.Wait()
}

// AddPeer adds a new peer to the UploadManager.
func (um *UploadManager) AddPeer(peer *pp.PeerConn) {
	um.Mu.Lock()
	defer um.Mu.Unlock()
	log.WithField("peer", peer.RemoteAddr().String()).Debug("Adding peer to UploadManager")
	um.Peers = append(um.Peers, peer)
}

// RemovePeer removes a peer from the UploadManager.
func (um *UploadManager) RemovePeer(peer *pp.PeerConn) {
	um.Mu.Lock()
	defer um.Mu.Unlock()
	log.WithField("peer", peer.RemoteAddr().String()).Debug("Removing peer from UploadManager")
	for i, p := range um.Peers {
		if p == peer {
			um.Peers = append(um.Peers[:i], um.Peers[i+1:]...)
			break
		}
	}
}

// GetBlock retrieves a specific block of data to send to a peer.
func (um *UploadManager) GetBlock(pieceIndex, offset, length uint32) ([]byte, error) {
	log := log.WithFields(logrus.Fields{
		"piece_index": pieceIndex,
		"offset":      offset,
		"length":      length,
	})

	info := um.Info
	pieceLength := info.PieceLength
	totalPieces := info.CountPieces()

	// Validate piece index
	if int(pieceIndex) >= totalPieces {
		log.Error("Invalid piece index")
		return nil, fmt.Errorf("invalid piece index: %d", pieceIndex)
	}

	// Validate offset and length
	if int64(offset)+int64(length) > pieceLength && int(pieceIndex) != totalPieces-1 {
		log.Error("Requested block exceeds piece boundaries")
		return nil, fmt.Errorf("requested block exceeds piece boundaries")
	}

	// For the last piece, adjust the length if necessary
	if int(pieceIndex) == totalPieces-1 {
		remaining := info.TotalLength() - int64(pieceIndex)*pieceLength
		if int64(offset)+int64(length) > remaining {
			log.Error("Requested block exceeds piece boundaries in last piece")
			return nil, fmt.Errorf("requested block exceeds piece boundaries")
		}
	}

	// Read the block from disk
	blockData := make([]byte, length)
	absoluteOffset := int64(pieceIndex)*pieceLength + int64(offset)

	if len(info.Files) == 0 {
		// Single-file torrent
		filePath := filepath.Join(um.DownloadDir, info.Name)
		file, err := os.Open(filePath)
		if err != nil {
			log.WithError(err).Error("Failed to open file for reading block")
			return nil, fmt.Errorf("failed to open file %s: %v", filePath, err)
		}
		defer file.Close()

		// Seek to the absolute offset
		_, err = file.Seek(absoluteOffset, io.SeekStart)
		if err != nil {
			log.WithError(err).Error("Failed to seek to block offset")
			return nil, fmt.Errorf("failed to seek to offset %d in file %s: %v", absoluteOffset, filePath, err)
		}

		// Read the block
		n, err := io.ReadFull(file, blockData)
		if err != nil && err != io.EOF {
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
	} else {
		// Multi-file torrent
		// Implement reading across multiple files based on absoluteOffset and length
		// Similar to the ReadPiece method in DownloadManager
		// [Implement code here]
	}

	log.Debug("Successfully retrieved block data")
	return blockData, nil
}

// VerifyPiece verifies the integrity of a piece before serving it to peers.
func (um *UploadManager) VerifyPiece(index uint32) (bool, error) {
	logrus.WithField("piece_index", index).Debug("Verifying piece")

	info := um.Info
	totalPieces := info.CountPieces()

	// Validate the piece index
	if int(index) >= totalPieces {
		logrus.WithFields(logrus.Fields{
			"piece_index":  index,
			"total_pieces": totalPieces,
		}).Error("Invalid piece index for verification")
		return false, fmt.Errorf("invalid piece index: %d", index)
	}

	// Read the piece data from disk
	pieceData, err := um.ReadPiece(index)
	if err != nil {
		logrus.WithError(err).Error("Failed to read piece from disk")
		return false, err
	}

	expectedHash := info.Pieces[index]
	actualHash := sha1.Sum(pieceData)

	if !bytes.Equal(expectedHash[:], actualHash[:]) {
		logrus.WithFields(logrus.Fields{
			"piece_index":   index,
			"expected_hash": fmt.Sprintf("%x", expectedHash),
			"actual_hash":   fmt.Sprintf("%x", actualHash),
		}).Error("Piece hash mismatch (disk verification)")
		return false, fmt.Errorf("piece hash mismatch for piece %d", index)
	}

	logrus.Info("Piece verified successfully")
	return true, nil
}

// ReadPiece reads the data of a specific piece from disk.
func (um *UploadManager) ReadPiece(index uint32) ([]byte, error) {
	logrus.WithField("piece_index", index).Debug("Reading piece from disk")
	info := um.Info
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

	if len(info.Files) == 0 {
		// Single-file torrent
		filePath := filepath.Join(um.DownloadDir, info.Name)
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
		absoluteOffset := int64(index) * pieceLength

		for _, fileInfo := range info.Files {
			filePath := filepath.Join(um.DownloadDir, fileInfo.Path(info))
			fileSize := fileInfo.Length

			if absoluteOffset >= currentOffset+fileSize {
				// The desired data is not in this file
				currentOffset += fileSize
				continue
			}

			// Open the file
			file, err := os.Open(filePath)
			if err != nil {
				return nil, fmt.Errorf("failed to open file %s: %v", filePath, err)
			}

			// Calculate the start position within the file
			fileStart := absoluteOffset - currentOffset
			// Calculate how much to read from this file
			remaining := actualLength - bytesRead
			fileRemaining := fileSize - fileStart
			readLength := remaining
			if readLength > fileRemaining {
				readLength = fileRemaining
			}

			// Seek to the start position
			_, err = file.Seek(fileStart, io.SeekStart)
			if err != nil {
				file.Close()
				return nil, fmt.Errorf("failed to seek in file %s: %v", filePath, err)
			}

			// Read the data
			n, err := io.ReadFull(file, pieceData[bytesRead:bytesRead+readLength])
			file.Close()
			if err != nil && err != io.EOF {
				return nil, fmt.Errorf("failed to read piece data from file %s: %v", filePath, err)
			}

			bytesRead += int64(n)
			currentOffset += fileSize
			absoluteOffset += int64(n)

			if bytesRead >= actualLength {
				break
			}
		}

		if bytesRead < actualLength {
			return nil, fmt.Errorf("incomplete read for piece %d", index)
		}
	}

	logrus.WithField("piece_index", index).Debug("Successfully read piece from disk")
	return pieceData, nil
}

// BroadcastHave sends a 'Have' message to all connected peers for a specific piece.
func (um *UploadManager) BroadcastHave(index uint32) {
	um.Mu.Lock()
	defer um.Mu.Unlock()

	for _, peer := range um.Peers {
		err := peer.SendHave(index)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"peer":        peer.RemoteAddr().String(),
				"piece_index": index,
			}).WithError(err).Error("Failed to send 'Have' message")
		} else {
			logrus.WithFields(logrus.Fields{
				"peer":        peer.RemoteAddr().String(),
				"piece_index": index,
			}).Info("Sent 'Have' message to peer")
		}
	}
}

// HandlePieceRequests processes piece requests from peers.
func (um *UploadManager) HandlePieceRequests(peer *pp.PeerConn) {
	log := log.WithField("peer", peer.RemoteAddr().String())
	for {
		select {
		case <-um.ctx.Done():
			log.Info("UploadManager context cancelled, stopping piece request handling")
			return
		default:
			// Read messages from the peer
			msg, err := peer.ReadMsg()
			if err != nil {
				if err == io.EOF {
					log.Info("Peer closed the connection")
				} else {
					log.WithError(err).Error("Error reading message from peer")
				}
				return
			}

			switch msg.Type {
			case pp.MTypeRequest:
				// Handle the piece request
				blockData, err := um.GetBlock(msg.Index, msg.Begin, msg.Length)
				if err != nil {
					log.WithError(err).Error("Failed to retrieve requested block")
					continue
				}

				// Send the piece
				err = peer.SendPiece(msg.Index, msg.Begin, blockData)
				if err != nil {
					log.WithError(err).Error("Failed to send piece to peer")
					continue
				}

				// Update upload stats
				uploadSize := int64(len(blockData))
				atomic.AddInt64(&um.Uploaded, uploadSize)

				log.WithFields(logrus.Fields{
					"index": msg.Index,
					"begin": msg.Begin,
					"size":  len(blockData),
				}).Debug("Successfully sent piece to peer")

			case pp.MTypeInterested:
				// Handle interested message
				err := peer.SendUnchoke()
				if err != nil {
					log.WithError(err).Error("Failed to send unchoke to peer")
				} else {
					log.Info("Peer is interested; sent unchoke")
				}

			case pp.MTypeNotInterested:
				// Handle not interested message
				log.Info("Peer is not interested")

			case pp.MTypeHave:
				// Peer has a new piece
				if int(msg.Index) < len(peer.BitField) {
					peer.BitField.Set(msg.Index)
					log.WithField("piece_index", msg.Index).Debug("Updated peer's bitfield with new piece")
				} else {
					log.WithField("piece_index", msg.Index).Warn("Received 'Have' for invalid piece index")
				}

			case pp.MTypeBitField:
				// Peer sent its bitfield
				peer.BitField = msg.BitField
				log.WithField("pieces", peer.BitField.String()).Debug("Received peer's bitfield")

			case pp.MTypeCancel:
				// Handle cancel request
				log.Debug("Received 'Cancel' message from peer (ignored in seeding)")

			default:
				// Log unhandled message types
				log.WithField("message_type", msg.Type.String()).Debug("Received unhandled message type from peer")
			}
		}
	}
}

// Serve starts handling requests from peers.
func (um *UploadManager) Serve() {
	um.wg.Add(1)
	defer um.wg.Done()

	log.Info("UploadManager is serving peers")
	// Implement logic if needed to periodically check peers or handle uploads
}

// GetUploadStats returns the total uploaded bytes.
func (um *UploadManager) GetUploadStats() int64 {
	return atomic.LoadInt64(&um.Uploaded)
}
