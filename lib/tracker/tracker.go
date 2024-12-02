package tracker

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
	"eeptorrent/lib/i2p"
	"eeptorrent/lib/util"
	"errors"
	"fmt"
	"github.com/go-i2p/go-i2p-bt/bencode"
	"github.com/go-i2p/go-i2p-bt/metainfo"
	"github.com/sirupsen/logrus"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

var GlobalTrackerManager *TrackerManager
var log = logrus.StandardLogger()

const USER_AGENT = "EepTorrent/0.0.0"

type TrackerConfig struct {
	Name        string // Human-readable name for logging
	TrackerAddr string // I2P address of the tracker
	Path        string // Announce path, e.g., "/a" or "/announce.php"
}

func init() {
	GlobalTrackerManager = NewTrackerManager()
}

// TrackerManager manages a dynamic list of trackers.
type TrackerManager struct {
	mu        sync.RWMutex
	trackers  []TrackerConfig
	fetchFunc func(context.Context, *metainfo.MetaInfo, TrackerConfig) ([][]byte, error)
}

// NewTrackerManager initializes the TrackerManager with built-in trackers.
func NewTrackerManager() *TrackerManager {
	builtInTrackers := []TrackerConfig{
		{
			Name:        "EepTorrent",
			TrackerAddr: "bjnkpy2rpwwlyjgmxeolt2cnp7h4oe437mtd54hb3pve3gmjqp5a.b32.i2p",
			Path:        "a",
		},
		{
			Name:        "Postman",
			TrackerAddr: "tracker2.postman.i2p",
			Path:        "announce.php",
		},
		{
			Name:        "Dg2",
			TrackerAddr: "opentracker.dg2.i2p",
			Path:        "announce.php",
		},
		{
			Name:        "Simp",
			TrackerAddr: "wc4sciqgkceddn6twerzkfod6p2npm733p7z3zwsjfzhc4yulita.b32.i2p",
			Path:        "a",
		},
		{
			Name:        "Skank",
			TrackerAddr: "opentracker.skank.i2p",
			Path:        "a",
		},
	}

	return &TrackerManager{
		trackers: builtInTrackers,
		fetchFunc: func(ctx context.Context, mi *metainfo.MetaInfo, tc TrackerConfig) ([][]byte, error) {
			return getPeersFromTracker(ctx, tc, mi)
		},
	}
}

// GetAllPeers retrieves peers from all registered trackers.
func (tm *TrackerManager) GetAllPeers(ctx context.Context, mi *metainfo.MetaInfo) ([][]byte, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	var allPeers [][]byte
	//timeout := time.Second * 15

	for _, getPeers := range tm.trackers {
		select {
		case <-ctx.Done():
			log.Infof("Peer fetching canceled for torrent due to context cancellation.")
			return nil, ctx.Err()
		default:
			peers, err := tm.fetchFunc(ctx, mi, getPeers)
			if err != nil {
				log.WithError(err).Warnf("Failed to get peers from tracker: %s", getPeers.Name)
			} else {
				allPeers = append(allPeers, peers...)
			}
			time.Sleep(1 * time.Second) // Throttle requests
		}
	}

	if len(allPeers) == 0 {
		return nil, errors.New("no peers found from any tracker")
	}
	return allPeers, nil
}

// AddTracker adds a new tracker to the list.
func (tm *TrackerManager) AddTracker(tc TrackerConfig) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.trackers = append(tm.trackers, tc)
}

// RemoveTracker removes a tracker by its name.
func (tm *TrackerManager) RemoveTracker(name string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	for i, tc := range tm.trackers {
		if tc.Name == name {
			tm.trackers = append(tm.trackers[:i], tm.trackers[i+1:]...)
			return nil
		}
	}
	return errors.New("tracker not found")
}

// GetTrackers returns a copy of the current tracker list.
func (tm *TrackerManager) GetTrackers() []TrackerConfig {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	trackersCopy := make([]TrackerConfig, len(tm.trackers))
	copy(trackersCopy, tm.trackers)
	return trackersCopy
}

