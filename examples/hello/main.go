package main

import (
	atomos "github.com/hwangtou/go-atomos"
	"log"
)

func main() {
	conf, err := atomos.NewConfigFromYamlFileArgument()
	if err != nil {
		log.Fatalf("config invalid, err=(%v)", err)
	}

	AtomosRunnable.SetConfig(conf)
	atomos.CosmosProcessMainFn(AtomosRunnable)
}
