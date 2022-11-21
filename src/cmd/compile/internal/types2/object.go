// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package types2

import (
	"bytes"
	"cmd/compile/internal/syntax"
	"fmt"
	"go/constant"
	"unicode"
	"unicode/utf8"
)

// An Object describes a named language entity such as a package,
// constant, type, variable, function (incl. methods), or label.
// All objects implement the Object interface.
// AST上的每一个节点都对应一个Object对应
type Object interface {
	Parent() *Scope  // 对象所在作用域 scope in which this object is declared; nil for methods and struct fields
	Pos() syntax.Pos // 对象在声明中的位置 position of object identifier in declaration
	Pkg() *Package   // 对象属于的包 package to which this object belongs; nil for labels and objects in the Universe scope
	Name() string    // package local object name
	Type() Type      // 对象的类型，最重要的东西 object type
	Exported() bool  // 对象是否是public的 reports whether the name starts with a capital letter
	Id() string      // object是public的话就是对象名字，否则是qualified名（包名+"."+obj 的名字） object name if exported, qualified name if not exported (see func Id)

	// String returns a human-readable string of the object.
	String() string

	// order reflects a package-level object's source order: if object
	// a is before object b in the source, then a.order() < b.order().
	// order returns a value > 0 for package-level objects; it returns
	// 0 for all other objects (including objects in file scopes).
	// 对象在源代码中的顺序，order越小，说明顺序越靠前。包级别的对象order>0,其它所有的对象返回0
	order() uint32

	// color returns the object's color.
	color() color

	// setType sets the type of the object.
	setType(Type)

	// setOrder sets the order number of the object. It must be > 0.
	setOrder(uint32)

	// setColor sets the object's color. It must not be white.
	setColor(color color)

	// setParent sets the parent scope of the object.
	setParent(*Scope)

	// sameId reports whether obj.Id() and Id(pkg, name) are the same.
	// 判断俩id（该对象的Id方法，和外边的ID函数返回值）是否相同
	sameId(pkg *Package, name string) bool

	// scopePos returns the start position of the scope of this Object
	// 对象作用域的起始位置
	scopePos() syntax.Pos

	// setScopePos sets the start position of the scope for this Object.
	setScopePos(pos syntax.Pos)
}

// 是不是导出的字段
func isExported(name string) bool {
	ch, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(ch)
}

// Id returns name if it is exported, otherwise it
// returns the name qualified with the package path.
// object是public的话就是对象名字，否则是qualified名（包名+"."+obj 的名字）
func Id(pkg *Package, name string) string {
	if isExported(name) {
		return name
	}
	// unexported names need the package path for differentiation
	// (if there's no package, make sure we don't start with '.'
	// as that may change the order of methods between a setup
	// inside a package and outside a package - which breaks some
	// tests)
	path := "_"
	// pkg is nil for objects in Universe scope and possibly types
	// introduced via Eval (see also comment in object.sameId)
	if pkg != nil && pkg.path != "" {
		path = pkg.path
	}
	return path + "." + name
}

// An object implements the common parts of an Object.
// Object接口实现的通用部分，所有object的实现都会内嵌这个类型
type object struct {
	parent    *Scope
	pos       syntax.Pos
	pkg       *Package
	name      string
	typ       Type
	order_    uint32
	color_    color
	scopePos_ syntax.Pos
}

// color encodes the color of an object (see Checker.objDecl for details).
// 对象循环依赖检查 ，就是GC呗
type color uint32

// An object may be painted in one of three colors.
// Color values other than white or black are considered grey.
const (
	white color = iota
	black
	grey // must be > white and black
)

func (c color) String() string {
	switch c {
	case white:
		return "white"
	case black:
		return "black"
	default:
		return "grey"
	}
}

// colorFor returns the (initial) color for an object depending on
// whether its type t is known or not.
// 根据type返回object的初始颜色
func colorFor(t Type) color {
	if t != nil {
		return black
	}
	return white
}

// Parent returns the scope in which the object is declared.
// The result is nil for methods and struct fields.
func (obj *object) Parent() *Scope { return obj.parent }

// Pos returns the declaration position of the object's identifier.
func (obj *object) Pos() syntax.Pos { return obj.pos }

