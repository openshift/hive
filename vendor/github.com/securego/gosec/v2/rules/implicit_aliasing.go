package rules

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/securego/gosec/v2"
	"github.com/securego/gosec/v2/issue"
)

type implicitAliasing struct {
	issue.MetaData
	aliases         map[*ast.Object]struct{}
	rightBrace      token.Pos
	acceptableAlias []*ast.UnaryExpr
}

func (r *implicitAliasing) ID() string {
	return r.MetaData.ID
}

func containsUnary(exprs []*ast.UnaryExpr, expr *ast.UnaryExpr) bool {
	for _, e := range exprs {
		if e == expr {
			return true
		}
	}
	return false
}

func getIdentExpr(expr ast.Expr) (*ast.Ident, bool) {
	return doGetIdentExpr(expr, false)
}

func doGetIdentExpr(expr ast.Expr, hasSelector bool) (*ast.Ident, bool) {
	switch node := expr.(type) {
	case *ast.Ident:
		return node, hasSelector
	case *ast.SelectorExpr:
		return doGetIdentExpr(node.X, true)
	case *ast.UnaryExpr:
		return doGetIdentExpr(node.X, hasSelector)
	default:
		return nil, false
	}
}

func (r *implicitAliasing) Match(n ast.Node, c *gosec.Context) (*issue.Issue, error) {
	// This rule does not apply for Go 1.22, see https://go.dev/doc/go1.22#language.
	major, minor, _ := gosec.GoVersion()
	if major >= 1 && minor >= 22 {
		return nil, nil
	}

	switch node := n.(type) {
	case *ast.RangeStmt:
		// When presented with a range statement, get the underlying Object bound to
		// by assignment and add it to our set (r.aliases) of objects to check for.
		if key, ok := node.Value.(*ast.Ident); ok {
			if key.Obj != nil {
				if assignment, ok := key.Obj.Decl.(*ast.AssignStmt); ok {
					if len(assignment.Lhs) < 2 {
						return nil, nil
					}

					if object, ok := assignment.Lhs[1].(*ast.Ident); ok {
						r.aliases[object.Obj] = struct{}{}

						if r.rightBrace < node.Body.Rbrace {
							r.rightBrace = node.Body.Rbrace
						}
					}
				}
			}
		}

	case *ast.UnaryExpr:
		// If this unary expression is outside of the last range statement we were looking at
		// then clear the list of objects we're concerned about because they're no longer in
		// scope
		if node.Pos() > r.rightBrace {
			r.aliases = make(map[*ast.Object]struct{})
			r.acceptableAlias = make([]*ast.UnaryExpr, 0)
		}

		// Short circuit logic to skip checking aliases if we have nothing to check against.
		if len(r.aliases) == 0 {
			return nil, nil
		}

		// If this unary is at the top level of a return statement then it is okay--
		// see *ast.ReturnStmt comment below.
		if containsUnary(r.acceptableAlias, node) {
			return nil, nil
		}

		// If we find a unary op of & (reference) of an object within r.aliases, complain.
		if identExpr, hasSelector := getIdentExpr(node); identExpr != nil && node.Op.String() == "&" {
			if _, contains := r.aliases[identExpr.Obj]; contains {
				_, isPointer := c.Info.TypeOf(identExpr).(*types.Pointer)

				if !hasSelector || !isPointer {
					return c.NewIssue(n, r.ID(), r.What, r.Severity, r.Confidence), nil
				}
			}
		}
	case *ast.ReturnStmt:
		// Returning a rangeStmt yielded value is acceptable since only one value will be returned
		for _, item := range node.Results {
			if unary, ok := item.(*ast.UnaryExpr); ok && unary.Op.String() == "&" {
				r.acceptableAlias = append(r.acceptableAlias, unary)
			}
		}
	}

	return nil, nil
}

// NewImplicitAliasing detects implicit memory aliasing of type: for blah := SomeCall() {... SomeOtherCall(&blah) ...}
func NewImplicitAliasing(id string, _ gosec.Config) (gosec.Rule, []ast.Node) {
	return &implicitAliasing{
		aliases:         make(map[*ast.Object]struct{}),
		rightBrace:      token.NoPos,
		acceptableAlias: make([]*ast.UnaryExpr, 0),
		MetaData: issue.MetaData{
			ID:         id,
			Severity:   issue.Medium,
			Confidence: issue.Medium,
			What:       "Implicit memory aliasing in for loop.",
		},
	}, []ast.Node{(*ast.RangeStmt)(nil), (*ast.UnaryExpr)(nil), (*ast.ReturnStmt)(nil)}
}

/*
This rule is prone to flag false positives.

Within GoSec, the rule is just an AST match-- there are a handful of other
implementation strategies which might lend more nuance to the rule at the
cost of allowing false negatives.

From a tooling side, I'd rather have this rule flag false positives than
potentially have some false negatives-- especially if the sentiment of this
rule (as I understand it, and Go) is that referencing a rangeStmt-yielded
value is kinda strange and does not have a strongly justified use case.

Which is to say-- a false positive _should_ just be changed.
*/
