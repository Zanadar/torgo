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

const (
	DL_FILE = "./Downloads/"
)

type Info struct {
	Name        string
	Pieces      string
	pieceStore  Pieces
	Length      int64
	PieceLength int64 `bencode:"piece length"`
}

type Pieces struct {
	data string
}

func (i *Pieces) String() [][]byte {
	pieces := []byte(i.data)
	var hashes [][]byte
	var hash []byte

	for i := 0; i < len(pieces); i += 20 {
		hash = pieces[i : i+20]
		hashes = append(hashes, hash)
	}

	return hashes
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

func (ti *TorrentInfo) callTracker(logger log.Logger) (*TrackerResponse, error) {
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
	level.Debug(ti.logger).Log("response", spew.Sdump(trackerResp))

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
			peers = append(peers, newPeer(ipString, port, log.With(logger, "Peer", ipString)))
		}
	}
	trackerResp.PeerList = peers
	level.Debug(ti.logger).Log("peers", spew.Sdump(peers))

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

	torrentInfo := &TorrentInfo{
		logger: logger,
	}
	id := [20]byte{} // This is important!  The ID must be 20 bytes long
	copy(id[:], "boblog123")
	torrentInfo.PeerId = id[:]
	_, err = torrentF.Seek(0, 0) // rewind
	errCheck(err)
	err = bencode.Unmarshal(torrentF, torrentInfo)
	errCheck(err)
	torrentInfo.InfoHash = infoHash.Sum(nil) // copy the hash into a full slice of the array
	torrentInfo.pieceStore.data = torrentInfo.Pieces

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
	msgs              chan message
	quitCh            chan os.Signal
	errChan           chan error
	PeerPieceLog      PieceLog
	RequestedPieceLog PieceLog
	WriteLog          []bool
	Piecer            Piecer
	sync.Mutex
	peerConns map[string]ConnPeer
	logger    log.Logger
}

func newTorrent(ti TorrentInfo, logger log.Logger) (*Torrent, error) {
	trackerResp, err := ti.callTracker(logger)
	if err != nil {
		return nil, err
	}

	h := Handshake{}
	h.InfoHash = [20]byte{}
	h.PeerId = [20]byte{}
	copy(h.InfoHash[:], ti.InfoHash)
	copy(h.PeerId[:], ti.PeerId)

	level.Debug(logger).Log("handshake", ti.InfoHash)

	pieceCount := int(ti.Length/ti.PieceLength) + 1 // this is a hack but so what if we over allocate...
	piecer, err := newPiecerFS(ti.Name, pieceCount, int(ti.PieceLength))

	torrent := &Torrent{
		TrackerResponse:   *trackerResp,
		Handshake:         h,
		ti:                ti,
		msgs:              make(chan message),
		quitCh:            make(chan os.Signal, 1),
		errChan:           make(chan error, 1),
		peerConns:         make(map[string]ConnPeer),
		PeerPieceLog:      newPieceLog(pieceCount),
		RequestedPieceLog: newPieceLog(pieceCount),
		WriteLog:          make([]bool, pieceCount),
		logger:            logger,
		Piecer:            piecer,
	}

	//this should start the torrent loop and return the torrent

	return torrent, err
}

func (t *Torrent) writeLoop() {
	// send shit to clients
}

func (t *Torrent) handleBitfield(msg message) {
	t.PeerPieceLog.LogField(msg.source, msg.payload)
}

func (t *Torrent) handleHave(msg message) {
	// turn the index into a bitfield payload
	i := binary.BigEndian.Uint32(msg.payload)
	t.PeerPieceLog.LogSingle(msg.source, int(i))
}

func (t *Torrent) handleUnchoke(msg message) {
	t.unchoke(msg.source)
}

func (t *Torrent) unchoke(id string) {
	t.Lock()
	defer t.Unlock()
	p := t.peerConns[id]
	p.PeerChoking(false)
	level.Debug(t.logger).Log("peerUnchoke", p.state())
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
	t.Lock()
	defer t.Unlock()
	p := t.peerConns[msg.source]
	p.AmInterested(true)
	p.Message(message{length: 1, kind: INTERST})
	level.Debug(t.logger).Log("interest", p.state())
}

func (t *Torrent) sendRequest(msg message) {
	// This makes requests in order
	p := t.peerConns[msg.source]
	level.Debug(t.logger).Log("request", p)
	OFFSET := 0
	for i, piece := range t.WriteLog { // check the WriteLog
		// if we havent' written or requested TODO add some sort of timeout to make sure requested pieces actually get written
		// TODO consider a data structure that allows us to query the PieceLogs for the  rarest piece
		if !piece && !(len(t.RequestedPieceLog.At(i)) > 0) {
			if peers := t.PeerPieceLog.At(i); len(peers) > 0 { // And a peer has it
				for pID, _ := range peers {
					t.Lock()
					p := t.peerConns[pID]
					// TODO Remove this?
					if p.GetPeerChoking() { // if we're being choked still
						break
					}
					p.Message(buildRequest(
						string(t.PeerId[:]),
						i,
						OFFSET,
						int(t.ti.PieceLength),
					))
					t.Unlock()
					t.RequestedPieceLog.LogSingle(pID, i)
				}
			}
		}
	}
	// Check for pieces we haven't requested
	// Check for pieces that peers have
	// Request the rarest of those pieces first
	// Record the request in the log
}

