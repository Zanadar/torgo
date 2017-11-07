package benlex

import (
	"bytes"
	// "fmt"
	// "math"
	"testing"
)

func TestDecodeSinglefileTorrentBencode(t *testing.T) {
	str := "d8:announce41:http://bttracker.debian.org:6969/announce7:comment35:\"Debian CD from cdimage.debian.org\"13:creation datei1391870037e9:httpseedsl85:http://cdimage.debian.org/cdimage/release/7.4.0/iso-cd/debian-7.4.0-amd64-netinst.iso85:http://cdimage.debian.org/cdimage/archive/7.4.0/iso-cd/debian-7.4.0-amd64-netinst.isoe4:infod6:lengthi232783872e4:name30:debian-7.4.0-amd64-netinst.iso12:piece lengthi262144e6:pieces0:ee"
	buf := bytes.NewBufferString(str)
	dict, err := Decode(buf)
	if err != nil {
		t.Error(err)
	}

	if dict["announce"] != "http://bttracker.debian.org:6969/announce" {
		t.Error("announce mismatch")
	} else if dict["comment"] != "\"Debian CD from cdimage.debian.org\"" {
		t.Error("comment mismatch")
	} else if dict["creation date"].(int64) != 1391870037 {
		t.Error("creation date mismatch")
	}

	// res := string(Encode(dict))
	// if res != str {
	// t.Error("mismatch")
	// }
}
