package go_atomos

import (
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"path"
	"strings"
	"time"
)

const (
	AtomosPrefix = "atomos_"

	VarRunPath = "/var/run/"
	VarRunPerm = 0775

	VarLogPath = "/var/log/"
	VarLogPerm = 0775

	EtcPath = "/etc/"
	EtcPerm = 0770
)

// Commander（指挥官）:
//   init - 初始化cosmos，以及node的配置
//   validate - 验证cosmos，以及node的配置
//   list - 列出所有配置的cosmos以及node
//   update - 从配置中心加载所有配置，并检查变更（以后支持）
//   build - 构建cosmos的node可执行文件
// Supervisor（监督者）：
//   list - 列出所有监控的cosmos以及node的状态
//   keepalive - 保持某个cosmos，或某个cosmos其中的某些node的运行
//   stop - 停止某个cosmos，或某个cosmos其中的某些node的运行
//   restart - 重启某个cosmos，或某个cosmos其中的某些node的运行

func InitConfig(cosmosUser, cosmosGroup, cosmosName string, nodeNameList []string) *Error {
	u, g, err := CheckSystemUserAndGroup(cosmosUser, cosmosGroup)
	if err != nil {
		return err.AutoStack(nil, nil)
	}

	// Check whether cosmos name is legal.
	// 检查cosmos名称是否合法。
	if !CheckCosmosName(cosmosName) {
		return NewError(ErrCosmosNameInvalid, "invalid cosmos name").AutoStack(nil, nil)
	}
	for _, nodeName := range nodeNameList {
		if !CheckNodeName(nodeName) {
			return NewError(ErrCosmosNodeNameInvalid, "invalid node name").AutoStack(nil, nil)
		}
	}
	// Check Supervisor.
	supervisor := &SupervisorConfig{CosmosName: cosmosName, NodeNameList: nodeNameList}
	if err = supervisor.InitConfig(u, g); err != nil {
		return err.AutoStack(nil, nil)
	}
	// Check Node List.
	for _, nodeName := range supervisor.NodeNameList {
		node := &NodeConfig{CosmosName: cosmosName, NodeName: nodeName}
		if err = node.InitConfig(cosmosName, nodeName, u, g); err != nil {
			return err.AutoStack(nil, nil)
		}
	}
	fmt.Println("Config initialized")
	return nil
}

func ValidateConfig(cosmosGroup, cosmosName string) *Error {
	u, _, err := CheckCurrentUserAndGroup(cosmosGroup)
	if err != nil {
		return err.AutoStack(nil, nil)
	}

	// Check whether cosmos name is legal.
	// 检查cosmos名称是否合法。
	if !CheckCosmosName(cosmosName) {
		return NewError(ErrCosmosNameInvalid, "invalid cosmos name").AutoStack(nil, nil)
	}

	supervisor := &SupervisorConfig{CosmosName: cosmosName, NodeNameList: nil}
	if err = supervisor.ValidateConfig(u); err != nil {
		return err.AutoStack(nil, nil)
	}
	nodeList := make([]*NodeConfig, 0, len(supervisor.config.NodeList))
	nodeMap := make(map[string]bool, len(supervisor.config.NodeList))
	for _, nodeName := range supervisor.config.NodeList {
		node := &NodeConfig{CosmosName: cosmosName, NodeName: nodeName}
		if err = node.ValidateConfig(u); err != nil {
			return err.AutoStack(nil, nil)
		}
		nodeList = append(nodeList, node)
		nodeMap[nodeName] = true
	}
	for _, node := range supervisor.config.KeepaliveNodeList {
		if !nodeMap[node] {
			return NewErrorf(ErrCosmosConfigInvalid, "keepalive node not found, node=(%s)", node)
		}
	}
	supervisor.KeepaliveNodes = supervisor.config.KeepaliveNodeList
	fmt.Println("Config initialized")
	return nil
}

