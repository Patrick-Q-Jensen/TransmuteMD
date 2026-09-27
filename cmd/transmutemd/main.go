package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/analyze"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/app"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/cli"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/extract/pdf/pdfium"
	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/render/markdown"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	runner, err := cli.NewRunner(newConverter, os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "transmutemd: initialize command: %v\n", err)
		return cli.ExitConversion
	}
	return runner.Run(ctx, os.Args[1:])
}

func newConverter(ctx context.Context) (cli.Converter, io.Closer, error) {
	runtime, err := pdfium.NewRuntime(ctx)
	if err != nil {
		return nil, nil, err
	}

	extractor, err := pdfium.NewExtractor(runtime)
	if err != nil {
		return nil, nil, errors.Join(err, runtime.Close())
	}
	converter, err := app.NewConverter(
		extractor,
		analyze.NewBasicAnalyzer(),
		markdown.NewRenderer(),
	)
	if err != nil {
		return nil, nil, errors.Join(err, runtime.Close())
	}
	return converter, runtime, nil
}
