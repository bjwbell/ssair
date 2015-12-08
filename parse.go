package gossa

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"

	"github.com/bjwbell/cmd/obj"
	"github.com/bjwbell/ssa"
)

// type phivar struct {
// 	parent  *ast.AssignStmt
// 	varName *ast.Ident
// 	typ     *ast.Ident
// 	expr    ast.Expr
// }

// type ssaVar struct {
// 	name string
// 	node *ast.AssignStmt
// }

// type fnSSA struct {
// 	phi  []phivar
// 	vars []ssaVar
// 	decl *ast.FuncDecl
// }

// func (fn *fnSSA) initPhi() bool {

// 	ast.Inspect(fn.decl, func(n ast.Node) bool {
// 		assignStmt, ok := n.(*ast.AssignStmt)
// 		if !ok {
// 			return true
// 		}
// 		if len(assignStmt.Lhs) != 1 {
// 			panic("invalid assignment stmt")
// 		}
// 		if len(assignStmt.Lhs) != 2 {
// 			return true
// 		}
// 		if _, ok := assignStmt.Lhs[0].(*ast.Ident); !ok {
// 			return true
// 		}
// 		phiType, ok := assignStmt.Rhs[1].(*ast.Ident)
// 		if !ok {
// 			return true
// 		}
// 		phiExpr := assignStmt.Rhs[0]
// 		phiLit, ok := phiExpr.(*ast.CompositeLit)
// 		if !ok {
// 			return true
// 		}
// 		if phiLit.Type == nil {
// 			return true
// 		}
// 		phiIdent, ok := phiLit.Type.(*ast.Ident)
// 		if !ok {
// 			return true
// 		}
// 		if phiIdent.Name != "phi" {
// 			return true
// 		}
// 		var phi phivar
// 		phi.parent = assignStmt
// 		phi.expr = phiExpr
// 		phi.typ = phiType
// 		phi.varName = assignStmt.Lhs[0].(*ast.Ident)
// 		fn.phi = append(fn.phi, phi)
// 		return true
// 	})

// 	return true
// }

// func (fn *fnSSA) removePhi() bool {
// 	return true
// }

// func (fn *fnSSA) rewriteAssign() bool {
// 	return true
// }

// func (fn *fnSSA) restorePhi() bool {
// 	return true
// }

// ParseSSA parses the function, fn, which must be in ssa form and returns
// the corresponding ssa.Func
func ParseSSA(file, pkgName, fn string) (ssafn *ssa.Func, usessa bool) {
	var conf types.Config
	conf.Importer = importer.Default()
	/*conf.Error = func(err error) {
		fmt.Println("terror:", err)
	}*/
	fset := token.NewFileSet()
	fileAst, err := parser.ParseFile(fset, file, nil, parser.AllErrors)
	fileTok := fset.File(fileAst.Pos())
	var terrors string
	if err != nil {
		fmt.Printf("Error parsing %v, error message: %v\n", file, err)
		terrors += fmt.Sprintf("err: %v\n", err)
		return
	}

	files := []*ast.File{fileAst}
	info := types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Defs:  make(map[*ast.Ident]types.Object),
		Uses:  make(map[*ast.Ident]types.Object),
	}
	pkg, err := conf.Check(pkgName, fset, files, &info)
	if err != nil {
		if terrors != fmt.Sprintf("err: %v\n", err) {
			fmt.Printf("Type error (%v) message: %v\n", file, err)
			return
		}
	}

	fmt.Println("pkg: ", pkg)
	fmt.Println("pkg.Complete:", pkg.Complete())
	scope := pkg.Scope()
	obj := scope.Lookup(fn)
	if obj == nil {
		fmt.Println("Couldnt lookup function: ", fn)
		return
	}
	function, ok := obj.(*types.Func)
	if !ok {
		fmt.Printf("%v is a %v, not a function\n", fn, obj.Type().String())
	}
	var fnDecl *ast.FuncDecl
	for _, decl := range fileAst.Decls {
		if fdecl, ok := decl.(*ast.FuncDecl); ok {
			if fdecl.Name.Name == fn {
				fnDecl = fdecl
				break
			}
		}
	}
	if fnDecl == nil {
		fmt.Println("couldn't find function: ", fn)
		return
	}
	ssafn, ok = parseSSA(fileTok, fileAst, fnDecl, function, &info)
	if ssafn == nil || !ok {
		fmt.Println("Error building SSA form")
	} else {
		fmt.Println("ssa:\n", ssafn)
	}
	if ssafn != nil && ok {
		fmt.Println("ssafn:", ssafn)
	}
	return ssafn, ok
}

type Ctx struct {
	file *token.File
	fn   *types.Info
}

type ssaVar interface {
	ssaVar()
}

type ssaParam struct {
	ssaVar
	ident *ast.Ident
	ctx   Ctx
}

type ssaId struct {
	ssaVar
	assign *ast.AssignStmt
	ctx    Ctx
}

func getParameters(ctx Ctx, fn *ast.FuncDecl) []*ssaParam {
	var params []*ssaParam
	for i := 0; i < fn.Type.Params.NumFields(); i++ {
		for _, param := range fn.Type.Params.List {
			for _, name := range param.Names {
				n := ssaParam{ident: name, ctx: ctx}
				params = append(params, &n)
			}
		}

	}
	return params

}

