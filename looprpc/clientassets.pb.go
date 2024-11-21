// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.31.0
// 	protoc        v3.6.1
// source: clientassets.proto

package looprpc

import (
	_ "github.com/lightninglabs/loop/swapserverrpc"
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

type SwapOutRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Amt   uint64 `protobuf:"varint,1,opt,name=amt,proto3" json:"amt,omitempty"`
	Asset []byte `protobuf:"bytes,2,opt,name=asset,proto3" json:"asset,omitempty"`
}

func (x *SwapOutRequest) Reset() {
	*x = SwapOutRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_clientassets_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SwapOutRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SwapOutRequest) ProtoMessage() {}

func (x *SwapOutRequest) ProtoReflect() protoreflect.Message {
	mi := &file_clientassets_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SwapOutRequest.ProtoReflect.Descriptor instead.
func (*SwapOutRequest) Descriptor() ([]byte, []int) {
	return file_clientassets_proto_rawDescGZIP(), []int{0}
}

func (x *SwapOutRequest) GetAmt() uint64 {
	if x != nil {
		return x.Amt
	}
	return 0
}

func (x *SwapOutRequest) GetAsset() []byte {
	if x != nil {
		return x.Asset
	}
	return nil
}

type SwapOutResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	SwapStatus *AssetSwapStatus `protobuf:"bytes,1,opt,name=swap_status,json=swapStatus,proto3" json:"swap_status,omitempty"`
}

func (x *SwapOutResponse) Reset() {
	*x = SwapOutResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_clientassets_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SwapOutResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SwapOutResponse) ProtoMessage() {}

func (x *SwapOutResponse) ProtoReflect() protoreflect.Message {
	mi := &file_clientassets_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SwapOutResponse.ProtoReflect.Descriptor instead.
func (*SwapOutResponse) Descriptor() ([]byte, []int) {
	return file_clientassets_proto_rawDescGZIP(), []int{1}
}

func (x *SwapOutResponse) GetSwapStatus() *AssetSwapStatus {
	if x != nil {
		return x.SwapStatus
	}
	return nil
}

type ListAssetSwapsRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *ListAssetSwapsRequest) Reset() {
	*x = ListAssetSwapsRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_clientassets_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ListAssetSwapsRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ListAssetSwapsRequest) ProtoMessage() {}

func (x *ListAssetSwapsRequest) ProtoReflect() protoreflect.Message {
	mi := &file_clientassets_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ListAssetSwapsRequest.ProtoReflect.Descriptor instead.
func (*ListAssetSwapsRequest) Descriptor() ([]byte, []int) {
	return file_clientassets_proto_rawDescGZIP(), []int{2}
}

type ListAssetSwapsResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	SwapStatus []*AssetSwapStatus `protobuf:"bytes,1,rep,name=swap_status,json=swapStatus,proto3" json:"swap_status,omitempty"`
}

func (x *ListAssetSwapsResponse) Reset() {
	*x = ListAssetSwapsResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_clientassets_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ListAssetSwapsResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ListAssetSwapsResponse) ProtoMessage() {}

func (x *ListAssetSwapsResponse) ProtoReflect() protoreflect.Message {
	mi := &file_clientassets_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ListAssetSwapsResponse.ProtoReflect.Descriptor instead.
func (*ListAssetSwapsResponse) Descriptor() ([]byte, []int) {
	return file_clientassets_proto_rawDescGZIP(), []int{3}
}

func (x *ListAssetSwapsResponse) GetSwapStatus() []*AssetSwapStatus {
	if x != nil {
		return x.SwapStatus
	}
	return nil
}

type AssetSwapStatus struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	SwapHash   []byte `protobuf:"bytes,1,opt,name=swap_hash,json=swapHash,proto3" json:"swap_hash,omitempty"`
	SwapStatus string `protobuf:"bytes,2,opt,name=swap_status,json=swapStatus,proto3" json:"swap_status,omitempty"`
}

func (x *AssetSwapStatus) Reset() {
	*x = AssetSwapStatus{}
	if protoimpl.UnsafeEnabled {
		mi := &file_clientassets_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *AssetSwapStatus) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*AssetSwapStatus) ProtoMessage() {}

