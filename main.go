package main

import (
	"crypto/sha1"
	"flag"
	"fmt"
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
}
type Info struct {
	Name        string
	Pieces      string
	Length      int64
	PieceLength int64 `bencode:"piece length"`
}

func errCheck(err error) {
	if err != nil {
		fmt.Printf("Problem: %e", err)
	}
}

func main() {
	verbose := flag.Bool("v", false, "Verbose output")
	flag.Parse()
	args := flag.Args()
	if len(args) < 1 {
		fmt.Println("You need to supply a torrent file!")
		os.Exit(0)
	}

	torrentBuf, err := os.Open(args[0])
	errCheck(err)
	torrentParts, err := bencode.Decode(torrentBuf)
	errCheck(err)

	t := torrentParts.(map[string]interface{})
	info := t["info"].(map[string]interface{})
	infoHash := sha1.New()
	err = bencode.Marshal(infoHash, info)
	errCheck(err)
	spew.Dump(infoHash.Sum(nil)) // This is correct per: https://allenkim67.github.io/programming/2016/05/04/how-to-make-your-own-bittorrent-client.html#info-hash
	// <Buffer 11 7e 3a 66 65 e8 ff 1b 15 7e 5e c3 78 23 57 8a db 8a 71 2b>

	torrentInfo := &TorrentInfo{}
	torrentBuf.Seek(0, 0) // rewind
	err = bencode.Unmarshal(torrentBuf, torrentInfo)
	errCheck(err)
	fmt.Printf("\n\n\ntorrentInfo: \n %#v \n\n", torrentInfo)

	url, err := url.Parse(torrentInfo.Announce)
	if err != nil {
		fmt.Printf("Errer: %e", err)
	}
	id := [20]byte{} // This is important!  The ID must be 20 bytes long
	copy(id[:], "boblog123")
	q := url.Query()
	q.Add("info_hash", string(infoHash.Sum(nil)))
	q.Add("peer_id", string(id[:]))
	q.Add("left", strconv.Itoa(int(torrentInfo.Info.Length)))
	url.RawQuery = q.Encode()

	resp, err := http.Get(url.String())
	if err != nil {
		fmt.Printf("Errer: %e", err)
	} else {
		fmt.Printf("\n\n\nUrl %+v %#v", url, url)
		fmt.Printf("\n\n\nResponse %+v %#v", resp, resp)
	}

	//Feedback
	if *verbose {
	}
}
