package main

import (
	"os"
	"path/filepath"

	"tytui/pipeline"
)

// exportSVG reserves a unique file, never overwriting an earlier proof. A failed
// export removes its incomplete file; callers only receive a path after Close.
func exportSVG(result *pipeline.Result, directory string) (path string, err error) {
	f, err := os.CreateTemp(directory, "paragraph-*.svg")
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(f.Name())
		}
	}()
	if err = pipeline.WriteSVG(f, result); err != nil {
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
