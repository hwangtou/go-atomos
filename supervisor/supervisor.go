package main

import (
	"fmt"
	atomos "github.com/hwangtou/go-atomos"
	"gopkg.in/fsnotify.v1"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v3/process"
)

func main() {
	atomos.SupervisorSet(map[string]atomos.AppUDSCommandFn{
		atomos.UDSPing:    udsPing,
		atomos.UDSStatus:  udsStatus,
		atomos.UDSStart:   udsStart,
		atomos.UDSStop:    udsStop,
		atomos.UDSRestart: udsRestart,
	})
	atomos.Main(runnable)
}

var runnable atomos.CosmosRunnable

func init() {
	mainScript := &supervisorMainScript{nodesMap: map[string]*nodeWatcher{}}
	runnable.SetMainScript(mainScript)
}

type supervisorMainScript struct {
	confPathW *configPathWatcher
	runPathW  *runPathWatcher

	nodeMutex sync.Mutex
	nodesMap  map[string]*nodeWatcher
}

func (s *supervisorMainScript) OnStartup() (err *atomos.Error) {
	err = initSupervisorConfigWatcher(s, path.Join(atomos.SharedApp().GetConfig().EtcPath, "supervisor.conf"))
	if err != nil {
		return err.AddStack(nil)
	}

	err = newRunPathWatcher(s)
	if err != nil {
		return err.AddStack(nil)
	}

	go s.confPathW.watch()
	go s.runPathW.watch()

	return nil
}

func (s *supervisorMainScript) OnShutdown() *atomos.Error {
	return nil
}

// Supervisor watcher

type configPathWatcher struct {
	main    *supervisorMainScript
	path    *atomos.Path
	config  *atomos.Config
	watcher *fsnotify.Watcher
}

func initSupervisorConfigWatcher(s *supervisorMainScript, supervisorConfPath string) *atomos.Error {
	s.confPathW = &configPathWatcher{
		main:   s,
		path:   atomos.NewPath(supervisorConfPath),
		config: nil,
	}
	// Check supervisor config path and create watcher.
	if err := s.confPathW.path.Refresh(); err != nil {
		return err.AddStack(nil)
	}
	watcher, er := fsnotify.NewWatcher()
	if er != nil {
		return atomos.NewErrorf(atomos.ErrSupervisorCreateWatcherFailed, "Supervisor: Create config watcher failed. err=(%v)", er).AddStack(nil)
	}
	s.confPathW.watcher = watcher

	// Check and first load supervisor config.
	if err := s.confPathW.refreshSupervisorConfig(); err != nil {
		atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: Supervisor config path error. err=(%v)", err.AddStack(nil))
	}

	if er := s.confPathW.watcher.Add(atomos.SharedApp().GetConfig().EtcPath); er != nil {
		return atomos.NewErrorf(atomos.ErrSupervisorCreateWatcherFailed, "Supervisor: Supervisor config watcher adds path failed. err=(%v)").AddStack(nil)
	}
	return nil
}

func (w *configPathWatcher) watch() {
	defer func() {
		if r := recover(); r != nil {
			atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: Supervisor config watcher panic. reason=(%v)", r)
		}
	}()

	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Chmod == fsnotify.Chmod {
				continue
			}
			if !strings.HasSuffix(event.Name, ".conf") {
				continue
			}
			p := atomos.NewPath(event.Name)
			if err := p.Refresh(); err != nil {
				atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: Config path error. err=(%v)", err.AddStack(nil))
				continue
			}
			name := filepath.Base(event.Name)
			if name == "supervisor.conf" {
				if err := w.refreshSupervisorConfig(); err != nil {
					atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: Supervisor config refresh error. err=(%v)", err.AddStack(nil))
				}
			} else {
				nodeName := strings.Trim(name, ".conf")
				n := w.main.getNode(nodeName)
				if n == nil {
					continue
				}
				if err := n.refreshNodeConfig(); err != nil {
					atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: Node config refresh error. err=(%v)", err.AddStack(nil))
				}
			}
		case er, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: File watcher error. err=(%v)", er)
		}
	}
}

