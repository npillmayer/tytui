package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"tytui/documentlayout"
)

// Frames deliberately have no CSS/style fields. Reject unknown fields instead
// of silently suggesting that frame typography can replace paragraph styles.
func loadFrames(path string) ([]documentlayout.FlowFrame, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	d := json.NewDecoder(f)
	d.DisallowUnknownFields()
	var frames []documentlayout.FlowFrame
	if err = d.Decode(&frames); err != nil {
		return nil, fmt.Errorf("frames: %w", err)
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("frames must contain exactly one JSON array")
	}
	if len(frames) == 0 {
		return nil, fmt.Errorf("frames must not be empty")
	}
	return frames, nil
}
