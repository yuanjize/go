// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package syntax

// ----------------------------------------------------------------------------
// Nodes

type Node interface {
	// Pos() returns the position associated with the node as follows:
	// 1) The position of a node representing a terminal syntax production
	//    (Name, BasicLit, etc.) is the position of the respective production
	//    in the source.
	// 2) The position of a node representing a non-terminal production
	//    (IndexExpr, IfStmt, etc.) is the position of a token uniquely
	//    associated with that production; usually the left-most one
	//    ('[' for IndexExpr, 'if' for IfStmt, etc.)
	Pos() Pos
	aNode()
}

type node struct {
	// commented out for now since not yet used
	// doc  *Comment // nil means no comment(s) attached
	pos Pos
}

func (n *node) Pos() Pos { return n.pos }
func (*node) aNode()     {}

// ----------------------------------------------------------------------------
// Files

// package PkgName; DeclList[0], DeclList[1], ...
type File struct {
	Pragma   Pragma
	PkgName  *Name
	DeclList []Decl
	EOF      Pos
	node
}

// ----------------------------------------------------------------------------
// Declarations

type (
	// 就是最外侧的全局声明，或者全局变量
	Decl interface {
		Node
		aDecl()
	}

	//              Path
	// LocalPkgName Path
	ImportDecl struct {
		Group        *Group // nil means not part of a group
		Pragma       Pragma
		LocalPkgName *Name     // 就是导入包时候起的别名，包括 。_ 还有真正的别名 including "."; nil means no rename present
		Path         *BasicLit // 导入的包，比如 "fmt" Path.Bad || Path.Kind == StringLit; nil means no path
		decl
	}

	// NameList
	// NameList      = Values
	// NameList Type = Values
	ConstDecl struct { // const声明语句
		Group    *Group // nil means not part of a group
		Pragma   Pragma
		NameList []*Name // 变量名列表
		Type     Expr    // nil means no type 变量类型
		Values   Expr    // nil means no values 变量赋的值列表
		decl
	}

	// Name Type
	TypeDecl struct { //类型定义
		Group      *Group // nil means not part of a group
		Pragma     Pragma
		Name       *Name    // 类型名
		TParamList []*Field // 泛型类型 nil means no type parameters 好像是泛型的
		Alias      bool     // 是否是起别名 type A int or type A = int
		Type       Expr     // 实际类型
		decl
	}

	// NameList Type
	// NameList Type = Values
	// NameList      = Values
	VarDecl struct { // 变量声明语句
		Group    *Group // nil means not part of a group
		Pragma   Pragma
		NameList []*Name // 变量名列表
		Type     Expr    // nil means no type 变量类型
		Values   Expr    // nil means no values 变量赋的值
		decl
	}

	// func          Name Type { Body }
	// func          Name Type
	// func Receiver Name Type { Body }
	// func Receiver Name Type
	FuncDecl struct { // 声明函数（这种是必须有名字的）
		Pragma     Pragma
		Recv       *Field     //如果是成员还是，那么这个就是类 nil means regular function
		Name       *Name      // 函数民主国i
		TParamList []*Field   // 泛型列表 nil means no type parameters
		Type       *FuncType  // 函数签名，包括参数和返回值
		Body       *BlockStmt // 函数体  nil means no body (forward declaration)
		decl
	}
)

type decl struct{ node }

func (*decl) aDecl() {}

// All declarations belonging to the same group point to the same Group node.
type Group struct {
	_ int // not empty so we are guaranteed different Group instances
}

// ----------------------------------------------------------------------------
// Expressions

func NewName(pos Pos, value string) *Name {
	n := new(Name)
	n.pos = pos
	n.Value = value
	return n
}

