package tokencount

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pkoukk/tiktoken-go"
	tiktoken_loader "github.com/pkoukk/tiktoken-go-loader"
	"github.com/sirupsen/logrus"
)

const defaultEncoding = "o200k_base"

var (
	encoder     *tiktoken.Tiktoken
	encoderOnce sync.Once
	encoderErr  error
)

func initEncoder() {
	encoderOnce.Do(func() {
		tiktoken.SetBpeLoader(tiktoken_loader.NewOfflineLoader())
		encoder, encoderErr = tiktoken.GetEncoding(defaultEncoding)
		if encoderErr != nil {
			logrus.Errorf("failed to initialize tiktoken encoder: %v", encoderErr)
		}
	})
}

// CountTokens reads a file and returns the estimated token count.
func CountTokens(path string, info os.FileInfo) int64 {
	initEncoder()

	if info.IsDir() {
		return 0
	}

	size := info.Size()
	if size == 0 {
		return 0
	}

	ext := strings.ToLower(filepath.Ext(path))

	if isImageExt(ext) {
		return countImageTokensFromFile(path, info)
	}

	if isBinaryExt(ext) {
		return 0
	}

	return countTextTokens(path, info)
}

func isImageExt(ext string) bool {
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp":
		return true
	}
	return false
}

func isBinaryExt(ext string) bool {
	switch ext {
	case ".o", ".so", ".a", ".dll", ".dylib", ".exe", ".lib", ".obj",
		".class", ".jar", ".war",
		".pyc", ".pyo", ".pyd",
		".gz", ".zip", ".tar", ".tgz", ".bz2", ".xz", ".7z", ".rar", ".zst",
		".woff", ".woff2", ".ttf", ".otf", ".eot",
		".mp3", ".mp4", ".avi", ".mov", ".mkv", ".wav", ".flac", ".ogg", ".aac",
		".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".odt", ".epub",
		".sqlite", ".db", ".mdb",
		".ico", ".tiff", ".tif", ".svgz", ".heic",
		".bin", ".dat", ".pak", ".bundle", ".wasm", ".npz", ".npy", ".pkl",
		".lock":
		return true
	}
	return false
}
