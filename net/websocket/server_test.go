package websocket

import (
	"log"
	"net/http"
	"testing"
	"time"
)

type logger struct {
	*testing.T
}

func (l logger) Write(p []byte) (n int, err error) {
	l.T.Log(string(p))
	return 0, nil
}

func TestServer(t *testing.T) {
	newServer(t)
	<-time.After(10 * time.Minute)
}

func TestClientDuplicated(t *testing.T) {
	newConn(t, "client1")
	<-time.After(2 * time.Second)
	newConn(t, "client1")
	<-time.After(2 * time.Second)
	newConn(t, "client1")
	<-time.After(2 * time.Second)
	newConn(t, "client1")
	<-time.After(2 * time.Second)
	newConn(t, "client1")
	<-time.After(1 * time.Minute)
}

func TestServerRun(t *testing.T) {
	go newServer(t)

	newConn(t, "client1")
	<-time.After(2 * time.Second)
	newConn(t, "client2")
	<-time.After(2 * time.Second)
	newConn(t, "client1")

	<-time.After(1 * time.Minute)
}

func newServer(t *testing.T) {
	s := &Server{}
	sD := serverDelegate{
		addr:     ":12345",
		name:     "server",
		certFile: "server.crt",
		keyFile:  "server.key",
		mux:      map[string]func(http.ResponseWriter, *http.Request){},
		logger:   log.New(&logger{t}, "", log.LstdFlags),
	}
	if err := s.Init(&sD); err != nil {
		t.Fatal(err)
	}
	s.Start()
	defer func() {
		<-time.After(1 * time.Second)
		s.Stop()
		<-time.After(1 * time.Second)
	}()
	<-time.After(10 * time.Minute)
}

func newConn(t *testing.T, name string) {
	c := &Client{}
	cD := clientDelegate{
		name:     name,
		addr:     "127.0.0.1:12345",
		certFile: "server.crt",
		logger:   log.New(&logger{t}, "", log.LstdFlags),
	}
	if err := c.Init(&cD); err != nil {
		t.Fatal(err)
	}
	if err := c.Connect(); err != nil {
		t.Fatal(err)
	}
}