// Pkg returns the package to which the object belongs.
// The result is nil for labels and objects in the Universe scope.
func (obj *object) Pkg() *Package { return obj.pkg }

// Name returns the object's (package-local, unqualified) name.
func (obj *object) Name() string { return obj.name }

// Type returns the object's type.
func (obj *object) Type() Type { return obj.typ }

// Exported reports whether the object is exported (starts with a capital letter).
// It doesn't take into account whether the object is in a local (function) scope
// or not.
func (obj *object) Exported() bool { return isExported(obj.name) }

// Id is a wrapper for Id(obj.Pkg(), obj.Name()).
func (obj *object) Id() string { return Id(obj.pkg, obj.name) }

func (obj *object) String() string       { panic("abstract") }
func (obj *object) order() uint32        { return obj.order_ }
func (obj *object) color() color         { return obj.color_ }
func (obj *object) scopePos() syntax.Pos { return obj.scopePos_ }

func (obj *object) setParent(parent *Scope)    { obj.parent = parent }
func (obj *object) setType(typ Type)           { obj.typ = typ }
func (obj *object) setOrder(order uint32)      { assert(order > 0); obj.order_ = order }
func (obj *object) setColor(color color)       { assert(color != white); obj.color_ = color }
func (obj *object) setScopePos(pos syntax.Pos) { obj.scopePos_ = pos }

func (obj *object) sameId(pkg *Package, name string) bool {
	// spec:
	// "Two identifiers are different if they are spelled differently,
	// or if they appear in different packages and are not exported.
	// Otherwise, they are the same."
	if name != obj.name {
		return false
	}
	// obj.Name == name
	if obj.Exported() {
		return true
	}
	// not exported, so packages must be the same (pkg == nil for
	// fields in Universe scope; this can only happen for types
	// introduced via Eval)
	if pkg == nil || obj.pkg == nil {
		return pkg == obj.pkg
	}
	// pkg != nil && obj.pkg != nil
	return pkg.path == obj.pkg.path
}

// less reports whether object a is ordered before object b.
//
// Objects are ordered nil before non-nil, exported before
// non-exported, then by name, and finally (for non-exported
// functions) by package height and path.
// 对象a是否在对象b之前声明
func (a *object) less(b *object) bool {
	if a == b {
		return false
	}

	// Nil before non-nil.
	if a == nil {
		return true
	}
	if b == nil {
		return false
	}

	// Exported functions before non-exported.
	ea := isExported(a.name)
	eb := isExported(b.name)
	if ea != eb {
		return ea
	}

	// Order by name and then (for non-exported names) by package.
	if a.name != b.name {
		return a.name < b.name
	}
	if !ea {
		if a.pkg.height != b.pkg.height {
			return a.pkg.height < b.pkg.height
		}
		return a.pkg.path < b.pkg.path
	}

	return false
}

// A PkgName represents an imported Go package.
// PkgNames don't have a type.
// 实现了object接口，代表一个被导入的go包，它没有类型。就是import后面那个
type PkgName struct {
	object
	imported *Package
	used     bool // set if the package was used
}

// NewPkgName returns a new PkgName object representing an imported package.
// The remaining arguments set the attributes found with all Objects.
func NewPkgName(pos syntax.Pos, pkg *Package, name string, imported *Package) *PkgName {
	return &PkgName{object{nil, pos, pkg, name, Typ[Invalid], 0, black, nopos}, imported, false}
}

// Imported returns the package that was imported.
// It is distinct from Pkg(), which is the package containing the import statement.
// 返回被导入的go包
func (obj *PkgName) Imported() *Package { return obj.imported }

// A Const represents a declared constant.
// 实现了Object接口，常量声明语句
type Const struct {
	object
	val constant.Value
}

// NewConst returns a new constant with value val.
// The remaining arguments set the attributes found with all Objects.
func NewConst(pos syntax.Pos, pkg *Package, name string, typ Type, val constant.Value) *Const {
	return &Const{object{nil, pos, pkg, name, typ, 0, colorFor(typ), nopos}, val}
}

// Val returns the constant's value.
// 返回常量值
func (obj *Const) Val() constant.Value { return obj.val }

