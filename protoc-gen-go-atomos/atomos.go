package main

import (
	"errors"
	"fmt"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/types/descriptorpb"
)

const (
	atomosPackage   = protogen.GoImportPath("github.com/hwangtou/go-atomos")
	protobufPackage = protogen.GoImportPath("google.golang.org/protobuf/proto")
	timePackage     = protogen.GoImportPath("time")
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

	g.P("// This is a compile-time assertion to ensure that this generated file")
	g.P("// is compatible with the atomos package it is being compiled against.")
	g.P()
	g.P("//////")
	g.P("//// INTERFACES")
	g.P("//")
	g.P()
	g.P()
	for _, service := range file.Services {
		elementName := service.GoName
		if elementName == "Main" {
			gen.Error(errors.New("cannot use element name \"Main\""))
			continue
		}
		// ID
		if len(service.Comments.Leading.String()) > 0 {
			g.P("//")
			g.P(service.Comments.Leading.String(), "//")
		}
		g.P()
		genTitle(g, service, "Element", service.GoName)
		genElementIDInterface(g, service)
		genTitle(g, service, "Atom", service.GoName)
		genAtomIDInterface(g, service)
		// Interface
		genElementInterface(g, service)
		genAtomInterface(g, service)
	}
	g.P()
	g.P("//////")
	g.P("//// IMPLEMENTATIONS")
	g.P("//")
	g.P()
	for _, service := range file.Services {
		elementName := service.GoName
		if elementName == "Main" {
			gen.Error(errors.New("cannot use element name \"Main\""))
			continue
		}
		genTitle(g, service, "Element", service.GoName)
		genElementIDInternal(g, service)
		genTitle(g, service, "Atom", service.GoName)
		genAtomIDInternal(g, service)
		genImplement(file, g, service)
	}
}

func genTitle(g *protogen.GeneratedFile, service *protogen.Service, prefix, name string) {
	head, tail := fmt.Sprintf("////////// %s: ", prefix), " //////////"
	nameLen := len(name) + len(head) + len(tail)
	c := strings.Repeat("/", nameLen)
	g.P(c)
	g.P(head, name, tail)
	g.P(c)
	g.P()
}

func genElementIDInterface(g *protogen.GeneratedFile, service *protogen.Service) {
	elementName := service.GoName
	elementIDName := service.GoName + "ElementID"

	g.P("const ", elementName, "Name = \"", elementName, "\"")
	g.P()
	g.P("// ", elementIDName, " is the interface of ", elementName, " element.")
	g.P()

	if service.Desc.Options().(*descriptorpb.ServiceOptions).GetDeprecated() {
		g.P("//")
		g.P(deprecationComment)
	}
	g.Annotate(elementIDName, service.Location)
	g.P("type ", elementIDName, " interface {")
	g.P(atomosPackage.Ident("ReleasableID"))
	// Element Methods
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
		g.Annotate(elementIDName+"."+methodName, method.Location)
		if method.Desc.Options().(*descriptorpb.MethodOptions).GetDeprecated() {
			g.P(deprecationComment)
		}
		g.P()
		commentLen := len(method.Comments.Leading.String())
		if commentLen > 0 {
			g.P(method.Comments.Leading.String()[:commentLen-1])
		}
		methodSign(g, method, methodName)
		g.P()
	}
	// Scale Methods
	hasScale := false
	for _, method := range service.Methods {
		methodName := method.GoName
		if !strings.HasPrefix(methodName, "Scale") {
			continue
		}
		if strings.TrimPrefix(methodName, "Scale") == "" {
			continue
		}
		if !hasScale {
			g.P()
			g.P("// Scale Methods")
			g.P()
			hasScale = true
		}
		g.Annotate(elementIDName+"."+methodName, method.Location)
		if method.Desc.Options().(*descriptorpb.MethodOptions).GetDeprecated() {
			g.P(deprecationComment)
		}
		g.P()
		commentLen := len(method.Comments.Leading.String())
		if commentLen > 0 {
			g.P(method.Comments.Leading.String()[:commentLen-1])
		}
		methodSign(g, method, methodName)
		g.P()
	}
	g.P("}")
	g.P()

	// NewClient factory.
	if service.Desc.Options().(*descriptorpb.ServiceOptions).GetDeprecated() {
		g.P(deprecationComment)
	}
	g.P("func Get", elementIDName, " (c ", atomosPackage.Ident("CosmosNode"), ") (", elementIDName, ", *", atomosPackage.Ident("Error"), ") {")
	g.P("ca, err := c.CosmosGetElementID(", elementName, "Name)")
	g.P("if err != nil { return nil, err }")
	g.P("return &", noExport(elementIDName), "{ca, nil, ", atomosPackage.Ident("DefaultTimeout"), "}, nil")
	g.P("}")
	g.P()
}

