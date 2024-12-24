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

func BenchmarkUtilStringRandomStringGenerator_RandomString(b *testing.B) {
	r := NewUtilStringRandomStringGenerator()
	for i := 0; i < b.N; i++ {
		r.RandomString(10)
	}
}

func TestUtilStringHashGenerator_Gen(t *testing.T) {
	type args struct {
		content string
		hash    string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "Test UtilStringHashGenerator Gen",
			args: args{
				content: UtilStringAZaz09,
				hash:    "db4bfcbd4da0cd85a60c3c37d3fbd8805c77f15fc6b1fdfe614ee0a7c8fdb4c0",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := NewUtilStringHashSHA256Generator()
			h, err := u.Gen(tt.args.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("Gen() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if h != tt.args.hash {
				t.Errorf("Gen() = %v, want %v", h, tt.args.hash)
				return
			}
		})
	}
}

func BenchmarkUtilStringHashGenerator_Gen(b *testing.B) {
	u := NewUtilStringHashSHA256Generator()
	for i := 0; i < b.N; i++ {
		u.Gen(UtilStringAZaz09)
	}
}
