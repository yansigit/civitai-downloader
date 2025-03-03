package downloader

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/schollz/progressbar/v3"
)

func DownloadFile(outputPath string, url string) error {
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directories: %v", err)
	}

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch %s: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP error for %s: %v", url, resp.StatusCode)
	}

	header := resp.Header.Get("content-disposition")
	if header != "" {
		parts := strings.Split(header, "filename=")
		if len(parts) > 1 {
			outputPath = filepath.Join(filepath.Dir(outputPath), strings.Trim(parts[1], "\""))
		}
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %v", outputPath, err)
	}
	defer file.Close()

	bar := progressbar.NewOptions(
		int(resp.ContentLength),
		progressbar.OptionSetWidth(15),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionSetDescription("[Downloading] "),
		progressbar.OptionSetTheme(
			progressbar.Theme{
				Saucer:        "[green]=[reset]",
				SaucerHead:    "[green]>[reset]",
				SaucerPadding: " ",
				BarStart:      "|",
				BarEnd:        "|",
			}),
	)

	_, err = io.Copy(io.MultiWriter(file, bar), resp.Body)
	return err
}
