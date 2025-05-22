package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
)

type FFProbeOutput struct {
	Streams []struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"streams"`
}

/*
Create a function getVideoAspectRatio(filePath string) (string, error) that takes a file path and returns the aspect ratio as a string.
- It should use exec.Command to run the same ffprobe command as above. In this case, the command is ffprobe and the arguments are -v, error, -print_format, json, -show_streams, and the file path.
- Set the resulting exec.Cmd's Stdout field to a pointer to a new bytes.Buffer.
- .Run() the command
- Unmarshal the stdout of the command from the buffer's .Bytes into a JSON struct so that you can get the width and height fields.
- I did a bit of math to determine the ratio, then returned one of three strings: 16:9, 9:16, or other.

** Aspect ratios might be slightly off due to rounding errors. You can use a tolerance range (or just use integer division and call it a day).
*/

func getVideoAspectRatio(filePath string) (string, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-print_format", "json",
		"-show_streams",
		filePath)

	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("error running ffprobe: %w", err)
	}

	var data FFProbeOutput
	if err := json.Unmarshal(out.Bytes(), &data); err != nil {
		return "", fmt.Errorf("error unmarshaling ffprobe output: %w", err)
	}

	if len(data.Streams) == 0 {
		return "", fmt.Errorf("no streams found in video file")
	}

	width := float64(data.Streams[0].Width)
	height := float64(data.Streams[0].Height)

	// Calculate aspect ratio
	ratio := width / height

	if math.Abs(ratio-16.0/9.0) < 0.1 {
		return "16:9", nil
	} else if math.Abs(ratio-9.0/16.0) < 0.1 {
		return "9:16", nil
	}

	return "other", nil
}
