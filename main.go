package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/davecgh/go-spew/spew"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/jackpal/bencode-go"
)

type TorrentInfo struct {
	Announce string
	Encoding string
	Info
	InfoHash [20]byte
}
type Info struct {
	Name        string
	Pieces      string
	Length      int64
	PieceLength int64 `bencode:"piece length"`
}

type TrackerResponse struct {
	FailureReason  string `bencode:"failure reason"`
	WarningMessage string `bencode:"warning message"`
	Interval       int
	MinInternal    int    `bencode:"min interval"`
	TrackerID      string `bencode:"tracker id"`
	Complete       int
	incomplete     int
	Peers          string // This is for the binary model of peers
}

type Peer struct {
	ID   int `bencode:"peer id"`
	IP   string
	Port int
}

func (p Peer) String() string {
	return fmt.Sprintf("%s:%d", p.IP, p.Port)
}

type Torrent struct {
	PeerId   []byte // Our id that we send to the clients
	InfoHash []byte
	Peers    []Peer
	TrackerResponse
}

type message struct {
	length  int
	kind    int
	payload []byte
}

func errCheck(err error) {
	if err != nil {
		fmt.Printf("Problem: %e", err)
	}
}

func parseTorrent(torrentF io.ReadSeeker, logger log.Logger) (*TorrentInfo, error) {
	torrentParts, err := bencode.Decode(torrentF)
	errCheck(err)

	t := torrentParts.(map[string]interface{})
	info := t["info"].(map[string]interface{})
	infoHash := sha1.New()
	err = bencode.Marshal(infoHash, info)
	errCheck(err)
	// This is correct per: https://allenkim67.github.io/programming/2016/05/04/how-to-make-your-own-bittorrent-client.html#info-hash
	// <Buffer 11 7e 3a 66 65 e8 ff 1b 15 7e 5e c3 78 23 57 8a db 8a 71 2b>

	torrentInfo := &TorrentInfo{}
	torrentF.Seek(0, 0) // rewind
	err = bencode.Unmarshal(torrentF, torrentInfo)
	errCheck(err)
	copy(torrentInfo.InfoHash[:], infoHash.Sum(nil)) // copy the hash into a full slice of the array

	return torrentInfo, err
}

func callTracker(ti TorrentInfo, logger log.Logger) (Torrent, error) {
	url, err := url.Parse(ti.Announce)
	errCheck(err)
	id := [20]byte{} // This is important!  The ID must be 20 bytes long
	copy(id[:], "boblog123")

	q := url.Query()
	q.Add("info_hash", string(ti.InfoHash[:]))
	q.Add("peer_id", string(id[:]))
	q.Add("left", strconv.Itoa(int(ti.Info.Length)))
	url.RawQuery = q.Encode()

	resp, err := http.Get(url.String())
	errCheck(err)
	defer resp.Body.Close()

	trackerResp := &TrackerResponse{}
	err = bencode.Unmarshal(resp.Body, trackerResp)
	errCheck(err)
	level.Debug(logger).Log("response", spew.Sdump(trackerResp))

	peers := []Peer{}
	var ip []interface{}
	for i := 0; i < len(trackerResp.Peers); i += 6 {
		peerBytes := []byte(trackerResp.Peers[i : i+6])
		ip = []interface{}{
			peerBytes[0],
			peerBytes[1],
			peerBytes[2],
			peerBytes[3],
		}
		port := int(peerBytes[4])*256 + int(peerBytes[5])
		peers = append(peers, Peer{
			ID:   i,
			IP:   fmt.Sprintf("%d.%d.%d.%d", ip...),
			Port: port,
		})
	}
	level.Debug(logger).Log("peers", spew.Sdump(peers))

	torrent := Torrent{
		TrackerResponse: *trackerResp,
		InfoHash:        ti.InfoHash[:],
		PeerId:          id[:],
		Peers:           peers,
	}

	return torrent, err
}

func main() {
	debug := flag.Bool("debug", false, "Print debug statements")
	flag.Parse()
	args := flag.Args()
	if len(args) < 1 {
		fmt.Println("You need to supply a torrent file!")
		os.Exit(0)
	}

	var logger log.Logger
	{
		logLevel := level.AllowInfo()
		if *debug {
			logLevel = level.AllowAll()
		}
		logger = log.NewLogfmtLogger(os.Stdout)
		logger = level.NewFilter(logger, logLevel)
	}

	torrentBuf, err := os.Open(args[0])
	errCheck(err)

	torrentInfo, err := parseTorrent(torrentBuf, log.With(logger, "parseTorrent"))
	errCheck(err)

	torrent, err := callTracker(*torrentInfo, log.With(logger, "trackerResponse"))
	errCheck(err)

	handshakeMsg := NewHandshake(torrent, logger)
	level.Debug(logger).Log("torrent", spew.Sdump(torrent))
	errCheck(err)

	conn, err := net.Dial("tcp", torrent.Peers[1].String())
	defer conn.Close()
	errCheck(err)
	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

	n, err := rw.Write(handshakeMsg.Marshall())
	errCheck(err)
	err = rw.Flush()
	errCheck(err)
	resp := [68]byte{}
	n, err = rw.Read(resp[:])
	spew.Dump(n, resp)

	// byteString := strings.NewReader("\x00\x00\x00\x0b\x05\xdf\xef\xdf\x77\xfb\xff\xff\xff\xff\xee\x00\x00\x00\x05\x04\x00\x00\x00\x02\x00\x00\x00\x05\x04\x00\x00\x00\x0b\x00\x00\x00\x05\x04\x00\x00\x00\x12\x00\x00\x00\x05\x04\x00\x00\x00\x18\x00\x00\x00\x05\x04\x00\x00\x00\x1c\x00\x00\x00\x05\x04\x00\x00\x00\x25\x00\x00\x00\x05\x04\x00\x00\x00\x4b")

	for {
		msg, err := readMessage(rw)
		errCheck(err)
		if err != nil {
			break
		}
		spew.Dump(msg)
	}
}

func readMessage(r *bufio.ReadWriter) (message, error) {
	msg := message{}
	LEN_HEADER := 4
	// Read first four bytes as an int -> LENGTH
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
	// if n != int(mlen) {
	// 	return msg, fmt.Errorf("problem with the payload %d", n)
	// }

	msg.payload = payload

	return msg, nil
}
