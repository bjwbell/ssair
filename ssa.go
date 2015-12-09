package gossa

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"github.com/bjwbell/ssa"
)

type state struct {
	// configuration (arch) information
	config *ssa.Config
	// context includes *token.File and *types.File
	ctx Ctx

	// function we're building
	f      *ssa.Func
	fnInfo *types.Info
	fnType *types.Func
	// labels and labeled control flow nodes in f
	labels       map[string]*ssaLabel
	labeledNodes map[ast.Node]*ssaLabel

	// gotos that jump forward; required for deferred checkGoto calls
	fwdGotos []*Node
	// Code that must precede any return
	// (e.g., copying value of heap-escaped paramout back to true paramout)
	//exitCode *NodeList

	// unlabeled break and continue statement tracking
	breakTo    *ssa.Block // current target for plain break statement
	continueTo *ssa.Block // current target for plain continue statement

	// current location where we're interpreting the AST
	curBlock *ssa.Block

	// variable assignments in the current block (map from variable symbol to ssa value)
	// *Node is the unique identifier (an ONAME Node) for the variable.
	vars map[ssaVar]*ssa.Value

	// all defined variables at the end of each block.  Indexed by block ID.
	defvars []map[ssaVar]*ssa.Value

	// addresses of PPARAM and PPARAMOUT variables.
	decladdrs map[ssaVar]*ssa.Value

	// symbols for PEXTERN, PAUTO and PPARAMOUT variables so they can be reused.
	varsyms map[ssaVar]interface{}

	// starting values.  Memory, stack pointer, and globals pointer
	startmem *ssa.Value
	sp       *ssa.Value
	sb       *ssa.Value

	// line number stack.  The current line number is top of stack
	line []int32
}

func (s *state) label(ident *ast.Ident) *ssaLabel {
	lab := s.labels[ident.Name]
	if lab == nil {
		lab = new(ssaLabel)
		s.labels[ident.Name] = lab
	}
	return lab
}

func (s *state) Logf(msg string, args ...interface{})   { s.config.Logf(msg, args...) }
func (s *state) Fatalf(msg string, args ...interface{}) { s.config.Fatalf(msg, args...) }
func (s *state) Unimplementedf(msg string, args ...interface{}) {
	// TODO: comment/remove when no longer needed for debugging
	fmt.Printf("s.UNIMPLEMENTED msg: %v\n", fmt.Sprintf(msg, args))

	s.config.Unimplementedf(msg, args...)
}
func (s *state) Warnl(line int, msg string, args ...interface{}) { s.config.Warnl(line, msg, args...) }
func (s *state) Debug_checknil() bool                            { return s.config.Debug_checknil() }

var (
// dummy node for the memory variable
//memVar = Node{class: Pxxx}

// dummy nodes for temporary variables
/*ptrVar   = Node{Op: ONAME, Class: Pxxx, Sym: &Sym{Name: "ptr"}}
capVar   = Node{Op: ONAME, Class: Pxxx, Sym: &Sym{Name: "cap"}}
typVar   = Node{Op: ONAME, Class: Pxxx, Sym: &Sym{Name: "typ"}}
idataVar = Node{Op: ONAME, Class: Pxxx, Sym: &Sym{Name: "idata"}}
okVar    = Node{Op: ONAME, Class: Pxxx, Sym: &Sym{Name: "ok"}}*/
)

// startBlock sets the current block we're generating code in to b.
func (s *state) startBlock(b *ssa.Block) {
	if s.curBlock != nil {
		s.Fatalf("starting block %v when block %v has not ended", b, s.curBlock)
	}
	s.curBlock = b
	//s.vars = map[*Node]*ssa.Value{}
}

// endBlock marks the end of generating code for the current block.
// Returns the (former) current block.  Returns nil if there is no current
// block, i.e. if no code flows to the current execution point.
func (s *state) endBlock() *ssa.Block {
	b := s.curBlock
	if b == nil {
		return nil
	}
	for len(s.defvars) <= int(b.ID) {
		s.defvars = append(s.defvars, nil)
	}
	s.defvars[b.ID] = s.vars
	s.curBlock = nil
	s.vars = nil
	b.Line = s.peekLine()
	return b
}

// pushLine pushes a line number on the line number stack.
func (s *state) pushLine(line int32) {
	s.line = append(s.line, line)
}

// popLine pops the top of the line number stack.
func (s *state) popLine() {
	//s.line = s.line[:len(s.line)-1]
}

// peekLine peek the top of the line number stack.
func (s *state) peekLine() int32 {
	return 0 //s.line[len(s.line)-1]
}

func (s *state) Errorf(msg string, args ...interface{}) {
	panic(msg)
}

// newValue0 adds a new value with no arguments to the current block.
func (s *state) newValue0(op ssa.Op, t ssa.Type) *ssa.Value {
	return s.curBlock.NewValue0(s.peekLine(), op, t)
}

// newValue0A adds a new value with no arguments and an aux value to the current block.
func (s *state) newValue0A(op ssa.Op, t ssa.Type, aux interface{}) *ssa.Value {
	return s.curBlock.NewValue0A(s.peekLine(), op, t, aux)
}

// newValue0I adds a new value with no arguments and an auxint value to the current block.
func (s *state) newValue0I(op ssa.Op, t ssa.Type, auxint int64) *ssa.Value {
	return s.curBlock.NewValue0I(s.peekLine(), op, t, auxint)
}

// newValue1 adds a new value with one argument to the current block.
func (s *state) newValue1(op ssa.Op, t ssa.Type, arg *ssa.Value) *ssa.Value {
	return s.curBlock.NewValue1(s.peekLine(), op, t, arg)
}

// newValue1A adds a new value with one argument and an aux value to the current block.
func (s *state) newValue1A(op ssa.Op, t ssa.Type, aux interface{}, arg *ssa.Value) *ssa.Value {
	return s.curBlock.NewValue1A(s.peekLine(), op, t, aux, arg)
}

// newValue1I adds a new value with one argument and an auxint value to the current block.
func (s *state) newValue1I(op ssa.Op, t ssa.Type, aux int64, arg *ssa.Value) *ssa.Value {
	return s.curBlock.NewValue1I(s.peekLine(), op, t, aux, arg)
}

// newValue2 adds a new value with two arguments to the current block.
func (s *state) newValue2(op ssa.Op, t ssa.Type, arg0, arg1 *ssa.Value) *ssa.Value {
	return s.curBlock.NewValue2(s.peekLine(), op, t, arg0, arg1)
}

// newValue2I adds a new value with two arguments and an auxint value to the current block.
func (s *state) newValue2I(op ssa.Op, t ssa.Type, aux int64, arg0, arg1 *ssa.Value) *ssa.Value {
	return s.curBlock.NewValue2I(s.peekLine(), op, t, aux, arg0, arg1)
}

// newValue3 adds a new value with three arguments to the current block.
func (s *state) newValue3(op ssa.Op, t ssa.Type, arg0, arg1, arg2 *ssa.Value) *ssa.Value {
	return s.curBlock.NewValue3(s.peekLine(), op, t, arg0, arg1, arg2)
}

// newValue3I adds a new value with three arguments and an auxint value to the current block.
func (s *state) newValue3I(op ssa.Op, t ssa.Type, aux int64, arg0, arg1, arg2 *ssa.Value) *ssa.Value {
	return s.curBlock.NewValue3I(s.peekLine(), op, t, aux, arg0, arg1, arg2)
}

// entryNewValue0 adds a new value with no arguments to the entry block.
func (s *state) entryNewValue0(op ssa.Op, t ssa.Type) *ssa.Value {
	return s.f.Entry.NewValue0(s.peekLine(), op, t)
}

// entryNewValue0A adds a new value with no arguments and an aux value to the entry block.
func (s *state) entryNewValue0A(op ssa.Op, t ssa.Type, aux interface{}) *ssa.Value {
	return s.f.Entry.NewValue0A(s.peekLine(), op, t, aux)
}

