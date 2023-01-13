package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/desertbit/grumble"
	"github.com/fatih/color"
	atomos "github.com/hwangtou/go-atomos"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"os"
	"os/user"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	configFile  = ".atomos_supervisor"
	historyFile = ".atomos_supervisor_commander.history"
)

type supervisorClient struct {
}

var app *grumble.App

func main() {
	// Env
	wd, er := os.UserHomeDir()
	if er != nil {
		wd = os.TempDir()
	}

	// Grumble
	app = grumble.New(&grumble.Config{
		Name:                  "Atomos Supervisor Commander",
		Description:           "A commander tool for Atomos Supervisor.",
		Flags:                 nil,
		HistoryFile:           path.Join(wd, historyFile),
		HistoryLimit:          100,
		NoColor:               false,
		Prompt:                "Commander » ",
		PromptColor:           color.New(color.FgBlue, color.Bold),
		MultiPrompt:           "",
		MultiPromptColor:      nil,
		ASCIILogoColor:        nil,
		ErrorColor:            nil,
		HelpHeadlineUnderline: true,
		HelpSubCommands:       true,
		HelpHeadlineColor:     color.New(color.FgBlue),
	})
	app.SetPrintASCIILogo(func(a *grumble.App) {
		_, _ = a.Println("Welcome to Atomos Supervisor Commander!")
		_, _ = a.Println()
		printEnv(a)
		_, _ = a.Println()
	})
	app.OnInit(func(a *grumble.App, flags grumble.FlagMap) error {
		// Config
		confPath = atomos.NewPath(path.Join(wd, configFile))
		if err := confPath.Refresh(); err != nil {
			return err
		}
		if confPath.Exist() {
			buf, er := ioutil.ReadFile(confPath.GetPath())
			if er != nil {
				return er
			}
			er = json.Unmarshal(buf, &conf)
			if er != nil {
				return er
			}
			return nil
		}
		conf.AtomosPrefix = "atomos_"
		conf.RunPath = "/var/run/"
		conf.RunPerm = 0775
		conf.LogPath = "/var/log/"
		conf.LogPerm = 0775
		conf.EtcPath = "/etc/"
		conf.EtcPerm = 0770
		buf, er := json.Marshal(&conf)
		if er != nil {
			return er
		}
		return ioutil.WriteFile(confPath.GetPath(), buf, 0777)
	})
	app.OnClose(func() error {
		buf, er := json.Marshal(&conf)
		if er != nil {
			return er
		}
		return ioutil.WriteFile(confPath.GetPath(), buf, 0777)
	})
	app.SetInterruptHandler(func(a *grumble.App, count int) {
		if count > 1 {
			os.Exit(0)
		}
		_, _ = a.Println("Press control-c once more to exit.")
	})

	app.AddCommand(&grumble.Command{
		Name:      "env",
		Aliases:   nil,
		Help:      "check or set supervisor environment",
		LongHelp:  "",
		HelpGroup: "",
		Usage:     "env --prefix atomos_ --etc_path /etc/ --log_path /var/log/ --run_path /var/run/",
		Flags: func(f *grumble.Flags) {
			f.String("p", "prefix", "", "Prefix of a cosmos in the config path.")
			f.String("r", "run_path", "", "Cosmos running path.")
			f.String("l", "log_path", "", "Cosmos logging path.")
			f.String("e", "etc_path", "", "Cosmos config path.")
		},
		Args: nil,
		Run: func(c *grumble.Context) error {
			prefix := c.Flags.String("prefix")
			runPath := c.Flags.String("run_path")
			logPath := c.Flags.String("log_path")
			etcPath := c.Flags.String("etc_path")
			if prefix != "" {
				conf.AtomosPrefix = prefix
			}
			if runPath != "" {
				conf.RunPath = runPath
			}
			if logPath != "" {
				conf.LogPath = logPath
			}
			if etcPath != "" {
				conf.EtcPath = etcPath
			}
			buf, er := json.MarshalIndent(&conf, "", " ")
			if er != nil {
				return er
			}
			printEnv(app)
			return ioutil.WriteFile(confPath.GetPath(), buf, 0777)
		},
		Completer: nil,
	})

	app.AddCommand(&grumble.Command{
		Name:      "init",
		Aliases:   nil,
		Help:      "initialize a cosmos",
		LongHelp:  "",
		HelpGroup: "",
		Usage:     "init --user={whoami} --group={staff} --cosmos={cosmos_name} --nodes={node1,node2,node3...}",
		Flags: func(f *grumble.Flags) {
			f.String("u", "user", "", "User allows to access the cosmos, current user by default.")
			f.String("g", "group", "", "Group allows to access the cosmos.")
			f.String("c", "cosmos", "", "Name of the cosmos.")
			f.String("n", "nodes", "", "Names list of nodes, seperated by ','.")
		},
		Args: nil,
		Run: func(c *grumble.Context) error {
			username := c.Flags.String("user")
			group := c.Flags.String("group")
			cosmosName := c.Flags.String("cosmos")
			nodeNameList := strings.Split(c.Flags.String("nodes"), ",")
			if username == "" {
				current, er := user.Current()
				if er != nil {
					_, _ = app.Println("Get default user failed,", er)
					return er
				}
				username = current.Username
			}
			if len(nodeNameList) == 1 && nodeNameList[0] == "" {
				nodeNameList = nil
			}
			return initPaths(app, username, group, cosmosName, nodeNameList)
		},
		Completer: nil,
	})

	app.AddCommand(&grumble.Command{
		Name:      "validate",
		Aliases:   nil,
		Help:      "validate a cosmos",
		LongHelp:  "",
		HelpGroup: "",
		Usage:     "validate --group={staff} --cosmos={cosmos_name} --nodes={node1,node2,node3...}",
		Flags: func(f *grumble.Flags) {
			f.String("g", "group", "", "Group allows to access the cosmos.")
			f.String("c", "cosmos", "", "Name of the cosmos.")
			f.String("n", "nodes", "", "Names list of nodes, seperated by ','.")
		},
		Args: nil,
		Run: func(c *grumble.Context) error {
			group := c.Flags.String("group")
			cosmosName := c.Flags.String("cosmos")
			nodeNameList := strings.Split(c.Flags.String("nodes"), ",")
			if group == "" {
				return errors.New("we have to know which group is allowed to access, please give a group name with -g argument")
			}
			return validatePaths(app, group, cosmosName, nodeNameList)
		},
		Completer: nil,
	})

	app.AddCommand(&grumble.Command{
		Name:      "supervisor",
		Aliases:   nil,
		Help:      "supervisor of a cosmos",
		LongHelp:  "",
		HelpGroup: "",
		Usage:     "supervisor --cosmos {cosmos_name}",
		Flags: func(f *grumble.Flags) {
			f.String("c", "cosmos", "", "Name of the cosmos.")
		},
		Args: nil,
		Run: func(c *grumble.Context) error {
			cosmosName := c.Flags.String("cosmos")
			s, err := getSupervisor(cosmosName)
			if err != nil {
				return err
			}
			//s.cmdCh
			_ = s
			return nil
		},
		Completer: nil,
	})

	//app.AddCommand(&grumble.Command{
	//	Name:      "status",
	//	Aliases:   nil,
	//	Help:      "status of all cosmoses",
	//	LongHelp:  "",
	//	HelpGroup: "",
	//	Usage:     "status",
	//	Flags:     nil,
	//	Args:      nil,
	//	Run: func(c *grumble.Context) error {
	//		configs, err := allCosmosSupervisorConfig()
	//		if err != nil {
	//			return err
	//		}
	//		for _, supervisorConf := range configs {
	//			supervisorsMutex.Lock()
	//			watcher, has := supervisorsMap[supervisorConf.Cosmos]
	//			if !has {
	//				watcher = &supervisorWatcher{
	//					config: supervisorConf,
	//					status: supervisorStatusInit,
	//					client: atomos.NewAppUDSClient(),
	//				}
	//				supervisorsMap[supervisorConf.Cosmos] = watcher
	//			}
	//			supervisorsMutex.Unlock()
	//			go watcher.Watch()
	//		}
	//		return nil
	//	},
	//	Completer: nil,
	//})

	//app.AddCommand(&grumble.Command{
	//	Name:      "test",
	//	Aliases:   nil,
	//	Help:      "testing",
	//	LongHelp:  "",
	//	HelpGroup: "",
	//	Usage:     "test",
	//	Flags:     nil,
	//	Args:      nil,
	//	Run: func(c *grumble.Context) error {
	//		supervisorsMutex.Lock()
	//		defer supervisorsMutex.Unlock()
	//		//supervisorsMap[]
	//		return nil
	//	},
	//	Completer: nil,
	//})

	grumble.Main(app)
}

