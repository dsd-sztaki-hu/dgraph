/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package chunk

import (
	"bufio"
	"compress/gzip"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/dgraph-io/dgraph/x"
)

// chunk.Reader wraps a bufio.Reader to hold additional information
// about the file being read.
type Reader struct {
	rd         *bufio.Reader
	offset     int // start of file is at offset 0
	line       int // first line is number 0
	compressed bool
	filename   string

	// these are used to handle UnreadRune
	prevOffset int
	prevLine   int
}

// NewReader returns an open reader and cleanup function for the given file. Gzip-compressed input
// is detected and decompressed automatically even without the gz extension. The caller is
// responsible for calling the returned cleanup function when done with the reader.
func NewReader(file string) (*Reader, func()) {
	var f *os.File
	var err error
	if file == "-" {
		f, file = os.Stdin, "/dev/stdin"
	} else {
		f, err = os.Open(file)
	}
	x.Check(err)

	return newReader(f)
}

func newReader(f *os.File) (*Reader, func()) {
	var rd = Reader{filename: f.Name()}
	var cleanup = func() { f.Close() }

	var gzf io.Reader
	if filepath.Ext(rd.filename) == ".gz" {
		gzf = f
	} else {
		rd.rd = bufio.NewReader(f)
		buf, _ := rd.rd.Peek(512)
		typ := http.DetectContentType(buf)
		if typ == "application/x-gzip" {
			gzf = rd.rd
		}
	}

	if gzf != nil {
		gzr, err := gzip.NewReader(gzf)
		x.CheckfNoTrace(err)
		rd.rd = bufio.NewReader(gzr)
		rd.compressed = true
		cleanup = func() { f.Close(); gzr.Close() }
	}

	return &rd, cleanup
}

// BytePos returns the current position of the reader in the file or stream. Or alternatively,
// returns the number of bytes that have been read.
func (r *Reader) Offset() int {
	return r.offset
}

// LinePos returns the number of newlines that have been read.
func (r *Reader) LineCount() int {
	return r.line
}

func (r *Reader) ReadSlice(delim byte) ([]byte, error) {
	r.prevOffset, r.prevLine = r.offset, r.line

	slc, err := r.rd.ReadSlice(delim)
	r.offset += len(slc)
	for _, b := range slc {
		if b == '\n' {
			r.line++
		}
	}

	return slc, err
}

func (r *Reader) ReadString(delim byte) (string, error) {
	r.prevOffset, r.prevLine = r.offset, r.line

	str, err := r.rd.ReadString(delim)
	r.offset += len(str)
	r.line += strings.Count(str, "\n")

	return str, err
}

func (r *Reader) ReadRune() (rune, int, error) {
	r.prevOffset, r.prevLine = r.offset, r.line

	char, size, err := r.rd.ReadRune()
	r.offset += size
	if char == '\n' {
		r.line++
	}
	return char, size, err
}

func (r *Reader) UnreadRune() error {
	r.offset, r.line = r.prevOffset, r.prevLine
	r.prevOffset, r.prevLine = 0, 0
	return r.rd.UnreadRune()
}
