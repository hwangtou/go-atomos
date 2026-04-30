package atomos

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func UtilProtoMessageIterate(message proto.Message) map[string]any {
	all := make(map[string]any)
	utilProtoMessageIterate(message.ProtoReflect(), "", all)
	return all
}

func utilProtoMessageIterate(message protoreflect.Message, prefix string, all map[string]any) {
	if prefix != "" {
		prefix += "."
	}
	message.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		// Check if the field is of message type (embedded message)
		if fd.Kind() == protoreflect.MessageKind {
			// Handle single embedded messages
			utilProtoMessageIterate(v.Message(), prefix+string(fd.Name()), all)
		} else {
			// Print field information for non-message fields
			all[prefix+string(fd.Name())] = v.Interface()
		}
		return true // Continue iterating over other fields
	})
}
