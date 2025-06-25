package main

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/tools/go/packages"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})))

	ctx := context.Background()
	if err := cmd().ExecuteContext(ctx); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func cmd() *cobra.Command {
	return &cobra.Command{
		Use:  "unused-sqlc-method [package path] [struct name] [project path]",
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), args[0], args[1], args[2])
		},
	}
}

func run(ctx context.Context, pkgPath, structName, pjPath string) error {
	// � 分析対象
	targetPkgPath := pkgPath       // ← struct が定義されている完全パス
	targetStructName := structName // ← struct 名
	analyzePath := pjPath          // ← 呼び出し調査範囲

	// �🧠 全体読み込み（呼び出し調査用）
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps,
		Fset: token.NewFileSet(),
		Env:  os.Environ(),
	}
	allPkgs, err := packages.Load(cfg, analyzePath)
	if err != nil {
		log.Fatal(err)
	}
	if packages.PrintErrors(allPkgs) > 0 {
		log.Fatal("load error")
	}

	// 🎯 struct の定義元パッケージを特定
	var targetStructObj *types.TypeName
	for _, pkg := range allPkgs {
		fmt.Printf("Analyzing package: %s\n", pkg.PkgPath)
		if pkg.PkgPath != targetPkgPath {
			continue
		}
		obj := pkg.Types.Scope().Lookup(targetStructName)
		if obj == nil {
			log.Fatalf("Struct %s not found in package %s", targetStructName, targetPkgPath)
		}
		named, ok := obj.Type().(*types.Named)
		if !ok {
			log.Fatalf("%s is not a named type", targetStructName)
		}
		targetStructObj = named.Obj()
		break
	}
	if targetStructObj == nil {
		log.Fatalf("Struct definition for %s not found", targetStructName)
	}

	// 📚 メソッド一覧取得
	methodSet := types.NewMethodSet(types.NewPointer(targetStructObj.Type()))
	// methodSet := types.NewMethodSet(targetStructObj.Type())
	methodMap := map[string]*types.Func{}
	calledMethods := map[string]bool{}

	fmt.Printf("Analyzing methods for struct: %s.%s\n", targetPkgPath, targetStructName)
	fmt.Printf("methodSet.Len() = %d\n", methodSet.Len())

	for i := 0; i < methodSet.Len(); i++ {
		sel := methodSet.At(i)
		fn := sel.Obj().(*types.Func)
		methodMap[fn.Name()] = fn
		fmt.Printf("Found method: %s.%s\n", targetStructName, fn.Name())
	}

	// 🔍 すべてのパッケージからメソッド呼び出しを探索
	for _, pkg := range allPkgs {
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

	// 📤 未使用メソッドの出力
	fmt.Println("Unused methods:")
	for name := range methodMap {
		if !calledMethods[name] {
			fmt.Printf("- %s\n", name)
		}
	}

	return nil

	// methods, err := getStructMethods(pkgPath, structName)
	// if err != nil {
	// 	return fmt.Errorf("failed to get struct methods: %w", err)
	// }

	// fmt.Printf("Found %d methods for struct %s:\n", len(methods), structName)
	// for _, method := range methods {
	// 	fmt.Printf("- %s\n", method)
	// }

	// usedMethods, err := findUsedMethods(pjPath, structName, methods)
	// if err != nil {
	// 	return fmt.Errorf("failed to find used methods: %w", err)
	// }

	// unusedMethods := []StructMethod{}
	// for _, method := range methods {
	// 	found := false
	// 	for _, used := range usedMethods {
	// 		if method == used {
	// 			found = true
	// 			break
	// 		}
	// 	}
	// 	if !found {
	// 		unusedMethods = append(unusedMethods, method)
	// 	}
	// }

	// fmt.Printf("\nUsed methods (%d):\n", len(usedMethods))
	// for _, method := range usedMethods {
	// 	fmt.Printf("✓ %s\n", method)
	// }

	// fmt.Printf("\nUnused methods (%d):\n", len(unusedMethods))
	// for _, method := range unusedMethods {
	// 	fmt.Printf("✗ %s\n", method)
	// }

	// return nil
}

// 🔧 ヘルパー: *T or T → T
func deref(t types.Type) types.Type {
	if ptr, ok := t.(*types.Pointer); ok {
		return ptr.Elem()
	}
	return t
}

type StructMethod struct {
	PackageName  string
	ReceiverName string
	MethodName   string
}

func (m StructMethod) String() string {
	return fmt.Sprintf("%s.%s.%s", m.PackageName, m.ReceiverName, m.MethodName)
}

func getStructMethods(pkgPath, structName string) ([]StructMethod, error) {
	var methods []StructMethod

	if err := filepath.Walk(pkgPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}

		ast.Inspect(node, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.FuncDecl:
				if x.Recv != nil && len(x.Recv.List) > 0 {
					recv := x.Recv.List[0]
					var recvTypeName string

					switch t := recv.Type.(type) {
					case *ast.StarExpr:
						if ident, ok := t.X.(*ast.Ident); ok {
							recvTypeName = ident.Name
						}
					case *ast.Ident:
						recvTypeName = t.Name
					}

					slog.Debug(
						"method decl",
						slog.Any("recv name", recvTypeName),
						slog.Any("method name", x.Name.Name),
					)

					if recvTypeName == structName {
						methods = append(methods, StructMethod{
							PackageName:  node.Name.Name,
							ReceiverName: recvTypeName,
							MethodName:   x.Name.Name,
						})
					}
				}
			}
			return true
		})

		return nil
	}); err != nil {
		return nil, err
	}

	return methods, nil
}

func findUsedMethods(pjPath, structName string, methods []StructMethod) ([]StructMethod, error) {
	var usedMethods []StructMethod
	methodSet := make(map[string]bool)
	for _, method := range methods {
		methodSet[method.String()] = false
	}

	if err := filepath.Walk(pjPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}

		ast.Inspect(node, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CallExpr:
				if selector, ok := x.Fun.(*ast.SelectorExpr); ok {
					methodName := selector.Sel.Name

					for _, method := range methods {
						if method.MethodName == methodName {
							if !methodSet[method.String()] {
								methodSet[method.String()] = true
								usedMethods = append(usedMethods, method)
								slog.Debug(
									"found method usage",
									slog.Any("method", method.String()),
									slog.Any("file", path),
								)
							}
						}
					}
				}
			}
			return true
		})

		return nil
	}); err != nil {
		return nil, err
	}

	return usedMethods, nil
}
