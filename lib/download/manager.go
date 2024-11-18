package download

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
	"fmt"
	"github.com/go-i2p/go-i2p-bt/metainfo"
	pp "github.com/go-i2p/go-i2p-bt/peerprotocol"
	"github.com/sirupsen/logrus"
	"sync/atomic"
)

var log = logrus.New()

type DownloadManager struct {
	Writer     metainfo.Writer
	PIndex     uint32
	POffset    uint32
	PLength    int64
	Doing      bool
	Bitfield   pp.BitField
	Downloaded int64
	Uploaded   int64
	Left       int64
}

func NewDownloadManager(writer metainfo.Writer, totalLength int64, pieceLength int64, totalPieces int) *DownloadManager {
	remainingData := totalLength
	var dm_plength int64
	if remainingData < pieceLength {
		dm_plength = remainingData
	} else {
		dm_plength = pieceLength
	}

	return &DownloadManager{
		Writer:   writer,
		Bitfield: pp.NewBitField(totalPieces),
		Left:     totalLength,
		PIndex:   0,
		POffset:  0,
		PLength:  dm_plength,
	}
}

func (dm *DownloadManager) IsFinished() bool {
	finished := dm.PIndex >= uint32(dm.Writer.Info().CountPieces())
	log.WithFields(logrus.Fields{
		"current_index": dm.PIndex,
		"total_pieces":  dm.Writer.Info().CountPieces(),
		"is_finished":   finished,
	}).Debug("Checking if download is finished")
	return finished
}
func (dm *DownloadManager) OnBlock(index, offset uint32, b []byte) error {
	log := log.WithFields(logrus.Fields{
		"piece_index":    index,
		"current_index":  dm.PIndex,
		"offset":         offset,
		"current_offset": dm.POffset,
		"block_size":     len(b),
	})

	//dm.requestPending = false
	if dm.PIndex != index {
		log.Error("Inconsistent piece index")
		return fmt.Errorf("inconsistent piece: old=%d, new=%d", dm.PIndex, index)
	}
	if dm.POffset != offset {
		log.Error("Inconsistent offset")
		return fmt.Errorf("inconsistent offset for piece '%d': old=%d, new=%d",
			index, dm.POffset, offset)
	}

	dm.Doing = false
	n, err := dm.Writer.WriteBlock(index, offset, b)
	if err == nil {
		dm.POffset = offset + uint32(n)
		dm.PLength -= int64(n)
		atomic.AddInt64(&dm.Downloaded, int64(n))
		atomic.AddInt64(&dm.Left, -int64(n))

		log.WithFields(logrus.Fields{
			"bytes_written":    n,
			"new_offset":       dm.POffset,
			"remaining_length": dm.PLength,
			"total_downloaded": dm.Downloaded,
			"remaining_bytes":  dm.Left,
		}).Debug("Updated download progress")

		// Update bitfield for completed piece
		if dm.PLength <= 0 {
			dm.Bitfield.Set(index)
			log.WithFields(logrus.Fields{
				"piece_index":  index,
				"total_pieces": dm.Writer.Info().CountPieces(),
				"progress":     fmt.Sprintf("%.2f%%", float64(index+1)/float64(dm.Writer.Info().CountPieces())*100),
			}).Info("Completed piece")
		}
	} else {
		log.WithError(err).Error("Failed to write block")
	}
	return err
}
func (dm *DownloadManager) NeedPiecesFrom(pc *pp.PeerConn) bool {
	log := log.WithField("peer", pc.RemoteAddr().String())

	for i := 0; i < dm.Writer.Info().CountPieces(); i++ {
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
func (dm *DownloadManager) LogProgress() {
	progress := dm.Progress()
	downloaded := atomic.LoadInt64(&dm.Downloaded)
	left := atomic.LoadInt64(&dm.Left)
	log.WithFields(logrus.Fields{
		"progress":         fmt.Sprintf("%.2f%%", progress),
		"downloaded_bytes": downloaded,
		"total_bytes":      dm.Writer.Info().TotalLength(),
		"remaining_bytes":  left,
		"current_piece":    dm.PIndex,
		"current_offset":   dm.POffset,
	}).Info("Download progress update")
}

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
