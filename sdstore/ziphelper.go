package sdstore

import (
	"archive/tar"
	"archive/zip"
	"fmt"
	"github.com/klauspost/compress/zstd"
	"go.uber.org/multierr"
	"golang.org/x/sys/unix"
	"io"
	"math/rand"
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

func setHeader(tw *tar.Writer, fInfo os.FileInfo, path, src string) error {
	var (
		link     string
		fileName string
	)
	link, _ = os.Readlink(path)
	if src != path {
		fileName = path[1+len(src):]
	} else {
		fileName = path
	}

	header, err := tar.FileInfoHeader(fInfo, filepath.ToSlash(link))
	if err != nil {
		return err
	}
	header.Name = filepath.ToSlash(fileName)
	header.ModTime = fInfo.ModTime()
	err = tw.WriteHeader(header)
	return err
}

func Compress(src, dst string, files []*FileInfo) error {
	var (
		err, aggregatedErr error
		file, dstFile      *os.File
		zw                 *zstd.Encoder
		// b                  int64
	)
	rand.Seed(time.Now().UnixNano())
	dstFile, err = os.OpenFile(dst, os.O_TRUNC|os.O_CREATE|os.O_RDWR, DefaultFilePermission)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	zstd.WithAllLitEntropyCompression(false)
	zw, err = zstd.NewWriter(dstFile, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(CompressionLevel)))
	if err != nil {
		return err
	}
	defer func() { _ = zw.Close() }()

	tw := tar.NewWriter(zw)
	defer func() { _ = tw.Close() }()

	for _, f := range files {
		fInfo, _ := os.Lstat(f.Path)
		if fInfo.Mode().IsDir() {
			err = setHeader(tw, fInfo, f.Path, src)
			if err != nil {
				aggregatedErr = multierr.Append(aggregatedErr, err)
			}
		} else {
			if fInfo.Mode()&os.ModeSymlink != 0 {
				err = setHeader(tw, fInfo, f.Path, src)
				if err != nil {
					aggregatedErr = multierr.Append(aggregatedErr, err)
				}
			} else {
				file, err = os.Open(f.Path)
				if err != nil {
					aggregatedErr = multierr.Append(aggregatedErr, fmt.Errorf("ignoring file %q: %v", f, err))
					continue
				}
				err = setHeader(tw, fInfo, f.Path, src)
				if err != nil {
					file.Close()
					aggregatedErr = multierr.Append(aggregatedErr, err)
					continue
				}
				if _, err = io.Copy(tw, file); err != nil {
					file.Close()
					aggregatedErr = multierr.Append(aggregatedErr, fmt.Errorf("error copying file %q to tar: %v", f, err))
					continue
				}
				// fmt.Printf("wrote %d B of %d B for %q", b, fInfo.Size(), file.Name())
				file.Close()
			}
		}
	}
	return aggregatedErr
}

func Decompress(src, dst string) error {
	var (
		err, aggregatedErr error
		zr                 *zstd.Decoder
		file, srcFile      *os.File
		hdr                *tar.Header
		mtime              [2]unix.Timeval
		written            int64
	)

	srcFile, err = os.OpenFile(src, os.O_RDONLY, DefaultFilePermission)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	zr, err = zstd.NewReader(srcFile)
	if err != nil {
		return err
	}
	defer zr.Close()

	tr := tar.NewReader(zr)

	for {
		hdr, err = tr.Next()
		if err == io.EOF {
			break
		}

		if err != nil {
			aggregatedErr = multierr.Append(aggregatedErr, err)
			break
		}
		info := hdr.FileInfo()
		if info.IsDir() {
			dirPath := filepath.Join(dst, hdr.Name)
			if err = os.MkdirAll(dirPath, hdr.FileInfo().Mode()); err != nil {
				aggregatedErr = multierr.Append(aggregatedErr, fmt.Errorf("error creating dir %q: %v", dirPath, err))
				break
			}
			err = os.Chtimes(dirPath, info.ModTime(), info.ModTime())
			if err != nil {
				aggregatedErr = multierr.Append(aggregatedErr, fmt.Errorf("error setting chtimes for directory %q: %v", dirPath, err))
				break
			}
		} else {
			if hdr.Typeflag == tar.TypeSymlink {
				fPath := filepath.Join(dst, hdr.Name)
				source := hdr.Linkname

				err := os.Symlink(source, fPath)
				if err != nil {
					aggregatedErr = multierr.Append(aggregatedErr, fmt.Errorf("error creating symlink %q %q: %v", source, fPath, err))
					break
				}
				mtime[0] = unix.NsecToTimeval(info.ModTime().UnixNano())
				mtime[1] = unix.NsecToTimeval(info.ModTime().UnixNano())
				err = unix.Lutimes(fPath, mtime[0:])
				if err != nil {
					aggregatedErr = multierr.Append(aggregatedErr, fmt.Errorf("error setting symlink chtime %q: %v", fPath, err))
					break
				}
			} else {
				fPath := filepath.Join(dst, hdr.Name)

				file, err = os.Create(fPath)
				if err != nil {
					aggregatedErr = multierr.Append(aggregatedErr, fmt.Errorf("error creating file %q: %v", fPath, err))
					break
				}
				written, err = io.Copy(file, tr)
				if err != nil {
					file.Close()
					aggregatedErr = multierr.Append(aggregatedErr, fmt.Errorf("error writing to file %q: %v", fPath, err))
					break
				}
				if written != hdr.Size {
					file.Close()
					aggregatedErr = multierr.Append(aggregatedErr, fmt.Errorf("wrote %d bytes, expected to write %d", written, hdr.Size))
					break
				}
				file.Close()
				err = os.Chtimes(fPath, info.ModTime(), info.ModTime())
				if err != nil {
					aggregatedErr = multierr.Append(aggregatedErr, fmt.Errorf("error setting file chtimes %q: %v", fPath, err))
					break
				}
				err = os.Chmod(fPath, info.Mode())
				if err != nil {
					aggregatedErr = multierr.Append(aggregatedErr, fmt.Errorf("error setting file mode %q: %v", fPath, err))
					break
				}
			}
		}
	}
	return aggregatedErr
}