func linenum(f *token.File, p token.Pos) int32 {
	return int32(f.Line(p))
}

func getIdent(ctx Ctx, obj types.Object) *ast.Ident {
	for ident, obj := range ctx.fn.Defs {
		if obj == obj {
			return ident
		}
	}
	return nil
}

func getLocalDecls(ctx Ctx, fn *types.Func) []*ssaId {
	scope := fn.Scope()
	names := scope.Names()
	var locals []*ssaId
	for i := 0; i < len(names); i++ {
		name := names[i]
		obj := scope.Lookup(name)
		ident := getIdent(ctx, obj)
		if ident == nil {
			panic(fmt.Sprintf("Couldn't lookup: %v", name))
		}
		//node := ssaId{assign: ident, ctx: ctx}
		node := ssaId{assign: nil, ctx: ctx}
		locals = append(locals, &node)
	}
	return locals
}

func parseSSA(ftok *token.File, f *ast.File, fn *ast.FuncDecl, fnType *types.Func, fnInfo *types.Info) (ssafn *ssa.Func, ok bool) {

	// HACK, hardcoded
	arch := "amd64"

	signature, ok := fnType.Type().(*types.Signature)
	if !ok {
		panic("function type is not types.Signature")
	}
	if signature.Recv() != nil {
		fmt.Println("Methods not supported")
		return nil, false
	}
	if signature.Results().Len() > 1 {
		fmt.Println("Multiple return values not supported")
	}

	var e ssaExport
	e.log = true
	link := obj.Link{}
	config := ssa.NewConfig(arch, &e, &link)
	ssafn = config.NewFunc()
	ssafn.Name = fnType.Name()
	ssafn.Entry = ssafn.NewBlock(ssa.BlockPlain)
	fmt.Println("f :", f)

	return nil, false
}

// ssaExport exports a bunch of compiler services for the ssa backend.
type ssaExport struct {
	log bool
}

func (s *ssaExport) TypeBool() ssa.Type    { return Typ[types.Bool] }
func (s *ssaExport) TypeInt8() ssa.Type    { return Typ[types.Int8] }
func (s *ssaExport) TypeInt16() ssa.Type   { return Typ[types.Int16] }
func (s *ssaExport) TypeInt32() ssa.Type   { return Typ[types.Int32] }
func (s *ssaExport) TypeInt64() ssa.Type   { return Typ[types.Int64] }
func (s *ssaExport) TypeUInt8() ssa.Type   { return Typ[types.Uint8] }
func (s *ssaExport) TypeUInt16() ssa.Type  { return Typ[types.Uint16] }
func (s *ssaExport) TypeUInt32() ssa.Type  { return Typ[types.Uint32] }
func (s *ssaExport) TypeUInt64() ssa.Type  { return Typ[types.Uint64] }
func (s *ssaExport) TypeFloat32() ssa.Type { return Typ[types.Float32] }
func (s *ssaExport) TypeFloat64() ssa.Type { return Typ[types.Float64] }
func (s *ssaExport) TypeInt() ssa.Type     { return Typ[types.Int] }
func (s *ssaExport) TypeUintptr() ssa.Type { return Typ[types.Uintptr] }
func (s *ssaExport) TypeString() ssa.Type  { return Typ[types.String] }
func (s *ssaExport) TypeBytePtr() ssa.Type { return Typ[types.Uint8].PtrTo() }

// StringData returns a symbol (a *Sym wrapped in an interface) which
// is the data component of a global string constant containing s.
func (*ssaExport) StringData(s string) interface{} {
	// TODO
	return nil
}

func (e *ssaExport) Auto(t ssa.Type) ssa.GCNode {
	/*n := temp(t.(*Type))   // Note: adds new auto to Curfn.Func.Dcl list
	e.mustImplement = true // This modifies the input to SSA, so we want to make sure we succeed from here!*/
	//return n
	return nil
}

func (e *ssaExport) CanSSA(t ssa.Type) bool {
	return true //canSSAType(t.(*Type))
}

// Log logs a message from the compiler.
func (e *ssaExport) Logf(msg string, args ...interface{}) {
	// If e was marked as unimplemented, anything could happen. Ignore.
	if e.log {
		fmt.Printf(msg, args...)
	}
}

func Fatalf(format string, args ...interface{}) {
	msg := "internal compiler error: " + format
	fmt.Printf(msg, args)
	fmt.Printf("\n")
	panic("")
}

// Fatal reports a compiler error and exits.
func (e *ssaExport) Fatalf(msg string, args ...interface{}) {
	Fatalf(msg, args...)
}

// Unimplemented reports that the function cannot be compiled.
// It will be removed once SSA work is complete.
func (e *ssaExport) Unimplementedf(msg string, args ...interface{}) {
	Fatalf(msg, args...)
}

// Warnl reports a "warning", which is usually flag-triggered
// logging output for the benefit of tests.
func (e *ssaExport) Warnl(line int, fmt_ string, args ...interface{}) {
	panic("Warnl")
	//Warnl(line, fmt_, args...)
}

func (e *ssaExport) Debug_checknil() bool {
	return false
}
