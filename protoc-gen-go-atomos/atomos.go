package main

import (
	"errors"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/types/descriptorpb"
)

const (
	atomosPackage   = protogen.GoImportPath("github.com/hwangtou/go-atomos")
	protobufPackage = protogen.GoImportPath("google.golang.org/protobuf/proto")
)

// generateFile generates a _grpc.pb.go file containing gRPC service definitions.
func generateFile(gen *protogen.Plugin, file *protogen.File) *protogen.GeneratedFile {
	if len(file.Services) == 0 {
		return nil
	}
	filename := file.GeneratedFilenamePrefix + "_atomos.pb.go"
	g := gen.NewGeneratedFile(filename, file.GoImportPath)
	g.P("// Code generated by protoc-gen-go-atomos. DO NOT EDIT.")
	g.P()
	g.P("package ", file.GoPackageName)
	g.P()
	generateFileContent(gen, file, g)
	return g
}

// generateFileContent generates the atom definitions, excluding the package statement.
func generateFileContent(gen *protogen.Plugin, file *protogen.File, g *protogen.GeneratedFile) {
	if len(file.Services) == 0 {
		return
	}
	g.P()
	for _, service := range file.Services {
		elementName := service.GoName
		if elementName == "Main" {
			gen.Error(errors.New("cannot use element name \"Main\""))
			continue
		}

		g.P()
		g.P("const ", elementName, "Name = \"", elementName, "\"")
		g.P()

		g.P("////////////////////////////////////")
		g.P("/////////// 需要实现的接口 ///////////")
		g.P("////// Interface to implement //////")
		g.P("////////////////////////////////////")
		g.P()

		if len(service.Comments.Leading.String()) > 0 {
			g.P("//")
			g.P(service.Comments.Leading.String(), "//")
			g.P("")
		}

		genElementInterface(g, service)
		genAtomInterface(g, service)

		g.P("////////////////////////////////////")
		g.P("/////////////// 识别符 //////////////")
		g.P("//////////////// ID ////////////////")
		g.P("////////////////////////////////////")

		// Interface
		genElementIDInternal(g, service)
		genAtomIDInternal(g, service)

		// Internal
		g.P("// Atomos Interface")
		g.P()
		genImplement(file, g, service)
	}
	g.P()
}

func genElementInterface(g *protogen.GeneratedFile, service *protogen.Service) {
	elementName := service.GoName + "Element"

	// Server struct.
	g.P("// ", elementName, " is the atomos implements of ", service.GoName, " element.")
	g.P()
	if service.Desc.Options().(*descriptorpb.ServiceOptions).GetDeprecated() {
		g.P("//")
		g.P(deprecationComment)
	}
	g.Annotate(elementName, service.Location)
	g.P("type ", elementName, " interface {")
	g.P(atomosPackage.Ident("Atomos"))
	hasElementSpawn := false
	// Element
	for _, method := range service.Methods {
		methodName := method.GoName
		if !strings.HasPrefix(methodName, "Element") {
			continue
		}
		methodName = strings.TrimPrefix(methodName, "Element")

		// Comment
		g.Annotate(elementName+"."+methodName, method.Location)
		if method.Desc.Options().(*descriptorpb.MethodOptions).GetDeprecated() {
			g.P(deprecationComment)
		}
		commentLen := len(method.Comments.Leading.String())
		if commentLen > 0 {
			g.P(method.Comments.Leading.String()[:commentLen-1])
		}

		// Spawn & Methods
		if methodName == "Spawn" {
			hasElementSpawn = true
			spawnElementSign(g, method)
			g.P()
		} else {
			methodSign(g, method, methodName)
		}
	}

	// Scale
	hasScale := false
	for _, method := range service.Methods {
		methodName := method.GoName
		if !strings.HasPrefix(methodName, "Scale") {
			continue
		}
		if !hasScale {
			g.P()
			g.P("// Scale Methods")
			g.P()
			hasScale = true
		}
		g.Annotate(elementName+"."+methodName, method.Location)
		if method.Desc.Options().(*descriptorpb.MethodOptions).GetDeprecated() {
			g.P(deprecationComment)
		}
		commentLen := len(method.Comments.Leading.String())
		if commentLen > 0 {
			g.P(method.Comments.Leading.String()[:commentLen-1])
		}
		elementScaleSign(g, method, service.GoName, methodName)
	}
	if !hasElementSpawn {
		spawnElementDefaultSign(g)
	}
	g.P("}")
	g.P()
}