func getPeersFromTracker(ctx context.Context, tc TrackerConfig, mi *metainfo.MetaInfo) ([][]byte, error) {
	log.Infof("Getting peers from %s tracker", tc.Name)
	sam := i2p.GlobalSAM
	keys := i2p.GlobalKeys
	stream := i2p.GlobalStreamSession
	addr, err := sam.Lookup(tc.TrackerAddr)
	if err != nil {
		log.WithError(err).Errorf("Failed to lookup %s tracker address", tc.Name)
		return nil, err
	}

	info, err := mi.Info()
	if err != nil {
		log.WithError(err).Error("Failed to parse metainfo")
		return nil, err
	}

	totalLength := info.TotalLength()
	ihEnc := mi.InfoHash().Bytes()
	pidEnc := util.GeneratePeerId()

	log.WithFields(logrus.Fields{
		"info_hash": fmt.Sprintf("%x", ihEnc),
		"peer_id":   pidEnc,
	}).Debug("Preparing tracker request")

	query := url.Values{}
	query.Set("info_hash", string(ihEnc))
	query.Set("peer_id", pidEnc)
	query.Set("port", strconv.Itoa(6881))
	query.Set("uploaded", "0")
	query.Set("downloaded", "0")
	query.Set("left", strconv.FormatInt(totalLength, 10))
	query.Set("compact", "1")
	destination := util.UrlEncodeBytes([]byte(keys.Addr().Base64()))
	destination += ".i2p"
	query.Set("ip", destination)
	query.Set("event", "started")

	announcePath := fmt.Sprintf("/%s?%s", tc.Path, query.Encode())
	httpRequest := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: %s\r\nAccept-Encoding: identity\r\nConnection: close\r\n\r\n", announcePath, addr.Base32(), USER_AGENT)

	log.WithField("request", httpRequest).Debug("Sending tracker request")

	// Establish a channel to receive the connection or error
	connCh := make(chan net.Conn, 1)
	errCh := make(chan error, 1)

	go func() {
		conn, err := stream.Dial("tcp", addr.String())
		if err != nil {
			errCh <- err
			return
		}
		connCh <- conn
	}()

	select {
	case <-ctx.Done():
		log.Errorf("Tracker connection to %s timed out", tc.Name)
		return nil, fmt.Errorf("tracker connection to %s timed out", tc.Name)
	case err := <-errCh:
		log.WithError(err).Errorf("Failed to connect to %s tracker", tc.Name)
		return nil, err
	case conn := <-connCh:
		defer conn.Close()

		// Set a deadline for the entire request based on context
		if deadline, ok := ctx.Deadline(); ok {
			conn.SetDeadline(deadline)
		} else {
			// Set a default timeout if context has no deadline
			conn.SetDeadline(time.Now().Add(30 * time.Second))
		}

		// Send the HTTP request
		_, err = conn.Write([]byte(httpRequest))
		if err != nil {
			log.WithError(err).Error("Failed to send request to tracker")
			return nil, err
		}

		var responseBuffer bytes.Buffer
		buffer := make([]byte, 4096)

	outerLoop:
		for {
			select {
			case <-ctx.Done():
				log.Errorf("Reading from tracker %s timed out", tc.Name)
				return nil, fmt.Errorf("reading from tracker %s timed out", tc.Name)
			default:
				conn.SetReadDeadline(time.Now().Add(5 * time.Second)) // Adjust read timeout as needed
				n, err := conn.Read(buffer)
				if err != nil {
					if err == io.EOF {
						break outerLoop // Exit the for loop
					}
					log.WithError(err).Error("Error reading tracker response")
					return nil, err
				}
				responseBuffer.Write(buffer[:n])
			}
		}

		// This code is now reachable
		response := responseBuffer.String()
		log.WithField("response", response).Debug("Received tracker response")
		headerEnd := strings.Index(response, "\r\n\r\n")
		if headerEnd == -1 {
			log.Error("Invalid HTTP response: no header-body separator found")
			return nil, fmt.Errorf("invalid HTTP response: no header-body separator found")
		}
		body := response[headerEnd+4:]
		var trackerResp map[string]interface{}
		err = bencode.DecodeBytes([]byte(body), &trackerResp)
		if err != nil {
			log.WithError(err).Error("Failed to parse tracker response")
			return nil, fmt.Errorf("failed to parse tracker response: %v", err)
		}

		// Extract 'peers' key
		peersValue, ok := trackerResp["peers"]
		if !ok {
			log.Error("No 'peers' key in tracker response")
			return nil, fmt.Errorf("no 'peers' key in tracker response")
		}

		// Handle compact peers
		peersStr, ok := peersValue.(string)
		if !ok {
			log.Error("'peers' is not a string")
			return nil, fmt.Errorf("'peers' is not a string")
		}

		peersBytes := []byte(peersStr)

		if len(peersBytes)%32 != 0 {
			log.WithField("length", len(peersBytes)).Error("Peers string length is not a multiple of 32")
			return nil, fmt.Errorf("peers string length is not a multiple of 32")
		}

		peerHashes := [][]byte{}
		for i := 0; i < len(peersBytes); i += 32 {
			peerHash := peersBytes[i : i+32]
			peerHashes = append(peerHashes, peerHash)
		}
		log.WithField("peer_count", len(peerHashes)).Info("Successfully retrieved peers from tracker")
		return peerHashes, nil
	}
}

func GetPeersFromEepTorrentTracker(ctx context.Context, mi *metainfo.MetaInfo, timeout time.Duration) ([][]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	log.Info("Getting peers from EepTorrent tracker")
	config := TrackerConfig{
		Name:        "EepTorrent",
		TrackerAddr: "bjnkpy2rpwwlyjgmxeolt2cnp7h4oe437mtd54hb3pve3gmjqp5a.b32.i2p",
		//TrackerAddr: "opentracker.eeptorrent.i2p",
		Path: "a",
	}
	return getPeersFromTracker(ctx, config, mi)
}

