// Code generated by protoc-gen-go. DO NOT EDIT.
// source: peer/chaincode.proto

package peer // import "github.com/hyperledger/fabric/protos/peer"

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion2 // please upgrade the proto package

type ChaincodeSpec_Type int32

const (
	ChaincodeSpec_UNDEFINED ChaincodeSpec_Type = 0
	ChaincodeSpec_GOLANG    ChaincodeSpec_Type = 1
	ChaincodeSpec_NODE      ChaincodeSpec_Type = 2
	ChaincodeSpec_CAR       ChaincodeSpec_Type = 3
	ChaincodeSpec_JAVA      ChaincodeSpec_Type = 4
)

var ChaincodeSpec_Type_name = map[int32]string{
	0: "UNDEFINED",
	1: "GOLANG",
	2: "NODE",
	3: "CAR",
	4: "JAVA",
}
var ChaincodeSpec_Type_value = map[string]int32{
	"UNDEFINED": 0,
	"GOLANG":    1,
	"NODE":      2,
	"CAR":       3,
	"JAVA":      4,
}

func (x ChaincodeSpec_Type) String() string {
	return proto.EnumName(ChaincodeSpec_Type_name, int32(x))
}
func (ChaincodeSpec_Type) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_chaincode_4b693947e61b8a8b, []int{2, 0}
}

type ChaincodeDeploymentSpec_ExecutionEnvironment int32

const (
	ChaincodeDeploymentSpec_DOCKER ChaincodeDeploymentSpec_ExecutionEnvironment = 0
	ChaincodeDeploymentSpec_SYSTEM ChaincodeDeploymentSpec_ExecutionEnvironment = 1
)

var ChaincodeDeploymentSpec_ExecutionEnvironment_name = map[int32]string{
	0: "DOCKER",
	1: "SYSTEM",
}
var ChaincodeDeploymentSpec_ExecutionEnvironment_value = map[string]int32{
	"DOCKER": 0,
	"SYSTEM": 1,
}

func (x ChaincodeDeploymentSpec_ExecutionEnvironment) String() string {
	return proto.EnumName(ChaincodeDeploymentSpec_ExecutionEnvironment_name, int32(x))
}
func (ChaincodeDeploymentSpec_ExecutionEnvironment) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_chaincode_4b693947e61b8a8b, []int{3, 0}
}

// ChaincodeID contains the path as specified by the deploy transaction
// that created it as well as the hashCode that is generated by the
// system for the path. From the user level (ie, CLI, REST API and so on)
// deploy transaction is expected to provide the path and other requests
// are expected to provide the hashCode. The other value will be ignored.
// Internally, the structure could contain both values. For instance, the
// hashCode will be set when first generated using the path
type ChaincodeID struct {
	// deploy transaction will use the path
	Path string `protobuf:"bytes,1,opt,name=path,proto3" json:"path,omitempty"`
	// all other requests will use the name (really a hashcode) generated by
	// the deploy transaction
	Name string `protobuf:"bytes,2,opt,name=name,proto3" json:"name,omitempty"`
	// user friendly version name for the chaincode
	Version              string   `protobuf:"bytes,3,opt,name=version,proto3" json:"version,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *ChaincodeID) Reset()         { *m = ChaincodeID{} }
func (m *ChaincodeID) String() string { return proto.CompactTextString(m) }
func (*ChaincodeID) ProtoMessage()    {}
func (*ChaincodeID) Descriptor() ([]byte, []int) {
	return fileDescriptor_chaincode_4b693947e61b8a8b, []int{0}
}
func (m *ChaincodeID) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ChaincodeID.Unmarshal(m, b)
}
func (m *ChaincodeID) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ChaincodeID.Marshal(b, m, deterministic)
}
func (dst *ChaincodeID) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ChaincodeID.Merge(dst, src)
}
func (m *ChaincodeID) XXX_Size() int {
	return xxx_messageInfo_ChaincodeID.Size(m)
}
func (m *ChaincodeID) XXX_DiscardUnknown() {
	xxx_messageInfo_ChaincodeID.DiscardUnknown(m)
}

var xxx_messageInfo_ChaincodeID proto.InternalMessageInfo

func (m *ChaincodeID) GetPath() string {
	if m != nil {
		return m.Path
	}
	return ""
}

func (m *ChaincodeID) GetName() string {
	if m != nil {
		return m.Name
	}
	return ""
}

func (m *ChaincodeID) GetVersion() string {
	if m != nil {
		return m.Version
	}
	return ""
}

// Carries the chaincode function and its arguments.
// UnmarshalJSON in transaction.go converts the string-based REST/JSON input to
// the []byte-based current ChaincodeInput structure.
type ChaincodeInput struct {
	Args                 [][]byte          `protobuf:"bytes,1,rep,name=args,proto3" json:"args,omitempty"`
	Decorations          map[string][]byte `protobuf:"bytes,2,rep,name=decorations,proto3" json:"decorations,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	XXX_NoUnkeyedLiteral struct{}          `json:"-"`
	XXX_unrecognized     []byte            `json:"-"`
	XXX_sizecache        int32             `json:"-"`
}

