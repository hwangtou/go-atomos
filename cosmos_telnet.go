package go_atomos

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"plugin"
	"reflect"
	"runtime/debug"
	"strings"
	"unicode"
)

type cosmosTelnet struct {
	self     *CosmosSelf
	enabled  bool
	started  bool
	listener net.Listener
	config   *TelnetServerConfig
}

func newCosmosTelnet(self *CosmosSelf) *cosmosTelnet {
	return &cosmosTelnet{
		self: self,
	}
}

func (t *cosmosTelnet) init() (err error) {
	// Config shortcut
	config := t.self.config
	// Enable Server
	if telnetConfig := config.EnableTelnet; telnetConfig != nil {
		t.self.logInfo("Cosmos.Init: Enable Telnet, network=%s,address=%s",
			config.EnableTelnet.Network, config.EnableTelnet.Address)
		t.enabled = true
		t.config = telnetConfig
		if cert := t.self.listenCert; cert != nil {
			t.listener, err = tls.Listen(telnetConfig.Network, telnetConfig.Address, cert)
		} else {
			t.listener, err = net.Listen(telnetConfig.Network, telnetConfig.Address)
		}
		if err != nil {
			return err
		}
		t.started = true
		go func() {
			defer func() {
				if r := recover(); r != nil {
					t.self.logFatal("Cosmos.Telnet: Panic, err=%v", r)
				}
			}()
			for {
				conn, err := t.listener.Accept()
				if err != nil {
					t.self.logFatal("Cosmos.Telnet: Accept failed, err=%v", err)
					return
				}
				go func(c net.Conn) {
					t.self.logFatal("Cosmos.Telnet: Accepted conn, conn=%s", c.RemoteAddr())
					defer func() {
						if r := recover(); r != nil {
							t.self.logFatal("Cosmos.Telnet: Conn panic, err=%v,stack=%s", r, string(debug.Stack()))
						}
					}()
					defer c.Close()
					c.Write([]byte("====================\n       ATOMOS       \n====================\n"))
					reader := bufio.NewReader(c)
					// Auth
					if admin := t.config.Admin; admin != nil {
						// Username
						c.Write([]byte("User: "))
						usernameByte, _, err := reader.ReadLine()
						if err != nil {
							c.Write([]byte("Read username failed.\n"))
							t.self.logFatal("Cosmos.Telnet: Read username failed, err=%v", err)
							return
						}
						username := string(usernameByte)
						if username == "" {
							c.Write([]byte("Username should not be blank.\n"))
							t.self.logFatal("Cosmos.Telnet: Username should not be blank.")
							return
						}
						// Passwd
						c.Write([]byte("Password: "))
						passwdByte, _, err := reader.ReadLine()
						if err != nil {
							c.Write([]byte("Read password failed.\n"))
							t.self.logFatal("Cosmos.Telnet: Read password failed, err=%v", err)
							return
						}
						passwd := string(passwdByte)
						// Auth
						if admin.Username != username || admin.Password != passwd {
							c.Write([]byte("Invalid username or password.\n"))
							t.self.logFatal("Cosmos.Telnet: Invalid username or password")
							return
						}
					}
					c.Write([]byte("\n       AUTHED       \n--------------------\n\n"))

					// Loop
					cmdLine := ""
					for {
						msg, isPrefix, err := reader.ReadLine()
						if err != nil {
							c.Write([]byte("Conn panic.\n"))
							t.self.logFatal("Cosmos.Telnet: Conn panic, err=%v", err)
							return
						}
						cmdLine += string(msg)
						if !isPrefix {
							t.self.logInfo("Telnet Command: %s", cmdLine)
							if err = t.handleCmd(cmdLine, c); err != nil {
								c.Write([]byte("Conn write closed.\n"))
								t.self.logError("Cosmos.Telnet: Conn write closed, err=%v", err)
								return
							}
							cmdLine = ""
						}
					}
				}(conn)
			}
		}()
	}
	return nil
}

func (t *cosmosTelnet) close() {
	if err := t.listener.Close(); err != nil {
		t.self.logError("Cosmos.Telnet: Close error, err=%v", err)
	}
}

func (t *cosmosTelnet) handleCmd(cmdLine string, conn net.Conn) error {
	var reply bytes.Buffer
	cmdSlice := strings.Split(cmdLine, " ")
	switch cmdSlice[0] {
	case "state":
		t.handleState(cmdSlice, &reply)
	case "element":
		t.handleElement(cmdSlice, &reply)
	case "atomos":
		t.handleAtomos(cmdSlice, &reply)
	case "reload":
		t.handleReload(cmdSlice, &reply)
	case "stop":
		t.handleStop(cmdSlice, &reply)
	case "remote":
		// TODO
	default:
		reply.WriteString("Invalid command\n")
		t.self.logWarn("Cosmos.Telnet: Invalid command, cmd=%s", cmdSlice[0])
	}
	reply.WriteString("\n")
	_, err := conn.Write(reply.Bytes())
	return err
}

func (t *cosmosTelnet) handleState(cmdSlice []string, reply *bytes.Buffer) {
	reply.WriteString(fmt.Sprintf("Running: %v\n", t.self.running))
	t.self.logInfo("Cosmos.Telnet: State\n%s", reply.String())
}

