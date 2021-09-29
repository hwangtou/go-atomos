package go_atomos

import "errors"

type Wormhole interface {
	Atom
	Accept()
	Close()
}

type WormholeId interface {
	Id
	SetWormholeConn(from Id, conn interface{}) error
}

type WormholeSelf interface {
	AtomSelf
	SetWormholeLooper(WormholeReader)
}

type WormholeConn interface {
	Send([]byte) error
	Close() error
}

// Wormhole

func (a *AtomCore) pushWormholeMail(data interface{}, err error) error {
	am := allocAtomMail()
	initWormholeMail(am, data, err)
	if ok := a.mailbox.PushTail(am.Mail); !ok {
		return ErrAtomIsNotRunning
	}
	_, er := am.waitReply()
	deallocAtomMail(am)
	return er
}

func (a *AtomCore) handleWormhole(am *atomMail) error {

}

func (a *AtomCore) handleSetWormholeConn(id Id, conn interface{}) error {
	wa, ok := a.instance.(WormholeAtom)
	if !ok {
		return errors.New("atom cannot set conn")
	}
	return wa.SetWormholeConn(id, conn)
}

func (a *AtomCore) SetWormholeLooper(reader WormholeReader) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
			}
		}()
		for {
			if err := reader(); err != nil {
				return
			}
		}
	}()
}