func (m *ChaincodeInput) Reset()         { *m = ChaincodeInput{} }
func (m *ChaincodeInput) String() string { return proto.CompactTextString(m) }
func (*ChaincodeInput) ProtoMessage()    {}
func (*ChaincodeInput) Descriptor() ([]byte, []int) {
	return fileDescriptor_chaincode_4b693947e61b8a8b, []int{1}
}
func (m *ChaincodeInput) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ChaincodeInput.Unmarshal(m, b)
}
func (m *ChaincodeInput) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ChaincodeInput.Marshal(b, m, deterministic)
}
func (dst *ChaincodeInput) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ChaincodeInput.Merge(dst, src)
}
func (m *ChaincodeInput) XXX_Size() int {
	return xxx_messageInfo_ChaincodeInput.Size(m)
}
func (m *ChaincodeInput) XXX_DiscardUnknown() {
	xxx_messageInfo_ChaincodeInput.DiscardUnknown(m)
}

var xxx_messageInfo_ChaincodeInput proto.InternalMessageInfo

func (m *ChaincodeInput) GetArgs() [][]byte {
	if m != nil {
		return m.Args
	}
	return nil
}

func (m *ChaincodeInput) GetDecorations() map[string][]byte {
	if m != nil {
		return m.Decorations
	}
	return nil
}

// Carries the chaincode specification. This is the actual metadata required for
// defining a chaincode.
type ChaincodeSpec struct {
	Type                 ChaincodeSpec_Type `protobuf:"varint,1,opt,name=type,proto3,enum=protos.ChaincodeSpec_Type" json:"type,omitempty"`
	ChaincodeId          *ChaincodeID       `protobuf:"bytes,2,opt,name=chaincode_id,json=chaincodeId,proto3" json:"chaincode_id,omitempty"`
	Input                *ChaincodeInput    `protobuf:"bytes,3,opt,name=input,proto3" json:"input,omitempty"`
	Timeout              int32              `protobuf:"varint,4,opt,name=timeout,proto3" json:"timeout,omitempty"`
	XXX_NoUnkeyedLiteral struct{}           `json:"-"`
	XXX_unrecognized     []byte             `json:"-"`
	XXX_sizecache        int32              `json:"-"`
}

func (m *ChaincodeSpec) Reset()         { *m = ChaincodeSpec{} }
func (m *ChaincodeSpec) String() string { return proto.CompactTextString(m) }
func (*ChaincodeSpec) ProtoMessage()    {}
func (*ChaincodeSpec) Descriptor() ([]byte, []int) {
	return fileDescriptor_chaincode_4b693947e61b8a8b, []int{2}
}
func (m *ChaincodeSpec) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ChaincodeSpec.Unmarshal(m, b)
}
func (m *ChaincodeSpec) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ChaincodeSpec.Marshal(b, m, deterministic)
}
func (dst *ChaincodeSpec) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ChaincodeSpec.Merge(dst, src)
}
func (m *ChaincodeSpec) XXX_Size() int {
	return xxx_messageInfo_ChaincodeSpec.Size(m)
}
func (m *ChaincodeSpec) XXX_DiscardUnknown() {
	xxx_messageInfo_ChaincodeSpec.DiscardUnknown(m)
}

