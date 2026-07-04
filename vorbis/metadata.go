package vorbis

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	librespot "github.com/devgianlu/go-librespot"
)

const (
	segmentTypeSeekTable  = 0
	segmentTypeReplayGain = 1
	segmentTypeFFFFFFFF   = 2
)

var seekTableLookup = []uint16{
	0, 112, 197, 327, 374, 394, 407, 417, 425, 433, 439, 444, 449, 454, 458, 462, 466, 470, 473, 477, 480, 483, 486,
	489, 491, 494, 497, 499, 502, 504, 506, 509, 511, 513, 515, 517, 519, 521, 523, 525, 527, 529, 531, 533, 535, 537,
	538, 540, 542, 544, 545, 547, 549, 550, 552, 554, 555, 557, 558, 560, 562, 563, 565, 566, 568, 569, 571, 572, 574,
	575, 577, 578, 580, 581, 583, 584, 585, 587, 588, 590, 591, 593, 594, 595, 597, 598, 599, 601, 602, 604, 605, 606,
	608, 609, 610, 612, 613, 615, 616, 617, 619, 620, 621, 623, 624, 625, 627, 628, 629, 631, 632, 634, 635, 636, 638,
	639, 640, 642, 643, 644, 646, 647, 649, 650, 651, 653, 654, 655, 657, 658, 660, 661, 662, 664, 665, 667, 668, 669,
	671, 672, 674, 675, 677, 678, 679, 681, 682, 684, 685, 687, 688, 690, 691, 693, 694, 696, 697, 699, 700, 702, 704,
	705, 707, 708, 710, 712, 713, 715, 716, 718, 720, 721, 723, 725, 727, 728, 730, 732, 734, 735, 737, 739, 741, 743,
	745, 747, 748, 750, 752, 754, 756, 758, 760, 763, 765, 767, 769, 771, 773, 776, 778, 780, 782, 785, 787, 790, 792,
	795, 797, 800, 803, 805, 808, 811, 814, 817, 820, 823, 826, 829, 833, 836, 840, 843, 847, 851, 855, 859, 863, 868,
	872, 877, 882, 887, 893, 898, 904, 911, 918, 925, 933, 941, 951, 961, 972, 985, 1000, 1017, 1039, 1067, 1108, 1183,
	1520, 2658, 4666, 8191,
}

type MetadataPage struct {
	log librespot.Logger

	seekSamples  uint32
	seekSize     uint32
	offset       int32
	seekTable    []int32
	hasSeekTable bool

	trackGainDb   float32
	trackPeak     float32
	albumGainDb   float32
	albumPeak     float32
	hasReplayGain bool
}

// readFirstOggPage parses one Ogg page header by hand (pure Go, relux-works
// fork change — this was the last cgo dependency). Returns the first packet's
// body and the total page length in bytes.
func readFirstOggPage(rr io.Reader) (packet []byte, pageLen int64, err error) {
	head := make([]byte, 27)
	if _, err := io.ReadFull(rr, head); err != nil {
		return nil, 0, fmt.Errorf("failed reading vorbis stream head: %w", err)
	}
	if !bytes.Equal(head[0:4], []byte("OggS")) || head[4] != 0 {
		return nil, 0, errors.New("vorbis: not a valid Ogg bitstream")
	}
	nSegs := int(head[26])
	lacing := make([]byte, nSegs)
	if _, err := io.ReadFull(rr, lacing); err != nil {
		return nil, 0, fmt.Errorf("failed reading ogg lacing table: %w", err)
	}
	bodyLen := 0
	firstPacketLen := 0
	packetDone := false
	for _, l := range lacing {
		bodyLen += int(l)
		if !packetDone {
			firstPacketLen += int(l)
			if l < 255 {
				packetDone = true
			}
		}
	}
	body := make([]byte, bodyLen)
	if _, err := io.ReadFull(rr, body); err != nil {
		return nil, 0, fmt.Errorf("failed reading ogg page body: %w", err)
	}
	return body[:firstPacketLen], int64(27 + nSegs + bodyLen), nil
}

