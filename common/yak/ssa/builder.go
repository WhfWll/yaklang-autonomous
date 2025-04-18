package ssa

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"reflect"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/sca/dxtypes"

	"github.com/yaklang/yaklang/common/utils"

	"github.com/yaklang/yaklang/common/utils/memedit"

	"github.com/yaklang/yaklang/common/log"
)

type ParentScope struct {
	scope ScopeIF
	next  *ParentScope
}

func (p *ParentScope) Create(scope ScopeIF) *ParentScope {
	return &ParentScope{
		scope: scope,
		next:  p,
	}
}

// Function builder API
type FunctionBuilder struct {
	*Function

	ctx context.Context

	// do not use it directly
	_editor *memedit.MemEditor

	// disable free-value
	SupportClosure bool

	IncludeStack *utils.Stack[string]

	Included bool
	IsReturn bool

	RefParameter map[string]struct{ Index int }

	target *target // for break and continue
	labels map[string]*BasicBlock
	// defer function call

	// for build
	CurrentBlock *BasicBlock     // current block to build
	CurrentRange memedit.RangeIf // current position in source code
	CurrentFile  string          // current file name

	parentScope *ParentScope

	DefineFunc map[string]any

	MarkedFuncName  string
	MarkedFuncType  *FunctionType
	MarkedFunctions []*Function

	MarkedVariable           *Variable
	MarkedThisObject         Value
	MarkedThisClassBlueprint *Blueprint

	MarkedMemberCallWantMethod bool
	parentBuilder              *FunctionBuilder

	//External variables acquired by use will determine whether sideEffect should be generated when assign variable is assigned
	captureFreeValue map[string]struct{}
}

func NewBuilder(editor *memedit.MemEditor, f *Function, parent *FunctionBuilder) *FunctionBuilder {
	b := &FunctionBuilder{
		_editor:          editor,
		Function:         f,
		target:           &target{},
		labels:           make(map[string]*BasicBlock),
		CurrentBlock:     nil,
		CurrentRange:     nil,
		parentBuilder:    parent,
		RefParameter:     make(map[string]struct{ Index int }),
		IncludeStack:     utils.NewStack[string](),
		captureFreeValue: make(map[string]struct{}),
	}
	if parent != nil {
		b.DefineFunc = parent.DefineFunc
		b.MarkedThisObject = parent.MarkedThisObject
		// sub scope
		// b.parentScope = parent.CurrentBlock.ScopeTable
		b.parentScope = parent.parentScope.Create(parent.CurrentBlock.ScopeTable)
		b.SetBuildSupport(parent)

		b.SupportClosure = parent.SupportClosure
		// b.SupportClassStaticModifier = parent.SupportClassStaticModifier
		// b.SupportClass = parent.SupportClass
		b.ctx = parent.ctx
	}

	// b.ScopeStart()
	// b.Function.SetScope(b.CurrentScope)
	var ok bool
	b.CurrentBlock, ok = ToBasicBlock(f.EnterBlock)
	if !ok {
		log.Errorf("function (%v) enter block is not a basic block", f.name)
	}
	f.builder = b
	return b
}
func (f *FunctionBuilder) AddCaptureFreevalue(name string) {
	f.captureFreeValue[name] = struct{}{}
}
func (b *FunctionBuilder) GetFunc(name, pkg string) *Function {
	return b.GetProgram().GetFunction(name, pkg)
}

func (b *FunctionBuilder) SetBuildSupport(parent *FunctionBuilder) {
	if parent == nil {
		return
	}
	// b.SupportClass = parent.SupportClass
	// b.SupportClassStaticModifier = parent.SupportClassStaticModifier
	b.SupportClosure = parent.SupportClosure
}

func (b *FunctionBuilder) SetEditor(editor *memedit.MemEditor) {
	b._editor = editor
}

func (b *FunctionBuilder) GetEditor() *memedit.MemEditor {
	return b._editor
}

