package jmespath

import (
	"errors"
)

type Executor struct {
	intr     *treeInterpreter
	root     interface{}
}

func (ex *Executor) Execute(expr Value, item interface{}) (interface{}, error) {
	if !expr.IsExpression() {
		return nil, errors.New("Non-expression passed to Execute")
	}
	return ex.intr.execute(expr.expression.ref, item, ex.root)
}

type CustomFunction func(input []Value, executor *Executor) (interface{}, error)


var globalCustomFunctions map[string]CustomFunction

func assertGlobalCustomFunctionsInit() {
	if globalCustomFunctions == nil {
		globalCustomFunctions = make(map[string]CustomFunction)
	}
}


type customFunctionCaller struct {
	functionList map[string]CustomFunction
}

func newCustomFunctionCaller() *customFunctionCaller {
	fn_caller := &customFunctionCaller{}
	fn_caller.functionList = make(map[string]CustomFunction)

	assertGlobalCustomFunctionsInit()

	for name, fn := range globalCustomFunctions {
		fn_caller.functionList[name] = fn
	}

	return fn_caller
}

func (fc *customFunctionCaller) canCall(name string) bool {
	if _, ok := fc.functionList[name]; ok {
		return true
	} else {
		return false
	}
}

func (fc *customFunctionCaller) callFunction(name string, arguments []interface{}, intr *treeInterpreter, rootValue interface{}) (interface{}, error) {
	fn, ok := fc.functionList[name]
	if !ok {
		return nil, errors.New("Function not registered")
	}

	ex := &Executor {
		intr: intr,
		root: rootValue,
	}

	val_arguments := make([]Value, len(arguments))
	for i := 0; i < len(arguments); i++ {
		val_arguments[i] = *AsValue(arguments[i])
	}

	return fn(val_arguments, ex)
}


func RegisterFunction(name string, fn CustomFunction) error {
	assertGlobalCustomFunctionsInit()

	globalCustomFunctions[name] = fn
	return nil
}
