package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"tytui/documentlayout"
	"tytui/pipeline"
)

// exportSVG reserves a unique file, never overwriting an earlier proof. A failed
// export removes its incomplete file; callers only receive a path after Close.
func exportSVG(result *pipeline.Result, directory string) (path string, err error) {
	return exportProof(directory, "paragraph-*.svg", func(w io.Writer) error { return pipeline.WriteSVG(w, result) })
}

func exportDocumentSVG(result *documentSnapshot, directory string) (string, error) {
	return exportProof(directory, "document-*.svg", func(w io.Writer) error {
		if result == nil {
			return fmt.Errorf("no retained document")
		}
		return documentlayout.WriteSVG(context.Background(), w, result.result, result.services)
	})
}

func exportProof(directory, pattern string, write func(io.Writer) error) (path string, err error) {
	f, err := os.CreateTemp(directory, pattern)
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(f.Name())
		}
	}()
	if err = write(f); err != nil {
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	return filepath.Clean(f.Name()), nil
}

type exportMsg struct {
	path string
	err  error
}