// entryNewValue0I adds a new value with no arguments and an auxint value to the entry block.
func (s *state) entryNewValue0I(op ssa.Op, t ssa.Type, auxint int64) *ssa.Value {
	return s.f.Entry.NewValue0I(s.peekLine(), op, t, auxint)
}

// entryNewValue1 adds a new value with one argument to the entry block.
func (s *state) entryNewValue1(op ssa.Op, t ssa.Type, arg *ssa.Value) *ssa.Value {
	return s.f.Entry.NewValue1(s.peekLine(), op, t, arg)
}

// entryNewValue1 adds a new value with one argument and an auxint value to the entry block.
func (s *state) entryNewValue1I(op ssa.Op, t ssa.Type, auxint int64, arg *ssa.Value) *ssa.Value {
	return s.f.Entry.NewValue1I(s.peekLine(), op, t, auxint, arg)
}

// entryNewValue1A adds a new value with one argument and an aux value to the entry block.
func (s *state) entryNewValue1A(op ssa.Op, t ssa.Type, aux interface{}, arg *ssa.Value) *ssa.Value {
	return s.f.Entry.NewValue1A(s.peekLine(), op, t, aux, arg)
}

// entryNewValue2 adds a new value with two arguments to the entry block.
func (s *state) entryNewValue2(op ssa.Op, t ssa.Type, arg0, arg1 *ssa.Value) *ssa.Value {
	return s.f.Entry.NewValue2(s.peekLine(), op, t, arg0, arg1)
}

// const* routines add a new const value to the entry block.
func (s *state) constBool(c bool) *ssa.Value {
	return s.f.ConstBool(s.peekLine(), Typ[types.Bool], c)
}
func (s *state) constInt8(t ssa.Type, c int8) *ssa.Value {
	return s.f.ConstInt8(s.peekLine(), t, c)
}
func (s *state) constInt16(t ssa.Type, c int16) *ssa.Value {
	return s.f.ConstInt16(s.peekLine(), t, c)
}
func (s *state) constInt32(t ssa.Type, c int32) *ssa.Value {
	return s.f.ConstInt32(s.peekLine(), t, c)
}
func (s *state) constInt64(t ssa.Type, c int64) *ssa.Value {
	return s.f.ConstInt64(s.peekLine(), t, c)
}
func (s *state) constFloat32(t ssa.Type, c float64) *ssa.Value {
	return s.f.ConstFloat32(s.peekLine(), t, c)
}
func (s *state) constFloat64(t ssa.Type, c float64) *ssa.Value {
	return s.f.ConstFloat64(s.peekLine(), t, c)
}
func (s *state) constInt(t ssa.Type, c int64) *ssa.Value {
	if s.config.IntSize == 8 {
		return s.constInt64(t, c)
	}
	if int64(int32(c)) != c {
		s.Fatalf("integer constant too big %d", c)
	}
	return s.constInt32(t, int32(c))
}

func (s *state) labeledEntryBlock(block *ast.BlockStmt) bool {
	return s.entryBlockLabel(block) != nil
}

func (s *state) entryBlockLabel(block *ast.BlockStmt) *ast.LabeledStmt {
	// the first stmt may be a label for the entry block
	if len(block.List) >= 1 {
		if labeledStmt, ok := block.List[0].(*ast.LabeledStmt); ok {
			return labeledStmt
		}

	}
	return nil
}

// body converts the body of fn to SSA and adds it to s.
func (s *state) body(block *ast.BlockStmt) {

	if !s.labeledEntryBlock(block) {
		panic("entry block must be labeled (even if with \"_\")")
	}
	s.stmtList(block.List)
}

// ssaStmtList converts the statement n to SSA and adds it to s.
func (s *state) stmtList(stmtList []ast.Stmt) {
	for _, stmt := range stmtList {
		s.stmt(stmt)
	}
}

func NewNode(n ast.Node, ctx Ctx) *Node {
	return &Node{node: n, ctx: ctx}
}

func isBlankIdent(ident *ast.Ident) bool {
	return ident != nil && ident.Name == "_"
}

// ssaStmt converts the statement stmt to SSA and adds it to s.
func (s *state) stmt(stmt ast.Stmt) {
	// node := stmt.(ast.Node)
	// n := &Node{node: node, ctx: s.ctx}
	// s.pushLine(n.Lineno())
	// defer s.popLine()

	// If s.curBlock is nil, then we're about to generate dead code.
	// We can't just short-circuit here, though,
	// because we check labels and gotos as part of SSA generation.
	// Provide a block for the dead code so that we don't have
	// to add special cases everywhere else.
	if s.curBlock == nil {
		dead := s.f.NewBlock(ssa.BlockPlain)
		s.startBlock(dead)
	}

	// TODO

	switch stmt := stmt.(type) {
	case *ast.LabeledStmt:
		lblIdent := stmt.Label
		if isBlankIdent(lblIdent) {
			// Empty identifier is valid but useless.
			// See issues 11589, 11593.
			return
		}

		lab := s.label(lblIdent)

		if !lab.defined() {
			lab.defNode = NewNode(stmt, s.ctx)
		} else {
			s.Errorf("label %v already defined at %v", lblIdent.Name, "<line#>")
			lab.reported = true
		}
		// The label might already have a target block via a goto.
		if lab.target == nil {
			lab.target = s.f.NewBlock(ssa.BlockPlain)
		}

		// go to that label (we pretend "label:" is preceded by "goto label")
		b := s.endBlock()
		b.AddEdgeTo(lab.target)
		s.startBlock(lab.target)
	case *ast.AssignStmt:
		s.assignStmt(stmt)
	case *ast.BadStmt:
		panic("error BadStmt")
	case *ast.BlockStmt:
		// TODO: handle correctly
		s.stmtList(stmt.List)
	case *ast.BranchStmt:
		n := NewNode(stmt, s.ctx)
		switch stmt.Tok {
		case token.GOTO:
		default:
			panic("Error: only goto branch statements supported (not break, continue, or fallthrough ")
		}

		lab := s.label(stmt.Label)
		if lab.target == nil {
			lab.target = s.f.NewBlock(ssa.BlockPlain)
		}
		if !lab.used() {
			lab.useNode = n
		}

		if lab.defined() {
			s.checkGoto(n, lab.defNode)
		} else {
			s.fwdGotos = append(s.fwdGotos, n)
		}

		b := s.endBlock()
		b.AddEdgeTo(lab.target)

	case *ast.DeclStmt:
		panic("todo ast.DeclStmt")
	case *ast.EmptyStmt: // No op
	case *ast.ExprStmt:
		panic("todo ast.ExprStmt")
	case *ast.IfStmt:
		var errored bool
		if stmt.Init != nil {
			panic("Error: if statement cannot have init expr")
		}
		errMsg := "Error: if statement must be of the form \"if t1 { goto lbl1 } else { goto lbl2 }\""
		if len(stmt.Body.List) != 1 {
			panic(errMsg)
		}
		bdyStmt, ok := stmt.Body.List[0].(*ast.BranchStmt)
		errored = errored || !ok

		if stmt.Else == nil {
			errored = true
		}

		elseBody, ok := stmt.Else.(*ast.BlockStmt)
		errored = errored || !ok

		elseStmt, ok := elseBody.List[0].(*ast.BranchStmt)
		errored = errored || !ok

		condIdent, ok := stmt.Cond.(*ast.Ident)
		errored = errored || !ok

		if !errored {
			panic(errMsg)
		}
		fmt.Println("if condIdent:", condIdent)
		fmt.Println("if bdyStmt:", bdyStmt)
		fmt.Println("if elseStmt:", elseStmt)
		panic("todo ast.IfStmt")
	case *ast.IncDecStmt:
		panic("todo ast.IncDecStmt")
	case *ast.ReturnStmt:
		panic("todo ast.ReturnStmt")
	case *ast.ForStmt:
		panic("unsupported: ForStmt")
	case *ast.GoStmt:
		panic("unsupported: GoStmt")
	case *ast.RangeStmt:
		panic("unsupported: RangeStmt")
	case *ast.DeferStmt:
		panic("unsupported: DeferStmt")
	case *ast.SelectStmt:
		panic("unsupported: SelectStmt")
	case *ast.SendStmt:
		panic("unsupported: SendStmt")
	case *ast.SwitchStmt:
		panic("unsupported: SwitchStmt")
	case *ast.TypeSwitchStmt:
		panic("unsupported: TypeSwitchStmt")
	default:
		fmt.Println("stmt: ", stmt)
		panic("unknown ast.Stmt")
	}
}

