// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.31.0
// 	protoc        v4.24.3
// source: examples/hello_atomos/api/hello.proto

package api

import (
	go_atomos "github.com/hwangtou/go-atomos"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// 测试模式
type HADoTestI_TestMode int32

const (
	// 无
	HADoTestI_None HADoTestI_TestMode = 0
	// 测试自我同步调用死锁
	HADoTestI_SyncSelfCallDeadlock HADoTestI_TestMode = 1
	// 测试自我异步调用无死锁
	HADoTestI_AsyncSelfCallNoDeadlock HADoTestI_TestMode = 2
	// 测试环形同步调用死锁
	HADoTestI_SyncRingCallDeadlock HADoTestI_TestMode = 3
	// 测试环形异步调用无死锁
	HADoTestI_AsyncRingCallNoDeadlock HADoTestI_TestMode = 4
	// 测试环形异步调用死锁
	HADoTestI_AsyncRingCallDeadlock HADoTestI_TestMode = 5
	// 测试环形异步调用无死锁无循环
	HADoTestI_AsyncRingCallNoReplyNoDeadlock HADoTestI_TestMode = 6
	// 测试环形异步调用无死锁有循环
	HADoTestI_AsyncRingCallNoReplyDeadlock HADoTestI_TestMode = 7
	// 后续步骤
	HADoTestI_SyncRingCallDeadlockStep2           HADoTestI_TestMode = 100
	HADoTestI_AsyncRingCallNoDeadlockStep2        HADoTestI_TestMode = 101
	HADoTestI_AsyncRingCallDeadlockStep2          HADoTestI_TestMode = 102
	HADoTestI_AsyncRingCallNoReplyNoDeadlockStep2 HADoTestI_TestMode = 103
	HADoTestI_AsyncRingCallNoReplyDeadlockStep2   HADoTestI_TestMode = 104
)

// Enum value maps for HADoTestI_TestMode.
var (
	HADoTestI_TestMode_name = map[int32]string{
		0:   "None",
		1:   "SyncSelfCallDeadlock",
		2:   "AsyncSelfCallNoDeadlock",
		3:   "SyncRingCallDeadlock",
		4:   "AsyncRingCallNoDeadlock",
		5:   "AsyncRingCallDeadlock",
		6:   "AsyncRingCallNoReplyNoDeadlock",
		7:   "AsyncRingCallNoReplyDeadlock",
		100: "SyncRingCallDeadlockStep2",
		101: "AsyncRingCallNoDeadlockStep2",
		102: "AsyncRingCallDeadlockStep2",
		103: "AsyncRingCallNoReplyNoDeadlockStep2",
		104: "AsyncRingCallNoReplyDeadlockStep2",
	}
	HADoTestI_TestMode_value = map[string]int32{
		"None":                                0,
		"SyncSelfCallDeadlock":                1,
		"AsyncSelfCallNoDeadlock":             2,
		"SyncRingCallDeadlock":                3,
		"AsyncRingCallNoDeadlock":             4,
		"AsyncRingCallDeadlock":               5,
		"AsyncRingCallNoReplyNoDeadlock":      6,
		"AsyncRingCallNoReplyDeadlock":        7,
		"SyncRingCallDeadlockStep2":           100,
		"AsyncRingCallNoDeadlockStep2":        101,
		"AsyncRingCallDeadlockStep2":          102,
		"AsyncRingCallNoReplyNoDeadlockStep2": 103,
		"AsyncRingCallNoReplyDeadlockStep2":   104,
	}
)

func (x HADoTestI_TestMode) Enum() *HADoTestI_TestMode {
	p := new(HADoTestI_TestMode)
	*p = x
	return p
}

func (x HADoTestI_TestMode) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (HADoTestI_TestMode) Descriptor() protoreflect.EnumDescriptor {
	return file_examples_hello_atomos_api_hello_proto_enumTypes[0].Descriptor()
}

func (HADoTestI_TestMode) Type() protoreflect.EnumType {
	return &file_examples_hello_atomos_api_hello_proto_enumTypes[0]
}

func (x HADoTestI_TestMode) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use HADoTestI_TestMode.Descriptor instead.
func (HADoTestI_TestMode) EnumDescriptor() ([]byte, []int) {
	return file_examples_hello_atomos_api_hello_proto_rawDescGZIP(), []int{9, 0}
}

type HAEData struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *HAEData) Reset() {
	*x = HAEData{}
	if protoimpl.UnsafeEnabled {
		mi := &file_examples_hello_atomos_api_hello_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *HAEData) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*HAEData) ProtoMessage() {}

