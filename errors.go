package go_atomos

import "errors"

var (
	ErrElementNotFound       = errors.New("atomos: define not found")
	ErrElementNotLoaded      = errors.New("atomos: define not loaded")
	ErrFromNotFound          = errors.New("atomos: from not found")
	ErrAtomNotFound          = errors.New("atomos: AtomCore not found")
	ErrAtomExists            = errors.New("atomos: AtomCore exists")
	ErrAtomType              = errors.New("atomos: AtomCore type")
	ErrAtomMessageNotFound   = errors.New("atomos: AtomCore message not found")
	ErrAtomMessageAtomType   = errors.New("atomos: AtomCore message AtomCore type")
	ErrAtomMessageArgType    = errors.New("atomos: AtomCore message arg type")
	ErrAtomMessageReplyType  = errors.New("atomos: AtomCore message reply type")
	ErrAtomIsNotSpawning     = errors.New("atomos: AtomCore is not spawning")
	ErrAtomReplyType         = errors.New("atomos: AtomCore reply type")
	ErrAtomCannotKill        = errors.New("atomos: AtomCore cannot kill")
	ErrAtomAddTaskNotFunc    = errors.New("atomos: AtomCore add task not func")
	ErrAtomAddTaskIllegalArg = errors.New("atomos: AtomCore add task illegal arg")
	ErrAtomAddTaskIllegalMsg = errors.New("atomos: AtomCore add task illegal msg")
	ErrAtomTaskNotFound      = errors.New("atomos: AtomCore task not fond")
	ErrAtomTaskCannotDelete  = errors.New("atomos: AtomCore task cannot delete")
	ErrAtomReload            = errors.New("atomos: AtomCore reload")
)
