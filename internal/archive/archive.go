// Package archive packs a source directory into a single archive file
// (tar.gz or zip) suitable for upload to a storage backend. It depends only on
// the standard library and internal/models.
package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/flate"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"archivesync/internal/models"
)

// Result describes a successfully created archive file.
type Result struct {
	Path      string // filesystem path of the archive file
	Size      int64  // final file size in bytes
	FileCount int    // number of regular files stored
	Format    string // "tar.gz" or "zip"
}

// Ext returns the file extension (including the leading dot) for the archive
// format selected by opts: ".zip" for zip, otherwise ".tar.gz".
func Ext(opts models.ArchiveOptions) string {
	if opts.Format == "zip" {
		return ".zip"
	}
	return ".tar.gz"
}

// formatName returns the canonical format label for opts.
func formatName(opts models.ArchiveOptions) string {
	if opts.Format == "zip" {
		return "zip"
	}
	return "tar.gz"
}

// Create packs srcDir into a new archive file placed inside destDir and named
// baseName+Ext(opts). It honors the include/exclude globs and compression level
// in opts. Regular files are stored with their mode and modification time
// preserved; directories are recorded as entries; symlinks are skipped. Paths
// inside the archive are stored relative to srcDir using forward slashes.
//
// The archive file itself is skipped if it happens to live inside srcDir.
func Create(ctx context.Context, srcDir, destDir, baseName string, opts models.ArchiveOptions) (*Result, error) {
	info, err := os.Stat(srcDir)
	if err != nil {
		return nil, fmt.Errorf("archive: stat source %q: %w", srcDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("archive: source %q is not a directory", srcDir)
	}
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return nil, fmt.Errorf("archive: create destination dir %q: %w", destDir, err)
	}

	format := formatName(opts)
	destPath := filepath.Join(destDir, baseName+Ext(opts))

	// Resolve the absolute archive path so the walk can skip it if it happens
	// to live under srcDir.
	destAbs, err := filepath.Abs(destPath)
	if err != nil {
		destAbs = destPath
	}

	// 0600: archives may contain sensitive data and live in a shared temp dir.
	out, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("archive: create %q: %w", destPath, err)
	}

	var fileCount int
	if format == "zip" {
		fileCount, err = writeZip(ctx, out, srcDir, destAbs, opts)
	} else {
		fileCount, err = writeTarGz(ctx, out, srcDir, destAbs, opts)
	}
	if err != nil {
		_ = out.Close()
		_ = os.Remove(destPath)
		return nil, err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(destPath)
		return nil, fmt.Errorf("archive: close %q: %w", destPath, err)
	}

	st, err := os.Stat(destPath)
	if err != nil {
		return nil, fmt.Errorf("archive: stat archive %q: %w", destPath, err)
	}

	return &Result{
		Path:      destPath,
		Size:      st.Size(),
		FileCount: fileCount,
		Format:    format,
	}, nil
}

// entry is one item selected by the walk to be written to the archive.
type entry struct {
	rel   string      // slash-separated path relative to srcDir
	full  string      // filesystem path
	info  fs.FileInfo // stat of the entry
	isDir bool
}

// writeTarGz streams a gzip-compressed tar archive to w. Writers are closed in
// the correct order (tar then gzip) and the first error is surfaced.
func writeTarGz(ctx context.Context, w io.Writer, srcDir, destAbs string, opts models.ArchiveOptions) (int, error) {
	gz, err := gzip.NewWriterLevel(w, gzipLevel(opts.Compression))
	if err != nil {
		return 0, fmt.Errorf("archive: init gzip: %w", err)
	}
	tw := tar.NewWriter(gz)

	count, walkErr := walkEntries(ctx, srcDir, destAbs, opts, func(e entry) error {
		return tarAdd(tw, e)
	})

	// Flush and close in order: tar first, then gzip. Keep the earliest error.
	closeErr := tw.Close()
	if cerr := gz.Close(); cerr != nil && closeErr == nil {
		closeErr = cerr
	}

	if walkErr != nil {
		return count, walkErr
	}
	if closeErr != nil {
		return count, fmt.Errorf("archive: finalize tar.gz: %w", closeErr)
	}
	return count, nil
}

// writeZip streams a zip archive to w, using DEFLATE at the configured level.
func writeZip(ctx context.Context, w io.Writer, srcDir, destAbs string, opts models.ArchiveOptions) (int, error) {
	zw := zip.NewWriter(w)
	level := flateLevel(opts.Compression)
	zw.RegisterCompressor(zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(out, level)
	})

	count, walkErr := walkEntries(ctx, srcDir, destAbs, opts, func(e entry) error {
		return zipAdd(zw, e)
	})

	closeErr := zw.Close()

	if walkErr != nil {
		return count, walkErr
	}
	if closeErr != nil {
		return count, fmt.Errorf("archive: finalize zip: %w", closeErr)
	}
	return count, nil
}

