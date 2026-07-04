// Package flac decodes FLAC streams to float32 PCM.
//
// Pure-Go implementation on github.com/mewkiz/flac (relux-works fork change):
// the upstream decoder binds libFLAC through cgo, which blocks CGO_ENABLED=0
// builds (reproducible releases) and the Windows port entirely. The public
// surface matches the upstream decoder exactly.
package flac

import (
	"errors"
	"fmt"
	"io"

	"github.com/mewkiz/flac"

	librespot "github.com/devgianlu/go-librespot"
)

// Decoder implements a FLAC decoder.
type Decoder struct {
	log librespot.Logger

	SampleRate int32
	Channels   int32

	gain float32

	stream *flac.Stream

	// buffer holds interleaved float32 samples not yet consumed by Read.
	buffer []float32
	// samplePos is the absolute inter-channel sample position of the NEXT
	// frame to be decoded (honest position, unlike upstream's byte-based one).
	samplePos int64

	closed bool
}

func New(log librespot.Logger, r librespot.SizedReadAtSeeker, gain float32) (*Decoder, error) {
	stream, err := flac.NewSeek(r)
	if err != nil {
		return nil, fmt.Errorf("could not initialize FLAC decoder: %w", err)
	}

	d := &Decoder{
		log:        log,
		gain:       gain,
		stream:     stream,
		SampleRate: int32(stream.Info.SampleRate),
		Channels:   int32(stream.Info.NChannels),
	}
	d.log.Infof("FLAC stream info: sample rate = %d, channels = %d", d.SampleRate, d.Channels)
	return d, nil
}

// decodeNextFrame appends one FLAC frame worth of interleaved, gain-applied
// float32 samples to the buffer. Returns io.EOF at end of stream.
func (d *Decoder) decodeNextFrame() error {
	f, err := d.stream.ParseNext()
	if err != nil {
		return err
	}

	channels := len(f.Subframes)
	if channels == 0 {
		return errors.New("FLAC frame without subframes")
	}
	blockSize := len(f.Subframes[0].Samples)

	// Normalization factor for the stream's bit depth.
	scale := float32(int64(1) << (f.BitsPerSample - 1))

	for i := 0; i < blockSize; i++ {
		for ch := 0; ch < channels; ch++ {
			s := float32(f.Subframes[ch].Samples[i]) / scale
			d.buffer = append(d.buffer, s*d.gain)
		}
	}
	d.samplePos += int64(blockSize)
	return nil
}

func (d *Decoder) Read(p []float32) (n int, err error) {
	for {
		nn := copy(p, d.buffer)
		p = p[nn:]
		n += nn
		d.buffer = d.buffer[nn:]

		if len(p) == 0 {
			return n, nil
		}

		if err := d.decodeNextFrame(); err != nil {
			if errors.Is(err, io.EOF) {
				if len(d.buffer) == 0 {
					return n, io.EOF
				}
				continue
			}
			return 0, fmt.Errorf("error while decoding FLAC frame: %w", err)
		}
	}
}

func (d *Decoder) SetPositionMs(pos int64) error {
	posSamples := uint64(pos * int64(d.SampleRate) / 1000)
	actual, err := d.stream.Seek(posSamples)
	if err != nil {
		return fmt.Errorf("could not seek to position: %w", err)
	}
	d.buffer = d.buffer[:0]
	d.samplePos = int64(actual)
	return nil
}

func (d *Decoder) PositionMs() int64 {
	if d.SampleRate == 0 {
		return 0
	}
	// Position of the next sample Read will return: decoded frames minus
	// what is still sitting in the buffer.
	buffered := int64(len(d.buffer)) / int64(d.Channels)
	return (d.samplePos - buffered) * 1000 / int64(d.SampleRate)
}

func (d *Decoder) Close() error {
	d.closed = true
	return d.stream.Close()
}
