package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"github.com/sirupsen/logrus"
	"net"
	"sync"
	"time"

	//"github.com/eyedeekay/i2pkeys"
	//"github.com/eyedeekay/sam3"
	"github.com/go-i2p/go-i2p-bt/bencode"
	"github.com/go-i2p/go-i2p-bt/metainfo"
	pp "github.com/go-i2p/go-i2p-bt/peerprotocol"
	"github.com/go-i2p/sam3"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
)

var log = logrus.New()

func init() {
	// Configure logrus
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		DisableColors: false,
	})
	log.SetLevel(logrus.DebugLevel)
}

/* //Trackers
String convertedurl = url.replace("ahsplxkbhemefwvvml7qovzl5a2b5xo5i7lyai7ntdunvcyfdtna.b32.i2p", "tracker2.postman.i2p")
.replace("w7tpbzncbcocrqtwwm3nezhnnsw4ozadvi2hmvzdhrqzfxfum7wa.b32.i2p", "opentracker.dg2.i2p")
.replace("afuuortfaqejkesne272krqvmafn65mhls6nvcwv3t7l2ic2p4kq.b32.i2p", "lyoko.i2p")
.replace("s5ikrdyjwbcgxmqetxb3nyheizftms7euacuub2hic7defkh3xhq.b32.i2p", "tracker.thebland.i2p")
.replace("nfrjvknwcw47itotkzmk6mdlxmxfxsxhbhlr5ozhlsuavcogv4hq.b32.i2p", "torrfreedom.i2p")
.replace("http://", "");

*/
/*
p6zlufbvhcn426427wiaylzejdwg4hbdlrccst6owijhlvgalb7a.b32.i2p
cpfrxck5c4stxqrrjsp5syqvhfbtmc2jebyiyuv4hwjnhbxopuyq.b32.i2p
6p225yhqnr2t3kjdh5vy3h2bsv5unfip4777dqfk7qv2ihluf6va.b32.i2p+
cofho7nrtwu47mzejuwk6aszk7zj7aox6b5v2ybdhh5ykrz64jka.b32.i2p+

*/
var globalSAM *sam3.SAM
var globalStreamSession *sam3.StreamSession

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
	//requestPending bool
}

type peerState struct {
	requestPending bool
}

func generatePeerId() string {
	// Client identifier (8 bytes)
	clientId := "-GT0001-"

	// Generate 12 random bytes for uniqueness
	randomBytes := make([]byte, 12)
	_, err := rand.Read(randomBytes)
	if err != nil {
		log.Fatalf("Failed to generate peer ID: %v", err)
	}

	// Combine them into a 20-byte peer ID
	return clientId + string(randomBytes)
}

