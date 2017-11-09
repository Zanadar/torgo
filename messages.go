package main

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
)

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

	spew.Dump(handShake)

	return handShake
}

func NewHandshake(t Torrent, logger log.Logger) Handshake {
	handshake := Handshake{
		InfoHash: t.InfoHash,
		PeerId:   t.PeerId,
	}

	level.Debug(logger).Log("handshake", t.InfoHash)

	return handshake
}