func (t *cosmosTelnet) handleElement(cmdSlice []string, reply *bytes.Buffer) {
	reply.WriteString(fmt.Sprintf("Elements List: \n"))
	for elemName, elem := range t.self.runtime.elements {
		reply.WriteString(fmt.Sprintf("%s:\n", elemName))
		reply.WriteString(fmt.Sprintf("-- Handlers\n"))
		for msgName, msg := range elem.current.Interface.AtomMessages {
			in, _ := msg.InDec([]byte(""))
			out, _ := msg.OutDec([]byte(""))
			reply.WriteString(fmt.Sprintf("---- %s : %T -> %T\n", msgName, in, out))
			inVal := reflect.ValueOf(in).Elem()
			for i := 0; i != inVal.NumField(); i += 1 {
				name := inVal.Type().Field(i).Name
				if len(name) == 0 || !unicode.IsUpper(rune(name[0])) {
					continue
				}
				reply.WriteString(fmt.Sprintf("------ %s : %T \n", name, inVal.Type().Field(i).Type.Name()))
			}
		}
	}
}

func (t *cosmosTelnet) handleAtomos(cmdSlice []string, reply *bytes.Buffer) {
	if len(cmdSlice) < 3 {
		reply.WriteString("Invalid command, use: atomos <element> (list/has/get/call/kill) <atomos>\n")
		return
	}
	elemName := cmdSlice[1]
	cmd := cmdSlice[2]
	elem, err := t.self.Local().getElement(elemName)
	if err != nil {
		reply.WriteString(fmt.Sprintf("Element not found, name=%s,err=%v\n", elemName, err))
		return
	}
	// Solo argument
	switch cmd {
	case "list":
		reply.WriteString(fmt.Sprintf("-- All atomos, num %d\n", elem.names.Len()))
		for atom := elem.names.Front(); atom != nil; atom = atom.Next() {
			v := atom.Value
			if v == nil {
				reply.WriteString(fmt.Sprintf("---- Invalid\n"))
				continue
			}
			atomName, ok := v.(string)
			if !ok {
				reply.WriteString(fmt.Sprintf("---- Invalid\n"))
				continue
			}
			a, err := elem.elementGetAtom(atomName)
			state := "Error"
			if a != nil {
				state = a.state.String()
			} else if err == ErrAtomNotFound {
				state = AtomosHalt.String()
			} else if err != nil {
				state = err.Error()
			}
			reply.WriteString(fmt.Sprintf("---- %s\t(%s)\n", atomName, state))
		}
		return
	}
	// Dual arguments
	if len(cmdSlice) < 4 {
		reply.WriteString("Invalid command, use: atomos <element> (list/has/get/call/kill) <atomos>\n")
		return
	}
	atomName := cmdSlice[3]
	atom, err := elem.elementGetAtom(atomName)
	if err != nil {
		reply.WriteString(fmt.Sprintf("Atomos not found, element=%s,name=%s,err=%v\n", elemName, atomName, err))
		return
	}
	switch cmd {
	case "has":
		reply.WriteString(fmt.Sprintf("Atomos found, element=%s,name=%s\n", elemName, atomName))
	case "get":
		reply.WriteString(fmt.Sprintf("Atomos, element=%s,name=%s,state=%v,mailbox=%d,tasks=%d,data=%+v\n",
			elemName, atomName, atom.state, atom.mailbox.num, atom.task.getTasksNum(), atom.instance))
	case "call":
		if len(cmdSlice) < 6 {
			reply.WriteString("Invalid call command, use: atomos <element> call <atomos> <message> <json_body>\n")
			return
		}
		reply.WriteString(fmt.Sprintf(cmdSlice[5]))

		handler, has := elem.current.Interface.AtomMessages[cmdSlice[4]]
		if !has {
			reply.WriteString("Invalid call command, unknown handler, use: atomos <element> call <atomos> <message> <json_body>\n")
			return
		}
		in, _ := handler.InDec([]byte(""))
		if err = json.Unmarshal([]byte(cmdSlice[5]), in); err != nil {
			reply.WriteString("Invalid call command, invalid json body.\n")
			return
		}
		resp, err := atom.pushMessageMail(t.self.runtime.mainAtom, cmdSlice[4], in)
		if err != nil {
			reply.WriteString(fmt.Sprintf("Invalid call command, err=%v.\n", err))
			return
		}
		reply.WriteString(fmt.Sprintf("Response: %+v.\n", resp))
	case "kill":
		if len(cmdSlice) < 4 {
			reply.WriteString("Invalid call command, use: atomos <element> kill <atomos>\n")
			return
		}
		err = atom.pushKillMail(t.self.runtime.mainAtom, true)
		if err != nil {
			reply.WriteString(fmt.Sprintf("Invalid kill command, err=%v.\n", err))
			return
		}
		reply.WriteString(fmt.Sprintf("Kill sent.\n"))
	default:
		reply.WriteString("Invalid command, use: atomos <element> (list/has/get/call/kill) <atomos>\n")
	}
}

func (t *cosmosTelnet) handleReload(cmdSlice []string, reply *bytes.Buffer) {
	if len(cmdSlice) < 2 {
		reply.WriteString("Invalid command, use: reload <path>\n")
		return
	}
	path := cmdSlice[1]
	plug, err := plugin.Open(path)
	if err != nil {
		reply.WriteString(fmt.Sprintf("Invalid runnable, err=%v\n", err))
		return
	}
	r, err := plug.Lookup(RunnableName)
	if err != nil {
		reply.WriteString(fmt.Sprintf("Invalid runnable, err=%v\n", err))
		return
	}
	runnable, ok := r.(*CosmosRunnable)
	if !ok || runnable == nil {
		reply.WriteString(fmt.Sprintf("Invalid runnable, runnable is nil.\n"))
		return
	}
	err = t.self.Send(NewRunnableUpdateCommand(runnable))
	if err != nil {
		reply.WriteString(fmt.Sprintf("Run runnable failed, err=%v.\n", err))
	}
}

func (t *cosmosTelnet) handleStop(cmdSlice []string, reply *bytes.Buffer) {
	err := t.self.Send(NewStopCommand())
	if err != nil {
		reply.WriteString(fmt.Sprintf("Stop failed, err=%v.\n", err))
	}
}