var xxx_messageInfo_ChaincodeSpec proto.InternalMessageInfo

func (m *ChaincodeSpec) GetType() ChaincodeSpec_Type {
	if m != nil {
		return m.Type
	}
	return ChaincodeSpec_UNDEFINED
}

func (m *ChaincodeSpec) GetChaincodeId() *ChaincodeID {
	if m != nil {
		return m.ChaincodeId
	}
	return nil
}

func (m *ChaincodeSpec) GetInput() *ChaincodeInput {
	if m != nil {
		return m.Input
	}
	return nil
}

func (m *ChaincodeSpec) GetTimeout() int32 {
	if m != nil {
		return m.Timeout
	}
	return 0
}

// Specify the deployment of a chaincode.
// TODO: Define `codePackage`.
type ChaincodeDeploymentSpec struct {
	ChaincodeSpec        *ChaincodeSpec                               `protobuf:"bytes,1,opt,name=chaincode_spec,json=chaincodeSpec,proto3" json:"chaincode_spec,omitempty"`
	CodePackage          []byte                                       `protobuf:"bytes,3,opt,name=code_package,json=codePackage,proto3" json:"code_package,omitempty"`
	ExecEnv              ChaincodeDeploymentSpec_ExecutionEnvironment `protobuf:"varint,4,opt,name=exec_env,json=execEnv,proto3,enum=protos.ChaincodeDeploymentSpec_ExecutionEnvironment" json:"exec_env,omitempty"`
	XXX_NoUnkeyedLiteral struct{}                                     `json:"-"`
	XXX_unrecognized     []byte                                       `json:"-"`
	XXX_sizecache        int32                                        `json:"-"`
}

func (m *ChaincodeDeploymentSpec) Reset()         { *m = ChaincodeDeploymentSpec{} }
func (m *ChaincodeDeploymentSpec) String() string { return proto.CompactTextString(m) }
func (*ChaincodeDeploymentSpec) ProtoMessage()    {}
func (*ChaincodeDeploymentSpec) Descriptor() ([]byte, []int) {
	return fileDescriptor_chaincode_4b693947e61b8a8b, []int{3}
}
func (m *ChaincodeDeploymentSpec) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ChaincodeDeploymentSpec.Unmarshal(m, b)
}
func (m *ChaincodeDeploymentSpec) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ChaincodeDeploymentSpec.Marshal(b, m, deterministic)
}
func (dst *ChaincodeDeploymentSpec) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ChaincodeDeploymentSpec.Merge(dst, src)
}
func (m *ChaincodeDeploymentSpec) XXX_Size() int {
	return xxx_messageInfo_ChaincodeDeploymentSpec.Size(m)
}
func (m *ChaincodeDeploymentSpec) XXX_DiscardUnknown() {
	xxx_messageInfo_ChaincodeDeploymentSpec.DiscardUnknown(m)
}

var xxx_messageInfo_ChaincodeDeploymentSpec proto.InternalMessageInfo

func (m *ChaincodeDeploymentSpec) GetChaincodeSpec() *ChaincodeSpec {
	if m != nil {
		return m.ChaincodeSpec
	}
	return nil
}

func (m *ChaincodeDeploymentSpec) GetCodePackage() []byte {
	if m != nil {
		return m.CodePackage
	}
	return nil
}

func (m *ChaincodeDeploymentSpec) GetExecEnv() ChaincodeDeploymentSpec_ExecutionEnvironment {
	if m != nil {
		return m.ExecEnv
	}
	return ChaincodeDeploymentSpec_DOCKER
}

// Carries the chaincode function and its arguments.
type ChaincodeInvocationSpec struct {
	ChaincodeSpec        *ChaincodeSpec `protobuf:"bytes,1,opt,name=chaincode_spec,json=chaincodeSpec,proto3" json:"chaincode_spec,omitempty"`
	XXX_NoUnkeyedLiteral struct{}       `json:"-"`
	XXX_unrecognized     []byte         `json:"-"`
	XXX_sizecache        int32          `json:"-"`
}

