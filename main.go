package main

import (
	"fmt"
	"io"
	"log"
	"time"

	"github.com/go-i2p/go-i2p-bt/metainfo"
	pp "github.com/go-i2p/go-i2p-bt/peerprotocol"
)

/*
p6zlufbvhcn426427wiaylzejdwg4hbdlrccst6owijhlvgalb7a.b32.i2p
cpfrxck5c4stxqrrjsp5syqvhfbtmc2jebyiyuv4hwjnhbxopuyq.b32.i2p
6p225yhqnr2t3kjdh5vy3h2bsv5unfip4777dqfk7qv2ihluf6va.b32.i2p+
cofho7nrtwu47mzejuwk6aszk7zj7aox6b5v2ybdhh5ykrz64jka.b32.i2p+

*/

const BlockSize = 16384 // 16KB blocks

type downloadManager struct {
	writer     metainfo.Writer
	pindex     uint32
	poffset    uint32
	plength    int64
	doing      bool
	bitfield   pp.BitField
	downloaded int64
	uploaded   int64
	left       int64
}

func main() {
	// Load the torrent file
	mi, err := metainfo.LoadFromFile("test.txt.torrent")
	if err != nil {
		log.Fatalf("Failed to load torrent: %v", err)
	}

	info, err := mi.Info()
	if err != nil {
		log.Fatalf("Failed to parse torrent info: %v", err)
	}

	fmt.Printf("Downloading: %s\n", info.Name)
	fmt.Printf("Info Hash: %s\n", mi.InfoHash().HexString())
	fmt.Printf("Total size: %d bytes\n", info.TotalLength())
	fmt.Printf("Piece length: %d bytes\n", info.PieceLength)
	fmt.Printf("Pieces: %d\n", len(info.Pieces))

	// Initialize download manager
	dm := &downloadManager{
		writer:   metainfo.NewWriter("", info, 0600),
		plength:  info.PieceLength,
		bitfield: pp.NewBitField(info.CountPieces()),
		left:     info.TotalLength(),
	}
	defer dm.writer.Close()

	// Try direct connection to I2P peer if tracker-less
	peer := "p6zlufbvhcn426427wiaylzejdwg4hbdlrccst6owijhlvgalb7a.b32.i2p:6881" // Replace with actual peer
	peerId := metainfo.NewRandomHash()

	fmt.Printf("Attempting connection to peer: %s\n", peer)
	err = downloadFromPeer(peer, peerId, mi.InfoHash(), dm)
	if err != nil {
		log.Fatalf("Download failed: %v", err)
	}
}

func downloadFromPeer(peer string, id, infohash metainfo.Hash, dm *downloadManager) error {
	// Create peer connection with extended protocol support
	pc, err := pp.NewPeerConnByDial(peer, id, infohash, time.Second*30)
	if err != nil {
		return fmt.Errorf("failed to connect to peer: %v", err)
	}
	defer pc.Close()

	// Enable extended protocol and metadata exchange
	pc.ExtBits.Set(pp.ExtensionBitExtended)

	// Perform handshake
	if err := pc.Handshake(); err != nil {
		return fmt.Errorf("handshake failed: %v", err)
	}
	fmt.Printf("Connected to peer %s (ID: %s)\n", peer, pc.PeerID.HexString())

	// Create download handler
	handler := createDownloadHandler(dm)

	// Send interested and unchoke messages
	if err := pc.SetInterested(); err != nil {
		return fmt.Errorf("failed to send interested: %v", err)
	}

	// Main download loop
	for !dm.IsFinished() {
		msg, err := pc.ReadMsg()
		if err != nil {
			if err == io.EOF {
				return fmt.Errorf("peer connection closed")
			}
			return fmt.Errorf("failed to read message: %v", err)
		}

		err = pc.HandleMessage(msg, handler)
		if err == pp.ErrChoked {
			continue
		}
		if err != nil {
			return fmt.Errorf("failed to handle message: %v", err)
		}

		// Request more pieces if not currently downloading
		if !dm.doing && !pc.PeerChoked {
			err = requestNextBlock(pc, dm)
			if err != nil {
				return fmt.Errorf("failed to request block: %v", err)
			}
		}
	}

	return nil
}

func createDownloadHandler(dm *downloadManager) pp.Handler {
	return &downloadHandler{
		NoopHandler: pp.NoopHandler{},
		dm:          dm,
	}
}

type downloadHandler struct {
	pp.NoopHandler
	dm *downloadManager
}

func (h *downloadHandler) OnHandShake(pc *pp.PeerConn) error {
	return nil
}

func (h *downloadHandler) OnMessage(pc *pp.PeerConn, msg pp.Message) error {
	switch msg.Type {
	case pp.MTypeBitField:
		pc.BitField = msg.BitField
		return nil

	case pp.MTypeHave:
		pc.BitField.Set(msg.Index)
		return nil

	case pp.MTypePiece:
		return h.dm.OnBlock(msg.Index, msg.Begin, msg.Piece)

	default:
		return nil
	}
}

func requestNextBlock(pc *pp.PeerConn, dm *downloadManager) error {
	if dm.plength <= 0 {
		dm.pindex++
		if dm.IsFinished() {
			return nil
		}

		dm.poffset = 0
		dm.plength = dm.writer.Info().Piece(int(dm.pindex)).Length()
	}

	// Check if peer has the piece
	if !pc.BitField.IsSet(dm.pindex) {
		// Try next piece
		dm.pindex++
		dm.poffset = 0
		return nil
	}

	// Calculate block size
	length := uint32(BlockSize)
	if length > uint32(dm.plength) {
		length = uint32(dm.plength)
	}

	// Request the block
	err := pc.SendRequest(dm.pindex, dm.poffset, length)
	if err == nil {
		dm.doing = true
		fmt.Printf("\rRequesting piece %d (%d/%d), offset %d, length %d",
			dm.pindex, dm.pindex+1, dm.writer.Info().CountPieces(), dm.poffset, length)
	}
	return err
}

func (dm *downloadManager) IsFinished() bool {
	return dm.pindex >= uint32(dm.writer.Info().CountPieces())
}

func (dm *downloadManager) OnBlock(index, offset uint32, b []byte) error {
	if dm.pindex != index {
		return fmt.Errorf("inconsistent piece: old=%d, new=%d", dm.pindex, index)
	}
	if dm.poffset != offset {
		return fmt.Errorf("inconsistent offset for piece '%d': old=%d, new=%d",
			index, dm.poffset, offset)
	}

	dm.doing = false
	n, err := dm.writer.WriteBlock(index, offset, b)
	if err == nil {
		dm.poffset = offset + uint32(n)
		dm.plength -= int64(n)
		dm.downloaded += int64(n)
		dm.left -= int64(n)

		// Update bitfield for completed piece
		if dm.plength <= 0 {
			dm.bitfield.Set(index)
			fmt.Printf("\nCompleted piece %d\n", index)
		}
	}
	return err
}
