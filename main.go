package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/jackpal/bencode-go"
)

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
	/* level.Debug(ti.logger).Log("response", spew.Sdump(trackerResp)) */

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
			IP:   fmt.Sprintf("%d.%d.%d.%d", ip...),
			Port: port,
		})
	}
	trackerResp.PeerList = peers
	/* level.Debug(ti.logger).Log("peers", spew.Sdump(peers)) */

	return trackerResp, nil
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
	_, err = torrentF.Seek(0, 0) // rewind
	errCheck(err)
	err = bencode.Unmarshal(torrentF, torrentInfo)
	errCheck(err)
	torrentInfo.InfoHash = infoHash.Sum(nil) // copy the hash into a full slice of the array

	return torrentInfo, err
}

type TorrentInfo struct {
	Announce string
	Encoding string
	Info
	InfoHash []byte
	PeerId   []byte // Our id that we send to the clients
	logger   log.Logger
}

type Torrent struct {
	ti TorrentInfo
	TrackerResponse
	Handshake
	peerConns map[string]chan struct{}
	msgs      chan message
	quitCh    chan os.Signal
	PieceLog
	// That would allow us to do rarest first requests?
}

func newTorrent(ti TorrentInfo, logger log.Logger) (*Torrent, error) {
	trackerResp, err := ti.callTracker()
	if err != nil {
		return nil, err
	}

	h := Handshake{}
	h.InfoHash = [20]byte{}
	h.PeerId = [20]byte{}
	copy(h.InfoHash[:], ti.InfoHash)
	copy(h.PeerId[:], ti.PeerId)

	level.Debug(logger).Log("handshake", ti.InfoHash)

	torrent := &Torrent{
		TrackerResponse: *trackerResp,
		Handshake:       h,
		ti:              ti,
		msgs:            make(chan message),
		quitCh:          make(chan os.Signal, 1),
		peerConns:       make(map[string]chan struct{}),
		PieceLog:        newPieceLog(200),
	}

	//this should start the torrent loop and return the torrent

	return torrent, err
}

func (t *Torrent) writeLoop() {
	// send shit to clients
}

func (t *Torrent) handleBitfield(msg message) {
	t.LogField(msg.source, msg.payload)
}

func (t *Torrent) handleHave(msg message) {
	// turn the index into a bitfield payload
	i := binary.BigEndian.Uint32(msg.payload)
	t.LogSingle(msg.source, int(i))
}

func (t *Torrent) handleShutdown() {
	var wg sync.WaitGroup
	close := func(conn chan struct{}) {
		conn <- struct{}{}
		<-conn
		wg.Done()
	}
	wg.Add(len(t.peerConns))
	for _, peer := range t.peerConns {
		go close(peer)
	}
	wg.Wait()

}

func (t *Torrent) connectPeer(p *Peer) {
	conn, err := net.Dial("tcp", p.String())
	errCheck(err)
	defer conn.Close()
	// save this somewhere
	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	p.conn = rw
	n, err := p.conn.Write(t.Handshake.Marshall())
	errCheck(err)
	err = rw.Flush()
	errCheck(err)
	r := io.Reader(rw)
	reply, err := Unmarshal(r)
	p.ID = string(reply.PeerId[:])
	//TODO verify that InfoHash returned matches the one we have
	t.peerConns[p.ID] = make(chan struct{})

	errCheck(err)
	spew.Dump("resp", n, reply)
	go p.parseMsgs(t.msgs)
	p.readLoop(t.peerConns[p.ID])
	fmt.Printf("Peer %s done", p.ID)

	//this should start the peer loop and return the peer
}

type PieceLog struct {
	vector []map[string]struct{}
}

func newPieceLog(length int) PieceLog {
	vec := make([]map[string]struct{}, length)
	for i := range vec {
		vec[i] = make(map[string]struct{})
	}
	return PieceLog{vec}
}
func (p *PieceLog) LogField(id string, pieces []byte) error {
	bitString := ""
	for _, b := range pieces {
		bitString = fmt.Sprintf("%s%.8b", bitString, b) // prints 1111111111111101
	}
	for i, bit := range bitString {
		if bit == '1' {
			p.LogSingle(id, i)
		}
	}
	return nil
}

func (p *PieceLog) LogSingle(id string, piece int) {
	p.vector[piece][id] = struct{}{}
}

func (p *PieceLog) String() (filled string) {
	var has int
	for _, piece := range p.vector {
		if len(piece) > 0 {
			has = 1
		} else {
			has = 0
		}
		filled = fmt.Sprintf("%s%b", filled, has)
	}

	return filled
}

type Peer struct {
	ID         string
	IP         string
	Port       int
	conn       *bufio.ReadWriter
	choked     bool
	interested bool
}

func (p Peer) String() string {
	return fmt.Sprintf("%s:%d", p.IP, p.Port)
}

func (p *Peer) readLoop(shutdown chan struct{}) {
	<-shutdown
	fmt.Println("Shutting down peer")
	close(shutdown)
}

func (p *Peer) parseMsgs(msgs chan message) {
	for {
		msg, err := readMessage(p.conn)
		msg.source = p.ID
		errCheck(err)
		if err != nil {
			break
		}
		msgs <- msg
	}
}

func errCheck(err error) {
	if err != nil {
		fmt.Printf("Problem: %e", err)
	}
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

	t, err := newTorrent(*torrentInfo, log.With(logger, "trackerResponse"))
	signal.Notify(t.quitCh, os.Interrupt)
	errCheck(err)

	errCheck(err)
	spew.Dump(t.PeerList)

	// DialPeer this should be in a goroutine?
	p := t.PeerList[2]
	ticker := time.NewTicker(time.Second * 1)
	go t.connectPeer(&p)
	for {
		select {
		case msg := <-t.msgs:
			switch {
			case msg.kind == BITFLD:
				t.handleBitfield(msg)
				// Send INTERST to peer
			case msg.kind == HAVE:
				t.handleHave(msg)
			}
			spew.Dump(msg)
			/* spew.Dump(t) */
		case sig := <-t.quitCh:
			fmt.Println("Shutdown received")
			spew.Dump(sig)
			t.handleShutdown()
			return
		case <-ticker.C:
			fmt.Printf("Tick")
		}
		spew.Dump(t)
	}
	//TODO at this point, we can take the message from the peers and start to do things with them.
	// use IOTA to give them meaningful names, and then change the state of the torrent based on some kind of logic?
	/* we're getting BITFLD and HAVE messages from peers, so we need to track their state:
		        This starts as: i
		        {
		          INTERST: 0
		          CHOKE: 1
		        }

			   1. Choked
			   2. which pieces they have
			   3. which pieces they want

		           Then we send them an INTERST message and receive an UNCHOKE
	                   If we receive and INTERST msg we can respond with a UNCHOKE to indicate we will serve files
	                   After a peer is UNCHOKEd we can send requests

	*/

}
