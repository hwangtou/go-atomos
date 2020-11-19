package tcpacket

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"math"
	"net"
	"os"
	"sync"
	"syscall"
	"time"
)

var ErrReader = errors.New("tcpacket: Reader")
var ErrOverSize = errors.New("tcpacket: Over size")
var ErrHeaderSize = errors.New("tcpacket: Header size")

type Config struct {
	HeaderSize int
	Network string
	Address string
	HeartBeat uint
}

type Reader interface {
	RecvPacket(nonCopiedBuf []byte)
	RecvClose(err error)
}
type Writer interface {
	SendPacket(nonCopiedBuf []byte) error
	Close()
}
type NewConn func(writer Writer) (Reader, error)

type Hub struct {
	conf     Config
	acceptFn NewConn
	headSize int
	maxSize  int
	listener *net.TCPListener
}

func NewHub(c Config, accept NewConn) (h *Hub, err error) {
	// Check config
	if c.HeaderSize < 2 || 64 < c.HeaderSize {
		return nil, ErrHeaderSize
	}
	// Construct
	h = &Hub{
		conf: c,
		acceptFn: accept,
		headSize: c.HeaderSize,
		maxSize: int(math.Pow(2, float64(c.HeaderSize))),
	}
	log.Println("head", h.headSize, h.maxSize)
	addr, err := net.ResolveTCPAddr(c.Network, c.Address)
	if err != nil {
		return nil, err
	}
	if h.listener, err = net.ListenTCP(c.Network, addr); err != nil {
		return nil, err
	}
	return h, nil
}

func (h *Hub) Dial(network, address string, reader Reader) (Writer, error) {
	if reader == nil {
		return nil, ErrReader
	}
	addr, err := net.ResolveTCPAddr(network, address)
	if err != nil {
		return nil, err
	}
	c, err := net.DialTCP(network, nil, addr)
	if err != nil {
		return nil, err
	}
	conn := &tcpConn{
		Hub: h,
		TCPConn: c,
	}
	go h.handleConn(conn, reader)
	return conn, nil
}

func (h *Hub) Listen() {
	for {
		log.Println("listen")
		conn, err := h.listener.AcceptTCP()
		if err != nil {
			log.Println(err)
			return
		}
		go h.handleConn(&tcpConn{
			Hub: h,
			TCPConn: conn,
		}, nil)
	}
}

func (h *Hub) StopListen() error {
	return h.listener.Close()
}

type tcpConn struct {
	*Hub
	*net.TCPConn
	sync.Mutex
	active time.Time
}

func (c *tcpConn) SendPacket(nonCopiedBuf []byte) error {
	if len(nonCopiedBuf) >= c.Hub.maxSize {
		log.Println("send over size", len(nonCopiedBuf), c.Hub.maxSize)
		return ErrOverSize
	}
	bodySize := len(nonCopiedBuf)
	copiedBuf := make([]byte, c.Hub.headSize, c.Hub.headSize+bodySize)
	binary.PutUvarint(copiedBuf[:c.Hub.headSize], uint64(bodySize))
	b := append(copiedBuf, nonCopiedBuf...)
	log.Println("send", b)
	_, err := c.TCPConn.Write(b)
	c.active = time.Now()
	return err
}

func (c *tcpConn) Close() {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()
	log.Println("close")
	if c.TCPConn == nil {
		return
	}
	if err := c.TCPConn.Close(); err != nil {
		log.Println(err)
	}
	c.TCPConn = nil
	return
}

func (h *Hub) handleConn(conn *tcpConn, reader Reader) {
	log.Println("handleConn")
	c := conn.TCPConn
	var err error
	var fd *os.File
	var sndBufSize, rcvBufSize int
	if fd, err = c.File(); err != nil {
		log.Println(err)
		return
	}
	if sndBufSize, err = syscall.GetsockoptInt(int(fd.Fd()), syscall.SOL_SOCKET, syscall.SO_SNDBUF); err != nil {
		log.Println(err)
		return
	}
	if rcvBufSize, err = syscall.GetsockoptInt(int(fd.Fd()), syscall.SOL_SOCKET, syscall.SO_RCVBUF); err != nil {
		log.Println(err)
		return
	}
	log.Printf("tcpacket: New conn, snd=%v rcv=%v\n", sndBufSize, rcvBufSize)
	headNeed, headBuf := h.headSize, bytes.NewBuffer(make([]byte, 0, h.headSize))
	packNeed, packBuf := 0, bytes.NewBuffer(make([]byte, 0, rcvBufSize))
	if reader == nil {
		reader, err = h.acceptFn(conn)
		if err != nil {
			log.Println(err)
			return
		}
	}
	var readCache []byte
	if rcvBufSize > h.maxSize {
		readCache = make([]byte, h.maxSize)
	} else {
		readCache = make([]byte, rcvBufSize)
	}
	err = func() error {
		for {
			// read all to buffer
			readSize, err := c.Read(readCache[0:])
			conn.active = time.Now()
			log.Println("recv", readSize, readCache[0:readSize])
			// loop packet
			pos := 0
			for {
				log.Println("loop")
				if pos == readSize || readSize == 0 {
					log.Println("loop a")
					break
				}
				remain := readCache[pos:readSize]
				size := len(remain)
				// header
				if headNeed > 0 {
					if headNeed > size {
						headBuf.Write(remain)
						pos += size
						headNeed -= size
						packNeed = 0
						log.Println("loop b")
						break
					} else {
						headBuf.Write(remain[:headNeed])
						tmp := headBuf.Bytes()
						pn, _ := binary.Uvarint(tmp)
						headBuf.Reset()
						pos += headNeed
						headNeed = 0
						packNeed = int(pn)
						if packNeed == 0 {
							reader.RecvPacket(packBuf.Bytes())
							headNeed = h.headSize
							log.Println("loop c0")
						}
						log.Println("loop c")
						continue
					}
				}
				// body
				if packNeed > 0 {
					if packNeed > size {
						packBuf.Write(remain)
						pos += size
						packNeed -= size
						headNeed = 0
						log.Println("loop d")
						break
					} else {
						packBuf.Write(remain[:packNeed])
						reader.RecvPacket(packBuf.Bytes())
						packBuf.Reset()
						pos += packNeed
						packNeed = 0
						headNeed = h.headSize
						log.Println("loop e")
						continue
					}
				}
			}
			if err != nil {
				if err != io.EOF {
					log.Println(err)
				}
				log.Println("loop f")
				return err
			}
		}
	}()
	log.Println("tcpacket: Close conn")
	conn.Close()
	reader.RecvClose(err)
}
