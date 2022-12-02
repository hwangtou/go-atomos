cur_dir=`pwd`

go mod vendor
go build commander.go
go build supervisor.go

mv commander /usr/local/bin/atomos_commander
mv supervisor /usr/local/bin/atomos_supervisor