func (*Const) isDependency() {} // a constant may be a dependency of an initialization expression

// A TypeName represents a name for a (defined or alias) type.
// 使用type关键字定义的名字或者别名
type TypeName struct {
	object
}

// NewTypeName returns a new type name denoting the given typ.
// The remaining arguments set the attributes found with all Objects.
//
// The typ argument may be a defined (Named) type or an alias type.
// It may also be nil such that the returned TypeName can be used as
// argument for NewNamed, which will set the TypeName's type as a side-
// effect.
func NewTypeName(pos syntax.Pos, pkg *Package, name string, typ Type) *TypeName {
	return &TypeName{object{nil, pos, pkg, name, typ, 0, colorFor(typ), nopos}}
}

// NewTypeNameLazy returns a new defined type like NewTypeName, but it
// lazily calls resolve to finish constructing the Named object.
func NewTypeNameLazy(pos syntax.Pos, pkg *Package, name string, load func(named *Named) (tparams []*TypeParam, underlying Type, methods []*Func)) *TypeName {
	obj := NewTypeName(pos, pkg, name, nil)
	NewNamed(obj, nil, nil).loader = load
	return obj
}

// IsAlias reports whether obj is an alias name for a type.
// 判断obj是否是一个类型别名
func (obj *TypeName) IsAlias() bool {
	switch t := obj.typ.(type) {
	case nil:
		return false
	case *Basic:
		// unsafe.Pointer is not an alias.
		if obj.pkg == Unsafe {
			return false
		}
		// Any user-defined type name for a basic type is an alias for a
		// basic type (because basic types are pre-declared in the Universe
		// scope, outside any package scope), and so is any type name with
		// a different name than the name of the basic type it refers to.
		// Additionally, we need to look for "byte" and "rune" because they
		// are aliases but have the same names (for better error messages).
		return obj.pkg != nil || t.name != obj.name || t == universeByte || t == universeRune
	case *Named:
		return obj != t.obj
	case *TypeParam:
		return obj != t.obj
	default:
		return true
	}
}

// A Variable represents a declared variable (including function parameters and results, and struct fields).
type Var struct {
	object
	embedded bool // if set, the variable is an embedded struct field, and name is the type name
	isField  bool // var is struct field
	used     bool // set if the variable was used
	origin   *Var // if non-nil, the Var from which this one was instantiated
}

// NewVar returns a new variable.
// The arguments set the attributes found with all Objects.
// 返回一个变量
func NewVar(pos syntax.Pos, pkg *Package, name string, typ Type) *Var {
	return &Var{object: object{nil, pos, pkg, name, typ, 0, colorFor(typ), nopos}}
}

// NewParam returns a new variable representing a function parameter.
// 返回一个代表函数参数的var
func NewParam(pos syntax.Pos, pkg *Package, name string, typ Type) *Var {
	return &Var{object: object{nil, pos, pkg, name, typ, 0, colorFor(typ), nopos}, used: true} // parameters are always 'used'
}

// NewField returns a new variable representing a struct field.
// For embedded fields, the name is the unqualified type name
// under which the field is accessible.
// 返回一个结构体变量字段
func NewField(pos syntax.Pos, pkg *Package, name string, typ Type, embedded bool) *Var {
	return &Var{object: object{nil, pos, pkg, name, typ, 0, colorFor(typ), nopos}, embedded: embedded, isField: true}
}

// Anonymous reports whether the variable is an embedded field.
// Same as Embedded; only present for backward-compatibility.
// 是否是嵌入字段
func (obj *Var) Anonymous() bool { return obj.embedded }

// Embedded reports whether the variable is an embedded field.
// 是否是嵌入字段
func (obj *Var) Embedded() bool { return obj.embedded }

// IsField reports whether the variable is a struct field.
// 是否是结构体的一个字段
func (obj *Var) IsField() bool { return obj.isField }

// Origin returns the canonical Var for its receiver, i.e. the Var object
// recorded in Info.Defs.
//
// For synthetic Vars created during instantiation (such as struct fields or
// function parameters that depend on type arguments), this will be the
// corresponding Var on the generic (uninstantiated) type. For all other Vars
// Origin returns the receiver.
// 返回receiver或者泛型？
func (obj *Var) Origin() *Var {
	if obj.origin != nil {
		return obj.origin
	}
	return obj
}

