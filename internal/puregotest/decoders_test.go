// Tests for the relux-works pure-Go decoder ports (flac, vorbis) against
// real files produced by ffmpeg. Run: go test -run PureGo ./...
package puregotest

import (
	"io"
	"os"
	"testing"

	librespot "github.com/devgianlu/go-librespot"
	"github.com/devgianlu/go-librespot/flac"
	"github.com/devgianlu/go-librespot/vorbis"
)

type testLogger struct{}

func (testLogger) Tracef(string, ...interface{})                     {}
func (testLogger) Debugf(string, ...interface{})                     {}
func (testLogger) Infof(string, ...interface{})                      {}
func (testLogger) Warnf(string, ...interface{})                      {}
func (testLogger) Errorf(string, ...interface{})                     {}
func (testLogger) Trace(...interface{})                              {}
func (testLogger) Debug(...interface{})                              {}
func (testLogger) Info(...interface{})                               {}
func (testLogger) Warn(...interface{})                               {}
func (testLogger) Error(...interface{})                              {}
func (l testLogger) WithField(string, interface{}) librespot.Logger { return l }
func (l testLogger) WithError(error) librespot.Logger               { return l }

func open(t *testing.T, path string) librespot.SizedReadAtSeeker {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("fixture %s missing (generate with ffmpeg)", path)
	}
	t.Cleanup(func() { f.Close() })
	st, _ := f.Stat()
	return io.NewSectionReader(f, 0, st.Size())
}

func TestPureGoFLAC(t *testing.T) {
	d, err := flac.New(testLogger{}, open(t, "/tmp/decoder-test/test.flac"), 1.0)
	if err != nil {
		t.Fatal(err)
	}
	if d.SampleRate != 44100 || d.Channels != 2 {
		t.Fatalf("stream info: rate=%d ch=%d", d.SampleRate, d.Channels)
	}
	buf := make([]float32, 4096)
	total := 0
	var peak float32
	for {
		n, err := d.Read(buf)
		total += n
		for _, s := range buf[:n] {
			if s > peak {
				peak = s
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	// 2 seconds * 44100 * 2 channels
	if want := 2 * 44100 * 2; total < want-8820 || total > want+8820 {
		t.Fatalf("samples = %d, want ~%d", total, want)
	}
	if peak < 0.05 {
		t.Fatalf("sine peak %f too low — decode garbage?", peak)
	}
	if err := d.SetPositionMs(1000); err != nil {
		t.Fatal(err)
	}
	if pos := d.PositionMs(); pos < 900 || pos > 1100 {
		t.Fatalf("after seek PositionMs = %d, want ~1000", pos)
	}
}

func TestPureGoVorbis(t *testing.T) {
	d, err := vorbis.New(testLogger{}, open(t, "/tmp/decoder-test/test.ogg"), nil, 1.0)
	if err != nil {
		t.Fatal(err)
	}
	if d.SampleRate != 44100 || d.Channels != 2 {
		t.Fatalf("stream info: rate=%d ch=%d", d.SampleRate, d.Channels)
	}
	buf := make([]float32, 4096)
	total := 0
	var peak float32
	for {
		n, err := d.Read(buf)
		total += n
		for _, s := range buf[:n] {
			if s > peak {
				peak = s
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if want := 2 * 44100 * 2; total < want-8820 || total > want+8820 {
		t.Fatalf("samples = %d, want ~%d", total, want)
	}
	if peak < 0.05 {
		t.Fatalf("sine peak %f too low — decode garbage?", peak)
	}
	if err := d.SetPositionMs(500); err != nil {
		t.Fatal(err)
	}
	if pos := d.PositionMs(); pos < 400 || pos > 600 {
		t.Fatalf("after seek PositionMs = %d, want ~500", pos)
	}
}