func ExtractMetadataPage(log librespot.Logger, r io.ReaderAt, limit int64) (librespot.SizedReadAtSeeker, *MetadataPage, error) {
	rr := io.NewSectionReader(r, 0, limit)

	firstPacket, pageLen, err := readFirstOggPage(rr)
	if err != nil {
		return nil, nil, err
	}

	// we have the ogg packet, check it is the metadata page
	body := bytes.NewReader(firstPacket)
	if b, _ := body.ReadByte(); b != 0x81 {
		return nil, nil, fmt.Errorf("invalid metadata page")
	}

	var metadata MetadataPage
	metadata.log = log

	for {
		var segmentLen uint16
		if err := binary.Read(body, binary.LittleEndian, &segmentLen); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, nil, fmt.Errorf("failed reading segment length: %w", err)
		}

		segmentData := make([]byte, segmentLen)
		if _, err := io.ReadFull(body, segmentData); err != nil {
			return nil, nil, fmt.Errorf("failed reading segment data: %w", err)
		}

		segmentType := segmentData[0]
		seg := bytes.NewReader(segmentData[1:])

		switch segmentType {
		case segmentTypeSeekTable:
			if err := binary.Read(seg, binary.LittleEndian, &metadata.seekSamples); err != nil {
				return nil, nil, fmt.Errorf("failed reading seek table samples size: %w", err)
			} else if err := binary.Read(seg, binary.LittleEndian, &metadata.seekSize); err != nil {
				return nil, nil, fmt.Errorf("failed reading seek table bytes size: %w", err)
			}

			var offset uint8
			if err := binary.Read(seg, binary.LittleEndian, &offset); err != nil {
				return nil, nil, fmt.Errorf("failed reading seek table offset: %w", err)
			}

			metadata.offset = -int32(seekTableLookup[offset])
			metadata.seekTable = make([]int32, 100)

			cum := metadata.offset
			for i := 0; i < 100; i++ {
				var idx uint8
				if err := binary.Read(seg, binary.LittleEndian, &idx); err != nil {
					return nil, nil, fmt.Errorf("failed reading seek table index: %w", err)
				}

				cum += int32(seekTableLookup[idx])
				metadata.seekTable[i] = cum
			}

			metadata.hasSeekTable = true
		case segmentTypeReplayGain:
			if err := binary.Read(seg, binary.LittleEndian, &metadata.trackGainDb); err != nil {
				return nil, nil, fmt.Errorf("failed reading track gain metadata: %w", err)
			} else if err := binary.Read(seg, binary.LittleEndian, &metadata.trackPeak); err != nil {
				return nil, nil, fmt.Errorf("failed reading track peek metadata: %w", err)
			} else if err := binary.Read(seg, binary.LittleEndian, &metadata.albumGainDb); err != nil {
				return nil, nil, fmt.Errorf("failed reading album gain metadata: %w", err)
			} else if err := binary.Read(seg, binary.LittleEndian, &metadata.albumPeak); err != nil {
				return nil, nil, fmt.Errorf("failed reading album peek metadata: %w", err)
			} else if _, err := seg.ReadByte(); !errors.Is(err, io.EOF) {
				return nil, nil, fmt.Errorf("replay gain metadata underrun")
			}

			metadata.hasReplayGain = true
		case segmentTypeFFFFFFFF:
			var val int32
			if err := binary.Read(seg, binary.LittleEndian, &val); err != nil {
				return nil, nil, fmt.Errorf("failed reading FFFFFFFF value: %w", err)
			} else if val != -1 {
				log.Warnf("unexpected FFFFFFFF value: %d", val)
			}
		default:
			log.Warnf("unknown metadata page segment: %x (len: %d)", segmentType, segmentLen)
		}
	}

	// validate that what we need has been read
	if !metadata.hasSeekTable {
		return nil, nil, fmt.Errorf("no seek table metadata found")
	} else if !metadata.hasReplayGain {
		return nil, nil, fmt.Errorf("no replay gain metadata found")
	}

	// return a new stream without the metadata page
	return io.NewSectionReader(r, pageLen, limit-pageLen), &metadata, nil
}

func (m MetadataPage) GetSeekPosition(samplesPos int64) int64 {
	// figure out a relative position based on samples and clamp it to [0,100)
	samplesRelPos := float32(samplesPos) * 100 / float32(m.seekSamples)
	samplesIntPos := int32(samplesRelPos)
	if samplesIntPos <= 0 {
		samplesIntPos = 1
	} else if samplesIntPos > 99 {
		samplesIntPos = 99
	}

	// interpolate our requested samples position
	tablePrev, tableCurr := m.seekTable[samplesIntPos-1], m.seekTable[samplesIntPos]
	interpolatedPos := float32(tableCurr-tablePrev)*(samplesRelPos-float32(samplesIntPos)) + float32(tablePrev)

	// transform the interpolated position to a bytes position
	bytesPos := interpolatedPos * 1.525879e-05 * float32(m.seekSize)
	if bytesPos < 0 {
		return 0
	}

	return int64(bytesPos)
}
