package pipeline

import (
	"context"
	"fmt"

	"github.com/sealkit/dbseal/pkg/extension"
)

func init() {
	// Register the OSS default single-stream pipeline
	extension.SetPipeline(&SingleStreamPipeline{})
}

// SingleStreamPipeline is the open-source default streaming sync & slice pipeline.
type SingleStreamPipeline struct{}

func (p *SingleStreamPipeline) Name() string {
	return "oss-single-stream"
}

func (p *SingleStreamPipeline) Run(ctx context.Context, plan any) error {
	fmt.Printf("[dbseal OSS] Running single-stream streaming pipeline...\n")
	// TODO: implement single-stream sync and slice execution in Week 1-5
	return nil
}