func (b *FunctionBuilder) GetLanguage() consts.Language {
	lang, err := consts.ValidateLanguage(b.GetProgram().Language)
	_ = err
	return lang
}

// current block is finish?
func (b *FunctionBuilder) IsBlockFinish() bool {
	return b.CurrentBlock.finish
}

// new function
func (b *FunctionBuilder) NewFunc(name string) *Function {
	var f *Function
	if b.SupportClosure {
		f = b.prog.NewFunctionWithParent(name, b.Function)
	} else {
		f = b.prog.NewFunctionWithParent(name, nil)
	}
	f.SetRange(b.CurrentRange)
	f.SetFunc(b.Function)
	f.SetBlock(b.CurrentBlock)
	return f
}

// function stack
func (b *FunctionBuilder) PushFunction(newFunc *Function) *FunctionBuilder {
	build := NewBuilder(b.GetEditor(), newFunc, b)
	// build.MarkedThisObject = b.MarkedThisObject
	if this := b.MarkedThisObject; this != nil {
		newParentScopeLevel := build.parentScope.scope
		newParentScopeLevel = newParentScopeLevel.CreateSubScope()
		// create this object and assign
		v := newParentScopeLevel.CreateVariable(this.GetName(), false)
		newParentScopeLevel.AssignVariable(v, this)
		// update parent  scope
		build.parentScope.scope = newParentScopeLevel
	}
	if b.MarkedThisClassBlueprint != nil {
		build.MarkedThisClassBlueprint = b.MarkedThisClassBlueprint
	}

	if build.CurrentRange == nil {
		build.CurrentRange = newFunc.R
	}

	return build
}

func (b *FunctionBuilder) PopFunction() *FunctionBuilder {
	// if global := b.GetProgram().GlobalScope; global != nil {
	// 	for i, m := range global.GetAllMember() {
	// 		name := i.String()
	// 		value := b.EmitPhi(name, []Value{m, b.PeekValue(name)})
	// 		global.SetStringMember(name, value)
	// 	}
	// }

	return b.parentBuilder
}

// handler current function

// function param
func (b FunctionBuilder) HandlerEllipsis() {
	if ins, ok := b.Params[len(b.Params)-1].(*Parameter); ins != nil {
		_ = ok
		ins.SetType(NewSliceType(CreateAnyType()))
	} else {
		log.Warnf("param contains (%T) cannot be set type and ellipsis", ins)
	}
	b.hasEllipsis = true
}

func (b *FunctionBuilder) EmitDefer(instruction Instruction) {
	deferBlock := b.GetDeferBlock()
	endBlock := b.CurrentBlock
	defer func() {
		b.CurrentBlock = endBlock
	}()
	b.CurrentBlock = deferBlock
	b.emitEx(instruction, func(instruction Instruction) {
		if c, flag := ToCall(instruction); flag {
			c.handlerGeneric()
			c.handlerObjectMethod()
			c.handlerReturnType()
			c.handleCalleeFunction()
		}
		if len(deferBlock.Insts) == 0 {
			deferBlock.Insts = append(deferBlock.Insts, instruction)
		} else {
			deferBlock.Insts = utils.InsertSliceItem(deferBlock.Insts, instruction, 0)
		}
	})
}

func (b *FunctionBuilder) SetMarkedFunction(name string) (ret func()) {
	originName := b.MarkedFuncName
	originType := b.MarkedFuncType
	ret = func() {
		b.MarkedFuncName = originName
		b.MarkedFuncType = originType
	}

	b.MarkedFuncName = name
	i, ok := b.DefineFunc[name]
	if !ok {
		return
	}
	// fun := b.BuildValueFromAny()
	typ := reflect.TypeOf(i)
	if typ.Kind() != reflect.Func {
		log.Errorf("config define function %s is not function", name)
		return
	}
	funTyp := b.CoverReflectFunctionType(typ, 0)
	b.MarkedFuncType = funTyp
	return
}

