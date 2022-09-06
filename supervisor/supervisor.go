package main

import (
	"errors"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"strconv"
	"syscall"
)

func main() {
	//process, err := os.StartProcess("sample_process", nil, &os.ProcAttr{})
	//if err != nil {
	//	panic(err)
	//}
	//time.Sleep(2 * time.Second)
	//process.Signal(os.Interrupt)
	//log.Println(process)
	//time.Sleep(10 * time.Second)

	if len(os.Args) == 0 {
		log.Println("Welcome to Atomos Supervisor!")
		log.Println("Usage: init|check_config|status")
		return
	}
	switch os.Args[1] {
	case "init":
		if er := initPaths(u, g); er != nil {
			log.Println("Paths init failed:", er)
			return
		}
	case "check_config":
		if er := checkConfigs(); er != nil {
			log.Println("Check configs failed:", er)
			return
		}
	default:
		log.Println("Wrong Usage:", os.Args)
	}
}

const (
	name = "hello_world"
	u    = "hwangtou"
	g    = "admin"
)

var (
	list = []string{"user_wormhole", "user_core"}
)

func checkCosmosName(cosmosName string) bool {
	if cosmosName == "" {
		return false
	}
	for _, c := range cosmosName {
		if !('0' <= c && c <= '9' || 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || c == '.' || c == '_') {
			return false
		}
	}
	return true
}

func checkProcessName(processName string) bool {
	if processName == "" {
		return false
	}
	for _, c := range processName {
		if !('0' <= c && c <= '9' || 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || c == '.' || c == '_') {
			return false
		}
	}
	return true
}

// Init Supervisor

const (
	AtomosPrefix = "atomos_"

	VarRunPath = "/var/run/"
	VarRunPerm = 0775

	VarLogPath = "/var/log/"
	VarLogPerm = 0775

	EtcPath = "/etc/"
	EtcPerm = 0770
)

func initPaths(cosmosUser, cosmosGroup string) error {
	if os.Getuid() != 0 {
		return errors.New("init supervisor should run under root")
	}
	// Check user.
	u, er := user.Lookup(cosmosUser)
	if er != nil {
		if _, ok := er.(user.UnknownUserError); ok {
			log.Println("unknown user, err=", er)
			return er
		}
		log.Println("lookup user failed, err=", er)
		return er
	}
	// Check group.
	g, er := user.LookupGroup(cosmosGroup)
	if er != nil {
		if _, ok := er.(*user.UnknownGroupError); ok {
			log.Println("unknown group, err=", er)
			return er
		}
		log.Println("lookup group failed, err=", er)
		return er
	}
	userGroups, er := u.GroupIds()
	if er != nil {
		log.Println("iter user group failed, err=", er)
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
		log.Println("user not in group")
		return er
	}
	if er := initSupervisorPaths(name, u, g); er != nil {
		log.Println(er)
		return er
	}
	for _, proc := range list {
		if er := initProcessesPaths(name, proc, u, g); er != nil {
			log.Println(er)
			return er
		}
	}
	log.Println("Paths inited")
	return nil
}

func initSupervisorPaths(cosmosName string, u *user.User, g *user.Group) error {
	// Check whether cosmos name is legal.
	// 检查cosmos名称是否合法。
	if !checkCosmosName(name) {
		return errors.New("invalid cosmos name")
	}

	// Check whether /var/run/atomos_{cosmosName} directory exists or not.
	// 检查/var/run/atomos_{cosmosName}目录是否存在。
	if er := NewPath(VarRunPath+AtomosPrefix+cosmosName).CreateDirectoryIfNotExist(u, g, VarRunPerm); er != nil {
		return er
	}
	// Check whether /var/log/atomos_{cosmosName} directory exists or not.
	// 检查/var/log/atomos_{cosmosName}目录是否存在。
	if er := NewPath(VarLogPath+AtomosPrefix+cosmosName).CreateDirectoryIfNotExist(u, g, VarLogPerm); er != nil {
		return er
	}
	// Check whether /etc/atomos_{cosmosName} directory exists or not.
	// 检查/etc/atomos_{cosmosName}目录是否存在。
	if er := NewPath(EtcPath+AtomosPrefix+cosmosName).CreateDirectoryIfNotExist(u, g, EtcPerm); er != nil {
		return er
	}
	// Create supervisor.conf
	confPath := NewPath(EtcPath + AtomosPrefix + cosmosName + "/supervisor.conf")
	if er := confPath.Refresh(); er != nil {
		return er
	}
	conf := &SupervisorConfig{
		CosmosName: cosmosName,
		LogPath:    VarLogPath + AtomosPrefix + cosmosName,
		LogLevel:   "DEBUG",
	}
	buf, er := yaml.Marshal(conf)
	if er != nil {
		return er
	}
	if er := confPath.CreateFileIfNotExist(buf, EtcPerm); er != nil {
		return er
	}
	return nil
}

