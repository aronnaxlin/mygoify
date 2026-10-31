package app

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type entry struct{ name, body string }

func tarGz(t *testing.T, files ...entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range files {
		if err := tw.WriteHeader(&tar.Header{Name: f.name, Mode: 0o644, Size: int64(len(f.body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(f.body))
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func zipOf(t *testing.T, files ...entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		w, err := zw.Create(f.name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(f.body))
	}
	zw.Close()
	return buf.Bytes()
}

// Of an archive only Soloist's executable is taken, and put where it is
// asked: the other files, and names that climb out of the directory, write
// nothing.
func TestInstallTakesOnlyTheExecutable(t *testing.T) {
	defer func(f func(context.Context, string) error) { soloistRuns = f }(soloistRuns)
	name := soloistName()
	files := []entry{{"../evil", "x"}, {"README", "x"}, {"/abs/other", "x"}, {"soloist_release/" + name, "the build"}}
	for kind, data := range map[string][]byte{"archive.tar.gz": tarGz(t, files...), "archive.zip": zipOf(t, files...)} {
		t.Run(kind, func(t *testing.T) {
			parent := t.TempDir()
			dir := filepath.Join(parent, "bin")
			ar := filepath.Join(parent, kind)
			os.WriteFile(ar, data, 0o644)
			soloistRuns = func(context.Context, string) error { return nil }
			bin, err := installSoloist(context.Background(), ar, dir)
			if err != nil {
				t.Fatal(err)
			}
			if b, _ := os.ReadFile(bin); string(b) != "the build" || bin != filepath.Join(dir, name) {
				t.Errorf("installed %q at %s", b, bin)
			}
			if !isExecutable(bin) && name == "soloist" {
				t.Error("not executable")
			}
			left, _ := os.ReadDir(parent)
			inside, _ := os.ReadDir(dir)
			if len(left) != 2 || len(inside) != 1 {
				t.Errorf("wrote more than the executable: %v, %v", left, inside)
			}
		})
	}
}

// An executable that doesn't run doesn't replace the one that did, and an
// archive without one is no answer.
func TestInstallKeepsWhatRuns(t *testing.T) {
	defer func(f func(context.Context, string) error) { soloistRuns = f }(soloistRuns)
	dir := t.TempDir()
	old := filepath.Join(dir, soloistName())
	os.WriteFile(old, []byte("old"), 0o755)
	ar := filepath.Join(t.TempDir(), "a.tar.gz")
	os.WriteFile(ar, tarGz(t, entry{soloistName(), "new"}), 0o644)
	soloistRuns = func(context.Context, string) error { return errors.New("exit 1") }
	if _, err := installSoloist(context.Background(), ar, dir); !errors.Is(err, errNotSoloist) {
		t.Fatalf("err %v", err)
	}
	if b, _ := os.ReadFile(old); string(b) != "old" {
		t.Errorf("the executable is now %q", b)
	}
	os.WriteFile(ar, tarGz(t, entry{"README", "x"}), 0o644)
	if _, err := installSoloist(context.Background(), ar, dir); !errors.Is(err, errNoSoloist) {
		t.Errorf("err %v", err)
	}
}

// Nothing is downloaded until the listener has been told where from and
// said yes; then the build of their computer comes from Spotify's CDN, and
// is kept once it runs.
func TestOnboardingDownloadsSoloist(t *testing.T) {
	file := soloistArchive()
	if file == "" {
		t.Skip("no build for this computer")
	}
	o := newOnboarding(t)
	o.set(stepSoloist)
	data := tarGz(t, entry{soloistName(), "the build"})
	if filepath.Ext(file) == ".zip" {
		data = zipOf(t, entry{soloistName(), "the build"})
	}
	o.archive = data
	o.open()
	o.click("下载 Soloist")
	if len(o.fetched) != 0 {
		t.Fatal("downloaded before the listener said yes")
	}
	o.click("取消")
	o.click("下载 Soloist")
	o.click("下载")
	for deadline := time.Now().Add(2 * time.Second); !o.a.ob.have.bin && time.Now().Before(deadline); {
		o.settle()
	}
	want := filepath.Join(o.home, soloistName())
	if len(o.fetched) != 1 || o.fetched[0] != soloistCDN+file {
		t.Errorf("fetched %v", o.fetched)
	}
	if o.a.settings.Spotify.SoloistBin != want || o.a.ob.step != stepKey {
		t.Errorf("kept %q at step %d, want %q", o.a.settings.Spotify.SoloistBin, o.a.ob.step, want)
	}
	if left, _ := os.ReadDir(o.home); len(left) != 1 {
		t.Errorf("left %v, want the executable alone", left)
	}
}

// A download given up on is told to stop, and what it answers is dropped.
func TestOnboardingDownloadCanBeCancelled(t *testing.T) {
	if soloistArchive() == "" {
		t.Skip("no build for this computer")
	}
	o := newOnboarding(t)
	o.set(stepSoloist)
	o.fetchHeld = true
	o.open()
	o.click("下载 Soloist")
	o.click("下载")
	o.click("取消")
	o.settle()
	if o.a.ob.dl.busy || o.a.ob.have.bin || o.a.ob.binErr != "" {
		t.Errorf("a download cancelled left %+v", o.a.ob)
	}
}

// An archive chosen is unpacked, and kept as Soloist.
func TestOnboardingChoosesArchive(t *testing.T) {
	o := newOnboarding(t)
	o.set(stepSoloist)
	ar := filepath.Join(t.TempDir(), "soloist.tar.gz")
	os.WriteFile(ar, tarGz(t, entry{soloistName(), "the build"}), 0o644)
	o.bin = ar
	o.open()
	o.click("选择已下载的文件…")
	for deadline := time.Now().Add(2 * time.Second); !o.a.ob.have.bin && time.Now().Before(deadline); {
		o.settle()
	}
	if want := filepath.Join(o.home, soloistName()); o.a.settings.Spotify.SoloistBin != want {
		t.Errorf("kept %q, want %q", o.a.settings.Spotify.SoloistBin, want)
	}
}
