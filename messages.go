//go:generate stringer -type=msgID
package main

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
)

type msgID int

const (
	KPALIVE msgID = iota - 1
	CHOKE
	UNCHOKE
	INTERST
	UNINTERST
	HAVE
	BITFLD
	REQ
	PIECE
	CNCL // we can give these payload methods that know how to parse their payload
)
const (
	pstrlen = 19
	pstr    = "BitTorrent protocol"
)

var reserved = [8]byte{}

type Handshake struct {
	_        [1]byte  // pstrlen
	_        [19]byte // pstr
	_        [8]byte  //reserved
	InfoHash [20]byte
	PeerId   [20]byte
}

type message struct {
	source  string
	length  int
	kind    msgID
	payload []byte
}

func Unmarshal(r io.Reader) (*Handshake, error) {
	h := &Handshake{}
	err := binary.Read(r, binary.BigEndian, h)
	return h, err
}

func (h Handshake) Marshall() []byte {
	handShake := []byte{byte(19)}
	handShake = append(handShake, []byte(pstr)...)
	handShake = append(handShake, reserved[:]...) // Make array to slice
	handShake = append(handShake, []byte(h.InfoHash[:])...)
	handShake = append(handShake, []byte(h.PeerId[:])...)

	return handShake
}

func readMessage(r *bufio.ReadWriter) (message, error) {
	msg := message{}
	LEN_HEADER := 4

	lenBytes := make([]byte, LEN_HEADER, LEN_HEADER)
	n, err := r.Read(lenBytes[:LEN_HEADER])
	if err != nil {
		return message{}, err
	}
	if n != LEN_HEADER {
		return msg, errors.New("problem with the length")
	}

	mlen := binary.BigEndian.Uint32(lenBytes)
	msg.length = int(mlen) // handle the special case of a keepalive

	mkind, err := r.ReadByte()
	errCheck(err)
	msg.kind = msgID(mkind)

	payload := make([]byte, mlen-1, mlen-1)
	n, err = r.Read(payload)
	errCheck(err)

	msg.payload = payload

	return msg, nil
}
