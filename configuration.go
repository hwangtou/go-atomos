package go_atomos

import (
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"os/user"
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

func InitConfig(cosmosUser, cosmosGroup, cosmosName string, nodeNameList []string) *Error {
	u, g, err := CheckSystemUserAndGroup(cosmosUser, cosmosGroup)
	if err != nil {
		return err.AutoStack(nil, nil)
	}
	supervisor := &SupervisorConfig{CosmosName: cosmosName, NodeNameList: nodeNameList}
	if err = supervisor.InitConfig(u, g); err != nil {
		return err.AutoStack(nil, nil)
	}
	for _, nodeName := range supervisor.NodeNameList {
		node := &NodeConfig{NodeName: nodeName}
		if err = node.InitConfig(cosmosName, nodeName, u, g); err != nil {
			return err.AutoStack(nil, nil)
		}
	}
	fmt.Println("Config initialized")
	return nil
}

func ValidateConfig(cosmosGroup, cosmosName string) *Error {
	u, g, err := CheckCurrentUserAndGroup(cosmosGroup)
	if err != nil {
		return err.AutoStack(nil, nil)
	}
	supervisor := &SupervisorConfig{CosmosName: cosmosName, NodeNameList: nil}
	if err = supervisor.ValidateConfig(u, g); err != nil {
		return err.AutoStack(nil, nil)
	}
	for _, nodeName := range supervisor.config.NodeList {
		node := &NodeConfig{NodeName: nodeName}
		if err = node.ValidateConfig(cosmosName, nodeName, u, g); err != nil {
			return err.AutoStack(nil, nil)
		}
	}
	fmt.Println("Config initialized")
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

// Supervisor

type SupervisorConfig struct {
	CosmosName   string
	NodeNameList []string

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
	c.path = confPath
	c.config = conf
	return true, nil
}

func (c *SupervisorConfig) InitConfig(u *user.User, g *user.Group) *Error {
	// Check whether cosmos name is legal.
	// 检查cosmos名称是否合法。
	if !CheckCosmosName(c.CosmosName) {
		return NewError(ErrCosmosNameInvalid, "invalid cosmos name").AutoStack(nil, nil)
	}
	for _, nodeName := range c.NodeNameList {
		if !CheckNodeName(nodeName) {
			return NewError(ErrCosmosNodeNameInvalid, "invalid node name").AutoStack(nil, nil)
		}
	}

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
			fmt.Println("node=", nodeName)
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

func (c *SupervisorConfig) ValidateConfig(u *user.User, g *user.Group) *Error {

}

// Node

type NodeConfig struct {
	CosmosName string
	NodeName   string

	path   *Path
	config *NodeYAMLConfig
}

func (c *NodeConfig) GetConfigPath() string {
	//TODO implement me
	panic("implement me")
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
	// Check whether cosmos name is legal.
	// 检查cosmos名称是否合法。
	if !CheckCosmosName(cosmosName) {
		return NewError(ErrCosmosNameInvalid, "invalid cosmos name").AutoStack(nil, nil)
	}
	if !CheckNodeName(nodeName) {
		return NewError(ErrCosmosNameInvalid, "invalid node name").AutoStack(nil, nil)
	}

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

func (c *NodeConfig) ValidateConfig(name string, name2 string, u *user.User, g *user.Group) *Error {

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