func (*Var) isDependency() {} // a variable may be a dependency of an initialization expression

// A Func represents a declared function, concrete method, or abstract
// (interface) method. Its Type() is always a *Signature.
// An abstract method may belong to many interfaces due to embedding.
// 函数，方法，或者接口的抽象方法，type永远是Signature
type Func struct {
	object
	hasPtrRecv_ bool  // only valid for methods that don't have a type yet; use hasPtrRecv() to read
	origin      *Func // if non-nil, the Func from which this one was instantiated
}

// NewFunc returns a new function with the given signature, representing
// the function's type.
func NewFunc(pos syntax.Pos, pkg *Package, name string, sig *Signature) *Func {
	// don't store a (typed) nil signature
	var typ Type
	if sig != nil {
		typ = sig
	}
	return &Func{object{nil, pos, pkg, name, typ, 0, colorFor(typ), nopos}, false, nil}
}

// FullName returns the package- or receiver-type-qualified name of
// function or method obj.
func (obj *Func) FullName() string {
	var buf bytes.Buffer
	writeFuncName(&buf, obj, nil)
	return buf.String()
}

// Scope returns the scope of the function's body block.
// The result is nil for imported or instantiated functions and methods
// (but there is also no mechanism to get to an instantiated function).
func (obj *Func) Scope() *Scope { return obj.typ.(*Signature).scope }

// Origin returns the canonical Func for its receiver, i.e. the Func object
// recorded in Info.Defs.
//
// For synthetic functions created during instantiation (such as methods on an
// instantiated Named type or interface methods that depend on type arguments),
// this will be the corresponding Func on the generic (uninstantiated) type.
// For all other Funcs Origin returns the receiver.
// 返回函数receiver或者泛型
func (obj *Func) Origin() *Func {
	if obj.origin != nil {
		return obj.origin
	}
	return obj
}

// hasPtrRecv reports whether the receiver is of the form *T for the given method obj.
// 函数receiver是否是指针类型
func (obj *Func) hasPtrRecv() bool {
	// If a method's receiver type is set, use that as the source of truth for the receiver.
	// Caution: Checker.funcDecl (decl.go) marks a function by setting its type to an empty
	// signature. We may reach here before the signature is fully set up: we must explicitly
	// check if the receiver is set (we cannot just look for non-nil obj.typ).
	if sig, _ := obj.typ.(*Signature); sig != nil && sig.recv != nil {
		_, isPtr := deref(sig.recv.typ)
		return isPtr
	}

	// If a method's type is not set it may be a method/function that is:
	// 1) client-supplied (via NewFunc with no signature), or
	// 2) internally created but not yet type-checked.
	// For case 1) we can't do anything; the client must know what they are doing.
	// For case 2) we can use the information gathered by the resolver.
	return obj.hasPtrRecv_
}

func (*Func) isDependency() {} // a function may be a dependency of an initialization expression

// A Label represents a declared label.
// Labels don't have a type.
// 代表label，没有type
type Label struct {
	object
	used bool // set if the label was used
}

// NewLabel returns a new label.
func NewLabel(pos syntax.Pos, pkg *Package, name string) *Label {
	return &Label{object{pos: pos, pkg: pkg, name: name, typ: Typ[Invalid], color_: black}, false}
}

// A Builtin represents a built-in function.
// Builtins don't have a valid type.
// 内建函数，没有type
type Builtin struct {
	object
	id builtinId
}

func newBuiltin(id builtinId) *Builtin {
	return &Builtin{object{name: predeclaredFuncs[id].name, typ: Typ[Invalid], color_: black}, id}
}

// Nil represents the predeclared value nil.
// 代表nil
type Nil struct {
	object
}

