package minecraft

import (
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// WorldVersion reports the Minecraft version that last saved this instance's
// world, read from level.dat. It's the one dependable answer to "what version is
// this server on": level.dat is a vanilla file, so every loader has it, and the
// server rewrites it on every save, so it names the version actually running
// rather than accumulating the old ones the way versions/ and the loader's own
// folders do.
//
// A server that has never generated a world has no level.dat, so the caller gets
// an error and has to ask instead.
func WorldVersion(dir string) (string, error) {
	path := filepath.Join(dir, LevelName(dir), "level.dat")
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	defer gz.Close()

	data, err := io.ReadAll(gz)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	name, err := nbtString(data, "Data", "Version", "Name")
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return name, nil
}

// level.dat is NBT: a tree of tagged values, each written as a type byte, a name,
// and a payload whose length the type decides. We want exactly one string out of
// it, so the reader below walks named compounds along a path and skips every
// payload it isn't looking for, rather than decoding the whole file.
const (
	tagEnd = iota
	tagByte
	tagShort
	tagInt
	tagLong
	tagFloat
	tagDouble
	tagByteArray
	tagString
	tagList
	tagCompound
	tagIntArray
	tagLongArray
)

type nbtReader struct {
	buf []byte
	pos int
}

// nbtString returns the string at path, e.g. Data > Version > Name.
func nbtString(data []byte, path ...string) (string, error) {
	r := &nbtReader{buf: data}
	// The file is one compound holding everything; step over its own header to
	// stand inside it.
	tag, err := r.tag()
	if err != nil {
		return "", err
	}
	if tag != tagCompound {
		return "", fmt.Errorf("not NBT: root tag is %d, not a compound", tag)
	}
	if _, err := r.text(); err != nil {
		return "", err
	}
	return r.find(path)
}

// find walks the compound the reader is inside, following path one name at a
// time.
func (r *nbtReader) find(path []string) (string, error) {
	for {
		tag, err := r.tag()
		if err != nil {
			return "", err
		}
		if tag == tagEnd {
			return "", fmt.Errorf("no %s in level.dat", path[0])
		}
		name, err := r.text()
		if err != nil {
			return "", err
		}

		if name != path[0] {
			if err := r.skip(tag); err != nil {
				return "", err
			}
			continue
		}
		if len(path) > 1 {
			if tag != tagCompound {
				return "", fmt.Errorf("%s is not a compound", name)
			}
			return r.find(path[1:])
		}
		if tag != tagString {
			return "", fmt.Errorf("%s is not a string", name)
		}
		return r.text()
	}
}

// skip advances past one payload of the given type, descending into lists and
// compounds far enough to find where they end.
func (r *nbtReader) skip(tag byte) error {
	switch tag {
	case tagByte:
		return r.advance(1)
	case tagShort:
		return r.advance(2)
	case tagInt, tagFloat:
		return r.advance(4)
	case tagLong, tagDouble:
		return r.advance(8)
	case tagString:
		n, err := r.uint16()
		if err != nil {
			return err
		}
		return r.advance(int(n))
	case tagByteArray, tagIntArray, tagLongArray:
		n, err := r.int32()
		if err != nil {
			return err
		}
		return r.advance(n * elementSize(tag))
	case tagList:
		return r.skipList()
	case tagCompound:
		return r.skipCompound()
	default:
		return fmt.Errorf("unknown NBT tag %d", tag)
	}
}

func (r *nbtReader) skipList() error {
	elem, err := r.tag()
	if err != nil {
		return err
	}
	n, err := r.int32()
	if err != nil {
		return err
	}
	for i := 0; i < n; i++ {
		if err := r.skip(elem); err != nil {
			return err
		}
	}
	return nil
}

func (r *nbtReader) skipCompound() error {
	for {
		tag, err := r.tag()
		if err != nil {
			return err
		}
		if tag == tagEnd {
			return nil
		}
		if _, err := r.text(); err != nil {
			return err
		}
		if err := r.skip(tag); err != nil {
			return err
		}
	}
}

func elementSize(tag byte) int {
	switch tag {
	case tagIntArray:
		return 4
	case tagLongArray:
		return 8
	default:
		return 1
	}
}

func (r *nbtReader) tag() (byte, error) {
	if r.pos >= len(r.buf) {
		return 0, io.ErrUnexpectedEOF
	}
	tag := r.buf[r.pos]
	r.pos++
	return tag, nil
}

// text reads a length-prefixed string, the form both names and string values use.
func (r *nbtReader) text() (string, error) {
	n, err := r.uint16()
	if err != nil {
		return "", err
	}
	if r.pos+int(n) > len(r.buf) {
		return "", io.ErrUnexpectedEOF
	}
	s := string(r.buf[r.pos : r.pos+int(n)])
	r.pos += int(n)
	return s, nil
}

func (r *nbtReader) uint16() (uint16, error) {
	if r.pos+2 > len(r.buf) {
		return 0, io.ErrUnexpectedEOF
	}
	v := binary.BigEndian.Uint16(r.buf[r.pos:])
	r.pos += 2
	return v, nil
}

func (r *nbtReader) int32() (int, error) {
	if r.pos+4 > len(r.buf) {
		return 0, io.ErrUnexpectedEOF
	}
	v := int(int32(binary.BigEndian.Uint32(r.buf[r.pos:])))
	r.pos += 4
	if v < 0 {
		return 0, fmt.Errorf("negative NBT length %d", v)
	}
	return v, nil
}

func (r *nbtReader) advance(n int) error {
	if n < 0 || r.pos+n > len(r.buf) {
		return io.ErrUnexpectedEOF
	}
	r.pos += n
	return nil
}