func (w *configPathWatcher) refreshSupervisorConfig() *atomos.Error {
	if err := w.path.Refresh(); err != nil {
		return err.AddStack(nil)
	}
	newConf, err := atomos.NewSupervisorConfigFromYaml(w.path.GetPath())
	if err != nil {
		return atomos.NewErrorf(atomos.ErrSupervisorReadSupervisorConfigFailed, "Supervisor: Read supervisor config failed. err=(%v)", err).AddStack(nil)
	}

	// Nothing changed.
	if reflect.DeepEqual(w.config, newConf) {
		return nil
	}

	// Config validate.
	if err = newConf.ValidateSupervisorConfig(); err != nil {
		return err.AddStack(nil)
	}

	// What has changed? Some kinds of changes should only take effect after restart.
	notOk := false
	if w.config != nil {
		if newConf.Cosmos != w.config.Cosmos {
			notOk = true
			atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: Cosmos name has changed. Restart the whole cosmos to take effect.")
		}
		if newConf.ReporterUrl != w.config.ReporterUrl {
			notOk = true
			atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: Reporter URL has changed. Restart the whole cosmos to take effect.")
		}
		if newConf.ConfigerUrl != w.config.ConfigerUrl {
			notOk = true
			atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: Configer URL has changed. Restart the whole cosmos to take effect.")
		}
		if newConf.LogLevel != w.config.LogLevel {
			notOk = true
			atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: Log level has changed. Restart the whole cosmos to take effect.")
		}
		if newConf.LogPath != w.config.LogPath {
			notOk = true
			atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: Log path has changed. Restart the whole cosmos to take effect.")
		}
		if newConf.RunPath != w.config.RunPath {
			notOk = true
			atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: Run path has changed. Restart the whole cosmos to take effect.")
		}
		if newConf.EtcPath != w.config.EtcPath {
			notOk = true
			atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: Etc path has changed. Restart the whole cosmos to take effect.")
		}
	}

	// Check list.
	nodeChecker := map[string]bool{}
	for _, nodeName := range newConf.NodeList {
		if !atomos.CheckNodeName(nodeName) {
			notOk = true
			atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: Invalid node name. name=(%s)", nodeName)
		}
		if nodeChecker[nodeName] {
			notOk = true
			atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: Duplicated node name. name=(%s)", nodeName)
		}
		nodeChecker[nodeName] = true
	}

	// Check nodes.
	keepNodesMap := map[string]bool{}
	for _, keepNodeName := range newConf.KeepaliveNodeList {
		has := false
		for _, nodeName := range newConf.NodeList {
			if nodeName == keepNodeName {
				has = true
				break
			}
		}
		if !has {
			notOk = true
			atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: Keepalive node cannot found in node list. name=(%s)", keepNodeName)
		}
		if keepNodesMap[keepNodeName] {
			notOk = true
			atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: Duplicated keepalive node name. name=(%s)", keepNodeName)
		}
		keepNodesMap[keepNodeName] = true
	}

	// New or delete node?
	var oldNodeNameList, newNodeNameList []string
	var oldConfigNodeList []string
	if w.config != nil {
		oldConfigNodeList = w.config.NodeList
	}
	for _, oldNodeName := range oldConfigNodeList {
		isDelNode := true
		for _, newNodeName := range newConf.NodeList {
			if newNodeName == oldNodeName {
				isDelNode = false
				break
			}
		}
		if isDelNode {
			oldNodeNameList = append(oldNodeNameList, oldNodeName)
			if n := w.main.getNode(oldNodeName); n != nil {
				run, err := n.isRunning()
				if run || err != nil {
					notOk = true
					atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: Deleted node is still running or status error. name=(%s),err=(%v)", oldNodeName, err)
				}
			}
		}
	}
	for _, newNodeName := range newConf.NodeList {
		isNewNode := true
		for _, oldNodeName := range oldConfigNodeList {
			if oldNodeName == newNodeName {
				isNewNode = false
				break
			}
		}
		if isNewNode {
			newNodeNameList = append(newNodeNameList, newNodeName)
		}
	}

	if notOk {
		return atomos.NewError(atomos.ErrSupervisorRestartToTakeEffect, "Supervisor: Restart to take effect.").AddStack(nil)
	}
	w.config = newConf

	for _, nodeName := range oldNodeNameList {
		w.main.delNode(nodeName)
	}
	for _, nodeName := range newNodeNameList {
		w.main.newNode(nodeName)
	}

	return nil
}

