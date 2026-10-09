package graph

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/SlashNephy/kotatsu/internal/auth"
)

// depthLimit は、クエリの深さが上限を超えたら、実行の前に拒否する (設計書 9.1)。
// 入れ子の深いクエリで、サーバーに大量の解決をさせないためである。
type depthLimit struct {
	max int
}

var (
	_ graphql.HandlerExtension        = depthLimit{max: 0}
	_ graphql.OperationContextMutator = depthLimit{max: 0}
)

func (depthLimit) ExtensionName() string { return "DepthLimit" }

func (depthLimit) Validate(graphql.ExecutableSchema) error { return nil }

func (l depthLimit) MutateOperationContext(_ context.Context, rc *graphql.OperationContext) *gqlerror.Error {
	if rc.Operation == nil {
		return nil
	}
	if depth := selectionDepth(rc.Operation.SelectionSet, rc.Doc.Fragments, map[string]bool{}); depth > l.max {
		err := gqlerror.Errorf("query depth %d exceeds the limit of %d", depth, l.max)
		err.Extensions = map[string]any{extensionCode: CodeQueryTooDeep}

		return err
	}

	return nil
}

// selectionDepth は、選択の入れ子の深さを返す。フラグメントは展開して数え、同じフラグメントの循環は 1 回だけたどる。
func selectionDepth(set ast.SelectionSet, fragments ast.FragmentDefinitionList, visiting map[string]bool) int {
	deepest := 0
	for _, selection := range set {
		var depth int
		switch s := selection.(type) {
		case *ast.Field:
			depth = 1 + selectionDepth(s.SelectionSet, fragments, visiting)
		case *ast.InlineFragment:
			depth = selectionDepth(s.SelectionSet, fragments, visiting)
		case *ast.FragmentSpread:
			fragment := fragments.ForName(s.Name)
			if fragment == nil || visiting[s.Name] {
				continue
			}
			visiting[s.Name] = true
			depth = selectionDepth(fragment.SelectionSet, fragments, visiting)
			delete(visiting, s.Name)
		}
		deepest = max(deepest, depth)
	}

	return deepest
}

// introspectionForViewer は、スキーマの公開 (introspection) を、ログイン済みのユーザーにだけ許す (設計書 9.1)。
type introspectionForViewer struct{}

var (
	_ graphql.HandlerExtension        = introspectionForViewer{}
	_ graphql.OperationContextMutator = introspectionForViewer{}
)

func (introspectionForViewer) ExtensionName() string { return "IntrospectionForViewer" }

func (introspectionForViewer) Validate(graphql.ExecutableSchema) error { return nil }

func (introspectionForViewer) MutateOperationContext(ctx context.Context, rc *graphql.OperationContext) *gqlerror.Error {
	rc.DisableIntrospection = auth.PrincipalFrom(ctx) == nil

	return nil
}