func genAtomIDInterface(g *protogen.GeneratedFile, service *protogen.Service) {
	atomName := service.GoName
	atomNameID := service.GoName + "AtomID"

	g.P("// ", atomNameID, " is the interface of ", atomName, " atom.")
	g.P()

	if service.Desc.Options().(*descriptorpb.ServiceOptions).GetDeprecated() {
		g.P("//")
		g.P(deprecationComment)
	}
	g.Annotate(atomNameID, service.Location)
	g.P("type ", atomNameID, " interface {")
	g.P(atomosPackage.Ident("ReleasableID"))
	// Atom
	for _, method := range service.Methods {
		methodName := method.GoName
		if strings.HasPrefix(methodName, "Element") {
			continue
		}
		if strings.HasPrefix(methodName, "Scale") {
			continue
		}
		if strings.HasPrefix(methodName, "Spawn") {
			continue
		}
		if methodName == "" {
			continue
		}
		g.Annotate(atomNameID+"."+methodName, method.Location)
		if method.Desc.Options().(*descriptorpb.MethodOptions).GetDeprecated() {
			g.P(deprecationComment)
		}
		g.P()
		commentLen := len(method.Comments.Leading.String())
		if commentLen > 0 {
			g.P(method.Comments.Leading.String()[:commentLen-1])
		}
		methodSign(g, method, methodName)
		g.P()
	}
	// Scale
	hasScale := false
	for _, method := range service.Methods {
		methodName := method.GoName
		if !strings.HasPrefix(methodName, "Scale") {
			continue
		}
		methodName = strings.TrimPrefix(methodName, "Scale")
		if methodName == "" {
			continue
		}
		if !hasScale {
			g.P()
			g.P("// Scale Methods")
			g.P()
			hasScale = true
		}
		g.Annotate(atomNameID+"."+methodName, method.Location)
		if method.Desc.Options().(*descriptorpb.MethodOptions).GetDeprecated() {
			g.P(deprecationComment)
		}
		g.P()
		commentLen := len(method.Comments.Leading.String())
		if commentLen > 0 {
			g.P(method.Comments.Leading.String()[:commentLen-1])
		}
		methodSign(g, method, methodName)
		g.P()
	}
	g.P("}")
	g.P()

	// NewClient factory.
	if service.Desc.Options().(*descriptorpb.ServiceOptions).GetDeprecated() {
		g.P(deprecationComment)
	}
	g.P("func Get", atomNameID, " (c ", atomosPackage.Ident("CosmosNode"), ", name string) (", atomNameID, ", *", atomosPackage.Ident("Error"), ") {")
	g.P("ca, tracker, err := c.CosmosGetAtomID(", atomName, "Name, name)")
	g.P("if err != nil { return nil, err }")
	g.P("return &", noExport(atomNameID), "{ca, tracker, ", atomosPackage.Ident("DefaultTimeout"), "}, nil")
	g.P("}")
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
		g.Annotate(elementName+"."+methodName, method.Location)
		if method.Desc.Options().(*descriptorpb.MethodOptions).GetDeprecated() {
			g.P(deprecationComment)
		}
		commentLen := len(method.Comments.Leading.String())
		if commentLen > 0 {
			g.P(method.Comments.Leading.String()[:commentLen-1])
		}
		if methodName == "Spawn" {
			hasElementSpawn = true
			spawnElementSign(g, method)
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
	elementName := service.GoName
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
	var spawnArgTypeName string
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
			spawnArgTypeName = g.QualifiedGoIdent(method.Input.GoIdent)
			spawnAtomSign(g, method)
		} else {
			methodSign(g, method, methodName)
		}
	}
	// Scale
	for _, method := range service.Methods {
		methodName := method.GoName
		if !strings.HasPrefix(methodName, "Scale") {
			continue
		}
		methodName = strings.TrimPrefix(methodName, "Scale")
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

	idName := atomName + "ID"
	// Spawn
	if spawnArgTypeName != "" {
		g.P("func Spawn", atomName, "(c ", atomosPackage.Ident("CosmosNode"),
			", name string, arg *", spawnArgTypeName, ") (",
			idName, ", *", atomosPackage.Ident("Error"), ") {")
		g.P("id, tracker, err := c.CosmosSpawnAtom(", elementName, "Name, name, arg)")
		g.P("if id == nil { return nil, err.AddStack(nil) }")
		g.P("return &", noExport(idName), "{id, tracker, ", atomosPackage.Ident("DefaultTimeout"), "}, err")
		g.P("}")
	}
}

