module github.com/hwangtou/go-atomos/supervisor

go 1.18

require (
	github.com/hwangtou/go-atomos v0.2.0
	github.com/kardianos/service v1.2.1
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	golang.org/x/sys v0.0.0-20220908164124-27713097b956 // indirect
	google.golang.org/protobuf v1.26.0 // indirect
)

replace github.com/hwangtou/go-atomos v0.2.0 => ../