func (x *AssetSwapStatus) ProtoReflect() protoreflect.Message {
	mi := &file_clientassets_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use AssetSwapStatus.ProtoReflect.Descriptor instead.
func (*AssetSwapStatus) Descriptor() ([]byte, []int) {
	return file_clientassets_proto_rawDescGZIP(), []int{4}
}

func (x *AssetSwapStatus) GetSwapHash() []byte {
	if x != nil {
		return x.SwapHash
	}
	return nil
}

func (x *AssetSwapStatus) GetSwapStatus() string {
	if x != nil {
		return x.SwapStatus
	}
	return ""
}

type ClientListAvailableAssetsRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *ClientListAvailableAssetsRequest) Reset() {
	*x = ClientListAvailableAssetsRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_clientassets_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ClientListAvailableAssetsRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ClientListAvailableAssetsRequest) ProtoMessage() {}

func (x *ClientListAvailableAssetsRequest) ProtoReflect() protoreflect.Message {
	mi := &file_clientassets_proto_msgTypes[5]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ClientListAvailableAssetsRequest.ProtoReflect.Descriptor instead.
func (*ClientListAvailableAssetsRequest) Descriptor() ([]byte, []int) {
	return file_clientassets_proto_rawDescGZIP(), []int{5}
}

type ClientListAvailableAssetsResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	AvailableAssets []*Asset `protobuf:"bytes,1,rep,name=available_assets,json=availableAssets,proto3" json:"available_assets,omitempty"`
}

func (x *ClientListAvailableAssetsResponse) Reset() {
	*x = ClientListAvailableAssetsResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_clientassets_proto_msgTypes[6]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ClientListAvailableAssetsResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ClientListAvailableAssetsResponse) ProtoMessage() {}

func (x *ClientListAvailableAssetsResponse) ProtoReflect() protoreflect.Message {
	mi := &file_clientassets_proto_msgTypes[6]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ClientListAvailableAssetsResponse.ProtoReflect.Descriptor instead.
func (*ClientListAvailableAssetsResponse) Descriptor() ([]byte, []int) {
	return file_clientassets_proto_rawDescGZIP(), []int{6}
}

func (x *ClientListAvailableAssetsResponse) GetAvailableAssets() []*Asset {
	if x != nil {
		return x.AvailableAssets
	}
	return nil
}

type Asset struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	AssetId     []byte `protobuf:"bytes,1,opt,name=asset_id,json=assetId,proto3" json:"asset_id,omitempty"`
	Name        string `protobuf:"bytes,2,opt,name=name,proto3" json:"name,omitempty"`
	SatsPerUnit uint64 `protobuf:"varint,3,opt,name=sats_per_unit,json=satsPerUnit,proto3" json:"sats_per_unit,omitempty"`
}

func (x *Asset) Reset() {
	*x = Asset{}
	if protoimpl.UnsafeEnabled {
		mi := &file_clientassets_proto_msgTypes[7]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Asset) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Asset) ProtoMessage() {}

func (x *Asset) ProtoReflect() protoreflect.Message {
	mi := &file_clientassets_proto_msgTypes[7]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Asset.ProtoReflect.Descriptor instead.
func (*Asset) Descriptor() ([]byte, []int) {
	return file_clientassets_proto_rawDescGZIP(), []int{7}
}

func (x *Asset) GetAssetId() []byte {
	if x != nil {
		return x.AssetId
	}
	return nil
}

func (x *Asset) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *Asset) GetSatsPerUnit() uint64 {
	if x != nil {
		return x.SatsPerUnit
	}
	return 0
}

type ClientGetAssetSwapOutQuoteRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Amt   uint64 `protobuf:"varint,1,opt,name=amt,proto3" json:"amt,omitempty"`
	Asset []byte `protobuf:"bytes,2,opt,name=asset,proto3" json:"asset,omitempty"`
}

func (x *ClientGetAssetSwapOutQuoteRequest) Reset() {
	*x = ClientGetAssetSwapOutQuoteRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_clientassets_proto_msgTypes[8]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ClientGetAssetSwapOutQuoteRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ClientGetAssetSwapOutQuoteRequest) ProtoMessage() {}

