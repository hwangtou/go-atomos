package go_atomos

import (
	"strings"
	"testing"
)

func TestUtilStringRandomStringGenerator_RandomString(t *testing.T) {
	type args struct {
		length int
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "Test UtilStringRandomStringGenerator RandomString",
			args: args{
				length: 10,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewUtilStringRandomStringGenerator()
			got := r.RandomString(tt.args.length)
			if len(got) != tt.args.length {
				t.Errorf("RandomString() = %v, want %v", len(got), tt.args.length)
			}
			for _, c := range got {
				if !strings.Contains(UtilStringAZaz09, string(c)) {
					t.Errorf("RandomString() = %v, want %v", got, UtilStringAZaz09)
				}
			}
		})
	}
}
