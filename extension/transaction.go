package extension

import (
	"context"
	"fmt"
)

// Step lets a host coordinate changes across provider, tool, context, and
// skill registries while retaining reverse-order rollback.
type Step struct {
	Name     string
	Apply    func(context.Context) error
	Rollback func(context.Context) error
}

func ApplyTransaction(ctx context.Context, steps []Step) error {
	applied := make([]Step, 0, len(steps))
	for _, step := range steps {
		if step.Name == "" || step.Apply == nil {
			rollbackSteps(context.WithoutCancel(ctx), applied)
			return fmt.Errorf("extension transaction has an invalid step")
		}
		if err := ctx.Err(); err != nil {
			rollbackSteps(context.WithoutCancel(ctx), applied)
			return err
		}
		if err := step.Apply(ctx); err != nil {
			rollbackSteps(context.WithoutCancel(ctx), applied)
			return fmt.Errorf("apply extension step %q: %w", step.Name, err)
		}
		applied = append(applied, step)
	}
	return nil
}

func rollbackSteps(ctx context.Context, steps []Step) {
	for i := len(steps) - 1; i >= 0; i-- {
		if steps[i].Rollback != nil {
			_ = steps[i].Rollback(ctx)
		}
	}
}