type opAndType struct {
	op  NodeOp
	typ types.BasicKind
}

var opToSSA = map[opAndType]ssa.Op{
	opAndType{OADD, types.Int8}:   ssa.OpAdd8,
	opAndType{OADD, types.Uint8}:  ssa.OpAdd8,
	opAndType{OADD, types.Int16}:  ssa.OpAdd16,
	opAndType{OADD, types.Uint16}: ssa.OpAdd16,
	opAndType{OADD, types.Int32}:  ssa.OpAdd32,
	opAndType{OADD, types.Uint32}: ssa.OpAdd32,
	//opAndType{OADD, types.Ptr32}:   ssa.OpAdd32,
	opAndType{OADD, types.Int64}:  ssa.OpAdd64,
	opAndType{OADD, types.Uint64}: ssa.OpAdd64,
	//opAndType{OADD, types.Ptr64}:   ssa.OpAdd64,
	opAndType{OADD, types.Float32}: ssa.OpAdd32F,
	opAndType{OADD, types.Float64}: ssa.OpAdd64F,

	opAndType{OSUB, types.Int8}:    ssa.OpSub8,
	opAndType{OSUB, types.Uint8}:   ssa.OpSub8,
	opAndType{OSUB, types.Int16}:   ssa.OpSub16,
	opAndType{OSUB, types.Uint16}:  ssa.OpSub16,
	opAndType{OSUB, types.Int32}:   ssa.OpSub32,
	opAndType{OSUB, types.Uint32}:  ssa.OpSub32,
	opAndType{OSUB, types.Int64}:   ssa.OpSub64,
	opAndType{OSUB, types.Uint64}:  ssa.OpSub64,
	opAndType{OSUB, types.Float32}: ssa.OpSub32F,
	opAndType{OSUB, types.Float64}: ssa.OpSub64F,

	opAndType{ONOT, types.Bool}: ssa.OpNot,

	opAndType{OMINUS, types.Int8}:    ssa.OpNeg8,
	opAndType{OMINUS, types.Uint8}:   ssa.OpNeg8,
	opAndType{OMINUS, types.Int16}:   ssa.OpNeg16,
	opAndType{OMINUS, types.Uint16}:  ssa.OpNeg16,
	opAndType{OMINUS, types.Int32}:   ssa.OpNeg32,
	opAndType{OMINUS, types.Uint32}:  ssa.OpNeg32,
	opAndType{OMINUS, types.Int64}:   ssa.OpNeg64,
	opAndType{OMINUS, types.Uint64}:  ssa.OpNeg64,
	opAndType{OMINUS, types.Float32}: ssa.OpNeg32F,
	opAndType{OMINUS, types.Float64}: ssa.OpNeg64F,

	opAndType{OCOM, types.Int8}:   ssa.OpCom8,
	opAndType{OCOM, types.Uint8}:  ssa.OpCom8,
	opAndType{OCOM, types.Int16}:  ssa.OpCom16,
	opAndType{OCOM, types.Uint16}: ssa.OpCom16,
	opAndType{OCOM, types.Int32}:  ssa.OpCom32,
	opAndType{OCOM, types.Uint32}: ssa.OpCom32,
	opAndType{OCOM, types.Int64}:  ssa.OpCom64,
	opAndType{OCOM, types.Uint64}: ssa.OpCom64,

	opAndType{OIMAG, types.Complex64}:  ssa.OpComplexImag,
	opAndType{OIMAG, types.Complex128}: ssa.OpComplexImag,
	opAndType{OREAL, types.Complex64}:  ssa.OpComplexReal,
	opAndType{OREAL, types.Complex128}: ssa.OpComplexReal,

	opAndType{OMUL, types.Int8}:    ssa.OpMul8,
	opAndType{OMUL, types.Uint8}:   ssa.OpMul8,
	opAndType{OMUL, types.Int16}:   ssa.OpMul16,
	opAndType{OMUL, types.Uint16}:  ssa.OpMul16,
	opAndType{OMUL, types.Int32}:   ssa.OpMul32,
	opAndType{OMUL, types.Uint32}:  ssa.OpMul32,
	opAndType{OMUL, types.Int64}:   ssa.OpMul64,
	opAndType{OMUL, types.Uint64}:  ssa.OpMul64,
	opAndType{OMUL, types.Float32}: ssa.OpMul32F,
	opAndType{OMUL, types.Float64}: ssa.OpMul64F,

	opAndType{ODIV, types.Float32}: ssa.OpDiv32F,
	opAndType{ODIV, types.Float64}: ssa.OpDiv64F,

	opAndType{OHMUL, types.Int8}:   ssa.OpHmul8,
	opAndType{OHMUL, types.Uint8}:  ssa.OpHmul8u,
	opAndType{OHMUL, types.Int16}:  ssa.OpHmul16,
	opAndType{OHMUL, types.Uint16}: ssa.OpHmul16u,
	opAndType{OHMUL, types.Int32}:  ssa.OpHmul32,
	opAndType{OHMUL, types.Uint32}: ssa.OpHmul32u,

	opAndType{ODIV, types.Int8}:   ssa.OpDiv8,
	opAndType{ODIV, types.Uint8}:  ssa.OpDiv8u,
	opAndType{ODIV, types.Int16}:  ssa.OpDiv16,
	opAndType{ODIV, types.Uint16}: ssa.OpDiv16u,
	opAndType{ODIV, types.Int32}:  ssa.OpDiv32,
	opAndType{ODIV, types.Uint32}: ssa.OpDiv32u,
	opAndType{ODIV, types.Int64}:  ssa.OpDiv64,
	opAndType{ODIV, types.Uint64}: ssa.OpDiv64u,

	opAndType{OMOD, types.Int8}:   ssa.OpMod8,
	opAndType{OMOD, types.Uint8}:  ssa.OpMod8u,
	opAndType{OMOD, types.Int16}:  ssa.OpMod16,
	opAndType{OMOD, types.Uint16}: ssa.OpMod16u,
	opAndType{OMOD, types.Int32}:  ssa.OpMod32,
	opAndType{OMOD, types.Uint32}: ssa.OpMod32u,
	opAndType{OMOD, types.Int64}:  ssa.OpMod64,
	opAndType{OMOD, types.Uint64}: ssa.OpMod64u,

	opAndType{OAND, types.Int8}:   ssa.OpAnd8,
	opAndType{OAND, types.Uint8}:  ssa.OpAnd8,
	opAndType{OAND, types.Int16}:  ssa.OpAnd16,
	opAndType{OAND, types.Uint16}: ssa.OpAnd16,
	opAndType{OAND, types.Int32}:  ssa.OpAnd32,
	opAndType{OAND, types.Uint32}: ssa.OpAnd32,
	opAndType{OAND, types.Int64}:  ssa.OpAnd64,
	opAndType{OAND, types.Uint64}: ssa.OpAnd64,

	opAndType{OOR, types.Int8}:   ssa.OpOr8,
	opAndType{OOR, types.Uint8}:  ssa.OpOr8,
	opAndType{OOR, types.Int16}:  ssa.OpOr16,
	opAndType{OOR, types.Uint16}: ssa.OpOr16,
	opAndType{OOR, types.Int32}:  ssa.OpOr32,
	opAndType{OOR, types.Uint32}: ssa.OpOr32,
	opAndType{OOR, types.Int64}:  ssa.OpOr64,
	opAndType{OOR, types.Uint64}: ssa.OpOr64,

	opAndType{OXOR, types.Int8}:   ssa.OpXor8,
	opAndType{OXOR, types.Uint8}:  ssa.OpXor8,
	opAndType{OXOR, types.Int16}:  ssa.OpXor16,
	opAndType{OXOR, types.Uint16}: ssa.OpXor16,
	opAndType{OXOR, types.Int32}:  ssa.OpXor32,
	opAndType{OXOR, types.Uint32}: ssa.OpXor32,
	opAndType{OXOR, types.Int64}:  ssa.OpXor64,
	opAndType{OXOR, types.Uint64}: ssa.OpXor64,

	opAndType{OEQ, types.Bool}:   ssa.OpEq8,
	opAndType{OEQ, types.Int8}:   ssa.OpEq8,
	opAndType{OEQ, types.Uint8}:  ssa.OpEq8,
	opAndType{OEQ, types.Int16}:  ssa.OpEq16,
	opAndType{OEQ, types.Uint16}: ssa.OpEq16,
	opAndType{OEQ, types.Int32}:  ssa.OpEq32,
	opAndType{OEQ, types.Uint32}: ssa.OpEq32,
	opAndType{OEQ, types.Int64}:  ssa.OpEq64,
	opAndType{OEQ, types.Uint64}: ssa.OpEq64,
	// opAndType{OEQ, types.Inter}:     ssa.OpEqInter,
	// opAndType{OEQ, types.Array}:     ssa.OpEqSlice,
	// opAndType{OEQ, types.Func}:      ssa.OpEqPtr,
	// opAndType{OEQ, types.Map}:       ssa.OpEqPtr,
	// opAndType{OEQ, types.Chan}:      ssa.OpEqPtr,
	// opAndType{OEQ, types.Ptr64}:     ssa.OpEqPtr,
	opAndType{OEQ, types.Uintptr}: ssa.OpEqPtr,
	// opAndType{OEQ, types.Unsafeptr}: ssa.OpEqPtr,
	opAndType{OEQ, types.Float64}: ssa.OpEq64F,
	opAndType{OEQ, types.Float32}: ssa.OpEq32F,

	opAndType{ONE, types.Bool}:   ssa.OpNeq8,
	opAndType{ONE, types.Int8}:   ssa.OpNeq8,
	opAndType{ONE, types.Uint8}:  ssa.OpNeq8,
	opAndType{ONE, types.Int16}:  ssa.OpNeq16,
	opAndType{ONE, types.Uint16}: ssa.OpNeq16,
	opAndType{ONE, types.Int32}:  ssa.OpNeq32,
	opAndType{ONE, types.Uint32}: ssa.OpNeq32,
	opAndType{ONE, types.Int64}:  ssa.OpNeq64,
	opAndType{ONE, types.Uint64}: ssa.OpNeq64,
	// opAndType{ONE, types.Inter}:     ssa.OpNeqInter,
	// opAndType{ONE, types.Array}:     ssa.OpNeqSlice,
	// opAndType{ONE, types.Func}:      ssa.OpNeqPtr,
	// opAndType{ONE, types.Map}:       ssa.OpNeqPtr,
	// opAndType{ONE, types.Chan}:      ssa.OpNeqPtr,
	// opAndType{ONE, types.Ptr64}:     ssa.OpNeqPtr,
	opAndType{ONE, types.Uintptr}: ssa.OpNeqPtr,
	// opAndType{ONE, types.Unsafeptr}: ssa.OpNeqPtr,
	opAndType{ONE, types.Float64}: ssa.OpNeq64F,
	opAndType{ONE, types.Float32}: ssa.OpNeq32F,

	opAndType{OLT, types.Int8}:    ssa.OpLess8,
	opAndType{OLT, types.Uint8}:   ssa.OpLess8U,
	opAndType{OLT, types.Int16}:   ssa.OpLess16,
	opAndType{OLT, types.Uint16}:  ssa.OpLess16U,
	opAndType{OLT, types.Int32}:   ssa.OpLess32,
	opAndType{OLT, types.Uint32}:  ssa.OpLess32U,
	opAndType{OLT, types.Int64}:   ssa.OpLess64,
	opAndType{OLT, types.Uint64}:  ssa.OpLess64U,
	opAndType{OLT, types.Float64}: ssa.OpLess64F,
	opAndType{OLT, types.Float32}: ssa.OpLess32F,

	opAndType{OGT, types.Int8}:    ssa.OpGreater8,
	opAndType{OGT, types.Uint8}:   ssa.OpGreater8U,
	opAndType{OGT, types.Int16}:   ssa.OpGreater16,
	opAndType{OGT, types.Uint16}:  ssa.OpGreater16U,
	opAndType{OGT, types.Int32}:   ssa.OpGreater32,
	opAndType{OGT, types.Uint32}:  ssa.OpGreater32U,
	opAndType{OGT, types.Int64}:   ssa.OpGreater64,
	opAndType{OGT, types.Uint64}:  ssa.OpGreater64U,
	opAndType{OGT, types.Float64}: ssa.OpGreater64F,
	opAndType{OGT, types.Float32}: ssa.OpGreater32F,

	opAndType{OLE, types.Int8}:    ssa.OpLeq8,
	opAndType{OLE, types.Uint8}:   ssa.OpLeq8U,
	opAndType{OLE, types.Int16}:   ssa.OpLeq16,
	opAndType{OLE, types.Uint16}:  ssa.OpLeq16U,
	opAndType{OLE, types.Int32}:   ssa.OpLeq32,
	opAndType{OLE, types.Uint32}:  ssa.OpLeq32U,
	opAndType{OLE, types.Int64}:   ssa.OpLeq64,
	opAndType{OLE, types.Uint64}:  ssa.OpLeq64U,
	opAndType{OLE, types.Float64}: ssa.OpLeq64F,
	opAndType{OLE, types.Float32}: ssa.OpLeq32F,

	opAndType{OGE, types.Int8}:    ssa.OpGeq8,
	opAndType{OGE, types.Uint8}:   ssa.OpGeq8U,
	opAndType{OGE, types.Int16}:   ssa.OpGeq16,
	opAndType{OGE, types.Uint16}:  ssa.OpGeq16U,
	opAndType{OGE, types.Int32}:   ssa.OpGeq32,
	opAndType{OGE, types.Uint32}:  ssa.OpGeq32U,
	opAndType{OGE, types.Int64}:   ssa.OpGeq64,
	opAndType{OGE, types.Uint64}:  ssa.OpGeq64U,
	opAndType{OGE, types.Float64}: ssa.OpGeq64F,
	opAndType{OGE, types.Float32}: ssa.OpGeq32F,

	opAndType{OLROT, types.Uint8}:  ssa.OpLrot8,
	opAndType{OLROT, types.Uint16}: ssa.OpLrot16,
	opAndType{OLROT, types.Uint32}: ssa.OpLrot32,
	opAndType{OLROT, types.Uint64}: ssa.OpLrot64,

	opAndType{OSQRT, types.Float64}: ssa.OpSqrt,
}

