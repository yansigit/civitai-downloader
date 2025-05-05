package progress

import (
	"fmt"
	"io"
	"os"

	"github.com/schollz/progressbar/v3"
	"golang.org/x/term"
)

// NewProgressBar creates a new progress bar with the specified total and description.
// The color parameter can be any ANSI color code (e.g., "green", "blue", "yellow", "red", etc.)
func NewProgressBar(total int64, description string, color ...string) *progressbar.ProgressBar {
	// Default to green if no color is provided
	barColor := "green"
	if len(color) > 0 && color[0] != "" {
		barColor = color[0]
	}

	return progressbar.NewOptions64(
		total,
		progressbar.OptionSetDescription(description),
		progressbar.OptionSetWidth(15),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        fmt.Sprintf("[%s]=[reset]", barColor),
			SaucerHead:    fmt.Sprintf("[%s]>[reset]", barColor),
			SaucerPadding: " ",
			BarStart:      "|",
			BarEnd:        "|",
		}),
	)
}

func NewProgressReader(reader io.Reader, bar *progressbar.ProgressBar) io.Reader {
	r := progressbar.NewReader(reader, bar)
	return &r
}

func IsTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}