// Supervisor

var supervisorsMap = map[string]*supervisorWatcher{}
var supervisorsMutex sync.Mutex

type supervisorWatcher struct {
	config *atomos.SupervisorYAMLConfig
	status int32
	client *atomos.AppUDSClient
	cmdCh  chan *supervisorCmd
}

type supervisorCmd struct {
	command  string
	buf      []byte
	callback atomos.AppUDSClientCallback
	timeout  time.Duration
}

func (w *supervisorWatcher) Watch() {
	if !atomic.CompareAndSwapInt32(&w.status, supervisorStatusInit, supervisorStatusNotRunning) {
		return
	}
	go func() {
		for {
			var conn *atomos.AppUDSConn
			var err *atomos.Error
			var lastSent time.Time
			conn, err = w.client.Dial(path.Join(w.config.RunPath, "supervisor.socket"))
			if err != nil {
				logging(atomos.LogLevel_Warn, "Commander: Supervisor dial failed. err=(%v)", err.AddStack(nil))
				goto wait
			}

			for {
				select {
				case <-time.After(2 * time.Second):
					if conn.IsClosed() {
						goto wait
					}
					if time.Now().Sub(lastSent) > 2*time.Second {
						w.client.Send(conn, atomos.UDSPing, []byte("ping"), 1*time.Second, func(packet *atomos.UDSCommandPacket, err *atomos.Error) {
							if err != nil {
								conn.DaemonClose()
								logging(atomos.LogLevel_Debug, "Commander: Supervisor watcher ping-pong error. name=(%s),err=(%v)", w.config.Cosmos, err.AddStack(nil))
							} else {
								logging(atomos.LogLevel_Debug, "Commander: Supervisor watcher ping-pong. name=(%s),packet=(%v)", w.config.Cosmos, packet)
							}
						})
					}
					lastSent = time.Now()
				case cmd, ok := <-w.cmdCh:
					if conn.IsClosed() || !ok {
						goto wait
					}
					w.client.Send(conn, cmd.command, cmd.buf, cmd.timeout, func(packet *atomos.UDSCommandPacket, err *atomos.Error) {
						if err != nil {
							conn.DaemonClose()
							logging(atomos.LogLevel_Debug, "Commander: Supervisor watcher send error. name=(%s),err=(%v)", w.config.Cosmos, err.AddStack(nil))
						} else {
							logging(atomos.LogLevel_Debug, "Commander: Supervisor watcher send. name=(%s)", w.config.Cosmos)
						}
						cmd.callback(packet, err)
					})
					lastSent = time.Now()
				case <-conn.CloseCh():
					goto wait
				}
			}
		wait:
			<-time.After(100 * time.Millisecond)
		}
	}()
}

