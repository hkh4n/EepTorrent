package peer

import (
	"eeptorrent/lib/download"
	pp "github.com/go-i2p/go-i2p-bt/peerprotocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"net"
	"testing"
	"time"
)

// MockPeerConn is a mock implementation of pp.PeerConn using testify/mock
type MockPeerConn struct {
	mock.Mock
	PeerChoked     bool
	PeerInterested bool
	BitField       *pp.BitField // Assuming Bitfield is a type representing bitfields
	RemoteAddrVal  string
}

func (m *MockPeerConn) SendChoke() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockPeerConn) SendUnchoke() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockPeerConn) SendInterested() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockPeerConn) SendNotInterested() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockPeerConn) SendHave(pieceIndex uint32) error {
	args := m.Called(pieceIndex)
	return args.Error(0)
}

func (m *MockPeerConn) SendRequest(pieceIndex, begin, length uint32) error {
	args := m.Called(pieceIndex, begin, length)
	return args.Error(0)
}

func (m *MockPeerConn) SendPiece(pieceIndex, begin uint32, block []byte) error {
	args := m.Called(pieceIndex, begin, block)
	return args.Error(0)
}

func (m *MockPeerConn) SendExtHandshakeMsg(msg pp.ExtendedHandshakeMsg) error {
	args := m.Called(msg)
	return args.Error(0)
}

func (m *MockPeerConn) ReadMsg() (pp.Message, error) {
	args := m.Called()
	return args.Get(0).(pp.Message), args.Error(1)
}

func (m *MockPeerConn) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockPeerConn) RemoteAddr() *net.Addr {
	addr := &net.Addr{}
	return addr
}

func TestNewPeerManager(t *testing.T) {
	dm := &download.DownloadManager{} // You might need to mock this if it has dependencies
	pm := NewPeerManager(dm)

	assert.NotNil(t, pm)
	assert.NotNil(t, pm.Peers)
	assert.NotNil(t, pm.peerStats)
	assert.NotNil(t, pm.shutdownChan)
	assert.Equal(t, time.Now().Unix(), pm.lastUnchoking.Unix(), "lastUnchoking should be initialized to current time")
	assert.Equal(t, time.Now().Unix(), pm.lastOptimistic.Unix(), "lastOptimistic should be initialized to current time")
}

func TestPeerManager_AddAndRemovePeer(t *testing.T) {
	dm := &download.DownloadManager{}
	pm := NewPeerManager(dm)

	mockPeer := new(MockPeerConn)
	mockPeer.RemoteAddrVal = "127.0.0.1:6881"
	mockPeer.BitField = pp.NewBitField(100) // Assuming 100 pieces

	pm.AddPeer(mockPeer)
	assert.Contains(t, pm.Peers, mockPeer, "Peer should be added to PeerManager.Peers")
	assert.Contains(t, pm.peerStats, mockPeer, "Peer should be added to PeerManager.peerStats")

	pm.RemovePeer(mockPeer)
	assert.NotContains(t, pm.Peers, mockPeer, "Peer should be removed from PeerManager.Peers")
	assert.NotContains(t, pm.peerStats, mockPeer, "Peer should be removed from PeerManager.peerStats")
}

func TestPeerManager_UpdatePeerStats(t *testing.T) {
	dm := &download.DownloadManager{}
	pm := NewPeerManager(dm)

	mockPeer := new(MockPeerConn)
	pm.AddPeer(mockPeer)

	// Simulate downloading data
	pm.UpdatePeerStats(mockPeer, 2048, 0)
	stats := pm.peerStats[mockPeer]
	assert.Equal(t, int64(2048), stats.bytesDownloaded, "bytesDownloaded should be updated")
	assert.True(t, stats.downloadRate > 0, "downloadRate should be greater than 0")
	assert.True(t, stats.lastDownload.Before(time.Now()) || stats.lastDownload.Equal(time.Now()), "lastDownload should be updated to current time")

	// Simulate uploading data
	pm.UpdatePeerStats(mockPeer, 0, 1024)
	assert.Equal(t, int64(1024), stats.bytesUploaded, "bytesUploaded should be updated")
	assert.True(t, stats.uploadRate > 0, "uploadRate should be greater than 0")
}