func genAtomInterface(g *protogen.GeneratedFile, service *protogen.Service) {
	atomName := service.GoName + "Atom"

	// Server struct.
	g.P("// ", atomName, " is the atomos implements of ", service.GoName, " atom.")
	g.P()
	if service.Desc.Options().(*descriptorpb.ServiceOptions).GetDeprecated() {
		g.P("//")
		g.P(deprecationComment)
	}
	g.Annotate(atomName, service.Location)
	g.P("type ", atomName, " interface {")
	g.P(atomosPackage.Ident("Atomos"))
	g.P()
	// Atom
	for _, method := range service.Methods {
		methodName := method.GoName
		if strings.HasPrefix(methodName, "Element") {
			continue
		}
		if strings.HasPrefix(methodName, "Scale") {
			continue
		}
		g.Annotate(atomName+"."+methodName, method.Location)
		if method.Desc.Options().(*descriptorpb.MethodOptions).GetDeprecated() {
			g.P(deprecationComment)
		}
		commentLen := len(method.Comments.Leading.String())
		if commentLen > 0 {
			g.P(method.Comments.Leading.String()[:commentLen-1])
		}
		if methodName == "Spawn" {
			spawnAtomSign(g, method)
			g.P()
		} else {
			methodSign(g, method, methodName)
		}
	}

	g.P()
	g.P("// Scale Methods")
	g.P()
	// Scale
	for _, method := range service.Methods {
		methodName := method.GoName
		if !strings.HasPrefix(methodName, "Scale") {
			continue
		}
		//methodName = strings.TrimPrefix(methodName, "Scale")
		g.Annotate(atomName+"."+methodName, method.Location)
		if method.Desc.Options().(*descriptorpb.MethodOptions).GetDeprecated() {
			g.P(deprecationComment)
		}
		commentLen := len(method.Comments.Leading.String())
		if commentLen > 0 {
			g.P(method.Comments.Leading.String()[:commentLen-1])
		}
		methodSign(g, method, methodName)
	}
	g.P("}")
	g.P()
}

