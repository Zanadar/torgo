package main

import (
	"bufio"
	"crypto/sha1"
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
	InfoHash []byte
	PeerId   []byte // Our id that we send to the clients
	logger   log.Logger
}
type Info struct {
	Name        string
	Pieces      string
	Length      int64
	PieceLength int64 `bencode:"piece length"`
}

type TrackerResponse struct {
	PeerList       []Peer
	FailureReason  string `bencode:"failure reason"`
	WarningMessage string `bencode:"warning message"`
	Interval       int
	MinInternal    int    `bencode:"min interval"`
	TrackerID      string `bencode:"tracker id"`
	Complete       int
	incomplete     int
	Peers          string
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
	ti TorrentInfo
	TrackerResponse
	peerConns map[Peer]bufio.ReadWriter
	msgs      chan message
}

func (t *Torrent) readLoop() {
	// read shit from clients
}

func (t *Torrent) writeLoop() {
	// send shit to clients
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
	id := [20]byte{} // This is important!  The ID must be 20 bytes long
	copy(id[:], "boblog123")
	torrentInfo.PeerId = id[:]
	torrentF.Seek(0, 0) // rewind
	err = bencode.Unmarshal(torrentF, torrentInfo)
	errCheck(err)
	torrentInfo.InfoHash = infoHash.Sum(nil) // copy the hash into a full slice of the array

	return torrentInfo, err
}

func (ti *TorrentInfo) callTracker() (*TrackerResponse, error) {
	url, err := url.Parse(ti.Announce)
	errCheck(err)

	q := url.Query()
	q.Add("info_hash", string(ti.InfoHash[:]))
	q.Add("peer_id", string(ti.PeerId[:]))
	q.Add("left", strconv.Itoa(int(ti.Info.Length)))
	url.RawQuery = q.Encode()
	spew.Dump(url)

	resp, err := http.Get(url.String())
	errCheck(err)
	defer resp.Body.Close()

	trackerResp := &TrackerResponse{}
	err = bencode.Unmarshal(resp.Body, trackerResp)
	errCheck(err)

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
			ID:   i, //TODO don't do this so that we can save the actual ID?
			IP:   fmt.Sprintf("%d.%d.%d.%d", ip...),
			Port: port,
		})
	}
	trackerResp.PeerList = peers

	return trackerResp, nil
}

func newTorrent(ti TorrentInfo, logger log.Logger) (*Torrent, error) {
	trackerResp, err := ti.callTracker()
	if err != nil {
		return nil, err
	}
	torrent := &Torrent{
		TrackerResponse: *trackerResp,
		ti:              ti,
		peerConns:       make(map[Peer]bufio.ReadWriter),
		msgs:            make(chan message),
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

	torrent, err := newTorrent(*torrentInfo, log.With(logger, "trackerResponse"))
	errCheck(err)

	handshakeMsg := NewHandshake(torrent, logger)
	errCheck(err)
	spew.Dump(torrent.PeerList)

	// DialPeer this should be in a goroutine?
	conn, err := net.Dial("tcp", torrent.PeerList[1].String())
	defer conn.Close()
	errCheck(err)
	// save this somewhere
	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

	//do the handshake
	n, err := rw.Write(handshakeMsg.Marshall())
	errCheck(err)
	err = rw.Flush()
	errCheck(err)
	resp := [68]byte{} // we probably need to parse this and save the id and make sure its a legit connection
	n, err = rw.Read(resp[:])
	spew.Dump(n, resp)

	// start receiving messages
	for {
		msg, err := readMessage(rw)
		errCheck(err)
		if err != nil {
			break
		}
		spew.Dump(msg)
	}
	// responsd to messages as they come in
}
