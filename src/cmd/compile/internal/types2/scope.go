// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file implements Scopes.

package types2

import (
	"bytes"
	"cmd/compile/internal/syntax"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
)

// A Scope maintains a set of objects and links to its containing
// (parent) and contained (children) scopes. Objects may be inserted
// and looked up by name. The zero value for Scope is a ready-to-use
// empty scope.
// 作用域结构体，包含他的父作用域和子作用域，还有当前作用域的符号
type Scope struct {
	parent   *Scope            //当前作用于的父作用域
	children []*Scope          //当前作用于的子作用于
	number   int               //当前作用于在parent的children切片中的位置 parent.children[number-1] is this scope; 0 if there is no parent
	elems    map[string]Object // lazily allocated // 当前作用于的符号
	pos, end syntax.Pos        // scope extent; may be invalid 当前作用域的范围
	comment  string            // for debugging only
	isFunc   bool              // set if this is a function scope (internal use only)
}

// NewScope returns a new, empty scope contained in the given parent
// scope, if any. The comment is for debugging only.
// 创建一个作用域，并把当前作用域加入到parent的children中
func NewScope(parent *Scope, pos, end syntax.Pos, comment string) *Scope {
	s := &Scope{parent, nil, 0, nil, pos, end, comment, false}
	// don't add children to Universe scope!
	if parent != nil && parent != Universe {
		parent.children = append(parent.children, s)
		s.number = len(parent.children)
	}
	return s
}

// Parent returns the scope's containing (parent) scope.
// 返回夫作用域
func (s *Scope) Parent() *Scope { return s.parent }

// Len returns the number of scope elements.
// 当前作用域的符号个数
func (s *Scope) Len() int { return len(s.elems) }

// Names returns the scope's element names in sorted order.
// 当前作用域的所有符号名（按字典排序）
func (s *Scope) Names() []string {
	names := make([]string, len(s.elems))
	i := 0
	for name := range s.elems {
		names[i] = name
		i++
	}
	sort.Strings(names)
	return names
}

// NumChildren returns the number of scopes nested in s.
// 子作用域的个数
func (s *Scope) NumChildren() int { return len(s.children) }

// Child returns the i'th child scope for 0 <= i < NumChildren().
// 返回第i个子作用域
func (s *Scope) Child(i int) *Scope { return s.children[i] }

// Lookup returns the object in scope s with the given name if such an
// object exists; otherwise the result is nil.
// 返回当前作用域中名字是name的符号
func (s *Scope) Lookup(name string) Object {
	return resolve(name, s.elems[name])
}

// LookupParent follows the parent chain of scopes starting with s until
// it finds a scope where Lookup(name) returns a non-nil object, and then
// returns that scope and object. If a valid position pos is provided,
// only objects that were declared at or before pos are considered.
// If no such scope and object exists, the result is (nil, nil).
//
// Note that obj.Parent() may be different from the returned scope if the
// object was inserted into the scope and already had a parent at that
// time (see Insert). This can only happen for dot-imported objects
// whose scope is the scope of the package that exported them.
// 去所有的祖先类中查找是否存在name符号
func (s *Scope) LookupParent(name string, pos syntax.Pos) (*Scope, Object) {
	for ; s != nil; s = s.parent {
		if obj := s.Lookup(name); obj != nil && (!pos.IsKnown() || obj.scopePos().Cmp(pos) <= 0) {
			return s, obj
		}
	}
	return nil, nil
}

// Insert attempts to insert an object obj into scope s.
// If s already contains an alternative object alt with
// the same name, Insert leaves s unchanged and returns alt.
// Otherwise it inserts obj, sets the object's parent scope
// if not already set, and returns nil.
// 插入一个符号到当前作用域，如果符号已经插入过那么什么都不做直接返回之前插入的符号。如果没插入过那么插入进去并设置符号的父作用域是当前作用域并返回符号
func (s *Scope) Insert(obj Object) Object {
	name := obj.Name()
	if alt := s.Lookup(name); alt != nil {
		return alt
	}
	s.insert(name, obj)
	if obj.Parent() == nil {
		obj.setParent(s)
	}
	return nil
}

// InsertLazy is like Insert, but allows deferring construction of the
// inserted object until it's accessed with Lookup. The Object
// returned by resolve must have the same name as given to InsertLazy.
// If s already contains an alternative object with the same name,
// InsertLazy leaves s unchanged and returns false. Otherwise it
// records the binding and returns true. The object's parent scope
// will be set to s after resolve is called.
// 插入一个符号，但是这个符号只有在调用Lookup的时候才会用resolve去创建，返回值代表是否之前没有插入过
func (s *Scope) InsertLazy(name string, resolve func() Object) bool {
	if s.elems[name] != nil {
		return false
	}
	s.insert(name, &lazyObject{parent: s, resolve: resolve})
	return true
}

// 插入一个obj
func (s *Scope) insert(name string, obj Object) {
	if s.elems == nil {
		s.elems = make(map[string]Object)
	}
	s.elems[name] = obj
}

