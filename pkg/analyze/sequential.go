package analyze

import (
	"os"
	"path/filepath"

	"github.com/dundee/gdu/v5/internal/common"
	"github.com/dundee/gdu/v5/pkg/fs"
	"github.com/dundee/gdu/v5/pkg/gitignore"
	"github.com/dundee/gdu/v5/pkg/tokencount"
	log "github.com/sirupsen/logrus"
)

// SequentialAnalyzer implements Analyzer
type SequentialAnalyzer struct {
	BaseAnalyzer
}

// CreateSeqAnalyzer returns Analyzer
func CreateSeqAnalyzer() *SequentialAnalyzer {
	a := &SequentialAnalyzer{}
	a.Init()
	return a
}

// AnalyzeDir analyzes given path
func (a *SequentialAnalyzer) AnalyzeDir(
	path string, ignore common.ShouldDirBeIgnored, fileTypeFilter common.ShouldFileBeIgnored,
) fs.Item {
	a.ignoreDir = ignore
	a.ignoreFileType = fileTypeFilter

	go a.UpdateProgress()
	var giStack gitignore.Stack
	if a.autoGitignore {
		if m, _ := gitignore.MergeFromDir(path); m != nil {
			giStack = giStack.Push(m)
		}
	}
	dir := a.processDir(path, giStack)

	dir.BasePath = filepath.Dir(path)
	a.setCurrentDir(dir)

	a.progressDoneChan <- struct{}{}
	a.doneChan.Broadcast()

	return dir
}

func (a *SequentialAnalyzer) processDir(path string, giStack gitignore.Stack) *Dir {
	var (
		file      fs.Item
		err       error
		totalSize int64
		info      os.FileInfo
	)

	files, err := os.ReadDir(path)
	if err != nil {
		log.Print(err.Error())
	}

	dir := &Dir{
		File: &File{
			Name: filepath.Base(path),
			Flag: getDirFlag(err, len(files)),
		},
		ItemCount: 1,
		Files:     make(fs.Files, 0, len(files)),
	}
	setDirPlatformSpecificAttrs(dir, path)

	// Load local .gitignore and push onto the stack for this subtree
	if a.autoGitignore {
		if m, _ := gitignore.MergeFromDir(path); m != nil {
			giStack = giStack.Push(m)
		}
	}

	for _, f := range files {
		if a.IsCancelled() {
			break
		}
		name := f.Name()
		entryPath := filepath.Join(path, name)
		if f.IsDir() {
			if a.shouldSkipDir(name, entryPath) {
				continue
			}
			if len(giStack) > 0 && giStack.Match(entryPath, true) {
				continue
			}

			subdir := a.processDir(entryPath, giStack)
			subdir.Parent = dir
			dir.AddFile(subdir)
		} else {
			// Apply file type filter if set
			if a.ignoreFileType != nil && a.ignoreFileType(name) {
				continue // Skip this file
			}

			// Apply auto-gitignore for files
			if len(giStack) > 0 && giStack.Match(entryPath, false) {
				continue
			}

			info, err = f.Info()
			if err != nil {
				log.Print(err.Error())
				dir.SetFlag('!')
				continue
			}

			if a.followSymlinks && info.Mode()&os.ModeSymlink != 0 {
				infoF, err := followSymlink(entryPath, a.gitAnnexedSize)
				if err != nil {
					log.Print(err.Error())
					dir.SetFlag('!')
					continue
				}
				if infoF != nil {
					info = infoF
				}
			}

			symlinkTarget := readSymlinkTarget(f.Type(), entryPath)

			// Apply time filter if set
			if a.matchesTimeFilterFn != nil && !a.matchesTimeFilterFn(info.ModTime()) {
				continue // Skip this file
			}

			switch {
			case a.archiveBrowsing && isZipFile(name):
				zipDir, err := processZipFile(entryPath, info)
				if err != nil {
					log.Printf("Failed to process zip file %s: %v", entryPath, err)
					file = &File{
						Name:   name,
						Flag:   getFlag(info),
						Size:   info.Size(),
						Parent: dir,
					}
				} else {
					uncompressedSize, compressedSize, err := getZipFileSize(entryPath)
					if err == nil {
						zipDir.Size = uncompressedSize
						zipDir.Usage = compressedSize
					}
					zipDir.Parent = dir
					file = zipDir
				}
			case a.archiveBrowsing && isTarFile(name):
				tarDir, err := processTarFile(entryPath, info)
				if err != nil {
					log.Printf("Failed to process tar file %s: %v", entryPath, err)
					file = &File{
						Name:   name,
						Flag:   getFlag(info),
						Size:   info.Size(),
						Parent: dir,
					}
				} else {
					tarDir.Parent = dir
					file = tarDir
				}
			default:
				file = &File{
					Name:    name,
					Flag:    getFlag(info),
					Size:    info.Size(),
					Parent:  dir,
					Symlink: symlinkTarget,
				}
			}

		if file != nil {
			// Only set platform-specific attributes for regular files
			if regularFile, ok := file.(*File); ok {
				regularFile.Tokens = tokencount.CountTokens(entryPath, info)
				setPlatformSpecificAttrs(regularFile, info)
			}
			totalSize += file.GetUsage()
			dir.AddFile(file)
			}
		}
	}

	a.progressCurrentItemName.Store(path)
	a.progressItemCount.Add(int64(len(files)))
	a.progressTotalUsage.Add(totalSize)
	return dir
}