func initProcessesPaths(cosmosName, processName string, u *user.User, g *user.Group) error {
	// Check whether cosmos name is legal.
	// 检查cosmos名称是否合法。
	if !checkProcessName(name) {
		return errors.New("invalid process name")
	}

	// Check whether /var/run/atomos_{cosmosName} directory exists or not.
	// 检查/var/run/atomos_{cosmosName}目录是否存在。
	if er := NewPath(VarRunPath+AtomosPrefix+cosmosName+"/"+processName).CreateDirectoryIfNotExist(u, g, VarRunPerm); er != nil {
		return er
	}
	// Check whether /var/log/atomos_{cosmosName} directory exists or not.
	// 检查/var/log/atomos_{cosmosName}目录是否存在。
	if er := NewPath(VarLogPath+AtomosPrefix+cosmosName+"/"+processName).CreateDirectoryIfNotExist(u, g, VarLogPerm); er != nil {
		return er
	}
	// Check whether /etc/atomos_{cosmosName} directory exists or not.
	// 检查/etc/atomos_{cosmosName}目录是否存在。
	if er := NewPath(EtcPath+AtomosPrefix+cosmosName+"/"+processName).CreateDirectoryIfNotExist(u, g, EtcPerm); er != nil {
		return er
	}
	// Create {process}.conf
	confPath := NewPath(EtcPath + AtomosPrefix + cosmosName + "/" + processName + ".conf")
	if er := confPath.Refresh(); er != nil {
		return er
	}
	conf := &SupervisorConfig{
		CosmosName: cosmosName,
		LogPath:    VarLogPath + AtomosPrefix + cosmosName + "/" + processName,
		LogLevel:   "DEBUG",
	}
	buf, er := yaml.Marshal(conf)
	if er != nil {
		return er
	}
	if er := confPath.CreateFileIfNotExist(buf, EtcPerm); er != nil {
		return er
	}
	return nil
}

func checkConfigs() error {
	if er := checkSupervisorConfig(name); er != nil {
		return er
	}
	for _, proc := range list {
		if er := checkProcessesConfig(name, proc); er != nil {
			return er
		}
	}
	return nil
}

func checkSupervisorConfig(cosmosName string) error {
	if !NewPath(EtcPath + AtomosPrefix + cosmosName + "/supervisor.conf").exist() {
		return errors.New("supervisor config not exist")
	}
	return nil
}

func checkProcessesConfig(cosmosName, procName string) error {
	if !NewPath(VarRunPath + AtomosPrefix + cosmosName + "/" + procName + ".conf").exist() {
		return errors.New("supervisor config not exist")
	}
	return nil
}

// Path

type Path struct {
	os.FileInfo
	path string
}

func NewPath(path string) *Path {
	return &Path{path: path}
}

func (p *Path) Refresh() error {
	stat, er := os.Stat(p.path)
	if er != nil {
		if !os.IsNotExist(er) {
			return er
		}
	}
	p.FileInfo = stat
	return nil
}

func (p *Path) CreateDirectoryIfNotExist(u *user.User, g *user.Group, perm os.FileMode) error {
	// If it is not exists, create it. If it is exists and not a directory, return failed.
	if er := p.Refresh(); er != nil {
		log.Println("refresh failed, err=", er)
		return er
	}
	if !p.exist() {
		if er := p.makeDir(perm); er != nil {
			log.Println("make dir failed, err=", er)
			return er
		}
		if er := p.changeOwnerAndMode(u, g, perm); er != nil {
			log.Println("change owner and mode failed, err=", er)
			return er
		}
	} else if !p.IsDir() {
		log.Println("path should be dir")
		return errors.New("path should be directory")
	} else {
		// Check its owner and mode.
		if er := p.confirmOwnerAndMode(g, perm); er != nil {
			log.Println("confirm owner and mode failed, err=", er)
			return er
		}
	}
	return nil
}

func (p *Path) exist() bool {
	return p.FileInfo != nil
}

func (p *Path) makeDir(mode os.FileMode) error {
	return os.Mkdir(p.path, mode)
}

func (p *Path) changeOwnerAndMode(u *user.User, group *user.Group, perm os.FileMode) error {
	gid, er := strconv.ParseInt(group.Gid, 10, 64)
	if er != nil {
		return er
	}
	uid, er := strconv.ParseInt(u.Uid, 10, 64)
	if er != nil {
		return er
	}
	if er = os.Chown(p.path, int(uid), int(gid)); er != nil {
		return er
	}
	if er = os.Chmod(p.path, perm); er != nil {
		return er
	}
	return nil
}

func (p *Path) confirmOwnerAndMode(group *user.Group, perm os.FileMode) error {
	// Owner
	switch stat := p.Sys().(type) {
	case *syscall.Stat_t:
		if strconv.FormatUint(uint64(stat.Gid), 10) != group.Gid {
			return errors.New("not path owner")
		}
	default:
		return errors.New("unsupported file system")
	}
	// Mode
	if p.Mode().Perm() != perm {
		return errors.New("invalid file mode")
	}
	return nil
}

func (p *Path) CreateFileIfNotExist(buf []byte, perm os.FileMode) error {
	if p.exist() {
		return nil
	}
	er := ioutil.WriteFile(p.path, buf, perm)
	if er != nil {
		return er
	}
	return nil
}

// Config

type SupervisorConfig struct {
	CosmosName string `yaml:"cosmos-name"`
	LogPath    string `yaml:"log-path"`
	LogLevel   string `yaml:"log-level"`
}

type ProcessConfig struct {
	Node     string `yaml:"node"`
	LogPath  string `yaml:"log_path"`
	LogLevel string `yaml:"log_level"`

	EnableCert   *CertConfig         `yaml:"enable_cert"`
	EnableServer *RemoteServerConfig `yaml:"enable_server"`

	CustomizeConfig map[string]string `yaml:"customize"`
}

type CertConfig struct {
	CertPath           string `yaml:"cert_path"`
	KeyPath            string `yaml:"key_path"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`
}

type RemoteServerConfig struct {
	Host string `yaml:"host"`
	Port int32  `yaml:"port"`
}
