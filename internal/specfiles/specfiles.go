package specfiles

import (
	"io/fs"
	"path/filepath"
	"strings"
)

const Ext = ".json"

type File struct {
	Path   string
	Folder string
}

var (
	dirEscaper   = strings.NewReplacer("%", "%25", "/", "%2F")
	dirUnescaper = strings.NewReplacer("%2F", "/", "%25", "%")
)

func DirName(folderTitle string) string {
	return dirEscaper.Replace(folderTitle)
}

func FolderTitle(dirName string) string {
	return dirUnescaper.Replace(dirName)
}

func FileName(slug string) string {
	return slug + Ext
}

func FolderOf(path string) string {
	dir, _, nested := strings.Cut(path, "/")
	if !nested {
		return ""
	}
	return FolderTitle(dir)
}

func Scan(dir string) ([]File, error) {
	var files []File
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != dir && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), Ext) {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		files = append(files, File{Path: rel, Folder: FolderOf(rel)})
		return nil
	})
	return files, err
}
