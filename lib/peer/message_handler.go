package peer

import (
	"eeptorrent/lib/download"
	"fmt"
	pp "github.com/go-i2p/go-i2p-bt/peerprotocol"
	"github.com/sirupsen/logrus"
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

		err := dm.OnBlock(msg.Index, msg.Begin, msg.Piece)
		if err != nil {
			log.WithError(err).Error("Error handling piece")
			return err
		}
		// Request next block
		log.Debug("Successfully processed piece, requesting next block")
		return requestNextBlock(pc, dm, ps)
	case pp.MTypeExtended:
		log.Debug("Received extended message, which is currently not supported")
		return nil
	default:
		// Handle other message types if necessary
		log.WithField("message_type", msg.Type).Debug("Ignoring unhandled message type")
	}
	return nil
}
func requestNextBlock(pc *pp.PeerConn, dm *download.DownloadManager, ps *PeerState) error {
	log := log.WithField("peer", pc.RemoteAddr().String())

	for !dm.IsFinished() {
		if dm.PLength <= 0 {
			dm.PIndex++
			if dm.IsFinished() {
				log.Info("Download finished")
				return nil
			}

			dm.POffset = 0
			// Adjusted to use dm.writer.Info()
			/*
				pieceLength := dm.writer.Info().Piece(int(dm.pindex)).Length()
				dm.plength = pieceLength
				log.WithField("plength", dm.plength).Debug("Set dm.plength")

			*/
			remainingData := dm.Writer.Info().TotalLength() - int64(dm.PIndex)*dm.Writer.Info().PieceLength
			if remainingData < dm.Writer.Info().PieceLength {
				dm.PLength = remainingData
			} else {
				dm.PLength = dm.Writer.Info().PieceLength
			}
			log.WithField("plength", dm.PLength).Debug("Set dm.plength")
		}

		// Check if peer has the piece
		if !pc.BitField.IsSet(uint32(int(dm.PIndex))) {
			// Try next piece
			log.WithField("piece_index", dm.PIndex).Debug("Peer doesn't have requested piece, trying next")
			dm.PIndex++
			dm.POffset = 0
			continue
		}

		// Calculate block size
		length := uint32(BlockSize)
		remaining := uint32(dm.PLength) - dm.POffset
		if remaining < length {
			length = remaining
		}
		log.WithFields(logrus.Fields{
			"piece_index": dm.PIndex,
			"offset":      dm.POffset,
			"length":      length,
		}).Debug("Requesting block")
		// Adjust length to not exceed remaining data in the piece
		/*
			if uint32(dm.plength)-dm.poffset < length {
				length = uint32(dm.plength) - dm.poffset
			}

		*/
		/*
			if length > uint32(dm.plength) {
				length = uint32(dm.plength)
			}
		*/

		// Request the block
		err := pc.SendRequest(dm.PIndex, dm.POffset, length)
		if err == nil {
			dm.Doing = true
			ps.RequestPending = true
			//dm.requestPending = true
			//fmt.Printf("\rRequesting piece %d (%d/%d), offset %d, length %d",
			//dm.pindex, dm.pindex+1, dm.writer.Info().CountPieces(), dm.poffset, length)
			log.WithFields(logrus.Fields{
				"piece_index":  dm.PIndex,
				"total_pieces": dm.Writer.Info().CountPieces(),
				"offset":       dm.POffset,
				"length":       length,
			}).Info("Successfully requested block")
			return nil
		} else {
			log.WithError(err).Error("Failed to send request")
			return err
		}
	}
	return nil
}
