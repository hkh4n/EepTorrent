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
	pp "github.com/go-i2p/go-i2p-bt/peerprotocol"
	"github.com/sirupsen/logrus"
)

func createDownloadHandler(dm *DownloadManager) pp.Handler {
	log.Debug("Creating download handler")
	return &DownloadHandler{
		NoopHandler: pp.NoopHandler{},
		dm:          dm,
	}
}

type DownloadHandler struct {
	pp.NoopHandler
	dm *DownloadManager
}

func (h *DownloadHandler) OnHandShake(pc *pp.PeerConn) error {
	log.WithField("peer", pc.RemoteAddr().String()).Debug("OnHandShake called")
	return nil
}

func (h *DownloadHandler) OnMessage(pc *pp.PeerConn, msg pp.Message) error {
	log.WithFields(logrus.Fields{
		"peer":         pc.RemoteAddr().String(),
		"message_type": msg.Type.String(),
	}).Debug("OnMessage called")
	switch msg.Type {
	case pp.MTypeBitField:
		pc.BitField = msg.BitField
		log.Debug("Processed BitField message")
		return nil

	case pp.MTypeHave:
		pc.BitField.Set(msg.Index)
		log.WithField("piece_index", msg.Index).Debug("Processed Have message")
		return nil

	case pp.MTypePiece:
		log.WithFields(logrus.Fields{
			"piece_index": msg.Index,
			"begin":       msg.Begin,
			"length":      len(msg.Piece),
		}).Debug("Processing Piece message")
		return h.dm.OnBlock(msg.Index, msg.Begin, msg.Piece)

	default:
		log.WithField("message_type", msg.Type.String()).Debug("Ignoring unhandled message type")
		return nil
	}
}
