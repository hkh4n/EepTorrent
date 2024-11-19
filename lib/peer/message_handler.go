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
	"eeptorrent/lib/download"
	"fmt"
	"github.com/go-i2p/go-i2p-bt/bencode"
	pp "github.com/go-i2p/go-i2p-bt/peerprotocol"
	"github.com/sirupsen/logrus"
	"sync/atomic"
)

var log = logrus.New()

const BlockSize = 16384 // 16KB blocks

func handleMessage(pc *pp.PeerConn, msg pp.Message, dm *download.DownloadManager, ps *PeerState) error {
	log := log.WithFields(logrus.Fields{
		"peer":         pc.RemoteAddr().String(),
		"message_type": msg.Type.String(),
	})

	log.WithField("msg.ExtendedPayload", fmt.Sprintf("%x", msg.ExtendedPayload)).Debug("Handling peer message")
	log.WithFields(logrus.Fields{
		"extended_id":     msg.ExtendedID,
		"ExtendedPayload": fmt.Sprintf("%x", msg.ExtendedPayload),
	}).Debug("Handling peer message in detail")

	switch msg.Type {
	case pp.MTypeBitField:
		available := 0
		for i := 0; i < len(msg.BitField); i++ {
			if msg.BitField.IsSet(uint32(i)) {
				available++
			}
		}
		log.WithFields(logrus.Fields{
			"peer":             pc.RemoteAddr().String(),
			"bitfield_length":  len(msg.BitField),
			"pieces_available": available,
			"total_pieces":     dm.Writer.Info().CountPieces(),
			"peer_choked":      pc.PeerChoked,
		}).Info("Received bitfield")

		pc.BitField = msg.BitField
		// Send interested message if the peer has pieces we need
		if dm.NeedPiecesFrom(pc) {
			log.Debug("Peer has needed pieces, sending interested message")
			err := pc.SendInterested()
			if err != nil {
				log.WithError(err).Error("Failed to send Interested message")
				//log.Printf("Failed to send Interested message: %v", err)
				return err
			}
			log.Debug("Successfully sent interested message")
			return requestNextBlock(pc, dm, ps)
		} else {
			log.Debug("Peer has no needed pieces")
		}

	case pp.MTypeHave:
		log.WithField("piece_index", msg.Index).Debug("Received have message")
		pc.BitField.Set(msg.Index)
		// Send interested message if the peer has pieces we need
		if dm.NeedPiecesFrom(pc) {
			log.Debug("Peer has needed pieces, sending interested message")
			err := pc.SendInterested()
			if err != nil {
				log.WithError(err).Error("Failed to send Interested message")
				return err
			}
			log.Debug("Successfully sent interested message")
		} else {
			log.Debug("Peer has no needed pieces")
		}
	case pp.MTypeChoke:
		log.WithField("peer", pc.RemoteAddr().String()).Info("Peer choked us")
		pc.PeerChoked = true
		ps.RequestPending = false                 // Reset request pending state
		atomic.StoreInt32(&ps.PendingRequests, 0) // Reset pending requests
		// When choked, we should stop sending requests
		//ps.C = true
	case pp.MTypeUnchoke:
		// Start requesting pieces
		log.WithFields(logrus.Fields{
			"pending_requests": atomic.LoadInt32(&ps.PendingRequests),
			"request_pending":  ps.RequestPending,
			"peer_choked":      pc.PeerChoked,
		}).Info("Received unchoke message")

		pc.PeerChoked = false

		if !ps.RequestPending {
			log.Info("Peer has unchoked us, starting to request pieces")
			return requestNextBlock(pc, dm, ps)
		} else {
			log.WithFields(logrus.Fields{
				"pending_requests": atomic.LoadInt32(&ps.PendingRequests),
			}).Info("Already have pending request, waiting for piece")
		}
	case pp.MTypeInterested:
		log.Debug("Received Interested message")
		pc.PeerInterested = true
		// Optionally, decide whether to choke or unchoke the peer
		return nil
	case pp.MTypeNotInterested:
		log.Debug("Received Not Interested message")
		pc.PeerInterested = false
		// Optionally, decide whether to choke the peer
	case pp.MTypeRequest:
		log.WithFields(logrus.Fields{
			"piece_index": msg.Index,
			"begin":       msg.Begin,
			"length":      msg.Length,
		}).Debug("Received Request message")
		// Handle requests from peers (uploading)
	case pp.MTypeCancel:
		log.WithFields(logrus.Fields{
			"piece_index": msg.Index,
			"begin":       msg.Begin,
			"length":      msg.Length,
		}).Debug("Received Cancel message")
		// Handle cancel requests from peers
		// If we have any pending uploads matching the request, we should cancel them
	case pp.MTypePort:
		log.WithField("listen_port", msg.Port).Debug("Received Port message")
		//Requires DHT support
	case pp.MTypePiece:
		log.WithFields(logrus.Fields{
			"peer":           pc.RemoteAddr().String(),
			"piece_index":    msg.Index,
			"begin":          msg.Begin,
			"length":         len(msg.Piece),
			"current_piece":  dm.CurrentPiece,
			"current_offset": dm.CurrentOffset,
		}).Info("Received piece")

		ps.RequestPending = false
		atomic.AddInt32(&ps.PendingRequests, -1) // Safely decrement

		err := dm.OnBlock(msg.Index, msg.Begin, msg.Piece)
		if err != nil {
			log.WithError(err).Error("Error handling piece")
			return err
		}
		/*
			pendingAfter := atomic.LoadInt32(&ps.PendingRequests)
			log.WithFields(logrus.Fields{
				"piece_index":     msg.Index,
				"pending_after":   pendingAfter,
				"request_pending": ps.RequestPending,
			}).Debug("Successfully processed piece")

		*/
		// Clear this block from RequestedBlocks
		ps.Lock()
		if blocks, exists := ps.RequestedBlocks[msg.Index]; exists {
			delete(blocks, msg.Begin)
		}
		ps.Unlock()

		// Request next block
		log.Debug("Successfully processed piece, requesting next block")
		return requestNextBlock(pc, dm, ps)
	case pp.MTypeExtended:
		//log.Debug("Received extended message, which is currently not supported")
		if msg.ExtendedID == 0 { // Extended handshake
			handshake := pp.ExtendedHandshakeMsg{
				V: "EepTorrent 0.0.0",
				M: make(map[string]uint8),
			}
			handshake.M["ut_metadata"] = 1
			//handshake.M = make(map[string]int)
			var remoteHandshake pp.ExtendedHandshakeMsg
			if err := bencode.DecodeBytes(msg.ExtendedPayload, &remoteHandshake); err != nil {
				log.WithError(err).Error("Failed to decode extended handshake")
				return err
			}
			// Log the remote client info
			log.WithFields(logrus.Fields{
				"remote_client":        remoteHandshake.V,
				"supported_extensions": remoteHandshake.M,
			}).Info("Received extended handshake from peer")
			if err := pc.SendExtHandshakeMsg(handshake); err != nil {
				log.WithError(err).Error("Failed to send extended handshake")
				return err
			}
			log.Debug("Successfully sent extended handshake")
		}
		return nil
	default:
		// Handle other message types if necessary
		log.WithField("message_type", msg.Type).Debug("Ignoring unhandled message type")
	}
	return nil
}
func requestNextBlock(pc *pp.PeerConn, dm *download.DownloadManager, ps *PeerState) error {
	log := log.WithFields(logrus.Fields{
		"peer":             pc.RemoteAddr().String(),
		"pending_requests": atomic.LoadInt32(&ps.PendingRequests),
	})

	// Don't request if choked
	if pc.PeerChoked {
		log.Debug("Not requesting blocks - peer has us choked")
		return nil
	}

	dm.Mu.Lock()
	defer dm.Mu.Unlock()

	// Reset request state if no pending requests
	if atomic.LoadInt32(&ps.PendingRequests) == 0 {
		ps.RequestPending = false
	}

	pipelineLimit := 5
	var firstErr error

	for atomic.LoadInt32(&ps.PendingRequests) < int32(pipelineLimit) && !dm.IsFinished() {
		var pieceIndex uint32 = dm.CurrentPiece

		// Validate the piece index
		if int(pieceIndex) >= dm.Writer.Info().CountPieces() {
			log.Debug("No more pieces to request")
			return nil
		}

		// Check if peer has this piece
		if !pc.BitField.IsSet(pieceIndex) {
			log.WithField("piece_index", pieceIndex).Debug("Peer doesn't have this piece")
			dm.CurrentPiece++
			dm.CurrentOffset = 0
			continue
		}

		// Calculate block size
		pieceLength := dm.Writer.Info().PieceLength
		if pieceIndex == uint32(dm.Writer.Info().CountPieces()-1) {
			remaining := dm.Writer.Info().TotalLength() - int64(pieceIndex)*pieceLength
			if remaining < pieceLength {
				pieceLength = remaining
			}
		}

		offset := dm.CurrentOffset
		length := uint32(BlockSize)
		if int64(offset)+int64(length) > pieceLength {
			length = uint32(pieceLength - int64(offset))
		}

		// Send request
		err := pc.SendRequest(pieceIndex, offset, length)
		if err != nil {
			log.WithError(err).Error("Failed to send request")
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		log.WithFields(logrus.Fields{
			"piece_index": pieceIndex,
			"offset":      offset,
			"length":      length,
		}).Info("Requested block")

		// Mark block as requested
		ps.Lock()
		if ps.RequestedBlocks[pieceIndex] == nil {
			ps.RequestedBlocks[pieceIndex] = make(map[uint32]bool)
		}
		ps.RequestedBlocks[pieceIndex][offset] = true
		ps.Unlock()

		atomic.AddInt32(&ps.PendingRequests, 1)
		ps.RequestPending = true

		// Update offsets
		dm.CurrentOffset += length
		if dm.CurrentOffset >= uint32(pieceLength) {
			dm.CurrentPiece++
			dm.CurrentOffset = 0
		}
	}

	if firstErr != nil {
		log.WithError(firstErr).Error("Errors occurred during block requests")
	}

	return firstErr
}

/*
// requestNextBlock requests the next available block from the DownloadManager
func requestNextBlock(pc *pp.PeerConn, dm *download.DownloadManager, ps *PeerState) error {
	log := log.WithFields(logrus.Fields{
		"peer":             pc.RemoteAddr().String(),
		"pending_requests": atomic.LoadInt32(&ps.PendingRequests),
		"request_pending":  ps.RequestPending,
	})
	pipelineLimit := 5
	var firstErr error

	// Add debug logging for initial state
	log.WithFields(logrus.Fields{
		"peer_has_pieces": pc.BitField != nil,
		"current_piece":   dm.CurrentPiece,
		"current_offset":  dm.CurrentOffset,
		"total_pieces":    dm.Writer.Info().CountPieces(),
	}).Debug("Starting block request")

	dm.Mu.Lock()
	defer dm.Mu.Unlock()

	for atomic.LoadInt32(&ps.PendingRequests) < int32(pipelineLimit) && !dm.IsFinished() {
		// Find next piece we need that this peer has
		var pieceIndex uint32
		found := false
		for i := dm.CurrentPiece; i < uint32(dm.Writer.Info().CountPieces()); i++ {
			if !dm.Bitfield.IsSet(i) && pc.BitField.IsSet(i) {
				pieceIndex = i
				found = true
				log.WithFields(logrus.Fields{
					"piece_index": pieceIndex,
					"we_have_it":  dm.Bitfield.IsSet(i),
					"peer_has_it": pc.BitField.IsSet(i),
				}).Debug("Found needed piece")
				break
			}
		}

		if !found {
			log.Debug("No more pieces needed from this peer")
			return nil
		}

		// Calculate block size for this piece
		remainingInPiece := dm.Writer.Info().PieceLength
		if pieceIndex == uint32(dm.Writer.Info().CountPieces()-1) {
			remaining := dm.Writer.Info().TotalLength() - int64(pieceIndex)*dm.Writer.Info().PieceLength
			if remaining < dm.Writer.Info().PieceLength {
				remainingInPiece = remaining
			}
		}

		// Calculate offset and length for next block
		offset := dm.CurrentOffset
		length := uint32(BlockSize)
		if int64(offset+length) > remainingInPiece {
			length = uint32(remainingInPiece - int64(offset))
		}

		// Check if block already requested
		ps.Lock()
		alreadyRequested := ps.RequestedBlocks[pieceIndex][offset]
		requested := ps.RequestedBlocks[pieceIndex]
		if !alreadyRequested {
			if ps.RequestedBlocks[pieceIndex] == nil {
				ps.RequestedBlocks[pieceIndex] = make(map[uint32]bool)
			}
			ps.RequestedBlocks[pieceIndex][offset] = true
		}
		ps.Unlock()

		log.WithFields(logrus.Fields{
			"piece_index":       pieceIndex,
			"offset":            offset,
			"length":            length,
			"already_requested": alreadyRequested,
			"requested_blocks":  len(requested),
		}).Debug("Block request status")

		if alreadyRequested {
			dm.CurrentOffset += uint32(BlockSize)
			if int64(dm.CurrentOffset) >= remainingInPiece {
				dm.CurrentPiece++
				dm.CurrentOffset = 0
			}
			continue
		}

		// Request the block
		err := pc.SendRequest(pieceIndex, offset, length)
		if err != nil {
			log.WithError(err).Error("Failed to send request")
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		atomic.AddInt32(&ps.PendingRequests, 1)
		ps.RequestPending = true

		log.WithFields(logrus.Fields{
			"piece_index":      pieceIndex,
			"offset":           offset,
			"length":           length,
			"pending_requests": atomic.LoadInt32(&ps.PendingRequests),
		}).Info("Successfully requested block")

		// Increment offset/piece for next request
		dm.CurrentOffset += uint32(BlockSize)
		if int64(dm.CurrentOffset) >= remainingInPiece {
			dm.CurrentPiece++
			dm.CurrentOffset = 0
		}
	}
	if firstErr != nil {
		log.WithError(firstErr).Error("Errors occurred during block requests")
	}

	return firstErr
}


*/
