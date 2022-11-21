// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package types2

import (
	"fmt"
)

// A Package describes a Go package.
// 包的结构表示
type Package struct {
	path     string     // 路径，就是import abc的abc
	name     string     // 包的名称，就是package abc 的adb
	scope    *Scope     // 包的作用域
	imports  []*Package // 导入的包
	height   int
	complete bool //是否
	fake     bool // scope lookup errors are silently dropped if package is fake (internal use only)
	cgo      bool // uses of this package will be rewritten into uses of declarations from _cgo_gotypes.go
}

// NewPackage returns a new Package for the given package path and name.
// The package is not complete and contains no explicit imports.
func NewPackage(path, name string) *Package {
	return NewPackageHeight(path, name, 0)
}

// NewPackageHeight is like NewPackage, but allows specifying the
// package's height.
func NewPackageHeight(path, name string, height int) *Package {
	scope := NewScope(Universe, nopos, nopos, fmt.Sprintf("package %q", path))
	return &Package{path: path, name: name, scope: scope, height: height}
}

// Path returns the package path.
func (pkg *Package) Path() string { return pkg.path }

// Name returns the package name.
func (pkg *Package) Name() string { return pkg.name }

// Height returns the package height.
func (pkg *Package) Height() int { return pkg.height }

// SetName sets the package name.
func (pkg *Package) SetName(name string) { pkg.name = name }

// Scope returns the (complete or incomplete) package scope
// holding the objects declared at package level (TypeNames,
// Consts, Vars, and Funcs).
// For a nil pkg receiver, Scope returns the Universe scope.
// 返回包作用域下面的那堆东西，pkg要是空，那么作用域就是全域
func (pkg *Package) Scope() *Scope {
	if pkg != nil {
		return pkg.scope
	}
	return Universe
}

// A package is complete if its scope contains (at least) all
// exported objects; otherwise it is incomplete.
// 包是否是complete状态，如果他的作用域包含了所有导出的对象，那么就是完成状态
func (pkg *Package) Complete() bool { return pkg.complete }

// MarkComplete marks a package as complete.
// 设置包为完成状态
func (pkg *Package) MarkComplete() { pkg.complete = true }

// Imports returns the list of packages directly imported by
// pkg; the list is in source order.
//
// If pkg was loaded from export data, Imports includes packages that
// provide package-level objects referenced by pkg. This may be more or
// less than the set of packages directly imported by pkg's source code.
func (pkg *Package) Imports() []*Package { return pkg.imports }

// SetImports sets the list of explicitly imported packages to list.
// It is the caller's responsibility to make sure list elements are unique.
func (pkg *Package) SetImports(list []*Package) { pkg.imports = list }

func (pkg *Package) String() string {
	return fmt.Sprintf("package %s (%q)", pkg.name, pkg.path)
}