func (x *HAEData) ProtoReflect() protoreflect.Message {
	mi := &file_examples_hello_atomos_api_hello_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use HAEData.ProtoReflect.Descriptor instead.
func (*HAEData) Descriptor() ([]byte, []int) {
	return file_examples_hello_atomos_api_hello_proto_rawDescGZIP(), []int{0}
}

type HAEHelloI struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
}

func (x *HAEHelloI) Reset() {
	*x = HAEHelloI{}
	if protoimpl.UnsafeEnabled {
		mi := &file_examples_hello_atomos_api_hello_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *HAEHelloI) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*HAEHelloI) ProtoMessage() {}

func (x *HAEHelloI) ProtoReflect() protoreflect.Message {
	mi := &file_examples_hello_atomos_api_hello_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use HAEHelloI.ProtoReflect.Descriptor instead.
func (*HAEHelloI) Descriptor() ([]byte, []int) {
	return file_examples_hello_atomos_api_hello_proto_rawDescGZIP(), []int{1}
}

func (x *HAEHelloI) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

type HAEHelloO struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Message string `protobuf:"bytes,1,opt,name=message,proto3" json:"message,omitempty"`
}

func (x *HAEHelloO) Reset() {
	*x = HAEHelloO{}
	if protoimpl.UnsafeEnabled {
		mi := &file_examples_hello_atomos_api_hello_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *HAEHelloO) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*HAEHelloO) ProtoMessage() {}

func (x *HAEHelloO) ProtoReflect() protoreflect.Message {
	mi := &file_examples_hello_atomos_api_hello_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use HAEHelloO.ProtoReflect.Descriptor instead.
func (*HAEHelloO) Descriptor() ([]byte, []int) {
	return file_examples_hello_atomos_api_hello_proto_rawDescGZIP(), []int{2}
}

func (x *HAEHelloO) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}

type HABonjourI struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *HABonjourI) Reset() {
	*x = HABonjourI{}
	if protoimpl.UnsafeEnabled {
		mi := &file_examples_hello_atomos_api_hello_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *HABonjourI) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*HABonjourI) ProtoMessage() {}

func (x *HABonjourI) ProtoReflect() protoreflect.Message {
	mi := &file_examples_hello_atomos_api_hello_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use HABonjourI.ProtoReflect.Descriptor instead.
func (*HABonjourI) Descriptor() ([]byte, []int) {
	return file_examples_hello_atomos_api_hello_proto_rawDescGZIP(), []int{3}
}

type HABonjourO struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *HABonjourO) Reset() {
	*x = HABonjourO{}
	if protoimpl.UnsafeEnabled {
		mi := &file_examples_hello_atomos_api_hello_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *HABonjourO) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*HABonjourO) ProtoMessage() {}

func (x *HABonjourO) ProtoReflect() protoreflect.Message {
	mi := &file_examples_hello_atomos_api_hello_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use HABonjourO.ProtoReflect.Descriptor instead.
func (*HABonjourO) Descriptor() ([]byte, []int) {
	return file_examples_hello_atomos_api_hello_proto_rawDescGZIP(), []int{4}
}

type HAData struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *HAData) Reset() {
	*x = HAData{}
	if protoimpl.UnsafeEnabled {
		mi := &file_examples_hello_atomos_api_hello_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *HAData) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*HAData) ProtoMessage() {}