// Helper function to URL-encode binary data as per BitTorrent protocol
func urlEncodeBytes(b []byte) string {
	var buf strings.Builder
	for _, c := range b {
		if (c >= 'A' && c <= 'Z') ||
			(c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~' {
			buf.WriteByte(c)
		} else {
			buf.WriteString(fmt.Sprintf("%%%02X", c))
		}
	}
	return buf.String()
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
func cleanBase32Address(addr string) string {
	// Remove any trailing equals signs
	addr = strings.TrimRight(addr, "=")
	return addr + ".b32.i2p"
}
func generatePeerIdMeta() metainfo.Hash {
	var peerId metainfo.Hash
	_, err := rand.Read(peerId[:])
	if err != nil {
		log.Fatalf("Failed to generate peer ID: %v", err)
	}
	return peerId
}
func getPeersFromSimpTracker(mi *metainfo.MetaInfo) ([][]byte, error) {
	log.Info("Getting peers from simp tracker")
	sam, err := sam3.NewSAM("127.0.0.1:7656")
	if err != nil {
		return nil, err
	}
	//defer sam.Close()

	keys, err := sam.NewKeys()
	if err != nil {
		return nil, err
	}
	sessionName := fmt.Sprintf("getpeers-%d", os.Getpid())
	stream, err := sam.NewPrimarySessionWithSignature(sessionName, keys, sam3.Options_Default, strconv.Itoa(7))
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	simpAddr, err := sam.Lookup("wc4sciqgkceddn6twerzkfod6p2npm733p7z3zwsjfzhc4yulita.b32.i2p")
	if err != nil {
		return nil, err
	}

	// Create URL string
	ihEnc := mi.InfoHash().Bytes()
	pidEnc := generatePeerId()

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
	query.Set("left", "65536")
	query.Set("compact", "0")
	destination := urlEncodeBytes([]byte(keys.Addr().Base64()))
	destination += ".i2p"
	query.Set("ip", destination)
	query.Set("event", "started")
	announcePath := fmt.Sprintf("/a?%s", query.Encode())
	httpRequest := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: EXPERIMENTAL-SOFTWARE/0.0.0\r\nAccept-Encoding: identity\r\nConnection: close\r\n\r\n", announcePath, simpAddr.Base32())

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

func connectToPeer(peerHash []byte, index int, mi *metainfo.MetaInfo, dm *downloadManager) {
	/*
		fmt.Println("Generating resolver for peer")
		peerSAM, err := sam3.NewSAM("127.0.0.1:7656")
		if err != nil {
			log.Fatalf("Failed to create SAM connection: %v", err)
		}
		//defer peerSAM.Close()

		peerKeys, err := peerSAM.NewKeys()
		if err != nil {
			log.Fatalf("Failed to generate keys: %v", err)
		}

		// Create unique session name for each peer
		peerSessionName := fmt.Sprintf("peer-session-%d-%d", os.Getpid(), index)
		peerStream, err := peerSAM.NewPrimarySession(
			peerSessionName,
			peerKeys,
			sam3.Options_Default,
		)
		if err != nil {
			log.Fatalf("Failed to create SAM session: %v", err)
		}
		defer peerStream.Close()

	*/
	peerStream := globalStreamSession

	// Convert hash to Base32 address
	peerHashBase32 := strings.ToLower(base32.StdEncoding.EncodeToString(peerHash))
	peerB32Addr := cleanBase32Address(peerHashBase32)

	// Lookup the peer's Destination
	peerDest, err := globalSAM.Lookup(peerB32Addr)
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
	peerId := generatePeerIdMeta()
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
	err = handlePeerConnection(pc, dm)
	if err != nil {
		log.Printf("Peer connection error: %v", err)
	}
}
func handlePeerConnection(pc *pp.PeerConn, dm *downloadManager) error {
	//log.WithField("peer", pc.RemoteAddr().String()).Debug()
	log := log.WithField("peer", pc.RemoteAddr().String())
	defer pc.Close()

	ps := &peerState{} // Initialize per-peer state

	// Send BitField if needed
	err := pc.SendBitfield(dm.bitfield)
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
func handleMessage(pc *pp.PeerConn, msg pp.Message, dm *downloadManager, ps *peerState) error {
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
		if dm.needPiecesFrom(pc) {
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
		if dm.needPiecesFrom(pc) {
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
		if !ps.requestPending {
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

		ps.requestPending = false

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

func main() {
	// init download stats
	stats := newDownloadStats()

	//init global sam
	var err error
	globalSAM, err = sam3.NewSAM("127.0.0.1:7656")
	if err != nil {
		log.Fatalf("Failed to create global SAM session: %v", err)
	}
	defer globalSAM.Close()

	// Generate keys for the global session
	globalKeys, err := globalSAM.NewKeys()
	if err != nil {
		log.Fatalf("failed to generate keys for global SAM session: %v", err)
	}
	//set options
	options := []string{
		"inbound.length=1",
		"outbound.length=1",
		"inbound.quantity=3",
		"outbound.quantity=3",
		"inbound.backupQuantity=1",
		"outbound.backupQuantity=1",
		"inbound.lengthVariance=0",
		"outbound.lengthVariance=0",
	}

	//

	globalSessionName := fmt.Sprintf("global-session-%d", os.Getpid())
	globalStreamSession, err = globalSAM.NewStreamSessionWithSignature(
		globalSessionName,
		globalKeys,
		options,
		strconv.Itoa(7),
	)
	if err != nil {
		log.Fatalf("Failed to create global SAM stream session: %v", err)
	}
	defer globalStreamSession.Close()
	//http://tracker2.postman.i2p/announce.php
	//ahsplxkbhemefwvvml7qovzl5a2b5xo5i7lyai7ntdunvcyfdtna.b32.i2p <-> tracker2.postman.i2p
	// Load the torrent file
	mi, err := metainfo.LoadFromFile("torrent-i2pify+script.torrent")
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

	// Initialize the file writer
	/*
		outputFile := info.Name
		mode := os.FileMode(0644)
		file, err := os.Create(outputFile)
		if err != nil {
			log.Fatalf("Failed to create output file: %v", err)
		}
		defer file.Close()

	*/
	var outputPath string
	var mode os.FileMode
	if len(info.Files) == 0 {
		// Single-file torrent
		outputPath = info.Name
		mode = 0644
	} else {
		// Multi-file torrent
		outputPath = info.Name
		mode = 0755
		// Create the directory if it doesn't exist
		err := os.MkdirAll(outputPath, mode)
		if err != nil && !os.IsExist(err) {
			log.Fatalf("Failed to create output directory: %v", err)
		}
	}

	writer := metainfo.NewWriter(outputPath, info, mode)

	remainingData := info.TotalLength()
	var dm_plength int64
	if remainingData < info.PieceLength {
		dm_plength = remainingData
	} else {
		dm_plength = info.PieceLength
	}

	// Init downloadManager
	dm := &downloadManager{
		writer:   writer, // Assign the value, not a pointer
		bitfield: pp.NewBitField(len(info.Pieces)),
		left:     info.TotalLength(),
		pindex:   0,
		poffset:  0,
		plength:  dm_plength,
	}

	// Start progress monitoring goroutine
	progressTicker := time.NewTicker(5 * time.Second)
	go func() {
		for range progressTicker.C {
			dm.logProgress()
		}
	}()
	defer progressTicker.Stop()

	// Obtain peers from the tracker
	peers, err := getPeersFromSimpTracker(&mi)
	if err != nil {
		log.Fatalf("Failed to get peers from tracker: %v", err)
	}

	// Use a WaitGroup to wait for all peer connections to finish
	var wg sync.WaitGroup

	// Connect to peers
	for i, peerHash := range peers {
		wg.Add(1)
		go func(peerHash []byte, index int) {
			defer wg.Done()
			stats.connectionStarted()
			defer stats.connectionEnded()
			peerLog := log.WithFields(logrus.Fields{
				"peer_index": index,
				"peer_hash":  fmt.Sprintf("%x", peerHash),
			})
			peerLog.Info("Starting peer connection")
			connectToPeer(peerHash, index, &mi, dm)
			peerLog.Info("Peer connection completed")
		}(peerHash, i)
	}

	// Wait until all peer connections are done
	wg.Wait()

	// Check if the download is complete
	if dm.IsFinished() {
		log.WithFields(logrus.Fields{
			"total_downloaded": dm.downloaded,
			"elapsed_time":     time.Since(stats.startTime),
			"avg_speed_kBps":   float64(dm.downloaded) / time.Since(stats.startTime).Seconds() / 1024,
			"peak_speed_kBps":  stats.peakSpeed / 1024,
		}).Info("Download completed?")
	} else {
		fmt.Println("Download incomplete")
	}
	return
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

/*
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
*/
func requestNextBlock(pc *pp.PeerConn, dm *downloadManager, ps *peerState) error {
	log := log.WithField("peer", pc.RemoteAddr().String())

	for !dm.IsFinished() {
		if dm.plength <= 0 {
			dm.pindex++
			if dm.IsFinished() {
				log.Info("Download finished")
				return nil
			}

			dm.poffset = 0
			// Adjusted to use dm.writer.Info()
			/*
				pieceLength := dm.writer.Info().Piece(int(dm.pindex)).Length()
				dm.plength = pieceLength
				log.WithField("plength", dm.plength).Debug("Set dm.plength")

			*/
			remainingData := dm.writer.Info().TotalLength() - int64(dm.pindex)*dm.writer.Info().PieceLength
			if remainingData < dm.writer.Info().PieceLength {
				dm.plength = remainingData
			} else {
				dm.plength = dm.writer.Info().PieceLength
			}
			log.WithField("plength", dm.plength).Debug("Set dm.plength")
		}

		// Check if peer has the piece
		if !pc.BitField.IsSet(uint32(int(dm.pindex))) {
			// Try next piece
			log.WithField("piece_index", dm.pindex).Debug("Peer doesn't have requested piece, trying next")
			dm.pindex++
			dm.poffset = 0
			continue
		}

		// Calculate block size
		length := uint32(BlockSize)
		remaining := uint32(dm.plength) - dm.poffset
		if remaining < length {
			length = remaining
		}
		log.WithFields(logrus.Fields{
			"piece_index": dm.pindex,
			"offset":      dm.poffset,
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
		err := pc.SendRequest(dm.pindex, dm.poffset, length)
		if err == nil {
			dm.doing = true
			ps.requestPending = true
			//dm.requestPending = true
			//fmt.Printf("\rRequesting piece %d (%d/%d), offset %d, length %d",
			//dm.pindex, dm.pindex+1, dm.writer.Info().CountPieces(), dm.poffset, length)
			log.WithFields(logrus.Fields{
				"piece_index":  dm.pindex,
				"total_pieces": dm.writer.Info().CountPieces(),
				"offset":       dm.poffset,
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
func (dm *downloadManager) IsFinished() bool {
	finished := dm.pindex >= uint32(dm.writer.Info().CountPieces())
	log.WithFields(logrus.Fields{
		"current_index": dm.pindex,
		"total_pieces":  dm.writer.Info().CountPieces(),
		"is_finished":   finished,
	}).Debug("Checking if download is finished")
	return finished
}

func (dm *downloadManager) OnBlock(index, offset uint32, b []byte) error {
	log := log.WithFields(logrus.Fields{
		"piece_index":    index,
		"current_index":  dm.pindex,
		"offset":         offset,
		"current_offset": dm.poffset,
		"block_size":     len(b),
	})

	//dm.requestPending = false
	if dm.pindex != index {
		log.Error("Inconsistent piece index")
		return fmt.Errorf("inconsistent piece: old=%d, new=%d", dm.pindex, index)
	}
	if dm.poffset != offset {
		log.Error("Inconsistent offset")
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

		log.WithFields(logrus.Fields{
			"bytes_written":    n,
			"new_offset":       dm.poffset,
			"remaining_length": dm.plength,
			"total_downloaded": dm.downloaded,
			"remaining_bytes":  dm.left,
		}).Debug("Updated download progress")

		// Update bitfield for completed piece
		if dm.plength <= 0 {
			dm.bitfield.Set(index)
			log.WithFields(logrus.Fields{
				"piece_index":  index,
				"total_pieces": dm.writer.Info().CountPieces(),
				"progress":     fmt.Sprintf("%.2f%%", float64(index+1)/float64(dm.writer.Info().CountPieces())*100),
			}).Info("Completed piece")
		}
	} else {
		log.WithError(err).Error("Failed to write block")
	}
	return err
}
func (dm *downloadManager) needPiecesFrom(pc *pp.PeerConn) bool {
	log := log.WithField("peer", pc.RemoteAddr().String())

	for i := 0; i < dm.writer.Info().CountPieces(); i++ {
		if !dm.bitfield.IsSet(uint32(i)) && pc.BitField.IsSet(uint32(i)) {
			log.WithFields(logrus.Fields{
				"piece_index":    i,
				"have_piece":     dm.bitfield.IsSet(uint32(i)),
				"peer_has_piece": pc.BitField.IsSet(uint32(i)),
			}).Debug("Found needed piece from peer")
			return true
		}
	}
	log.Debug("No needed pieces from this peer")
	return false
}
func (dm *downloadManager) logProgress() {
	totalPieces := dm.writer.Info().CountPieces()
	completedPieces := 0
	for i := 0; i < totalPieces; i++ {
		if dm.bitfield.IsSet(uint32(i)) {
			completedPieces++
		}
	}

	log.WithFields(logrus.Fields{
		"completed_pieces": completedPieces,
		"total_pieces":     totalPieces,
		"progress":         fmt.Sprintf("%.2f%%", float64(completedPieces)/float64(totalPieces)*100),
		"downloaded_bytes": dm.downloaded,
		"total_bytes":      dm.writer.Info().TotalLength(),
		"remaining_bytes":  dm.left,
		"current_piece":    dm.pindex,
		"current_offset":   dm.poffset,
	}).Info("Download progress update")
}

// Add this struct to track download statistics
type downloadStats struct {
	startTime          time.Time
	lastProgressUpdate time.Time
	totalBytesReceived int64
	currentSpeed       float64
	peakSpeed          float64
	activeConnections  int
	mu                 sync.Mutex
}

func newDownloadStats() *downloadStats {
	return &downloadStats{
		startTime:          time.Now(),
		lastProgressUpdate: time.Now(),
	}
}

func (stats *downloadStats) updateProgress(bytesReceived int64) {
	stats.mu.Lock()
	defer stats.mu.Unlock()

	now := time.Now()
	duration := now.Sub(stats.lastProgressUpdate).Seconds()
	if duration > 0 {
		stats.currentSpeed = float64(bytesReceived) / duration
		if stats.currentSpeed > stats.peakSpeed {
			stats.peakSpeed = stats.currentSpeed
		}
	}

	stats.totalBytesReceived += bytesReceived
	stats.lastProgressUpdate = now

	log.WithFields(logrus.Fields{
		"current_speed_kBps": stats.currentSpeed / 1024,
		"peak_speed_kBps":    stats.peakSpeed / 1024,
		"total_received_MB":  float64(stats.totalBytesReceived) / 1024 / 1024,
		"elapsed_time":       now.Sub(stats.startTime).String(),
		"active_connections": stats.activeConnections,
	}).Info("Download statistics update")
}

func (stats *downloadStats) connectionStarted() {
	stats.mu.Lock()
	defer stats.mu.Unlock()
	stats.activeConnections++
	log.WithField("active_connections", stats.activeConnections).Debug("Peer connection started")
}

func (stats *downloadStats) connectionEnded() {
	stats.mu.Lock()
	defer stats.mu.Unlock()
	stats.activeConnections--
	log.WithField("active_connections", stats.activeConnections).Debug("Peer connection ended")
}
func example_postman_request() {
	fmt.Println("Generating new SAM")
	_sam, err := sam3.NewSAM("127.0.0.1:7656")
	if err != nil {
		panic(err)
	}
	fmt.Println("Generated new SAM")
	//	defer _sam.Close()

	keys, err := _sam.NewKeys()
	if err != nil {
		panic(err)
	}
	fmt.Println("Generated new keys")
	sessionName := fmt.Sprintf("postman-tracker-%d", os.Getpid())
	_stream, err := _sam.NewPrimarySession(sessionName, keys, sam3.Options_Default)
	fmt.Println("Generated new primary session")
	defer _stream.Close()

	//postmanAddr, err := _sam.Lookup("ahsplxkbhemefwvvml7qovzl5a2b5xo5i7lyai7ntdunvcyfdtna.b32.i2p")
	postmanAddr, err := _sam.Lookup("tracker2.postman.i2p")
	if err != nil {
		panic(err)
	}
	fmt.Printf("Connecting to %s", postmanAddr)

	requestStr := fmt.Sprintf("GET /announce.php?info_hash=%s&peer_id=%s&port=6881&uploaded=0&downloaded=0&left=0&compact=1&event=started HTTP/1.1\r\n"+
		"Host: tracker2.postman.i2p\r\n"+
		"User-Agent: EepTorrent/0.0.0\r\n"+
		"Accept: */*\r\n\r\n",
		url.QueryEscape("73D3CA92B5C927D2845D4A3BF67871EC866F11FA"),
		url.QueryEscape(generatePeerId()))

	conn, err := _stream.Dial("tcp", postmanAddr.String())
	if err != nil {
		panic(err)
	}

	conn.Write([]byte(requestStr))
	buffer := make([]byte, 4096)
	// Read the response into the buffer
	n, err := conn.Read(buffer)
	if err != nil {
		panic(err)
	}

	// Convert response to string and print it
	response := string(buffer[:n])
	fmt.Println(response)
}

func example_simp_request() {
	mi, err := metainfo.LoadFromFile("torrent-i2pify+script.torrent")
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

	//BEGIN RAW
	rawSAM, err := sam3.NewSAM("127.0.0.1:7656")
	if err != nil {
		panic(err)
	}
	defer rawSAM.Close()
	rawKeys, err := rawSAM.NewKeys()
	if err != nil {
		panic(err)
	}
	sessionName := fmt.Sprintf("postman-tracker-%d", os.Getpid())
	//rawStream, err := rawSAM.NewPrimarySession(sessionName, rawKeys, sam3.Options_Default)
	rawStream, err := rawSAM.NewPrimarySessionWithSignature(sessionName, rawKeys, sam3.Options_Default, strconv.Itoa(7))
	defer rawStream.Close()

	//postmanAddr, err := rawSAM.Lookup("tracker2.postman.i2p")
	postmanAddr, err := rawSAM.Lookup("wc4sciqgkceddn6twerzkfod6p2npm733p7z3zwsjfzhc4yulita.b32.i2p")
	if err != nil {
		panic(err)
	}

	//create url string
	//ihEnc := "73D3CA92B5C927D2845D4A3BF67871EC866F11FA"
	ihEnc := mi.InfoHash().Bytes()
	//ihEnc := mi.InfoHash().String()
	pidEnc := generatePeerId()
	query := url.Values{}
	query.Set("info_hash", string(ihEnc))
	query.Set("peer_id", pidEnc)
	query.Set("port", strconv.Itoa(6881))
	query.Set("uploaded", "0")
	query.Set("downloaded", "0")
	query.Set("left", "65536")
	query.Set("compact", "0")
	destination := urlEncodeBytes([]byte(rawKeys.Addr().Base64()))
	destination += ".i2p"
	//query.Set("ip", urlEncodeBytes([]byte(rawKeys.Addr().Base64())))
	query.Set("ip", destination)
	query.Set("event", "started")
	announcePath := fmt.Sprintf("/a?%s", query.Encode())
	httpRequest := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: EXPERIMENTAL-SOFTWARE/0.0.0\r\nAccept-Encoding: identity\r\nConnection: close\r\n\r\n", announcePath, postmanAddr.Base32())
	fmt.Printf("BEGIN HTTP REQUEST\n%s\nEND HTTP REQUEST\n", httpRequest)
	conn, err := rawStream.Dial("tcp", postmanAddr.String())
	if err != nil {
		panic(err)
	}
	conn.Write([]byte(httpRequest))
	// Read the response
	// Create a bytes.Buffer to store the complete response
	var responseBuffer bytes.Buffer
	buffer := make([]byte, 4096)

	// Read until no more data or error
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			if err == io.EOF {
				break
			}
			panic(err)
		}
		responseBuffer.Write(buffer[:n])
	}

	response := responseBuffer.String()
	fmt.Println(response)
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