func genElementIDInternal(g *protogen.GeneratedFile, service *protogen.Service) {
	idName := service.GoName + "ElementID"
	elementIDName := service.GoName + "ElementID"
	elementValueName := noExport(service.GoName) + "ElementValue"
	elementMessengerValueName := noExport(service.GoName) + "ElementMessengerValue"

	atomIDName := service.GoName + "AtomID"
	atomValueName := noExport(service.GoName) + "AtomValue"
	atomMessengerValueName := noExport(service.GoName) + "AtomMessengerValue"

	g.P()
	g.P("// Element: " + service.GoName)
	g.P()

	// ID structure.
	g.P("type ", idName, " struct {")
	g.P(atomosPackage.Ident("ID"))
	g.P("*", atomosPackage.Ident("IDTracker"))
	g.P("}")
	g.P()

	g.P("// 获取某节点中的ElementID")
	g.P("// Get element id of node")
	// NewClient factory.
	if service.Desc.Options().(*descriptorpb.ServiceOptions).GetDeprecated() {
		g.P(deprecationComment)
	}
	g.P("func Get", elementIDName, " (c ", atomosPackage.Ident("CosmosNode"), ") (*", elementIDName, ", *", atomosPackage.Ident("Error"), ") {")
	g.P("ca, err := c.CosmosGetElementID(", service.GoName, "Name)")
	g.P("if err != nil { return nil, err }")
	g.P("return &", elementIDName, "{ca, nil, ", "}, nil")
	g.P("}")
	g.P()

	// Client method implementations.
	for _, method := range service.Methods {
		methodName := method.GoName
		if !strings.HasPrefix(methodName, "Element") {
			continue
		}
		if strings.HasPrefix(methodName, "ElementSpawn") {
			continue
		}
		if strings.HasPrefix(methodName, "Spawn") {
			continue
		}
		methodName = strings.TrimPrefix(methodName, "Element")
		if methodName == "" {
			continue
		}

		// Comment
		g.P()
		commentLen := len(method.Comments.Leading.String())
		if commentLen > 0 {
			g.P(method.Comments.Leading.String()[:commentLen-1])
		}

		// Sync
		g.P("// Sync")
		g.P("func (c *", idName, ") ", methodName+"(callerID ", atomosPackage.Ident("SelfID"),
			", in *", g.QualifiedGoIdent(method.Input.GoIdent),
			", ext ...interface{}) (out *", g.QualifiedGoIdent(method.Output.GoIdent),
			", err *", atomosPackage.Ident("Error"), ")", " {")
		g.P("/* CODE JUMPER 代码跳转 */ _ = func() { _ = " + elementValueName + "." + methodName + " }")
		g.P("return " + elementMessengerValueName + "." + methodName + "().SyncElement(c, callerID, in, ext...)")
		g.P("}")

		// Async
		g.P("// Async")
		g.P("func (c *", idName, ") Async", methodName+"(callerID ", atomosPackage.Ident("SelfID"),
			", in *", g.QualifiedGoIdent(method.Input.GoIdent),
			", callback func(out *", g.QualifiedGoIdent(method.Output.GoIdent), ", err *", atomosPackage.Ident("Error"), ")",
			", ext ...interface{}) {")
		g.P("/* CODE JUMPER 代码跳转 */ _ = func() { _ = " + elementValueName + "." + methodName + " }")
		g.P(elementMessengerValueName + "." + methodName + "().AsyncElement(c, callerID, in, callback, ext...)")
		g.P("}")
		g.P()
	}

	// Scale method implementations.
	for _, method := range service.Methods {
		methodName := method.GoName
		if !strings.HasPrefix(methodName, "Scale") {
			continue
		}
		scaleName := strings.TrimPrefix(methodName, "Scale")

		// Scale
		g.P("// GetID")
		g.P("func (c *", idName, ") ", methodName+"GetID(callerID ", atomosPackage.Ident("SelfID"),
			", in *", g.QualifiedGoIdent(method.Input.GoIdent),
			", ext ...interface{}) (id *", atomIDName,
			", err *", atomosPackage.Ident("Error"), ")", " {")
		g.P("/* CODE JUMPER 代码跳转 */ _ = func() { _ = " + elementValueName + "." + methodName + " }")
		g.P("i, tracker, err := ", elementMessengerValueName, ".", methodName, "().GetScaleID(c, callerID, ", service.GoName, "Name, in, ext...)")
		g.P("if err != nil {")
		g.P("return nil, err.AddStack(nil)")
		g.P("}")
		g.P("return &", service.GoName, "AtomID{i, tracker}, nil")
		g.P("}")

		// Sync
		g.P("// Sync")
		g.P("func (c *", idName, ") ", methodName+"(callerID ", atomosPackage.Ident("SelfID"),
			", in *", g.QualifiedGoIdent(method.Input.GoIdent),
			", ext ...interface{}) (out *", g.QualifiedGoIdent(method.Output.GoIdent),
			", err *", atomosPackage.Ident("Error"), ")", " {")
		g.P("/* CODE JUMPER 代码跳转 */ _ = func() { _ = " + elementValueName + "." + methodName + " }")
		g.P("/* CODE JUMPER 代码跳转 */ _ = func() { _ = " + atomValueName + "." + methodName + " }")
		g.P("id, err := c.", methodName, "GetID(callerID, in, ext...)")
		g.P("if err != nil {")
		g.P("return nil, err.AddStack(nil)")
		g.P("}")
		g.P("defer id.Release()")
		g.P("return ", atomMessengerValueName+"."+methodName+"().SyncAtom(id, callerID, in, ext...)")
		g.P("}")

		// Async
		g.P("// Async")
		g.P("func (c *", idName, ") ScaleAsync", scaleName+"(callerID ", atomosPackage.Ident("SelfID"),
			", in *", g.QualifiedGoIdent(method.Input.GoIdent),
			", callback func(*", g.QualifiedGoIdent(method.Output.GoIdent), ", *", atomosPackage.Ident("Error"), ")",
			", ext ...interface{}) {")
		g.P("/* CODE JUMPER 代码跳转 */ _ = func() { _ = " + elementValueName + "." + methodName + " }")
		g.P("/* CODE JUMPER 代码跳转 */ _ = func() { _ = " + atomValueName + "." + methodName + " }")
		g.P("id, err := c.", methodName, "GetID(callerID, in, ext...)")
		g.P("if err != nil {")
		g.P("callback(nil, err.AddStack(nil))")
		g.P("return")
		g.P("}")
		g.P("defer id.Release()")
		g.P(atomMessengerValueName + "." + methodName + "().AsyncAtom(id, callerID, in, callback, ext...)")
		g.P("}")
	}
}