// Run Path

type runPathWatcher struct {
	main    *supervisorMainScript
	path    *atomos.Path
	watcher *fsnotify.Watcher
}

func newRunPathWatcher(s *supervisorMainScript) *atomos.Error {
	s.runPathW = &runPathWatcher{
		main:    s,
		path:    atomos.NewPath(s.confPathW.config.RunPath),
		watcher: nil,
	}

	// Check run path and create watcher.
	if err := s.runPathW.path.Refresh(); err != nil {
		return err.AddStack(nil)
	}
	watcher, er := fsnotify.NewWatcher()
	if er != nil {
		return atomos.NewErrorf(atomos.ErrSupervisorCreateWatcherFailed, "Supervisor: Create run watcher failed. err=(%v)", er).AddStack(nil)
	}
	s.runPathW.watcher = watcher

	if err := s.runPathW.watcher.Add(s.runPathW.path.GetPath()); err != nil {
		return atomos.NewErrorf(atomos.ErrSupervisorCreateWatcherFailed, "Supervisor: Run watcher adds path failed. err=(%v)", err).AddStack(nil)
	}
	return nil
}

func (w *runPathWatcher) watch() {
	defer func() {
		if r := recover(); r != nil {
			atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: Supervisor run watcher panic. reason=(%v)", r)
		}
	}()

	for {
		select {
		// File Watcher
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Chmod == fsnotify.Chmod {
				continue
			}
			if !strings.HasSuffix(event.Name, ".pid") {
				continue
			}
			nodeName := strings.Trim(filepath.Base(event.Name), ".pid")
			n := w.main.getNode(nodeName)
			if n == nil {
				atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: Run watcher found unknown node. name=(%s)", nodeName)
				continue
			}
			n.onPIDFileChanged()
		case er, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: File watcher error. err=(%v)", er)
		// Cycle Watcher
		case <-time.After(time.Second):
			nodes := w.main.getAllNodes()
			for _, node := range nodes {
				node.onPIDProcessCheck()
			}
		}
	}
}

// Node

func (s *supervisorMainScript) getNode(nodeName string) *nodeWatcher {
	s.nodeMutex.Lock()
	n := s.nodesMap[nodeName]
	s.nodeMutex.Unlock()
	return n
}

func (s *supervisorMainScript) isKeepNode(nodeName string) bool {
	for _, name := range s.confPathW.config.KeepaliveNodeList {
		if name == nodeName {
			return true
		}
	}
	return false
}

func (s *supervisorMainScript) getAllNodes() []*nodeWatcher {
	s.nodeMutex.Lock()
	nodes := make([]*nodeWatcher, 0, len(s.nodesMap))
	for _, node := range s.nodesMap {
		nodes = append(nodes, node)
	}
	s.nodeMutex.Unlock()
	return nodes
}

func (s *supervisorMainScript) newNode(nodeName string) {
	s.nodeMutex.Lock()
	n, has := s.nodesMap[nodeName]
	if !has {
		n = newNodeWatcher(s, nodeName, s.isKeepNode(nodeName))
		s.nodesMap[nodeName] = n
		go n.watch()
	}
	s.nodeMutex.Unlock()

	if err := n.refreshNodeConfig(); err != nil {
		atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: Refresh node config error. err=(%v)", err.AddStack(nil))
	}
}

func (s *supervisorMainScript) delNode(nodeName string) {
	s.nodeMutex.Lock()
	n, has := s.nodesMap[nodeName]
	s.nodeMutex.Unlock()
	if !has {
		return
	}
	n.stopWatch()
	if run, err := n.isRunning(); run || err != nil {
		n.nextConfig = nil
		n.nextRemove = true
		return
	}
	s.nodeMutex.Lock()
	delete(s.nodesMap, nodeName)
	s.nodeMutex.Unlock()
}

// Node watcher

type nodeWatcher struct {
	main *supervisorMainScript

	nodeName  string
	keepalive bool
	isStopped bool
	client    *atomos.AppUDSClient
	cmdCh     chan *nodeCmd

	config     *atomos.Config
	configPath *atomos.Path

	nextConfig     *atomos.Config
	nextConfigPath *atomos.Path
	nextRemove     bool

	pid int
}

