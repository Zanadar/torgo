package main

import (
	"strings"
	"testing"
)

func Test_handleBitfieldMsg(t *testing.T) {
	cases := []struct {
		name     string
		payload  []byte
		source   string
		expected string
	}{
		{"4 pieces", []byte("\x06"), "boblog123", "01100000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tor := &Torrent{
				PieceLog: newPieceLog(len(tc.payload) * 8),
			}
			msg := message{
				source:  tc.source,
				kind:    BITFLD,
				payload: tc.payload,
			}
			tor.handleBitfield(msg)
			res := tor.PieceLog.String()
			if res != tc.expected {
				t.Fatalf("got %s; want %s to be stored", res, tc.expected)
			}

			first := strings.Index(res, "1")
			_, ok := tor.PieceLog.vector[first][tc.source]
			if !ok {
				t.Fatalf("got %s; not stored at %s", tor.PieceLog.vector, ok)
			}
		})
	}

}

func Test_PieceLogString(t *testing.T) {
	pieces := []byte("\x06")
	pieceLog := newPieceLog(len(pieces) * 8)
	err := pieceLog.Log("boblog123", pieces)
	if err != nil {
		t.Error(err)
	}

	res := pieceLog.String()

	if res != "01100000" {
		t.Errorf("got '%s'; expected '%s'", res, "01100000")
	}
}