type Piecer interface {
	Write(int, int, []byte, chan error)
}

type PiecerFS struct {
	file       *os.File
	path       string
	blockSize  int
	pieceCount int
}

func newPiecerFS(path string, pieceCount int, blockSize int) (*PiecerFS, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &PiecerFS{
		file:       f,
		path:       path,
		blockSize:  blockSize,
		pieceCount: pieceCount,
	}, nil
}

func (p *PiecerFS) Write(index int, begin int, data []byte, errChan chan error) {
	offset := p.calcOffset(index, begin)
	_, err := p.file.WriteAt(data, offset)
	if err != nil {
		errChan <- err
	}
}

func (p *PiecerFS) calcOffset(index, begin int) int64 {
	return int64((index * p.blockSize) + begin)
}

type PieceLog struct {
	sync.RWMutex
	vector []map[string]struct{}
}

func newPieceLog(length int) PieceLog {
	vec := make([]map[string]struct{}, length)
	for i := range vec {
		vec[i] = make(map[string]struct{})
	}
	return PieceLog{vector: vec}
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
	p.Lock()
	defer p.Unlock()
	p.vector[piece][id] = struct{}{}
}

func (p *PieceLog) String() (filled string) {
	var has int
	for _, piece := range p.Logged() {
		if piece {
			has = 1
		} else {
			has = 0
		}
		filled = fmt.Sprintf("%s%b", filled, has)
	}

	return filled
}

func (p *PieceLog) Logged() []bool {
	p.RLock()
	defer p.Unlock()
	var have []bool
	for i, piece := range p.vector {
		if len(piece) > 0 {
			have[i] = true
		}
	}

	return have
}

// We need to start making these access synced
func (p *PieceLog) At(index int) map[string]struct{} {
	p.RLock()
	defer p.RUnlock()
	return p.vector[index]
}

type ConnPeer interface {
	Message(message)
	Connect(Handshake, chan message)
	ParseMsgs(chan message)
	AmChoking(bool)
	GetAmChoking() bool
	AmInterested(bool)
	GetAmInterested() bool
	PeerChoking(bool)
	GetPeerChoking() bool
	PeerInterested(bool)
	GetPeerInterested() bool
	state() string
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
	logger          log.Logger
}

func newPeer(IP string, Port int, logger log.Logger) *Peer {
	return &Peer{
		IP:              IP,
		Port:            Port,
		am_choking:      true,
		am_interested:   false,
		peer_choking:    true,
		peer_interested: false,
		shutdown:        make(chan struct{}),
		messages:        make(chan message),
		logger:          logger,
	}
}

func (p *Peer) String() string {
	pString := fmt.Sprintf("%s:%d", p.IP, p.Port)
	return pString
}

func (p *Peer) ID() string {
	return p.id
}

func (p *Peer) AmChoking(choke bool) {
	p.am_choking = choke
}

func (p *Peer) GetAmChoking() bool {
	return p.am_choking
}

func (p *Peer) AmInterested(interest bool) {
	p.am_interested = interest
}

func (p *Peer) GetAmInterested() bool {
	return p.am_interested
}

func (p *Peer) PeerInterested(interest bool) {
	p.peer_interested = interest
}

func (p *Peer) GetPeerInterested() bool {
	return p.peer_interested
}

func (p *Peer) PeerChoking(choke bool) {
	p.peer_choking = choke
}

func (p *Peer) GetPeerChoking() bool {
	return p.peer_choking
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
	level.Debug(p.logger).Log("connected", p.state())

	//this should start the peer loop and return the peer
}

func (p *Peer) readLoop() {
	for {
		select {
		case <-p.shutdown:
			fmt.Printf("Shutting down peer: %s\n", p.ID())
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
			fmt.Printf("Issue parsing:%v, %+#v\n", err, msg)
			break
		}
		msgs <- msg
	}
}

func (p *Peer) Message(msg message) {
	p.messages <- msg
}

func (p *Peer) state() string {
	return fmt.Sprintf("Peer:\n %+#v", p)
}

func (p *Peer) send(msg message) {
	fmt.Printf("Sending: %+v\n", msg)
	n, err := p.conn.Write(msg.Unmarshal())
	if n != msg.length {
		fmt.Printf("Tried to send %v bytes but sent %v\n", msg.length, n)
	}
	errCheck(err)
	err = p.rw.Flush()
	errCheck(err)
}

func errCheck(err error) {
	if err != nil {
		fmt.Printf("Problem: %v\n", err)
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

	ti, err := parseTorrent(torrentBuf, log.With(logger, "component", "TorrentInfo"))
	errCheck(err)

	t, err := newTorrent(*ti, log.With(logger, "component", "Torrent"))
	signal.Notify(t.quitCh, os.Interrupt)
	errCheck(err)

	// DialPeer this should be in a goroutine?
	level.Debug(logger).Log("PeerList", spew.Sdump(t.PeerList))
	p := t.PeerList[1]
	p.Connect(t.Handshake, t.msgs)
	t.Lock()
	t.peerConns[p.ID()] = p
	t.Unlock()
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
				level.Debug(logger).Log("msg", spew.Sdump(msg))
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
