// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

// Package expr wraps github.com/expr-lang/expr in the small surface that the
// function-expr node and the change-node's expr value-type need: compile once
// at deploy, run per message. Compile errors carry line/column info so the
// editor can show useful status messages.
package expr

import (
	"errors"
	"fmt"

	exprlang "github.com/expr-lang/expr"
	"github.com/expr-lang/expr/file"
	"github.com/expr-lang/expr/vm"
)

// CompileError carries the position info that expr-lang attaches to syntax
// and type errors. Engines surface this so node-status messages can point at
// the offending line/column.
type CompileError struct {
	Line    int
	Column  int
	Message string
}

func (e *CompileError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("L%d:%d: %s", e.Line, e.Column, e.Message)
	}
	return e.Message
}

// Program is the compiled form of a single expression. Run is safe to call
// concurrently — the VM is the per-call object and `compiled` is read-only
// after Compile.
type Program struct {
	compiled *vm.Program
}

// Compile turns the user expression into a runnable Program. envShape declares
// the variables the expression will see at run time; expr-lang uses it for
// type-validation of field accesses (`payload.x`) and obvious type errors.
//
// Variables in envShape with a nil value are dropped — expr-lang treats them
// as type "unknown" and rejects every operator, which is too strict for our
// use case where `payload` may legitimately be any type. Such variables stay
// dynamic via AllowUndefinedVariables instead, so operations on them are
// checked at run time rather than compile time.
func Compile(expression string, envShape map[string]any) (*Program, error) {
	clean := make(map[string]any, len(envShape))
	for k, v := range envShape {
		if v != nil {
			clean[k] = v
		}
	}
	prog, err := exprlang.Compile(expression, exprlang.Env(clean), exprlang.AllowUndefinedVariables())
	if err != nil {
		var fe *file.Error
		if errors.As(err, &fe) {
			return nil, &CompileError{
				Line:    fe.Line,
				Column:  fe.Column,
				Message: fe.Message,
			}
		}
		return nil, &CompileError{Message: err.Error()}
	}
	return &Program{compiled: prog}, nil
}

// Run evaluates the program against the given env. Returns whatever the
// expression evaluates to; runtime errors (e.g. nil-deref on a missing field)
// are returned as plain Go errors.
func (p *Program) Run(env map[string]any) (any, error) {
	return vm.Run(p.compiled, env)
}
