package pipeline_test

import (
	"context"
	"fmt"

	"github.com/npillmayer/khipu/dimen"
	"tytui/pipeline"
)

func ExampleRun() {
	result, err := pipeline.Run(context.Background(), pipeline.Input{
		Text: "éa bb cc", Width: 12 * dimen.PT,
	})
	if err != nil {
		panic(err)
	}
	fmt.Println("lines:", result.Frozen.LineCount())
	fmt.Println("source byte endpoints:", result.BreakPositions)
	// Output:
	// lines: 3
	// source byte endpoints: [4 7 9]
}
