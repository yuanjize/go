// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package types2

import "cmd/compile/internal/syntax"

// ----------------------------------------------------------------------------
// API

// An Interface represents an interface type.
type Interface struct {
	check     *Checker      // for error reporting; nil once type set is computed
	methods   []*Func       // ordered list of explicitly declared methods
	embeddeds []Type        // ordered list of explicitly embedded elements
	embedPos  *[]syntax.Pos // positions of embedded elements; or nil (for error messages) - use pointer to save space
	implicit  bool          //隐式接口，就是没有函数，只有范型声明 interface is wrapper for type set literal (non-interface T, ~T, or A|B)
	complete  bool          // indicates that all fields (except for tset) are set up

	tset *_TypeSet // type set described by this interface, computed lazily
}

// typeSet returns the type set for interface t.
func (t *Interface) typeSet() *_TypeSet { return computeInterfaceTypeSet(t.check, nopos, t) }

// emptyInterface represents the empty interface
var emptyInterface = Interface{complete: true, tset: &topTypeSet}

// NewInterfaceType returns a new interface for the given methods and embedded types.
// NewInterfaceType takes ownership of the provided methods and may modify their types
// by setting missing receivers.
func NewInterfaceType(methods []*Func, embeddeds []Type) *Interface {
	if len(methods) == 0 && len(embeddeds) == 0 {
		return &emptyInterface
	}

	// set method receivers if necessary
	typ := (*Checker)(nil).newInterface()
	for _, m := range methods {
		if sig := m.typ.(*Signature); sig.recv == nil {
			sig.recv = NewVar(m.pos, m.pkg, "", typ)
		}
	}

	// sort for API stability
	sortMethods(methods)

	typ.methods = methods
	typ.embeddeds = embeddeds
	typ.complete = true

	return typ
}

// check may be nil
func (check *Checker) newInterface() *Interface {
	typ := &Interface{check: check}
	if check != nil {
		check.needsCleanup(typ)
	}
	return typ
}

// MarkImplicit marks the interface t as implicit, meaning this interface
// corresponds to a constraint literal such as ~T or A|B without explicit
// interface embedding. MarkImplicit should be called before any concurrent use
// of implicit interfaces.
func (t *Interface) MarkImplicit() {
	t.implicit = true
}

// NumExplicitMethods returns the number of explicitly declared methods of interface t.
// 接口显式声明的方法的个数
func (t *Interface) NumExplicitMethods() int { return len(t.methods) }

// ExplicitMethod returns the i'th explicitly declared method of interface t for 0 <= i < t.NumExplicitMethods().
// The methods are ordered by their unique Id.
// 获取接口显式声明的方法
func (t *Interface) ExplicitMethod(i int) *Func { return t.methods[i] }

// NumEmbeddeds returns the number of embedded types in interface t.
// 嵌入类型的个数
func (t *Interface) NumEmbeddeds() int { return len(t.embeddeds) }

// EmbeddedType returns the i'th embedded type of interface t for 0 <= i < t.NumEmbeddeds().
// 获取接口内嵌的类型
func (t *Interface) EmbeddedType(i int) Type { return t.embeddeds[i] }

// NumMethods returns the total number of methods of interface t.
// 接口t所有的方法的总数(就是包含嵌入的)
func (t *Interface) NumMethods() int { return t.typeSet().NumMethods() }

// Method returns the i'th method of interface t for 0 <= i < t.NumMethods().
// The methods are ordered by their unique Id.
// 获取接口t所有的方法的某一个
func (t *Interface) Method(i int) *Func { return t.typeSet().Method(i) }

// Empty reports whether t is the empty interface.
// 判断是否是空接口
func (t *Interface) Empty() bool { return t.typeSet().IsAll() }

// IsComparable reports whether each type in interface t's type set is comparable.
// 是否t的type集合是可以比较的
func (t *Interface) IsComparable() bool { return t.typeSet().IsComparable(nil) }

// IsMethodSet reports whether the interface t is fully described by its method set.
// 是否接口只有method没有别的（范型什么的）
func (t *Interface) IsMethodSet() bool { return t.typeSet().IsMethodSet() }

// IsImplicit reports whether the interface t is a wrapper for a type set literal.
func (t *Interface) IsImplicit() bool { return t.implicit }

func (t *Interface) Underlying() Type { return t }
func (t *Interface) String() string   { return TypeString(t, nil) }

// ----------------------------------------------------------------------------
// Implementation

func (t *Interface) cleanup() {
	t.check = nil
	t.embedPos = nil
}

func (check *Checker) interfaceType(ityp *Interface, iface *syntax.InterfaceType, def *Named) {
	addEmbedded := func(pos syntax.Pos, typ Type) {
		ityp.embeddeds = append(ityp.embeddeds, typ)
		if ityp.embedPos == nil {
			ityp.embedPos = new([]syntax.Pos)
		}
		*ityp.embedPos = append(*ityp.embedPos, pos)
	}

	for _, f := range iface.MethodList {
		if f.Name == nil {
			addEmbedded(posFor(f.Type), parseUnion(check, f.Type))
			continue
		}
		// f.Name != nil

		// We have a method with name f.Name.
		name := f.Name.Value
		if name == "_" {
			check.error(f.Name, "methods must have a unique non-blank name")
			continue // ignore
		}

		typ := check.typ(f.Type)
		sig, _ := typ.(*Signature)
		if sig == nil {
			if typ != Typ[Invalid] {
				check.errorf(f.Type, invalidAST+"%s is not a method signature", typ)
			}
			continue // ignore
		}

		// use named receiver type if available (for better error messages)
		var recvTyp Type = ityp
		if def != nil {
			recvTyp = def
		}
		sig.recv = NewVar(f.Name.Pos(), check.pkg, "", recvTyp)

		m := NewFunc(f.Name.Pos(), check.pkg, name, sig)
		check.recordDef(f.Name, m)
		ityp.methods = append(ityp.methods, m)
	}

	// All methods and embedded elements for this interface are collected;
	// i.e., this interface may be used in a type set computation.
	ityp.complete = true

	if len(ityp.methods) == 0 && len(ityp.embeddeds) == 0 {
		// empty interface
		ityp.tset = &topTypeSet
		return
	}

	// sort for API stability
	// (don't sort embeddeds: they must correspond to *embedPos entries)
	sortMethods(ityp.methods)

	// Compute type set as soon as possible to report any errors.
	// Subsequent uses of type sets will use this computed type
	// set and won't need to pass in a *Checker.
	check.later(func() {
		computeInterfaceTypeSet(check, iface.Pos(), ityp)
	}).describef(iface, "compute type set for %s", ityp)
}