func (s *state) ssaOp(op NodeOp, t *Type) ssa.Op {
	/*etype := s.concreteEtype(t)
	x, ok := opToSSA[opAndType{op, etype}]
	if !ok {
		//s.Unimplementedf("unhandled binary op %s %s", opnames[op], Econv(int(etype), 0))
	}
	return x*/
	return opToSSA[opAndType{}]
}

func floatForComplex(t *Type) *Type {
	if t.Size() == 8 {
		return Typ[types.Float32]
	} else {
		return Typ[types.Float64]
	}
}

type opAndTwoTypes struct {
	op     NodeOp
	etype1 types.BasicKind
	etype2 types.BasicKind
}

type twoTypes struct {
	etype1 types.BasicKind
	etype2 types.BasicKind
}

type twoOpsAndType struct {
	op1              ssa.Op
	op2              ssa.Op
	intermediateType types.BasicKind
}

var fpConvOpToSSA = map[twoTypes]twoOpsAndType{

	twoTypes{types.Int8, types.Float32}:  twoOpsAndType{ssa.OpSignExt8to32, ssa.OpCvt32to32F, types.Int32},
	twoTypes{types.Int16, types.Float32}: twoOpsAndType{ssa.OpSignExt16to32, ssa.OpCvt32to32F, types.Int32},
	twoTypes{types.Int32, types.Float32}: twoOpsAndType{ssa.OpCopy, ssa.OpCvt32to32F, types.Int32},
	twoTypes{types.Int64, types.Float32}: twoOpsAndType{ssa.OpCopy, ssa.OpCvt64to32F, types.Int64},

	twoTypes{types.Int8, types.Float64}:  twoOpsAndType{ssa.OpSignExt8to32, ssa.OpCvt32to64F, types.Int32},
	twoTypes{types.Int16, types.Float64}: twoOpsAndType{ssa.OpSignExt16to32, ssa.OpCvt32to64F, types.Int32},
	twoTypes{types.Int32, types.Float64}: twoOpsAndType{ssa.OpCopy, ssa.OpCvt32to64F, types.Int32},
	twoTypes{types.Int64, types.Float64}: twoOpsAndType{ssa.OpCopy, ssa.OpCvt64to64F, types.Int64},

	twoTypes{types.Float32, types.Int8}:  twoOpsAndType{ssa.OpCvt32Fto32, ssa.OpTrunc32to8, types.Int32},
	twoTypes{types.Float32, types.Int16}: twoOpsAndType{ssa.OpCvt32Fto32, ssa.OpTrunc32to16, types.Int32},
	twoTypes{types.Float32, types.Int32}: twoOpsAndType{ssa.OpCvt32Fto32, ssa.OpCopy, types.Int32},
	twoTypes{types.Float32, types.Int64}: twoOpsAndType{ssa.OpCvt32Fto64, ssa.OpCopy, types.Int64},

	twoTypes{types.Float64, types.Int8}:  twoOpsAndType{ssa.OpCvt64Fto32, ssa.OpTrunc32to8, types.Int32},
	twoTypes{types.Float64, types.Int16}: twoOpsAndType{ssa.OpCvt64Fto32, ssa.OpTrunc32to16, types.Int32},
	twoTypes{types.Float64, types.Int32}: twoOpsAndType{ssa.OpCvt64Fto32, ssa.OpCopy, types.Int32},
	twoTypes{types.Float64, types.Int64}: twoOpsAndType{ssa.OpCvt64Fto64, ssa.OpCopy, types.Int64},
	// unsigned
	twoTypes{types.Uint8, types.Float32}:  twoOpsAndType{ssa.OpZeroExt8to32, ssa.OpCvt32to32F, types.Int32},
	twoTypes{types.Uint16, types.Float32}: twoOpsAndType{ssa.OpZeroExt16to32, ssa.OpCvt32to32F, types.Int32},
	twoTypes{types.Uint32, types.Float32}: twoOpsAndType{ssa.OpZeroExt32to64, ssa.OpCvt64to32F, types.Int64}, // go wide to dodge unsigned
	twoTypes{types.Uint64, types.Float32}: twoOpsAndType{ssa.OpCopy, ssa.OpInvalid, types.Uint64},            // Cvt64Uto32F, branchy code expansion instead

	twoTypes{types.Uint8, types.Float64}:  twoOpsAndType{ssa.OpZeroExt8to32, ssa.OpCvt32to64F, types.Int32},
	twoTypes{types.Uint16, types.Float64}: twoOpsAndType{ssa.OpZeroExt16to32, ssa.OpCvt32to64F, types.Int32},
	twoTypes{types.Uint32, types.Float64}: twoOpsAndType{ssa.OpZeroExt32to64, ssa.OpCvt64to64F, types.Int64}, // go wide to dodge unsigned
	twoTypes{types.Uint64, types.Float64}: twoOpsAndType{ssa.OpCopy, ssa.OpInvalid, types.Uint64},            // Cvt64Uto64F, branchy code expansion instead

	twoTypes{types.Float32, types.Uint8}:  twoOpsAndType{ssa.OpCvt32Fto32, ssa.OpTrunc32to8, types.Int32},
	twoTypes{types.Float32, types.Uint16}: twoOpsAndType{ssa.OpCvt32Fto32, ssa.OpTrunc32to16, types.Int32},
	twoTypes{types.Float32, types.Uint32}: twoOpsAndType{ssa.OpCvt32Fto64, ssa.OpTrunc64to32, types.Int64}, // go wide to dodge unsigned
	twoTypes{types.Float32, types.Uint64}: twoOpsAndType{ssa.OpInvalid, ssa.OpCopy, types.Uint64},          // Cvt32Fto64U, branchy code expansion instead

	twoTypes{types.Float64, types.Uint8}:  twoOpsAndType{ssa.OpCvt64Fto32, ssa.OpTrunc32to8, types.Int32},
	twoTypes{types.Float64, types.Uint16}: twoOpsAndType{ssa.OpCvt64Fto32, ssa.OpTrunc32to16, types.Int32},
	twoTypes{types.Float64, types.Uint32}: twoOpsAndType{ssa.OpCvt64Fto64, ssa.OpTrunc64to32, types.Int64}, // go wide to dodge unsigned
	twoTypes{types.Float64, types.Uint64}: twoOpsAndType{ssa.OpInvalid, ssa.OpCopy, types.Uint64},          // Cvt64Fto64U, branchy code expansion instead

	// float
	twoTypes{types.Float64, types.Float32}: twoOpsAndType{ssa.OpCvt64Fto32F, ssa.OpCopy, types.Float32},
	twoTypes{types.Float64, types.Float64}: twoOpsAndType{ssa.OpCopy, ssa.OpCopy, types.Float64},
	twoTypes{types.Float32, types.Float32}: twoOpsAndType{ssa.OpCopy, ssa.OpCopy, types.Float32},
	twoTypes{types.Float32, types.Float64}: twoOpsAndType{ssa.OpCvt32Fto64F, ssa.OpCopy, types.Float64},
}