type nodeCmd struct {
	command  string
	buf      []byte
	callback atomos.AppUDSClientCallback
	timeout  time.Duration
}

func newNodeWatcher(s *supervisorMainScript, nodeName string, keepalive bool) *nodeWatcher {
	return &nodeWatcher{
		main:      s,
		nodeName:  nodeName,
		keepalive: keepalive,
		isStopped: false,
		client:    atomos.NewAppUDSClient(atomos.SharedLogging().PushProcessLog),
	}
}

func (w *nodeWatcher) refreshNodeConfig() *atomos.Error {
	p := atomos.NewPath(path.Join(w.main.confPathW.config.EtcPath, w.nodeName+".conf"))
	if err := p.Refresh(); err != nil {
		atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: Config path error. err=(%v)", err.AddStack(nil))
		return err
	}
	newConf, err := atomos.NewCosmosNodeConfigFromYamlPath(p.GetPath())
	if err != nil {
		return atomos.NewErrorf(atomos.ErrCosmosNodeReadSupervisorConfigFailed, "Supervisor: Read node config failed. err=(%v)", err).AddStack(nil)
	}

	// Nothing changed.
	if reflect.DeepEqual(w.config, newConf) {
		return nil
	}

	// Config validate.
	if err = newConf.ValidateCosmosNodeConfig(); err != nil {
		return err.AddStack(nil)
	}

	// First load.
	if w.config == nil {
		w.config = newConf
		w.configPath = p
		return nil
	}

	// Update load. What has changed? Some kinds of changes should only take effect after restart.
	notOk := false
	if run, err := w.isRunning(); run || err != nil {
		if w.config != nil {
			if newConf.Cosmos != w.config.Cosmos {
				notOk = true
				atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: Cosmos name has changed. Restart the whole cosmos to take effect.")
			}
			if newConf.Node != w.config.Node {
				notOk = true
				atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: Cosmos name has changed. Restart the whole cosmos to take effect.")
			}
			if newConf.ReporterUrl != w.config.ReporterUrl {
				notOk = true
				atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: Reporter URL has changed. Restart the whole cosmos to take effect.")
			}
			if newConf.ConfigerUrl != w.config.ConfigerUrl {
				notOk = true
				atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: Configer URL has changed. Restart the whole cosmos to take effect.")
			}
			if newConf.LogLevel != w.config.LogLevel {
				notOk = true
				atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: Log level has changed. Restart the whole cosmos to take effect.")
			}
			if newConf.LogPath != w.config.LogPath {
				notOk = true
				atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: Log path has changed. Restart the whole cosmos to take effect.")
			}
			if newConf.RunPath != w.config.RunPath {
				notOk = true
				atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: Run path has changed. Restart the whole cosmos to take effect.")
			}
			if newConf.EtcPath != w.config.EtcPath {
				notOk = true
				atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: Etc path has changed. Restart the whole cosmos to take effect.")
			}
		}
		w.nextConfig = newConf
		w.nextConfigPath = p
		w.nextRemove = false
	}

	if notOk {
		return atomos.NewError(atomos.ErrCosmosNodeRestartToTakeEffect, "Supervisor: Restart to take effect.").AddStack(nil)
	}
	w.config = newConf
	w.configPath = p

	return nil
}

func (w *nodeWatcher) watch() {
	defer func() {
		if r := recover(); r != nil {
			atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: Node watcher panic. reason=(%v)", r)
		}
	}()

	for {
		var conn *atomos.AppUDSConn
		var err *atomos.Error
		var lastSent time.Time

		if w.config == nil {
			goto wait
		}
		if run, err := w.isRunning(); err != nil || !run {
			goto wait
		}
		if w.isStopped {
			return
		}
		conn, err = w.client.Dial(path.Join(w.config.RunPath, w.config.Node+".socket"))
		if err != nil {
			atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Warn, "Supervisor: Node dial failed. err=(%v)", err.AddStack(nil))
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
							atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Debug, "Supervisor: Node watcher ping-pong error. name=(%s),err=(%v)", w.nodeName, err.AddStack(nil))
						} else {
							//atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Debug, "Supervisor: Node watcher ping-pong. name=(%s),packet=(%v)", w.nodeName, packet)
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
						atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Debug, "Supervisor: Node watcher send error. name=(%s),err=(%v)", w.nodeName, err.AddStack(nil))
					} else {
						//atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Debug, "Supervisor: Node watcher send. name=(%s)", w.nodeName)
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
}

