package main

const (
	pstrlen = 19
	pstr    = "BitTorrent protocol"
)

var reserved = [8]byte{}

type Handshake struct {
	InfoHash []byte
	PeerId   []byte
}

func (h Handshake) Marshall() []byte {
	handShake := []byte{byte(19)}
	handShake = append(handShake, []byte(pstr)...)
	handShake = append(handShake, reserved[:]...) // Make array to slice
	handShake = append(handShake, []byte(h.InfoHash)...)
	handShake = append(handShake, []byte(h.PeerId)...)

	return handShake
}

func NewHandshake(t Torrent) Handshake {
	return Handshake{
		InfoHash: t.InfoHash,
		PeerId:   t.PeerId,
	}
}