func genAtomIDInternal(g *protogen.GeneratedFile, service *protogen.Service) {
	atomName := service.GoName + "AtomID"
	atomValueName := noExport(service.GoName) + "AtomValue"
	atomMessengerValueName := noExport(service.GoName) + "AtomMessengerValue"

	g.P("// Atom: ", service.GoName)
	g.P()

	// ID structure.
	g.P("type ", atomName, " struct {")
	g.P(atomosPackage.Ident("ID"))
	g.P("*", atomosPackage.Ident("IDTracker"))
	g.P("}")
	g.P()

	g.P("// 创建（自旋）某节点中的一个Atom，并返回AtomID")
	g.P("// Create (spin) an atom in a node and return the AtomID")

	// Spawn
	for _, method := range service.Methods {
		methodName := method.GoName
		if methodName == "Spawn" {
			elementName := service.GoName
			spawnArgTypeName := g.QualifiedGoIdent(method.Input.GoIdent)
			g.P("func Spawn", service.GoName, "Atom(caller ", atomosPackage.Ident("SelfID"), ", c ", atomosPackage.Ident("CosmosNode"),
				", name string, arg *", spawnArgTypeName, ") (*",
				atomName, ", *", atomosPackage.Ident("Error"), ") {")
			g.P("id, tracker, err := c.CosmosSpawnAtom(caller,", elementName, "Name, name, arg)")
			g.P("if id == nil { return nil, err.AddStack(nil) }")
			g.P("return &", atomName, "{id, tracker, ", "}, err")
			g.P("}")
			break
		}
	}

	g.P("// 获取某节点中的AtomID")
	g.P("// Get atom id of node")
	// NewClient factory.
	if service.Desc.Options().(*descriptorpb.ServiceOptions).GetDeprecated() {
		g.P(deprecationComment)
	}
	g.P("func Get", atomName, " (c ", atomosPackage.Ident("CosmosNode"), ", name string) (*", atomName, ", *", atomosPackage.Ident("Error"), ") {")
	g.P("ca, tracker, err := c.CosmosGetAtomID(", service.GoName, "Name, name)")
	g.P("if err != nil { return nil, err }")
	g.P("return &", atomName, "{ca, tracker, ", "}, nil")
	g.P("}")
	g.P()

	// Client method implementations.
	for _, method := range service.Methods {
		methodName := method.GoName
		if strings.HasPrefix(methodName, "Element") {
			continue
		}
		if strings.HasPrefix(methodName, "Spawn") {
			continue
		}
		if methodName == "" {
			continue
		}

		// Sync
		g.P("// Sync")
		g.P("func (c *", atomName, ") ", methodName+"(callerID ", atomosPackage.Ident("SelfID"),
			", in *", g.QualifiedGoIdent(method.Input.GoIdent),
			", ext ...interface{}) (out *", g.QualifiedGoIdent(method.Output.GoIdent),
			", err *", atomosPackage.Ident("Error"), ")", " {")
		g.P("/* CODE JUMPER 代码跳转 */ _ = func() { _ = " + atomValueName + "." + methodName + " }")
		g.P("return " + atomMessengerValueName + "." + methodName + "().SyncAtom(c, callerID, in, ext...)")
		g.P("}")

		// Async
		g.P("// Async")
		g.P("func (c *", atomName, ") Async", methodName+"(callerID ", atomosPackage.Ident("SelfID"),
			", in *", g.QualifiedGoIdent(method.Input.GoIdent),
			", callback func(out *", g.QualifiedGoIdent(method.Output.GoIdent), ", err *", atomosPackage.Ident("Error"), ")",
			", ext ...interface{}) {")
		g.P("/* CODE JUMPER 代码跳转 */ _ = func() { _ = " + atomValueName + "." + methodName + " }")
		g.P(atomMessengerValueName + "." + methodName + "().AsyncAtom(c, callerID, in, callback, ext...)")
		g.P("}")
		g.P()
	}
}