func (w *nodeWatcher) stopWatch() {
	w.isStopped = true
}

func (w *nodeWatcher) getPIDFile() (pid int, err *atomos.Error) {
	c := w.config
	if c == nil {
		return -1, atomos.NewError(atomos.ErrCosmosNodeConfigInvalid, "Supervisor: Get node config failed.").AddStack(nil)
	}
	p := atomos.NewPath(path.Join(c.RunPath, c.Node+".pid"))
	if err = p.Refresh(); err != nil {
		return -1, err.AddStack(nil)
	}
	if !p.Exist() {
		return -1, nil
	}
	pidBuf, er := os.ReadFile(p.GetPath())
	if er != nil {
		return -1, atomos.NewErrorf(atomos.ErrCosmosNodePIDFileInvalid, "Supervisor: Get pid failed. err=(%v)", er).AddStack(nil)
	}

	processID, er := strconv.ParseInt(string(pidBuf), 10, 64)
	if er != nil {
		return -1, atomos.NewErrorf(atomos.ErrCosmosNodePIDFileInvalid, "Supervisor: Parse pid failed. err=(%v)", er).AddStack(nil)
	}
	return int(processID), nil
}

func (w *nodeWatcher) getProcess() (*process.Process, *atomos.Error) {
	pid, err := w.getPIDFile()
	if err != nil {
		return nil, err.AddStack(nil)
	}
	if pid < 0 {
		return nil, nil
	}
	p, er := process.NewProcess(int32(pid))
	if er != nil {
		return nil, atomos.NewErrorf(atomos.ErrCosmosNodeGetProcessFailed, "Supervisor: Get node process failed. err=(%v)", er).AddStack(nil)
	}
	return p, nil
}

func (w *nodeWatcher) isRunning() (bool, *atomos.Error) {
	p, err := w.getProcess()
	if err != nil {
		return false, err.AddStack(nil)
	}
	if p == nil {
		return false, nil
	}
	run, er := p.IsRunning()
	if er != nil {
		return false, atomos.NewErrorf(atomos.ErrCosmosNodeGetProcessFailed, "Supervisor: Get node process running info failed. err=(%v)", er).AddStack(nil)
	}
	return run, nil
}

func (w *nodeWatcher) build(goPath string, buildOptions []string) *atomos.Error {
	nowStr := time.Now().Format("20060102150405")
	c := w.config
	if w.nextConfig != nil {
		c = w.nextConfig
		atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Info, "Supervisor: Build with new config, which is still not take affect yet.")
	}

	// Build path.
	buildPath := atomos.NewPath(c.BuildPath)
	if err := buildPath.Refresh(); err != nil {
		return err.AddStack(nil)
	}
	if !buildPath.Exist() {
		return atomos.NewError(atomos.ErrCosmosNodeConfigInvalid, "Supervisor: Build path is not exists.").AddStack(nil)
	}
	// Out binary path.
	outPath := atomos.NewPath(path.Join(c.BinPath, c.Node+"_"+nowStr))
	if err := outPath.Refresh(); err != nil {
		return err.AddStack(nil)
	}
	args := []string{"build"}
	if len(buildOptions) > 0 {
		args = append(args, buildOptions...)
	}
	args = append(args, "-o", outPath.GetPath(), buildPath.GetPath())
	cmd := exec.Command(goPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = path.Join(path.Dir(buildPath.GetPath()))
	if er := cmd.Run(); er != nil {
		return atomos.NewErrorf(atomos.ErrCosmosNodeBuildBinaryFailed, "Supervisor: Node builds failed. name=(%s),err=(%v)\n", c.Node, er).AddStack(nil)
	}

	linkPath := atomos.NewPath(path.Join(c.BinPath, c.Node+"_latest"))
	if err := linkPath.Refresh(); err != nil {
		return err.AddStack(nil)
	}
	_ = exec.Command("rm", linkPath.GetPath()).Run()

	cmd = exec.Command("ln", "-s", outPath.GetPath(), linkPath.GetPath())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if er := cmd.Run(); er != nil {
		return atomos.NewErrorf(atomos.ErrCosmosNodeBuildBinaryFailed, "Supervisor: Node link binary failed. name=(%s),err=(%v)\n", c.Node, er).AddStack(nil)
	}

	atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Info, "Supervisor: Node binary builds succeed. name=(%s),bin=(%s)", c.Node, outPath.GetPath())
	return nil
}

