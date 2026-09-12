package main

import (
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/runtime/protoimpl"
	"google.golang.org/protobuf/types/descriptorpb"
)

// Custom struct tag extensions, declared in the vendored
// zara/customtag/options.proto (package zara.options). The extension
// definitions are hand-written here so the generator has no dependency on a
// separate options package; the extension numbers (50001/50002) and the
// fully-qualified names must match the .proto file.
var (
	// E_Tags is the (zara.options.tags) field option: the full struct tag
	// literal for the field, e.g. `xml:"flag,attr" gorm:"primaryKey"`.
	E_Tags = &protoimpl.ExtensionInfo{
		ExtendedType:  (*descriptorpb.FieldOptions)(nil),
		ExtensionType: (*string)(nil),
		Field:         50001,
		Name:          "zara.options.tags",
		Tag:           "bytes,50001,opt,name=tags",
		Filename:      "zara/customtag/options.proto",
	}

	// E_OneofTags is the (zara.options.oneof_tags) oneof option: the full
	// struct tag literal for the oneof field on the message struct.
	E_OneofTags = &protoimpl.ExtensionInfo{
		ExtendedType:  (*descriptorpb.OneofOptions)(nil),
		ExtensionType: (*string)(nil),
		Field:         50002,
		Name:          "zara.options.oneof_tags",
		Tag:           "bytes,50002,opt,name=oneof_tags",
		Filename:      "zara/customtag/options.proto",
	}
)

func init() {
	// Register the extensions so proto.HasExtension/GetExtension resolve
	// them when the descriptor options are unmarshaled. protobuf-go's
	// generated code registers extensions through protoimpl.TypeBuilder;
	// here the definitions are hand-written, so register directly.
	protoregistry.GlobalTypes.RegisterExtension(E_Tags)
	protoregistry.GlobalTypes.RegisterExtension(E_OneofTags)
}