func genElementIDInternal(g *protogen.GeneratedFile, service *protogen.Service) {
	idName := service.GoName + "ElementID"

	// ID structure.
	g.P("type ", noExport(idName), " struct {")
	g.P(atomosPackage.Ident("ID"))
	g.P("*", atomosPackage.Ident("IDTracker"))
	g.P("Timeout ", timePackage.Ident("Duration"))
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
		g.P("func (c *", noExport(idName), ") ", methodName+"(from ", atomosPackage.Ident("ID"),
			", in *", g.QualifiedGoIdent(method.Input.GoIdent),
			") (*", g.QualifiedGoIdent(method.Output.GoIdent),
			", *", atomosPackage.Ident("Error"), ")", " {")
		g.P("r, err := c.Cosmos().CosmosMessageElement(from, c, \"", methodName, "\", c.Timeout, in)")
		g.P("if r == nil { return nil, err }")
		g.P("reply, ok := r.(*", method.Output.GoIdent, ")")
		g.P("if !ok { return nil, ", atomosPackage.Ident("NewErrorf("), atomosPackage.Ident("ErrAtomMessageReplyType"), ", \"Reply type=(%T)\", r) }")
		g.P("return reply, err")
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
		g.P("func (c *", noExport(idName), ") ", methodName+"(from ", atomosPackage.Ident("ID"),
			", in *", g.QualifiedGoIdent(method.Input.GoIdent),
			") (*", g.QualifiedGoIdent(method.Output.GoIdent),
			", *", atomosPackage.Ident("Error"), ")", " {")
		g.P("id, tracker, err := c.Cosmos().CosmosGetScaleAtomID(from, ", service.GoName, "Name, \"", scaleName, "\", c.Timeout, in)")
		g.P("if err != nil { return nil, err }")
		g.P("defer tracker.Release()")
		g.P("r, err := c.Cosmos().CosmosMessageAtom(from, id, \"", scaleName, "\", c.Timeout, in)")
		g.P("if r == nil { return nil, err }")
		g.P("reply, ok := r.(*", method.Output.GoIdent, ")")
		g.P("if !ok { return nil, ", atomosPackage.Ident("NewErrorf("), atomosPackage.Ident("ErrAtomMessageReplyType"), ", \"Reply type=(%T)\", r) }")
		g.P("return reply, err")
		g.P("}")
		g.P()
	}
}

func genAtomIDInternal(g *protogen.GeneratedFile, service *protogen.Service) {
	idName := service.GoName + "AtomID"

	// ID structure.
	g.P("type ", noExport(idName), " struct {")
	g.P(atomosPackage.Ident("ID"))
	g.P("*", atomosPackage.Ident("IDTracker"))
	g.P("Timeout ", timePackage.Ident("Duration"))
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
		if strings.HasPrefix(methodName, "Scale") {
			methodName = strings.TrimPrefix(methodName, "Scale")
		}
		if methodName == "" {
			continue
		}
		g.P("func (c *", noExport(idName), ") ", methodName+"(from ", atomosPackage.Ident("ID"),
			", in *", g.QualifiedGoIdent(method.Input.GoIdent),
			") (*", g.QualifiedGoIdent(method.Output.GoIdent),
			", *", atomosPackage.Ident("Error"), ")", " {")
		g.P("r, err := c.Cosmos().CosmosMessageAtom(from, c, \"", methodName, "\", c.Timeout, in)")
		g.P("if r == nil { return nil, err }")
		g.P("reply, ok := r.(*", method.Output.GoIdent, ")")
		g.P("if !ok { return nil, ", atomosPackage.Ident("NewErrorf("), atomosPackage.Ident("ErrAtomMessageReplyType"), ", \"Reply type=(%T)\", r) }")
		g.P("return reply, err")
		g.P("}")
		g.P()
	}
}

