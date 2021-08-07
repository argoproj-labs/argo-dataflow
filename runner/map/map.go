package _map

import (
	"context"
	"fmt"

	"github.com/argoproj-labs/argo-dataflow/sdks/golang"

	"github.com/antonmedv/expr"

	"github.com/argoproj-labs/argo-dataflow/runner/util"
)

func Exec(ctx context.Context, x string) error {
	prog, err := expr.Compile(x)
	if err != nil {
		return fmt.Errorf("failed to compile %q: %w", x, err)
	}
	return golang.StartWithContext(ctx, func(ctx context.Context, msg []byte) ([]byte, error) {
		res, err := expr.Run(prog, util.ExprEnv(msg))
		if err != nil {
			return nil, err
		}
		b, ok := res.([]byte)
		if !ok {
			return nil, fmt.Errorf("must return []byte")
		}
		return b, nil
	})
}