var shiftOpToSSA = map[opAndTwoTypes]ssa.Op{
	opAndTwoTypes{OLSH, types.Int8, types.Uint8}:   ssa.OpLsh8x8,
	opAndTwoTypes{OLSH, types.Uint8, types.Uint8}:  ssa.OpLsh8x8,
	opAndTwoTypes{OLSH, types.Int8, types.Uint16}:  ssa.OpLsh8x16,
	opAndTwoTypes{OLSH, types.Uint8, types.Uint16}: ssa.OpLsh8x16,
	opAndTwoTypes{OLSH, types.Int8, types.Uint32}:  ssa.OpLsh8x32,
	opAndTwoTypes{OLSH, types.Uint8, types.Uint32}: ssa.OpLsh8x32,
	opAndTwoTypes{OLSH, types.Int8, types.Uint64}:  ssa.OpLsh8x64,
	opAndTwoTypes{OLSH, types.Uint8, types.Uint64}: ssa.OpLsh8x64,

	opAndTwoTypes{OLSH, types.Int16, types.Uint8}:   ssa.OpLsh16x8,
	opAndTwoTypes{OLSH, types.Uint16, types.Uint8}:  ssa.OpLsh16x8,
	opAndTwoTypes{OLSH, types.Int16, types.Uint16}:  ssa.OpLsh16x16,
	opAndTwoTypes{OLSH, types.Uint16, types.Uint16}: ssa.OpLsh16x16,
	opAndTwoTypes{OLSH, types.Int16, types.Uint32}:  ssa.OpLsh16x32,
	opAndTwoTypes{OLSH, types.Uint16, types.Uint32}: ssa.OpLsh16x32,
	opAndTwoTypes{OLSH, types.Int16, types.Uint64}:  ssa.OpLsh16x64,
	opAndTwoTypes{OLSH, types.Uint16, types.Uint64}: ssa.OpLsh16x64,

	opAndTwoTypes{OLSH, types.Int32, types.Uint8}:   ssa.OpLsh32x8,
	opAndTwoTypes{OLSH, types.Uint32, types.Uint8}:  ssa.OpLsh32x8,
	opAndTwoTypes{OLSH, types.Int32, types.Uint16}:  ssa.OpLsh32x16,
	opAndTwoTypes{OLSH, types.Uint32, types.Uint16}: ssa.OpLsh32x16,
	opAndTwoTypes{OLSH, types.Int32, types.Uint32}:  ssa.OpLsh32x32,
	opAndTwoTypes{OLSH, types.Uint32, types.Uint32}: ssa.OpLsh32x32,
	opAndTwoTypes{OLSH, types.Int32, types.Uint64}:  ssa.OpLsh32x64,
	opAndTwoTypes{OLSH, types.Uint32, types.Uint64}: ssa.OpLsh32x64,

	opAndTwoTypes{OLSH, types.Int64, types.Uint8}:   ssa.OpLsh64x8,
	opAndTwoTypes{OLSH, types.Uint64, types.Uint8}:  ssa.OpLsh64x8,
	opAndTwoTypes{OLSH, types.Int64, types.Uint16}:  ssa.OpLsh64x16,
	opAndTwoTypes{OLSH, types.Uint64, types.Uint16}: ssa.OpLsh64x16,
	opAndTwoTypes{OLSH, types.Int64, types.Uint32}:  ssa.OpLsh64x32,
	opAndTwoTypes{OLSH, types.Uint64, types.Uint32}: ssa.OpLsh64x32,
	opAndTwoTypes{OLSH, types.Int64, types.Uint64}:  ssa.OpLsh64x64,
	opAndTwoTypes{OLSH, types.Uint64, types.Uint64}: ssa.OpLsh64x64,

	opAndTwoTypes{ORSH, types.Int8, types.Uint8}:   ssa.OpRsh8x8,
	opAndTwoTypes{ORSH, types.Uint8, types.Uint8}:  ssa.OpRsh8Ux8,
	opAndTwoTypes{ORSH, types.Int8, types.Uint16}:  ssa.OpRsh8x16,
	opAndTwoTypes{ORSH, types.Uint8, types.Uint16}: ssa.OpRsh8Ux16,
	opAndTwoTypes{ORSH, types.Int8, types.Uint32}:  ssa.OpRsh8x32,
	opAndTwoTypes{ORSH, types.Uint8, types.Uint32}: ssa.OpRsh8Ux32,
	opAndTwoTypes{ORSH, types.Int8, types.Uint64}:  ssa.OpRsh8x64,
	opAndTwoTypes{ORSH, types.Uint8, types.Uint64}: ssa.OpRsh8Ux64,

	opAndTwoTypes{ORSH, types.Int16, types.Uint8}:   ssa.OpRsh16x8,
	opAndTwoTypes{ORSH, types.Uint16, types.Uint8}:  ssa.OpRsh16Ux8,
	opAndTwoTypes{ORSH, types.Int16, types.Uint16}:  ssa.OpRsh16x16,
	opAndTwoTypes{ORSH, types.Uint16, types.Uint16}: ssa.OpRsh16Ux16,
	opAndTwoTypes{ORSH, types.Int16, types.Uint32}:  ssa.OpRsh16x32,
	opAndTwoTypes{ORSH, types.Uint16, types.Uint32}: ssa.OpRsh16Ux32,
	opAndTwoTypes{ORSH, types.Int16, types.Uint64}:  ssa.OpRsh16x64,
	opAndTwoTypes{ORSH, types.Uint16, types.Uint64}: ssa.OpRsh16Ux64,

	opAndTwoTypes{ORSH, types.Int32, types.Uint8}:   ssa.OpRsh32x8,
	opAndTwoTypes{ORSH, types.Uint32, types.Uint8}:  ssa.OpRsh32Ux8,
	opAndTwoTypes{ORSH, types.Int32, types.Uint16}:  ssa.OpRsh32x16,
	opAndTwoTypes{ORSH, types.Uint32, types.Uint16}: ssa.OpRsh32Ux16,
	opAndTwoTypes{ORSH, types.Int32, types.Uint32}:  ssa.OpRsh32x32,
	opAndTwoTypes{ORSH, types.Uint32, types.Uint32}: ssa.OpRsh32Ux32,
	opAndTwoTypes{ORSH, types.Int32, types.Uint64}:  ssa.OpRsh32x64,
	opAndTwoTypes{ORSH, types.Uint32, types.Uint64}: ssa.OpRsh32Ux64,

	opAndTwoTypes{ORSH, types.Int64, types.Uint8}:   ssa.OpRsh64x8,
	opAndTwoTypes{ORSH, types.Uint64, types.Uint8}:  ssa.OpRsh64Ux8,
	opAndTwoTypes{ORSH, types.Int64, types.Uint16}:  ssa.OpRsh64x16,
	opAndTwoTypes{ORSH, types.Uint64, types.Uint16}: ssa.OpRsh64Ux16,
	opAndTwoTypes{ORSH, types.Int64, types.Uint32}:  ssa.OpRsh64x32,
	opAndTwoTypes{ORSH, types.Uint64, types.Uint32}: ssa.OpRsh64Ux32,
	opAndTwoTypes{ORSH, types.Int64, types.Uint64}:  ssa.OpRsh64x64,
	opAndTwoTypes{ORSH, types.Uint64, types.Uint64}: ssa.OpRsh64Ux64,
}

