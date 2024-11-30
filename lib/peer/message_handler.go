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
	"eeptorrent/lib/upload"
	"fmt"
	"github.com/go-i2p/go-i2p-bt/bencode"
	"github.com/go-i2p/go-i2p-bt/downloader"
	pp "github.com/go-i2p/go-i2p-bt/peerprotocol"
	"github.com/sirupsen/logrus"
	"sync/atomic"
)

const BlockSize = downloader.BlockSize // 16KB blocks

func handleMessage(pc *pp.PeerConn, msg pp.Message, dm *download.DownloadManager, ps *PeerState, pm *PeerManager) error {
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
		info := dm.Writer.Info()
		log.WithFields(logrus.Fields{
			"peer":             pc.RemoteAddr().String(),
			"bitfield_length":  len(msg.BitField),
			"pieces_available": available,
			"total_pieces":     info.CountPieces(),
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
		pm.OnPeerChoke(pc)
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
		pm.OnPeerUnchoke(pc)

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
		//pc.PeerInterested = true
		pm.OnPeerInterested(pc)
		return nil
	case pp.MTypeNotInterested:
		log.Debug("Received Not Interested message")
		//pc.PeerInterested = false
		pm.OnPeerNotInterested(pc)
	case pp.MTypeRequest:
		log.WithFields(logrus.Fields{
			"piece_index": msg.Index,
			"begin":       msg.Begin,
			"length":      msg.Length,
		}).Info("Received Request message from peer")

		// Handle upload request
		blockData, err := dm.GetBlock(msg.Index, msg.Begin, msg.Length)
		if err != nil {
			log.WithError(err).Error("Failed to retrieve requested block")
			return err
		}

		// Send the piece message back to the peer
		err = pc.SendPiece(msg.Index, msg.Begin, blockData)
		if err != nil {
			log.WithError(err).Error("Failed to send Piece message to peer")
			return err
		}

		log.WithFields(logrus.Fields{
			"piece_index": msg.Index,
			"begin":       msg.Begin,
			"length":      msg.Length,
		}).Info("Sent Piece message in response to Request")
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

		pm.UpdatePeerStats(pc, int64(len(msg.Piece)), 0)

		pendingAfter := atomic.LoadInt32(&ps.PendingRequests)
		log.WithFields(logrus.Fields{
			"piece_index":     msg.Index,
			"pending_after":   pendingAfter,
			"request_pending": ps.RequestPending,
		}).Debug("Successfully processed piece")

		//if dm.IsPieceComplete(msg.Index) { // Ensure this method accurately checks piece completion
		// Call VerifyPiece to check if the piece is complete and valid
		verified, err := dm.VerifyPiece(msg.Index)
		if err != nil {
			logrus.Errorf("Failed to verify piece: %v", err)
		}
		if verified {
			// Send 'Have' message to inform peers about the completed piece
			err := pc.SendHave(msg.Index)
			if err != nil {
				log.WithError(err).Error("Failed to send 'Have' message to peer")
			} else {
				log.WithField("piece_index", msg.Index).Info("Sent 'Have' message to peer")
			}
		}

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

// requestNextBlock requests the next available block from the peer.
func requestNextBlock(pc *pp.PeerConn, dm *download.DownloadManager, ps *PeerState) error {
	log := logrus.WithFields(logrus.Fields{
		"peer":             pc.RemoteAddr().String(),
		"pending_requests": atomic.LoadInt32(&ps.PendingRequests),
	})

	// Don't request if choked
	if pc.PeerChoked {
		log.Debug("Not requesting blocks - peer has us choked")
		return nil
	}

	const pipelineLimit = 5
	var firstErr error

	for atomic.LoadInt32(&ps.PendingRequests) < int32(pipelineLimit) && !dm.IsFinished() {
		var pieceIndex uint32
		var blockNum int
		var found bool

		// Lock dm only when accessing shared resources
		dm.Mu.Lock()
		totalPieces := len(dm.Pieces)
		dm.Mu.Unlock()

		// Iterate over all pieces
		for i := 0; i < totalPieces; i++ {
			dm.Mu.Lock()
			// Check if we already have this piece
			if dm.Bitfield.IsSet(uint32(i)) {
				dm.Mu.Unlock()
				continue
			}
			// Check if the peer has this piece
			if !pc.BitField.IsSet(uint32(i)) {
				dm.Mu.Unlock()
				continue
			}
			piece := dm.Pieces[i]
			dm.Mu.Unlock()

			// Lock the piece when accessing its blocks
			piece.Mu.Lock()
			for j, received := range piece.Blocks {
				offset := uint32(j) * BlockSize
				// Check if the block is already received or requested
				if !received && !ps.IsBlockRequested(uint32(i), offset) {
					pieceIndex = uint32(i)
					blockNum = j
					found = true
					// Mark the block as requested
					ps.MarkBlockRequested(pieceIndex, offset)
					atomic.AddInt32(&ps.PendingRequests, 1)
					log.WithFields(logrus.Fields{
						"piece_index":      pieceIndex,
						"block_num":        blockNum,
						"pending_requests": atomic.LoadInt32(&ps.PendingRequests),
					}).Info("Preparing to request block")
					piece.Mu.Unlock()
					break
				}
			}
			if !found {
				piece.Mu.Unlock()
			} else {
				break
			}
		}

		if !found {
			log.Debug("No blocks to request from this peer")
			break
		}

		// Calculate offset and length based on block number
		offset := uint32(blockNum) * BlockSize
		length := BlockSize

		// Adjust length for the last block in the piece
		pieceLength := dm.GetPieceLength(pieceIndex)

		if int64(offset)+int64(length) > pieceLength {
			length = int(pieceLength - int64(offset))
		}

		// For small pieces, adjust length if the piece is smaller than BlockSize
		if pieceLength < int64(length) {
			length = int(pieceLength)
		}

		log.WithFields(logrus.Fields{
			"piece_index": pieceIndex,
			"block_num":   blockNum,
			"offset":      offset,
			"length":      length,
		}).Info("Requesting block")

		// Send the request
		err := pc.SendRequest(pieceIndex, offset, uint32(length))
		if err != nil {
			log.WithError(err).Error("Failed to send request")
			if firstErr == nil {
				firstErr = err
			}
			// Decrement pending requests since the request failed
			atomic.AddInt32(&ps.PendingRequests, -1)
			continue
		}

		// Proceed to request the next block
	}

	if firstErr != nil {
		log.WithError(firstErr).Error("Errors occurred during block requests")
	}

	return firstErr
}

// handleSeedingPeer manages an incoming seeding peer connection.
// It handles requests for pieces, manages upload interactions, and ensures robust error handling.
func HandleSeedingPeer(pc *pp.PeerConn, um *upload.UploadManager, pm *PeerManager) error {
	log := log.WithField("peer", pc.RemoteAddr().String())
	log.Info("Handling new seeding connection")

	// Add peer to PeerManager
	pm.Mu.Lock()
	if pm.Peers == nil {
		pm.Peers = make(map[*pp.PeerConn]*PeerState)
	}
	pm.Peers[pc] = NewPeerState()
	pm.Mu.Unlock()

	defer func() {
		pm.Mu.Lock()
		delete(pm.Peers, pc)
		pm.Mu.Unlock()
	}()

	// Send bitfield to the peer
	err := pc.SendBitfield(um.Bitfield)
	if err != nil {
		log.WithError(err).Error("Failed to send bitfield to peer")
		return fmt.Errorf("failed to send bitfield: %v", err)
	}

	// Unchoke the peer to allow requests
	err = pc.SendUnchoke()
	if err != nil {
		log.WithError(err).Error("Failed to send unchoke to peer")
		return fmt.Errorf("failed to send unchoke: %v", err)
	}

	// Main message handling loop
	for {
		msg, err := pc.ReadMsg()
		if err != nil {
			if err.Error() == "EOF" {
				log.Info("Peer closed the connection")
			} else {
				log.WithError(err).Error("Failed to read message from peer")
			}
			return fmt.Errorf("failed to read message: %v", err)
		}

		switch msg.Type {
		case pp.MTypeRequest:
			// Handle piece requests from the peer
			blockData, err := um.GetBlock(msg.Index, msg.Begin, msg.Length)
			if err != nil {
				log.WithError(err).Error("Failed to retrieve requested block")
				continue
			}

			// Send the piece to the peer
			err = pc.SendPiece(msg.Index, msg.Begin, blockData)
			if err != nil {
				log.WithError(err).Error("Failed to send piece to peer")
				continue
			}

			// Update upload stats
			uploadSize := int64(len(blockData))
			um.Mu.Lock()
			um.Uploaded += uploadSize
			um.Mu.Unlock()
			pm.UpdatePeerStats(pc, 0, uploadSize)

			log.WithFields(logrus.Fields{
				"index": msg.Index,
				"begin": msg.Begin,
				"size":  len(blockData),
			}).Debug("Successfully sent piece to peer")

		case pp.MTypeInterested:
			// Handle interested messages
			pm.OnPeerInterested(pc)
			log.Debug("Peer is interested in downloading")

		case pp.MTypeNotInterested:
			// Handle not interested messages
			pm.OnPeerNotInterested(pc)
			log.Debug("Peer is not interested in downloading")

		case pp.MTypeHave:
			// Update peer's bitfield
			if int(msg.Index) < len(pc.BitField) {
				pc.BitField.Set(msg.Index)
				log.WithField("piece_index", msg.Index).Debug("Updated peer's bitfield with new piece")
			} else {
				log.WithField("piece_index", msg.Index).Warn("Received 'Have' for invalid piece index")
			}

		case pp.MTypeBitField:
			// Update peer's bitfield
			pc.BitField = msg.BitField
			log.WithField("pieces", pc.BitField.String()).Debug("Received peer's bitfield")

		case pp.MTypeCancel:
			// Handle cancel requests if needed
			log.Debug("Received 'Cancel' message from peer (ignored in seeding)")

		default:
			// Log unhandled message types
			log.WithField("message_type", msg.Type.String()).Debug("Received unhandled message type from peer")
		}
	}
}
