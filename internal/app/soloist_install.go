package app

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Soloist's builds are Spotify's to hand out: the listener's own browser,
// or the app on their behalf, takes them from Spotify's CDN, which the
// terms allow, and the app neither carries nor passes them on (DESIGN.md
// §4.2, §10).
const (
	soloistCDN = "https://soloist-builds.spotifycdn.com/"
	// soloistDownloadsURL is the page Spotify asks to be linked to.
	soloistDownloadsURL = "https://developer.spotify.com/documentation/soloist/reference/downloads-and-updates"
	// maxSoloist is the most an executable unpacked may be: Soloist's are
	// tens of megabytes.
	maxSoloist = 400 << 20
)

// These reach outside the app; tests replace them.
var (
	// soloistFetch opens the download of url, and says how long it is, if
	// it knows (-1 if not).
	soloistFetch = func(ctx context.Context, url string) (io.ReadCloser, int64, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, 0, err
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, 0, err
		}
		if res.StatusCode != http.StatusOK {
			res.Body.Close()
			return nil, 0, fmt.Errorf("soloist: %s answered %s", url, res.Status)
		}
		return res.Body, res.ContentLength, nil
	}
	// soloistRuns says bin runs, as `bin --version` does.
	soloistRuns = func(ctx context.Context, bin string) error {
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		return exec.CommandContext(ctx, bin, "--version").Run()
	}
	// soloistHome is where the app keeps the Soloist it unpacked.
	soloistHome = func() string { return filepath.Join(dataDir(), "soloist-bin") }
)

// soloistArchive is the file of Soloist's builds for the computer this
// is, or "" for one it has none for.
func soloistArchive() string {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "darwin/arm64":
		return "soloist_release_osx_arm64.tar.gz"
	case "linux/amd64":
		return "soloist_release_x86_64.tar.gz"
	case "linux/arm64":
		return "soloist_release_arm64.tar.gz"
	case "linux/arm":
		return "soloist_release_arm32.tar.gz"
	case "windows/amd64":
		return "soloist_release_windows_x86_64.zip"
	}
	return ""
}

// soloistName is what Soloist's executable is called.
func soloistName() string {
	if runtime.GOOS == "windows" {
		return "soloist.exe"
	}
	return "soloist"
}

func isArchive(file string) bool {
	f := strings.ToLower(file)
	return strings.HasSuffix(f, ".tar.gz") || strings.HasSuffix(f, ".tgz") || strings.HasSuffix(f, ".zip")
}

// installSoloist unpacks Soloist's executable from the archive into dir,
// and returns its path once it runs. Of the archive it takes that one
// file, wherever it lies and whatever the others are called, so a name
// with ../ in it writes nowhere; the executable that was there before is
// replaced only by one that runs.
func installSoloist(ctx context.Context, archive, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	name := soloistName()
	next := filepath.Join(dir, name+".new")
	defer os.Remove(next)
	var err error
	if strings.HasSuffix(strings.ToLower(archive), ".zip") {
		err = unzipOne(archive, name, next)
	} else {
		err = untarOne(archive, name, next)
	}
	if err != nil {
		return "", err
	}
	if err := os.Chmod(next, 0o755); err != nil {
		return "", err
	}
	if err := soloistRuns(ctx, next); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", errNotSoloist
	}
	bin := filepath.Join(dir, name)
	if err := os.Rename(next, bin); err != nil {
		return "", err
	}
	return bin, nil
}

var (
	errNotSoloist = errors.New("soloist: what was unpacked does not run")
	errNoSoloist  = errors.New("soloist: the archive has no executable of Soloist's")
)

// untarOne copies the file called name from a .tar.gz to dest.
func untarOne(archive, name, dest string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return errNoSoloist
		}
		if err != nil {
			return err
		}
		if h.Typeflag == tar.TypeReg && path.Base(h.Name) == name {
			return writeLimited(dest, tr)
		}
	}
}

// unzipOne copies the file called name from a .zip to dest.
func unzipOne(archive, name, dest string) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.FileInfo().Mode().IsRegular() && path.Base(f.Name) == name {
			r, err := f.Open()
			if err != nil {
				return err
			}
			defer r.Close()
			return writeLimited(dest, r)
		}
	}
	return errNoSoloist
}

// writeLimited writes what r holds to dest, unless it is more than an
// executable of Soloist's could be.
func writeLimited(dest string, r io.Reader) error {
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700)
	if err != nil {
		return err
	}
	n, err := io.Copy(out, io.LimitReader(r, maxSoloist+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > maxSoloist {
		err = errors.New("soloist: the executable is too large")
	}
	return err
}

// progressWriter counts what is written, and tells fn of it, at most
// every 100 ms and once more at the end.
type progressWriter struct {
	w     io.Writer
	n     int64
	last  time.Time
	every func(n int64)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	p.n += int64(n)
	if now := time.Now(); now.Sub(p.last) >= 100*time.Millisecond {
		p.last = now
		p.every(p.n)
	}
	return n, err
}

// downloadSoloist downloads the build of this computer into dir, and
// installs it, telling progress how much has come.
func downloadSoloist(ctx context.Context, dir string, progress func(done, total int64)) (string, error) {
	file := soloistArchive()
	if file == "" {
		return "", errors.New("soloist: no build for this computer")
	}
	body, total, err := soloistFetch(ctx, soloistCDN+file)
	if err != nil {
		return "", err
	}
	defer body.Close()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, "download-*"+filepath.Ext(file))
	if err != nil {
		return "", err
	}
	// The archive is not kept: only what runs.
	defer os.Remove(tmp.Name())
	pw := &progressWriter{w: tmp, every: func(n int64) { progress(n, total) }}
	_, err = io.Copy(pw, io.LimitReader(body, maxSoloist+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", err
	}
	progress(pw.n, total)
	return installSoloist(ctx, tmp.Name(), dir)
}