const (
	supervisorStatusInit       int32 = 0
	supervisorStatusNotRunning int32 = 1
	supervisorStatusRunning    int32 = 2
)

// Config

type config struct {
	AtomosPrefix string `json:"atomos_prefix"`

	RunPath string      `json:"run_path"`
	RunPerm os.FileMode `json:"run_perm"`

	LogPath string      `json:"log_path"`
	LogPerm os.FileMode `json:"log_perm"`

	EtcPath string      `json:"etc_path"`
	EtcPerm os.FileMode `json:"etc_perm"`
}

var confPath *atomos.Path
var conf config

func getRunPath(cosmosName string) string {
	return path.Join(conf.RunPath, conf.AtomosPrefix+cosmosName)
}

func getLogPath(cosmosName string) string {
	return path.Join(conf.LogPath, conf.AtomosPrefix+cosmosName)
}

func getEtcPath(cosmosName string) string {
	return path.Join(conf.EtcPath, conf.AtomosPrefix+cosmosName)
}

func getSupervisorConfig(cosmosName string) (*atomos.SupervisorYAMLConfig, *atomos.Error) {
	supervisorConfigPath := atomos.NewPath(path.Join(getEtcPath(cosmosName), "/supervisor.conf"))
	if err := supervisorConfigPath.Refresh(); err != nil {
		return nil, err.AddStack(nil)
	}
	if !supervisorConfigPath.Exist() {
		return nil, atomos.NewError(atomos.ErrSupervisorReadSupervisorConfigFailed, "Commander: Supervisor config not exists.").AddStack(nil)
	}
	buf, er := ioutil.ReadFile(supervisorConfigPath.GetPath())
	if er != nil {
		return nil, atomos.NewErrorf(atomos.ErrSupervisorReadSupervisorConfigFailed, "Commander: Supervisor read file failed. err=(%v)", er).AddStack(nil)
	}
	supervisorConfig := &atomos.SupervisorYAMLConfig{}
	er = yaml.Unmarshal(buf, supervisorConfig)
	if er != nil {
		return nil, atomos.NewErrorf(atomos.ErrSupervisorReadSupervisorConfigFailed, "Commander: Supervisor unmarshal config failed. err=(%v)", er).AddStack(nil)
	}
	if supervisorConfig.Cosmos != cosmosName {
		return nil, atomos.NewError(atomos.ErrSupervisorReadSupervisorConfigFailed, "Commander: Supervisor config name not match.").AddStack(nil)
	}
	return supervisorConfig, nil
}

