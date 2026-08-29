package minecraft

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// levelDat builds the part of a real level.dat this package reads: a root
// compound holding Data, which holds the data version and a Version compound with
// the game's name, alongside the other tag types the reader has to skip past to
// reach them. A dataVersion of 0 leaves that tag out entirely.
func levelDat(version string, dataVersion int32) []byte {
	var b bytes.Buffer
	b.WriteByte(tagCompound)
	writeName(&b, "")

	b.WriteByte(tagCompound)
	writeName(&b, "Data")

	b.WriteByte(tagLong) // a payload to skip before the one we want
	writeName(&b, "Time")
	binary.Write(&b, binary.BigEndian, int64(1234))

	if dataVersion != 0 {
		b.WriteByte(tagInt)
		writeName(&b, "DataVersion")
		binary.Write(&b, binary.BigEndian, dataVersion)
	}

	b.WriteByte(tagList) // a list of compounds, the awkward skip
	writeName(&b, "ServerBrands")
	b.WriteByte(tagString)
	binary.Write(&b, binary.BigEndian, int32(1))
	writeName(&b, "fabric")

	b.WriteByte(tagCompound)
	writeName(&b, "Version")
	b.WriteByte(tagInt)
	writeName(&b, "Id")
	binary.Write(&b, binary.BigEndian, int32(4554))
	b.WriteByte(tagString)
	writeName(&b, "Name")
	writeName(&b, version)
	b.WriteByte(tagEnd) // Version

	b.WriteByte(tagEnd) // Data
	b.WriteByte(tagEnd) // root

	var gz bytes.Buffer
	w := gzip.NewWriter(&gz)
	w.Write(b.Bytes())
	w.Close()
	return gz.Bytes()
}

func writeName(b *bytes.Buffer, s string) {
	binary.Write(b, binary.BigEndian, uint16(len(s)))
	b.WriteString(s)
}

func TestWorldVersion(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "world"), 0o755)
	os.WriteFile(filepath.Join(dir, "world", "level.dat"), levelDat("1.21.9", 4554), 0o644)

	got, err := WorldVersion(dir)
	if err != nil {
		t.Fatalf("WorldVersion: %v", err)
	}
	if got != "1.21.9" {
		t.Errorf("WorldVersion = %q, want 1.21.9", got)
	}
}

// A world folder named by level-name is where the version has to be read from;
// assuming world/ would miss it entirely.
func TestWorldVersionCustomLevelName(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "server.properties"), []byte("level-name=myworld\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, "myworld"), 0o755)
	os.WriteFile(filepath.Join(dir, "myworld", "level.dat"), levelDat("26.2", 5120), 0o644)

	got, err := WorldVersion(dir)
	if err != nil {
		t.Fatalf("WorldVersion: %v", err)
	}
	if got != "26.2" {
		t.Errorf("WorldVersion = %q, want 26.2", got)
	}
}

func TestReadLevel(t *testing.T) {
	got, err := ReadLevel(bytes.NewReader(levelDat("1.21.9", 4554)))
	if err != nil {
		t.Fatalf("ReadLevel: %v", err)
	}
	if got.Version != "1.21.9" || got.DataVersion != 4554 {
		t.Errorf("ReadLevel = %+v, want {1.21.9 4554}", got)
	}
}

// A level.dat without a DataVersion still names its version: the number is only
// there for comparisons, which the callers can skip.
func TestReadLevelWithoutDataVersion(t *testing.T) {
	got, err := ReadLevel(bytes.NewReader(levelDat("1.21.9", 0)))
	if err != nil {
		t.Fatalf("ReadLevel: %v", err)
	}
	if got.Version != "1.21.9" || got.DataVersion != 0 {
		t.Errorf("ReadLevel = %+v, want {1.21.9 0}", got)
	}
}

func TestWorldVersionNoWorld(t *testing.T) {
	if _, err := WorldVersion(t.TempDir()); err == nil {
		t.Error("expected an error when the server has no world yet")
	}
}
