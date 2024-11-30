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
	"bytes"
	"context"
	"eeptorrent/lib/download"
	"eeptorrent/lib/i2p"
	"eeptorrent/lib/util"
	"encoding/base32"
	"fmt"
	"github.com/go-i2p/go-i2p-bt/metainfo"
	pp "github.com/go-i2p/go-i2p-bt/peerprotocol"
	"github.com/go-i2p/i2pkeys"
	"github.com/sirupsen/logrus"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"time"
)

var log = logrus.StandardLogger()

func ConnectToPeer(ctx context.Context, peerHash []byte, index int, mi *metainfo.MetaInfo, dm *download.DownloadManager, pm *PeerManager) error {
	log.WithFields(logrus.Fields{
		"peer_index": index,
		"peer_hash":  fmt.Sprintf("%x", peerHash),
	}).Debug("Attempting to connect to peer")

	// Create a context with timeout for the entire connection process
	connCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	peerStream := i2p.GlobalStreamSession

	// Convert hash to Base32 address
	peerHashBase32 := strings.ToLower(base32.StdEncoding.EncodeToString(peerHash))
	peerB32Addr := util.CleanBase32Address(peerHashBase32)

	// Lookup the peer's Destination
	var peerDest i2pkeys.I2PAddr
	lookupDone := make(chan error, 1)
	go func() {
		var err error
		peerDest, err = i2p.GlobalSAM.Lookup(peerB32Addr)
		lookupDone <- err
	}()

	select {
	case <-connCtx.Done():
		log.Errorf("Lookup timed out for peer %s", peerB32Addr)
		return fmt.Errorf("lookup timed out for peer %s", peerB32Addr)
	case err := <-lookupDone:
		if err != nil {
			log.Errorf("Failed to lookup peer %s: %v", peerB32Addr, err)
			return fmt.Errorf("failed to lookup peer %s: %v", peerB32Addr, err)
		} else {
			log.Infof("Successfully looked up peer %s", peerB32Addr)
		}
	}

	// Attempt to connect with timeout
	connCh := make(chan net.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		conn, err := peerStream.DialContext(connCtx, "tcp", peerDest.Base64())
		if err != nil {
			errCh <- err
			return
		}
		connCh <- conn
	}()

	var peerConn net.Conn
	select {
	case <-connCtx.Done():
		log.Errorf("Connection timed out to peer %s", peerB32Addr)
		return fmt.Errorf("connection timed out to peer %s", peerB32Addr)
	case err := <-errCh:
		log.Errorf("Failed to connect to peer %s: %v", peerB32Addr, err)
		return fmt.Errorf("failed to connect to peer %s: %v", peerB32Addr, err)
	case peerConn = <-connCh:
		log.Infof("Successfully connected to peer %s", peerB32Addr)
	}

	defer peerConn.Close()

	// Set read/write deadlines on the connection
	deadline := time.Now().Add(30 * time.Second)
	err := peerConn.SetDeadline(deadline)
	if err != nil {
		log.Errorf("Failed to set deadline: %v", err)
		return fmt.Errorf("failed to set deadline: %v", err)
	}

	// Perform the BitTorrent handshake
	peerId := util.GeneratePeerIdMeta()
	err = PerformHandshake(peerConn, mi.InfoHash().Bytes(), string(peerId[:]))
	if err != nil {
		log.Errorf("Handshake with peer %s failed: %v", peerB32Addr, err)
		return fmt.Errorf("failed to handshake with peer %s: %v", peerB32Addr, err)
	} else {
		log.Infof("Handshake successful with peer: %s", peerB32Addr)
	}

	// Wrap the connection with pp.PeerConn
	pc := pp.NewPeerConn(peerConn, peerId, mi.InfoHash())
	pc.Timeout = 30 * time.Second

	// Start the message handling loop
	err = HandlePeerConnection(ctx, pc, dm, pm)
	if err != nil {
		log.Errorf("Peer connection error: %v", err)
		return fmt.Errorf("peer connection error: %v", err)
	}
	return nil
}

