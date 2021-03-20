package sdstore

import (
	"archive/tar"
	"fmt"
	"github.com/klauspost/compress/zstd"
	"go.uber.org/multierr"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"path/filepath"
)

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

	dstFile, err = os.OpenFile(dst, os.O_TRUNC|os.O_CREATE|os.O_RDWR, 0777)
	if err != nil {
		return err
	}
	defer dstFile.Close()
	zw, err = zstd.NewWriter(dstFile, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(CompressionLevel)))
	if err != nil {
		return err
	}
	defer func() { _ = zw.Close() }()

	tw := tar.NewWriter(zw)
	defer func() { _ = tw.Close() }()

	for _, path := range files {
		fInfo, _ := os.Lstat(path.Path)
		if fInfo.Mode().IsDir() {
			err = setHeader(tw, fInfo, path.Path, src)
			if err != nil {
				aggregatedErr = multierr.Append(aggregatedErr, err)
			}
		} else {
			if fInfo.Mode()&os.ModeSymlink != 0 {
				err = setHeader(tw, fInfo, path.Path, src)
				if err != nil {
					aggregatedErr = multierr.Append(aggregatedErr, err)
				}
			} else {
				file, err = os.Open(path.Path)
				if err != nil {
					aggregatedErr = multierr.Append(aggregatedErr, fmt.Errorf("ignoring file %q: %v", path, err))
					continue
				}
				err = setHeader(tw, fInfo, path.Path, src)
				if err != nil {
					file.Close()
					aggregatedErr = multierr.Append(aggregatedErr, err)
					continue
				}
				if _, err = io.Copy(tw, file); err != nil {
					file.Close()
					aggregatedErr = multierr.Append(aggregatedErr, fmt.Errorf("error copying file %q to tar: %v", path, err))
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
				path := filepath.Join(dst, hdr.Name)
				source := hdr.Linkname

				err := os.Symlink(source, path)
				if err != nil {
					aggregatedErr = multierr.Append(aggregatedErr, fmt.Errorf("error creating symlink %q %q: %v", source, path, err))
					break
				}
				mtime[0] = unix.NsecToTimeval(info.ModTime().UnixNano())
				mtime[1] = unix.NsecToTimeval(info.ModTime().UnixNano())
				err = unix.Lutimes(path, mtime[0:])
				if err != nil {
					aggregatedErr = multierr.Append(aggregatedErr, fmt.Errorf("error setting symlink chtime %q: %v", path, err))
					break
				}
			} else {
				path := filepath.Join(dst, hdr.Name)

				file, err = os.Create(path)
				if err != nil {
					aggregatedErr = multierr.Append(aggregatedErr, fmt.Errorf("error creating file %q: %v", path, err))
					break
				}
				written, err = io.Copy(file, tr)
				if err != nil {
					file.Close()
					aggregatedErr = multierr.Append(aggregatedErr, fmt.Errorf("error writing to file %q: %v", path, err))
					break
				}
				if written != hdr.Size {
					file.Close()
					aggregatedErr = multierr.Append(aggregatedErr, fmt.Errorf("wrote %d bytes, expected to write %d", written, hdr.Size))
					break
				}
				file.Close()
				err = os.Chtimes(path, info.ModTime(), info.ModTime())
				if err != nil {
					aggregatedErr = multierr.Append(aggregatedErr, fmt.Errorf("error setting file chtimes %q: %v", path, err))
					break
				}
				err = os.Chmod(path, info.Mode())
				if err != nil {
					aggregatedErr = multierr.Append(aggregatedErr, fmt.Errorf("error setting file mode %q: %v", path, err))
					break
				}
			}
		}
	}
	return aggregatedErr
}
