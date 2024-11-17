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
	"bytes"
	"context"
	"eeptorrent/lib/download"
	"eeptorrent/lib/i2p"
	"eeptorrent/lib/util"
	"encoding/base32"
	"fmt"
	"github.com/go-i2p/go-i2p-bt/metainfo"
	pp "github.com/go-i2p/go-i2p-bt/peerprotocol"
	"github.com/sirupsen/logrus"
	"io"
	"net"
	"strings"
	"time"
)

type PeerState struct {
	RequestPending  bool
	PendingRequests int
}

func ConnectToPeer(ctx context.Context, peerHash []byte, index int, mi *metainfo.MetaInfo, dm *download.DownloadManager) {
	peerStream := i2p.GlobalStreamSession

	// Convert hash to Base32 address
	peerHashBase32 := strings.ToLower(base32.StdEncoding.EncodeToString(peerHash))
	peerB32Addr := util.CleanBase32Address(peerHashBase32)

	// Lookup the peer's Destination
	peerDest, err := i2p.GlobalSAM.Lookup(peerB32Addr)
	if err != nil {
		log.Fatalf("Failed to lookup peer %s: %v", peerB32Addr, err)
	} else {
		log.Printf("Successfully looked up peer %s\n", peerB32Addr)
	}

	// Attempt to connect
	peerConn, err := peerStream.Dial("tcp", peerDest.String())
	if err != nil {
		log.Fatalf("Failed to connect to peer %s: %v", peerB32Addr, err)
		return
	} else {
		fmt.Printf("Successfully connected to peer %s\n", peerB32Addr)
	}
	defer peerConn.Close()

	// Perform the BitTorrent handshake
	peerId := util.GeneratePeerIdMeta()
	err = performHandshake(peerConn, mi.InfoHash().Bytes(), string(peerId[:]))
	if err != nil {
		log.Fatalf("Handshake with peer %s failed: %v", peerB32Addr, err)
	} else {
		fmt.Printf("Handshake successful with peer: %s\n", peerB32Addr)
	}

	// Wrap the connection with pp.PeerConn
	pc := pp.NewPeerConn(peerConn, peerId, mi.InfoHash())
	pc.Timeout = 120 * time.Second

	// Start the message handling loop
	err = handlePeerConnection(ctx, pc, dm)
	if err != nil {
		log.Printf("Peer connection error: %v", err)
	}
}

func handlePeerConnection(ctx context.Context, pc *pp.PeerConn, dm *download.DownloadManager) error {
	//log.WithField("peer", pc.RemoteAddr().String()).Debug()
	log := log.WithField("peer", pc.RemoteAddr().String())
	defer pc.Close()

	ps := &PeerState{} // Initialize per-peer state

	// Send BitField if needed
	err := pc.SendBitfield(dm.Bitfield)
	if err != nil {
		log.WithError(err).Error("Failed to send Bitfield")
		return fmt.Errorf("Failed to send Bitfield: %v", err)
	}

	// Send Interested message
	err = pc.SendInterested()
	if err != nil {
		log.WithError(err).Error("Failed to send Interested message")
		return fmt.Errorf("Failed to send Interested message: %v", err)
	}

	log.Info("Successfully initiated peer connection")

	for {
		select {
		case <-ctx.Done():
			log.Info("Peer connection cancelled")
			return nil
		default:
			msg, err := pc.ReadMsg()
			if err != nil {
				if err == io.EOF {
					log.Info("Peer connection closed")
					return nil
				}
				log.WithError(err).Error("Error reading message from peer")
				return err
			}

			log.WithField("message_type", msg.Type.String()).Debug("Received message from peer")

			err = handleMessage(pc, msg, dm, ps)
			if err != nil {
				log.WithError(err).Error("Error handling message from peer")
				return err
			}
		}
	}
}

func performHandshake(conn net.Conn, infoHash []byte, peerId string) error {
	log.WithFields(logrus.Fields{
		"peer_id":   peerId,
		"info_hash": fmt.Sprintf("%x", infoHash),
	}).Debug("Starting handshake with peer")
	// Build the handshake message
	pstr := "BitTorrent protocol"
	pstrlen := byte(len(pstr))
	reserved := make([]byte, 8)
	handshake := make([]byte, 49+len(pstr))
	handshake[0] = pstrlen
	copy(handshake[1:], pstr)
	copy(handshake[1+len(pstr):], reserved)
	copy(handshake[1+len(pstr)+8:], infoHash)
	copy(handshake[1+len(pstr)+8+20:], []byte(peerId))

	// Log the raw handshake message being sent
	log.WithField("handshake_bytes", fmt.Sprintf("%x", handshake[:])).Debug("Sending handshake")

	// Send handshake
	_, err := conn.Write(handshake)
	if err != nil {
		log.WithError(err).Error("Failed to send handshake")
		return fmt.Errorf("failed to send handshake: %v", err)
	}

	// Read handshake response
	response := make([]byte, 68)
	_, err = io.ReadFull(conn, response)
	if err != nil {
		log.WithError(err).Error("Failed to read handshake response")
		return fmt.Errorf("failed to read handshake: %v", err)
	}

	log.Debug("Successfully received handshake response")
	log.WithField("handshake_response_bytes", fmt.Sprintf("%x", response)).Debug("Received handshake response")
	// Validate response
	if response[0] != pstrlen {
		log.WithFields(logrus.Fields{
			"expected": pstrlen,
			"got":      response[0],
		}).Error("Invalid pstrlen in handshake")
		return fmt.Errorf("invalid pstrlen in handshake")
	}
	if string(response[1:1+len(pstr)]) != pstr {
		log.Error("Invalid protocol string in handshake")
		return fmt.Errorf("invalid protocol string in handshake")
	}
	// Optionally check reserved bytes
	// Extract info_hash and peer_id from response
	receivedInfoHash := response[1+len(pstr)+8 : 1+len(pstr)+8+20]
	if !bytes.Equal(receivedInfoHash, infoHash) {
		log.WithFields(logrus.Fields{
			"expected": fmt.Sprintf("%x", infoHash),
			"got":      fmt.Sprintf("%x", receivedInfoHash),
		}).Error("Info hash mismatch")
		return fmt.Errorf("info_hash does not match")
	}
	// Peer ID can be extracted if needed
	// receivedPeerId := response[1+len(pstr)+8+20:]
	log.Debug("Handshake completed successfully")
	return nil
}