func genImplement(file *protogen.File, g *protogen.GeneratedFile, service *protogen.Service) {
	elementName := service.GoName
	elementAtomName := elementName + "Atom"
	elementElementName := elementName + "Element"
	interfaceName := service.GoName + "Interface"

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
		methodName = strings.TrimPrefix(methodName, "Element")
		if methodName == "" {
			continue
		}
		g.P("\"", methodName, "\": {")
		g.P("InDec: func(b []byte, p bool) (", protobufPackage.Ident("Message"), ", *", atomosPackage.Ident("Error"), ") { return ", atomosPackage.Ident("MessageUnmarshal"), "(b, &", method.Input.GoIdent, "{}, p) },")
		g.P("OutDec: func(b []byte, p bool) (", protobufPackage.Ident("Message"), ", *", atomosPackage.Ident("Error"), ") { return ", atomosPackage.Ident("MessageUnmarshal"), "(b, &", method.Output.GoIdent, "{}, p) },")
		g.P("},")
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
		if strings.HasPrefix(methodName, "Scale") {
			methodName = strings.TrimPrefix(methodName, "Scale")
		}
		if methodName == "" {
			continue
		}
		g.P("\"", methodName, "\": {")
		g.P("InDec: func(b []byte, p bool) (", protobufPackage.Ident("Message"), ", *", atomosPackage.Ident("Error"), ") { return ", atomosPackage.Ident("MessageUnmarshal"), "(b, &", method.Input.GoIdent, "{}, p) },")
		g.P("OutDec: func(b []byte, p bool) (", protobufPackage.Ident("Message"), ", *", atomosPackage.Ident("Error"), ") { return ", atomosPackage.Ident("MessageUnmarshal"), "(b, &", method.Output.GoIdent, "{}, p) },")
		g.P("},")
	}
	g.P("}")

	//g.P("elem.Config.Messages = map[string]*", atomosPackage.Ident("AtomMessageConfig"), "{")
	//for _, method := range service.Methods {
	//	if method.GoName == "Spawn" || method.GoName == "SpawnWormhole" {
	//		continue
	//	}
	//	g.P("\"", method.GoName, "\": ", atomosPackage.Ident("NewAtomCallConfig"), "(&", method.Input.GoIdent, "{}, &", method.Output.GoIdent, "{}),")
	//}
	//g.P("}")
	//g.P("elem.AtomMessages = map[string]*", atomosPackage.Ident("ElementAtomMessage"), "{")
	//for _, method := range service.Methods {
	//	if method.GoName == "Spawn" || method.GoName == "SpawnWormhole" {
	//		continue
	//	}
	//	g.P("\"", method.GoName, "\": {")
	//	g.P("InDec: func(b []byte) (", protobufPackage.Ident("Message"), ", error) { return ", atomosPackage.Ident("MessageUnmarshal"), "(b, &", method.Input.GoIdent, "{}) },")
	//	g.P("OutDec: func(b []byte) (", protobufPackage.Ident("Message"), ", error) { return ", atomosPackage.Ident("MessageUnmarshal"), "(b, &", method.Output.GoIdent, "{}) },")
	//	g.P("},")
	//}
	//g.P("}")
	g.P("return elem")
	g.P("}")

	g.P()

	g.P("func Get", elementName, "Implement(dev ", atomosPackage.Ident("ElementDeveloper"), ") *", atomosPackage.Ident("ElementImplementation"), "{")
	g.P("elem := ", atomosPackage.Ident("NewImplementationFromDeveloper"), "(dev)")
	g.P("elem.Interface = Get", interfaceName, "(dev)")
	g.P("elem.ElementHandlers = map[string]", atomosPackage.Ident("MessageHandler"), "{")

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
		g.P("\"", methodName, "\": func(from ", atomosPackage.Ident("ID"), ", to ", atomosPackage.Ident("Atomos"), ", in ", protobufPackage.Ident("Message"), ") (", protobufPackage.Ident("Message"), ", *", atomosPackage.Ident("Error"), ") {")
		g.P("req, ok := in.(*", method.Input.GoIdent, ")")
		g.P("if !ok { return nil, ", atomosPackage.Ident("NewErrorf"), "(", atomosPackage.Ident("ErrAtomMessageArgType"), ", \"Arg type=(%T)\", in) }")
		g.P("a, ok := to.(", elementElementName, ")")
		g.P("if !ok { return nil, ", atomosPackage.Ident("NewErrorf"), "(", atomosPackage.Ident("ErrAtomMessageAtomType"), ", \"Atom type=(%T)\", to) }")
		g.P("return a.", methodName, "(from, req)")
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
		if strings.HasPrefix(methodName, "Scale") {
			methodName = strings.TrimPrefix(methodName, "Scale")
		}
		if methodName == "" {
			continue
		}
		g.P("\"", methodName, "\": func(from ", atomosPackage.Ident("ID"), ", to ", atomosPackage.Ident("Atomos"), ", in ", protobufPackage.Ident("Message"), ") (", protobufPackage.Ident("Message"), ", *", atomosPackage.Ident("Error"), ") {")
		g.P("req, ok := in.(*", method.Input.GoIdent, ")")
		g.P("if !ok { return nil, ", atomosPackage.Ident("NewErrorf"), "(", atomosPackage.Ident("ErrAtomMessageArgType"), ", \"Arg type=(%T)\", in) }")
		g.P("a, ok := to.(", elementAtomName, ")")
		g.P("if !ok { return nil, ", atomosPackage.Ident("NewErrorf"), "(", atomosPackage.Ident("ErrAtomMessageAtomType"), ", \"Atom type=(%T)\", to) }")
		g.P("return a.", methodName, "(from, req)")
		g.P("},")
	}
	g.P("}")

	g.P("elem.ScaleHandlers = map[string]", atomosPackage.Ident("ScaleHandler"), "{")
	for _, method := range service.Methods {
		methodName := method.GoName
		if !strings.HasPrefix(methodName, "Scale") {
			continue
		}
		methodName = strings.TrimPrefix(methodName, "Scale")
		if methodName == "" {
			continue
		}
		g.P("\"", methodName, "\": func(from ", atomosPackage.Ident("ID"), ", e ", atomosPackage.Ident("Atomos"), ", message string, in ", protobufPackage.Ident("Message"), ") (id ", atomosPackage.Ident("ID"), ", err *", atomosPackage.Ident("Error"), ") {")
		g.P("req, ok := in.(*", method.Input.GoIdent, ")")
		g.P("if !ok { return nil, ", atomosPackage.Ident("NewErrorf"), "(", atomosPackage.Ident("ErrAtomMessageArgType"), ", \"Arg type=(%T)\", in) }")
		g.P("a, ok := e.(", elementElementName, ")")
		g.P("if !ok { return nil, ", atomosPackage.Ident("NewErrorf"), "(", atomosPackage.Ident("ErrAtomMessageAtomType"), ", \"Element type=(%T)\", e) }")
		g.P("return a.Scale", methodName, "(from, req)")
		g.P("},")
	}
	g.P("}")

	g.P("return elem")
	g.P("}")
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
		") (*", g.QualifiedGoIdent(method.Output.GoIdent),
		", *", atomosPackage.Ident("Error"),
		")")
}

func elementScaleSign(g *protogen.GeneratedFile, method *protogen.Method, elemName, methodName string) {
	g.P(methodName+"(from ", atomosPackage.Ident("ID"),
		", in *", g.QualifiedGoIdent(method.Input.GoIdent),
		") (", elemName+"AtomID, *", atomosPackage.Ident("Error"),
		")")
}

func noExport(s string) string {
	return strings.ToLower(s[:1]) + s[1:]
}
