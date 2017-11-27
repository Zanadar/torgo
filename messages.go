package main

import (
	"bufio"
	"encoding/binary"
	"errors"

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

type message struct {
	length  int
	kind    int
	payload []byte
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

func NewHandshake(t *Torrent, logger log.Logger) Handshake {
	handshake := Handshake{
		InfoHash: t.InfoHash,
		PeerId:   t.PeerId,
	}

	level.Debug(logger).Log("handshake", t.InfoHash)

	return handshake
}

func readMessage(r *bufio.ReadWriter) (message, error) {
	msg := message{}
	LEN_HEADER := 4

	lenBytes := make([]byte, LEN_HEADER, LEN_HEADER)
	n, err := r.Read(lenBytes[:LEN_HEADER])
	errCheck(err)
	if n != LEN_HEADER {
		return msg, errors.New("problem with the length")
	}

	mlen := binary.BigEndian.Uint32(lenBytes)
	msg.length = int(mlen)

	mkind, err := r.ReadByte()
	errCheck(err)
	msg.kind = int(mkind)

	payload := make([]byte, mlen-1, mlen-1)
	n, err = r.Read(payload)

	msg.payload = payload

	return msg, nil
}
