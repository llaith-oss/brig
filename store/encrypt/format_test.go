package encrypt

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/disorganizer/brig/util/testutil"
)

var TestKey = []byte("01234567890ABCDE01234567890ABCDE")

func encryptFile(key []byte, from, to string) (int64, error) {
	fdFrom, _ := os.Open(from)
	defer fdFrom.Close()

	fdTo, _ := os.OpenFile(to, os.O_CREATE|os.O_WRONLY, 0755)
	defer fdTo.Close()

	return Encrypt(key, fdFrom, fdTo)
}

func decryptFile(key []byte, from, to string) (int64, error) {
	fdFrom, _ := os.Open(from)
	defer fdFrom.Close()

	fdTo, _ := os.OpenFile(to, os.O_CREATE|os.O_WRONLY, 0755)
	defer fdTo.Close()

	return Decrypt(key, fdFrom, fdTo)
}

func testSimpleEncDec(t *testing.T, size int) {
	path := testutil.CreateFile(int64(size))
	defer os.Remove(path)

	encPath := path + "_enc"
	decPath := path + "_dec"

	_, err := encryptFile(TestKey, path, encPath)
	defer os.Remove(encPath)

	if err != nil {
		t.Errorf("Encrypt failed: %v", err)
	}

	_, err = decryptFile(TestKey, encPath, decPath)
	defer os.Remove(decPath)

	if (err == io.EOF && size != 0) || (err != nil && err != io.EOF) {
		t.Errorf("Decrypt failed: %v", err)
	}

	a, _ := ioutil.ReadFile(path)
	b, _ := ioutil.ReadFile(decPath)
	c, _ := ioutil.ReadFile(encPath)

	if !bytes.Equal(a, b) {
		t.Errorf("Source and decrypted not equal")
	}

	if bytes.Equal(a, c) && size != 0 {
		t.Errorf("Source was not encrypted (same as source)")
		t.Errorf("%v|||%v|||%v", a, b, c)
	}
}

func TestSimpleEncDec(t *testing.T) {
	t.Parallel()

	sizes := []int{
		0,
		1,
		MaxBlockSize - 1,
		MaxBlockSize,
		MaxBlockSize + 1,
		GoodDecBufferSize - 1,
		GoodDecBufferSize,
		GoodDecBufferSize + 1,
		GoodEncBufferSize - 1,
		GoodEncBufferSize,
		GoodEncBufferSize + 1,
	}

	for size := range sizes {
		t.Logf("Testing SimpleEncDec for size %d", size)
		testSimpleEncDec(t, size)
	}
}

func TestSeek(t *testing.T) {
	N := int64(2 * MaxBlockSize)
	a := testutil.CreateDummyBuf(N)
	b := make([]byte, 0, N)

	source := bytes.NewBuffer(a)
	shared := &bytes.Buffer{}
	dest := bytes.NewBuffer(b)

	encLayer, err := NewWriter(shared, TestKey, false)
	if err != nil {
		panic(err)
	}

	buf := make([]byte, GoodEncBufferSize)

	// Encrypt:
	_, err = io.CopyBuffer(encLayer, source, buf)
	if err != nil {
		panic(err)
	}

	// This needs to be here, since close writes
	// left over data to the write stream
	encLayer.Close()

	sharedReader := bytes.NewReader(shared.Bytes())
	decLayer, err := NewReader(sharedReader, TestKey)
	if err != nil {
		panic(err)
	}
	defer decLayer.Close()

	seekTest := int64(MaxBlockSize)
	pos, err := decLayer.Seek(seekTest, os.SEEK_SET)
	if err != nil {
		t.Errorf("Seek error'd: %v", err)
		return
	}

	if pos != seekTest {
		t.Errorf("Seek is a bad jumper: %d (should %d)", pos, MaxBlockSize)
		return
	}

	pos, _ = decLayer.Seek(0, os.SEEK_CUR)
	if pos != seekTest {
		t.Errorf("SEEK_CUR(0) deliverd wrong status")
		return
	}

	pos, _ = decLayer.Seek(seekTest/2, os.SEEK_CUR)
	if pos != seekTest+seekTest/2 {
		t.Errorf("SEEK_CUR jumped to the wrong pos: %d", pos)
	}

	pos, _ = decLayer.Seek(-seekTest, os.SEEK_CUR)
	if pos != seekTest/2 {
		t.Errorf("SEEK_CUR does not like negative indices: %d", pos)
	}

	pos, _ = decLayer.Seek(seekTest/2, os.SEEK_CUR)
	if pos != seekTest {
		t.Errorf("SEEK_CUR has problems after negative indices: %d", pos)
	}

	// Decrypt:
	_, err = io.CopyBuffer(dest, decLayer, buf)
	if err != nil {
		t.Errorf("Decrypt failed: %v", err)
		return
	}

	if !bytes.Equal(a[seekTest:], dest.Bytes()) {
		b := dest.Bytes()
		fmt.Printf("AAA %d %x %x\n", len(a), a[:10], a[len(a)-10:])
		fmt.Printf("BBB %d %x %x\n", len(b), b[:10], b[len(b)-10:])
		t.Errorf("Buffers are not equal")
		return
	}
}

func BenchmarkEncDec(b *testing.B) {
	for n := 0; n < b.N; n++ {
		testSimpleEncDec(nil, MaxBlockSize*100)
	}
}

// Regression test:
// check that reader does not read first block first,
// even if jumping right into the middle of the file.
func TestSeekThenRead(t *testing.T) {
	N := int64(2 * MaxBlockSize)
	a := testutil.CreateDummyBuf(N)
	b := make([]byte, 0, N)

	source := bytes.NewBuffer(a)
	shared := &bytes.Buffer{}
	dest := bytes.NewBuffer(b)

	encLayer, err := NewWriter(shared, TestKey, false)
	if err != nil {
		panic(err)
	}

	// Use a different buf size for a change:
	buf := make([]byte, 4096)

	// Encrypt:
	_, err = io.CopyBuffer(encLayer, source, buf)
	if err != nil {
		panic(err)
	}

	// This needs to be here, since close writes
	// left over data to the write stream
	encLayer.Close()

	sharedReader := bytes.NewReader(shared.Bytes())
	decLayer, err := NewReader(sharedReader, TestKey)
	if err != nil {
		panic(err)
	}
	defer decLayer.Close()

	// Jump somewhere inside the large file:
	jumpPos := N/2 + N/4 + 1
	newPos, err := decLayer.Seek(jumpPos, os.SEEK_SET)
	if err != nil {
		t.Errorf("Seek failed in SeekThenRead: %v", err)
		return
	}

	if newPos != jumpPos {
		t.Errorf("Seek jumped to %v (should be %v)", newPos, N/2+N/4)
		return
	}

	// Decrypt:
	copiedBytes, err := io.CopyBuffer(dest, decLayer, buf)
	if err != nil {
		t.Errorf("Decrypt failed: %v", err)
		return
	}

	if copiedBytes != N-jumpPos {
		t.Errorf("Copied different amount of decrypted data than expected.")
		t.Errorf("Should be %v, was %v bytes.", copiedBytes, N-jumpPos)
		return
	}

	// Check the data actually matches the source data.
	if !bytes.Equal(a[newPos:], dest.Bytes()) {
		t.Errorf("Seeked data does not match expectations.")
		t.Errorf("\tEXPECTED: %v...", a[newPos:newPos:10])
		t.Errorf("\tGOT:      %v...", dest.Bytes()[:10])
	}
}