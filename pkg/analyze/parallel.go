package analyze

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/lamnguyenx/gtu/v2026/internal/common"
	"github.com/lamnguyenx/gtu/v2026/pkg/fs"
	"github.com/lamnguyenx/gtu/v2026/pkg/gitignore"
	"github.com/lamnguyenx/gtu/v2026/pkg/tokencount"
	log "github.com/sirupsen/logrus"
)

var concurrencyLimit = make(chan struct{}, 2*runtime.GOMAXPROCS(0))

var _ common.Analyzer = (*ParallelAnalyzer)(nil)

// ParallelAnalyzer implements Analyzer
type ParallelAnalyzer struct {
	BaseAnalyzer
}

// CreateAnalyzer returns Analyzer
func CreateAnalyzer() *ParallelAnalyzer {
	a := &ParallelAnalyzer{}
	a.Init()
	return a
}

// AnalyzeDir analyzes given path
func (a *ParallelAnalyzer) AnalyzeDir(
	path string, ignore common.ShouldDirBeIgnored, fileTypeFilter common.ShouldFileBeIgnored,
) fs.Item {
	a.ignoreDir = ignore
	a.ignoreFileType = fileTypeFilter

	go a.UpdateProgress()
	dir := a.processDir(path, nil)

	dir.BasePath = filepath.Dir(path)
	a.setCurrentDir(dir)
	a.wait.Wait()

	a.progressDoneChan <- struct{}{}
	a.doneChan.Broadcast()

	return dir
}

func (a *ParallelAnalyzer) processQueuedDir(path string, parent *Dir, result chan<- *Dir, giStack gitignore.Stack) {
	concurrencyLimit <- struct{}{}
	if a.IsCancelled() {
		<-concurrencyLimit
		result <- nil
		return
	}

	subdir := a.processDir(path, giStack)
	subdir.Parent = parent
	<-concurrencyLimit
	result <- subdir
}

func addSubDir(parent, child *Dir) {
	if child != nil {
		parent.AddFile(child)
	}
}

func (a *ParallelAnalyzer) processDir(path string, giStack gitignore.Stack) *Dir {
	var (
		file       fs.Item
		err        error
		totalUsage int64
		info       os.FileInfo
		subDirChan = make(chan *Dir)
		dirCount   int
	)

	a.wait.Add(1)

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
		if m, _ := gitignore.MergeFromDirWithEntries(path, files); m != nil {
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
			dirCount++

			go a.processQueuedDir(entryPath, dir, subDirChan, giStack)
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
					if !a.ignoreTokens {
						regularFile.Tokens = tokencount.CountTokens(entryPath, info)
					}
					setPlatformSpecificAttrs(regularFile, info)
				}
				totalUsage += file.GetUsage()
				dir.AddFile(file)
			}
		}
	}

	go func() {
		var sub *Dir

		for range dirCount {
			sub = <-subDirChan
			addSubDir(dir, sub)
		}

		a.wait.Done()
	}()

	a.progressCurrentItemName.Store(path)
	a.progressItemCount.Add(int64(len(files)))
	a.progressTotalUsage.Add(totalUsage)
	return dir
}

func getDirFlag(err error, items int) rune {
	switch {
	case err != nil:
		return '!'
	case items == 0:
		return 'e'
	default:
		return ' '
	}
}

func getFlag(f os.FileInfo) rune {
	if f.Mode()&os.ModeSymlink != 0 || f.Mode()&os.ModeSocket != 0 {
		return '@'
	}
	return ' '
}