func (x *ClientGetAssetSwapOutQuoteRequest) ProtoReflect() protoreflect.Message {
	mi := &file_clientassets_proto_msgTypes[8]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ClientGetAssetSwapOutQuoteRequest.ProtoReflect.Descriptor instead.
func (*ClientGetAssetSwapOutQuoteRequest) Descriptor() ([]byte, []int) {
	return file_clientassets_proto_rawDescGZIP(), []int{8}
}

func (x *ClientGetAssetSwapOutQuoteRequest) GetAmt() uint64 {
	if x != nil {
		return x.Amt
	}
	return 0
}

func (x *ClientGetAssetSwapOutQuoteRequest) GetAsset() []byte {
	if x != nil {
		return x.Asset
	}
	return nil
}

type ClientGetAssetSwapOutQuoteResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	SwapFee     float64 `protobuf:"fixed64,1,opt,name=swap_fee,json=swapFee,proto3" json:"swap_fee,omitempty"`
	PrepayAmt   uint64  `protobuf:"varint,2,opt,name=prepay_amt,json=prepayAmt,proto3" json:"prepay_amt,omitempty"`
	SatsPerUnit uint64  `protobuf:"varint,3,opt,name=sats_per_unit,json=satsPerUnit,proto3" json:"sats_per_unit,omitempty"`
}

func (x *ClientGetAssetSwapOutQuoteResponse) Reset() {
	*x = ClientGetAssetSwapOutQuoteResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_clientassets_proto_msgTypes[9]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ClientGetAssetSwapOutQuoteResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ClientGetAssetSwapOutQuoteResponse) ProtoMessage() {}