func (s *state) ssaShiftOp(op NodeOp, t *Type, u *Type) ssa.Op {
	return opToSSA[opAndType{}]
	/*etype1 := s.concreteEtype(t)
	etype2 := s.concreteEtype(u)
	x, ok := shiftOpToSSA[opAndTwoTypes{op, etype1, etype2}]
	if !ok {
		//s.Unimplementedf("unhandled shift op %s etype=%s/%s", opnames[op], Econv(int(etype1), 0), Econv(int(etype2), 0))
	}
	return x*/
}

func (s *state) ssaRotateOp(op NodeOp, t *Type) ssa.Op {
	return opToSSA[opAndType{}]
	/*etype1 := s.concreteEtype(t)
	x, ok := opToSSA[opAndType{op, etype1}]
	if !ok {
		//s.Unimplementedf("unhandled rotate op %s etype=%s", opnames[op], Econv(int(etype1), 0))
	}
	return x*/
}

// expr converts the expression n to ssa, adds it to s and returns the ssa result.
func (s *state) expr(n *Node) *ssa.Value {
	// TODO
	return nil
}

// condBranch evaluates the boolean expression cond and branches to yes
// if cond is true and no if cond is false.
// This function is intended to handle && and || better than just calling
// s.expr(cond) and branching on the result.
func (s *state) condBranch(cond ast.Expr, yes, no *ssa.Block) {
	switch e := cond.(type) {
	case *ast.ParenExpr:
		s.condBranch(e.X, yes, no)
		return

	case *ast.BinaryExpr:
		switch e.Op {
		case token.LAND:
			ltrue := s.f.NewBlock(ssa.BlockPlain) // "cond.true"
			s.condBranch(e.X, ltrue, no)
			s.curBlock = ltrue
			s.condBranch(e.Y, yes, no)
			return

		case token.LOR:
			lfalse := s.f.NewBlock(ssa.BlockPlain) // "cond.false"
			s.condBranch(e.X, yes, lfalse)
			s.curBlock = lfalse
			s.condBranch(e.Y, yes, no)
			return
		}

	case *ast.UnaryExpr:
		if e.Op == token.NOT {
			s.condBranch(e.X, no, yes)
			return
		}
	}
	c := s.expr(NewNode(cond, s.ctx))
	b := s.endBlock()
	b.Kind = ssa.BlockIf
	b.Control = c
	b.Likely = 0
	b.AddEdgeTo(yes)
	b.AddEdgeTo(no)
}

