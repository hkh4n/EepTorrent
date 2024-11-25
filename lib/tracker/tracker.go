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
	"eeptorrent/lib/i2p"
	"eeptorrent/lib/util"
	"fmt"
	"github.com/go-i2p/go-i2p-bt/bencode"
	"github.com/go-i2p/go-i2p-bt/metainfo"
	"github.com/sirupsen/logrus"
	"io"
	"net/url"
	"strconv"
	"strings"
)

var log = logrus.StandardLogger()

func GetPeersFromSkankTracker(mi *metainfo.MetaInfo) ([][]byte, error) {
	log.Info("Getting peers from postman tracker")
	sam := i2p.GlobalSAM
	keys, err := i2p.GlobalSAM.NewKeys()
	if err != nil {
		log.WithError(err).Error("Failed to generate SAM keys")
		return nil, err
	}
	stream := i2p.GlobalStreamSession
	postmanAddr, err := sam.Lookup("opentracker.skank.i2p")
	if err != nil {
		log.WithError(err).Error("Failed to lookup postman tracker address")
		return nil, err
	}
	info, err := mi.Info()
	if err != nil {
		log.WithError(err).Error("Failed to parse metainfo")
		return nil, err
	}
	totalLength := info.TotalLength()
	// Create URL string
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
	announcePath := fmt.Sprintf("/a?%s", query.Encode())
	httpRequest := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: EepTorrent/0.0.0\r\nAccept-Encoding: identity\r\nConnection: close\r\n\r\n", announcePath, postmanAddr.Base32())

	log.WithField("request", httpRequest).Debug("Sending tracker request")

	conn, err := stream.Dial("tcp", postmanAddr.String())
	if err != nil {
		log.WithError(err).Error("Failed to connect to tracker")
		return nil, err
	}
	defer conn.Close()
	// Send the HTTP request
	_, err = conn.Write([]byte(httpRequest))
	if err != nil {
		log.WithError(err).Error("Failed to send request to tracker")
		return nil, err
	}
	var responseBuffer bytes.Buffer
	buffer := make([]byte, 4096)

	// Read until no more data or error
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			if err == io.EOF {
				break
			}
			log.WithError(err).Error("Error reading tracker response")
			return nil, err
		}
		responseBuffer.Write(buffer[:n])
	}
	response := responseBuffer.String()
	log.WithField("response", response).Debug("Received tracker response")
	headerEnd := strings.Index(response, "\r\n\r\n")
	if headerEnd == -1 {
		log.Error("Invalid HTTP response: no header-body separator found")
		return nil, fmt.Errorf("Invalid HTTP response: no header-body separator found")
	}
	body := response[headerEnd+4:]
	var trackerResp map[string]interface{}
	err = bencode.DecodeBytes([]byte(body), &trackerResp)
	if err != nil {
		log.WithError(err).Error("Failed to parse tracker response")
		return nil, fmt.Errorf("Failed to parse tracker response: %v", err)
	}
	// Extract 'peers' key
	peersValue, ok := trackerResp["peers"]
	if !ok {
		log.Error("No 'peers' key in tracker response")
		return nil, fmt.Errorf("No 'peers' key in tracker response")
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
		return nil, fmt.Errorf("Peers string length is not a multiple of 32")
	}

	peerHashes := [][]byte{}
	for i := 0; i < len(peersBytes); i += 32 {
		peerHash := peersBytes[i : i+32]
		peerHashes = append(peerHashes, peerHash)
	}
	log.WithField("peer_count", len(peerHashes)).Info("Successfully retrieved peers from tracker")
	return peerHashes, nil
}

