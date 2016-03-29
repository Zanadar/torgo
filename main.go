package main

import (
	// "bytes"
	"flag"
	"fmt"
	"github.com/jackpal/bencode-go"
	// "io"
	// "io/ioutil"
	"net/http"
	"os"
)

func init() {
}

type TorrentInfor struct {
	infoUrl string
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
	// We have to asser the type in order to work with it.
	t := torrentParts.(map[string]interface{})
	url := t["announce"].(string)
	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("Errer:", err)
	} else {
		fmt.Println(resp)
	}
	if *verbose {
		for k, v := range t {
			fmt.Printf("%q, %q", k, v)
		}
		fmt.Printf("%T", torrentParts)
	}

}