func getSupervisor(cosmosName string) (s *supervisorWatcher, err *atomos.Error) {
	supervisorsMutex.Lock()
	s, has := supervisorsMap[cosmosName]
	if !has {
		s = &supervisorWatcher{
			config: nil,
			status: supervisorStatusInit,
			client: atomos.NewAppUDSClient(logging),
		}
		s.config, err = getSupervisorConfig(cosmosName)
		if err == nil {
			supervisorsMap[cosmosName] = s
		}
	}
	supervisorsMutex.Unlock()
	if err != nil {
		return nil, err
	}
	if !has {
		s.Watch()
	}
	return s, nil
}

func logging(level atomos.LogLevel, format string, args ...interface{}) {
	_, _ = app.Printf("["+atomos.LogLevel_name[int32(level)]+"]\t"+format+"\n", args...)
}

// Env

func printEnv(app *grumble.App) {
	_, _ = app.Printf("Environment Info:\n")
	if os.Getuid() == 0 {
		_, _ = app.Println("Commander now is running under root!")
	}
	_, _ = app.Printf("Config Path: (%v) %s%s\n", conf.EtcPerm, conf.EtcPath, envWarnInfo(conf.EtcPath))
	_, _ = app.Printf("Run Path:    (%v) %s%s\n", conf.RunPerm, conf.RunPath, envWarnInfo(conf.RunPath))
	_, _ = app.Printf("Log Path:    (%v) %s%s\n", conf.LogPerm, conf.LogPath, envWarnInfo(conf.LogPath))
}

func envWarnInfo(path string) string {
	p := atomos.NewPath(path)
	if err := p.Refresh(); err != nil {
		return fmt.Sprintf(" [Path Error: %v]", err)
	}
	if !p.Exist() {
		return " [Path Not Found: Please make the directory.]"
	}
	if !p.IsDir() {
		return " [Path Not Dir: Ensure the path is directory.]"
	}
	return ""
}

// Init

