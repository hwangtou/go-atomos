package go_atomos

import (
	"reflect"
	"testing"
)

func TestUtilFileSizeToString(t *testing.T) {
	type args struct {
		size      int64
		precision int
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "TestUtilFileSizeToString-0",
			args: args{
				size:      0,
				precision: 3,
			},
			want: "0B",
		},
		{
			name: "TestUtilFileSizeToString-1000",
			args: args{
				size:      1000,
				precision: 1,
			},
			want: "1000B",
		},
		{
			name: "TestUtilFileSizeToString-1024",
			args: args{
				size:      1024,
				precision: 1,
			},
			want: "1.0KB",
		},
		{
			name: "TestUtilFileSizeToString-1024*1024-1",
			args: args{
				size:      1024*1024 - 1,
				precision: 3,
			},
			want: "1023.999KB",
		},
		{
			name: "TestUtilFileSizeToString-1024*1024",
			args: args{
				size:      1024 * 1024,
				precision: 1,
			},
			want: "1.0MB",
		},
		{
			name: "TestUtilFileSizeToString-1024*1024*1024-1",
			args: args{
				size:      1024*1024*1024 - 1024,
				precision: 3,
			},
			want: "1023.999MB",
		},
		{
			name: "TestUtilFileSizeToString-1024*1024*1024",
			args: args{
				size:      1024 * 1024 * 1024,
				precision: 1,
			},
			want: "1.0GB",
		},
		{
			name: "TestUtilFileSizeToString-1024*1024*1024*1024-1",
			args: args{
				size:      1024*1024*1024*1024 - 1024*1024,
				precision: 3,
			},
			want: "1023.999GB",
		},
		{
			name: "TestUtilFileSizeToString-1024*1024*1024*1024",
			args: args{
				size:      1024 * 1024 * 1024 * 1024,
				precision: 1,
			},
			want: "1.0TB",
		},
		{
			name: "TestUtilFileSizeToString-1024*1024*1024*1024*1024",
			args: args{
				size:      1024 * 1024 * 1024 * 1024 * 1024,
				precision: 1,
			},
			want: "1024.0TB",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UtilFileSizeToString(tt.args.size, tt.args.precision); got != tt.want {
				t.Errorf("UtilFileSizeToString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStringToDiskSize(t *testing.T) {
	type args struct {
		sizeStr string
	}
	tests := []struct {
		name    string
		args    args
		want    int64
		wantErr *Error
	}{
		{
			name: "TestStringToDiskSize-Empty",
			args: args{
				sizeStr: "",
			},
			want:    0,
			wantErr: NewError(ErrFrameworkIncorrectUsage, "UtilStringToFileSize: The string should not be empty."),
		},
		{
			name: "TestStringToDiskSize-1",
			args: args{
				sizeStr: "1",
			},
			want:    1,
			wantErr: nil,
		},
		{
			name: "TestStringToDiskSize-1B",
			args: args{
				sizeStr: "1B",
			},
			want:    1,
			wantErr: nil,
		},
		{
			name: "TestStringToDiskSize-0.5KB",
			args: args{
				sizeStr: "0.5KB",
			},
			want:    512,
			wantErr: nil,
		},
		{
			name: "TestStringToDiskSize-0.99KB",
			args: args{
				sizeStr: "0.99KB",
			},
			want:    1013,
			wantErr: nil,
		},
		{
			name: "TestStringToDiskSize-1024",
			args: args{
				sizeStr: "1024",
			},
			want:    1024,
			wantErr: nil,
		},
		{
			name: "TestStringToDiskSize-1KB",
			args: args{
				sizeStr: "1KB",
			},
			want:    1024,
			wantErr: nil,
		},
		{
			name: "TestStringToDiskSize-1.001KB",
			args: args{
				sizeStr: "1.001KB",
			},
			want:    1025,
			wantErr: nil,
		},
		{
			name: "TestStringToDiskSize-1.5KB",
			args: args{
				sizeStr: "1.5KB",
			},
			want:    1536,
			wantErr: nil,
		},
		{
			name: "TestStringToDiskSize-0.99MB",
			args: args{
				sizeStr: "0.99MB",
			},
			want:    1038090,
			wantErr: nil,
		},
		{
			name: "TestStringToDiskSize-1MB",
			args: args{
				sizeStr: "1MB",
			},
			want:    1048576,
			wantErr: nil,
		},
		{
			name: "TestStringToDiskSize-1.001MB",
			args: args{
				sizeStr: "1.001MB",
			},
			want:    1049624,
			wantErr: nil,
		},
		{
			name: "TestStringToDiskSize-1.5MB",
			args: args{
				sizeStr: "1.5MB",
			},
			want:    1572864,
			wantErr: nil,
		},
		{
			name: "TestStringToDiskSize-0.99GB",
			args: args{
				sizeStr: "0.99GB",
			},
			want:    1063004405,
			wantErr: nil,
		},
		{
			name: "TestStringToDiskSize-1GB",
			args: args{
				sizeStr: "1GB",
			},
			want:    1073741824,
			wantErr: nil,
		},
		{
			name: "TestStringToDiskSize-1.001GB",
			args: args{
				sizeStr: "1.001GB",
			},
			want:    1074815565,
			wantErr: nil,
		},
		{
			name: "TestStringToDiskSize-1.5GB",
			args: args{
				sizeStr: "1.5GB",
			},
			want:    1610612736,
			wantErr: nil,
		},
		{
			name: "TestStringToDiskSize-0.99TB",
			args: args{
				sizeStr: "0.99TB",
			},
			want:    1088516511498,
			wantErr: nil,
		},
		{
			name: "TestStringToDiskSize-1TB",
			args: args{
				sizeStr: "1TB",
			},
			want:    1099511627776,
			wantErr: nil,
		},
		{
			name: "TestStringToDiskSize-1.001TB",
			args: args{
				sizeStr: "1.001TB",
			},
			want:    1100611139403,
			wantErr: nil,
		},
		{
			name: "TestStringToDiskSize-1.5TB",
			args: args{
				sizeStr: "1.5TB",
			},
			want:    1649267441664,
			wantErr: nil,
		},
		{
			name: "TestStringToDiskSize-1.0.1GB",
			args: args{
				sizeStr: "1.0.1GB",
			},
			want:    0,
			wantErr: NewErrorf(ErrFrameworkIncorrectUsage, "UtilStringToFileSize: Invalid size string. size=(1.0.1GB)"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := UtilStringToFileSize(tt.args.sizeStr)
			if got != tt.want {
				t.Errorf("UtilStringToFileSize() got = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(got1, tt.wantErr) {
				t.Errorf("UtilStringToFileSize() got1 = %v, want %v", got1, tt.wantErr)
			}
		})
	}
}