func HandlePeerConnection(ctx context.Context, pc *pp.PeerConn, dm *download.DownloadManager, pm *PeerManager) error {
	// Create a done channel for cleanup
	done := make(chan struct{})
	//defer close(done)

	ps := NewPeerState()

	dm.AddPeer(pc)
	pm.AddPeer(pc)
	defer func() {
		dm.RemovePeer(pc)
		pm.RemovePeer(pc)
	}()

	err := pc.SendBitfield(dm.Bitfield)
	if err != nil {
		log.WithError(err).Error("Failed to send Bitfield")
		return fmt.Errorf("failed to send Bitfield: %v", err)
	}

	err = pc.SendInterested()
	if err != nil {
		log.WithError(err).Error("Failed to send Interested message")
		return fmt.Errorf("failed to send Interested message: %v", err)
	}

	log.Info("Successfully initiated peer connection")

	go func() {
		defer func() {
			pc.Close()
			select {
			case done <- struct{}{}:
			default:

			}
		}()

		for {
			select {
			case <-ctx.Done():
				return
			default:
				err := pc.Conn.SetReadDeadline(time.Now().Add(1 * time.Second))
				if err != nil {
					log.WithError(err).Error("Failed to set read deadline")
					return
				}

				msg, err := pc.ReadMsg()
				if err != nil {
					if ne, ok := err.(net.Error); ok && ne.Timeout() {
						// Just a timeout, continue reading
						continue
					}
					if err == io.EOF {
						log.Info("Peer connection closed by peer")
						return
					}
					log.WithError(err).Error("Error reading message from peer")
					return
				}

				log.WithField("message_type", msg.Type.String()).Debug("Received message from peer")

				if err := handleMessage(pc, msg, dm, ps, pm); err != nil {
					log.WithError(err).Error("Error handling message from peer")
					return
				}
			}
		}
	}()

	// Periodically check seeding status
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if dm.IsFinished() {
					pm.HandleSeeding()
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Wait for either context cancellation or connection completion
	select {
	case <-ctx.Done():
		log.Info("Peer connection cancelled")
		pc.Close()
		return nil
	case <-done:
		return nil
	}
}
func reRequestPendingBlocks(pc *pp.PeerConn, dm *download.DownloadManager, ps *PeerState) {
	ps.Lock()
	defer ps.Unlock()

	dm.Mu.Lock()
	defer dm.Mu.Unlock()

	for pieceIndex, blocks := range ps.RequestedBlocks {
		if int(pieceIndex) >= len(dm.Pieces) {
			continue
		}
		piece := dm.Pieces[pieceIndex]
		if piece == nil {
			continue
		}
		piece.Mu.Lock()
		for offset := range blocks {
			// Mark block as not requested
			// So it can be requested from other peers
			delete(ps.RequestedBlocks[pieceIndex], offset)
			// Also, decrement the pending requests
			atomic.AddInt32(&ps.PendingRequests, -1)
		}
		piece.Mu.Unlock()
	}
}
func PerformHandshake(conn net.Conn, infoHash []byte, peerId string) error {
	// Set deadline
	deadline := time.Now().Add(15 * time.Second)
	err := conn.SetDeadline(deadline)
	if err != nil {
		return fmt.Errorf("failed to set deadline: %v", err)
	}
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
	_, err = conn.Write(handshake)
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

func RemoveDuplicatePeers(peers [][]byte) [][]byte {
	peerSet := make(map[string]struct{})
	uniquePeers := make([][]byte, 0, len(peers))

	for _, peer := range peers {
		peerStr := string(peer)
		if _, exists := peerSet[peerStr]; !exists {
			peerSet[peerStr] = struct{}{}
			uniquePeers = append(uniquePeers, peer)
		}
	}
	return uniquePeers
}
