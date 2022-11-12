package main

import (
	"errors"
	"flag"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path"
	"strconv"
	"strings"
	"syscall"
)

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

	BuildPath string `yaml:"build-path"`
	BinPath   string `yaml:"bin-path"`

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
		action     = flag.String("action", "help", "[init|validate|build]")
		userName   = flag.String("user", "", "user name")
		groupName  = flag.String("group", "", "group name")
		cosmosName = flag.String("cosmos", "", "cosmos name")
		goPath     = flag.String("go", "go", "go binary path")
		nodeNames  = flag.String("node", "", "node name list (separates by ,)")
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
			log.Println("Config init failed:", er)
			return
		}
		return
	case "validate":
		if *cosmosName == "" || nodeNameList == nil {
			log.Println("-c or -n is empty")
			goto help
		}
		if *groupName == "" {
			log.Println("We should know which group is allowed to access, please give a group name with -g argument.")
			goto help
		}
		if er := validatePaths(*groupName, *cosmosName, nodeNameList); er != nil {
			log.Println("Config validate failed:", er)
			return
		}
		return
	case "build":
		if *cosmosName == "" || nodeNameList == nil {
			log.Println("-c or -n is empty")
			goto help
		}
		if er := buildPaths(*cosmosName, *goPath, nodeNameList); er != nil {
			log.Println("Config build failed:", er)
			return
		}
		return
		//case "status":
		//	if *cosmosName == "" || nodeNameList == nil {
		//		log.Println("-c or -n is empty")
		//		goto help
		//	}
		//	NewPath(VarRunPath + AtomosPrefix + *cosmosName)
	}