func (w *nodeWatcher) start() *atomos.Error {
	run, err := w.isRunning()
	if err != nil {
		return err.AddStack(nil)
	}
	if run {
		return atomos.NewError(atomos.ErrCosmosNodeIsAlreadyRunning, "Supervisor: Node is already running.").AddStack(nil)
	}

	if w.nextConfig != nil {
		w.config = w.nextConfig
		w.configPath = w.nextConfigPath
		w.nextConfig = nil
		w.nextConfigPath = nil
		atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Info, "Supervisor: Build with new config, which is still not take affect yet.")
	}
	c, p := w.config, w.configPath

	binPath := atomos.NewPath(path.Join(c.BinPath, c.Node+"_latest"))
	if err = binPath.Refresh(); err != nil {
		return err.AddStack(nil)
	}

	cmd := exec.Command(binPath.GetPath(), fmt.Sprintf("--config=%s", p.GetPath()))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if er := cmd.Run(); er != nil {
		return atomos.NewErrorf(atomos.ErrCosmosNodeBuildBinaryFailed, "Supervisor: Node link binary failed. name=(%s),err=(%v)\n", c.Node, er).AddStack(nil)
	}

	if w.main.isKeepNode(w.nodeName) {
		w.keepalive = true
	}

	return nil
}

func (w *nodeWatcher) stop() *atomos.Error {
	p, err := w.getProcess()
	if err != nil {
		return err.AddStack(nil)
	}
	if p == nil {
		return nil
	}
	run, er := p.IsRunning()
	if er != nil {
		return atomos.NewErrorf(atomos.ErrCosmosNodeGetProcessFailed, "Supervisor: Get node process running info failed. err=(%v)", er).AddStack(nil)
	}
	if !run {
		return atomos.NewError(atomos.ErrCosmosNodeIsAlreadyStopped, "Supervisor: Node is already stopped.").AddStack(nil)
	}

	w.keepalive = false
	if er = p.SendSignal(syscall.SIGINT); er != nil {
		return atomos.NewErrorf(atomos.ErrCosmosNodeStopsError, "Supervisor: Node stops error. err=(%v)", er).AddStack(nil)
	}

	for {
		<-time.After(100 * time.Millisecond)
		run, err = w.isRunning()
		if err != nil {
			return err.AddStack(nil)
		}
		if run {
			continue
		}
		atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Info, "Supervisor: Node stopped, will restart. name=(%s)", w.nodeName)
		break
	}

	return nil
}

func (w *nodeWatcher) restart() *atomos.Error {
	if err := w.stop(); err != nil {
		return err.AddStack(nil)
	}
	if err := w.start(); err != nil {
		return err.AddStack(nil)
	}
	return nil
}

func (w *nodeWatcher) status() {
}

func (w *nodeWatcher) sendCommand() {
}

func (w *nodeWatcher) onPIDFileChanged() {
	// TODO
}

func (w *nodeWatcher) onPIDProcessCheck() {
	run, err := w.isRunning()
	if err != nil {
		atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Fatal, "Supervisor: Check PID status failed. name=(%s),err=(%v)", w.nodeName, err.AddStack(nil))
		return
	}
	if run {
		// TODO: Send Ping to process to check.
		return
	}
	if w.main.isKeepNode(w.nodeName) && w.keepalive {
		if err := w.start(); err != nil {
			atomos.SharedLogging().PushProcessLog(atomos.LogLevel_Info, "Supervisor: Keepalive error. name=(%s),err=(%v)", w.nodeName, err.AddStack(nil))
		}
	}
}

