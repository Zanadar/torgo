package main

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/zanadar/benlex"
	"io/ioutil"
	"net/http"
	"os"
)

func init() {
}

func main() {
	verbose := flag.Bool("v", false, "Verbose output")
	flag.Parse()
	args := flag.Args()
	if len(args) < 1 {
		fmt.Println("You need to supply a torrent file!")
		os.Exit(0)
	}

	torrent, err := ioutil.ReadFile(args[0])
	if err != nil {
		fmt.Printf("Problem with ", args[0])
	}

	torrentBuf := bytes.NewReader(torrent)

	torrentParts, _ := benlex.Decode(torrentBuf)
	announce := torrentParts["announce"]
	announceUrl := announce.(string)

	resp, err := http.Get(announceUrl)
	if err != nil {
		fmt.Println("Couldn't connect to:", announceUrl)
	} else {
		fmt.Println(resp, "From", announceUrl)
	}

	if *verbose {
		for k, v := range torrentParts {
			fmt.Printf("%s: %q\n", k, v)
		}
	}

}