//assign(left *Node, right *ssa.Value, wb bool) {
func (s *state) assignStmt(stmt *ast.AssignStmt) {
	/*if left.Op() == ONAME && isblank(left) {
		return
	}
	t := left.Type
	dowidth(t)
	if right == nil {
		// right == nil means use the zero value of the assigned type.
		if !canSSA(left) {
			// if we can't ssa this memory, treat it as just zeroing out the backing memory
			//addr := s.addr(left, false)
			if left.Op() == ONAME {
				s.vars[&memVar] = s.newValue1A(ssa.OpVarDef, ssa.TypeMem, left, s.mem())
			}
			s.vars[&memVar] = s.newValue2I(ssa.OpZero, ssa.TypeMem, t.Size(), addr, s.mem())
			return
		}
		right = s.zeroVal(t)
	}
	if left.Op() == ONAME && canSSA(left) {
		// Update variable assignment.
		//s.vars[left] = right
		s.addNamedValue(left, right)
		return
	}
	// not ssa-able.  Treat as a store.
	addr := s.addr(left, false)
	if left.Op() == ONAME {
		s.vars[&memVar] = s.newValue1A(ssa.OpVarDef, ssa.TypeMem, left, s.mem())
	}
	s.vars[&memVar] = s.newValue3I(ssa.OpStore, ssa.TypeMem, t.Size(), addr, right, s.mem())
	if wb {
		s.insertWB(left.Type, addr, left.Lineno())
	}*/
}

// zeroVal returns the zero value for type t.
func (s *state) zeroVal(t *Type) *ssa.Value {
	switch {
	case t.IsInteger():
		switch t.Size() {
		case 1:
			return s.constInt8(t, 0)
		case 2:
			return s.constInt16(t, 0)
		case 4:
			return s.constInt32(t, 0)
		case 8:
			return s.constInt64(t, 0)
		default:
			s.Fatalf("bad sized integer type %s", t)
		}
	case t.IsFloat():
		switch t.Size() {
		case 4:
			return s.constFloat32(t, 0)
		case 8:
			return s.constFloat64(t, 0)
		default:
			s.Fatalf("bad sized float type %s", t)
		}
	case t.IsComplex():
		switch t.Size() {
		case 8:
			z := s.constFloat32(Typ[types.Float32], 0)
			return s.entryNewValue2(ssa.OpComplexMake, t, z, z)
		case 16:
			z := s.constFloat64(Typ[types.Float64], 0)
			return s.entryNewValue2(ssa.OpComplexMake, t, z, z)
		default:
			s.Fatalf("bad sized complex type %s", t)
		}

	case t.IsString():
		return s.entryNewValue0A(ssa.OpConstString, t, "")
	case t.IsPtr():
		return s.entryNewValue0(ssa.OpConstNil, t)
	case t.IsBoolean():
		return s.constBool(false)
	case t.IsInterface():
		return s.entryNewValue0(ssa.OpConstInterface, t)
	case t.IsSlice():
		return s.entryNewValue0(ssa.OpConstSlice, t)
	}
	s.Unimplementedf("zero for type %v not implemented", t)
	return nil
}

type callKind int8

const (
	callNormal callKind = iota
	callDefer
	callGo
)

func (s *state) call(n *Node, k callKind) *ssa.Value {
	return nil
	/*var sym *Sym           // target symbol (if static)
	var closure *ssa.Value // ptr to closure to run (if dynamic)
	var codeptr *ssa.Value // ptr to target code (if dynamic)
	var rcvr *ssa.Value    // receiver to set
	fn := n.Left()
	switch n.Op() {
	case OCALLFUNC:
		if k == callNormal && fn.Op() == ONAME && fn.Class() == PFUNC {
			sym = fn.Sym
			break
		}
		closure = s.expr(fn)
		if closure == nil {
			return nil // TODO: remove when expr always returns non-nil
		}
	case OCALLMETH:
		if fn.Op() != ODOTMETH {
			Fatalf("OCALLMETH: n.Left() not an ODOTMETH: %v", fn)
		}
		if fn.Right().Op() != ONAME {
			Fatalf("OCALLMETH: n.Left().Right() not a ONAME: %v", fn.Right())
		}
		if k == callNormal {
			sym = fn.Right().Sym
			break
		}
		n2 := *fn.Right()
		n2.Class() = PFUNC
		closure = s.expr(&n2)
		// Note: receiver is already assigned in n.List, so we don't
		// want to set it here.
	case OCALLINTER:
		if fn.Op() != ODOTINTER {
			Fatalf("OCALLINTER: n.Left() not an ODOTINTER: %v", Oconv(int(fn.Op()), 0))
		}
		i := s.expr(fn.Left())
		itab := s.newValue1(ssa.OpITab, Types[TUINTPTR], i)
		itabidx := fn.Xoffset() + 3*int64(Widthptr) + 8 // offset of fun field in runtime.itab
		itab = s.newValue1I(ssa.OpOffPtr, Types[TUINTPTR], itabidx, itab)
		if k == callNormal {
			codeptr = s.newValue2(ssa.OpLoad, Types[TUINTPTR], itab, s.mem())
		} else {
			closure = itab
		}
		rcvr = s.newValue1(ssa.OpIData, Types[TUINTPTR], i)
	}
	dowidth(fn.Type)
	stksize := fn.Type.Argwid // includes receiver

	// Run all argument assignments.  The arg slots have already
	// been offset by the appropriate amount (+2*widthptr for go/defer,
	// +widthptr for interface calls).
	// For OCALLMETH, the receiver is set in these statements.
	s.stmtList(n.List)

	// Set receiver (for interface calls)
	if rcvr != nil {
		panic("interface calls not implemented")
	}

	// Defer/go args
	if k != callNormal {
		// Write argsize and closure (args to Newproc/Deferproc).
		//argsize := s.constInt32(Types[TUINT32], int32(stksize))
		//s.vars[&memVar] = s.newValue3I(ssa.OpStore, ssa.TypeMem, 4, s.sp, argsize, s.mem())
		//addr := s.entryNewValue1I(ssa.OpOffPtr, Ptrto(Types[TUINTPTR]), int64(Widthptr), s.sp)
		//s.vars[&memVar] = s.newValue3I(ssa.OpStore, ssa.TypeMem, int64(Widthptr), addr, closure, s.mem())
		stksize += 2 * int64(Widthptr)
	}

	// call target
	bNext := s.f.NewBlock(ssa.BlockPlain)
	var call *ssa.Value
	switch {
	case k == callDefer:
		call = s.newValue1(ssa.OpDeferCall, ssa.TypeMem, s.mem())
	case k == callGo:
		call = s.newValue1(ssa.OpGoCall, ssa.TypeMem, s.mem())
	case closure != nil:
		codeptr = s.newValue2(ssa.OpLoad, Types[TUINTPTR], closure, s.mem())
		call = s.newValue3(ssa.OpClosureCall, ssa.TypeMem, codeptr, closure, s.mem())
	case codeptr != nil:
		call = s.newValue2(ssa.OpInterCall, ssa.TypeMem, codeptr, s.mem())
	case sym != nil:
		call = s.newValue1A(ssa.OpStaticCall, ssa.TypeMem, sym, s.mem())
	default:
		//Fatalf("bad call type %s %v", opnames[n.Op()], n)
	}
	call.AuxInt = stksize // Call operations carry the argsize of the callee along with them

	// Finish call block
	//s.vars[&memVar] = call
	b := s.endBlock()
	b.Kind = ssa.BlockCall
	b.Control = call
	b.AddEdgeTo(bNext)

	// Read result from stack at the start of the fallthrough block
	s.startBlock(bNext)
	var titer Iter
	fp := Structfirst(&titer, Getoutarg(n.Left().Type))
	if fp == nil || k != callNormal {
		// call has no return value. Continue with the next statement.
		return nil
	}
	a := s.entryNewValue1I(ssa.OpOffPtr, Ptrto(fp.Type), fp.Width(), s.sp)
	return s.newValue2(ssa.OpLoad, fp.Type, a, call)*/
}