type (
	Expr interface {
		Node
		aExpr()
	}

	// 表达式有错误 Placeholder for an expression that failed to parse
	// correctly and where we can't provide a better node.
	BadExpr struct {
		expr
	}

	// Value 名字，函数名，变量名，类型名啥的
	Name struct {
		Value string
		expr
	}

	// Value 字面量值
	BasicLit struct {
		Value string  // 常量值
		Kind  LitKind // 字面量类型，比如是字符串还是数字啥的
		Bad   bool    // 字面值是不是有错误 true means the literal Value has syntax errors
		expr
	}

	// Type { ElemList[0], ElemList[1], ... }
	CompositeLit struct { // 看起来是组合元素，例如初始化数组，map，结构体啥的(chan 不算)
		Type     Expr   // nil means no literal type 元素类型
		ElemList []Expr //每个元素的值
		NKeys    int    // number of elements with keys key个数。map？
		Rbrace   Pos
		expr
	}

	// Key: Value  KV类型，map和struct初始化的时候都是这个类型
	KeyValueExpr struct {
		Key, Value Expr
		expr
	}

	// func Type { Body }
	FuncLit struct { // 就是匿名函数（闭包函数），一般会赋给变量或者直接调用
		Type *FuncType  // 函数签名(参数和返回值)
		Body *BlockStmt // 函数体
		expr
	}

	// (X)
	ParenExpr struct {
		X Expr
		expr
	}

	// X.Sel  选择表达式 person.name
	SelectorExpr struct {
		X   Expr  // 上面的person
		Sel *Name // 上面的name
		expr
	}

	// X[Index]
	// X[T1, T2, ...] (with Ti = Index.(*ListExpr).ElemList[i])
	// 索引表达式，比如slice[1],array[10],map["name"]
	IndexExpr struct {
		X     Expr
		Index Expr
		expr
	}

	// X[Index[0] : Index[1] : Index[2]]
	// 切片表达式，slice[low:hight:max]
	SliceExpr struct {
		X     Expr
		Index [3]Expr // 就是low:hight:max
		// Full indicates whether this is a simple or full slice expression.
		// In a valid AST, this is equivalent to Index[2] != nil.
		// TODO(mdempsky): This is only needed to report the "3-index
		// slice of string" error when Index[2] is missing.
		Full bool //是否是全切
		expr
	}

	// X.(Type) 断言/转换表达式 p:=a.(*Person)这种
	AssertExpr struct {
		X    Expr // 变量
		Type Expr // 断言的类型
		expr
	}

	// X.(type)
	// Lhs := X.(type)
	// 就是switch中的类型转换 switch a:=b.(type)
	TypeSwitchGuard struct {
		Lhs *Name // nil means no Lhs := 就是上面的a，可以为空
		X   Expr  // X.(type)  就是b
		expr
	}

	// 操作表达式。用操作符的都可以用这个，包括从chan读取数据
	Operation struct {
		Op   Operator // 操作符
		X, Y Expr     // X是操作符左边，Y是操作符右边 Y == nil means unary expression
		expr
	}

	// Fun(ArgList[0], ArgList[1], ...)
	CallExpr struct { // 函数调用表达式
		Fun     Expr   // 函数调用
		ArgList []Expr // nil means no arguments //所有的参数
		HasDots bool   // last argument is followed by ...
		expr
	}

	// ElemList[0], ElemList[1], ... 逗号分隔
	ListExpr struct { // 赋值时候用逗号隔开元素值列表 var a,c=1,2 这里的1，2就是这种
		ElemList []Expr
		expr
	}

	// [Len]Elem 数组类型
	ArrayType struct {
		// TODO(gri) consider using Name{"..."} instead of nil (permits attaching of comments)
		Len  Expr //数组长度 nil means Len is ...
		Elem Expr // 数组元素类型
		expr
	}

	// []Elem 切片类型
	SliceType struct {
		Elem Expr // 元素类型
		expr
	}

	// ...Elem  例如...int
	DotsType struct {
		Elem Expr // 元素类型,例如上面的int
		expr
	}

	// struct { FieldList[0] TagList[0]; FieldList[1] TagList[1]; ... }
	// 结构体类型
	StructType struct {
		FieldList []*Field    // 字段列表
		TagList   []*BasicLit //tag列表，字段要是没有对应的tag，这个切片对应位置的值就是null i >= len(TagList) || TagList[i] == nil means no tag for field i
		expr
	}

	// Name Type
	//      Type
	Field struct { // 就是字段名和类型
		Name *Name // nil means anonymous field/parameter (structs/parameters), or embedded element (interfaces)
		Type Expr  // field names declared in a list share the same Type (identical pointers)
		node
	}

	// interface { MethodList[0]; MethodList[1]; ... }
	// 接口类型
	InterfaceType struct {
		MethodList []*Field // 接口类型包含的函数列表，如果是interface{},那么这个列表就是null
		expr
	}

	FuncType struct { // 函数签名
		ParamList  []*Field // 函数参数
		ResultList []*Field // 返回值
		expr
	}

	// map[Key]Value map类型
	MapType struct {
		Key, Value Expr
		expr
	}

	//   chan Elem
	// <-chan Elem
	// chan<- Elem
	ChanType struct { // 类型是chain
		Dir  ChanDir // 0 means no direction
		Elem Expr    // chan的类型，例如chan int，类型就是int
		expr
	}
)

type expr struct{ node }

func (*expr) aExpr() {}

type ChanDir uint

const (
	_ ChanDir = iota
	SendOnly
	RecvOnly
)

// ----------------------------------------------------------------------------
// Statements

