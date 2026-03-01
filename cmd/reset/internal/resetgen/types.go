package resetgen

type ResetTarget struct {
	PkgPath string
	PkgName string
	Name    string
	Fields  []StructField
}

type StructField struct {
	Name string
	Type string
	Kind FieldKind
}

type FieldKind int

const (
	KindPrimitive FieldKind = iota
	KindPointer
	KindSlice
	KindMap
	KindStruct
	KindUnknown
	KindEmbeddedStruct
	KindNamedStruct
)