func initPaths(a *grumble.App, cosmosUser, cosmosGroup, cosmosName string, nodeNameList []string) error {
	//if os.Getuid() != 0 {
	//	return errors.New("init supervisor should run under root")
	//}
	// Check user.
	u, er := user.Lookup(cosmosUser)
	if er != nil {
		if _, ok := er.(user.UnknownUserError); ok {
			_, _ = a.Println("unknown user, err=", er)
			return er
		}
		_, _ = a.Println("lookup user failed, err=", er)
		return er
	}
	// Check group.
	g, er := user.LookupGroup(cosmosGroup)
	if er != nil {
		if _, ok := er.(*user.UnknownGroupError); ok {
			_, _ = a.Println("unknown group, err=", er)
			return er
		}
		_, _ = a.Println("lookup group failed, err=", er)
		return er
	}
	userGroups, er := u.GroupIds()
	if er != nil {
		_, _ = a.Println("iter user group failed, err=", er)
		return er
	}
	inGroup := false
	for _, gid := range userGroups {
		if gid == g.Gid {
			inGroup = true
			break
		}
	}
	if !inGroup {
		_, _ = a.Println("user not in group")
		return er
	}
	if err := initSupervisorPaths(cosmosName, nodeNameList, u, g); err != nil {
		_, _ = a.Println(err)
		return err
	}
	for _, nodeName := range nodeNameList {
		if err := initNodePaths(cosmosName, nodeName, u, g); err != nil {
			_, _ = a.Println(err)
			return err
		}
	}
	_, _ = a.Println("Paths inited")
	return nil
}

func initSupervisorPaths(cosmosName string, nodeNameList []string, u *user.User, g *user.Group) error {
	// Check whether cosmos name is legal.
	// 检查cosmos名称是否合法。
	if !atomos.CheckCosmosName(cosmosName) {
		return errors.New("invalid cosmos name")
	}

	// Check whether /var/run/atomos_{cosmosName} directory exists or not.
	// 检查/var/run/atomos_{cosmosName}目录是否存在。
	if err := atomos.NewPath(getRunPath(cosmosName)).CreateDirectoryIfNotExist(u, g, conf.RunPerm); err != nil {
		return err
	}
	// Check whether /var/log/atomos_{cosmosName} directory exists or not.
	// 检查/var/log/atomos_{cosmosName}目录是否存在。
	if err := atomos.NewPath(getLogPath(cosmosName)).CreateDirectoryIfNotExist(u, g, conf.LogPerm); err != nil {
		return err
	}
	if err := atomos.NewPath(path.Join(getLogPath(cosmosName), "supervisor")).CreateDirectoryIfNotExist(u, g, conf.LogPerm); err != nil {
		return err
	}
	// Check whether /etc/atomos_{cosmosName} directory exists or not.
	// 检查/etc/atomos_{cosmosName}目录是否存在。
	if err := atomos.NewPath(getEtcPath(cosmosName)).CreateDirectoryIfNotExist(u, g, conf.EtcPerm); err != nil {
		return err
	}
	// Create supervisor.conf
	supervisorConfigPath := atomos.NewPath(path.Join(getEtcPath(cosmosName), "/supervisor.conf"))
	if err := supervisorConfigPath.Refresh(); err != nil {
		return err
	}
	supervisorConfig := &atomos.SupervisorYAMLConfig{
		Cosmos:            cosmosName,
		NodeList:          nodeNameList,
		KeepaliveNodeList: nil,
		ReporterUrl:       "",
		ConfigerUrl:       "",
		LogLevel:          atomos.LogLevel_name[int32(atomos.LogLevel_Debug)],
		LogPath:           path.Join(getLogPath(cosmosName), "supervisor"),
		RunPath:           getRunPath(cosmosName),
		EtcPath:           getEtcPath(cosmosName),
	}
	buf, er := yaml.Marshal(supervisorConfig)
	if er != nil {
		return er
	}
	if err := supervisorConfigPath.CreateFileIfNotExist(buf, conf.EtcPerm); err != nil {
		return err
	}
	return nil
}