func GetPeersFromDg2Tracker(mi *metainfo.MetaInfo) ([][]byte, error) {
	log.Info("Getting peers from postman tracker")
	sam := i2p.GlobalSAM
	keys, err := i2p.GlobalSAM.NewKeys()
	if err != nil {
		log.WithError(err).Error("Failed to generate SAM keys")
		return nil, err
	}
	stream := i2p.GlobalStreamSession
	postmanAddr, err := sam.Lookup("opentracker.dg2.i2p")
	if err != nil {
		log.WithError(err).Error("Failed to lookup postman tracker address")
		return nil, err
	}
	info, err := mi.Info()
	if err != nil {
		log.WithError(err).Error("Failed to parse metainfo")
		return nil, err
	}
	totalLength := info.TotalLength()
	// Create URL string
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
	announcePath := fmt.Sprintf("/announce.php?%s", query.Encode())
	httpRequest := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: EepTorrent/0.0.0\r\nAccept-Encoding: identity\r\nConnection: close\r\n\r\n", announcePath, postmanAddr.Base32())

	log.WithField("request", httpRequest).Debug("Sending tracker request")

	conn, err := stream.Dial("tcp", postmanAddr.String())
	if err != nil {
		log.WithError(err).Error("Failed to connect to tracker")
		return nil, err
	}
	defer conn.Close()
	// Send the HTTP request
	_, err = conn.Write([]byte(httpRequest))
	if err != nil {
		log.WithError(err).Error("Failed to send request to tracker")
		return nil, err
	}
	var responseBuffer bytes.Buffer
	buffer := make([]byte, 4096)

	// Read until no more data or error
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			if err == io.EOF {
				break
			}
			log.WithError(err).Error("Error reading tracker response")
			return nil, err
		}
		responseBuffer.Write(buffer[:n])
	}
	response := responseBuffer.String()
	log.WithField("response", response).Debug("Received tracker response")
	headerEnd := strings.Index(response, "\r\n\r\n")
	if headerEnd == -1 {
		log.Error("Invalid HTTP response: no header-body separator found")
		return nil, fmt.Errorf("Invalid HTTP response: no header-body separator found")
	}
	body := response[headerEnd+4:]
	var trackerResp map[string]interface{}
	err = bencode.DecodeBytes([]byte(body), &trackerResp)
	if err != nil {
		log.WithError(err).Error("Failed to parse tracker response")
		return nil, fmt.Errorf("Failed to parse tracker response: %v", err)
	}
	// Extract 'peers' key
	peersValue, ok := trackerResp["peers"]
	if !ok {
		log.Error("No 'peers' key in tracker response")
		return nil, fmt.Errorf("No 'peers' key in tracker response")
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
		return nil, fmt.Errorf("Peers string length is not a multiple of 32")
	}

	peerHashes := [][]byte{}
	for i := 0; i < len(peersBytes); i += 32 {
		peerHash := peersBytes[i : i+32]
		peerHashes = append(peerHashes, peerHash)
	}
	log.WithField("peer_count", len(peerHashes)).Info("Successfully retrieved peers from tracker")
	return peerHashes, nil
}

func GetPeersFromPostmanTracker(mi *metainfo.MetaInfo) ([][]byte, error) {
	log.Info("Getting peers from postman tracker")
	sam := i2p.GlobalSAM
	keys, err := i2p.GlobalSAM.NewKeys()
	if err != nil {
		log.WithError(err).Error("Failed to generate SAM keys")
		return nil, err
	}
	stream := i2p.GlobalStreamSession
	postmanAddr, err := sam.Lookup("tracker2.postman.i2p")
	if err != nil {
		log.WithError(err).Error("Failed to lookup postman tracker address")
		return nil, err
	}
	info, err := mi.Info()
	if err != nil {
		log.WithError(err).Error("Failed to parse metainfo")
		return nil, err
	}
	totalLength := info.TotalLength()
	// Create URL string
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
	announcePath := fmt.Sprintf("/announce.php?%s", query.Encode())
	httpRequest := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: EepTorrent/0.0.0\r\nAccept-Encoding: identity\r\nConnection: close\r\n\r\n", announcePath, postmanAddr.Base32())

	log.WithField("request", httpRequest).Debug("Sending tracker request")

	conn, err := stream.Dial("tcp", postmanAddr.String())
	if err != nil {
		log.WithError(err).Error("Failed to connect to tracker")
		return nil, err
	}
	defer conn.Close()
	// Send the HTTP request
	_, err = conn.Write([]byte(httpRequest))
	if err != nil {
		log.WithError(err).Error("Failed to send request to tracker")
		return nil, err
	}
	var responseBuffer bytes.Buffer
	buffer := make([]byte, 4096)

	// Read until no more data or error
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			if err == io.EOF {
				break
			}
			log.WithError(err).Error("Error reading tracker response")
			return nil, err
		}
		responseBuffer.Write(buffer[:n])
	}
	response := responseBuffer.String()
	log.WithField("response", response).Debug("Received tracker response")
	headerEnd := strings.Index(response, "\r\n\r\n")
	if headerEnd == -1 {
		log.Error("Invalid HTTP response: no header-body separator found")
		return nil, fmt.Errorf("Invalid HTTP response: no header-body separator found")
	}
	body := response[headerEnd+4:]
	var trackerResp map[string]interface{}
	err = bencode.DecodeBytes([]byte(body), &trackerResp)
	if err != nil {
		log.WithError(err).Error("Failed to parse tracker response")
		return nil, fmt.Errorf("Failed to parse tracker response: %v", err)
	}
	// Extract 'peers' key
	peersValue, ok := trackerResp["peers"]
	if !ok {
		log.Error("No 'peers' key in tracker response")
		return nil, fmt.Errorf("No 'peers' key in tracker response")
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
		return nil, fmt.Errorf("Peers string length is not a multiple of 32")
	}

	peerHashes := [][]byte{}
	for i := 0; i < len(peersBytes); i += 32 {
		peerHash := peersBytes[i : i+32]
		peerHashes = append(peerHashes, peerHash)
	}
	log.WithField("peer_count", len(peerHashes)).Info("Successfully retrieved peers from tracker")
	return peerHashes, nil
}

