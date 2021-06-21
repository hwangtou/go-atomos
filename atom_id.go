package go_atomos

// Id

type Id interface {
	Cosmos() CosmosNode
	Element() Element
	Name() string
	Kill(from Id) error
	getLocalAtom() *AtomCore
}

func NewAtomId(c CosmosNode, elemName, atomName string) (Id, error) {
	if c.IsLocal() {
		l := c.(*CosmosLocal)
		element, err := l.getElement(elemName)
		if err != nil {
			return nil, err
		}
		return element.getAtomId(atomName)
	}
	panic("")
}

type IdLocal struct {
	atom *AtomCore
}

func (c *IdLocal) Cosmos() CosmosNode {
	return c.atom.element.cosmos.local
}

func (c *IdLocal) Element() Element {
	return c.atom.element
}

func (c *IdLocal) getLocalAtom() *AtomCore {
	return c.atom
}

func (c *IdLocal) Name() string {
	return c.atom.Name()
}

func (c *IdLocal) Core() *AtomCore {
	return c.atom
}

func (c *IdLocal) Kill(from Id) error {
	return c.atom.Kill(from)
}

type idMain struct {
}

func (i idMain) Cosmos() CosmosNode {
	panic("implement me")
}

func (i idMain) Element() Element {
	panic("implement me")
}

func (i idMain) Name() string {
	panic("implement me")
}

func (i idMain) Kill(from Id) error {
	panic("implement me")
}

func (i idMain) getLocalAtom() *AtomCore {
	panic("implement me")
}