help:
	log.Println("Usage: -action=[init|validate|build]\n -user={user_name}\n -group={group_name}\n -cosmos={cosmos_name}\n --go={go_binary_path}\n --node={node_name_1,node_name_2,...}")
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
	log.Println("Config initialized")
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
	varRunPath := VarRunPath + AtomosPrefix + cosmosName
	if er := NewPath(varRunPath).CreateDirectoryIfNotExist(u, g, VarRunPerm); er != nil {
		return er
	}
	log.Printf("Supervisor Run Path: %s", varRunPath)

	// Check whether /var/log/atomos_{cosmosName} directory exists or not.
	// 检查/var/log/atomos_{cosmosName}目录是否存在。
	varLogPath := VarLogPath + AtomosPrefix + cosmosName
	if er := NewPath(varLogPath).CreateDirectoryIfNotExist(u, g, VarLogPerm); er != nil {
		return er
	}
	log.Printf("Supervisor Log Path: %s", varLogPath)

	// Check whether /etc/atomos_{cosmosName} directory exists or not.
	// 检查/etc/atomos_{cosmosName}目录是否存在。
	etcPath := EtcPath + AtomosPrefix + cosmosName
	if er := NewPath(etcPath).CreateDirectoryIfNotExist(u, g, EtcPerm); er != nil {
		return er
	}
	log.Printf("Supervisor Etc Path: %s", etcPath)

	// Create supervisor.conf
	supervisorConfigPath := EtcPath + AtomosPrefix + cosmosName + "/supervisor.conf"
	confPath := NewPath(supervisorConfigPath)
	if er := confPath.Refresh(); er != nil {
		return er
	}
	log.Printf("Supervisor Config Path: %s", supervisorConfigPath)

	conf := &SupervisorConfig{}
	if confPath.exist() {
		buf, er := ioutil.ReadFile(confPath.path)
		if er != nil {
			return er
		}
		er = yaml.Unmarshal(buf, conf)
		if er != nil {
			return er
		}
		if conf.CosmosName != cosmosName {
			log.Printf("Supervisor Config Invalid Cosmos Name, name=(%s)", conf.CosmosName)
			return errors.New("invalid cosmos name")
		}
		log.Println("Supervisor Config Updated")
	} else {
		conf.CosmosName = cosmosName
		conf.LogPath = varLogPath
		conf.LogLevel = "DEBUG"
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
	varRunPath := VarRunPath + AtomosPrefix + cosmosName + "/" + nodeName
	if er := NewPath(varRunPath).CreateDirectoryIfNotExist(u, g, VarRunPerm); er != nil {
		return er
	}
	log.Printf("Node %s Run Path: %s", nodeName, varRunPath)

	// Check whether /var/log/atomos_{cosmosName} directory exists or not.
	// 检查/var/log/atomos_{cosmosName}目录是否存在。
	varLogPath := VarLogPath + AtomosPrefix + cosmosName + "/" + nodeName
	if er := NewPath(varLogPath).CreateDirectoryIfNotExist(u, g, VarLogPerm); er != nil {
		return er
	}
	log.Printf("Node %s Log Path: %s", nodeName, varLogPath)

	// Check whether /etc/atomos_{cosmosName} directory exists or not.
	// 检查/etc/atomos_{cosmosName}目录是否存在。
	etcPath := EtcPath + AtomosPrefix + cosmosName + "/" + nodeName
	if er := NewPath(etcPath).CreateDirectoryIfNotExist(u, g, EtcPerm); er != nil {
		return er
	}
	log.Printf("Node %s Etc Path: %s", nodeName, etcPath)

	// Create {node}.conf
	configPath := EtcPath + AtomosPrefix + cosmosName + "/" + nodeName + ".conf"
	confPath := NewPath(configPath)
	if er := confPath.Refresh(); er != nil {
		return er
	}
	log.Printf("Node %s Config Path: %s", nodeName, configPath)

	conf := &NodeConfig{
		CosmosName:      cosmosName,
		Node:            nodeName,
		LogPath:         varLogPath,
		LogLevel:        "DEBUG",
		EnableCert:      nil,
		EnableServer:    nil,
		CustomizeConfig: nil,
	}
	if confPath.exist() {
		buf, er := ioutil.ReadFile(confPath.path)
		if er != nil {
			return er
		}
		er = yaml.Unmarshal(buf, conf)
		if er != nil {
			return er
		}
		if conf.CosmosName != cosmosName {
			log.Printf("Node %s Config Invalid Cosmos Name, name=(%s)", nodeName, conf.CosmosName)
			return errors.New("invalid cosmos name")
		}
		if conf.Node != nodeName {
			log.Printf("Node %s Config Invalid Node Name, name=(%s)", nodeName, conf.Node)
			return errors.New("invalid cosmos name")
		}
		log.Printf("Node %s Config Updated", nodeName)
	} else {
		conf.CosmosName = cosmosName
		conf.LogPath = varLogPath
		conf.LogLevel = "DEBUG"
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

// VALIDATE

func validatePaths(cosmosGroup, cosmosName string, nodeNameList []string) error {
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
	if er := validateSupervisorPath(cosmosName, u); er != nil {
		return er
	}
	for _, nodeName := range nodeNameList {
		if er := validateNodePath(cosmosName, nodeName, u); er != nil {
			return er
		}
	}
	log.Println("Config validated")
	return nil
}

func validateSupervisorPath(cosmosName string, u *user.User) error {
	// Check whether /var/run/atomos_{cosmosName} directory exists or not.
	// 检查/var/run/atomos_{cosmosName}目录是否存在。
	varRunPath := VarRunPath + AtomosPrefix + cosmosName
	if er := NewPath(varRunPath).CheckDirectoryOwnerAndMode(u, VarRunPerm); er != nil {
		return er
	}
	log.Printf("Supervisor Run Path Validated: %s", varRunPath)

	// Check whether /var/log/atomos_{cosmosName} directory exists or not.
	// 检查/var/log/atomos_{cosmosName}目录是否存在。
	varLogPath := VarLogPath + AtomosPrefix + cosmosName
	if er := NewPath(varLogPath).CheckDirectoryOwnerAndMode(u, VarLogPerm); er != nil {
		return er
	}
	log.Printf("Supervisor Log Path Validated: %s", varLogPath)

	// Check whether /etc/atomos_{cosmosName} directory exists or not.
	// 检查/etc/atomos_{cosmosName}目录是否存在。
	etcPath := EtcPath + AtomosPrefix + cosmosName
	if er := NewPath(etcPath).CheckDirectoryOwnerAndMode(u, EtcPerm); er != nil {
		return er
	}
	log.Printf("Supervisor Etc Path Validated: %s", varLogPath)

	// Get supervisor.conf
	supervisorConfigPath := EtcPath + AtomosPrefix + cosmosName + "/supervisor.conf"
	confPath := NewPath(supervisorConfigPath)
	if er := confPath.Refresh(); er != nil {
		return er
	}
	log.Printf("Supervisor Config Path Validated: %s", supervisorConfigPath)

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
		log.Println("Supervisor Config Path has changed to", conf.LogPath)
	}
	log.Println("Supervisor Config Validated")
	return nil
}

func validateNodePath(cosmosName, nodeName string, u *user.User) error {
	// Check whether cosmos name is legal.
	// 检查cosmos名称是否合法。
	if !checkNodeName(cosmosName) {
		return errors.New("invalid node name")
	}

	// Check whether /var/run/atomos_{cosmosName} directory exists or not.
	// 检查/var/run/atomos_{cosmosName}目录是否存在。
	varRunPath := VarRunPath + AtomosPrefix + cosmosName + "/" + nodeName
	if er := NewPath(varRunPath).CheckDirectoryOwnerAndMode(u, VarRunPerm); er != nil {
		return er
	}
	log.Printf("Node %s Run Path Validated: %s", nodeName, varRunPath)

	// Check whether /var/log/atomos_{cosmosName} directory exists or not.
	// 检查/var/log/atomos_{cosmosName}目录是否存在。
	varLogPath := VarLogPath + AtomosPrefix + cosmosName + "/" + nodeName
	if er := NewPath(varLogPath).CheckDirectoryOwnerAndMode(u, VarLogPerm); er != nil {
		return er
	}
	log.Printf("Node %s Log Path Validated: %s", nodeName, varLogPath)

	// Check whether /etc/atomos_{cosmosName} directory exists or not.
	// 检查/etc/atomos_{cosmosName}目录是否存在。
	etcPath := EtcPath + AtomosPrefix + cosmosName + "/" + nodeName
	if er := NewPath(etcPath).CheckDirectoryOwnerAndMode(u, EtcPerm); er != nil {
		return er
	}
	log.Printf("Node %s Etc Path Validated: %s", nodeName, etcPath)

	// Get {node}.conf
	configPath := EtcPath + AtomosPrefix + cosmosName + "/" + nodeName + ".conf"
	confPath := NewPath(configPath)
	if er := confPath.Refresh(); er != nil {
		return er
	}
	log.Printf("Node %s Config Path Validated: %s", nodeName, configPath)

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
		log.Printf("Node %s Config Invalid Cosmos Name, name=(%s)", nodeName, conf.CosmosName)
		return errors.New("invalid cosmos name")
	}
	if conf.Node != nodeName {
		log.Printf("Node %s Config Invalid Node Name, name=(%s)", nodeName, conf.Node)
		return errors.New("invalid cosmos name")
	}
	if conf.LogPath != VarLogPath+AtomosPrefix+cosmosName+"/"+nodeName {
		log.Println("log path has changed to", conf.LogPath)
	}
	if conf.BuildPath != "" {
		buildPath := NewPath(conf.BuildPath)
		er := buildPath.Refresh()
		if er == nil && !buildPath.exist() {
			er = errors.New("path not exists")
		}
		if er != nil {
			log.Printf("Node %s Config Invalid Build Path, name=(%s),err=(%v)", nodeName, conf.Node, er)
			return er
		}
		log.Printf("Node %s Config Build Path Validated: %s", nodeName, buildPath.path)
	}
	if conf.BinPath != "" {
		binPath := NewPath(conf.BinPath)
		er := binPath.Refresh()
		if er != nil {
			log.Printf("Node %s Config Invalid Bin Path, name=(%s),err=(%v)", nodeName, conf.Node, er)
			return er
		}
		log.Printf("Node %s Config Bin Path Validated: %s", nodeName, binPath.path)
	}
	return nil
}

