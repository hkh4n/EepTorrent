package peer

import (
	"eeptorrent/lib/peer/mocks"
	pp "github.com/go-i2p/go-i2p-bt/peerprotocol"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"net"
	"sync"
	"testing"
	"time"
)

func TestPeerManager_AddRemovePeer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPM := mocks.NewMockPeerManagerInterface(ctrl)
	peer := &pp.PeerConn{}

	mockPM.EXPECT().AddPeer(peer).Times(1)
	mockPM.AddPeer(peer)

	mockPM.EXPECT().RemovePeer(peer).Times(1)
	mockPM.RemovePeer(peer)
}

func TestPeerManager_Stats(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPM := mocks.NewMockPeerManagerInterface(ctrl)
	peer := &pp.PeerConn{}

	mockPM.EXPECT().
		UpdatePeerStats(peer, int64(1000), int64(500)).
		Times(1)

	mockPM.UpdatePeerStats(peer, 1000, 500)
}

func TestPeerManager_InterestStates(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPM := mocks.NewMockPeerManagerInterface(ctrl)
	peer := &pp.PeerConn{}

	mockPM.EXPECT().OnPeerInterested(peer).Times(1)
	mockPM.OnPeerInterested(peer)

	mockPM.EXPECT().OnPeerNotInterested(peer).Times(1)
	mockPM.OnPeerNotInterested(peer)
}

func TestPeerManager_ChokeStates(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPM := mocks.NewMockPeerManagerInterface(ctrl)
	peer := &pp.PeerConn{}

	mockPM.EXPECT().OnPeerChoke(peer).Times(1)
	mockPM.OnPeerChoke(peer)

	mockPM.EXPECT().OnPeerUnchoke(peer).Times(1)
	mockPM.OnPeerUnchoke(peer)
}

func TestPeerManager_Seeding(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPM := mocks.NewMockPeerManagerInterface(ctrl)

	mockPM.EXPECT().HandleSeeding().Times(1)
	mockPM.HandleSeeding()
}

func TestPeerManager_Shutdown(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPM := mocks.NewMockPeerManagerInterface(ctrl)

	mockPM.EXPECT().Shutdown().Times(1)
	mockPM.Shutdown()
}

func TestPeerScore(t *testing.T) {
	pm := &PeerManager{
		peerStats: make(map[*pp.PeerConn]*PeerStats),
	}
	peer := &pp.PeerConn{}

	pm.peerStats[peer] = &PeerStats{
		downloadRate:       100,
		uploadRate:         50,
		hasUnchokedUs:      true,
		unchokedUsDuration: 6 * time.Minute,
		lastWeUnchoked:     time.Now(),
	}

	score := pm.calculatePeerScore(peer)
	assert.Greater(t, score, float64(0), "Score should be positive")

	emptyPeer := &pp.PeerConn{}
	emptyScore := pm.calculatePeerScore(emptyPeer)
	assert.Equal(t, float64(0), emptyScore, "Score should be zero for peer with no stats")
}

func TestPeerOptimisticUnchoke(t *testing.T) {
	pm := NewPeerManager(nil)
	peer := &pp.PeerConn{}

	pm.AddPeer(peer)
	pm.peerStats[peer].isInterested = true

	pm.lastOptimistic = time.Now().Add(-31 * time.Second)
	pm.optimisticPeer = nil

	pm.handleSeedingMode(time.Now())

	pm.Shutdown()
}

func TestConcurrentPeerOperations(t *testing.T) {
	pm := NewPeerManager(nil)
	defer pm.Shutdown()

	numPeers := 10
	var wg sync.WaitGroup

	for i := 0; i < numPeers; i++ {
		wg.Add(1)
		go func(peerID int) {
			defer wg.Done()

			peer := &pp.PeerConn{
				Interested: false,
				PeerChoked: true,
				BitField:   pp.NewBitField(10),
			}
			peer.Conn = &net.TCPConn{}

			pm.Mu.Lock()
			pm.Peers[peer] = NewPeerState()
			pm.peerStats[peer] = &PeerStats{
				bytesDownloaded: 1000,
				bytesUploaded:   500,
				lastDownload:    time.Now(),
				lastUpload:      time.Now(),
			}

			delete(pm.Peers, peer)
			delete(pm.peerStats, peer)
			pm.Mu.Unlock()
		}(i)
	}

	wg.Wait()

	pm.Mu.Lock()
	peerCount := len(pm.Peers)
	pm.Mu.Unlock()

	assert.Equal(t, 0, peerCount, "All peers should be removed")
}
