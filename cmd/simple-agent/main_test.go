package main

import (
	"reflect"
	"testing"
)

func TestNormalizeResumeArgs(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "long flag with separate value",
			in:   []string{"--resume", "session-123"},
			want: []string{"--resume=session-123"},
		},
		{
			name: "short flag with separate value",
			in:   []string{"-r", "session-123"},
			want: []string{"-r=session-123"},
		},
		{
			name: "no bare value when next token is another flag",
			in:   []string{"--resume", "--verbose"},
			want: []string{"--resume", "--verbose"},
		},
		{
			name: "existing equals syntax preserved",
			in:   []string{"--resume=session-123"},
			want: []string{"--resume=session-123"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeResumeArgs(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("normalizeResumeArgs(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
