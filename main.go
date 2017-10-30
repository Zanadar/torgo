package main

import (
	"crypto/sha1"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/jackpal/bencode-go"
)

func init() {
}

type TorrentInfo struct {
	Info
	Announce string
	Encoding string
}
type Info struct {
	Name        string
	Pieces      string
	Length      int64
	PieceLength int64 `bencode:"piece length"`
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
	if err != nil {
		fmt.Printf("Problem with ", args)
	}

	torrentInfo := &TorrentInfo{}
	err = bencode.Unmarshal(torrentBuf, torrentInfo)
	if err != nil {
		fmt.Printf("Problem unmarshalling ", err)
	}
	fmt.Printf("\n\n\ntorrentInfo: \n %#v \n\n", torrentInfo)

	// We have to assert the type in order to work with it.

	infoHash := sha1.New()
	bencode.Marshal(infoHash, torrentInfo.Info)
	fmt.Printf("Sha of inforString: % x", infoHash.Sum(nil))

	url, err := url.Parse(torrentInfo.Announce)
	if err != nil {
		fmt.Printf("Errer: %e", err)
	}
	q := url.Query()
	q.Add("info_hash", string(infoHash.Sum(nil)))
	q.Add("peer_id", "boblog123")
	q.Add("left", strconv.Itoa(int(torrentInfo.Info.Length)))
	url.RawQuery = q.Encode()

	//All of this monkey business is to sort the hash the info dictionay
	// The keys have to appear in consistent order ....

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
