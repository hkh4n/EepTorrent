package peer

/*
import (
	"testing"
	"time"

	"github.com/go-i2p/go-i2p-bt/peerprotocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockPeerConn struct {
	mock.Mock
	peerprotocol.PeerConn
	Choked         bool
	PeerChoked     bool
	PeerInterested bool
	BitField       *peerprotocol.BitField
}

func (m *MockPeerConn) SendUnchoke() error {
	m.Called()
	m.Choked = false
	return nil
}

func (m *MockPeerConn) SendChoke() error {
	m.Called()
	m.Choked = true
	return nil
}

func (m *MockPeerConn) SendInterested() error {
	m.Called()
	m.PeerInterested = true
	return nil
}

func (m *MockPeerConn) SendNotInterested() error {
	m.Called()
	m.PeerInterested = false
	return nil
}

func TestPeerManagerChokingAlgorithm(t *testing.T) {
	// Setup
	writer := NewMockWriter(metainfo.Info{})
	dm := NewDownloadManager(writer, 1024*1024, 256*1024, 4, "/tmp/download")
	pm := NewPeerManager(dm)

	defer pm.Shutdown()

	// Create mock peers with varying download rates
	mockPeers := []*MockPeerConn{
		&MockPeerConn{BitField: peerprotocol.NewBitField(4)},
		&MockPeerConn{BitField: peerprotocol.NewBitField(4)},
		&MockPeerConn{BitField: peerprotocol.NewBitField(4)},
		&MockPeerConn{BitField: peerprotocol.NewBitField(4)},
		&MockPeerConn{BitField: peerprotocol.NewBitField(4)},
	}

	// Initialize their download rates
	peerRates := []float64{500, 300, 700, 200, 600} // in KB/s

	for i, peer := range mockPeers {
		pm.peerStats[peer] = &PeerStats{
			downloadRate: peerRates[i],
		}
	}

	// Add peers to PeerManager
	for _, peer := range mockPeers {
		pm.Peers[peer] = &PeerState{}
	}

	// Run choking algorithm
	pm.runChokingAlgorithm()

	// Verify that top 4 peers are unchoked based on download rates
	// Peers with rates: 700, 600, 500, 300 should be unchoked
	// Peer with rate 200 should be choked

	expectedUnchoked := map[*MockPeerConn]bool{
		mockPeers[2]: true,  // 700
		mockPeers[4]: true,  // 600
		mockPeers[0]: true,  // 500
		mockPeers[1]: true,  // 300
		mockPeers[3]: false, // 200
	}

	for _, peer := range mockPeers {
		unchokeCalled := peer.Calls.Find(func(args mock.Arguments) bool {
			return args.Method == "SendUnchoke"
		})
		chokeCalled := peer.Calls.Find(func(args mock.Arguments) bool {
			return args.Method == "SendChoke"
		})
		if expectedUnchoked[peer] {
			assert.True(t, peer.Choked == false, "Peer should be unchoked")
			assert.True(t, unchokeCalled != nil, "SendUnchoke should have been called")
		} else {
			assert.True(t, peer.Choked == true, "Peer should be choked")
			assert.True(t, chokeCalled != nil, "SendChoke should have been called")
		}
	}
}

func TestPeerManagerOptimisticUnchoking(t *testing.T) {
	// Setup
	writer := NewMockWriter(metainfo.Info{})
	dm := NewDownloadManager(writer, 1024*1024, 256*1024, 6, "/tmp/download")
	pm := NewPeerManager(dm)
	defer pm.Shutdown()

	// Create mock peers with same download rates
	mockPeers := []*MockPeerConn{
		&MockPeerConn{BitField: peerprotocol.NewBitField(6)},
		&MockPeerConn{BitField: peerprotocol.NewBitField(6)},
		&MockPeerConn{BitField: peerprotocol.NewBitField(6)},
		&MockPeerConn{BitField: peerprotocol.NewBitField(6)},
		&MockPeerConn{BitField: peerprotocol.NewBitField(6)},
		&MockPeerConn{BitField: peerprotocol.NewBitField(6)},
		&MockPeerConn{BitField: peerprotocol.NewBitField(6)},
	}

	for _, peer := range mockPeers {
		pm.peerStats[peer] = &PeerStats{
			downloadRate: 300, // Same rate
		}
		pm.Peers[peer] = &PeerState{}
	}

	// Run choking algorithm multiple times to trigger optimistic unchoking
	for i := 0; i < 4; i++ { // Run enough times to cover the optimistic unchoke interval
		pm.runChokingAlgorithm()
		time.Sleep(31 * time.Second) // Ensure the optimistic unchoke interval passes
	}

	// Verify that an additional peer has been unchoked optimistically
	unchokedCount := 0
	for _, peer := range mockPeers {
		if !peer.Choked {
			unchokedCount++
		}
	}
	assert.Equal(t, 5, unchokedCount, "Expected 4 regular unchokes + 1 optimistic unchoke")
}


*/
