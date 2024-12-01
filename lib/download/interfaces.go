package download

import (
	pp "github.com/go-i2p/go-i2p-bt/peerprotocol"
)

//go:generate mockgen -destination=mocks/mock_download_manager.go -package=mocks eeptorrent/lib/download DownloadManagerInterface

type DownloadManagerInterface interface {
	OnBlock(index uint32, begin uint32, block []byte) error
	IsFinished() bool
	Progress() float64
	GetBlock(pieceIndex, offset, length uint32) ([]byte, error)
	VerifyPiece(index uint32) (bool, error)
	NeedPiecesFrom(pc *pp.PeerConn) bool
	AddPeer(peer *pp.PeerConn)
	RemovePeer(peer *pp.PeerConn)
	GetAllPeers() []*pp.PeerConn
}