// Squash merges s with its parent scope p by adding all
// objects of s to p, adding all children of s to the
// children of p, and removing s from p's children.
// The function f is called for each object obj in s which
// has an object alt in p. s should be discarded after
// having been squashed.
// 就是把当前作用域(自己的符号和子作用域) 合并到父作用域，并释放自己这个作用域
func (s *Scope) Squash(err func(obj, alt Object)) {
	p := s.parent
	assert(p != nil)
	for name, obj := range s.elems {
		obj = resolve(name, obj)
		obj.setParent(nil)
		if alt := p.Insert(obj); alt != nil {
			err(obj, alt)
		}
	}

	j := -1 // index of s in p.children
	for i, ch := range p.children {
		if ch == s {
			j = i
			break
		}
	}
	assert(j >= 0)
	k := len(p.children) - 1
	// 干掉自己
	p.children[j] = p.children[k]
	p.children = p.children[:k]
	// 合并children
	p.children = append(p.children, s.children...)

	s.children = nil
	s.elems = nil
}

// Pos and End describe the scope's source code extent [pos, end).
// The results are guaranteed to be valid only if the type-checked
// AST has complete position information. The extent is undefined
// for Universe and package scopes.
// 返回作用域在源代码中的范围，Universe(最高作用域)和包作用域对这两个值没有做定义
func (s *Scope) Pos() syntax.Pos { return s.pos }
func (s *Scope) End() syntax.Pos { return s.end }

// Contains reports whether pos is within the scope's extent.
// The result is guaranteed to be valid only if the type-checked
// AST has complete position information.
// pos这个位置是否在s的作用域中
func (s *Scope) Contains(pos syntax.Pos) bool {
	return s.pos.Cmp(pos) <= 0 && pos.Cmp(s.end) < 0
}

// Innermost returns the innermost (child) scope containing
// pos. If pos is not within any scope, the result is nil.
// The result is also nil for the Universe scope.
// The result is guaranteed to be valid only if the type-checked
// AST has complete position information.
// 找到pos所在的直接作用域
func (s *Scope) Innermost(pos syntax.Pos) *Scope {
	// Package scopes do not have extents since they may be
	// discontiguous, so iterate over the package's files.
	if s.parent == Universe {
		for _, s := range s.children {
			if inner := s.Innermost(pos); inner != nil {
				return inner
			}
		}
	}

	if s.Contains(pos) {
		for _, s := range s.children {
			if s.Contains(pos) {
				return s.Innermost(pos)
			}
		}
		return s
	}
	return nil
}

// WriteTo writes a string representation of the scope to w,
// with the scope elements sorted by name.
// The level of indentation is controlled by n >= 0, with
// n == 0 for no indentation.
// If recurse is set, it also writes nested (children) scopes.
// 用来可视化打印scope
func (s *Scope) WriteTo(w io.Writer, n int, recurse bool) {
	const ind = ".  "
	indn := strings.Repeat(ind, n)

	fmt.Fprintf(w, "%s%s scope %p {\n", indn, s.comment, s)

	indn1 := indn + ind
	for _, name := range s.Names() {
		fmt.Fprintf(w, "%s%s\n", indn1, s.Lookup(name))
	}

	if recurse {
		for _, s := range s.children {
			s.WriteTo(w, n+1, recurse)
		}
	}

	fmt.Fprintf(w, "%s}\n", indn)
}

// String returns a string representation of the scope, for debugging.
// 可视化scope字符串
func (s *Scope) String() string {
	var buf bytes.Buffer
	s.WriteTo(&buf, 0, false)
	return buf.String()
}

// A lazyObject represents an imported Object that has not been fully
// resolved yet by its importer.
type lazyObject struct {
	parent  *Scope
	resolve func() Object
	obj     Object
	once    sync.Once
}

// resolve returns the Object represented by obj, resolving lazy
// objects as appropriate.
// 就是根据lazyObject创建真正的object
func resolve(name string, obj Object) Object {
	if lazy, ok := obj.(*lazyObject); ok {
		lazy.once.Do(func() {
			obj := lazy.resolve()

			if _, ok := obj.(*lazyObject); ok {
				panic("recursive lazy object")
			}
			if obj.Name() != name {
				panic("lazy object has unexpected name")
			}

			if obj.Parent() == nil {
				obj.setParent(lazy.parent)
			}
			lazy.obj = obj
		})

		obj = lazy.obj
	}
	return obj
}

// stub implementations so *lazyObject implements Object and we can
// store them directly into Scope.elems.
func (*lazyObject) Parent() *Scope                        { panic("unreachable") }
func (*lazyObject) Pos() syntax.Pos                       { panic("unreachable") }
func (*lazyObject) Pkg() *Package                         { panic("unreachable") }
func (*lazyObject) Name() string                          { panic("unreachable") }
func (*lazyObject) Type() Type                            { panic("unreachable") }
func (*lazyObject) Exported() bool                        { panic("unreachable") }
func (*lazyObject) Id() string                            { panic("unreachable") }
func (*lazyObject) String() string                        { panic("unreachable") }
func (*lazyObject) order() uint32                         { panic("unreachable") }
func (*lazyObject) color() color                          { panic("unreachable") }
func (*lazyObject) setType(Type)                          { panic("unreachable") }
func (*lazyObject) setOrder(uint32)                       { panic("unreachable") }
func (*lazyObject) setColor(color color)                  { panic("unreachable") }
func (*lazyObject) setParent(*Scope)                      { panic("unreachable") }
func (*lazyObject) sameId(pkg *Package, name string) bool { panic("unreachable") }
func (*lazyObject) scopePos() syntax.Pos                  { panic("unreachable") }
func (*lazyObject) setScopePos(pos syntax.Pos)            { panic("unreachable") }
