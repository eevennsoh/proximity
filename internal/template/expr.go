package template

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/expr-lang/expr"
)

// RenderExpr renders an Expr expression with the given environment and storage
func (r *Renderer) RenderExpr(exprStr string, env map[string]any, temporaryStorage map[string]string) ([]byte, error) {
	if temporaryStorage == nil {
		temporaryStorage = make(map[string]string)
	}

	// Create environment with custom functions
	options := []expr.Option{
		expr.Env(env),
		expr.Function("safeEncode", r.exprSafeEncode),
		expr.Function("trimStr", r.exprTrim),
		expr.Function("timestamp", r.exprTimestamp),
		expr.Function("formattedTimestamp", r.exprFormattedTimestamp),
		expr.Function("setToStorage", r.exprSetFunc(temporaryStorage)),
		expr.Function("getFromStorage", r.exprGetFunc(temporaryStorage)),
		// expr.Function("getFromMap", r.exprGetFromMap),
		expr.Function("type", r.exprType),
		expr.Function("has", r.exprHas),
		expr.Function("regexFind", r.exprRegexFind),
		expr.Function("regexReplace", r.exprRegexReplace),
		expr.Function("slauthtoken", r.exprSlauthToken),
		expr.Function("filterOutKeys", r.exprFilterOutKeys),
		expr.Function("merge", r.exprMerge),
		expr.Function("toCompactJson", r.exprToCompactJson),
		expr.Function("getIndex", r.exprGetIndex),
		// expr.Function("string", r.exprString),
		// expr.Function("req", r.exprReq) <- Make a request and return the result. This will be useful for forwarding requests for models to the ML endpoints
		// expr.Function("log", r.exprLog) <- Log something out using the Logger
	}

	program, err := expr.Compile(exprStr, options...)
	if err != nil {
		return nil, fmt.Errorf("expr compile error: %w", err)
	}

	output, err := expr.Run(program, env)
	if err != nil {
		return nil, fmt.Errorf("expr run error: %w", err)
	}

	return []byte(fmt.Sprint(output)), nil
}

// Expr helper functions

func (r *Renderer) exprSafeEncode(params ...any) (any, error) {
	if len(params) != 1 {
		return nil, fmt.Errorf("safeEncode expects 1 argument (input)")
	}

	return r.safeEncodeFn(params[0])
}

func (r *Renderer) exprTrim(params ...any) (any, error) {
	if len(params) != 3 {
		return nil, fmt.Errorf("trim expects 3 arguments (str, prefix, suffix)")
	}

	str := fmt.Sprint(params[0])
	prefix := fmt.Sprint(params[1])
	suffix := fmt.Sprint(params[2])

	return r.trimFn(str, prefix, suffix), nil
}

func (r *Renderer) exprTimestamp(params ...any) (any, error) {
	return r.timestampFn(), nil
}

func (r *Renderer) exprFormattedTimestamp(params ...any) (any, error) {
	if len(params) != 1 {
		return nil, fmt.Errorf("formattedTimestamp expects 1 argument (layout)")
	}

	layout := fmt.Sprint(params[0])

	return r.formattedTimestampFn(layout), nil
}

func (r *Renderer) exprSetFunc(temporaryStorage map[string]string) func(params ...any) (any, error) {
	return func(params ...any) (any, error) {
		if len(params) != 2 {
			return nil, fmt.Errorf("set expects 2 arguments (key, value)")
		}

		key := fmt.Sprint(params[0])
		val := params[1]

		return r.setFn(temporaryStorage)(key, val), nil
	}
}

func (r *Renderer) exprGetFunc(temporaryStorage map[string]string) func(params ...any) (any, error) {
	return func(params ...any) (any, error) {
		if len(params) != 1 {
			return nil, fmt.Errorf("get expects 1 argument (key)")
		}

		key := fmt.Sprint(params[0])

		return r.getFn(temporaryStorage)(key), nil
	}
}

func (r *Renderer) exprType(params ...any) (any, error) {
	if len(params) != 1 {
		return nil, fmt.Errorf("type expects 1 argument")
	}

	v := params[0]

	return r.getTypeFn(v), nil
}

func (r *Renderer) exprHas(params ...any) (any, error) {
	if len(params) != 2 {
		return nil, fmt.Errorf("has expects 2 arguments (obj, key)")
	}

	obj := params[0]
	key := fmt.Sprint(params[1])

	if obj == nil {
		return false, nil
	}

	v := reflect.ValueOf(obj)

	switch v.Kind() {
	case reflect.Map:
		return v.MapIndex(reflect.ValueOf(key)).IsValid(), nil
	case reflect.Struct:
		return v.FieldByName(key).IsValid(), nil
	default:
		return false, nil
	}
}