func (x *HAData) ProtoReflect() protoreflect.Message {
	mi := &file_examples_hello_atomos_api_hello_proto_msgTypes[5]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use HAData.ProtoReflect.Descriptor instead.
func (*HAData) Descriptor() ([]byte, []int) {
	return file_examples_hello_atomos_api_hello_proto_rawDescGZIP(), []int{5}
}

type HASpawnArg struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Id int32 `protobuf:"varint,1,opt,name=id,proto3" json:"id,omitempty"`
}

func (x *HASpawnArg) Reset() {
	*x = HASpawnArg{}
	if protoimpl.UnsafeEnabled {
		mi := &file_examples_hello_atomos_api_hello_proto_msgTypes[6]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *HASpawnArg) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*HASpawnArg) ProtoMessage() {}

func (x *HASpawnArg) ProtoReflect() protoreflect.Message {
	mi := &file_examples_hello_atomos_api_hello_proto_msgTypes[6]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use HASpawnArg.ProtoReflect.Descriptor instead.
func (*HASpawnArg) Descriptor() ([]byte, []int) {
	return file_examples_hello_atomos_api_hello_proto_rawDescGZIP(), []int{6}
}

func (x *HASpawnArg) GetId() int32 {
	if x != nil {
		return x.Id
	}
	return 0
}

type HAGreetingI struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Message string `protobuf:"bytes,1,opt,name=message,proto3" json:"message,omitempty"`
}

func (x *HAGreetingI) Reset() {
	*x = HAGreetingI{}
	if protoimpl.UnsafeEnabled {
		mi := &file_examples_hello_atomos_api_hello_proto_msgTypes[7]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *HAGreetingI) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*HAGreetingI) ProtoMessage() {}

func (x *HAGreetingI) ProtoReflect() protoreflect.Message {
	mi := &file_examples_hello_atomos_api_hello_proto_msgTypes[7]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use HAGreetingI.ProtoReflect.Descriptor instead.
func (*HAGreetingI) Descriptor() ([]byte, []int) {
	return file_examples_hello_atomos_api_hello_proto_rawDescGZIP(), []int{7}
}

func (x *HAGreetingI) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}

type HAGreetingO struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Message string `protobuf:"bytes,1,opt,name=message,proto3" json:"message,omitempty"`
}

func (x *HAGreetingO) Reset() {
	*x = HAGreetingO{}
	if protoimpl.UnsafeEnabled {
		mi := &file_examples_hello_atomos_api_hello_proto_msgTypes[8]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *HAGreetingO) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*HAGreetingO) ProtoMessage() {}

func (x *HAGreetingO) ProtoReflect() protoreflect.Message {
	mi := &file_examples_hello_atomos_api_hello_proto_msgTypes[8]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use HAGreetingO.ProtoReflect.Descriptor instead.
func (*HAGreetingO) Descriptor() ([]byte, []int) {
	return file_examples_hello_atomos_api_hello_proto_rawDescGZIP(), []int{8}
}

func (x *HAGreetingO) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}

type HADoTestI struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Mode HADoTestI_TestMode `protobuf:"varint,1,opt,name=mode,proto3,enum=api.HADoTestI_TestMode" json:"mode,omitempty"`
}

func (x *HADoTestI) Reset() {
	*x = HADoTestI{}
	if protoimpl.UnsafeEnabled {
		mi := &file_examples_hello_atomos_api_hello_proto_msgTypes[9]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *HADoTestI) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*HADoTestI) ProtoMessage() {}

func (x *HADoTestI) ProtoReflect() protoreflect.Message {
	mi := &file_examples_hello_atomos_api_hello_proto_msgTypes[9]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use HADoTestI.ProtoReflect.Descriptor instead.
func (*HADoTestI) Descriptor() ([]byte, []int) {
	return file_examples_hello_atomos_api_hello_proto_rawDescGZIP(), []int{9}
}

func (x *HADoTestI) GetMode() HADoTestI_TestMode {
	if x != nil {
		return x.Mode
	}
	return HADoTestI_None
}

type HADoTestO struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *HADoTestO) Reset() {
	*x = HADoTestO{}
	if protoimpl.UnsafeEnabled {
		mi := &file_examples_hello_atomos_api_hello_proto_msgTypes[10]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *HADoTestO) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*HADoTestO) ProtoMessage() {}

