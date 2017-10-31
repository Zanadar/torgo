package main

import (
	"crypto/sha1"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
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

	//All of this monkey business is to sort the hash the info dictionay
	// The keys have to appear in consistent order ....

	info := t["info"].(map[string]interface{})
	keys := []string{}
	for k := range info {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	infoBytes := []byte{}
	for _, k := range keys {
		infoBytes = append(infoBytes, []byte(k)...)
		switch typ := info[k].(type) {
		case int64:
			infoBytes = append(infoBytes, byte(typ))
		}
	}
	infoSHA := sha1.Sum(infoBytes)
	fmt.Printf("\ninfo SHA1: % x\n", infoSHA)

	torrentInfo := &TorrentInfo{}
	torrentBuf.Seek(0, 0) // rewind
	err = bencode.Unmarshal(torrentBuf, torrentInfo)
	errCheck(err)
	fmt.Printf("\n\n\ntorrentInfo: \n %#v \n\n", torrentInfo)

	infoHash := sha1.New()
	bencode.Marshal(infoHash, torrentInfo.Info)
	fmt.Printf("Sha of inforString: %#x", infoHash.Sum(nil))

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