// etypesign returns the signed-ness of e, for integer/pointer etypes.
// -1 means signed, +1 means unsigned, 0 means non-integer/non-pointer.
func etypesign(e uint8) int8 {
	// switch e {
	// case TINT8, TINT16, TINT32, TINT64, TINT:
	// 	return -1
	// case TUINT8, TUINT16, TUINT32, TUINT64, TUINT, TUINTPTR, TUNSAFEPTR:
	// 	return +1
	// }
	return 0
}

// lookupSymbol is used to retrieve the symbol (Extern, Arg or Auto) used for a particular node.
// This improves the effectiveness of cse by using the same Aux values for the
// same symbols.
func (s *state) lookupSymbol(n *Node, sym interface{}) interface{} {
	switch sym.(type) {
	default:
		s.Fatalf("sym %v is of uknown type %T", sym, sym)
	case *ssa.ExternSymbol, *ssa.ArgSymbol, *ssa.AutoSymbol:
		// these are the only valid types
	}

	// if lsym, ok := s.varsyms[n]; ok {
	// 	return lsym
	// } else {
	// 	s.varsyms[n] = sym
	// 	return sym
	// }
	return nil
}

// addr converts the address of the expression n to SSA, adds it to s and returns the SSA result.
// The value that the returned Value represents is guaranteed to be non-nil.
// If bounded is true then this address does not require a nil check for its operand
// even if that would otherwise be implied.
func (s *state) addr(n *Node, bounded bool) *ssa.Value {
	return nil
	// t := Ptrto(n.Type())
	// switch n.Op() {
	// case ONAME:
	// 	switch n.Class() {
	// 	case PEXTERN:
	// 		panic("External variables are unsupported")
	// 	case PPARAM:
	// 		// parameter slot
	// 		v := s.decladdrs[n]
	// 		if v == nil {
	// 			if flag_race != 0 && n.String() == ".fp" {
	// 				s.Unimplementedf("race detector mishandles nodfp")
	// 			}
	// 			s.Fatalf("addr of undeclared ONAME %v. declared: %v", n, s.decladdrs)
	// 		}
	// 		return v
	// 	case PAUTO:
	// 		// We need to regenerate the address of autos
	// 		// at every use.  This prevents LEA instructions
	// 		// from occurring before the corresponding VarDef
	// 		// op and confusing the liveness analysis into thinking
	// 		// the variable is live at function entry.
	// 		// TODO: I'm not sure if this really works or we're just
	// 		// getting lucky.  We might need a real dependency edge
	// 		// between vardef and addr ops.
	// 		aux := &ssa.AutoSymbol{Typ: n.Type(), Node: n}
	// 		return s.newValue1A(ssa.OpAddr, t, aux, s.sp)
	// 	case PPARAMOUT: // Same as PAUTO -- cannot generate LEA early.
	// 		// ensure that we reuse symbols for out parameters so
	// 		// that cse works on their addresses
	// 		aux := s.lookupSymbol(n, &ssa.ArgSymbol{Typ: n.Type(), Node: n})
	// 		return s.newValue1A(ssa.OpAddr, t, aux, s.sp)
	// 	case PAUTO | PHEAP, PPARAM | PHEAP, PPARAMOUT | PHEAP, PPARAMREF:
	// 		return s.expr(n.Name().Heapaddr())
	// 	default:
	// 		s.Unimplementedf("variable address class %v not implemented", n.Class)
	// 		return nil
	// 	}
	// case OINDREG:
	// 	// indirect off a register
	// 	// used for storing/loading arguments/returns to/from callees
	// 	if int(n.Reg()) != Thearch.REGSP {
	// 		s.Unimplementedf("OINDREG of non-SP register %s in addr: %v", "n.Reg", n) //obj.Rconv(int(n.Reg)), n)
	// 		return nil
	// 	}
	// 	return s.entryNewValue1I(ssa.OpOffPtr, t, n.Xoffset(), s.sp)
	// case OINDEX:
	// 	if n.Left().Type().IsSlice() {
	// 		a := s.expr(n.Left())
	// 		i := s.expr(n.Right())
	// 		i = s.extendIndex(i)
	// 		len := s.newValue1(ssa.OpSliceLen, Types[TINT], a)
	// 		if !n.Bounded() {
	// 			s.boundsCheck(i, len)
	// 		}
	// 		p := s.newValue1(ssa.OpSlicePtr, t, a)
	// 		return s.newValue2(ssa.OpPtrIndex, t, p, i)
	// 	} else { // array
	// 		a := s.addr(n.Left(), bounded)
	// 		i := s.expr(n.Right())
	// 		i = s.extendIndex(i)
	// 		len := s.constInt(Types[TINT], n.Left().Type().Bound())
	// 		if !n.Bounded() {
	// 			s.boundsCheck(i, len)
	// 		}
	// 		et := n.Left().Type().Elem()
	// 		elemType := et.(*Type)
	// 		return s.newValue2(ssa.OpPtrIndex, Ptrto(elemType), a, i)
	// 	}
	// case OIND:
	// 	p := s.expr(n.Left())
	// 	if !bounded {
	// 		s.nilCheck(p)
	// 	}
	// 	return p
	// case ODOT:
	// 	p := s.addr(n.Left(), bounded)
	// 	return s.newValue2(ssa.OpAddPtr, t, p, s.constInt(Types[TINT], n.Xoffset()))
	// case ODOTPTR:
	// 	p := s.expr(n.Left())
	// 	if !bounded {
	// 		s.nilCheck(p)
	// 	}
	// 	return s.newValue2(ssa.OpAddPtr, t, p, s.constInt(Types[TINT], n.Xoffset()))
	// case OCLOSUREVAR:
	// 	return s.newValue2(ssa.OpAddPtr, t,
	// 		s.entryNewValue0(ssa.OpGetClosurePtr, Ptrto(Types[TUINT8])),
	// 		s.constInt(Types[TINT], n.Xoffset()))
	// case OPARAM:
	// 	p := n.Left()
	// 	if p.Op() != ONAME || !(p.Class() == PPARAM|PHEAP || p.Class() == PPARAMOUT|PHEAP) {
	// 		panic("OPARAM not of ONAME,{PPARAM,PPARAMOUT}|PHEAP")
	// 	}

	// 	// Recover original offset to address passed-in param value.
	// 	original_p := *p
	// 	//original_p.Xoffset() = n.Xoffset()
	// 	aux := &ssa.ArgSymbol{Typ: n.Type(), Node: &original_p}
	// 	return s.entryNewValue1A(ssa.OpAddr, t, aux, s.sp)
	// case OCONVNOP:
	// 	addr := s.addr(n.Left(), bounded)
	// 	return s.newValue1(ssa.OpCopy, t, addr) // ensure that addr has the right type

	// default:
	// 	s.Unimplementedf("unhandled addr %v", Oconv(int(n.Op()), 0))
	// 	return nil
	// }
}

// checkGoto checks that a goto from from to to does not
// jump into a block
func (s *state) checkGoto(from *Node, to *Node) {
	// TODO: determine if goto jumps into a block
	var block *ssa.Block
	if block != nil {
		s.Errorf("goto %v jumps into block starting at %v", "<checkGoto.lblName>", "<checkGoto.line#")
	}

}
