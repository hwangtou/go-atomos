cur_dir=`pwd`

go mod vendor
go build commander.go

cp commander /usr/local/bin/atomos_commander