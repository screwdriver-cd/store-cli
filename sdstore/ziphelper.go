package sdstore

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/screwdriver-cd/store-cli/logger"
)

var compressedFormats = map[string]struct{}{
	".7z":   {},
	".avi":  {},
	".bz2":  {},
	".cab":  {},
	".gif":  {},
	".gz":   {},
	".jar":  {},
	".jpeg": {},
	".jpg":  {},
	".lz":   {},
	".lzma": {},
	".mov":  {},
	".mp3":  {},
	".mp4":  {},
	".mpeg": {},
	".mpg":  {},
	".png":  {},
	".rar":  {},
	".tbz2": {},
	".tgz":  {},
	".txz":  {},
	".xz":   {},
	".zip":  {},
	".zipx": {},
}

// Zip is repurposed from https://github.com/mholt/archiver/pull/92/files
// To include support for symbolic links
func Zip(source, target string) error {
	zipfile, err := os.Create(target)
	if err != nil {
		return logger.Error(err)
	}
	defer zipfile.Close()

	w := zip.NewWriter(zipfile)
	defer func() { _ = w.Close() }()

	sourceInfo, err := os.Stat(source)
	if err != nil {
		return logger.Error(fmt.Errorf("%s: stat: %v", source, err))
	}

	var baseDir string
	if sourceInfo.IsDir() {
		baseDir = filepath.Base(source)
	}

	return filepath.Walk(source, func(fpath string, info os.FileInfo, err error) error {
		if err != nil {
			return logger.Error(fmt.Errorf("walking to %s: %v", fpath, err))
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return logger.Error(fmt.Errorf("%s: getting header: %v", fpath, err))
		}

		if baseDir != "" {
			name, err := filepath.Rel(source, fpath)
			if err != nil {
				return logger.Error(err)
			}
			header.Name = path.Join(baseDir, filepath.ToSlash(name))
		}

		if info.IsDir() {
			header.Name += "/"
			header.Method = zip.Store
		} else {
			ext := strings.ToLower(path.Ext(header.Name))
			if _, ok := compressedFormats[ext]; ok {
				header.Method = zip.Store
			} else {
				header.Method = zip.Deflate
			}
		}

		writer, err := w.CreateHeader(header)
		if err != nil {
			return logger.Error(fmt.Errorf("%s: making header: %v", fpath, err))
		}

		if info.IsDir() {
			return nil
		}

		if (header.Mode() & os.ModeSymlink) != 0 {
			linkTarget, err := os.Readlink(fpath)
			if err != nil {
				return logger.Error(fmt.Errorf("%s: readlink: %v", fpath, err))
			}
			_, err = writer.Write([]byte(filepath.ToSlash(linkTarget)))
			if err != nil {
				return logger.Error(fmt.Errorf("%s: writing symlink target: %v", fpath, err))
			}
			return nil
		}

		if header.Mode().IsRegular() {
			file, err := os.Open(fpath)
			if err != nil {
				return logger.Error(fmt.Errorf("%s: opening: %v", fpath, err))
			}
			defer file.Close()

			_, err = io.CopyN(writer, file, info.Size())
			if err != nil && err != io.EOF {
				return logger.Error(fmt.Errorf("%s: copying contents: %v", fpath, err))
			}
		}

		return nil
	})
}

// Unzip is repurposed from https://github.com/mholt/archiver/pull/92/files
// To include support for symbolic links
func Unzip(src string, dest string) ([]string, error) {
	var files []string
	type fileTime struct {
		path    string
		modtime time.Time
	}
	var filesTime []fileTime

	zr, err := zip.OpenReader(src)
	if err != nil {
		_ = logger.Error(err)
		return files, err
	}
	defer func() { _ = zr.Close() }()

	for _, file := range zr.File {
		fPath, fTime, err := func(file *zip.File) (string, fileTime, error) {
			var fPath string
			var fTime fileTime

			rc, err := file.Open()
			if err != nil {
				_ = logger.Error(err)
				return fPath, fTime, err
			}
			defer func() { _ = rc.Close() }()

			fPath = filepath.Join(dest, file.Name)

			// Check for ZipSlip. More Info: http://bit.ly/2MsjAWE
			if dest != "/" && !strings.HasPrefix(fPath, filepath.Clean(dest)+string(os.PathSeparator)) {
				msg := fmt.Errorf("%s: illegal file path", fPath)
				_ = logger.Error(msg)
				return fPath, fTime, msg
			}

			if file.FileInfo().IsDir() {
				_ = os.MkdirAll(fPath, os.ModePerm)
				fTime = fileTime{fPath, file.Modified}
			} else if (file.FileInfo().Mode() & os.ModeSymlink) != 0 {
				buffer := make([]byte, file.FileInfo().Size())
				size, err := rc.Read(buffer)
				if err != nil && err != io.EOF {
					_ = logger.Error(err)
					return fPath, fTime, err
				}
				target := string(buffer[:size])
				err = os.Symlink(target, fPath)
				if err != nil {
					_ = logger.Error(err)
					return fPath, fTime, err
				}
			} else {
				if err = os.MkdirAll(filepath.Dir(fPath), os.ModePerm); err != nil {
					_ = logger.Error(err)
					return fPath, fTime, err
				}

				outFile, err := os.OpenFile(fPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
				if err != nil {
					_ = logger.Error(err)
					return fPath, fTime, err
				}
				defer outFile.Close()

				_, err = io.Copy(outFile, rc)

				if err != nil {
					_ = logger.Error(err)
					return fPath, fTime, err
				}
				fTime = fileTime{fPath, file.Modified}
			}
			return fPath, fTime, nil
		}(file)

		if err != nil {
			_ = logger.Error(err)
			return files, err
		}
		files = append(files, fPath)
		filesTime = append(filesTime, fTime)
	}

	// sort longest first
	sort.Slice(filesTime, func(i, j int) bool {
		return len(filesTime[i].path) > len(filesTime[j].path)
	})

	for _, ft := range filesTime {
		if err := os.Chtimes(ft.path, time.Now(), ft.modtime); err != nil {
			logger.Warn(fmt.Sprintf("failed to update file timestamps: %v", err))
		}
	}

	return files, nil
}