func (b *FunctionBuilder) GetMarkedFunction() *FunctionType {
	return b.MarkedFuncType
}

func (b *FunctionBuilder) ReferenceParameter(name string, index int) {
	b.RefParameter[name] = struct{ Index int }{Index: index}
}
func (b *FunctionBuilder) ClassConstructor(bluePrint *Blueprint, args []Value) Value {
	method := bluePrint.GetMagicMethod(Constructor)
	constructor := b.NewCall(method, args)
	b.EmitCall(constructor)
	desctructor := bluePrint.GetMagicMethod(Destructor)
	call := b.NewCall(desctructor, []Value{constructor})
	b.EmitDefer(call)
	return constructor
}
func (b *FunctionBuilder) GetStaticMember(classname *Blueprint, field string) *Variable {
	return b.CreateVariable(fmt.Sprintf("%s_%s", classname.Name, strings.TrimPrefix(field, "$")))
}

func (b *FunctionBuilder) GenerateDependence(pkgs []*dxtypes.Package, filename string) {
	container := b.ReadValue("__dependency__")
	if utils.IsNil(container) {
		log.Warnf("not found __dependency__")
		return
	}

	setDependencyRange := func(name string) {
		id := strings.Split(name, ":")
		if len(id) != 2 {
			return
		}
		group, artifact := id[0], id[1]
		rs1 := b.GetRangesByText(artifact)
		if len(rs1) == 1 {
			b.SetRangeByRangeIf(rs1[0])
			return
		}
		rs2 := b.GetRangesByText(group)
		if len(rs2) == 1 {
			b.SetRangeByRangeIf(rs2[0])
			return
		}
		b.SetEmptyRange()
	}
	/*
		__dependency__.name?{}
	*/
	b.SetEmptyRange()
	for _, pkg := range pkgs {
		sub := b.EmitEmptyContainer()
		// check item
		// 1. name
		// 2. version
		// 3. filename
		// 4. group
		// 5. artifact
		for k, v := range map[string]string{
			"name":     pkg.Name,
			"version":  pkg.Version,
			"filename": filename,
		} {
			if k == "name" {
				setDependencyRange(v)
			}
			b.AssignVariable(
				b.CreateMemberCallVariable(sub, b.EmitUndefined(k)),
				b.EmitConstInst(v),
			)
		}

		pkgItem := b.CreateMemberCallVariable(container, b.EmitUndefined(pkg.Name))
		b.AssignVariable(pkgItem, sub)
	}
}

func (b *FunctionBuilder) GenerateProjectConfig() {
	prog := b.GetProgram()
	if prog == nil {
		return
	}

	config := b.PeekValue(ProjectConfigVariable)
	if utils.IsNil(config) {
		return
	}
	backUp := b.GetEditor()
	defer b.SetEditor(backUp)

	for k, pc := range prog.ProjectConfig {
		cv := pc.ConfigValue
		if content, ok := prog.ExtraFile[pc.Filepath]; ok {
			var editor *memedit.MemEditor
			if len(content) <= 128 {
				hash := content
				editor, _ = ssadb.GetIrSourceFromHash(hash)
			} else {
				editor = memedit.NewMemEditorWithFileUrl(content, pc.Filepath)
			}
			b.SetEditor(editor)
			rng := b.GetRangesByText(k)
			if len(rng) == 1 {
				b.SetRangeByRangeIf(rng[0])
			} else {
				b.SetEmptyRange()
			}
			variable := b.CreateMemberCallVariable(config, b.EmitConstInst(k))
			b.AssignVariable(variable, b.EmitConstInst(cv))

			val := b.CreateVariable("test")
			b.AssignVariable(val, b.EmitConstInst(cv))
		}
	}
	return
}

func (b *FunctionBuilder) SetForceCapture(bo bool) {
	b.CurrentBlock.ScopeTable.SetForceCapture(bo)
}

func (b *FunctionBuilder) GetForceCapture() bool {
	return b.CurrentBlock.ScopeTable.GetForceCapture()
}