// 把对象字符串化写到buf中
func writeObject(buf *bytes.Buffer, obj Object, qf Qualifier) {
	var tname *TypeName
	typ := obj.Type()

	switch obj := obj.(type) {
	case *PkgName:
		fmt.Fprintf(buf, "package %s", obj.Name())
		if path := obj.imported.path; path != "" && path != obj.name {
			fmt.Fprintf(buf, " (%q)", path)
		}
		return

	case *Const:
		buf.WriteString("const")

	case *TypeName:
		tname = obj
		buf.WriteString("type")
		if isTypeParam(typ) {
			buf.WriteString(" parameter")
		}

	case *Var:
		if obj.isField {
			buf.WriteString("field")
		} else {
			buf.WriteString("var")
		}

	case *Func:
		buf.WriteString("func ")
		// 写函数名
		writeFuncName(buf, obj, qf)
		if typ != nil {
			// 写函数签名
			WriteSignature(buf, typ.(*Signature), qf)
		}
		return

	case *Label:
		buf.WriteString("label")
		typ = nil

	case *Builtin:
		buf.WriteString("builtin")
		typ = nil

	case *Nil:
		buf.WriteString("nil")
		return

	default:
		panic(fmt.Sprintf("writeObject(%T)", obj))
	}

	buf.WriteByte(' ')

	// For package-level objects, qualify the name.
	if obj.Pkg() != nil && obj.Pkg().scope.Lookup(obj.Name()) == obj {
		writePackage(buf, obj.Pkg(), qf)
	}
	buf.WriteString(obj.Name())

	if typ == nil {
		return
	}

	if tname != nil {
		switch t := typ.(type) {
		case *Basic:
			// Don't print anything more for basic types since there's
			// no more information.
			return
		case *Named:
			if t.TypeParams().Len() > 0 {
				newTypeWriter(buf, qf).tParamList(t.TypeParams().list())
			}
		}
		if tname.IsAlias() {
			buf.WriteString(" =")
		} else if t, _ := typ.(*TypeParam); t != nil {
			typ = t.bound
		} else {
			// TODO(gri) should this be fromRHS for *Named?
			typ = under(typ)
		}
	}

	// Special handling for any: because WriteType will format 'any' as 'any',
	// resulting in the object string `type any = any` rather than `type any =
	// interface{}`. To avoid this, swap in a different empty interface.
	if obj == universeAny {
		assert(Identical(typ, &emptyInterface))
		typ = &emptyInterface
	}

	buf.WriteByte(' ')
	WriteType(buf, typ, qf)
}

// 把包的路径写进去（就是导入这个包的时候使用的路径）
func writePackage(buf *bytes.Buffer, pkg *Package, qf Qualifier) {
	if pkg == nil {
		return
	}
	var s string
	if qf != nil {
		s = qf(pkg)
	} else {
		s = pkg.Path()
	}
	if s != "" {
		buf.WriteString(s)
		buf.WriteByte('.')
	}
}

// ObjectString returns the string form of obj.
// The Qualifier controls the printing of
// package-level objects, and may be nil.
// 返回object的string
func ObjectString(obj Object, qf Qualifier) string {
	var buf bytes.Buffer
	writeObject(&buf, obj, qf)
	return buf.String()
}

func (obj *PkgName) String() string  { return ObjectString(obj, nil) }
func (obj *Const) String() string    { return ObjectString(obj, nil) }
func (obj *TypeName) String() string { return ObjectString(obj, nil) }
func (obj *Var) String() string      { return ObjectString(obj, nil) }
func (obj *Func) String() string     { return ObjectString(obj, nil) }
func (obj *Label) String() string    { return ObjectString(obj, nil) }
func (obj *Builtin) String() string  { return ObjectString(obj, nil) }
func (obj *Nil) String() string      { return ObjectString(obj, nil) }

// 写函数名，格式是 receiver/package.functionName
func writeFuncName(buf *bytes.Buffer, f *Func, qf Qualifier) {
	if f.typ != nil {
		sig := f.typ.(*Signature)
		if recv := sig.Recv(); recv != nil {
			buf.WriteByte('(')
			if _, ok := recv.Type().(*Interface); ok {
				// gcimporter creates abstract methods of
				// named interfaces using the interface type
				// (not the named type) as the receiver.
				// Don't print it in full.
				buf.WriteString("interface")
			} else {
				WriteType(buf, recv.Type(), qf)
			}
			buf.WriteByte(')')
			buf.WriteByte('.')
		} else if f.pkg != nil {
			writePackage(buf, f.pkg, qf)
		}
	}
	buf.WriteString(f.name)
}
