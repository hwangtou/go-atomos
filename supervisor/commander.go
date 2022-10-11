package main

import (
	"errors"
	"flag"
	"fmt"
	go_atomos "github.com/hwangtou/go-atomos"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	fmt.Println("Welcome to Atomos Commander!")
	fmt.Println("Now:\t", time.Now().Format("2006-01-02 15:04:05 MST -07:00"))

	var (
		action     = flag.String("action", "help", "[init|validate|list|build]")
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
		if *cosmosName == "" || nodeNameList == nil || nodeNameList[0] == "" {
			fmt.Println("Error: -cosmos or -node is empty")
			goto help
		}
		if *groupName == "" {
			fmt.Println("Error: We should know which group is allowed to access, please give a group name with -g argument.")
			goto help
		}
		if *userName == "" {
			u, er := user.Current()
			if er != nil {
				fmt.Println("Error: Get default user failed,", er)
				goto help
			}
			*userName = u.Username
		}
		if err := go_atomos.InitConfig(*userName, *groupName, *cosmosName, nodeNameList); err != nil {
			fmt.Println("Error: Config init failed:", err)
			return
		}
		return
	case "validate":
		if *cosmosName == "" { //|| nodeNameList == nil || nodeNameList[0] == "" {
			fmt.Println("Error: -cosmos is empty")
			goto help
		}
		if *groupName == "" {
			fmt.Println("Error: We should know which group is allowed to access, please give a group name with -g argument.")
			goto help
		}
		if err := go_atomos.ValidateConfig(*groupName, *cosmosName); err != nil {
			fmt.Println("Error: Config validate failed:", err)
			return
		}
		return
	case "list":
		supervisorList, err := go_atomos.ListAllConfig()
		if err != nil {
			fmt.Println("Error: List atomos failed:", err)
			return
		}
		for _, supervisorConfig := range supervisorList {
			fmt.Printf(" * Cosmos Name: %s\n", supervisorConfig.CosmosName)
			for _, nodeConfig := range supervisorConfig.NodeConfigList {
				fmt.Printf(" +-- Node Name: %s\n", nodeConfig.NodeName)
			}
			fmt.Print("Keepalive Nodes: ")
			for _, keepAlive := range supervisorConfig.KeepaliveNodes {
				fmt.Printf(keepAlive, ", ")
			}
			fmt.Println()
		}
		return
	case "build":
		if *cosmosName == "" || nodeNameList == nil {
			fmt.Println("Error: -cosmos or -node is empty")
			goto help
		}
		if er := go_atomos.BuildConfig(*cosmosName, *goPath, nodeNameList); er != nil {
			fmt.Println("Error: Config build failed:", er)
			return
		}
		return
	case "startup_supervisor":
		if *cosmosName == "" {
			fmt.Println("Error: -cosmos is empty")
			goto help
		}
		if er := startupSupervisor(*cosmosName); er != nil {
			fmt.Println("Error: Startup Supervisor failed:", er)
			return
		}
		return
		//case "status":
		//	if *cosmosName == "" || nodeNameList == nil {
		//		fmt.Println("Error: -cosmos or -node is empty")
		//		goto help
		//	}
		//	NewPath(VarRunPath + AtomosPrefix + *cosmosName)
	}
help:
	fmt.Println("Usage:\n\t-action=[init|validate|list|build|daemon]\n\t-user={user_name}\n\t-group={group_name}\n\t-cosmos={cosmos_name}\n\t-go={go_binary_path}\n\t-node={node_name_1,node_name_2,...}")
	fmt.Println("Example (Init):\n\tsudo atomos_commander -action=init -user=`whoami` -group=`id -ng` -cosmos=hello_cosmos -node=cosmos1,cosmos2")
	fmt.Println("Example (Validate):\n\tatomos_commander -action=validate -user=`whoami` -group=`id -ng` -cosmos=hello_cosmos")
	fmt.Println("Example (List):\n\tatomos_commander -action=list")
	fmt.Println("Example (Build):\n\tatomos_commander -action=build -cosmos=hello_cosmos -node=cosmos1")
	fmt.Println("Example (StartUp Supervisor):\n\tatomos_commander -action=startup_supervisor -cosmos=hello_cosmos")
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

//// INIT
//
//func initPaths(cosmosUser, cosmosGroup, cosmosName string, nodeNameList []string) error {
//	if os.Getuid() != 0 {
//		return errors.New("init supervisor should run under root")
//	}
//	// Check user.
//	u, er := user.Lookup(cosmosUser)
//	if er != nil {
//		if _, ok := er.(user.UnknownUserError); ok {
//			fmt.Println("Error: Unknown user, err=", er)
//			return er
//		}
//		fmt.Println("Error: Lookup user failed, err=", er)
//		return er
//	}
//	// Check group.
//	g, er := user.LookupGroup(cosmosGroup)
//	if er != nil {
//		if _, ok := er.(*user.UnknownGroupError); ok {
//			fmt.Println("Error: Unknown group, err=", er)
//			return er
//		}
//		fmt.Println("Error: Lookup group failed, err=", er)
//		return er
//	}
//	userGroups, er := u.GroupIds()
//	if er != nil {
//		fmt.Println("Error: Iter user group failed, err=", er)
//		return er
//	}
//	inGroup := false
//	for _, gid := range userGroups {
//		if gid == g.Gid {
//			inGroup = true
//			break
//		}
//	}
//	if !inGroup {
//		fmt.Println("Error: User not in group")
//		return er
//	}
//	if er := initSupervisorPaths(cosmosName, nodeNameList, u, g); er != nil {
//		fmt.Println("Error:", er)
//		return er
//	}
//	for _, nodeName := range nodeNameList {
//		if er := initNodePaths(cosmosName, nodeName, u, g); er != nil {
//			fmt.Println("Error:", er)
//			return er
//		}
//	}
//	fmt.Println("Config initialized")
//	return nil
//}
//
//func initSupervisorPaths(cosmosName string, nodeNameList []string, u *user.User, g *user.Group) error {
//	// Check whether cosmos name is legal.
//	// 检查cosmos名称是否合法。
//	if !checkCosmosName(cosmosName) {
//		return errors.New("invalid cosmos name")
//	}
//
//	for _, nodeName := range nodeNameList {
//		if !checkNodeName(nodeName) {
//			return errors.New("invalid node name")
//		}
//	}
//
//	// Check whether /var/run/atomos_{cosmosName} directory exists or not.
//	// 检查/var/run/atomos_{cosmosName}目录是否存在。
//	varRunPath := VarRunPath + AtomosPrefix + cosmosName
//	if er := NewPath(varRunPath).CreateDirectoryIfNotExist(u, g, VarRunPerm); er != nil {
//		return er
//	}
//	fmt.Printf("Supervisor Run Path: %s\n", varRunPath)
//
//	// Check whether /var/log/atomos_{cosmosName} directory exists or not.
//	// 检查/var/log/atomos_{cosmosName}目录是否存在。
//	varLogPath := VarLogPath + AtomosPrefix + cosmosName
//	if er := NewPath(varLogPath).CreateDirectoryIfNotExist(u, g, VarLogPerm); er != nil {
//		return er
//	}
//	fmt.Printf("Supervisor Log Path: %s\n", varLogPath)
//
//	// Check whether /etc/atomos_{cosmosName} directory exists or not.
//	// 检查/etc/atomos_{cosmosName}目录是否存在。
//	etcPath := EtcPath + AtomosPrefix + cosmosName
//	if er := NewPath(etcPath).CreateDirectoryIfNotExist(u, g, EtcPerm); er != nil {
//		return er
//	}
//	fmt.Printf("Supervisor Etc Path: %s\n", etcPath)
//
//	// Create supervisor.conf
//	supervisorConfigPath := EtcPath + AtomosPrefix + cosmosName + "/supervisor.conf"
//	confPath := NewPath(supervisorConfigPath)
//	if er := confPath.Refresh(); er != nil {
//		return er
//	}
//	fmt.Printf("Supervisor Config Path: %s\n", supervisorConfigPath)
//
//	conf := &go_atomos.SupervisorYAMLConfig{}
//	if confPath.exist() {
//		buf, er := ioutil.ReadFile(confPath.path)
//		if er != nil {
//			return er
//		}
//		er = yaml.Unmarshal(buf, conf)
//		if er != nil {
//			return er
//		}
//		if conf.Cosmos != cosmosName {
//			fmt.Printf("Error: Supervisor Config Invalid Cosmos Name, name=(%s)\n", conf.Cosmos)
//			return errors.New("invalid cosmos name")
//		}
//		if conf.LogPath != varLogPath {
//			fmt.Println("Error: Supervisor Log Path has changed to", conf.LogPath)
//		}
//		if conf.RunPath != varRunPath {
//			fmt.Println("Error: Supervisor Run Path has changed to", conf.RunPath)
//		}
//		if conf.EtcPath != etcPath {
//			fmt.Println("Error: Supervisor Etc Path has changed to", conf.EtcPath)
//		}
//		for _, nodeName := range nodeNameList {
//			fmt.Println("node=", nodeName)
//			has := false
//			for _, s := range conf.NodeList {
//				if s == nodeName {
//					has = true
//					break
//				}
//			}
//			if !has {
//				conf.NodeList = append(conf.NodeList, nodeName)
//			}
//		}
//	} else {
//		conf.Cosmos = cosmosName
//		conf.NodeList = nodeNameList
//		conf.LogPath = varLogPath
//		conf.LogLevel = "DEBUG"
//		conf.RunPath = varRunPath
//		conf.EtcPath = etcPath
//	}
//	buf, er := yaml.Marshal(conf)
//	if er != nil {
//		return er
//	}
//	if er := confPath.saveAndCreateFileIfNotExist(buf, EtcPerm); er != nil {
//		return er
//	}
//	return nil
//}
//
//func initNodePaths(cosmosName, nodeName string, u *user.User, g *user.Group) error {
//	// Check whether cosmos name is legal.
//	// 检查cosmos名称是否合法。
//	if !checkNodeName(nodeName) {
//		return errors.New("invalid node name")
//	}
//
//	// Check whether /var/run/atomos_{cosmosName} directory exists or not.
//	// 检查/var/run/atomos_{cosmosName}目录是否存在。
//	varRunPath := VarRunPath + AtomosPrefix + cosmosName + "/" + nodeName
//	if er := NewPath(varRunPath).CreateDirectoryIfNotExist(u, g, VarRunPerm); er != nil {
//		return er
//	}
//	fmt.Printf("Node %s Run Path: %s\n", nodeName, varRunPath)
//
//	// Check whether /var/log/atomos_{cosmosName} directory exists or not.
//	// 检查/var/log/atomos_{cosmosName}目录是否存在。
//	varLogPath := VarLogPath + AtomosPrefix + cosmosName + "/" + nodeName
//	if er := NewPath(varLogPath).CreateDirectoryIfNotExist(u, g, VarLogPerm); er != nil {
//		return er
//	}
//	fmt.Printf("Node %s Log Path: %s\n", nodeName, varLogPath)
//
//	// Check whether /etc/atomos_{cosmosName} directory exists or not.
//	// 检查/etc/atomos_{cosmosName}目录是否存在。
//	etcPath := EtcPath + AtomosPrefix + cosmosName + "/" + nodeName
//	if er := NewPath(etcPath).CreateDirectoryIfNotExist(u, g, EtcPerm); er != nil {
//		return er
//	}
//	fmt.Printf("Node %s Etc Path: %s\n", nodeName, etcPath)
//
//	// Create {node}.conf
//	configPath := EtcPath + AtomosPrefix + cosmosName + "/" + nodeName + ".conf"
//	confPath := NewPath(configPath)
//	if er := confPath.Refresh(); er != nil {
//		return er
//	}
//	fmt.Printf("Node %s Config Path: %s\n", nodeName, configPath)
//
//	conf := &go_atomos.NodeYAMLConfig{
//		Cosmos:          cosmosName,
//		Node:            nodeName,
//		LogLevel:        "DEBUG",
//		LogPath:         varLogPath,
//		BuildPath:       "",
//		BinPath:         "",
//		RunPath:         varRunPath,
//		EtcPath:         etcPath,
//		EnableCert:      nil,
//		EnableServer:    nil,
//		CustomizeConfig: nil,
//	}
//	if confPath.exist() {
//		buf, er := ioutil.ReadFile(confPath.path)
//		if er != nil {
//			return er
//		}
//		er = yaml.Unmarshal(buf, conf)
//		if er != nil {
//			return er
//		}
//		if conf.Cosmos != cosmosName {
//			fmt.Printf("Error: Node %s Config Invalid Cosmos Name, name=(%s)\n", nodeName, conf.Cosmos)
//			return errors.New("invalid cosmos name")
//		}
//		if conf.Node != nodeName {
//			fmt.Printf("Error: Node %s Config Invalid Node Name, name=(%s)\n", nodeName, conf.Node)
//			return errors.New("invalid cosmos name")
//		}
//		if conf.LogPath != varLogPath {
//			fmt.Println("Notice: Log path has changed to", conf.LogPath)
//		}
//		if conf.RunPath != varRunPath {
//			fmt.Println("Notice: Run path has changed to", conf.RunPath)
//		}
//		if conf.EtcPath != etcPath {
//			fmt.Println("Notice: Etc path has changed to", conf.EtcPath)
//		}
//		fmt.Printf("Notice: Node %s Config Updated\n", nodeName)
//	}
//
//	buf, er := yaml.Marshal(conf)
//	if er != nil {
//		return er
//	}
//	if er := confPath.saveAndCreateFileIfNotExist(buf, EtcPerm); er != nil {
//		return er
//	}
//	return nil
//}

//// VALIDATE
//
//func validatePaths(cosmosGroup, cosmosName string) error {
//	u, er := user.Current()
//	if er != nil {
//		return er
//	}
//	// Check group.
//	g, er := user.LookupGroup(cosmosGroup)
//	if er != nil {
//		if _, ok := er.(*user.UnknownGroupError); ok {
//			fmt.Println("Error: Unknown group, err=", er)
//			return er
//		}
//		fmt.Println("Error: Lookup group failed, err=", er)
//		return er
//	}
//	userGroups, er := u.GroupIds()
//	if er != nil {
//		fmt.Println("Error: Iter user group failed, err=", er)
//		return er
//	}
//	inGroup := false
//	for _, gid := range userGroups {
//		if gid == g.Gid {
//			inGroup = true
//			break
//		}
//	}
//	if !inGroup {
//		fmt.Println("Error: User not in group")
//		return er
//	}
//	nodeNameList, er := validateSupervisorPath(cosmosName, u)
//	if er != nil {
//		return er
//	}
//	for _, nodeName := range nodeNameList {
//		if er := validateNodePath(cosmosName, nodeName, u); er != nil {
//			return er
//		}
//	}
//	fmt.Println("Config validated")
//	return nil
//}
//
//func validateSupervisorPath(cosmosName string, u *user.User) ([]string, error) {
//	// Check whether /var/run/atomos_{cosmosName} directory exists or not.
//	// 检查/var/run/atomos_{cosmosName}目录是否存在。
//	varRunPath := VarRunPath + AtomosPrefix + cosmosName
//	if er := NewPath(varRunPath).CheckDirectoryOwnerAndMode(u, VarRunPerm); er != nil {
//		return nil, er
//	}
//	fmt.Printf("Supervisor Run Path Validated: %s\n", varRunPath)
//
//	// Check whether /var/log/atomos_{cosmosName} directory exists or not.
//	// 检查/var/log/atomos_{cosmosName}目录是否存在。
//	varLogPath := VarLogPath + AtomosPrefix + cosmosName
//	if er := NewPath(varLogPath).CheckDirectoryOwnerAndMode(u, VarLogPerm); er != nil {
//		return nil, er
//	}
//	fmt.Printf("Supervisor Log Path Validated: %s\n", varLogPath)
//
//	// Check whether /etc/atomos_{cosmosName} directory exists or not.
//	// 检查/etc/atomos_{cosmosName}目录是否存在。
//	etcPath := EtcPath + AtomosPrefix + cosmosName
//	if er := NewPath(etcPath).CheckDirectoryOwnerAndMode(u, EtcPerm); er != nil {
//		return nil, er
//	}
//	fmt.Printf("Supervisor Etc Path Validated: %s\n", varLogPath)
//
//	// Get supervisor.conf
//	supervisorConfigPath := EtcPath + AtomosPrefix + cosmosName + "/supervisor.conf"
//	confPath := NewPath(supervisorConfigPath)
//	if er := confPath.Refresh(); er != nil {
//		return nil, er
//	}
//	fmt.Printf("Supervisor Config Path Validated: %s\n", supervisorConfigPath)
//
//	if !confPath.exist() {
//		return nil, errors.New("file not exist")
//	}
//	buf, er := ioutil.ReadFile(confPath.path)
//	if er != nil {
//		return nil, er
//	}
//	conf := &go_atomos.SupervisorYAMLConfig{}
//	er = yaml.Unmarshal(buf, conf)
//	if er != nil {
//		return nil, er
//	}
//	if conf.Cosmos != cosmosName {
//		return nil, errors.New("invalid cosmos name")
//	}
//	if conf.LogPath != varLogPath {
//		fmt.Println("Error: Supervisor Config Path has changed to", conf.LogPath)
//	}
//	if conf.RunPath != varRunPath {
//		fmt.Println("Error: Supervisor Run Path has changed to", conf.RunPath)
//	}
//	if conf.EtcPath != etcPath {
//		fmt.Println("Error: Supervisor Etc Path has changed to", conf.EtcPath)
//	}
//	fmt.Println("Supervisor Config Validated")
//	return conf.NodeList, nil
//}
//
//func validateNodePath(cosmosName, nodeName string, u *user.User) error {
//	// Check whether cosmos name is legal.
//	// 检查cosmos名称是否合法。
//	if !checkNodeName(nodeName) {
//		return errors.New("invalid node name")
//	}
//
//	// Check whether /var/run/atomos_{cosmosName} directory exists or not.
//	// 检查/var/run/atomos_{cosmosName}目录是否存在。
//	varRunPath := VarRunPath + AtomosPrefix + cosmosName + "/" + nodeName
//	if er := NewPath(varRunPath).CheckDirectoryOwnerAndMode(u, VarRunPerm); er != nil {
//		return er
//	}
//	fmt.Printf("Node %s Run Path Validated: %s\n", nodeName, varRunPath)
//
//	// Check whether /var/log/atomos_{cosmosName} directory exists or not.
//	// 检查/var/log/atomos_{cosmosName}目录是否存在。
//	varLogPath := VarLogPath + AtomosPrefix + cosmosName + "/" + nodeName
//	if er := NewPath(varLogPath).CheckDirectoryOwnerAndMode(u, VarLogPerm); er != nil {
//		return er
//	}
//	fmt.Printf("Node %s Log Path Validated: %s\n", nodeName, varLogPath)
//
//	// Check whether /etc/atomos_{cosmosName} directory exists or not.
//	// 检查/etc/atomos_{cosmosName}目录是否存在。
//	etcPath := EtcPath + AtomosPrefix + cosmosName + "/" + nodeName
//	if er := NewPath(etcPath).CheckDirectoryOwnerAndMode(u, EtcPerm); er != nil {
//		return er
//	}
//	fmt.Printf("Node %s Etc Path Validated: %s\n", nodeName, etcPath)
//
//	// Get {node}.conf
//	configPath := EtcPath + AtomosPrefix + cosmosName + "/" + nodeName + ".conf"
//	confPath := NewPath(configPath)
//	if er := confPath.Refresh(); er != nil {
//		return er
//	}
//	fmt.Printf("Node %s Config Path Validated: %s\n", nodeName, configPath)
//
//	if !confPath.exist() {
//		return errors.New("file not exist")
//	}
//	buf, er := ioutil.ReadFile(confPath.path)
//	if er != nil {
//		return er
//	}
//	conf := &go_atomos.NodeYAMLConfig{}
//	er = yaml.Unmarshal(buf, conf)
//	if er != nil {
//		return er
//	}
//	if conf.Cosmos != cosmosName {
//		fmt.Printf("Error: Node %s Config Invalid Cosmos Name, name=(%s)\n", nodeName, conf.Cosmos)
//		return errors.New("invalid cosmos name")
//	}
//	if conf.Node != nodeName {
//		fmt.Printf("Error: Node %s Config Invalid Node Name, name=(%s)\n", nodeName, conf.Node)
//		return errors.New("invalid cosmos name")
//	}
//	if conf.LogPath != varLogPath {
//		fmt.Println("Notice: Log path has changed to", conf.LogPath)
//	}
//	if conf.RunPath != varRunPath {
//		fmt.Println("Notice: Run path has changed to", conf.RunPath)
//	}
//	if conf.EtcPath != etcPath {
//		fmt.Println("Notice: Etc path has changed to", conf.EtcPath)
//	}
//	if conf.BuildPath != "" {
//		buildPath := NewPath(conf.BuildPath)
//		er := buildPath.Refresh()
//		if er == nil && !buildPath.exist() {
//			er = errors.New("path not exists")
//		}
//		if er != nil {
//			fmt.Printf("Error: Node %s Config Invalid Build Path, name=(%s),err=(%v)\n", nodeName, conf.Node, er)
//			return er
//		}
//		fmt.Printf("Notice: Node %s Config Build Path Validated: %s\n", nodeName, buildPath.path)
//	}
//	if conf.BinPath != "" {
//		binPath := NewPath(conf.BinPath)
//		er := binPath.Refresh()
//		if er != nil {
//			fmt.Printf("Error: Node %s Config Invalid Bin Path, name=(%s),err=(%v)\n", nodeName, conf.Node, er)
//			return er
//		}
//		fmt.Printf("Node %s Config Bin Path Validated: %s\n", nodeName, binPath.path)
//	}
//	return nil
//}

//// LIST
//
//func listProjects() error {
//	var nameList [][]string
//	etcPathDir, er := ioutil.ReadDir(EtcPath)
//	if er != nil {
//		return er
//	}
//	for _, dir := range etcPathDir {
//		if !dir.IsDir() {
//			continue
//		}
//		if !strings.HasPrefix(dir.Name(), AtomosPrefix) {
//			continue
//		}
//
//		var names []string
//		names = append(names, dir.Name())
//
//		dirPath := EtcPath + dir.Name()
//		dirInfo, er := ioutil.ReadDir(dirPath)
//		if er != nil {
//			return er
//		}
//		for _, info := range dirInfo {
//			if !info.IsDir() {
//				continue
//			}
//			names = append(names, info.Name())
//		}
//
//		nameList = append(nameList, names)
//	}
//	if er != nil {
//		return er
//	}
//	if len(nameList) == 0 {
//		fmt.Println("List: No Cosmos?")
//		return nil
//	}
//	fmt.Printf("List: %d Cosmos Found.\n", len(nameList))
//	for _, s := range nameList {
//		if len(s) == 0 {
//			continue
//		}
//		fmt.Printf(" * Cosmos Name: %s\n", s[0])
//		for i := 1; i < len(s); i += 1 {
//			fmt.Printf(" +-- Node Name: %s\n", s[i])
//		}
//	}
//	return nil
//}

//// BUILD
//
//func buildPaths(cosmosName, goPath string, nodeNameList []string) error {
//	supervisorConfigPath := EtcPath + AtomosPrefix + cosmosName + "/supervisor.conf"
//	buf, er := ioutil.ReadFile(supervisorConfigPath)
//	if er != nil {
//		return er
//	}
//	conf := &go_atomos.SupervisorYAMLConfig{}
//	er = yaml.Unmarshal(buf, conf)
//	if er != nil {
//		return er
//	}
//
//	now := time.Now().Format("20060102150405")
//	for _, nodeName := range nodeNameList {
//		if er := buildNodePath(cosmosName, goPath, nodeName, now); er != nil {
//			return er
//		}
//	}
//	fmt.Println("Config built")
//	return nil
//}
//
//func buildNodePath(cosmosName, goPath, nodeName, nowStr string) error {
//	// Check whether cosmos name is legal.
//	// 检查cosmos名称是否合法。
//	if !checkNodeName(nodeName) {
//		return errors.New("invalid node name")
//	}
//
//	// Get {node}.conf
//	configPath := EtcPath + AtomosPrefix + cosmosName + "/" + nodeName + ".conf"
//	confPath := NewPath(configPath)
//	if er := confPath.Refresh(); er != nil {
//		return er
//	}
//	fmt.Printf("Node %s Config Path Validated: %s\n", nodeName, configPath)
//
//	if !confPath.exist() {
//		return errors.New("file not exist")
//	}
//	buf, er := ioutil.ReadFile(confPath.path)
//	if er != nil {
//		return er
//	}
//	conf := &go_atomos.NodeYAMLConfig{}
//	er = yaml.Unmarshal(buf, conf)
//	if er != nil {
//		return er
//	}
//	if conf.Cosmos != cosmosName {
//		fmt.Printf("Error: Node %s Config Invalid Cosmos Name, name=(%s)\n", nodeName, conf.Cosmos)
//		return errors.New("invalid cosmos name")
//	}
//	if conf.Node != nodeName {
//		fmt.Printf("Error: Node %s Config Invalid Node Name, name=(%s)\n", nodeName, conf.Node)
//		return errors.New("invalid cosmos name")
//	}
//
//	buildPath := NewPath(conf.BuildPath)
//	if er := buildPath.Refresh(); er != nil || !buildPath.exist() {
//		fmt.Printf("Error: Node %s Config Invalid Build Path, path=(%s)\n", nodeName, conf.BuildPath)
//		return errors.New("invalid build path")
//	}
//	binPath := NewPath(conf.BinPath)
//	if er := binPath.Refresh(); er != nil {
//		fmt.Printf("Error: Node %s Config Invalid Bin Path, name=(%s),err=(%v)\n", nodeName, conf.BinPath, er)
//		return er
//	}
//
//	outPath := binPath.path + "/" + nodeName + "_" + nowStr
//	cmd := exec.Command(goPath, "build", "-o", outPath, buildPath.path)
//	cmd.Stdout = os.Stdout
//	cmd.Stderr = os.Stderr
//	cmd.Dir = path.Join(path.Dir(buildPath.path))
//	if er := cmd.Run(); er != nil {
//		fmt.Printf("Error: Node %s Config Built Failed: err=(%v)\n", nodeName, er)
//		return er
//	}
//
//	linkPath := binPath.path + "/" + nodeName + "_latest"
//	exec.Command("rm", linkPath).Run()
//
//	cmd = exec.Command("ln", "-s", outPath, linkPath)
//	cmd.Stdout = os.Stdout
//	cmd.Stderr = os.Stderr
//	if er := cmd.Run(); er != nil {
//		fmt.Printf("Error: Node %s Link Failed: err=(%v)\n", nodeName, er)
//		return er
//	}
//
//	fmt.Printf("Node %s Config Built Result: %s\n", nodeName, binPath.path)
//	return nil
//}

// Supervisor

func startupSupervisor(cosmosName string) error {
	supervisorConfigPath := EtcPath + AtomosPrefix + cosmosName + "/supervisor.conf"

	cmd := exec.Command("atomos_supervisor", "-config", supervisorConfigPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if er := cmd.Run(); er != nil {
		fmt.Printf("Error: Startup Supervisor Failed: err=(%v)\n", er)
		return er
	}

	fmt.Println("Startup Supervisor Executed")
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
		fmt.Println("Error: Refresh failed, err=", er)
		return er
	}
	if !p.exist() {
		if er := p.makeDir(perm); er != nil {
			fmt.Println("Error: Make dir failed, err=", er)
			return er
		}
		if er := p.changeOwnerAndMode(u, g, perm); er != nil {
			fmt.Println("Error: Change owner and mode failed, err=", er)
			return er
		}
	} else if !p.IsDir() {
		fmt.Println("Error: Path should be dir")
		return errors.New("path should be directory")
	} else {
		// Check its owner and mode.
		if er := p.confirmOwnerAndMode(g, perm); er != nil {
			fmt.Println("Error: Confirm owner and mode failed, err=", er)
			return er
		}
	}
	return nil
}

func (p *Path) CheckDirectoryOwnerAndMode(u *user.User, perm os.FileMode) error {
	if er := p.Refresh(); er != nil {
		fmt.Println("Error: Refresh failed, err=", er)
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

func (p *Path) saveAndCreateFileIfNotExist(buf []byte, perm os.FileMode) error {
	//if p.exist() {
	//	return nil
	//}
	er := ioutil.WriteFile(p.path, buf, perm)
	if er != nil {
		return er
	}
	return nil
}