func LoadConfig(cosmosName string) (*SupervisorConfig, *Error) {
	u, er := user.Current()
	if er != nil {
		return nil, NewErrorf(ErrPathUserInvalid, "get user failed, err=(%v)", er).AutoStack(nil, nil)
	}

	// Check whether cosmos name is legal.
	// 检查cosmos名称是否合法。
	if !CheckCosmosName(cosmosName) {
		return nil, NewError(ErrCosmosNameInvalid, "invalid cosmos name").AutoStack(nil, nil)
	}

	supervisor := &SupervisorConfig{CosmosName: cosmosName, NodeNameList: nil}
	if err := supervisor.ValidateConfig(u); err != nil {
		return nil, err.AutoStack(nil, nil)
	}
	nodeList := make([]*NodeConfig, 0, len(supervisor.config.NodeList))
	nodeMap := make(map[string]bool, len(supervisor.config.NodeList))
	for _, nodeName := range supervisor.config.NodeList {
		node := &NodeConfig{CosmosName: cosmosName, NodeName: nodeName}
		if err := node.ValidateConfig(u); err != nil {
			return nil, err.AutoStack(nil, nil)
		}
		nodeList = append(nodeList, node)
		nodeMap[nodeName] = true
	}
	for _, node := range supervisor.config.KeepaliveNodeList {
		if !nodeMap[node] {
			return nil, NewErrorf(ErrCosmosConfigInvalid, "keepalive node not found, node=(%s)", node)
		}
	}
	supervisor.NodeConfigList = nodeList
	supervisor.KeepaliveNodes = supervisor.config.KeepaliveNodeList
	fmt.Println("Config loaded")
	return supervisor, nil
}

func ListAllConfig() ([]*SupervisorConfig, *Error) {
	etcPathDir, er := ioutil.ReadDir(EtcPath)
	if er != nil {
		return nil, NewErrorf(ErrCosmosReadEtcPath, "read etc path failed, err=(%v)", er).AutoStack(nil, nil)
	}

	var cosmosList []string
	for _, dir := range etcPathDir {
		if !dir.IsDir() {
			continue
		}
		if !strings.HasPrefix(dir.Name(), AtomosPrefix) {
			continue
		}
		cosmosName := dir.Name()[len(AtomosPrefix):]
		cosmosList = append(cosmosList, cosmosName)
	}

	var supervisorList []*SupervisorConfig
	if len(cosmosList) == 0 {
		fmt.Println("List: No Cosmos Found.")
		return supervisorList, nil
	}
	fmt.Printf("List: %d Cosmos Found.\n", len(cosmosList))
	for _, cosmosName := range cosmosList {
		supervisorConfig, err := LoadConfig(cosmosName)
		if err != nil {
			return nil, err.AutoStack(nil, nil)
		}
		supervisorList = append(supervisorList, supervisorConfig)
	}
	return supervisorList, nil
}

func BuildConfig(cosmosName, goPath string, nodeNameList []string) *Error {
	supervisorConfig, err := LoadConfig(cosmosName)
	if err != nil {
		return err.AutoStack(nil, nil)
	}
	nodeConfigMap := make(map[string]*NodeConfig, len(supervisorConfig.NodeConfigList))
	for _, config := range supervisorConfig.NodeConfigList {
		nodeConfigMap[config.NodeName] = config
	}

	if nodeNameList == nil {
		nodeNameList = supervisorConfig.NodeNameList
	} else {
		for _, nodeName := range nodeNameList {
			if _, has := nodeConfigMap[nodeName]; !has {
				return NewErrorf(ErrCosmosConfigNotFound, "build node not found, name=(%s)", nodeName).AutoStack(nil, nil)
			}
		}
	}
	now := time.Now().Format("20060102150405")
	for _, nodeName := range nodeNameList {
		if err = buildNodePath(nodeConfigMap[nodeName], goPath, now); err != nil {
			return err.AutoStack(nil, nil)
		}
	}
	fmt.Println("Config built")
	return nil
}

