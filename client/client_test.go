package torgo

import (
	"github.com/zanadar/benlex"
	"os"
	"testing"
)

func TestArgs(t *testing.T) {
	expected := "bla"
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"cmd", "./fixtures/flagfromserver.torrent"}

}
