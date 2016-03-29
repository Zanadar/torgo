package main

import (
	"crypto/sha1"
	"flag"
	"fmt"
	"github.com/jackpal/bencode-go"
	"sort"
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
	// We have to assert the type in order to work with it.
	t := torrentParts.(map[string]interface{})
	url := t["announce"].(string)

	//All of this monkey business is to sort the hash the info dictionay
	// The keys have to appear in consistent order ....

	info := t["info"].(map[string]interface{})
	keys := []string{}
	for k, _ := range info {
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

	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("Errer:", err)
	} else {
		fmt.Println(resp)
	}

	//Feedback
	if *verbose {
		for k, v := range t {
			if k != "info" {
				fmt.Printf("%s: %X\n\n", k, v)
			}
		}
		for k, v := range info {
			fmt.Printf("%s: %q\n\n", k, v)
		}
	}

}
