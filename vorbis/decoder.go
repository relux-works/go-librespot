// Package vorbis decodes ogg/vorbis streams to float32 PCM.
//
// Pure-Go implementation on github.com/jfreymuth/oggvorbis (relux-works fork
// change): upstream binds libvorbis through xlab/vorbis-go (cgo), which
// blocks CGO_ENABLED=0 reproducible builds and the Windows port. The public
// surface matches the upstream decoder.
//
// Seeking note: upstream jumps by byte offsets from the Spotify seek table
// (MetadataPage) and re-syncs on an ogg page boundary. Here SetPositionMs
// delegates to oggvorbis.SetPosition (granule bisection over the stream),
// which is exact; the stream is librespot's on-disk cached file, so the
// extra reads of bisection are cheap. The MetadataPage stays in the
// constructor signature for compatibility (and future fast-path use).
package vorbis

import (
	"fmt"
	"sync"

	"github.com/jfreymuth/oggvorbis"

	librespot "github.com/devgianlu/go-librespot"
)

// Decoder implements an OggVorbis decoder.
type Decoder struct {
	sync.Mutex

	log librespot.Logger

	SampleRate int32
	Channels   int32

	meta *MetadataPage
	gain float32

	reader *oggvorbis.Reader
}

func New(log librespot.Logger, r librespot.SizedReadAtSeeker, meta *MetadataPage, gain float32) (*Decoder, error) {
	reader, err := oggvorbis.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("failed initializing vorbis reader: %w", err)
	}

	d := &Decoder{
		log:        log,
		meta:       meta,
		gain:       gain,
		reader:     reader,
		SampleRate: int32(reader.SampleRate()),
		Channels:   int32(reader.Channels()),
	}
	d.log.Debugf("vorbis stream info: sample rate = %d, channels = %d", d.SampleRate, d.Channels)
	return d, nil
}

func (d *Decoder) Read(p []float32) (n int, err error) {
	d.Lock()
	defer d.Unlock()

	n, err = d.reader.Read(p)
	if d.gain != 1 {
		for i := 0; i < n; i++ {
			p[i] *= d.gain
		}
	}
	return n, err
}

func (d *Decoder) SetPositionMs(pos int64) error {
	d.Lock()
	defer d.Unlock()

	posSamples := pos * int64(d.SampleRate) / 1000
	if err := d.reader.SetPosition(posSamples); err != nil {
		return fmt.Errorf("failed seeking vorbis stream: %w", err)
	}
	d.log.Tracef("seek to %dms (samples: %d)", pos, posSamples)
	return nil
}

func (d *Decoder) PositionMs() int64 {
	d.Lock()
	defer d.Unlock()

	if d.SampleRate == 0 {
		return 0
	}
	return d.reader.Position() * 1000 / int64(d.SampleRate)
}

func (d *Decoder) Close() {
	// The underlying stream is owned and closed by the caller.
}