// BUILD

func buildPaths(cosmosName, goPath string, nodeNameList []string) error {
	supervisorConfigPath := EtcPath + AtomosPrefix + cosmosName + "/supervisor.conf"
	buf, er := ioutil.ReadFile(supervisorConfigPath)
	if er != nil {
		return er
	}
	conf := &SupervisorConfig{}
	er = yaml.Unmarshal(buf, conf)
	if er != nil {
		return er
	}

	for _, nodeName := range nodeNameList {
		if er := buildNodePath(cosmosName, goPath, nodeName); er != nil {
			return er
		}
	}
	log.Println("Config built")
	return nil
}

func buildNodePath(cosmosName, goPath, nodeName string) error {
	// Check whether cosmos name is legal.
	// 检查cosmos名称是否合法。
	if !checkNodeName(cosmosName) {
		return errors.New("invalid node name")
	}

	// Get {node}.conf
	configPath := EtcPath + AtomosPrefix + cosmosName + "/" + nodeName + ".conf"
	confPath := NewPath(configPath)
	if er := confPath.Refresh(); er != nil {
		return er
	}
	log.Printf("Node %s Config Path Validated: %s", nodeName, configPath)

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
		log.Printf("Node %s Config Invalid Cosmos Name, name=(%s)", nodeName, conf.CosmosName)
		return errors.New("invalid cosmos name")
	}
	if conf.Node != nodeName {
		log.Printf("Node %s Config Invalid Node Name, name=(%s)", nodeName, conf.Node)
		return errors.New("invalid cosmos name")
	}

	buildPath := NewPath(conf.BuildPath)
	if er := buildPath.Refresh(); er != nil || !buildPath.exist() {
		log.Printf("Node %s Config Invalid Build Path, path=(%s)", nodeName, conf.BuildPath)
		return errors.New("invalid build path")
	}
	binPath := NewPath(conf.BinPath)
	if er := binPath.Refresh(); er != nil {
		log.Printf("Node %s Config Invalid Bin Path, name=(%s),err=(%v)", nodeName, conf.BinPath, er)
		return er
	}

	cmd := exec.Command(goPath, "build", "-o", binPath.path, buildPath.path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = path.Join(path.Dir(buildPath.path))
	if er := cmd.Run(); er != nil {
		log.Printf("Node %s Config Built Failed: err=(%v)", nodeName, er)
		return er
	}

	log.Printf("Node %s Config Built Result: %s\n", nodeName, binPath.path)
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