func (m *ChaincodeInvocationSpec) Reset()         { *m = ChaincodeInvocationSpec{} }
func (m *ChaincodeInvocationSpec) String() string { return proto.CompactTextString(m) }
func (*ChaincodeInvocationSpec) ProtoMessage()    {}
func (*ChaincodeInvocationSpec) Descriptor() ([]byte, []int) {
	return fileDescriptor_chaincode_4b693947e61b8a8b, []int{4}
}
func (m *ChaincodeInvocationSpec) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ChaincodeInvocationSpec.Unmarshal(m, b)
}
func (m *ChaincodeInvocationSpec) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ChaincodeInvocationSpec.Marshal(b, m, deterministic)
}
func (dst *ChaincodeInvocationSpec) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ChaincodeInvocationSpec.Merge(dst, src)
}
func (m *ChaincodeInvocationSpec) XXX_Size() int {
	return xxx_messageInfo_ChaincodeInvocationSpec.Size(m)
}
func (m *ChaincodeInvocationSpec) XXX_DiscardUnknown() {
	xxx_messageInfo_ChaincodeInvocationSpec.DiscardUnknown(m)
}

var xxx_messageInfo_ChaincodeInvocationSpec proto.InternalMessageInfo

func (m *ChaincodeInvocationSpec) GetChaincodeSpec() *ChaincodeSpec {
	if m != nil {
		return m.ChaincodeSpec
	}
	return nil
}