func GetPeersFromPostmanTracker(ctx context.Context, mi *metainfo.MetaInfo, timeout time.Duration) ([][]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	log.Info("Getting peers from postman tracker")
	config := TrackerConfig{
		Name:        "Dg2",
		TrackerAddr: "tracker2.postman.i2p",
		Path:        "announce.php",
	}
	return getPeersFromTracker(ctx, config, mi)
}

func GetPeersFromSimpTracker(ctx context.Context, mi *metainfo.MetaInfo, timeout time.Duration) ([][]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	log.Info("Getting peers from Simp tracker")
	config := TrackerConfig{
		Name:        "Simp",
		TrackerAddr: "wc4sciqgkceddn6twerzkfod6p2npm733p7z3zwsjfzhc4yulita.b32.i2p",
		Path:        "a",
	}
	return getPeersFromTracker(ctx, config, mi)
}

func GetPeersFromDg2Tracker(ctx context.Context, mi *metainfo.MetaInfo, timeout time.Duration) ([][]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	log.Info("Getting peers from Dg2 tracker")
	config := TrackerConfig{
		Name:        "Dg2",
		TrackerAddr: "opentracker.dg2.i2p",
		Path:        "announce.php",
	}
	return getPeersFromTracker(ctx, config, mi)
}

func GetPeersFromSkankTracker(ctx context.Context, mi *metainfo.MetaInfo, timeout time.Duration) ([][]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	log.Info("Getting peers from Skank tracker")
	config := TrackerConfig{
		Name:        "Skank",
		TrackerAddr: "opentracker.skank.i2p",
		Path:        "a",
	}
	return getPeersFromTracker(ctx, config, mi)
}

func GetPeersFromOmitTracker(ctx context.Context, mi *metainfo.MetaInfo, timeout time.Duration) ([][]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	log.Info("Getting peers from Omit tracker")
	config := TrackerConfig{
		Name:        "Omit",
		TrackerAddr: "a5ruhsktpdhfk5w46i6yf6oqovgdlyzty7ku6t5yrrpf4qedznjq.b32.i2p",
		Path:        "announce.php",
	}
	return getPeersFromTracker(ctx, config, mi)
}

func GetPeersFrom6kw6Tracker(ctx context.Context, mi *metainfo.MetaInfo, timeout time.Duration) ([][]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	log.Info("Getting peers from 6kw6 tracker")
	config := TrackerConfig{
		Name:        "6kw6",
		TrackerAddr: "6kw6voy3v5jzmkbg4i3rlqjysre4msgarpkpme6mt5u2jw33nffa.b32.i2p",
		Path:        "announce",
	}
	return getPeersFromTracker(ctx, config, mi)
}

// getAllPeers retrieves peers from multiple trackers.
func GetAllPeers(ctx context.Context, mi *metainfo.MetaInfo) ([][]byte, error) {
	var allPeers [][]byte
	timeout := time.Second * 15

	// List of tracker functions
	trackerFuncs := []func(context.Context, *metainfo.MetaInfo, time.Duration) ([][]byte, error){
		GetPeersFromEepTorrentTracker,
		GetPeersFromPostmanTracker,
		GetPeersFromSimpTracker,
		GetPeersFromDg2Tracker,
		GetPeersFromSkankTracker,
		GetPeersFromOmitTracker,
		GetPeersFrom6kw6Tracker,
	}

	for _, getPeers := range trackerFuncs {
		select {
		case <-ctx.Done():
			log.Infof("Peer fetching canceled for torrent due to context cancellation.")
			return nil, ctx.Err()
		default:
			peers, err := getPeers(ctx, mi, timeout)
			if err != nil {
				log.WithError(err).Warn("Failed to get peers from tracker")
			} else {
				allPeers = append(allPeers, peers...)
			}
			time.Sleep(1 * time.Second) // Throttle requests
		}
	}

	if len(allPeers) == 0 {
		return nil, fmt.Errorf("No peers found")
	}
	return allPeers, nil
}

func checkPostmanTrackerResponse(response string) error {
	// Check for specific error conditions
	if strings.Contains(response, "Request denied!") {
		return fmt.Errorf("tracker request denied: blocked request")
	}

	// Check for HTML response when we expected bencode
	if strings.Contains(response, "<!DOCTYPE html>") ||
		strings.Contains(response, "<html>") {
		// Extract title if present
		titleStart := strings.Index(response, "<title>")
		titleEnd := strings.Index(response, "</title>")
		if titleStart != -1 && titleEnd != -1 {
			title := response[titleStart+7 : titleEnd]
			return fmt.Errorf("received HTML response instead of bencode: %s", title)
		}
		return fmt.Errorf("received HTML response instead of bencode")
	}

	// Check if response starts with 'd' (valid bencoded dict)
	if !strings.HasPrefix(strings.TrimSpace(response), "d") {
		return fmt.Errorf("invalid tracker response format: expected bencoded dictionary")
	}

	return nil
}