// walkEntries walks srcDir and invokes add for every directory and regular file
// that survives the include/exclude filters. It returns the number of regular
// files passed to add. It honors ctx cancellation between entries.
func walkEntries(ctx context.Context, srcDir, destAbs string, opts models.ArchiveOptions, add func(entry) error) (int, error) {
	var count int
	err := filepath.WalkDir(srcDir, func(fullPath string, de fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("archive: walk %q: %w", fullPath, walkErr)
		}
		if e := ctx.Err(); e != nil {
			return e
		}

		// Never include the archive file we are currently writing.
		if abs, aerr := filepath.Abs(fullPath); aerr == nil && abs == destAbs {
			if de.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		rel, rerr := filepath.Rel(srcDir, fullPath)
		if rerr != nil {
			return fmt.Errorf("archive: relativize %q: %w", fullPath, rerr)
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil // the source root itself is not stored as an entry
		}

		// Symlinks are not followed and not recorded.
		if de.Type()&fs.ModeSymlink != 0 {
			return nil
		}

		if de.IsDir() {
			// A directory matching an exclude glob prunes its whole subtree.
			if matchAny(opts.Exclude, rel) {
				return filepath.SkipDir
			}
			info, ierr := de.Info()
			if ierr != nil {
				return fmt.Errorf("archive: stat dir %q: %w", fullPath, ierr)
			}
			return add(entry{rel: rel, full: fullPath, info: info, isDir: true})
		}

		// Only regular files carry content.
		if !de.Type().IsRegular() {
			return nil
		}
		if matchAny(opts.Exclude, rel) {
			return nil
		}
		if len(opts.Include) > 0 && !matchAny(opts.Include, rel) {
			return nil
		}
		info, ierr := de.Info()
		if ierr != nil {
			return fmt.Errorf("archive: stat file %q: %w", fullPath, ierr)
		}
		if aerr := add(entry{rel: rel, full: fullPath, info: info, isDir: false}); aerr != nil {
			return aerr
		}
		count++
		return nil
	})
	return count, err
}

// tarAdd writes a single directory or regular-file entry to the tar stream,
// preserving mode and modification time.
func tarAdd(tw *tar.Writer, e entry) error {
	hdr, err := tar.FileInfoHeader(e.info, "")
	if err != nil {
		return fmt.Errorf("archive: tar header %q: %w", e.rel, err)
	}
	if e.isDir {
		hdr.Name = e.rel + "/"
	} else {
		hdr.Name = e.rel
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("archive: write tar header %q: %w", e.rel, err)
	}
	if e.isDir {
		return nil
	}
	f, err := os.Open(e.full)
	if err != nil {
		return fmt.Errorf("archive: open %q: %w", e.full, err)
	}
	defer f.Close()
	if _, err := io.Copy(tw, f); err != nil {
		return fmt.Errorf("archive: copy %q: %w", e.rel, err)
	}
	return nil
}

// zipAdd writes a single directory or regular-file entry to the zip stream,
// using DEFLATE for files and preserving the modification time.
func zipAdd(zw *zip.Writer, e entry) error {
	hdr, err := zip.FileInfoHeader(e.info)
	if err != nil {
		return fmt.Errorf("archive: zip header %q: %w", e.rel, err)
	}
	hdr.Modified = e.info.ModTime()
	if e.isDir {
		hdr.Name = e.rel + "/"
		hdr.Method = zip.Store
	} else {
		hdr.Name = e.rel
		hdr.Method = zip.Deflate
	}
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		return fmt.Errorf("archive: create zip entry %q: %w", e.rel, err)
	}
	if e.isDir {
		return nil
	}
	f, err := os.Open(e.full)
	if err != nil {
		return fmt.Errorf("archive: open %q: %w", e.full, err)
	}
	defer f.Close()
	if _, err := io.Copy(w, f); err != nil {
		return fmt.Errorf("archive: copy %q: %w", e.rel, err)
	}
	return nil
}

// matchAny reports whether rel matches any of the glob patterns, testing both
// the full slash path and each individual path segment.
func matchAny(patterns []string, rel string) bool {
	if len(patterns) == 0 {
		return false
	}
	segments := strings.Split(rel, "/")
	for _, p := range patterns {
		if p == "" {
			continue
		}
		if ok, _ := path.Match(p, rel); ok {
			return true
		}
		for _, seg := range segments {
			if ok, _ := path.Match(p, seg); ok {
				return true
			}
		}
	}
	return false
}

// gzipLevel clamps a caller-supplied compression level into gzip's range,
// defaulting to gzip.DefaultCompression when unset (<= 0).
func gzipLevel(c int) int {
	if c <= 0 {
		return gzip.DefaultCompression
	}
	if c > gzip.BestCompression {
		return gzip.BestCompression
	}
	return c
}

// flateLevel clamps a caller-supplied compression level into flate's range,
// defaulting to flate.DefaultCompression when unset (<= 0).
func flateLevel(c int) int {
	if c <= 0 {
		return flate.DefaultCompression
	}
	if c > flate.BestCompression {
		return flate.BestCompression
	}
	return c
}