func initNodePaths(cosmosName, nodeName string, u *user.User, g *user.Group) error {
	// Check whether cosmos name is legal.
	// 检查cosmos名称是否合法。
	if !atomos.CheckNodeName(nodeName) {
		return errors.New("invalid node name")
	}

	// Check whether /var/log/atomos_{cosmosName} directory exists or not.
	// 检查/var/log/atomos_{cosmosName}目录是否存在。
	if err := atomos.NewPath(path.Join(getLogPath(cosmosName), nodeName)).CreateDirectoryIfNotExist(u, g, conf.LogPerm); err != nil {
		return err
	}
	// Check whether /etc/atomos_{cosmosName} directory exists or not.
	// 检查/etc/atomos_{cosmosName}目录是否存在。
	if err := atomos.NewPath(path.Join(getEtcPath(cosmosName), nodeName)).CreateDirectoryIfNotExist(u, g, conf.EtcPerm); err != nil {
		return err
	}
	// Create {node}.conf
	nodeConfPath := atomos.NewPath(path.Join(getEtcPath(cosmosName), nodeName+".conf"))
	if err := nodeConfPath.Refresh(); err != nil {
		return err
	}
	nodeConf := &atomos.NodeYAMLConfig{
		Cosmos:          cosmosName,
		Node:            nodeName,
		ReporterUrl:     "",
		ConfigerUrl:     "",
		LogLevel:        atomos.LogLevel_name[int32(atomos.LogLevel_Debug)],
		LogPath:         path.Join(getLogPath(cosmosName), nodeName),
		LogMaxSize:      10000000,
		BuildPath:       "",
		BinPath:         "",
		RunPath:         getRunPath(cosmosName),
		EtcPath:         getEtcPath(cosmosName),
		EnableCert:      nil,
		EnableServer:    nil,
		CustomizeConfig: map[string][]byte{},
	}
	buf, er := yaml.Marshal(nodeConf)
	if er != nil {
		return er
	}
	if err := nodeConfPath.CreateFileIfNotExist(buf, conf.EtcPerm); err != nil {
		return err
	}
	return nil
}

// Validate

func validatePaths(a *grumble.App, cosmosGroup, cosmosName string, nodeNameList []string) error {
	u, er := user.Current()
	if er != nil {
		return er
	}
	// Check group.
	g, er := user.LookupGroup(cosmosGroup)
	if er != nil {
		if _, ok := er.(*user.UnknownGroupError); ok {
			_, _ = a.Println("unknown group, err=", er)
			return er
		}
		_, _ = a.Println("lookup group failed, err=", er)
		return er
	}
	userGroups, er := u.GroupIds()
	if er != nil {
		_, _ = a.Println("iter user group failed, err=", er)
		return er
	}
	inGroup := false
	for _, gid := range userGroups {
		if gid == g.Gid {
			inGroup = true
			break
		}
	}
	if !inGroup {
		_, _ = a.Println("user not in group")
		return er
	}
	if er := validateSupervisorPath(a, cosmosName, u); er != nil {
		return er
	}
	for _, nodeName := range nodeNameList {
		if er := validateNodePath(a, cosmosName, nodeName, u); er != nil {
			return er
		}
	}
	_, _ = a.Println("Paths checked")
	return nil
}

func validateSupervisorPath(a *grumble.App, cosmosName string, u *user.User) error {
	// Check whether /var/log/atomos_{cosmosName} directory exists or not.
	// 检查/var/log/atomos_{cosmosName}目录是否存在。
	if err := atomos.NewPath(getLogPath(cosmosName)).CheckDirectoryOwnerAndMode(u, conf.LogPerm); err != nil {
		return err
	}
	// Check whether /etc/atomos_{cosmosName} directory exists or not.
	// 检查/etc/atomos_{cosmosName}目录是否存在。
	if err := atomos.NewPath(getEtcPath(cosmosName)).CheckDirectoryOwnerAndMode(u, conf.EtcPerm); err != nil {
		return err
	}
	// Get supervisor.conf
	supervisorConfigPath := atomos.NewPath(path.Join(getEtcPath(cosmosName), "/supervisor.conf"))
	if err := supervisorConfigPath.Refresh(); err != nil {
		return err
	}
	if !supervisorConfigPath.Exist() {
		return errors.New("file not exist")
	}
	buf, er := ioutil.ReadFile(supervisorConfigPath.GetPath())
	if er != nil {
		return er
	}
	supervisorConfig := &atomos.SupervisorYAMLConfig{}
	er = yaml.Unmarshal(buf, supervisorConfig)
	if er != nil {
		return er
	}
	if supervisorConfig.Cosmos != cosmosName {
		return errors.New("invalid cosmos name")
	}
	if supervisorConfig.LogPath != getLogPath(cosmosName) {
		_, _ = a.Println("log path has changed to", supervisorConfig.LogPath)
	}
	return nil
}

