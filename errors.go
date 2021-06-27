package go_atomos

import "errors"

// TODO: No errors.go file in the future

var (
	ErrConfigIsNil           = errors.New("config not found")
	ErrConfigNodeInvalid     = errors.New("config node name is invalid")
	ErrConfigLogPathInvalid  = errors.New("config log path is invalid")
	ErrElementUpgradeVersion = errors.New("element upgrade version invalid")
	ErrElementNotLoaded      = errors.New("element not loaded")
	ErrFromNotFound          = errors.New("fromId not found")
	ErrAtomNotFound          = errors.New("AtomCore not found")
	ErrAtomCannotSpawn       = errors.New("atom cannot spawn")
	ErrAtomExists            = errors.New("AtomCore exists")
	ErrAtomType              = errors.New("AtomCore type")
	ErrAtomMessageNotFound   = errors.New("AtomCore message not found")
	ErrAtomMessageAtomType   = errors.New("AtomCore message AtomCore type")
	ErrAtomMessageArgType    = errors.New("AtomCore message arg type")
	ErrAtomMessageReplyType  = errors.New("AtomCore message reply type")
	ErrAtomIsNotSpawning     = errors.New("AtomCore is not spawning")
	ErrAtomNotReloadable     = errors.New("atom cannot reload")
	ErrAtomReplyType         = errors.New("AtomCore reply type")
	ErrAtomCannotKill        = errors.New("AtomCore cannot kill")
	ErrAtomAddTaskNotFunc    = errors.New("AtomCore add task not func")
	ErrAtomAddTaskIllegalArg = errors.New("AtomCore add task illegal arg")
	ErrAtomAddTaskIllegalMsg = errors.New("AtomCore add task illegal msg")
	ErrAtomTaskNotFound      = errors.New("AtomCore task not fond")
	ErrAtomTaskCannotDelete  = errors.New("AtomCore task cannot delete")
)
