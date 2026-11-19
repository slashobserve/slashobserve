package specfiles

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const Ext = ".json"

var errOutsideDirectory = errors.New("the file links outside the provisioning directory")

type File struct {
	Path   string
	Folder string
}

var (
	dirEscaper   = strings.NewReplacer("%", "%25", "/", "%2F", "\\", "%5C")
	dirUnescaper = strings.NewReplacer("%2F", "/", "%5C", "\\", "%2E", ".", "%25", "%")
)

func DirName(folderTitle string) string {
	name := dirEscaper.Replace(folderTitle)
	if strings.HasPrefix(name, ".") {
		return "%2E" + name[1:]
	}
	return name
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

func Read(dir, rel string) ([]byte, error) {
	raw, err := readInRoot(dir, filepath.FromSlash(rel))
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		return raw, err
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, err
	}
	target, err := filepath.EvalSymlinks(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		return nil, err
	}
	inside, err := filepath.Rel(realDir, target)
	if err != nil || !filepath.IsLocal(inside) {
		return nil, fmt.Errorf("%s: %w", rel, errOutsideDirectory)
	}
	return readInRoot(realDir, inside)
}

func readInRoot(dir, rel string) ([]byte, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	return root.ReadFile(rel)
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
		if kind := d.Type(); !kind.IsRegular() && kind&fs.ModeSymlink == 0 {
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
