// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package types2

// A term describes elementary type sets:
//
//	 ∅:  (*term)(nil)     == ∅                      // set of no types (empty set)
//	 𝓤:  &term{}          == 𝓤                      // set of all types (𝓤niverse)
//	 T:  &term{false, T}  == {T}                    // set of type T
//	~t:  &term{true, t}   == {t' | under(t') == t}  // set of types with underlying type t
// 代表范性类型集合
/*
  *term空指针，代表是空集
  &term{},代表全集
  &term{false, T} 代表集合里面只有一个元素是类型T
  &term{true, t}代表集合里面的元素包括t和底层类型是t的元素
*/
type term struct {
	tilde bool // valid if typ != nil是否带～
	typ   Type
}

func (x *term) String() string {
	switch {
	case x == nil:
		return "∅"
	case x.typ == nil:
		return "𝓤"
	case x.tilde:
		return "~" + x.typ.String()
	default:
		return x.typ.String()
	}
}

// equal reports whether x and y represent the same type set.
// 是否俩人表示相同的类型范围
func (x *term) equal(y *term) bool {
	// easy cases
	switch {
	case x == nil || y == nil:
		return x == y
	case x.typ == nil || y.typ == nil:
		return x.typ == y.typ
	}
	// ∅ ⊂ x, y ⊂ 𝓤

	return x.tilde == y.tilde && Identical(x.typ, y.typ)
}

// union returns the union x ∪ y: zero, one, or two non-nil terms.
// x和y的并集
func (x *term) union(y *term) (_, _ *term) {
	// easy cases
	switch {
	case x == nil && y == nil:
		return nil, nil // ∅ ∪ ∅ == ∅
	case x == nil:
		return y, nil // ∅ ∪ y == y
	case y == nil:
		return x, nil // x ∪ ∅ == x
	case x.typ == nil:
		return x, nil // 𝓤 ∪ y == 𝓤
	case y.typ == nil:
		return y, nil // x ∪ 𝓤 == 𝓤
	}
	// ∅ ⊂ x, y ⊂ 𝓤

	if x.disjoint(y) {
		return x, y // x ∪ y == (x, y) if x ∩ y == ∅
	}
	// x.typ == y.typ

	// ~t ∪ ~t == ~t
	// ~t ∪  T == ~t
	//  T ∪ ~t == ~t
	//  T ∪  T ==  T
	if x.tilde || !y.tilde {
		return x, nil
	}
	return y, nil
}

// intersect returns the intersection x ∩ y.
// 取x和y的交集
func (x *term) intersect(y *term) *term {
	// easy cases
	switch {
	case x == nil || y == nil:
		return nil // ∅ ∩ y == ∅ and ∩ ∅ == ∅
	case x.typ == nil:
		return y // 𝓤 ∩ y == y
	case y.typ == nil:
		return x // x ∩ 𝓤 == x
	}
	// ∅ ⊂ x, y ⊂ 𝓤

	if x.disjoint(y) {
		return nil // x ∩ y == ∅ if x ∩ y == ∅
	}
	// x.typ == y.typ

	// ~t ∩ ~t == ~t
	// ~t ∩  T ==  T
	//  T ∩ ~t ==  T
	//  T ∩  T ==  T
	if !x.tilde || y.tilde {
		return x
	}
	return y
}

// includes reports whether t ∈ x.
// t是否属于x范围内
func (x *term) includes(t Type) bool {
	// easy cases
	switch {
	case x == nil:
		return false // t ∈ ∅ == false
	case x.typ == nil:
		return true // t ∈ 𝓤 == true
	}
	// ∅ ⊂ x ⊂ 𝓤

	u := t
	if x.tilde {
		u = under(u)
	}
	return Identical(x.typ, u)
}

// subsetOf reports whether x ⊆ y.
// x是否是y的子集
func (x *term) subsetOf(y *term) bool {
	// easy cases
	switch {
	case x == nil:
		return true // ∅ ⊆ y == true
	case y == nil:
		return false // x ⊆ ∅ == false since x != ∅
	case y.typ == nil:
		return true // x ⊆ 𝓤 == true
	case x.typ == nil:
		return false // 𝓤 ⊆ y == false since y != 𝓤
	}
	// ∅ ⊂ x, y ⊂ 𝓤

	if x.disjoint(y) {
		return false // x ⊆ y == false if x ∩ y == ∅
	}
	// x.typ == y.typ

	// ~t ⊆ ~t == true
	// ~t ⊆ T == false
	//  T ⊆ ~t == true
	//  T ⊆  T == true
	return !x.tilde || y.tilde
}

// disjoint reports whether x ∩ y == ∅.
// x.typ and y.typ must not be nil.
// x和y是否有重合的部分，就是x和y交集不为空
func (x *term) disjoint(y *term) bool {
	if debug && (x.typ == nil || y.typ == nil) {
		panic("invalid argument(s)")
	}
	ux := x.typ
	if y.tilde {
		ux = under(ux)
	}
	uy := y.typ
	if x.tilde {
		uy = under(uy)
	}
	return !Identical(ux, uy)
}