func udsPing(bytes []byte) ([]byte, *atomos.Error) {
	return []byte("pong"), nil
}

func udsStatus(bytes []byte) ([]byte, *atomos.Error) {
	p := atomos.SharedCosmosProcess()
	if p == nil {
		return nil, atomos.NewError(atomos.ErrCosmosIsClosed, "UDS: Node is not running").AddStack(nil)
	}
	return nil, nil
}

func udsStart(bytes []byte) ([]byte, *atomos.Error) {
	return []byte("start"), nil
}

func udsStop(bytes []byte) ([]byte, *atomos.Error) {
	return []byte("stop"), nil
}

func udsRestart(bytes []byte) ([]byte, *atomos.Error) {
	return []byte("restart"), nil
}

//type command struct {
//	configPath string
//	daemon     *atomos.App
//}
//
//func (c *command) Command(cmd string) (string, bool, *atomos.Error) {
//	if len(cmd) > 0 {
//		i := strings.Index(cmd, " ")
//		if i == -1 {
//			i = len(cmd)
//		}
//		switch cmd[:i] {
//		case "status":
//			return c.handleCommandStatus()
//		case "start all":
//			return c.handleCommandStartAll()
//		case "stop all":
//			return c.handleCommandStopAll()
//		case "exit":
//			return "closing", true, nil
//		}
//	}
//	return fmt.Sprintf("invalid command \"%s\"\n", cmd), false, nil
//}
//
//func (c *command) Greeting() string {
//	return "Hello Atomos!\n"
//}
//
//func (c *command) PrintNow() string {
//	return "Now:\t" + time.Now().Format("2006-01-02 15:04:05 MST -07:00")
//}
//
//func (c *command) Bye() string {
//	return "Bye Atomos!\n"
//}
//
//func (c *command) handleCommandStatus() (string, bool, *atomos.Error) {
//	buf := strings.Builder{}
//	for _, process := range nodeList {
//		buf.WriteString(fmt.Sprintf("Node: %s, status=(%s)", process.config.Node, process.getStatus()))
//	}
//	return buf.String(), false, nil
//}
//
//func (c *command) handleCommandStartAll() (string, bool, *atomos.Error) {
//	return "", false, nil
//}
//
//func (c *command) handleCommandStopAll() (string, bool, *atomos.Error) {
//	return "stop all", false, nil
//}
//
//func (c *command) getNodeListConfig() ([]*nodeProcess, *atomos.Error) {
//	//log.Printf("Daemon process load config failed, config=(%s),err=(%v)", *configPath, err)
//	var nodeListConfig []*nodeProcess
//	// Load Config.
//	conf, err := atomos.NewSupervisorConfigFromYaml(c.configPath)
//	if err != nil {
//		return nodeListConfig, err.AddStack(nil)
//	}
//	if err = conf.Check(); err != nil {
//		return nodeListConfig, err.AddStack(nil)
//	}
//	for _, node := range conf.NodeList {
//		np := &nodeProcess{}
//		nodeListConfig = append(nodeListConfig, np)
//
//		np.confPath = conf.LogPath + "/" + node + ".conf"
//		np.config, np.err = atomos.NewCosmosNodeConfigFromYamlPath(np.confPath)
//		if err != nil {
//			np.err = np.err.AddStack(nil)
//			continue
//		}
//		if err = conf.Check(); err != nil {
//			np.err = np.err.AddStack(nil)
//			continue
//		}
//	}
//	return nodeListConfig, nil
//}
//
//// Node Process
//
//type nodeProcess struct {
//	confPath string
//	config   *atomos.Config
//	err      *atomos.Error
//}
//
//func (p nodeProcess) getStatus() string {
//	return ""
//	//attr := &os.ProcAttr{
//	//	Dir:   c.process.workPath,
//	//	Env:   c.process.env,
//	//	Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
//	//	Sys:   &syscall.SysProcAttr{Setsid: true},
//	//}
//	//proc, er = os.StartProcess(c.process.executablePath, c.process.args, attr)
//	//if er != nil {
//	//	return false, pid, NewError(ErrAppLaunchedFailed, er.Error()).AddStack(nil)
//	//}
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
