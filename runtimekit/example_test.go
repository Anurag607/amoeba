package runtimekit_test

import (
	"context"
	"fmt"

	"github.com/anurgosw/agentic-moe/runtimekit"
)

func ExampleRuntime_Plan() {
	cfg := runtimekit.DefaultConfig()
	runtime, err := runtimekit.New(cfg, runtimekit.Options{})
	if err != nil {
		panic(err)
	}
	defer func() { _ = runtime.Close() }()

	plan, err := runtime.Plan(context.Background(), "Review this API", runtimekit.RoutingInput{HasCodeContext: true})
	if err != nil {
		panic(err)
	}
	fmt.Println(plan.Expert.ID)
	// Output: coding
}