type (
	Stmt interface {
		Node
		aStmt()
	}

	SimpleStmt interface {
		Stmt
		aSimpleStmt()
	}

	EmptyStmt struct {
		simpleStmt
	}
	// label的代码段
	LabeledStmt struct {
		Label *Name // label的名字
		Stmt  Stmt  // label下面的代码语句
		stmt
	}
	// 代码块
	BlockStmt struct {
		List   []Stmt // 语句块
		Rbrace Pos
		stmt
	}
	//就是把表达式转换成stmt，实体是X
	ExprStmt struct {
		X Expr
		simpleStmt
	}
	// 向chan发送消息的语句
	SendStmt struct {
		Chan, Value Expr // Chan <- Value
		simpleStmt
	}
	// 代码里面的局部声明语句，例如var a=1;var f=func()P{}.cost啥的，反正最上面的Decl代码基本都可以
	DeclStmt struct {
		DeclList []Decl
		stmt
	}
	// 赋值语句 a=b
	AssignStmt struct { //操作符，上面的=
		Op       Operator // 0 means no operation
		Lhs, Rhs Expr     // Lhs就是左边的符号，就是a，rhs是右边，就是b Rhs == nil means Lhs++ (Op == Add) or Lhs-- (Op == Sub)
		simpleStmt
	}
	// 分支语句
	BranchStmt struct {
		Tok   token // 使用的语句 Break, Continue, Fallthrough, or Goto
		Label *Name // 要跳转的label
		// Target is the continuation of the control flow after executing
		// the branch; it is computed by the parser if CheckBranches is set.
		// Target is a *LabeledStmt for gotos, and a *SwitchStmt, *SelectStmt,
		// or *ForStmt for breaks and continues, depending on the context of
		// the branch. Target is not set for fallthroughs.
		Target Stmt // 跳转之后的控制流，就是下一个要执行的代码位置
		stmt
	}

	// 函数调用语句，只有go个defer函数调用用这个，普通函数调用使用 ExprStmt
	CallStmt struct {
		Tok  token     // Go or Defer 关键词
		Call *CallExpr //函数调用表达式
		stmt
	}
	// return语句
	ReturnStmt struct {
		Results Expr // 返回值，nil代表没有返回值 nil means no explicit return values
		stmt
	}
	// if 语句 if a:=OK();a!=nil{}else{}
	IfStmt struct {
		Init SimpleStmt // 就是a:=OK() 这个语句
		Cond Expr       // 条件语句a!=nil
		Then *BlockStmt // if为true时候的代码块
		Else Stmt       // else代码块 either nil, *IfStmt, or *BlockStmt
		stmt
	}
	// for循环语句，range语句只有Init语句
	ForStmt struct {
		Init SimpleStmt // 初始化语句或者range语句 incl. *RangeClause
		Cond Expr       // 条件语句
		Post SimpleStmt // 条件语句后面的那一句
		Body *BlockStmt //循环体
		stmt
	}
	// switch语句 switch a{}; switch b:=a.(type){}
	SwitchStmt struct {
		Init   SimpleStmt    // switch是可以有初始化语句的，但是一般不这么用。 switch a:=10;a{}
		Tag    Expr          //switch后面接的，普通switch就是后面的值(上面的)，断言switch就是TypeSwitchGuard（上面的a.(type)） incl. *TypeSwitchGuard
		Body   []*CaseClause //后面跟着的case和default语句
		Rbrace Pos
		stmt
	}
	// select语句
	SelectStmt struct {
		Body   []*CommClause // select的case语句
		Rbrace Pos
		stmt
	}
)

type (
	// range语句 for k,v:=range slice{}
	RangeClause struct {
		Lhs Expr // 代表左边的k,v nil means no Lhs = or Lhs :=
		Def bool // 代表:= means :=
		X   Expr // 代表range slice range X
		simpleStmt
	}
	// switch case语句
	CaseClause struct {
		Cases Expr   // case后面跟着的判断。如果是default语句那么这个就是nil nil means default clause
		Body  []Stmt // case匹配之后要执行的语句
		Colon Pos    //case后面的 : 在文件中的位置
		node
	}
	// select case语句
	CommClause struct {
		Comm  SimpleStmt // case条件，对chan的发送或者接收，default语句该字段是nill send or receive stmt; nil means default clause
		Body  []Stmt     // 匹配之后要执行的逻辑
		Colon Pos        // : 符号的位置
		node
	}
)

type stmt struct{ node }

func (stmt) aStmt() {}

type simpleStmt struct {
	stmt
}

func (simpleStmt) aSimpleStmt() {}

// ----------------------------------------------------------------------------
// Comments

// TODO(gri) Consider renaming to CommentPos, CommentPlacement, etc.
// Kind = Above doesn't make much sense.
type CommentKind uint

const (
	Above CommentKind = iota
	Below
	Left
	Right
)

type Comment struct {
	Kind CommentKind
	Text string
	Next *Comment
}
