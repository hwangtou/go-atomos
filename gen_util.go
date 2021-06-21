package go_atomos

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func ElementFromImplement(implement ElementImplement) *ElementDefine {
	name, version, logLevel, initNum := implement.Info()
	return &ElementDefine{
		Config: &ElementConfig{
			Name:          name,
			Version:       version,
			LogLevel:      logLevel,
			AtomInitNum:   int32(initNum),
		},
		AtomCreator: implement.AtomCreator,
		AtomSaver: implement.AtomSaver,
		AtomCanKill: implement.AtomCanKill,
	}
}

func ElementAtomCallConfig(in, out proto.Message) *AtomosCallConfig {
	return &AtomosCallConfig{
		In: MessageToAny(in),
		Out: MessageToAny(out),
	}
}

func MessageToAny(p proto.Message) *anypb.Any {
	any, _ := anypb.New(p)
	return any
}

func CallUnmarshal(b []byte, p proto.Message) (proto.Message, error) {
	return p, proto.Unmarshal(b, p)
}
