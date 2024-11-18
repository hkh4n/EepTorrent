package peer

/*
An I2P-only BitTorrent client.
Copyright (C) 2024 Haris Khan
Copyright (C) 2024 The EepTorrent Developers

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
	log.Debug("Handling peer message")

	switch msg.Type {
	case pp.MTypeBitField:
		log.WithField("bitfield_length", len(msg.BitField)).Debug("Received bitfield")
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
		log.Debug("Received Choke message")
		pc.PeerChoked = true
		// When choked, we should stop sending requests
		//ps.C = true
	case pp.MTypeUnchoke:
		// Start requesting pieces
		//log.Info("Peer has unchoked us, starting to request pieces")
		//return requestNextBlock(pc, dm)
		if !ps.RequestPending {
			log.Info("Peer has unchoked us, starting to request pieces")
			return requestNextBlock(pc, dm, ps)
		} else {
			log.Info("Already have a pending request, waiting for piece")
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
			"piece_index": msg.Index,
			"begin":       msg.Begin,
			"length":      len(msg.Piece),
		}).Debug("Received piece")

		ps.RequestPending = false
		atomic.AddInt32(&ps.PendingRequests, -1) // Safely decrement
		err := dm.OnBlock(msg.Index, msg.Begin, msg.Piece)
		if err != nil {
			log.WithError(err).Error("Error handling piece")
			return err
		}
		// Request next block
		log.Debug("Successfully processed piece, requesting next block")
		return requestNextBlock(pc, dm, ps)
	case pp.MTypeExtended:
		//log.Debug("Received extended message, which is currently not supported")
		if msg.ExtendedID == 0 { // Extended handshake
			handshake := pp.ExtendedHandshakeMsg{
				V: "EepTorrent 0.0.1",
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
	log := log.WithField("peer", pc.RemoteAddr().String())
	pipelineLimit := 5
	var firstErr error

	for atomic.LoadInt32(&ps.PendingRequests) < int32(pipelineLimit) && !dm.IsFinished() {
		if dm.PLength <= 0 {
			dm.PIndex++
			if dm.IsFinished() {
				log.Info("Download finished")
				break
			}

			dm.POffset = 0
			remainingData := dm.Writer.Info().TotalLength() - int64(dm.PIndex)*dm.Writer.Info().PieceLength
			if remainingData < dm.Writer.Info().PieceLength {
				dm.PLength = remainingData
			} else {
				dm.PLength = dm.Writer.Info().PieceLength
			}
			log.WithField("plength", dm.PLength).Debug("Set dm.PLength")
		}

		// Check if peer has the piece
		if !pc.BitField.IsSet(uint32(dm.PIndex)) {
			// Try next piece
			log.WithField("piece_index", dm.PIndex).Debug("Peer doesn't have requested piece, trying next")
			dm.PIndex++
			dm.POffset = 0
			continue
		}

		// Calculate remaining bytes in the piece
		remaining := dm.PLength - int64(dm.POffset)
		if remaining <= 0 {
			// No more data to request in this piece
			dm.PLength = 0
			continue
		}

		// Calculate block size
		var length uint32
		if remaining < int64(BlockSize) {
			length = uint32(remaining)
		} else {
			length = uint32(BlockSize)
		}

		log.WithFields(logrus.Fields{
			"piece_index": dm.PIndex,
			"offset":      dm.POffset,
			"length":      length,
		}).Debug("Requesting block")

		// Send the request
		err := pc.SendRequest(dm.PIndex, dm.POffset, length)
		if err != nil {
			log.WithError(err).Error("Failed to send request")
			if firstErr == nil {
				firstErr = err
			}
			// Optionally, decide whether to break or continue on error
			continue
		}

		atomic.AddInt32(&ps.PendingRequests, 1)
		dm.Doing = true
		ps.RequestPending = true
		log.WithFields(logrus.Fields{
			"piece_index":  dm.PIndex,
			"total_pieces": dm.Writer.Info().CountPieces(),
			"offset":       dm.POffset,
			"length":       length,
		}).Info("Successfully requested block")

		// Move to the next block
		dm.POffset += uint32(length)
		if dm.POffset >= uint32(dm.PLength) {
			dm.PLength = 0
		}
	}

	return firstErr
}
