package main

import (
	"errors"
	"flag"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"strconv"
	"strings"
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

	log.Println("Welcome to Atomos Supervisor!")

	var (
		action     = flag.String("a", "help", "[init|check|status]")
		userName   = flag.String("u", "", "user name")
		groupName  = flag.String("g", "", "group name")
		cosmosName = flag.String("c", "", "cosmos name")
		nodeNames  = flag.String("n", "", "node name list (separates by ,)")
	)

	flag.Parse()

	nodeNameList := strings.Split(*nodeNames, ",")
	switch *action {
	case "init":
		if *cosmosName == "" || nodeNameList == nil {
			log.Println("-c or -n is empty")
			goto help
		}
		if *groupName == "" {
			log.Println("We should know which group is allowed to access, please give a group name with -g argument.")
			goto help
		}
		if *userName == "" {
			u, er := user.Current()
			if er != nil {
				log.Println("Get default user failed,", er)
				goto help
			}
			*userName = u.Username
		}
		if er := initPaths(*userName, *groupName, *cosmosName, nodeNameList); er != nil {
			log.Println("Paths init failed:", er)
			return
		}
		return
	case "check":
		if *cosmosName == "" || nodeNameList == nil {
			log.Println("-c or -n is empty")
			goto help
		}
		if *groupName == "" {
			log.Println("We should know which group is allowed to access, please give a group name with -g argument.")
			goto help
		}
		if er := checkPaths(*groupName, *cosmosName, nodeNameList); er != nil {
			log.Println("Paths check failed:", er)
			return
		}
		return
	case "status":
	}
help:
	log.Println("Usage: -a [init|check|status] -u {user_name} -g {group_name} -c {cosmos_name} -n {node_name_1,node_name_2,...}")
}

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

