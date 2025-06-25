package main

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"slices"

	"github.com/spf13/cobra"
	"golang.org/x/tools/go/packages"
)

func main() {
	if err := cmd().ExecuteContext(context.Background()); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func cmd() *cobra.Command {
	var (
		fail          *bool
		ignoreMethods []string
	)
	c := &cobra.Command{
		Use:  "unused-sqlc-method [target package path] [target struct name]",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			unusedMethods, err := search(cmd.Context(), args[0], args[1])
			if err != nil {
				return err
			}

			// remove ignored methods
			if len(ignoreMethods) > 0 {
				ignoreSet := make(map[string]struct{}, len(ignoreMethods))
				for _, m := range ignoreMethods {
					ignoreSet[m] = struct{}{}
				}
				var filtered []string
				for _, method := range unusedMethods {
					if _, ok := ignoreSet[method]; !ok {
						filtered = append(filtered, method)
					}
				}
				unusedMethods = filtered
			}

			for _, method := range unusedMethods {
				fmt.Println(method)
			}

			if len(unusedMethods) > 0 && *fail {
				os.Exit(1)
			}

			return nil
		},
	}
	fail = c.Flags().Bool("fail", false, "Exit with non-zero status if unused methods are found")
	c.Flags().StringSliceVar(&ignoreMethods, "ignore", nil, "Methods to ignore check (comma-separated)")
	return c
}

func search(ctx context.Context, pkgPath, structName string) ([]string, error) {
	targetPkgPath := pkgPath       // ← struct が定義されている完全パス
	targetStructName := structName // ← struct 名

	pkgs, err := packages.Load(
		&packages.Config{
			Mode: packages.NeedName | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps,
			Fset: token.NewFileSet(),
			Env:  os.Environ(),
		},
		"./...",
	)
	if err != nil {
		return nil, err
	}

	methods, err := listMethods(pkgs, targetPkgPath, targetStructName)
	if err != nil {
		return nil, err
	}

	calledMethods := map[string]bool{}

	// 🔍 すべてのパッケージからメソッド呼び出しを探索
	for _, pkg := range pkgs {
		info := pkg.TypesInfo

		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				selection := info.Selections[sel]
				if selection == nil || selection.Kind() != types.MethodVal {
					return true
				}

				fn := selection.Obj().(*types.Func)
				recvType := fn.Type().(*types.Signature).Recv().Type()

				named, ok := deref(recvType).(*types.Named)
				if !ok {
					return true
				}

				if named.Obj().Name() == targetStructName && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == targetPkgPath {
					// 一致する struct のメソッド呼び出し
					calledMethods[fn.Name()] = true
				}
				return true
			})
		}
	}

	var unusedMethods []string
	for _, name := range methods {
		if !calledMethods[name] {
			unusedMethods = append(unusedMethods, name)
		}
	}

	return unusedMethods, nil
}

// *T|T -> T
func deref(t types.Type) types.Type {
	if ptr, ok := t.(*types.Pointer); ok {
		return ptr.Elem()
	}
	return t
}

func listMethods(pkgs []*packages.Package, pkgPath, structName string) ([]string, error) {
	var targetStructObj *types.TypeName
	for _, pkg := range pkgs {
		if pkg.PkgPath != pkgPath {
			continue
		}
		obj := pkg.Types.Scope().Lookup(structName)
		if obj == nil {
			return nil, fmt.Errorf("struct %s not found in package %s", structName, pkgPath)
		}
		named, ok := obj.Type().(*types.Named)
		if !ok {
			return nil, fmt.Errorf("%s is not a named type", structName)
		}
		targetStructObj = named.Obj()
		break
	}
	if targetStructObj == nil {
		return nil, fmt.Errorf("struct %s not found", structName)
	}

	var methods []string

	// *T methods
	ptrRcvMethodSet := types.NewMethodSet(types.NewPointer(targetStructObj.Type()))
	for i := 0; i < ptrRcvMethodSet.Len(); i++ {
		sel := ptrRcvMethodSet.At(i)
		fn := sel.Obj().(*types.Func)
		methods = append(methods, fn.Name())
	}

	// T methods
	valueRcvMethodSet := types.NewMethodSet(targetStructObj.Type())
	for i := 0; i < valueRcvMethodSet.Len(); i++ {
		sel := valueRcvMethodSet.At(i)
		fn := sel.Obj().(*types.Func)
		methods = append(methods, fn.Name())
	}

	slices.Sort(methods)
	return slices.Compact(methods), nil
}
