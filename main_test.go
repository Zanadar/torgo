package main

import (
	"reflect"
	"testing"
)

func Test_splitMessages(t *testing.T) {
	type args struct {
		data  []byte
		atEOF bool
	}
	tests := []struct {
		name        string
		args        args
		wantAdvance int
		wantToken   []byte
		wantErr     bool
	}{
	// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAdvance, gotToken, err := splitMessages(tt.args.data, tt.args.atEOF)
			if (err != nil) != tt.wantErr {
				t.Errorf("splitMessages() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotAdvance != tt.wantAdvance {
				t.Errorf("splitMessages() gotAdvance = %v, want %v", gotAdvance, tt.wantAdvance)
			}
			if !reflect.DeepEqual(gotToken, tt.wantToken) {
				t.Errorf("splitMessages() gotToken = %v, want %v", gotToken, tt.wantToken)
			}
		})
	}
}
