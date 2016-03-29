package main

import (
	"crypto/sha1"
	"flag"
	"fmt"
	"github.com/jackpal/bencode-go"
	"net/http"
	"os"
	"sort"
)

func init() {
}

type TorrentInfor struct {
	Info map[string]interface{}
}
type Info struct {
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
		fmt.Printf("Problem with ", args[0])
	}

	torrentParts, _ := bencode.Decode(torrentBuf)

	// We have to assert the type in order to work with it.
	t := torrentParts.(map[string]interface{})
	url := t["announce"].(string)

	//All of this monkey business is to sort the hash the info dictionay
	// The keys have to appear in consistent order ....
	info := t["info"].(map[string]interface{})
	infoHash := sha1.New()
	bencode.Marshal(infoHash, info)
	fmt.Printf("Sha of inforString: % x", infoHash.Sum(nil))

	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("Errer:", err)
	} else {
		fmt.Println(resp)
	}

	//Feedback
	if *verbose {
		for k, v := range info {
			fmt.Printf("%s: %q\n\n", k, v)
		}
	}

}
