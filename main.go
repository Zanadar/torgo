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
	PeerList       []ConnPeer
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

	resp, err := http.Get(url.String())
	errCheck(err)
	defer resp.Body.Close()

	trackerResp := &TrackerResponse{}
	err = bencode.Unmarshal(resp.Body, trackerResp)
	errCheck(err)
	/* level.Debug(ti.logger).Log("response", spew.Sdump(trackerResp)) */

	peers := []ConnPeer{}
	var ip []interface{}
	for i := 0; i < len(trackerResp.Peers); i += 6 {
		peerBytes := []byte(trackerResp.Peers[i : i+6])
		{
			ip = []interface{}{
				peerBytes[0],
				peerBytes[1],
				peerBytes[2],
				peerBytes[3],
			}
			ipString := fmt.Sprintf("%d.%d.%d.%d", ip...)
			port := int(peerBytes[4])*256 + int(peerBytes[5])
			peers = append(peers, newPeer(ipString, port))
		}
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
	msgs   chan message
	quitCh chan os.Signal
	PieceLog
	peerLock  sync.Mutex
	peerConns map[string]ConnPeer
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
		peerConns:       make(map[string]ConnPeer),
		PieceLog:        newPieceLog(200),
	}
	torrent.peerLock.Lock()

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

func (t *Torrent) handleUnchoke(msg message) {
	t.unchoke(msg.source)
}

func (t *Torrent) unchoke(id string) {
	t.peerLock.Unlock()
	defer t.peerLock.Lock()
	t.peerConns[id].PeerChoking(false)
}

func (t *Torrent) handleShutdown() {
	var wg sync.WaitGroup
	shutdown := func(p *Peer) {
		p.shutdown <- struct{}{}
		<-p.shutdown
		wg.Done()
	}
	wg.Add(len(t.peerConns))
	for _, peer := range t.peerConns {
		go shutdown(peer.(*Peer))
	}
	wg.Wait()
}

func (t *Torrent) sendInterest(msg message) {
	t.peerLock.Unlock()
	defer t.peerLock.Lock()
	p := t.peerConns[msg.source]
	p.Message(message{length: 1, kind: INTERST})
}

func (t *Torrent) sendRequest(msg message) {
	t.peerLock.Unlock()
	defer t.peerLock.Lock()
	p := t.peerConns[msg.source]
	p.Message(message{length: 1, kind: INTERST})

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

type ConnPeer interface {
	Message(message)
	Connect(Handshake, chan message)
	ParseMsgs(chan message)
	AmChoking(bool)
	AmInterested(bool)
	PeerChoking(bool)
	PeerInterested(bool)
	ID() string
}

type Peer struct {
	id              string
	IP              string
	Port            int
	rw              *bufio.ReadWriter
	conn            net.Conn
	am_choking      bool
	peer_choking    bool
	peer_interested bool
	am_interested   bool
	shutdown        chan struct{}
	messages        chan message
}

func newPeer(IP string, Port int) *Peer {
	return &Peer{
		IP:              IP,
		Port:            Port,
		am_choking:      true,
		am_interested:   false,
		peer_choking:    true,
		peer_interested: false,
		shutdown:        make(chan struct{}),
		messages:        make(chan message),
	}
}

func (p Peer) String() string {
	return fmt.Sprintf("%s:%d", p.IP, p.Port)
}

func (p Peer) ID() string {
	return p.id
}

func (p *Peer) AmChoking(choke bool) {
	p.am_choking = choke
}

func (p *Peer) AmInterested(interest bool) {
	p.am_interested = interest
}

func (p *Peer) PeerInterested(interest bool) {
	p.peer_interested = interest
}

func (p *Peer) PeerChoking(choke bool) {
	p.peer_choking = choke
}

func (p *Peer) Connect(hs Handshake, msgs chan message) {
	conn, err := net.Dial("tcp", p.String())
	errCheck(err)
	p.conn = conn
	// save this somewhere
	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	p.rw = rw
	_, err = p.conn.Write(hs.Marshall())
	errCheck(err)
	err = rw.Flush()
	errCheck(err)
	r := io.Reader(rw)
	reply, err := Unmarshal(r)
	p.id = string(reply.PeerId[:])
	//TODO verify that InfoHash returned matches the one we have

	errCheck(err)
	go p.ParseMsgs(msgs)
	go p.readLoop()
	fmt.Printf("Peer %s setup done\n", p.ID)

	//this should start the peer loop and return the peer
}

func (p *Peer) readLoop() {
	for {
		select {
		case <-p.shutdown:
			fmt.Printf("Shutting down peer: %s\n", p.ID)
			p.conn.Close()
			close(p.shutdown)
			return
		case msg := <-p.messages:
			p.send(msg)
		}
	}
}

func (p *Peer) ParseMsgs(msgs chan message) {
	for {
		msg, err := readMessage(p.rw)
		msg.source = p.ID()
		errCheck(err)
		if err != nil {
			// maybe we should make a reconnect mssage here?
			fmt.Printf("Issue parsing:%e\n", err)
			break
		}
		msgs <- msg
	}
}

func (p *Peer) Message(msg message) {
	p.messages <- msg
}

func (p *Peer) send(msg message) {
	p.rw.Write(msg.Unmarshal())
	p.rw.Flush()
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

	// DialPeer this should be in a goroutine?
	spew.Dump(t.PeerList)
	p := t.PeerList[1]
	p.Connect(t.Handshake, t.msgs)
	t.peerLock.Unlock()
	t.peerConns[p.ID()] = p
	t.peerLock.Lock()
	for {
		select {
		case msg := <-t.msgs:
			switch {
			case msg.kind == BITFLD:
				t.handleBitfield(msg)
				t.sendInterest(msg)
				// Send INTERST to peer
			case msg.kind == HAVE:
				t.handleHave(msg)
				t.sendInterest(msg)
			case msg.kind == UNCHOKE:
				t.handleUnchoke(msg)
				t.sendRequest(msg)
			default:
				spew.Dump(msg)
			}
		case <-t.quitCh:
			fmt.Println("Shutdown received")
			t.handleShutdown()
			return
		}
	}
	spew.Dump(t)
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
