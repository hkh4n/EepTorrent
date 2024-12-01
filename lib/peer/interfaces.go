package peer

import pp "github.com/go-i2p/go-i2p-bt/peerprotocol"

//go:generate mockgen -destination=mocks/mock_peer_manager.go -package=mocks eeptorrent/lib/peer PeerManagerInterface

type PeerManagerInterface interface {
	AddPeer(pc *pp.PeerConn)
	RemovePeer(pc *pp.PeerConn)
	OnPeerChoke(pc *pp.PeerConn)
	OnPeerUnchoke(pc *pp.PeerConn)
	OnPeerInterested(pc *pp.PeerConn)
	OnPeerNotInterested(pc *pp.PeerConn)
	UpdatePeerStats(pc *pp.PeerConn, bytesDownloaded, bytesUploaded int64)
	HandleSeeding()
	Shutdown()
}