func genImplement(file *protogen.File, g *protogen.GeneratedFile, service *protogen.Service) {
	elementName := service.GoName
	elementAtomName := elementName + "Atom"
	elementElementName := elementName + "Element"
	interfaceName := service.GoName + "Interface"

	elementIDName := service.GoName + "ElementID"
	elementValueName := noExport(service.GoName) + "ElementValue"
	elementMessengerName := noExport(service.GoName) + "ElementMessenger"
	elementMessengerValueName := noExport(service.GoName) + "ElementMessengerValue"

	atomIDName := service.GoName + "AtomID"
	atomValueName := noExport(service.GoName) + "AtomValue"
	atomMessengerName := noExport(service.GoName) + "AtomMessenger"
	atomMessengerValueName := noExport(service.GoName) + "AtomMessengerValue"

	g.P("func Get", elementName, "Implement(dev ", atomosPackage.Ident("ElementDeveloper"), ") *", atomosPackage.Ident("ElementImplementation"), "{")
	g.P("elem := ", atomosPackage.Ident("NewImplementationFromDeveloper"), "(dev)")
	g.P("elem.Interface = Get", interfaceName, "(dev)")
	g.P("elem.ElementHandlers = map[string]", atomosPackage.Ident("MessageHandler"), "{")

	for _, method := range service.Methods {
		methodName := method.GoName
		if !strings.HasPrefix(methodName, "Element") || methodName == "Element" {
			continue
		}
		if strings.HasPrefix(methodName, "ElementSpawn") {
			continue
		}
		if strings.HasPrefix(methodName, "Spawn") {
			continue
		}
		elemName := strings.TrimPrefix(methodName, "Element")
		g.P("\"", elemName, "\": func(from ", atomosPackage.Ident("ID"), ", to ", atomosPackage.Ident("Atomos"), ", in ", protobufPackage.Ident("Message"), ") (", protobufPackage.Ident("Message"), ", *", atomosPackage.Ident("Error"), ") {")
		g.P("a, i, err := ", elementMessengerValueName, ".", elemName, "().ExecuteAtom(to, in)")
		g.P("if err != nil { return nil, err.AddStack(nil) }")
		g.P("return a.", elemName, "(from, i)")
		g.P("},")
	}
	g.P("}")

	g.P("elem.AtomHandlers = map[string]", atomosPackage.Ident("MessageHandler"), "{")
	for _, method := range service.Methods {
		methodName := method.GoName
		if strings.HasPrefix(methodName, "Element") {
			continue
		}
		if strings.HasPrefix(methodName, "Spawn") {
			continue
		}
		if methodName == "" {
			continue
		}
		g.P("\"", methodName, "\": func(from ", atomosPackage.Ident("ID"), ", to ", atomosPackage.Ident("Atomos"), ", in ", protobufPackage.Ident("Message"), ") (", protobufPackage.Ident("Message"), ", *", atomosPackage.Ident("Error"), ") {")
		g.P("a, i, err := ", atomMessengerValueName, ".", methodName, "().ExecuteAtom(to, in)")
		g.P("if err != nil { return nil, err.AddStack(nil) }")
		g.P("return a.", methodName, "(from, i)")
		g.P("},")
	}
	g.P("}")

	g.P("elem.ScaleHandlers = map[string]", atomosPackage.Ident("ScaleHandler"), "{")
	for _, method := range service.Methods {
		methodName := method.GoName
		if !strings.HasPrefix(methodName, "Scale") || methodName == "Scale" {
			continue
		}
		g.P("\"", methodName, "\": func(from ", atomosPackage.Ident("ID"), ", e ", atomosPackage.Ident("Atomos"), ", message string, in ", protobufPackage.Ident("Message"), ") (id ", atomosPackage.Ident("ID"), ", err *", atomosPackage.Ident("Error"), ") {")
		g.P("a, i, err := ", elementMessengerValueName, ".", methodName, "().ExecuteScale(e, in)")
		g.P("if err != nil { return nil, err.AddStack(nil) }")
		g.P("return a.", methodName, "(from, i)")
		g.P("},")
	}
	g.P("}")

	g.P("return elem")
	g.P("}")

	g.P("func Get", elementName, "Interface(dev ", atomosPackage.Ident("ElementDeveloper"), ") *", atomosPackage.Ident("ElementInterface"), "{")
	g.P("elem := ", atomosPackage.Ident("NewInterfaceFromDeveloper("), elementName, "Name, dev)")

	hasElementSpawn := false
	for _, method := range service.Methods {
		if method.GoName != "ElementSpawn" {
			continue
		}
		hasElementSpawn = true
		g.P("elem.ElementSpawner = func(s ", atomosPackage.Ident("ElementSelfID"), ", a ", atomosPackage.Ident("Atomos"), ", data ", protobufPackage.Ident("Message"), ") *", atomosPackage.Ident("Error"), " {")
		g.P("dataT, _ := data.(*", method.Output.GoIdent, ")")
		g.P("elem, ok := a.(", elementElementName, ")")
		g.P("if !ok { return ", atomosPackage.Ident("NewErrorf"), "(", atomosPackage.Ident("ErrElementNotImplemented"), ", \"Element not implemented, type=(", elementElementName, ")\") }")
		g.P("return elem.Spawn(s, dataT)")
		g.P("}")
	}
	if !hasElementSpawn {
		g.P("elem.ElementSpawner = func(s ", atomosPackage.Ident("ElementSelfID"), ", a ", atomosPackage.Ident("Atomos"), ", data ", protobufPackage.Ident("Message"), ") *", atomosPackage.Ident("Error"), " {")
		//g.P("dataT, _ := data.(*", method.Output.GoIdent, ")")
		g.P("elem, ok := a.(", elementElementName, ")")
		g.P("if !ok { return ", atomosPackage.Ident("NewErrorf"), "(", atomosPackage.Ident("ErrElementNotImplemented"), ", \"Element not implemented, type=(", elementElementName, ")\") }")
		g.P("return elem.Spawn(s, nil)")
		g.P("}")
	}

	for _, method := range service.Methods {
		if method.GoName != "Spawn" {
			continue
		}
		g.P("elem.AtomSpawner = func(s ", atomosPackage.Ident("AtomSelfID"), ", a ", atomosPackage.Ident("Atomos"), ", arg, data ", protobufPackage.Ident("Message"), ") *", atomosPackage.Ident("Error"), " {")
		g.P("argT, _ := arg.(*", method.Input.GoIdent, "); dataT, _ := data.(*", method.Output.GoIdent, ")")
		g.P("atom, ok := a.(", elementAtomName, ")")
		g.P("if !ok { return ", atomosPackage.Ident("NewErrorf"), "(", atomosPackage.Ident("ErrAtomNotImplemented"), ", \"Atom not implemented, type=(", elementAtomName, ")\") }")
		g.P("return atom.Spawn(s, argT, dataT)")
		g.P("}")
	}

	g.P("elem.ElementDecoders = map[string]*", atomosPackage.Ident("IOMessageDecoder"), "{")
	for _, method := range service.Methods {
		methodName := method.GoName
		if !strings.HasPrefix(methodName, "Element") && !strings.HasPrefix(methodName, "Scale") {
			continue
		}
		if strings.HasPrefix(methodName, "ElementSpawn") {
			continue
		}
		if strings.HasPrefix(methodName, "Spawn") {
			continue
		}
		elemName := strings.TrimPrefix(methodName, "Element")
		if elemName == "" {
			continue
		}
		g.P("\"", elemName, "\": ", elementMessengerValueName, ".", elemName, "().Decoder(&", method.Input.GoIdent, "{}, &", method.Output.GoIdent, "{}),")
	}
	g.P("}")

	g.P("elem.AtomDecoders = map[string]*", atomosPackage.Ident("IOMessageDecoder"), "{")
	for _, method := range service.Methods {
		methodName := method.GoName
		if strings.HasPrefix(methodName, "Element") {
			continue
		}
		if strings.HasPrefix(methodName, "Spawn") {
			continue
		}
		if methodName == "Scale" {
			continue
		}
		g.P("\"", methodName, "\": ", atomMessengerValueName, ".", methodName, "().Decoder(&", method.Input.GoIdent, "{}, &", method.Output.GoIdent, "{}),")
	}
	g.P("}")

	g.P("return elem")
	g.P("}")

	g.P()
	g.P("// Atomos Internal")
	g.P()
	g.P("// Element Define")
	g.P()

	g.P("type ", elementMessengerName, " struct {}")
	for _, method := range service.Methods {
		methodName := method.GoName
		if !strings.HasPrefix(methodName, "Element") && !strings.HasPrefix(methodName, "Scale") {
			continue
		}
		if strings.HasPrefix(methodName, "ElementSpawn") {
			continue
		}
		if strings.HasPrefix(methodName, "Spawn") {
			continue
		}
		elemFnName := strings.TrimPrefix(methodName, "Element")
		if elemFnName == "" {
			continue
		}
		g.P("func (m ", elementMessengerName, ") ", elemFnName, "() ", atomosPackage.Ident("Messenger"), "[*", elementIDName, ", *", atomIDName, ", ", elementElementName, ", *", method.Input.GoIdent, ", *", method.Output.GoIdent, "]{")
		g.P("return ", atomosPackage.Ident("Messenger"), "[*", elementIDName, ", *", atomIDName, ", ", elementElementName, ", *", method.Input.GoIdent, ", *", method.Output.GoIdent, "]{nil, nil, \"", elemFnName, "\"}")
		g.P("}")
	}
	g.P("var ", elementMessengerValueName, " ", elementMessengerName)
	g.P("var ", elementValueName, " ", elementElementName)
	g.P()

	g.P("// Atom Define")
	g.P()
	g.P("type ", atomMessengerName, " struct {}")
	for _, method := range service.Methods {
		methodName := method.GoName
		if strings.HasPrefix(methodName, "Element") {
			continue
		}
		if strings.HasPrefix(methodName, "Spawn") {
			continue
		}
		atomFnName := strings.TrimPrefix(methodName, "Element")
		if atomFnName == "" {
			continue
		}
		g.P("func (m ", atomMessengerName, ") ", atomFnName, "() ", atomosPackage.Ident("Messenger"), "[*", elementIDName, ", *", atomIDName, ", ", elementAtomName, ", *", method.Input.GoIdent, ", *", method.Output.GoIdent, "]{")
		g.P("return ", atomosPackage.Ident("Messenger"), "[*", elementIDName, ", *", atomIDName, ", ", elementAtomName, ", *", method.Input.GoIdent, ", *", method.Output.GoIdent, "]{nil, nil, \"", atomFnName, "\"}")
		g.P("}")
	}
	g.P("var ", atomMessengerValueName, " ", atomMessengerName)
	g.P("var ", atomValueName, " ", elementAtomName)
	g.P()
}