// LifecycleEvent is used as the payload of the chaincode event emitted by LSCC
type LifecycleEvent struct {
	ChaincodeName        string   `protobuf:"bytes,1,opt,name=chaincode_name,json=chaincodeName,proto3" json:"chaincode_name,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *LifecycleEvent) Reset()         { *m = LifecycleEvent{} }
func (m *LifecycleEvent) String() string { return proto.CompactTextString(m) }
func (*LifecycleEvent) ProtoMessage()    {}
func (*LifecycleEvent) Descriptor() ([]byte, []int) {
	return fileDescriptor_chaincode_4b693947e61b8a8b, []int{5}
}
func (m *LifecycleEvent) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_LifecycleEvent.Unmarshal(m, b)
}
func (m *LifecycleEvent) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_LifecycleEvent.Marshal(b, m, deterministic)
}
func (dst *LifecycleEvent) XXX_Merge(src proto.Message) {
	xxx_messageInfo_LifecycleEvent.Merge(dst, src)
}
func (m *LifecycleEvent) XXX_Size() int {
	return xxx_messageInfo_LifecycleEvent.Size(m)
}
func (m *LifecycleEvent) XXX_DiscardUnknown() {
	xxx_messageInfo_LifecycleEvent.DiscardUnknown(m)
}

var xxx_messageInfo_LifecycleEvent proto.InternalMessageInfo

func (m *LifecycleEvent) GetChaincodeName() string {
	if m != nil {
		return m.ChaincodeName
	}
	return ""
}

func init() {
	proto.RegisterType((*ChaincodeID)(nil), "protos.ChaincodeID")
	proto.RegisterType((*ChaincodeInput)(nil), "protos.ChaincodeInput")
	proto.RegisterMapType((map[string][]byte)(nil), "protos.ChaincodeInput.DecorationsEntry")
	proto.RegisterType((*ChaincodeSpec)(nil), "protos.ChaincodeSpec")
	proto.RegisterType((*ChaincodeDeploymentSpec)(nil), "protos.ChaincodeDeploymentSpec")
	proto.RegisterType((*ChaincodeInvocationSpec)(nil), "protos.ChaincodeInvocationSpec")
	proto.RegisterType((*LifecycleEvent)(nil), "protos.LifecycleEvent")
	proto.RegisterEnum("protos.ChaincodeSpec_Type", ChaincodeSpec_Type_name, ChaincodeSpec_Type_value)
	proto.RegisterEnum("protos.ChaincodeDeploymentSpec_ExecutionEnvironment", ChaincodeDeploymentSpec_ExecutionEnvironment_name, ChaincodeDeploymentSpec_ExecutionEnvironment_value)
}

func init() { proto.RegisterFile("peer/chaincode.proto", fileDescriptor_chaincode_4b693947e61b8a8b) }

var fileDescriptor_chaincode_4b693947e61b8a8b = []byte{
	// 588 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xa4, 0x54, 0x5f, 0x6f, 0xd3, 0x3e,
	0x14, 0xfd, 0xa5, 0xc9, 0xfe, 0xdd, 0x74, 0x55, 0x7e, 0x66, 0x40, 0xb4, 0xa7, 0x12, 0x09, 0x51,
	0x24, 0x94, 0x4a, 0x05, 0x01, 0x42, 0x68, 0x52, 0x59, 0xc2, 0xd4, 0x31, 0x5a, 0xe4, 0x0d, 0x24,
	0x78, 0xa9, 0x32, 0xe7, 0x36, 0x8d, 0xd6, 0x3a, 0x51, 0xe2, 0x46, 0xcb, 0xc7, 0xe0, 0x93, 0xf0,
	0x11, 0x41, 0xb6, 0xd7, 0x3f, 0x63, 0x7b, 0xe3, 0xa9, 0xd7, 0xd7, 0xc7, 0xe7, 0x9e, 0x73, 0xea,
	0x18, 0x0e, 0x72, 0xc4, 0xa2, 0xcb, 0xa6, 0x51, 0xca, 0x59, 0x16, 0xa3, 0x9f, 0x17, 0x99, 0xc8,
	0xc8, 0xb6, 0xfa, 0x29, 0xbd, 0x11, 0xd8, 0xc7, 0xcb, 0xad, 0x41, 0x40, 0x08, 0x58, 0x79, 0x24,
	0xa6, 0xae, 0xd1, 0x36, 0x3a, 0x7b, 0x54, 0xd5, 0xb2, 0xc7, 0xa3, 0x39, 0xba, 0x0d, 0xdd, 0x93,
	0x35, 0x71, 0x61, 0xa7, 0xc2, 0xa2, 0x4c, 0x33, 0xee, 0x9a, 0xaa, 0xbd, 0x5c, 0x7a, 0xbf, 0x0c,
	0x68, 0xad, 0x19, 0x79, 0xbe, 0x10, 0x92, 0x20, 0x2a, 0x92, 0xd2, 0x35, 0xda, 0x66, 0xa7, 0x49,
	0x55, 0x4d, 0x06, 0x60, 0xc7, 0xc8, 0xb2, 0x22, 0x12, 0x69, 0xc6, 0x4b, 0xb7, 0xd1, 0x36, 0x3b,
	0x76, 0xef, 0x99, 0x16, 0x57, 0xfa, 0xb7, 0x09, 0xfc, 0x60, 0x8d, 0x0c, 0xb9, 0x28, 0x6a, 0xba,
	0x79, 0xf6, 0xf0, 0x08, 0x9c, 0xbf, 0x01, 0xc4, 0x01, 0xf3, 0x0a, 0xeb, 0x1b, 0x1b, 0xb2, 0x24,
	0x07, 0xb0, 0x55, 0x45, 0xb3, 0x85, 0xb6, 0xd1, 0xa4, 0x7a, 0xf1, 0xae, 0xf1, 0xd6, 0xf0, 0x7e,
	0x1b, 0xb0, 0xbf, 0x1a, 0x78, 0x9e, 0x23, 0x23, 0x3e, 0x58, 0xa2, 0xce, 0x51, 0x1d, 0x6f, 0xf5,
	0x0e, 0xef, 0xa8, 0x92, 0x20, 0xff, 0xa2, 0xce, 0x91, 0x2a, 0x1c, 0x79, 0x0d, 0xcd, 0x55, 0xbe,
	0xe3, 0x34, 0x56, 0x23, 0xec, 0xde, 0x83, 0xbb, 0x6e, 0x02, 0x6a, 0xaf, 0x80, 0x83, 0x98, 0xbc,
	0x80, 0xad, 0x54, 0x1a, 0x54, 0x19, 0xda, 0xbd, 0x47, 0xf7, 0xdb, 0xa7, 0x1a, 0x24, 0x33, 0x17,
	0xe9, 0x1c, 0xb3, 0x85, 0x70, 0xad, 0xb6, 0xd1, 0xd9, 0xa2, 0xcb, 0xa5, 0x77, 0x04, 0x96, 0x54,
	0x43, 0xf6, 0x61, 0xef, 0xeb, 0x30, 0x08, 0x3f, 0x0e, 0x86, 0x61, 0xe0, 0xfc, 0x47, 0x00, 0xb6,
	0x4f, 0x46, 0x67, 0xfd, 0xe1, 0x89, 0x63, 0x90, 0x5d, 0xb0, 0x86, 0xa3, 0x20, 0x74, 0x1a, 0x64,
	0x07, 0xcc, 0xe3, 0x3e, 0x75, 0x4c, 0xd9, 0x3a, 0xed, 0x7f, 0xeb, 0x3b, 0x96, 0xf7, 0xb3, 0x01,
	0x8f, 0x57, 0x33, 0x03, 0xcc, 0x67, 0x59, 0x3d, 0x47, 0x2e, 0x54, 0x16, 0xef, 0xa1, 0xb5, 0xf6,
	0x56, 0xe6, 0xc8, 0x54, 0x2a, 0x76, 0xef, 0xe1, 0xbd, 0xa9, 0xd0, 0x7d, 0x76, 0x2b, 0xc9, 0x27,
	0xd0, 0x54, 0x07, 0xf3, 0x88, 0x5d, 0x45, 0x09, 0x2a, 0xa3, 0x4d, 0x6a, 0xcb, 0xde, 0x17, 0xdd,
	0x22, 0x23, 0xd8, 0xc5, 0x6b, 0x64, 0x63, 0xe4, 0x95, 0xf2, 0xd5, 0xea, 0xbd, 0xba, 0x43, 0x7d,
	0x5b, 0x93, 0x1f, 0x5e, 0x23, 0x5b, 0xc8, 0x7f, 0x3b, 0xe4, 0x55, 0x5a, 0x64, 0x5c, 0x6e, 0xd0,
	0x1d, 0xc9, 0x12, 0xf2, 0xca, 0xf3, 0xe1, 0xe0, 0x3e, 0x80, 0x8c, 0x23, 0x18, 0x1d, 0x7f, 0x0a,
	0xa9, 0x8e, 0xe6, 0xfc, 0xfb, 0xf9, 0x45, 0xf8, 0xd9, 0x31, 0x4e, 0xad, 0xdd, 0x86, 0x63, 0xd2,
	0x16, 0x4e, 0x26, 0xc8, 0x44, 0x5a, 0xe1, 0x38, 0x8e, 0x04, 0x7a, 0xf9, 0x46, 0x24, 0x03, 0x5e,
	0x65, 0x4c, 0x5d, 0xaf, 0x7f, 0x8f, 0xe4, 0x66, 0xdc, 0xff, 0x69, 0x3c, 0x4e, 0x90, 0xa3, 0xbe,
	0xb5, 0xe3, 0x68, 0x96, 0x78, 0x6f, 0xa0, 0x75, 0x96, 0x4e, 0x90, 0xd5, 0x6c, 0x86, 0x61, 0x25,
	0x15, 0x3f, 0xdd, 0x1c, 0xa4, 0xbe, 0x41, 0x7d, 0xa1, 0xd7, 0x8c, 0xc3, 0x68, 0x8e, 0x1f, 0x46,
	0xe0, 0x65, 0x45, 0xe2, 0x4f, 0xeb, 0x1c, 0x8b, 0x19, 0xc6, 0x09, 0x16, 0xfe, 0x24, 0xba, 0x2c,
	0x52, 0xb6, 0xd4, 0x23, 0x5f, 0x80, 0x1f, 0xcf, 0x93, 0x54, 0x4c, 0x17, 0x97, 0x3e, 0xcb, 0xe6,
	0xdd, 0x0d, 0x68, 0x57, 0x43, 0xbb, 0x1a, 0xda, 0x95, 0xd0, 0x4b, 0xfd, 0x38, 0xbc, 0xfc, 0x13,
	0x00, 0x00, 0xff, 0xff, 0xc9, 0xc0, 0x1a, 0x35, 0x3b, 0x04, 0x00, 0x00,
}
