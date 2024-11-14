package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"

	//"github.com/eyedeekay/i2pkeys"
	//"github.com/eyedeekay/sam3"
	"github.com/go-i2p/go-i2p-bt/bencode"
	"github.com/go-i2p/i2pkeys"
	"github.com/go-i2p/sam3"
	"io"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-i2p/go-i2p-bt/metainfo"
	pp "github.com/go-i2p/go-i2p-bt/peerprotocol"
)

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

// Global SAM client for I2P connections
var sam *sam3.SAM
var localKeys *i2pkeys.I2PKeys
var stream *sam3.StreamSession

func dialI2P(network string, addr string) (net.Conn, error) {
	fmt.Println("dialI2P called")
	// Extract the I2P destination from the address

	// Lookup the destination
	fmt.Printf("looking up %s\n", addr)
	destkeys, err := sam.Lookup(addr)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup I2P destination: %v", err)
	}

	// Dial through I2P
	conn, err := stream.DialI2P(destkeys)
	if err != nil {
		return nil, fmt.Errorf("failed to dial I2P destination: %v", err)
	}

	return conn, nil
}

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

// Announce to the tracker without using HTTP proxy
func announceOverI2P(infohash, peerId metainfo.Hash, destination string, trackerDest string, port int) ([]string, error) {
	// Establish a TCP connection via I2P
	conn, err := dialI2P("tcp", trackerDest)
	if err != nil {
		return nil, fmt.Errorf("failed to dial tracker via I2P: %v", err)
	}
	defer conn.Close()

	// Construct the announce URL path with query parameters
	ihEnc := urlEncodeBytes(infohash[:])
	pidEnc := urlEncodeBytes(peerId[:])

	query := url.Values{}
	query.Set("info_hash", ihEnc)
	query.Set("peer_id", pidEnc)
	query.Set("port", strconv.Itoa(port))
	query.Set("uploaded", "0")
	query.Set("downloaded", "0")
	//query.Set("left", strconv.FormatUint(uint64(infohash.CountPieces()), 10))
	query.Set("left", "0")
	query.Set("compact", "1")
	query.Set("ip", urlEncodeBytes([]byte(destination))) // Replace with your I2P Destination
	query.Set("event", "started")

	announcePath := fmt.Sprintf("/announce.php?%s", query.Encode())

	// Construct the HTTP GET request
	httpRequest := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: EepTorrent/0.0.0\r\nAccept-Encoding: identity\r\nConnection: close\r\n\r\n", announcePath, trackerDest)

	// Send the HTTP GET request
	_, err = conn.Write([]byte(httpRequest))
	if err != nil {
		return nil, fmt.Errorf("failed to send announce request: %v", err)
	}

	// Read the HTTP response
	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read status line: %v", err)
	}

	if !strings.Contains(statusLine, "200 OK") {
		return nil, fmt.Errorf("tracker responded with non-OK status: %s", statusLine)
	}

	// Read headers
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("failed to read headers: %v", err)
		}
		if line == "\r\n" {
			break // End of headers
		}
	}

	// Read body
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	// Parse bencoded response
	var trackerResp map[string]interface{}
	if err := bencode.DecodeBytes(body, &trackerResp); err != nil {
		return nil, fmt.Errorf("failed to decode tracker response: %v", err)
	}

	// Extract peers
	peers, ok := trackerResp["peers"]
	if !ok {
		return nil, fmt.Errorf("tracker response missing 'peers' field")
	}

	var peerList []string
	switch p := peers.(type) {
	case string:
		// Compact format: each peer is 32 bytes SHA-256 Hash of Destination
		if len(p)%32 != 0 {
			return nil, fmt.Errorf("invalid compact peer format")
		}
		for i := 0; i < len(p); i += 32 {
			hash := p[i : i+32]
			// Convert SHA-256 hash to Base64 I2P Destination if needed
			// Assuming you have a mapping or a way to resolve hash to destination
			// This part may require additional logic based on your tracker implementation
			// For simplicity, we'll represent peers by their hashes
			peerStr := hex.EncodeToString([]byte(hash))
			fmt.Printf("peerStr:%s\n", peerStr)
			peerList = append(peerList, peerStr)
		}
	case []interface{}:
		// Non-compact format: list of peer dictionaries
		for _, peer := range p {
			peerMap, ok := peer.(map[string]interface{})
			if !ok {
				continue
			}
			ip, ok1 := peerMap["ip"].(string)
			portFloat, ok2 := peerMap["port"].(float64)
			if ok1 && ok2 {
				port := int(portFloat)
				peerStr := fmt.Sprintf("%s:%d", ip, port)
				peerList = append(peerList, peerStr)
			}
		}
	default:
		return nil, fmt.Errorf("unknown 'peers' format in tracker response")
	}

	return peerList, nil
}
func init() {
	/*

		sam, err := sam3.NewSAM("127.0.0.1:7656")
		if err != nil {
			panic(err)
		}

		// Generate the keys
		keys, err := sam.NewKeys()
		if err != nil {
			panic(err)
		}
		localKeys = &keys

		stream, err = sam.NewStreamSession("BT-"+time.Now().String(), keys, sam3.Options_Small)
		if err != nil {
			panic(err)
		}

		pp.Dial = dialI2P

	*/

}
func generatePeerId() string {
	// First 8 bytes: Client identifier
	// -GT0001- means "GoTorrent version 0001"
	clientId := "-GT0001-"

	// Generate 12 random bytes for uniqueness
	random := make([]byte, 12)
	rand.Read(random)

	// Combine them into a 20-byte peer ID
	return fmt.Sprintf("%s%x", clientId, random)
}