func (r *Renderer) exprRegexFind(params ...any) (any, error) {
	if len(params) != 2 {
		return nil, fmt.Errorf("regexFind expects 2 arguments (pattern, string)")
	}

	pattern := fmt.Sprint(params[0])
	s := fmt.Sprint(params[1])

	return r.regexFindFn(pattern, s)
}

func (r *Renderer) exprRegexReplace(params ...any) (any, error) {
	if len(params) != 3 {
		return nil, fmt.Errorf("regexReplace expects 3 arguments (pattern, replacement, string)")
	}

	pattern := fmt.Sprint(params[0])
	replacement := fmt.Sprint(params[1])
	s := fmt.Sprint(params[2])

	return r.regexReplaceFn(pattern, replacement, s)
}

func (r *Renderer) exprSlauthToken(params ...any) (any, error) {
	if len(params) != 3 {
		return nil, fmt.Errorf("slauthtoken expects 3 arguments (groups, audience, environment)")
	}

	groups := fmt.Sprint(params[0])
	audience := fmt.Sprint(params[1])
	environment := fmt.Sprint(params[2])

	return r.slauthtokenFn(groups, audience, environment)
}

func (r *Renderer) exprFilterOutKeys(params ...any) (any, error) {
	if len(params) != 2 {
		return nil, fmt.Errorf("filterOutKeys expects 2 arguments (map, keys)")
	}

	inputMap := params[0].(map[string]any)
	keys := params[1].([]any)

	keysSet := make(map[string]bool)

	for _, key := range keys {
		keysSet[fmt.Sprint(key)] = true
	}

	outputMap := make(map[string]any)

	for key, value := range inputMap {
		if !keysSet[key] {
			outputMap[key] = value
		}
	}

	return outputMap, nil
}

func (r *Renderer) exprMerge(params ...any) (any, error) {
	mergedMap := make(map[string]any)

	for _, param := range params {
		paramMap := param.(map[string]any)

		for key, value := range paramMap {
			if _, ok := mergedMap[key]; ok {
				return nil, fmt.Errorf("duplicate keys %s found", key)
			}

			mergedMap[key] = value
		}
	}

	return mergedMap, nil
}

func (r *Renderer) exprToCompactJson(params ...any) (any, error) {
	if len(params) != 1 {
		return nil, fmt.Errorf("toCompactJson expects 1 argument (map)")
	}

	jsonBytes, err := json.Marshal(params[0])
	if err != nil {
		return nil, fmt.Errorf("json marshal error: %w", err)
	}

	var buf bytes.Buffer

	if err := json.Compact(&buf, jsonBytes); err != nil {
		return nil, err
	}

	return buf.String(), nil
}

// func (r *Renderer) exprGetFromMap(params ...any) (any, error) {
// 	if len(params) != 2 {
// 		return nil, fmt.Errorf("getFromMap expects 2 arguments (map, key)")
// 	}

// 	fmt.Println("GET FROM MAP")

// 	inputMap := params[0].(map[string]any)
// 	key := params[1].(string)

// 	if val, ok := inputMap[key]; ok {
// 		return val, nil
// 	}

// 	return nil, nil
// }

// func (r *Renderer) exprString(params ...any) (any, error) {
// 	if len(params) != 1 {
// 		return nil, fmt.Errorf("string expects 1 argument")
// 	}
// 	v := params[0]
// 	jsonBytes, err := json.Marshal(v)
// 	if err != nil {
// 		return fmt.Sprintf("%v", v), nil
// 	}
// 	return string(jsonBytes), nil
// }

// exprGetIndex safely gets an element from an array by index
// Usage: getIndex(array, index) - returns nil if index is out of bounds
func (r *Renderer) exprGetIndex(params ...any) (any, error) {
	if len(params) != 2 {
		return nil, fmt.Errorf("getIndex expects 2 arguments (array, index)")
	}

	arr := params[0]
	if arr == nil {
		return nil, nil
	}

	index, ok := params[1].(int)
	if !ok {
		return nil, fmt.Errorf("getIndex: index must be an integer")
	}

	v := reflect.ValueOf(arr)

	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		return nil, fmt.Errorf("getIndex: first argument must be an array or slice")
	}

	if index < 0 || index >= v.Len() {
		return nil, nil
	}

	return v.Index(index).Interface(), nil
}
