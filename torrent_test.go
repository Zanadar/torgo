package main

import (
	"fmt"
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
		{"more pieces", []byte("\x06\xff"), "boblog123", "0110000011111111"},
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
	cases := []struct {
		id       string
		pieces   []byte
		expected string
	}{
		{"boblog123", []byte("\x06"), "01100000"},
		{"boblog123", []byte("\x06\xff"), "0110000011111111"},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("Test: %s", tc.expected), func(t *testing.T) {
			pieceLog := newPieceLog(len(tc.pieces) * 8)
			pieceLog.Log(tc.id, tc.pieces)

			res := pieceLog.String()

			if res != tc.expected {
				t.Errorf("got '%s'; expected '%s'", res, tc.expected)
			}
		})
	}
}