func GetPeersFromSimpTracker(mi *metainfo.MetaInfo) ([][]byte, error) {
	log.Info("Getting peers from simp tracker")
	sam := i2p.GlobalSAM

	keys, err := i2p.GlobalSAM.NewKeys()
	if err != nil {
		log.WithError(err).Error("Failed to generate SAM keys")
		return nil, err
	}
	stream := i2p.GlobalStreamSession

	simpAddr, err := sam.Lookup("wc4sciqgkceddn6twerzkfod6p2npm733p7z3zwsjfzhc4yulita.b32.i2p")
	if err != nil {
		log.WithError(err).Error("Failed to lookup simp tracker address")
		return nil, err
	}
	info, err := mi.Info()
	if err != nil {
		log.WithError(err).Error("Failed to parse metainfo")
		return nil, err
	}
	totalLength := info.TotalLength()
	// Create URL string
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
	announcePath := fmt.Sprintf("/a?%s", query.Encode())
	httpRequest := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: EepTorrent/0.0.0\r\nAccept-Encoding: identity\r\nConnection: close\r\n\r\n", announcePath, simpAddr.Base32())

	log.WithField("request", httpRequest).Debug("Sending tracker request")

	conn, err := stream.Dial("tcp", simpAddr.String())
	if err != nil {
		log.WithError(err).Error("Failed to connect to tracker")
		return nil, err
	}
	defer conn.Close()

	// Send the HTTP request
	_, err = conn.Write([]byte(httpRequest))
	if err != nil {
		log.WithError(err).Error("Failed to send request to tracker")
		return nil, err
	}
	var responseBuffer bytes.Buffer
	buffer := make([]byte, 4096)

	// Read until no more data or error
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			if err == io.EOF {
				break
			}
			log.WithError(err).Error("Error reading tracker response")
			return nil, err
		}
		responseBuffer.Write(buffer[:n])
	}
	response := responseBuffer.String()
	log.WithField("response", response).Debug("Received tracker response")
	headerEnd := strings.Index(response, "\r\n\r\n")
	if headerEnd == -1 {
		log.Error("Invalid HTTP response: no header-body separator found")
		return nil, fmt.Errorf("Invalid HTTP response: no header-body separator found")
	}
	body := response[headerEnd+4:]
	var trackerResp map[string]interface{}
	err = bencode.DecodeBytes([]byte(body), &trackerResp)
	if err != nil {
		log.WithError(err).Error("Failed to parse tracker response")
		return nil, fmt.Errorf("Failed to parse tracker response: %v", err)
	}
	// Extract 'peers' key
	peersValue, ok := trackerResp["peers"]
	if !ok {
		log.Error("No 'peers' key in tracker response")
		return nil, fmt.Errorf("No 'peers' key in tracker response")
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
		return nil, fmt.Errorf("Peers string length is not a multiple of 32")
	}

	peerHashes := [][]byte{}
	for i := 0; i < len(peersBytes); i += 32 {
		peerHash := peersBytes[i : i+32]
		peerHashes = append(peerHashes, peerHash)
	}
	log.WithField("peer_count", len(peerHashes)).Info("Successfully retrieved peers from tracker")
	return peerHashes, nil
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
