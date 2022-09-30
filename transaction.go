package go_atomos

import "time"

type Transaction interface {
	TransactionInfo() (name string, fromID ID, begin time.Time)
	Set(fromID ID, name string) *ErrorInfo
	SetWithTimeout(fromID ID, name string, ttl time.Duration) *ErrorInfo
	Commit(fromID ID, name string) *ErrorInfo
	Rollback(fromID ID, name string) *ErrorInfo
}

type atomosTransactionManager struct {
	atomos *BaseAtomos

	name      string
	from      ID
	backup    Atomos
	beginAt   time.Time
	ttlCancel *time.Timer
}

func initAtomosTransactionManager(at *atomosTransactionManager, a *BaseAtomos) {
	at.atomos = a
	at.name = ""
	at.from = nil
	at.backup = nil
	at.beginAt = time.Time{}
}

func releaseAtomosTransaction(at *atomosTransactionManager) {
	at.atomos = nil
	at.name = ""
	at.from = nil
	at.backup = nil
	at.beginAt = time.Time{}
}

func (t atomosTransactionManager) TransactionInfo() (name string, fromID ID, begin time.Time) {
	return t.name, t.from, t.beginAt
}

func (t *atomosTransactionManager) Set(fromID ID, name string) *ErrorInfo {
	if name == "" {
		return NewError(ErrTransactionNameInvalid, "invalid name").AutoStack(nil, nil)
	}
	return t.atomos.PushTransactionMailAndWaitReply(fromID, name, transactionMailSet, 0)
}

func (t *atomosTransactionManager) SetWithTimeout(fromID ID, name string, ttl time.Duration) *ErrorInfo {
	if name == "" {
		return NewError(ErrTransactionNameInvalid, "invalid name").AutoStack(nil, nil)
	}
	return t.atomos.PushTransactionMailAndWaitReply(fromID, name, transactionMailSet, ttl)
}

func (t *atomosTransactionManager) Commit(fromID ID, name string) *ErrorInfo {
	if t.name == "" {
		return NewErrorf(ErrTransactionNotExists, "no transaction").AutoStack(nil, nil)
	}
	return t.atomos.PushTransactionMailAndWaitReply(fromID, name, transactionMailCommit, 0)
}

func (t *atomosTransactionManager) Rollback(fromID ID, name string) *ErrorInfo {
	if t.name == "" {
		return NewErrorf(ErrTransactionNotExists, "no transaction").AutoStack(nil, nil)
	}
	return t.atomos.PushTransactionMailAndWaitReply(fromID, name, transactionMailRollback, 0)
}

// onReceive

func (t *atomosTransactionManager) onSet(fromID ID, name string, ttl time.Duration) *ErrorInfo {
	if t.name != "" {
		return NewErrorf(ErrTransactionIsBusy, "transaction is running, name=(%s)", t.name).AutoStack(nil, nil)
	}
	at, ok := t.atomos.instance.(AtomosTransaction)
	if !ok || at == nil {
		return NewErrorf(ErrTransactionNotSupported, "implement AtomosTransaction").AutoStack(nil, nil)
	}
	t.name = name
	t.from = fromID
	t.backup = at.Clone()
	t.beginAt = time.Now()
	t.ttlCancel = nil
	at.Begin(name)
	if ttl > 0 {
		t.ttlCancel = time.AfterFunc(ttl, func() {
			if err := t.atomos.PushTransactionMailAndWaitReply(fromID, name, transactionMailTimeout, 0); err != nil {
				// TODO
				return
			}
		})
	}
	return nil
}

func (t *atomosTransactionManager) onCommit(fromID ID, name string) *ErrorInfo {
	if t.name != name {
		return NewErrorf(ErrTransactionNotMatched, "different transaction").AutoStack(nil, nil)
	}
	at, ok := t.atomos.instance.(AtomosTransaction)
	if !ok || at == nil {
		return NewErrorf(ErrTransactionNotSupported, "implement AtomosTransaction").AutoStack(nil, nil)
	}
	if t.ttlCancel != nil {
		t.ttlCancel.Stop()
	}
	t.name = ""
	t.from = nil
	t.backup = nil
	t.beginAt = time.Time{}
	t.ttlCancel = nil
	at.End(true)
	return nil
}

func (t *atomosTransactionManager) onRollback(fromID ID, name string) *ErrorInfo {
	if t.name == "" {
		return NewErrorf(ErrTransactionNotExists, "no transaction").AutoStack(nil, nil)
	}
	if t.name != name {
		return NewErrorf(ErrTransactionNotMatched, "different transaction").AutoStack(nil, nil)
	}
	cloned, ok := t.backup.(AtomosTransaction)
	if !ok || cloned == nil {
		return NewErrorf(ErrTransactionNotSupported, "implement AtomosTransaction").AutoStack(nil, nil)
	}
	if t.ttlCancel != nil {
		t.ttlCancel.Stop()
	}
	t.atomos.instance = t.backup
	t.name = ""
	t.from = nil
	t.backup = nil
	t.beginAt = time.Time{}
	t.ttlCancel = nil
	cloned.End(false)
	return nil
}

func (t *atomosTransactionManager) onTimeout(name string) *ErrorInfo {
	if t.name == "" {
		return nil
	}
	if t.name != name {
		return nil
	}
	cloned, ok := t.backup.(AtomosTransaction)
	if !ok || cloned == nil {
		return NewErrorf(ErrTransactionNotSupported, "implement AtomosTransaction").AutoStack(nil, nil)
	}
	t.atomos.instance = t.backup
	t.name = ""
	t.from = nil
	t.backup = nil
	t.beginAt = time.Time{}
	t.ttlCancel = nil
	// TODO: Warning
	cloned.End(false)
	return nil
}
