package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
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
	// pkgPath 内にある structName のメソッド一覧を取得する

	// pjPath 内の Go ファイルを解析し、pkgPath の structName のメソッドが使用されているかどうかを確認する

	return nil
}