func checkNodeName(nodeName string) bool {
	if nodeName == "" {
		return false
	}
	for _, c := range nodeName {
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

// INIT

func initPaths(cosmosUser, cosmosGroup, cosmosName string, nodeNameList []string) error {
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
	if er := initSupervisorPaths(cosmosName, u, g); er != nil {
		log.Println(er)
		return er
	}
	for _, nodeName := range nodeNameList {
		if er := initNodePaths(cosmosName, nodeName, u, g); er != nil {
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
	if !checkCosmosName(cosmosName) {
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

func initNodePaths(cosmosName, nodeName string, u *user.User, g *user.Group) error {
	// Check whether cosmos name is legal.
	// 检查cosmos名称是否合法。
	if !checkNodeName(cosmosName) {
		return errors.New("invalid node name")
	}

	// Check whether /var/run/atomos_{cosmosName} directory exists or not.
	// 检查/var/run/atomos_{cosmosName}目录是否存在。
	if er := NewPath(VarRunPath+AtomosPrefix+cosmosName+"/"+nodeName).CreateDirectoryIfNotExist(u, g, VarRunPerm); er != nil {
		return er
	}
	// Check whether /var/log/atomos_{cosmosName} directory exists or not.
	// 检查/var/log/atomos_{cosmosName}目录是否存在。
	if er := NewPath(VarLogPath+AtomosPrefix+cosmosName+"/"+nodeName).CreateDirectoryIfNotExist(u, g, VarLogPerm); er != nil {
		return er
	}
	// Check whether /etc/atomos_{cosmosName} directory exists or not.
	// 检查/etc/atomos_{cosmosName}目录是否存在。
	if er := NewPath(EtcPath+AtomosPrefix+cosmosName+"/"+nodeName).CreateDirectoryIfNotExist(u, g, EtcPerm); er != nil {
		return er
	}
	// Create {node}.conf
	confPath := NewPath(EtcPath + AtomosPrefix + cosmosName + "/" + nodeName + ".conf")
	if er := confPath.Refresh(); er != nil {
		return er
	}
	conf := &NodeConfig{
		CosmosName:      cosmosName,
		Node:            nodeName,
		LogPath:         VarLogPath + AtomosPrefix + cosmosName + "/" + nodeName,
		LogLevel:        "DEBUG",
		EnableCert:      nil,
		EnableServer:    nil,
		CustomizeConfig: nil,
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

// CHECK

func checkPaths(cosmosGroup, cosmosName string, nodeNameList []string) error {
	u, er := user.Current()
	if er != nil {
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
	if er := checkSupervisorPath(cosmosName, u); er != nil {
		return er
	}
	for _, nodeName := range nodeNameList {
		if er := checkNodePath(cosmosName, nodeName, u); er != nil {
			return er
		}
	}
	log.Println("Paths checked")
	return nil
}

func checkSupervisorPath(cosmosName string, u *user.User) error {
	// Check whether /var/run/atomos_{cosmosName} directory exists or not.
	// 检查/var/run/atomos_{cosmosName}目录是否存在。
	if er := NewPath(VarRunPath+AtomosPrefix+cosmosName).CheckDirectoryOwnerAndMode(u, VarRunPerm); er != nil {
		return er
	}
	// Check whether /var/log/atomos_{cosmosName} directory exists or not.
	// 检查/var/log/atomos_{cosmosName}目录是否存在。
	if er := NewPath(VarLogPath+AtomosPrefix+cosmosName).CheckDirectoryOwnerAndMode(u, VarLogPerm); er != nil {
		return er
	}
	// Check whether /etc/atomos_{cosmosName} directory exists or not.
	// 检查/etc/atomos_{cosmosName}目录是否存在。
	if er := NewPath(EtcPath+AtomosPrefix+cosmosName).CheckDirectoryOwnerAndMode(u, EtcPerm); er != nil {
		return er
	}
	// Get supervisor.conf
	confPath := NewPath(EtcPath + AtomosPrefix + cosmosName + "/supervisor.conf")
	if er := confPath.Refresh(); er != nil {
		return er
	}
	if !confPath.exist() {
		return errors.New("file not exist")
	}
	buf, er := ioutil.ReadFile(confPath.path)
	if er != nil {
		return er
	}
	conf := &SupervisorConfig{}
	er = yaml.Unmarshal(buf, conf)
	if er != nil {
		return er
	}
	if conf.CosmosName != cosmosName {
		return errors.New("invalid cosmos name")
	}
	if conf.LogPath != VarLogPath+AtomosPrefix+cosmosName {
		log.Println("log path has changed to", conf.LogPath)
	}
	return nil
}

func checkNodePath(cosmosName, nodeName string, u *user.User) error {
	// Check whether cosmos name is legal.
	// 检查cosmos名称是否合法。
	if !checkNodeName(cosmosName) {
		return errors.New("invalid node name")
	}

	// Check whether /var/run/atomos_{cosmosName} directory exists or not.
	// 检查/var/run/atomos_{cosmosName}目录是否存在。
	if er := NewPath(VarRunPath+AtomosPrefix+cosmosName+"/"+nodeName).CheckDirectoryOwnerAndMode(u, VarRunPerm); er != nil {
		return er
	}
	// Check whether /var/log/atomos_{cosmosName} directory exists or not.
	// 检查/var/log/atomos_{cosmosName}目录是否存在。
	if er := NewPath(VarLogPath+AtomosPrefix+cosmosName+"/"+nodeName).CheckDirectoryOwnerAndMode(u, VarLogPerm); er != nil {
		return er
	}
	// Check whether /etc/atomos_{cosmosName} directory exists or not.
	// 检查/etc/atomos_{cosmosName}目录是否存在。
	if er := NewPath(EtcPath+AtomosPrefix+cosmosName+"/"+nodeName).CheckDirectoryOwnerAndMode(u, EtcPerm); er != nil {
		return er
	}
	// Get {node}.conf
	confPath := NewPath(EtcPath + AtomosPrefix + cosmosName + "/" + nodeName + ".conf")
	if er := confPath.Refresh(); er != nil {
		return er
	}
	if !confPath.exist() {
		return errors.New("file not exist")
	}
	buf, er := ioutil.ReadFile(confPath.path)
	if er != nil {
		return er
	}
	conf := &NodeConfig{}
	er = yaml.Unmarshal(buf, conf)
	if er != nil {
		return er
	}
	if conf.CosmosName != cosmosName {
		return errors.New("invalid cosmos name")
	}
	if conf.Node != nodeName {
		return errors.New("invalid cosmos name")
	}
	if conf.LogPath != VarLogPath+AtomosPrefix+cosmosName+"/"+nodeName {
		log.Println("log path has changed to", conf.LogPath)
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

func (p *Path) CheckDirectoryOwnerAndMode(u *user.User, perm os.FileMode) error {
	if er := p.Refresh(); er != nil {
		log.Println("refresh failed, err=", er)
		return er
	}
	if !p.exist() {
		return errors.New("directory not exists")
	}
	// Owner
	gidList, er := u.GroupIds()
	if er != nil {
		return er
	}
	switch stat := p.Sys().(type) {
	case *syscall.Stat_t:
		groupOwner := false
		statGid := strconv.FormatUint(uint64(stat.Gid), 10)
		for _, gid := range gidList {
			if gid == statGid {
				groupOwner = true
				break
			}
		}
		if !groupOwner {
			return errors.New("user's group is not owned directory")
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

type NodeConfig struct {
	CosmosName string `yaml:"cosmos-name"`
	Node       string `yaml:"node"`
	LogPath    string `yaml:"log-path"`
	LogLevel   string `yaml:"log-level"`

	EnableCert   *CertConfig         `yaml:"enable-cert"`
	EnableServer *RemoteServerConfig `yaml:"enable-server"`

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