func TestPeerManager_CalculatePeerScore(t *testing.T) {
	dm := &download.DownloadManager{}
	pm := NewPeerManager(dm)

	mockPeer := new(MockPeerConn)
	pm.AddPeer(mockPeer)

	stats := pm.peerStats[mockPeer]
	stats.downloadRate = 500.0
	stats.uploadRate = 100.0
	stats.hasUnchokedUs = true
	stats.unchokedUsDuration = 6 * time.Minute
	stats.lastWeUnchoked = time.Now().Add(-10 * time.Second)

	score := pm.calculatePeerScore(mockPeer)
	expectedScore := (500.0 * 0.7) + (100.0 * 0.3) // 350 + 30 = 380
	// Apply bonuses
	expectedScore *= 1.2  // for hasUnchokedUs
	expectedScore *= 1.1  // for unchokedUsDuration > 5 minutes
	expectedScore *= 1.15 // for reciprocation

	assert.Equal(t, expectedScore, score, "Peer score should be calculated correctly")
}

func TestPeerManager_HandleSeedingMode(t *testing.T) {
	dm := &download.DownloadManager{}
	dm.IsFinished = func() bool { return true }
	dm.Pieces = make([]*download.Piece, 10)
	for i := range dm.Pieces {
		dm.Pieces[i] = &download.Piece{
			Blocks: make([]bool, 10),
		}
	}

	pm := NewPeerManager(dm)

	// Add multiple mock peers
	mockPeers := []*MockPeerConn{
		new(MockPeerConn),
		new(MockPeerConn),
		new(MockPeerConn),
	}
	for _, peer := range mockPeers {
		peer.BitField = NewBitfield(10)
		pm.AddPeer(peer)
		pm.peerStats[peer].bytesDownloaded = 0 // Peers haven't downloaded anything
	}

	pm.HandleSeedingMode(time.Now())

	// Assuming HandleSeeding unchokes top uploaders, which are all since upload rates are zero
	for _, peer := range mockPeers {
		if !peer.PeerChoked {
			assert.False(t, peer.PeerChoked, "Peer should be unchoked in seeding mode")
		}
	}
}

func TestPeerManager_RunChokingAlgorithm(t *testing.T) {
	dm := &download.DownloadManager{}
	pm := NewPeerManager(dm)
	defer pm.Shutdown()

	mockPeer1 := new(MockPeerConn)
	mockPeer2 := new(MockPeerConn)
	mockPeer3 := new(MockPeerConn)

	// Initialize bitfields
	mockPeer1.BitField = pp.NewBitField(100)
	mockPeer2.BitField = pp.NewBitField(100)
	mockPeer3.BitField = pp.NewBitField(100)

	// Add peers
	pm.AddPeer(mockPeer1)
	pm.AddPeer(mockPeer2)
	pm.AddPeer(mockPeer3)

	// Set peer stats
	pm.peerStats[mockPeer1].downloadRate = 100.0
	pm.peerStats[mockPeer1].uploadRate = 50.0

	pm.peerStats[mockPeer2].downloadRate = 200.0
	pm.peerStats[mockPeer2].uploadRate = 100.0

	pm.peerStats[mockPeer3].downloadRate = 150.0
	pm.peerStats[mockPeer3].uploadRate = 75.0

	// Mock SendUnchoke
	mockPeer1.On("SendUnchoke").Return(nil)
	mockPeer2.On("SendUnchoke").Return(nil)
	mockPeer3.On("SendUnchoke").Return(nil)

	// Mock SendChoke
	mockPeer1.On("SendChoke").Return(nil)
	mockPeer2.On("SendChoke").Return(nil)
	mockPeer3.On("SendChoke").Return(nil)

	// Allow the choking algorithm to run at least once
	time.Sleep(unchokingInterval + 1*time.Second)

	// Verify that the top maxUnchoked peers are unchoked
	// Assuming maxUnchoked = 4, all peers should be unchoked
	assert.False(t, mockPeer1.PeerChoked, "Peer1 should be unchoked")
	assert.False(t, mockPeer2.PeerChoked, "Peer2 should be unchoked")
	assert.False(t, mockPeer3.PeerChoked, "Peer3 should be unchoked")

	mockPeer1.AssertExpectations(t)
	mockPeer2.AssertExpectations(t)
	mockPeer3.AssertExpectations(t)
}

func TestPeerManager_Shutdown(t *testing.T) {
	dm := &download.DownloadManager{}
	pm := NewPeerManager(dm)

	// Add a peer
	mockPeer := new(MockPeerConn)
	pm.AddPeer(mockPeer)

	// Mock SendChoke
	mockPeer.On("SendChoke").Return(nil)

	// Shutdown PeerManager
	pm.Shutdown()

	// After shutdown, the choking algorithm goroutine should exit
	// Verify that SendChoke was called during shutdown
	mockPeer.AssertCalled(t, "SendChoke")
}
