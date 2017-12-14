//go:generate stringer -type=msgID
package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
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

type message struct {
	source  string
	length  int
	kind    msgID
	payload []byte
}

func (m message) Unmarshal() []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, uint32(m.length))
	binary.Write(&buf, binary.BigEndian, uint8(m.kind))
	binary.Write(&buf, binary.BigEndian, m.payload)

	return buf.Bytes()
}

func (m message) String() string {
	return fmt.Sprintf("source: %s len: %s, kind: %s \n %v\n", m.source, m.length, m.kind, m.Unmarshal())
}

type Handshake struct {
	_        [1]byte  // pstrlen
	_        [19]byte // pstr
	_        [8]byte  //reserved
	InfoHash [20]byte
	PeerId   [20]byte
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
	msg.length = int(mlen) // TODO handle the special case of a keepalive

	mkind, err := r.ReadByte()
	errCheck(err)
	msg.kind = msgID(mkind)

	payload := make([]byte, mlen-1, mlen-1)
	n, err = r.Read(payload)
	errCheck(err)

	msg.payload = payload

	return msg, nil
}

func buildRequest(id string, idx int, offset int, blockSize int) message {

	fmt.Printf("offset: %v\n", offset)
	fmt.Printf("Blocksize: %v\n", blockSize)
	fmt.Printf("Blocksize: %v\n", int32(blockSize))
	var payload bytes.Buffer
	binary.Write(&payload, binary.BigEndian, int32(idx))
	binary.Write(&payload, binary.BigEndian, int32(offset))
	binary.Write(&payload, binary.BigEndian, int32(blockSize))

	return message{
		kind:    REQ,
		source:  id,
		length:  13,
		payload: payload.Bytes(),
	}
}