func (x *HADoTestO) ProtoReflect() protoreflect.Message {
	mi := &file_examples_hello_atomos_api_hello_proto_msgTypes[10]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use HADoTestO.ProtoReflect.Descriptor instead.
func (*HADoTestO) Descriptor() ([]byte, []int) {
	return file_examples_hello_atomos_api_hello_proto_rawDescGZIP(), []int{10}
}

var File_examples_hello_atomos_api_hello_proto protoreflect.FileDescriptor

var file_examples_hello_atomos_api_hello_proto_rawDesc = []byte{
	0x0a, 0x25, 0x65, 0x78, 0x61, 0x6d, 0x70, 0x6c, 0x65, 0x73, 0x2f, 0x68, 0x65, 0x6c, 0x6c, 0x6f,
	0x5f, 0x61, 0x74, 0x6f, 0x6d, 0x6f, 0x73, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x68, 0x65, 0x6c, 0x6c,
	0x6f, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x03, 0x61, 0x70, 0x69, 0x1a, 0x0c, 0x61, 0x74,
	0x6f, 0x6d, 0x6f, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x09, 0x0a, 0x07, 0x48, 0x41,
	0x45, 0x44, 0x61, 0x74, 0x61, 0x22, 0x1f, 0x0a, 0x09, 0x48, 0x41, 0x45, 0x48, 0x65, 0x6c, 0x6c,
	0x6f, 0x49, 0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x22, 0x25, 0x0a, 0x09, 0x48, 0x41, 0x45, 0x48, 0x65, 0x6c,
	0x6c, 0x6f, 0x4f, 0x12, 0x18, 0x0a, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x18, 0x01,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x22, 0x0c, 0x0a,
	0x0a, 0x48, 0x41, 0x42, 0x6f, 0x6e, 0x6a, 0x6f, 0x75, 0x72, 0x49, 0x22, 0x0c, 0x0a, 0x0a, 0x48,
	0x41, 0x42, 0x6f, 0x6e, 0x6a, 0x6f, 0x75, 0x72, 0x4f, 0x22, 0x08, 0x0a, 0x06, 0x48, 0x41, 0x44,
	0x61, 0x74, 0x61, 0x22, 0x1c, 0x0a, 0x0a, 0x48, 0x41, 0x53, 0x70, 0x61, 0x77, 0x6e, 0x41, 0x72,
	0x67, 0x12, 0x0e, 0x0a, 0x02, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x05, 0x52, 0x02, 0x69,
	0x64, 0x22, 0x27, 0x0a, 0x0b, 0x48, 0x41, 0x47, 0x72, 0x65, 0x65, 0x74, 0x69, 0x6e, 0x67, 0x49,
	0x12, 0x18, 0x0a, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x22, 0x27, 0x0a, 0x0b, 0x48, 0x41,
	0x47, 0x72, 0x65, 0x65, 0x74, 0x69, 0x6e, 0x67, 0x4f, 0x12, 0x18, 0x0a, 0x07, 0x6d, 0x65, 0x73,
	0x73, 0x61, 0x67, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x6d, 0x65, 0x73, 0x73,
	0x61, 0x67, 0x65, 0x22, 0xcf, 0x03, 0x0a, 0x09, 0x48, 0x41, 0x44, 0x6f, 0x54, 0x65, 0x73, 0x74,
	0x49, 0x12, 0x2b, 0x0a, 0x04, 0x6d, 0x6f, 0x64, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0e, 0x32,
	0x17, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x48, 0x41, 0x44, 0x6f, 0x54, 0x65, 0x73, 0x74, 0x49, 0x2e,
	0x54, 0x65, 0x73, 0x74, 0x4d, 0x6f, 0x64, 0x65, 0x52, 0x04, 0x6d, 0x6f, 0x64, 0x65, 0x22, 0x94,
	0x03, 0x0a, 0x08, 0x54, 0x65, 0x73, 0x74, 0x4d, 0x6f, 0x64, 0x65, 0x12, 0x08, 0x0a, 0x04, 0x4e,
	0x6f, 0x6e, 0x65, 0x10, 0x00, 0x12, 0x18, 0x0a, 0x14, 0x53, 0x79, 0x6e, 0x63, 0x53, 0x65, 0x6c,
	0x66, 0x43, 0x61, 0x6c, 0x6c, 0x44, 0x65, 0x61, 0x64, 0x6c, 0x6f, 0x63, 0x6b, 0x10, 0x01, 0x12,
	0x1b, 0x0a, 0x17, 0x41, 0x73, 0x79, 0x6e, 0x63, 0x53, 0x65, 0x6c, 0x66, 0x43, 0x61, 0x6c, 0x6c,
	0x4e, 0x6f, 0x44, 0x65, 0x61, 0x64, 0x6c, 0x6f, 0x63, 0x6b, 0x10, 0x02, 0x12, 0x18, 0x0a, 0x14,
	0x53, 0x79, 0x6e, 0x63, 0x52, 0x69, 0x6e, 0x67, 0x43, 0x61, 0x6c, 0x6c, 0x44, 0x65, 0x61, 0x64,
	0x6c, 0x6f, 0x63, 0x6b, 0x10, 0x03, 0x12, 0x1b, 0x0a, 0x17, 0x41, 0x73, 0x79, 0x6e, 0x63, 0x52,
	0x69, 0x6e, 0x67, 0x43, 0x61, 0x6c, 0x6c, 0x4e, 0x6f, 0x44, 0x65, 0x61, 0x64, 0x6c, 0x6f, 0x63,
	0x6b, 0x10, 0x04, 0x12, 0x19, 0x0a, 0x15, 0x41, 0x73, 0x79, 0x6e, 0x63, 0x52, 0x69, 0x6e, 0x67,
	0x43, 0x61, 0x6c, 0x6c, 0x44, 0x65, 0x61, 0x64, 0x6c, 0x6f, 0x63, 0x6b, 0x10, 0x05, 0x12, 0x22,
	0x0a, 0x1e, 0x41, 0x73, 0x79, 0x6e, 0x63, 0x52, 0x69, 0x6e, 0x67, 0x43, 0x61, 0x6c, 0x6c, 0x4e,
	0x6f, 0x52, 0x65, 0x70, 0x6c, 0x79, 0x4e, 0x6f, 0x44, 0x65, 0x61, 0x64, 0x6c, 0x6f, 0x63, 0x6b,
	0x10, 0x06, 0x12, 0x20, 0x0a, 0x1c, 0x41, 0x73, 0x79, 0x6e, 0x63, 0x52, 0x69, 0x6e, 0x67, 0x43,
	0x61, 0x6c, 0x6c, 0x4e, 0x6f, 0x52, 0x65, 0x70, 0x6c, 0x79, 0x44, 0x65, 0x61, 0x64, 0x6c, 0x6f,
	0x63, 0x6b, 0x10, 0x07, 0x12, 0x1d, 0x0a, 0x19, 0x53, 0x79, 0x6e, 0x63, 0x52, 0x69, 0x6e, 0x67,
	0x43, 0x61, 0x6c, 0x6c, 0x44, 0x65, 0x61, 0x64, 0x6c, 0x6f, 0x63, 0x6b, 0x53, 0x74, 0x65, 0x70,
	0x32, 0x10, 0x64, 0x12, 0x20, 0x0a, 0x1c, 0x41, 0x73, 0x79, 0x6e, 0x63, 0x52, 0x69, 0x6e, 0x67,
	0x43, 0x61, 0x6c, 0x6c, 0x4e, 0x6f, 0x44, 0x65, 0x61, 0x64, 0x6c, 0x6f, 0x63, 0x6b, 0x53, 0x74,
	0x65, 0x70, 0x32, 0x10, 0x65, 0x12, 0x1e, 0x0a, 0x1a, 0x41, 0x73, 0x79, 0x6e, 0x63, 0x52, 0x69,
	0x6e, 0x67, 0x43, 0x61, 0x6c, 0x6c, 0x44, 0x65, 0x61, 0x64, 0x6c, 0x6f, 0x63, 0x6b, 0x53, 0x74,
	0x65, 0x70, 0x32, 0x10, 0x66, 0x12, 0x27, 0x0a, 0x23, 0x41, 0x73, 0x79, 0x6e, 0x63, 0x52, 0x69,
	0x6e, 0x67, 0x43, 0x61, 0x6c, 0x6c, 0x4e, 0x6f, 0x52, 0x65, 0x70, 0x6c, 0x79, 0x4e, 0x6f, 0x44,
	0x65, 0x61, 0x64, 0x6c, 0x6f, 0x63, 0x6b, 0x53, 0x74, 0x65, 0x70, 0x32, 0x10, 0x67, 0x12, 0x25,
	0x0a, 0x21, 0x41, 0x73, 0x79, 0x6e, 0x63, 0x52, 0x69, 0x6e, 0x67, 0x43, 0x61, 0x6c, 0x6c, 0x4e,
	0x6f, 0x52, 0x65, 0x70, 0x6c, 0x79, 0x44, 0x65, 0x61, 0x64, 0x6c, 0x6f, 0x63, 0x6b, 0x53, 0x74,
	0x65, 0x70, 0x32, 0x10, 0x68, 0x22, 0x0b, 0x0a, 0x09, 0x48, 0x41, 0x44, 0x6f, 0x54, 0x65, 0x73,
	0x74, 0x4f, 0x32, 0xf6, 0x02, 0x0a, 0x0b, 0x48, 0x65, 0x6c, 0x6c, 0x6f, 0x41, 0x74, 0x6f, 0x6d,
	0x6f, 0x73, 0x12, 0x2b, 0x0a, 0x0c, 0x45, 0x6c, 0x65, 0x6d, 0x65, 0x6e, 0x74, 0x53, 0x70, 0x61,
	0x77, 0x6e, 0x12, 0x0b, 0x2e, 0x61, 0x74, 0x6f, 0x6d, 0x6f, 0x73, 0x2e, 0x4e, 0x69, 0x6c, 0x1a,
	0x0c, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x48, 0x41, 0x45, 0x44, 0x61, 0x74, 0x61, 0x22, 0x00, 0x12,
	0x33, 0x0a, 0x0f, 0x45, 0x6c, 0x65, 0x6d, 0x65, 0x6e, 0x74, 0x53, 0x61, 0x79, 0x48, 0x65, 0x6c,
	0x6c, 0x6f, 0x12, 0x0e, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x48, 0x41, 0x45, 0x48, 0x65, 0x6c, 0x6c,
	0x6f, 0x49, 0x1a, 0x0e, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x48, 0x41, 0x45, 0x48, 0x65, 0x6c, 0x6c,
	0x6f, 0x4f, 0x22, 0x00, 0x12, 0x4a, 0x0a, 0x10, 0x45, 0x6c, 0x65, 0x6d, 0x65, 0x6e, 0x74, 0x42,
	0x72, 0x6f, 0x61, 0x64, 0x63, 0x61, 0x73, 0x74, 0x12, 0x19, 0x2e, 0x61, 0x74, 0x6f, 0x6d, 0x6f,
	0x73, 0x2e, 0x45, 0x6c, 0x65, 0x6d, 0x65, 0x6e, 0x74, 0x42, 0x72, 0x6f, 0x61, 0x64, 0x63, 0x61,
	0x73, 0x74, 0x49, 0x1a, 0x19, 0x2e, 0x61, 0x74, 0x6f, 0x6d, 0x6f, 0x73, 0x2e, 0x45, 0x6c, 0x65,
	0x6d, 0x65, 0x6e, 0x74, 0x42, 0x72, 0x6f, 0x61, 0x64, 0x63, 0x61, 0x73, 0x74, 0x4f, 0x22, 0x00,
	0x12, 0x32, 0x0a, 0x0c, 0x53, 0x63, 0x61, 0x6c, 0x65, 0x42, 0x6f, 0x6e, 0x6a, 0x6f, 0x75, 0x72,
	0x12, 0x0f, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x48, 0x41, 0x42, 0x6f, 0x6e, 0x6a, 0x6f, 0x75, 0x72,
	0x49, 0x1a, 0x0f, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x48, 0x41, 0x42, 0x6f, 0x6e, 0x6a, 0x6f, 0x75,
	0x72, 0x4f, 0x22, 0x00, 0x12, 0x27, 0x0a, 0x05, 0x53, 0x70, 0x61, 0x77, 0x6e, 0x12, 0x0f, 0x2e,
	0x61, 0x70, 0x69, 0x2e, 0x48, 0x41, 0x53, 0x70, 0x61, 0x77, 0x6e, 0x41, 0x72, 0x67, 0x1a, 0x0b,
	0x2e, 0x61, 0x70, 0x69, 0x2e, 0x48, 0x41, 0x44, 0x61, 0x74, 0x61, 0x22, 0x00, 0x12, 0x30, 0x0a,
	0x08, 0x47, 0x72, 0x65, 0x65, 0x74, 0x69, 0x6e, 0x67, 0x12, 0x10, 0x2e, 0x61, 0x70, 0x69, 0x2e,
	0x48, 0x41, 0x47, 0x72, 0x65, 0x65, 0x74, 0x69, 0x6e, 0x67, 0x49, 0x1a, 0x10, 0x2e, 0x61, 0x70,
	0x69, 0x2e, 0x48, 0x41, 0x47, 0x72, 0x65, 0x65, 0x74, 0x69, 0x6e, 0x67, 0x4f, 0x22, 0x00, 0x12,
	0x2a, 0x0a, 0x06, 0x44, 0x6f, 0x54, 0x65, 0x73, 0x74, 0x12, 0x0e, 0x2e, 0x61, 0x70, 0x69, 0x2e,
	0x48, 0x41, 0x44, 0x6f, 0x54, 0x65, 0x73, 0x74, 0x49, 0x1a, 0x0e, 0x2e, 0x61, 0x70, 0x69, 0x2e,
	0x48, 0x41, 0x44, 0x6f, 0x54, 0x65, 0x73, 0x74, 0x4f, 0x22, 0x00, 0x42, 0x07, 0x5a, 0x05, 0x2e,
	0x2f, 0x61, 0x70, 0x69, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_examples_hello_atomos_api_hello_proto_rawDescOnce sync.Once
	file_examples_hello_atomos_api_hello_proto_rawDescData = file_examples_hello_atomos_api_hello_proto_rawDesc
)

func file_examples_hello_atomos_api_hello_proto_rawDescGZIP() []byte {
	file_examples_hello_atomos_api_hello_proto_rawDescOnce.Do(func() {
		file_examples_hello_atomos_api_hello_proto_rawDescData = protoimpl.X.CompressGZIP(file_examples_hello_atomos_api_hello_proto_rawDescData)
	})
	return file_examples_hello_atomos_api_hello_proto_rawDescData
}

var file_examples_hello_atomos_api_hello_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_examples_hello_atomos_api_hello_proto_msgTypes = make([]protoimpl.MessageInfo, 11)
var file_examples_hello_atomos_api_hello_proto_goTypes = []interface{}{
	(HADoTestI_TestMode)(0),             // 0: api.HADoTestI.TestMode
	(*HAEData)(nil),                     // 1: api.HAEData
	(*HAEHelloI)(nil),                   // 2: api.HAEHelloI
	(*HAEHelloO)(nil),                   // 3: api.HAEHelloO
	(*HABonjourI)(nil),                  // 4: api.HABonjourI
	(*HABonjourO)(nil),                  // 5: api.HABonjourO
	(*HAData)(nil),                      // 6: api.HAData
	(*HASpawnArg)(nil),                  // 7: api.HASpawnArg
	(*HAGreetingI)(nil),                 // 8: api.HAGreetingI
	(*HAGreetingO)(nil),                 // 9: api.HAGreetingO
	(*HADoTestI)(nil),                   // 10: api.HADoTestI
	(*HADoTestO)(nil),                   // 11: api.HADoTestO
	(*go_atomos.Nil)(nil),               // 12: atomos.Nil
	(*go_atomos.ElementBroadcastI)(nil), // 13: atomos.ElementBroadcastI
	(*go_atomos.ElementBroadcastO)(nil), // 14: atomos.ElementBroadcastO
}
var file_examples_hello_atomos_api_hello_proto_depIdxs = []int32{
	0,  // 0: api.HADoTestI.mode:type_name -> api.HADoTestI.TestMode
	12, // 1: api.HelloAtomos.ElementSpawn:input_type -> atomos.Nil
	2,  // 2: api.HelloAtomos.ElementSayHello:input_type -> api.HAEHelloI
	13, // 3: api.HelloAtomos.ElementBroadcast:input_type -> atomos.ElementBroadcastI
	4,  // 4: api.HelloAtomos.ScaleBonjour:input_type -> api.HABonjourI
	7,  // 5: api.HelloAtomos.Spawn:input_type -> api.HASpawnArg
	8,  // 6: api.HelloAtomos.Greeting:input_type -> api.HAGreetingI
	10, // 7: api.HelloAtomos.DoTest:input_type -> api.HADoTestI
	1,  // 8: api.HelloAtomos.ElementSpawn:output_type -> api.HAEData
	3,  // 9: api.HelloAtomos.ElementSayHello:output_type -> api.HAEHelloO
	14, // 10: api.HelloAtomos.ElementBroadcast:output_type -> atomos.ElementBroadcastO
	5,  // 11: api.HelloAtomos.ScaleBonjour:output_type -> api.HABonjourO
	6,  // 12: api.HelloAtomos.Spawn:output_type -> api.HAData
	9,  // 13: api.HelloAtomos.Greeting:output_type -> api.HAGreetingO
	11, // 14: api.HelloAtomos.DoTest:output_type -> api.HADoTestO
	8,  // [8:15] is the sub-list for method output_type
	1,  // [1:8] is the sub-list for method input_type
	1,  // [1:1] is the sub-list for extension type_name
	1,  // [1:1] is the sub-list for extension extendee
	0,  // [0:1] is the sub-list for field type_name
}

func init() { file_examples_hello_atomos_api_hello_proto_init() }
func file_examples_hello_atomos_api_hello_proto_init() {
	if File_examples_hello_atomos_api_hello_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_examples_hello_atomos_api_hello_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*HAEData); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_examples_hello_atomos_api_hello_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*HAEHelloI); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_examples_hello_atomos_api_hello_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*HAEHelloO); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_examples_hello_atomos_api_hello_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*HABonjourI); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_examples_hello_atomos_api_hello_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*HABonjourO); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_examples_hello_atomos_api_hello_proto_msgTypes[5].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*HAData); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_examples_hello_atomos_api_hello_proto_msgTypes[6].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*HASpawnArg); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_examples_hello_atomos_api_hello_proto_msgTypes[7].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*HAGreetingI); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_examples_hello_atomos_api_hello_proto_msgTypes[8].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*HAGreetingO); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_examples_hello_atomos_api_hello_proto_msgTypes[9].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*HADoTestI); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_examples_hello_atomos_api_hello_proto_msgTypes[10].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*HADoTestO); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_examples_hello_atomos_api_hello_proto_rawDesc,
			NumEnums:      1,
			NumMessages:   11,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_examples_hello_atomos_api_hello_proto_goTypes,
		DependencyIndexes: file_examples_hello_atomos_api_hello_proto_depIdxs,
		EnumInfos:         file_examples_hello_atomos_api_hello_proto_enumTypes,
		MessageInfos:      file_examples_hello_atomos_api_hello_proto_msgTypes,
	}.Build()
	File_examples_hello_atomos_api_hello_proto = out.File
	file_examples_hello_atomos_api_hello_proto_rawDesc = nil
	file_examples_hello_atomos_api_hello_proto_goTypes = nil
	file_examples_hello_atomos_api_hello_proto_depIdxs = nil
}