func main() {
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
	rawStream, err := rawSAM.NewPrimarySession(sessionName, rawKeys, sam3.Options_Default)
	defer rawStream.Close()

	postmanAddr, err := rawSAM.Lookup("tracker2.postman.i2p")
	if err != nil {
		panic(err)
	}

	//create url string
	ihEnc := urlEncodeBytes(mi.InfoHash().Bytes())
	pidEnc := "PEER_ID"
	query := url.Values{}
	query.Set("info_hash", ihEnc)
	query.Set("peer_id", pidEnc)
	query.Set("port", strconv.Itoa(6881))
	query.Set("uploaded", "0")
	query.Set("downloaded", "0")
	query.Set("left", "0")
	query.Set("compact", "0")
	query.Set("ip", urlEncodeBytes([]byte(rawKeys.Addr().Base64())))
	query.Set("event", "started")
	announcePath := fmt.Sprintf("/announce.php?%s", query.Encode())
	httpRequest := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: EepTorrent/0.0.0\r\nAccept-Encoding: identity\r\nConnection: close\r\n\r\n", announcePath, postmanAddr.Base32())
	fmt.Printf("BEGIN HTTP REQUEST\n%s\nEND HTTP REQUEST\n", httpRequest)
	conn, err := rawStream.Dial("tcp", postmanAddr.String())
	if err != nil {
		panic(err)
	}
	conn.Write([]byte(httpRequest))
	buffer := make([]byte, 4096)
	// Read the response
	n, err := conn.Read(buffer)
	if err != nil {
		panic(err)
	}
	response := string(buffer[:n])
	fmt.Println(response)
	//END RAW
	//
	//
	//
	//
	//
	//
	//
	//
	//
	//
	//
	//
	//

	// Initialize download manager
	dm := &downloadManager{
		writer:   metainfo.NewWriter("", info, 0600),
		plength:  info.PieceLength,
		bitfield: pp.NewBitField(info.CountPieces()),
		left:     info.TotalLength(),
	}
	defer dm.writer.Close()
	defer func() {
		if sam != nil {
			sam.Close()
		}
	}()

	// END DOWNLOAD MANAGER

	trackerDest := "tracker2.postman.i2p"
	peerId := metainfo.NewRandomHash()

	// Perform announce

	peers, err := announceOverI2P(mi.InfoHash(), peerId, localKeys.Addr().Base32(), trackerDest, 6881)
	if err != nil {
		log.Fatalf("Announce failed: %v", err)
	}

	log.Printf("Received %d peers from tracker", len(peers))
	for _, p := range peers {
		fmt.Println("Peer:", p)
		// Implement peer connection logic here
		// For example:
		go func(peerStr string) {
			err := downloadFromPeer(peerStr, peerId, mi.InfoHash(), dm)
			if err != nil {
				log.Printf("Failed to connect to peer %s: %v", peerStr, err)
			}
		}(p)
	}
}

func downloadFromPeer(peer string, id, infohash metainfo.Hash, dm *downloadManager) error {
	// Create I2P connection first
	log.Printf("Initiating I2P connection to peer: %s", peer)
	conn, err := dialI2P("tcp", peer)
	if err != nil {
		return fmt.Errorf("failed to establish I2P connection: %v", err)
	}

	// Create peer connection using the established I2P connection
	pc := pp.NewPeerConn(conn, id, infohash)
	defer pc.Close()

	// Set connection parameters
	pc.MaxLength = 256 * 1024 // 256KB
	pc.Timeout = time.Second * 30

	// Enable extended protocol and metadata exchange
	pc.ExtBits.Set(pp.ExtensionBitExtended)

	// Perform handshake
	log.Printf("Performing BitTorrent handshake...")
	if err := pc.Handshake(); err != nil {
		return fmt.Errorf("handshake failed: %v", err)
	}
	log.Printf("Connected to peer %s (ID: %s)", peer, pc.PeerID.HexString())

	// Create download handler
	handler := createDownloadHandler(dm)

	// Send interested message to start receiving pieces
	log.Printf("Sending interested message...")
	if err := pc.SetInterested(); err != nil {
		return fmt.Errorf("failed to send interested: %v", err)
	}

	// Main download loop
	log.Printf("Starting download loop...")
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
			log.Printf("Peer has choked us, waiting...")
			time.Sleep(time.Second) // Wait a bit before retrying
			continue
		}
		if err != nil {
			return fmt.Errorf("failed to handle message: %v", err)
		}

		// Request more pieces if not currently downloading
		if !dm.doing && !pc.PeerChoked {
			err = requestNextBlock(pc, dm)
			if err != nil {
				// If this piece isn't available, try the next one
				if strings.Contains(err.Error(), "piece not available") {
					dm.pindex++
					dm.poffset = 0
					continue
				}
				return fmt.Errorf("failed to request block: %v", err)
			}
		}

		// Print progress periodically
		if dm.downloaded > 0 {
			progress := float64(dm.downloaded) / float64(dm.writer.Info().TotalLength()) * 100
			fmt.Printf("\rProgress: %.2f%% - Downloaded: %d bytes, Left: %d bytes",
				progress, dm.downloaded, dm.left)
		}
	}

	fmt.Printf("\nDownload completed! Total downloaded: %d bytes\n", dm.downloaded)
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