func (x *ClientGetAssetSwapOutQuoteResponse) ProtoReflect() protoreflect.Message {
	mi := &file_clientassets_proto_msgTypes[9]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ClientGetAssetSwapOutQuoteResponse.ProtoReflect.Descriptor instead.
func (*ClientGetAssetSwapOutQuoteResponse) Descriptor() ([]byte, []int) {
	return file_clientassets_proto_rawDescGZIP(), []int{9}
}

func (x *ClientGetAssetSwapOutQuoteResponse) GetSwapFee() float64 {
	if x != nil {
		return x.SwapFee
	}
	return 0
}

func (x *ClientGetAssetSwapOutQuoteResponse) GetPrepayAmt() uint64 {
	if x != nil {
		return x.PrepayAmt
	}
	return 0
}

func (x *ClientGetAssetSwapOutQuoteResponse) GetSatsPerUnit() uint64 {
	if x != nil {
		return x.SatsPerUnit
	}
	return 0
}

var File_clientassets_proto protoreflect.FileDescriptor

var file_clientassets_proto_rawDesc = []byte{
	0x0a, 0x12, 0x63, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x61, 0x73, 0x73, 0x65, 0x74, 0x73, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x12, 0x07, 0x6c, 0x6f, 0x6f, 0x70, 0x72, 0x70, 0x63, 0x1a, 0x1a, 0x73,
	0x77, 0x61, 0x70, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x72, 0x70, 0x63, 0x2f, 0x63, 0x6f, 0x6d,
	0x6d, 0x6f, 0x6e, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x38, 0x0a, 0x0e, 0x53, 0x77, 0x61,
	0x70, 0x4f, 0x75, 0x74, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x10, 0x0a, 0x03, 0x61,
	0x6d, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x04, 0x52, 0x03, 0x61, 0x6d, 0x74, 0x12, 0x14, 0x0a,
	0x05, 0x61, 0x73, 0x73, 0x65, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x05, 0x61, 0x73,
	0x73, 0x65, 0x74, 0x22, 0x4c, 0x0a, 0x0f, 0x53, 0x77, 0x61, 0x70, 0x4f, 0x75, 0x74, 0x52, 0x65,
	0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x39, 0x0a, 0x0b, 0x73, 0x77, 0x61, 0x70, 0x5f, 0x73,
	0x74, 0x61, 0x74, 0x75, 0x73, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x18, 0x2e, 0x6c, 0x6f,
	0x6f, 0x70, 0x72, 0x70, 0x63, 0x2e, 0x41, 0x73, 0x73, 0x65, 0x74, 0x53, 0x77, 0x61, 0x70, 0x53,
	0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x0a, 0x73, 0x77, 0x61, 0x70, 0x53, 0x74, 0x61, 0x74, 0x75,
	0x73, 0x22, 0x17, 0x0a, 0x15, 0x4c, 0x69, 0x73, 0x74, 0x41, 0x73, 0x73, 0x65, 0x74, 0x53, 0x77,
	0x61, 0x70, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x22, 0x53, 0x0a, 0x16, 0x4c, 0x69,
	0x73, 0x74, 0x41, 0x73, 0x73, 0x65, 0x74, 0x53, 0x77, 0x61, 0x70, 0x73, 0x52, 0x65, 0x73, 0x70,
	0x6f, 0x6e, 0x73, 0x65, 0x12, 0x39, 0x0a, 0x0b, 0x73, 0x77, 0x61, 0x70, 0x5f, 0x73, 0x74, 0x61,
	0x74, 0x75, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x18, 0x2e, 0x6c, 0x6f, 0x6f, 0x70,
	0x72, 0x70, 0x63, 0x2e, 0x41, 0x73, 0x73, 0x65, 0x74, 0x53, 0x77, 0x61, 0x70, 0x53, 0x74, 0x61,
	0x74, 0x75, 0x73, 0x52, 0x0a, 0x73, 0x77, 0x61, 0x70, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x22,
	0x4f, 0x0a, 0x0f, 0x41, 0x73, 0x73, 0x65, 0x74, 0x53, 0x77, 0x61, 0x70, 0x53, 0x74, 0x61, 0x74,
	0x75, 0x73, 0x12, 0x1b, 0x0a, 0x09, 0x73, 0x77, 0x61, 0x70, 0x5f, 0x68, 0x61, 0x73, 0x68, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x08, 0x73, 0x77, 0x61, 0x70, 0x48, 0x61, 0x73, 0x68, 0x12,
	0x1f, 0x0a, 0x0b, 0x73, 0x77, 0x61, 0x70, 0x5f, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x18, 0x02,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x0a, 0x73, 0x77, 0x61, 0x70, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73,
	0x22, 0x22, 0x0a, 0x20, 0x43, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x4c, 0x69, 0x73, 0x74, 0x41, 0x76,
	0x61, 0x69, 0x6c, 0x61, 0x62, 0x6c, 0x65, 0x41, 0x73, 0x73, 0x65, 0x74, 0x73, 0x52, 0x65, 0x71,
	0x75, 0x65, 0x73, 0x74, 0x22, 0x5e, 0x0a, 0x21, 0x43, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x4c, 0x69,
	0x73, 0x74, 0x41, 0x76, 0x61, 0x69, 0x6c, 0x61, 0x62, 0x6c, 0x65, 0x41, 0x73, 0x73, 0x65, 0x74,
	0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x39, 0x0a, 0x10, 0x61, 0x76, 0x61,
	0x69, 0x6c, 0x61, 0x62, 0x6c, 0x65, 0x5f, 0x61, 0x73, 0x73, 0x65, 0x74, 0x73, 0x18, 0x01, 0x20,
	0x03, 0x28, 0x0b, 0x32, 0x0e, 0x2e, 0x6c, 0x6f, 0x6f, 0x70, 0x72, 0x70, 0x63, 0x2e, 0x41, 0x73,
	0x73, 0x65, 0x74, 0x52, 0x0f, 0x61, 0x76, 0x61, 0x69, 0x6c, 0x61, 0x62, 0x6c, 0x65, 0x41, 0x73,
	0x73, 0x65, 0x74, 0x73, 0x22, 0x5a, 0x0a, 0x05, 0x41, 0x73, 0x73, 0x65, 0x74, 0x12, 0x19, 0x0a,
	0x08, 0x61, 0x73, 0x73, 0x65, 0x74, 0x5f, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0c, 0x52,
	0x07, 0x61, 0x73, 0x73, 0x65, 0x74, 0x49, 0x64, 0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65,
	0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x22, 0x0a, 0x0d,
	0x73, 0x61, 0x74, 0x73, 0x5f, 0x70, 0x65, 0x72, 0x5f, 0x75, 0x6e, 0x69, 0x74, 0x18, 0x03, 0x20,
	0x01, 0x28, 0x04, 0x52, 0x0b, 0x73, 0x61, 0x74, 0x73, 0x50, 0x65, 0x72, 0x55, 0x6e, 0x69, 0x74,
	0x22, 0x4b, 0x0a, 0x21, 0x43, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x47, 0x65, 0x74, 0x41, 0x73, 0x73,
	0x65, 0x74, 0x53, 0x77, 0x61, 0x70, 0x4f, 0x75, 0x74, 0x51, 0x75, 0x6f, 0x74, 0x65, 0x52, 0x65,
	0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x10, 0x0a, 0x03, 0x61, 0x6d, 0x74, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x04, 0x52, 0x03, 0x61, 0x6d, 0x74, 0x12, 0x14, 0x0a, 0x05, 0x61, 0x73, 0x73, 0x65, 0x74,
	0x18, 0x02, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x05, 0x61, 0x73, 0x73, 0x65, 0x74, 0x22, 0x82, 0x01,
	0x0a, 0x22, 0x43, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x47, 0x65, 0x74, 0x41, 0x73, 0x73, 0x65, 0x74,
	0x53, 0x77, 0x61, 0x70, 0x4f, 0x75, 0x74, 0x51, 0x75, 0x6f, 0x74, 0x65, 0x52, 0x65, 0x73, 0x70,
	0x6f, 0x6e, 0x73, 0x65, 0x12, 0x19, 0x0a, 0x08, 0x73, 0x77, 0x61, 0x70, 0x5f, 0x66, 0x65, 0x65,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x01, 0x52, 0x07, 0x73, 0x77, 0x61, 0x70, 0x46, 0x65, 0x65, 0x12,
	0x1d, 0x0a, 0x0a, 0x70, 0x72, 0x65, 0x70, 0x61, 0x79, 0x5f, 0x61, 0x6d, 0x74, 0x18, 0x02, 0x20,
	0x01, 0x28, 0x04, 0x52, 0x09, 0x70, 0x72, 0x65, 0x70, 0x61, 0x79, 0x41, 0x6d, 0x74, 0x12, 0x22,
	0x0a, 0x0d, 0x73, 0x61, 0x74, 0x73, 0x5f, 0x70, 0x65, 0x72, 0x5f, 0x75, 0x6e, 0x69, 0x74, 0x18,
	0x03, 0x20, 0x01, 0x28, 0x04, 0x52, 0x0b, 0x73, 0x61, 0x74, 0x73, 0x50, 0x65, 0x72, 0x55, 0x6e,
	0x69, 0x74, 0x32, 0x8a, 0x03, 0x0a, 0x0c, 0x41, 0x73, 0x73, 0x65, 0x74, 0x73, 0x43, 0x6c, 0x69,
	0x65, 0x6e, 0x74, 0x12, 0x3c, 0x0a, 0x07, 0x53, 0x77, 0x61, 0x70, 0x4f, 0x75, 0x74, 0x12, 0x17,
	0x2e, 0x6c, 0x6f, 0x6f, 0x70, 0x72, 0x70, 0x63, 0x2e, 0x53, 0x77, 0x61, 0x70, 0x4f, 0x75, 0x74,
	0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x18, 0x2e, 0x6c, 0x6f, 0x6f, 0x70, 0x72, 0x70,
	0x63, 0x2e, 0x53, 0x77, 0x61, 0x70, 0x4f, 0x75, 0x74, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73,
	0x65, 0x12, 0x51, 0x0a, 0x0e, 0x4c, 0x69, 0x73, 0x74, 0x41, 0x73, 0x73, 0x65, 0x74, 0x53, 0x77,
	0x61, 0x70, 0x73, 0x12, 0x1e, 0x2e, 0x6c, 0x6f, 0x6f, 0x70, 0x72, 0x70, 0x63, 0x2e, 0x4c, 0x69,
	0x73, 0x74, 0x41, 0x73, 0x73, 0x65, 0x74, 0x53, 0x77, 0x61, 0x70, 0x73, 0x52, 0x65, 0x71, 0x75,
	0x65, 0x73, 0x74, 0x1a, 0x1f, 0x2e, 0x6c, 0x6f, 0x6f, 0x70, 0x72, 0x70, 0x63, 0x2e, 0x4c, 0x69,
	0x73, 0x74, 0x41, 0x73, 0x73, 0x65, 0x74, 0x53, 0x77, 0x61, 0x70, 0x73, 0x52, 0x65, 0x73, 0x70,
	0x6f, 0x6e, 0x73, 0x65, 0x12, 0x72, 0x0a, 0x19, 0x43, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x4c, 0x69,
	0x73, 0x74, 0x41, 0x76, 0x61, 0x69, 0x6c, 0x61, 0x62, 0x6c, 0x65, 0x41, 0x73, 0x73, 0x65, 0x74,
	0x73, 0x12, 0x29, 0x2e, 0x6c, 0x6f, 0x6f, 0x70, 0x72, 0x70, 0x63, 0x2e, 0x43, 0x6c, 0x69, 0x65,
	0x6e, 0x74, 0x4c, 0x69, 0x73, 0x74, 0x41, 0x76, 0x61, 0x69, 0x6c, 0x61, 0x62, 0x6c, 0x65, 0x41,
	0x73, 0x73, 0x65, 0x74, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x2a, 0x2e, 0x6c,
	0x6f, 0x6f, 0x70, 0x72, 0x70, 0x63, 0x2e, 0x43, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x4c, 0x69, 0x73,
	0x74, 0x41, 0x76, 0x61, 0x69, 0x6c, 0x61, 0x62, 0x6c, 0x65, 0x41, 0x73, 0x73, 0x65, 0x74, 0x73,
	0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x75, 0x0a, 0x1a, 0x43, 0x6c, 0x69, 0x65,
	0x6e, 0x74, 0x47, 0x65, 0x74, 0x41, 0x73, 0x73, 0x65, 0x74, 0x53, 0x77, 0x61, 0x70, 0x4f, 0x75,
	0x74, 0x51, 0x75, 0x6f, 0x74, 0x65, 0x12, 0x2a, 0x2e, 0x6c, 0x6f, 0x6f, 0x70, 0x72, 0x70, 0x63,
	0x2e, 0x43, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x47, 0x65, 0x74, 0x41, 0x73, 0x73, 0x65, 0x74, 0x53,
	0x77, 0x61, 0x70, 0x4f, 0x75, 0x74, 0x51, 0x75, 0x6f, 0x74, 0x65, 0x52, 0x65, 0x71, 0x75, 0x65,
	0x73, 0x74, 0x1a, 0x2b, 0x2e, 0x6c, 0x6f, 0x6f, 0x70, 0x72, 0x70, 0x63, 0x2e, 0x43, 0x6c, 0x69,
	0x65, 0x6e, 0x74, 0x47, 0x65, 0x74, 0x41, 0x73, 0x73, 0x65, 0x74, 0x53, 0x77, 0x61, 0x70, 0x4f,
	0x75, 0x74, 0x51, 0x75, 0x6f, 0x74, 0x65, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x42,
	0x27, 0x5a, 0x25, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x6c, 0x69,
	0x67, 0x68, 0x74, 0x6e, 0x69, 0x6e, 0x67, 0x6c, 0x61, 0x62, 0x73, 0x2f, 0x6c, 0x6f, 0x6f, 0x70,
	0x2f, 0x6c, 0x6f, 0x6f, 0x70, 0x72, 0x70, 0x63, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_clientassets_proto_rawDescOnce sync.Once
	file_clientassets_proto_rawDescData = file_clientassets_proto_rawDesc
)

func file_clientassets_proto_rawDescGZIP() []byte {
	file_clientassets_proto_rawDescOnce.Do(func() {
		file_clientassets_proto_rawDescData = protoimpl.X.CompressGZIP(file_clientassets_proto_rawDescData)
	})
	return file_clientassets_proto_rawDescData
}

var file_clientassets_proto_msgTypes = make([]protoimpl.MessageInfo, 10)
var file_clientassets_proto_goTypes = []interface{}{
	(*SwapOutRequest)(nil),                     // 0: looprpc.SwapOutRequest
	(*SwapOutResponse)(nil),                    // 1: looprpc.SwapOutResponse
	(*ListAssetSwapsRequest)(nil),              // 2: looprpc.ListAssetSwapsRequest
	(*ListAssetSwapsResponse)(nil),             // 3: looprpc.ListAssetSwapsResponse
	(*AssetSwapStatus)(nil),                    // 4: looprpc.AssetSwapStatus
	(*ClientListAvailableAssetsRequest)(nil),   // 5: looprpc.ClientListAvailableAssetsRequest
	(*ClientListAvailableAssetsResponse)(nil),  // 6: looprpc.ClientListAvailableAssetsResponse
	(*Asset)(nil),                              // 7: looprpc.Asset
	(*ClientGetAssetSwapOutQuoteRequest)(nil),  // 8: looprpc.ClientGetAssetSwapOutQuoteRequest
	(*ClientGetAssetSwapOutQuoteResponse)(nil), // 9: looprpc.ClientGetAssetSwapOutQuoteResponse
}
var file_clientassets_proto_depIdxs = []int32{
	4, // 0: looprpc.SwapOutResponse.swap_status:type_name -> looprpc.AssetSwapStatus
	4, // 1: looprpc.ListAssetSwapsResponse.swap_status:type_name -> looprpc.AssetSwapStatus
	7, // 2: looprpc.ClientListAvailableAssetsResponse.available_assets:type_name -> looprpc.Asset
	0, // 3: looprpc.AssetsClient.SwapOut:input_type -> looprpc.SwapOutRequest
	2, // 4: looprpc.AssetsClient.ListAssetSwaps:input_type -> looprpc.ListAssetSwapsRequest
	5, // 5: looprpc.AssetsClient.ClientListAvailableAssets:input_type -> looprpc.ClientListAvailableAssetsRequest
	8, // 6: looprpc.AssetsClient.ClientGetAssetSwapOutQuote:input_type -> looprpc.ClientGetAssetSwapOutQuoteRequest
	1, // 7: looprpc.AssetsClient.SwapOut:output_type -> looprpc.SwapOutResponse
	3, // 8: looprpc.AssetsClient.ListAssetSwaps:output_type -> looprpc.ListAssetSwapsResponse
	6, // 9: looprpc.AssetsClient.ClientListAvailableAssets:output_type -> looprpc.ClientListAvailableAssetsResponse
	9, // 10: looprpc.AssetsClient.ClientGetAssetSwapOutQuote:output_type -> looprpc.ClientGetAssetSwapOutQuoteResponse
	7, // [7:11] is the sub-list for method output_type
	3, // [3:7] is the sub-list for method input_type
	3, // [3:3] is the sub-list for extension type_name
	3, // [3:3] is the sub-list for extension extendee
	0, // [0:3] is the sub-list for field type_name
}

func init() { file_clientassets_proto_init() }
func file_clientassets_proto_init() {
	if File_clientassets_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_clientassets_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*SwapOutRequest); i {
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
		file_clientassets_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*SwapOutResponse); i {
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
		file_clientassets_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ListAssetSwapsRequest); i {
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
		file_clientassets_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ListAssetSwapsResponse); i {
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
		file_clientassets_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*AssetSwapStatus); i {
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
		file_clientassets_proto_msgTypes[5].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ClientListAvailableAssetsRequest); i {
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
		file_clientassets_proto_msgTypes[6].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ClientListAvailableAssetsResponse); i {
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
		file_clientassets_proto_msgTypes[7].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Asset); i {
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
		file_clientassets_proto_msgTypes[8].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ClientGetAssetSwapOutQuoteRequest); i {
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
		file_clientassets_proto_msgTypes[9].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ClientGetAssetSwapOutQuoteResponse); i {
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
			RawDescriptor: file_clientassets_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   10,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_clientassets_proto_goTypes,
		DependencyIndexes: file_clientassets_proto_depIdxs,
		MessageInfos:      file_clientassets_proto_msgTypes,
	}.Build()
	File_clientassets_proto = out.File
	file_clientassets_proto_rawDesc = nil
	file_clientassets_proto_goTypes = nil
	file_clientassets_proto_depIdxs = nil
}