func validateNodePath(a *grumble.App, cosmosName, nodeName string, u *user.User) error {
	// Check whether cosmos name is legal.
	// 检查cosmos名称是否合法。
	if !atomos.CheckNodeName(cosmosName) {
		return errors.New("invalid node name")
	}
	// Check whether /var/log/atomos_{cosmosName} directory exists or not.
	// 检查/var/log/atomos_{cosmosName}目录是否存在。
	if err := atomos.NewPath(path.Join(getLogPath(cosmosName), nodeName)).CheckDirectoryOwnerAndMode(u, conf.LogPerm); err != nil {
		return err
	}
	// Check whether /etc/atomos_{cosmosName} directory exists or not.
	// 检查/etc/atomos_{cosmosName}目录是否存在。
	if err := atomos.NewPath(path.Join(getEtcPath(cosmosName), nodeName)).CheckDirectoryOwnerAndMode(u, conf.EtcPerm); err != nil {
		return err
	}
	// Get {node}.conf
	nodeConfPath := atomos.NewPath(path.Join(getEtcPath(cosmosName), nodeName+".conf"))
	if err := nodeConfPath.Refresh(); err != nil {
		return err
	}
	if !nodeConfPath.Exist() {
		return errors.New("file not exist")
	}
	buf, er := ioutil.ReadFile(nodeConfPath.GetPath())
	if er != nil {
		return er
	}
	nodeConf := &atomos.NodeYAMLConfig{}
	er = yaml.Unmarshal(buf, nodeConf)
	if er != nil {
		return er
	}
	if nodeConf.Cosmos != cosmosName {
		return errors.New("invalid cosmos name")
	}
	if nodeConf.Node != nodeName {
		return errors.New("invalid cosmos name")
	}
	if nodeConf.LogPath != path.Join(getLogPath(cosmosName), nodeName) {
		_, _ = a.Println("log path has changed to", nodeConf.LogPath)
	}
	return nil
}

// Status

func allCosmosSupervisorConfig() ([]*atomos.SupervisorYAMLConfig, *atomos.Error) {
	ePath := atomos.NewPath(conf.EtcPath)
	if err := ePath.Refresh(); err != nil {
		return nil, err
	}
	paths, err := ePath.ListDirectory()
	if err != nil {
		return nil, err
	}
	var cosmosPaths []*atomos.SupervisorYAMLConfig
	for _, p := range paths {
		if !p.IsDir() {
			continue
		}
		if !strings.HasPrefix(p.Name(), conf.AtomosPrefix) {
			continue
		}
		supervisorConfigPath := atomos.NewPath(path.Join(ePath.GetPath(), p.Name(), "/supervisor.conf"))
		if err = supervisorConfigPath.Refresh(); err != nil {
			return nil, err
		}
		if !supervisorConfigPath.Exist() {
			return nil, err
		}
		buf, er := ioutil.ReadFile(supervisorConfigPath.GetPath())
		if er != nil {
			return nil, err
		}
		supervisorConfig := &atomos.SupervisorYAMLConfig{}
		er = yaml.Unmarshal(buf, supervisorConfig)
		if er != nil {
			return nil, err
		}
		cosmosPaths = append(cosmosPaths, supervisorConfig)
	}
	return cosmosPaths, nil
}