const deprecationComment = "// Deprecated: Do not use."

func spawnElementSign(g *protogen.GeneratedFile, method *protogen.Method) {
	g.P("Spawn(self ", atomosPackage.Ident("ElementSelfID"),
		", data *", g.QualifiedGoIdent(method.Output.GoIdent),
		") *", atomosPackage.Ident("Error"))
}

func spawnElementDefaultSign(g *protogen.GeneratedFile) {
	g.P("Spawn(self ", atomosPackage.Ident("ElementSelfID"),
		", data *", atomosPackage.Ident("Nil"),
		") *", atomosPackage.Ident("Error"))
}

func spawnAtomSign(g *protogen.GeneratedFile, method *protogen.Method) {
	g.P("Spawn(self ", atomosPackage.Ident("AtomSelfID"),
		", arg *", g.QualifiedGoIdent(method.Input.GoIdent),
		", data *", g.QualifiedGoIdent(method.Output.GoIdent),
		") *", atomosPackage.Ident("Error"))
}

func methodSign(g *protogen.GeneratedFile, method *protogen.Method, methodName string) {
	g.P(methodName+"(from ", atomosPackage.Ident("ID"),
		", in *", g.QualifiedGoIdent(method.Input.GoIdent),
		") (out *", g.QualifiedGoIdent(method.Output.GoIdent),
		", err *", atomosPackage.Ident("Error"),
		")")
}

func elementScaleSign(g *protogen.GeneratedFile, method *protogen.Method, elemName, methodName string) {
	g.P(methodName+"(from ", atomosPackage.Ident("ID"),
		", in *", g.QualifiedGoIdent(method.Input.GoIdent),
		") (*", elemName+"AtomID, *", atomosPackage.Ident("Error"),
		")")
}

func noExport(s string) string {
	return strings.ToLower(s[:1]) + s[1:]
}
