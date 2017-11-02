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
	"github.com/jackpal/bencode-go"
)

func init() {
}

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

/*
failure reason: If present, then no other keys may be present. The value is a human-readable error message as to why the request failed (string).
warning message: (new, optional) Similar to failure reason, but the response still gets processed normally. The warning message is shown just like an error.
interval: Interval in seconds that the client should wait between sending regular requests to the tracker
min interval: (optional) Minimum announce interval. If present clients must not reannounce more frequently than this.
tracker id: A string that the client should send back on its next announcements. If absent and a previous announce sent a tracker id, do not discard the old value; keep using it.
complete: number of peers with the entire file, i.e. seeders (integer)
incomplete: number of non-seeder peers, aka "leechers" (integer)
peers: (dictionary model) The value is a list of dictionaries, each with the following keys:
peer id: peer's self-selected ID, as described above for the tracker request (string)
ip: peer's IP address either IPv6 (hexed) or IPv4 (dotted quad) or DNS name (string)
port: peer's port number (integer)
peers: (binary model) Instead of using the dictionary model described above, the peers value may be a string consisting of multiples of 6 bytes. First 4 bytes are the IP address and last 2 bytes are the port number. All in network (big endian) notation.

*/

func errCheck(err error) {
	if err != nil {
		fmt.Printf("Problem: %e", err)
	}
}

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) < 1 {
		fmt.Println("You need to supply a torrent file!")
		os.Exit(0)
	}

	torrentBuf, err := os.Open(args[0])
	errCheck(err)

	torrentInfo, err := parseTorrent(torrentBuf)
	errCheck(err)

	torrent, err := callTracker(*torrentInfo)
	errCheck(err)

	handshakeMsg := NewHandshake(torrent)
	errCheck(err)

	conn, err := net.Dial("tcp", torrent.Peers[0].String())
	defer conn.Close()
	errCheck(err)
	conn.Write([]byte(handshakeMsg.String()))
	resp := []byte{}
	read, err := bufio.NewReader(conn).Read(resp)
	spew.Dump(read, resp)
}

func parseTorrent(torrentF io.ReadSeeker) (*TorrentInfo, error) {
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

func callTracker(ti TorrentInfo) (Torrent, error) {
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
	spew.Dump(peers)

	torrent := Torrent{
		TrackerResponse: *trackerResp,
		InfoHash:        ti.InfoHash[:],
		PeerId:          id[:],
		Peers:           peers,
	}

	return torrent, err
}