func buildNodePath(nodeConfig *NodeConfig, goPath, nowStr string) *Error {
	buildPath := NewPath(nodeConfig.config.BuildPath)
	if er := buildPath.Refresh(); er != nil || !buildPath.Exist() {
		fmt.Printf("Error: Node \"%s\" Config Invalid Build Path, path=(%s)\n", nodeConfig.NodeName, nodeConfig.config.BuildPath)
		return NewErrorf(ErrCosmosConfigInvalid, "invalid build path, path=(%s)", nodeConfig.config.BuildPath).AutoStack(nil, nil)
	}
	binPath := NewPath(nodeConfig.config.BinPath)
	if er := binPath.Refresh(); er != nil {
		fmt.Printf("Error: Node \"%s\" Config Invalid Bin Path, name=(%s),err=(%v)\n", nodeConfig.NodeName, nodeConfig.config.BinPath, er)
		return NewErrorf(ErrCosmosConfigInvalid, "invalid bin path, path=(%s)", nodeConfig.config.BuildPath).AutoStack(nil, nil)
	}

	outPath := binPath.path + "/" + nodeConfig.NodeName + "_" + nowStr
	cmd := exec.Command(goPath, "build", "-o", outPath, buildPath.path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = path.Join(path.Dir(buildPath.path))
	if er := cmd.Run(); er != nil {
		fmt.Printf("Error: Node \"%s\" Config Built Failed: err=(%v)\n", nodeConfig.NodeName, er)
		return NewErrorf(ErrCosmosConfigBuildFailed, "build failed, err=(%v)", er).AutoStack(nil, nil)
	}

	linkPath := binPath.path + "/" + nodeConfig.NodeName + "_latest"
	exec.Command("rm", linkPath).Run()

	cmd = exec.Command("ln", "-s", outPath, linkPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if er := cmd.Run(); er != nil {
		fmt.Printf("Error: Node \"%s\" Link Failed: err=(%v)\n", nodeConfig.NodeName, er)
		return NewErrorf(ErrCosmosConfigBinLinkFileFailed, "link binary failed, err=(%v)", er).AutoStack(nil, nil)
	}

	fmt.Printf("Node \"%s\" Config Built Result: %s\n", nodeConfig.NodeName, binPath.path)
	return nil
}

func Keepalive(nodeConfig *NodeConfig) *Error {
	return nil
}

func CheckSystemUserAndGroup(cosmosUser, cosmosGroup string) (*user.User, *user.Group, *Error) {
	if os.Getuid() != 0 {
		return nil, nil, NewError(ErrPathUserInvalid, "init supervisor should run under root").AutoStack(nil, nil)
	}
	// Check user.
	u, er := user.Lookup(cosmosUser)
	if er != nil {
		if _, ok := er.(user.UnknownUserError); ok {
			return nil, nil, NewError(ErrPathUserInvalid, "unknown user").AutoStack(nil, nil)
		}
		fmt.Println("Error: Lookup user failed, err=", er)
		return nil, nil, NewErrorf(ErrPathUserInvalid, "lookup user failed, err=(%v)", er).AutoStack(nil, nil)
	}
	// Check group.
	g, er := user.LookupGroup(cosmosGroup)
	if er != nil {
		if _, ok := er.(*user.UnknownGroupError); ok {
			return nil, nil, NewError(ErrPathGroupInvalid, "unknown group").AutoStack(nil, nil)
		}
		fmt.Println("Error: Lookup group failed, err=", er)
		return nil, nil, NewErrorf(ErrPathGroupInvalid, "lookup group failed, err=(%v)", er).AutoStack(nil, nil)
	}
	userGroups, er := u.GroupIds()
	if er != nil {
		fmt.Println("Error: Iter user group failed, err=", er)
		return nil, nil, NewErrorf(ErrPathGroupInvalid, "iter user group failed, err=(%v)", er).AutoStack(nil, nil)
	}
	inGroup := false
	for _, gid := range userGroups {
		if gid == g.Gid {
			inGroup = true
			break
		}
	}
	if !inGroup {
		fmt.Println("Error: User not in group")
		return nil, nil, NewErrorf(ErrPathGroupInvalid, "user not in group").AutoStack(nil, nil)
	}
	return u, g, nil
}

func CheckCurrentUserAndGroup(cosmosGroup string) (*user.User, *user.Group, *Error) {
	u, er := user.Current()
	if er != nil {
		return nil, nil, NewErrorf(ErrPathUserInvalid, "get user failed, err=(%v)", er).AutoStack(nil, nil)
	}
	// Check group.
	g, er := user.LookupGroup(cosmosGroup)
	if er != nil {
		if _, ok := er.(*user.UnknownGroupError); ok {
			return nil, nil, NewError(ErrPathGroupInvalid, "unknown group").AutoStack(nil, nil)
		}
		fmt.Println("Error: Lookup group failed, err=", er)
		return nil, nil, NewErrorf(ErrPathUserInvalid, "lookup user failed, err=(%v)", er).AutoStack(nil, nil)
	}
	userGroups, er := u.GroupIds()
	if er != nil {
		fmt.Println("Error: Iter user group failed, err=", er)
		return nil, nil, NewErrorf(ErrPathGroupInvalid, "iter user group failed, err=(%v)", er).AutoStack(nil, nil)
	}
	inGroup := false
	for _, gid := range userGroups {
		if gid == g.Gid {
			inGroup = true
			break
		}
	}
	if !inGroup {
		fmt.Println("Error: User not in group")
		return nil, nil, NewErrorf(ErrPathGroupInvalid, "user not in group").AutoStack(nil, nil)
	}
	return u, g, nil
}

// Config interface

type Configuration interface {
	GetConfigPath() string
	GetVarRunPath() string
	GetVarLogPath() string
	GetEtcPath() string
	CreateVarRunPathIfNotExist(u *user.User, g *user.Group) *Error
	CreateVarLogPathIfNotExist(u *user.User, g *user.Group) *Error
	CreateEtcPathIfNotExist(u *user.User, g *user.Group) *Error

	GetConfigFile() (*Path, *Error)
	LoadConfig() (bool, *Error)
	ValidateConfig(u *user.User) *Error
}

// Supervisor

type SupervisorConfig struct {
	CosmosName     string
	NodeNameList   []string
	NodeConfigList []*NodeConfig
	KeepaliveNodes []string

	path   *Path
	config *SupervisorYAMLConfig
}

func (c *SupervisorConfig) GetConfigPath() string {
	return EtcPath + AtomosPrefix + c.CosmosName + "/supervisor.conf"
}

func (c *SupervisorConfig) GetVarRunPath() string {
	return VarRunPath + AtomosPrefix + c.CosmosName
}

func (c *SupervisorConfig) GetVarLogPath() string {
	return VarLogPath + AtomosPrefix + c.CosmosName
}

func (c *SupervisorConfig) GetEtcPath() string {
	return EtcPath + AtomosPrefix + c.CosmosName
}

// CreateVarRunPathIfNotExist
// Check whether /var/run/atomos_{cosmosName} directory exists or not.
// 检查/var/run/atomos_{cosmosName}目录是否存在。
func (c *SupervisorConfig) CreateVarRunPathIfNotExist(u *user.User, g *user.Group) *Error {
	if err := NewPath(c.GetVarRunPath()).CreateDirectoryIfNotExist(u, g, VarRunPerm); err != nil {
		return err.AutoStack(nil, nil)
	}
	fmt.Printf("Supervisor Run Path: %s\n", c.GetVarRunPath())
	return nil
}

// CheckVarRunPathOwnerAndMode
// Check whether /var/run/atomos_{cosmosName} directory exists or not.
// 检查/var/run/atomos_{cosmosName}目录是否存在。
func (c *SupervisorConfig) CheckVarRunPathOwnerAndMode(u *user.User) *Error {
	if err := NewPath(c.GetVarRunPath()).CheckDirectoryOwnerAndMode(u, VarRunPerm); err != nil {
		return err.AutoStack(nil, nil)
	}
	fmt.Printf("Supervisor Run Path Validated: %s\n", c.GetVarRunPath())
	return nil
}

// CreateVarLogPathIfNotExist
// Check whether /var/log/atomos_{cosmosName} directory exists or not.
// 检查/var/log/atomos_{cosmosName}目录是否存在。
func (c *SupervisorConfig) CreateVarLogPathIfNotExist(u *user.User, g *user.Group) *Error {
	if err := NewPath(c.GetVarLogPath()).CreateDirectoryIfNotExist(u, g, VarLogPerm); err != nil {
		return err.AutoStack(nil, nil)
	}
	fmt.Printf("Supervisor Log Path: %s\n", c.GetVarLogPath())
	return nil
}

// CheckVarLogPathOwnerAndMode
// Check whether /var/log/atomos_{cosmosName} directory exists or not.
// 检查/var/log/atomos_{cosmosName}目录是否存在。
func (c *SupervisorConfig) CheckVarLogPathOwnerAndMode(u *user.User) *Error {
	if err := NewPath(c.GetVarLogPath()).CheckDirectoryOwnerAndMode(u, VarLogPerm); err != nil {
		return err.AutoStack(nil, nil)
	}
	fmt.Printf("Supervisor Log Path Validated: %s\n", c.GetVarLogPath())
	return nil
}

// CreateEtcPathIfNotExist
// Check whether /etc/atomos_{cosmosName} directory exists or not.
// 检查/etc/atomos_{cosmosName}目录是否存在。
func (c *SupervisorConfig) CreateEtcPathIfNotExist(u *user.User, g *user.Group) *Error {
	if err := NewPath(c.GetEtcPath()).CreateDirectoryIfNotExist(u, g, EtcPerm); err != nil {
		return err.AutoStack(nil, nil)
	}
	fmt.Printf("Supervisor Etc Path: %s\n", c.GetEtcPath())
	return nil
}

// CheckEtcPathOwnerAndMode
// Check whether /etc/atomos_{cosmosName} directory exists or not.
// 检查/etc/atomos_{cosmosName}目录是否存在。
func (c *SupervisorConfig) CheckEtcPathOwnerAndMode(u *user.User) *Error {
	if err := NewPath(c.GetEtcPath()).CheckDirectoryOwnerAndMode(u, EtcPerm); err != nil {
		return err.AutoStack(nil, nil)
	}
	fmt.Printf("Supervisor Etc Path Validated: %s\n", c.GetEtcPath())
	return nil
}

// GetConfigFile
// Create supervisor.conf
func (c *SupervisorConfig) GetConfigFile() (*Path, *Error) {
	confPath := NewPath(c.GetConfigPath())
	if err := confPath.Refresh(); err != nil {
		return nil, err.AutoStack(nil, nil)
	}
	fmt.Printf("Supervisor Config Path: %s\n", c.GetConfigPath())
	return confPath, nil
}

func (c *SupervisorConfig) LoadConfig() (bool, *Error) {
	confPath, err := c.GetConfigFile()
	if err != nil {
		return false, err.AutoStack(nil, nil)
	}
	c.path = confPath

	conf := &SupervisorYAMLConfig{}
	if !confPath.Exist() {
		return false, nil
	}

	buf, er := ioutil.ReadFile(confPath.path)
	if er != nil {
		return true, NewErrorf(ErrCosmosConfigInvalid, "read config failed, err=(%v)", er).AutoStack(nil, nil)
	}
	er = yaml.Unmarshal(buf, conf)
	if er != nil {
		return true, NewErrorf(ErrCosmosConfigInvalid, "load config failed, err=(%v)", er).AutoStack(nil, nil)
	}
	c.config = conf
	return true, nil
}

func (c *SupervisorConfig) InitConfig(u *user.User, g *user.Group) *Error {
	if err := c.CreateVarRunPathIfNotExist(u, g); err != nil {
		return err.AutoStack(nil, nil)
	}
	if err := c.CreateVarLogPathIfNotExist(u, g); err != nil {
		return err.AutoStack(nil, nil)
	}
	if err := c.CreateEtcPathIfNotExist(u, g); err != nil {
		return err.AutoStack(nil, nil)
	}
	exist, err := c.LoadConfig()
	if err != nil {
		return err.AutoStack(nil, nil)
	}
	if exist {
		if c.config.Cosmos != c.CosmosName {
			return NewErrorf(ErrCosmosConfigInvalid, "cosmos not match, name=(%s)", c.config.Cosmos).AutoStack(nil, nil)
		}
		if c.config.LogPath != c.GetVarLogPath() {
			fmt.Println("WARN: Supervisor Log Path has changed to", c.config.LogPath)
		}
		if c.config.RunPath != c.GetVarRunPath() {
			fmt.Println("WARN: Supervisor Run Path has changed to", c.config.RunPath)
		}
		if c.config.EtcPath != c.GetEtcPath() {
			fmt.Println("WARN: Supervisor Etc Path has changed to", c.config.EtcPath)
		}
		for _, nodeName := range c.NodeNameList {
			has := false
			for _, s := range c.config.NodeList {
				if s == nodeName {
					has = true
					break
				}
			}
			if !has {
				c.config.NodeList = append(c.config.NodeList, nodeName)
			}
		}
	} else {
		c.config = &SupervisorYAMLConfig{
			Cosmos:   c.CosmosName,
			NodeList: c.NodeNameList,
			LogLevel: "DEBUG",
			LogPath:  c.GetVarLogPath(),
			RunPath:  c.GetVarRunPath(),
			EtcPath:  c.GetEtcPath(),
		}
	}

	buf, er := yaml.Marshal(c.config)
	if er != nil {
		return NewErrorf(ErrPathSaveFileFailed, "supervisor config marshal failed, err=(%v)", er).AutoStack(nil, nil)
	}
	if er = ioutil.WriteFile(c.path.path, buf, EtcPerm); er != nil {
		return NewErrorf(ErrPathSaveFileFailed, "supervisor config save failed, err=(%v)", er).AutoStack(nil, nil)
	}
	return nil
}

func (c *SupervisorConfig) ValidateConfig(u *user.User) *Error {
	if err := c.CheckVarRunPathOwnerAndMode(u); err != nil {
		return err.AutoStack(nil, nil)
	}
	if err := c.CheckVarLogPathOwnerAndMode(u); err != nil {
		return err.AutoStack(nil, nil)
	}
	if err := c.CheckEtcPathOwnerAndMode(u); err != nil {
		return err.AutoStack(nil, nil)
	}
	exist, err := c.LoadConfig()
	if err != nil {
		return err.AutoStack(nil, nil)
	}
	if !exist {
		return NewError(ErrCosmosConfigNotFound, "supervisor config not found")
	}

	if c.config.Cosmos != c.CosmosName {
		return NewErrorf(ErrCosmosConfigInvalid, "cosmos not match, name=(%s)", c.config.Cosmos).AutoStack(nil, nil)
	}
	for _, nodeName := range c.config.NodeList {
		if !CheckNodeName(nodeName) {
			return NewError(ErrCosmosNodeNameInvalid, "invalid node name").AutoStack(nil, nil)
		}
	}
	if c.config.LogPath != c.GetVarLogPath() {
		fmt.Println("WARN: Supervisor Log Path has changed to", c.config.LogPath)
	}
	if c.config.RunPath != c.GetVarRunPath() {
		fmt.Println("WARN: Supervisor Run Path has changed to", c.config.RunPath)
	}
	if c.config.EtcPath != c.GetEtcPath() {
		fmt.Println("WARN: Supervisor Etc Path has changed to", c.config.EtcPath)
	}
	fmt.Println("Supervisor Config Validated")
	return nil
}

// Node

type NodeConfig struct {
	CosmosName string
	NodeName   string

	path   *Path
	config *NodeYAMLConfig
}

func (c *NodeConfig) GetConfigPath() string {
	return EtcPath + AtomosPrefix + c.CosmosName + "/" + c.NodeName + ".conf"
}

func (c *NodeConfig) GetVarRunPath() string {
	return VarRunPath + AtomosPrefix + c.CosmosName + "/" + c.NodeName
}

func (c *NodeConfig) GetVarLogPath() string {
	return VarLogPath + AtomosPrefix + c.CosmosName + "/" + c.NodeName
}

func (c *NodeConfig) GetEtcPath() string {
	return EtcPath + AtomosPrefix + c.CosmosName + "/" + c.NodeName
}

// CreateVarRunPathIfNotExist
// Check whether /var/run/atomos_{cosmosName} directory exists or not.
// 检查/var/run/atomos_{cosmosName}目录是否存在。
func (c *NodeConfig) CreateVarRunPathIfNotExist(u *user.User, g *user.Group) *Error {
	if er := NewPath(c.GetVarRunPath()).CreateDirectoryIfNotExist(u, g, VarRunPerm); er != nil {
		return er
	}
	fmt.Printf("Node %s Run Path: %s\n", c.NodeName, c.GetVarRunPath())
	return nil
}

// CheckVarRunPathOwnerAndMode
// Check whether /var/run/atomos_{cosmosName} directory exists or not.
// 检查/var/run/atomos_{cosmosName}目录是否存在。
func (c *NodeConfig) CheckVarRunPathOwnerAndMode(u *user.User) *Error {
	if err := NewPath(c.GetVarRunPath()).CheckDirectoryOwnerAndMode(u, VarRunPerm); err != nil {
		return err.AutoStack(nil, nil)
	}
	fmt.Printf("Node %s Run Path Validated: %s\n", c.NodeName, c.GetVarRunPath())
	return nil
}

// CreateVarLogPathIfNotExist
// Check whether /var/log/atomos_{cosmosName} directory exists or not.
// 检查/var/log/atomos_{cosmosName}目录是否存在。
func (c *NodeConfig) CreateVarLogPathIfNotExist(u *user.User, g *user.Group) *Error {
	if er := NewPath(c.GetVarLogPath()).CreateDirectoryIfNotExist(u, g, VarLogPerm); er != nil {
		return er
	}
	fmt.Printf("Node %s Log Path: %s\n", c.NodeName, c.GetVarLogPath())
	return nil
}

// CheckVarLogPathOwnerAndMode
// Check whether /var/log/atomos_{cosmosName} directory exists or not.
// 检查/var/log/atomos_{cosmosName}目录是否存在。
func (c *NodeConfig) CheckVarLogPathOwnerAndMode(u *user.User) *Error {
	if err := NewPath(c.GetVarLogPath()).CheckDirectoryOwnerAndMode(u, VarLogPerm); err != nil {
		return err.AutoStack(nil, nil)
	}
	fmt.Printf("Node %s Log Path Validated: %s\n", c.NodeName, c.GetVarLogPath())
	return nil
}

// CreateEtcPathIfNotExist
// Check whether /etc/atomos_{cosmosName} directory exists or not.
// 检查/etc/atomos_{cosmosName}目录是否存在。
func (c *NodeConfig) CreateEtcPathIfNotExist(u *user.User, g *user.Group) *Error {
	if er := NewPath(c.GetEtcPath()).CreateDirectoryIfNotExist(u, g, EtcPerm); er != nil {
		return er
	}
	fmt.Printf("Node %s Etc Path: %s\n", c.NodeName, c.GetEtcPath())
	return nil
}

// CheckEtcPathOwnerAndMode
// Check whether /etc/atomos_{cosmosName} directory exists or not.
// 检查/etc/atomos_{cosmosName}目录是否存在。
func (c *NodeConfig) CheckEtcPathOwnerAndMode(u *user.User) *Error {
	if err := NewPath(c.GetEtcPath()).CheckDirectoryOwnerAndMode(u, EtcPerm); err != nil {
		return err.AutoStack(nil, nil)
	}
	fmt.Printf("Node %s Etc Path Validated: %s\n", c.NodeName, c.GetEtcPath())
	return nil
}

func (c *NodeConfig) GetConfigFile() (*Path, *Error) {
	confPath := NewPath(c.GetConfigPath())
	if err := confPath.Refresh(); err != nil {
		return nil, err.AutoStack(nil, nil)
	}
	fmt.Printf("Node %s Config Path: %s\n", c.NodeName, c.GetConfigPath())
	return confPath, nil
}

func (c *NodeConfig) LoadConfig() (bool, *Error) {
	confPath, err := c.GetConfigFile()
	if err != nil {
		return false, err.AutoStack(nil, nil)
	}
	c.path = confPath

	conf := &NodeYAMLConfig{}
	if !confPath.Exist() {
		return false, nil
	}

	buf, er := ioutil.ReadFile(confPath.path)
	if er != nil {
		return true, NewErrorf(ErrCosmosConfigInvalid, "read config failed, err=(%v)", er).AutoStack(nil, nil)
	}
	er = yaml.Unmarshal(buf, conf)
	if er != nil {
		return true, NewErrorf(ErrCosmosConfigInvalid, "load config failed, err=(%v)", er).AutoStack(nil, nil)
	}
	c.path = confPath
	c.config = conf
	return true, nil
}

func (c *NodeConfig) InitConfig(cosmosName, nodeName string, u *user.User, g *user.Group) *Error {
	if err := c.CreateVarRunPathIfNotExist(u, g); err != nil {
		return err.AutoStack(nil, nil)
	}
	if err := c.CreateVarLogPathIfNotExist(u, g); err != nil {
		return err.AutoStack(nil, nil)
	}
	if err := c.CreateEtcPathIfNotExist(u, g); err != nil {
		return err.AutoStack(nil, nil)
	}
	exist, err := c.LoadConfig()
	if err != nil {
		return err.AutoStack(nil, nil)
	}
	if exist {
		if c.config.Cosmos != cosmosName {
			return NewErrorf(ErrCosmosConfigInvalid, "Node %s Config Invalid Cosmos Name, name=(%s)\n", nodeName, c.config.Cosmos).AutoStack(nil, nil)
		}
		if c.config.Node != nodeName {
			return NewErrorf(ErrCosmosConfigInvalid, "Node %s Config Invalid Node Name, name=(%s)\n", nodeName, c.config.Cosmos).AutoStack(nil, nil)
		}
		if c.config.LogPath != c.GetVarLogPath() {
			fmt.Println("WARN: Log path has changed to", c.config.LogPath)
		}
		if c.config.RunPath != c.GetVarRunPath() {
			fmt.Println("WARN: Run path has changed to", c.config.RunPath)
		}
		if c.config.EtcPath != c.GetEtcPath() {
			fmt.Println("WARN: Etc path has changed to", c.config.EtcPath)
		}
		fmt.Printf("WARN: Node %s Config Updated\n", nodeName)
	} else {
		c.config = &NodeYAMLConfig{
			Cosmos:          cosmosName,
			Node:            nodeName,
			LogLevel:        "DEBUG",
			LogPath:         c.GetVarLogPath(),
			BuildPath:       "",
			BinPath:         "",
			RunPath:         c.GetVarRunPath(),
			EtcPath:         c.GetEtcPath(),
			EnableCert:      nil,
			EnableServer:    nil,
			CustomizeConfig: nil,
		}
	}

	buf, er := yaml.Marshal(c.config)
	if er != nil {
		return NewErrorf(ErrPathSaveFileFailed, "node config marshal failed, err=(%v)", er).AutoStack(nil, nil)
	}
	if er = ioutil.WriteFile(c.path.path, buf, EtcPerm); er != nil {
		return NewErrorf(ErrPathSaveFileFailed, "node config save failed, err=(%v)", er).AutoStack(nil, nil)
	}
	return nil
}

func (c *NodeConfig) ValidateConfig(u *user.User) *Error {
	if err := c.CheckVarRunPathOwnerAndMode(u); err != nil {
		return err.AutoStack(nil, nil)
	}
	if err := c.CheckVarLogPathOwnerAndMode(u); err != nil {
		return err.AutoStack(nil, nil)
	}
	if err := c.CheckEtcPathOwnerAndMode(u); err != nil {
		return err.AutoStack(nil, nil)
	}
	exist, err := c.LoadConfig()
	if err != nil {
		return err.AutoStack(nil, nil)
	}
	if !exist {
		return NewErrorf(ErrCosmosConfigNotFound, "node config not found, node=(%s)", c.NodeName)
	}

	if c.config.Cosmos != c.CosmosName {
		return NewErrorf(ErrCosmosConfigInvalid, "cosmos not match, name=(%s)", c.config.Cosmos).AutoStack(nil, nil)
	}
	if c.config.Node != c.NodeName {
		return NewErrorf(ErrCosmosConfigInvalid, "node not match, name=(%s)", c.config.Node).AutoStack(nil, nil)
	}
	if c.config.LogPath != c.GetVarLogPath() {
		fmt.Printf("WARN: Node \"%s\" Log Path has changed to %s\n", c.NodeName, c.config.LogPath)
	}
	if c.config.RunPath != c.GetVarRunPath() {
		fmt.Printf("WARN: Node \"%s\" Run Path has changed to %s\n", c.NodeName, c.config.RunPath)
	}
	if c.config.EtcPath != c.GetEtcPath() {
		fmt.Printf("WARN: Node \"%s\" Etc Path has changed to %s\n", c.NodeName, c.config.EtcPath)
	}
	if c.config.BuildPath != "" {
		buildPath := NewPath(c.config.BuildPath)
		err := buildPath.Refresh()
		if err == nil && !buildPath.Exist() {
			err = NewError(ErrCosmosConfigBuildPathInvalid, "path not exists").AutoStack(nil, nil)
		}
		if err != nil {
			fmt.Printf("Error: Node \"%s\" Config Invalid Build Path, name=(%s),err=(%v)\n", c.NodeName, c.config.Node, err)
			return err.AutoStack(nil, nil)
		}
		fmt.Printf("Notice: Node \"%s\" Config Build Path Validated: %s\n", c.NodeName, buildPath.path)
	}
	if c.config.BinPath != "" {
		binPath := NewPath(c.config.BinPath)
		err := binPath.Refresh()
		if err != nil {
			fmt.Printf("Error: Node \"%s\" Config Invalid Bin Path, name=(%s),err=(%v)\n", c.NodeName, c.config.Node, err)
			return err.AutoStack(nil, nil)
		}
		fmt.Printf("Node \"%s\" Config Bin Path Validated: %s\n", c.NodeName, binPath.path)
	}
	fmt.Printf("Node \"%s\" Config Validated\n", c.NodeName)
	return nil
}

// Check

func CheckCosmosName(cosmosName string) bool {
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

func CheckNodeName(nodeName string) bool {
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
