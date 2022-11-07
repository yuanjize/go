// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file implements source, a buffered rune reader
// specialized for scanning Go code: Reading
// ASCII characters, maintaining current (line, col)
// position information, and recording of the most
// recently read source segment are highly optimized.
// This file is self-contained (go tool compile source.go
// compiles) and thus could be made into its own package.
// 就是个文件字符扫描器，返回字符，维护当前位置信息，记录当前正在读的segment

package syntax

import (
	"io"
	"unicode/utf8"
)

// The source buffer is accessed using three indices b (begin),
// r (read), and e (end):
//
//   - If b >= 0, it points to the beginning of a segment of most
//     recently read characters (typically a Go literal).
//
//   - r points to the byte immediately following the most recently
//     read character ch, which starts at r-chw.
//
//   - e points to the byte immediately following the last byte that
//     was read into the buffer.
//
// The buffer content is terminated at buf[e] with the sentinel
// character utf8.RuneSelf. This makes it possible to test for
// the common case of ASCII characters with a single 'if' (see
// nextch method).
//
//	+------ content in use -------+
//	v                             v
//
// buf [...read...|...segment...|ch|...unread...|s|...free...]
//
//	^             ^  ^            ^
//	|             |  |            |
//	b         r-chw  r            e
//
// Invariant: -1 <= b < r <= e < len(buf) && buf[e] == sentinel
// 就是一个人文件内存buf,让scanner读字符的时候读的快点，每次读出来一个字符(不是字节)
type source struct {
	in   io.Reader //文件reader
	errh func(line, col uint, msg string)

	buf       []byte // source buffer  // 文件buf
	ioerr     error  // pending I/O error, or nil
	b, r, e   int    // buffer indices (see comment above) // buf的三个索引，b是段的起始位置，r是下一次要返回的字符，e是buf末尾
	line, col uint   // source position of ch (0-based) // 当前位置
	ch        rune   // most recently read character // 一个字符
	chw       int    // width of ch // 字符宽度，就是一个字符占几个字节
}

const sentinel = utf8.RuneSelf // 小于sentinel的是一个单字节字符

func (s *source) init(in io.Reader, errh func(line, col uint, msg string)) {
	s.in = in
	s.errh = errh

	if s.buf == nil {
		s.buf = make([]byte, nextSize(0))
	}
	s.buf[0] = sentinel
	s.ioerr = nil
	s.b, s.r, s.e = -1, 0, 0
	s.line, s.col = 0, 0
	s.ch = ' '
	s.chw = 0
}

// starting points for line and column numbers
const linebase = 1
const colbase = 1

// pos returns the (line, col) source position of s.ch.
func (s *source) pos() (line, col uint) {
	return linebase + s.line, colbase + s.col
}

// error reports the error msg at source position s.pos().
func (s *source) error(msg string) {
	line, col := s.pos()
	s.errh(line, col, msg)
}

// start starts a new active source segment (including s.ch).
// As long as stop has not been called, the active segment's
// bytes (excluding s.ch) may be retrieved by calling segment.
func (s *source) start()          { s.b = s.r - s.chw }             // 标记当前点为起始位置，更新b的值为上一个r
func (s *source) stop()           { s.b = -1 }                      // 结束骚婊
func (s *source) segment() []byte { return s.buf[s.b : s.r-s.chw] } //返回当前这个segment的buf，b到r之间三一个segment

// rewind rewinds the scanner's read position and character s.ch
// to the start of the currently active segment, which must not
// contain any newlines (otherwise position information will be
// incorrect). Currently, rewind is only needed for handling the
// source sequence ".."; it must not be called outside an active
// segment.
// 重新跑到segment的起始位置重新扫描
func (s *source) rewind() {
	// ok to verify precondition - rewind is rarely called
	if s.b < 0 {
		panic("no active segment")
	}
	s.col -= uint(s.r - s.b)
	s.r = s.b
	s.nextch()
}

// 就是把下一个字符读出来，同时更新了source的一些属性，比如当前字符长度，当前字符，行列等
func (s *source) nextch() {
redo:
	s.col += uint(s.chw)
	if s.ch == '\n' {
		s.line++
		s.col = 0
	}

	// fast common case: at least one ASCII character
	if s.ch = rune(s.buf[s.r]); s.ch < sentinel { // 一般来说上来至少一个单字节字符
		s.r++
		s.chw = 1
		if s.ch == 0 {
			s.error("invalid NUL character")
			goto redo
		}
		return
	}

	// slower general case: add more bytes to buffer if we don't have a full rune
	for s.e-s.r < utf8.UTFMax && !utf8.FullRune(s.buf[s.r:s.e]) && s.ioerr == nil {
		s.fill()
	}

	// EOF
	if s.r == s.e {
		if s.ioerr != io.EOF {
			// ensure we never start with a '/' (e.g., rooted path) in the error message
			s.error("I/O error: " + s.ioerr.Error())
			s.ioerr = nil
		}
		s.ch = -1
		s.chw = 0
		return
	}

	s.ch, s.chw = utf8.DecodeRune(s.buf[s.r:s.e])
	s.r += s.chw

	if s.ch == utf8.RuneError && s.chw == 1 {
		s.error("invalid UTF-8 encoding")
		goto redo
	}

	// BOM's are only allowed as the first character in a file
	const BOM = 0xfeff
	if s.ch == BOM {
		if s.line > 0 || s.col > 0 {
			s.error("invalid BOM in the middle of the file")
		}
		goto redo
	}
}

// 读取更多的文件内容到buf中
// fill reads more source bytes into s.buf.
// It returns with at least one more byte in the buffer, or with s.ioerr != nil.
func (s *source) fill() {
	// determine content to preserve
	b := s.r
	if s.b >= 0 {
		b = s.b
		s.b = 0 // after buffer has grown or content has been moved down
	}
	content := s.buf[b:s.e]

	// grow buffer or move content down
	if len(content)*2 > len(s.buf) {
		s.buf = make([]byte, nextSize(len(s.buf)))
		copy(s.buf, content)
	} else if b > 0 {
		copy(s.buf, content)
	}
	s.r -= b
	s.e -= b

	// read more data: try a limited number of times
	for i := 0; i < 10; i++ {
		var n int
		n, s.ioerr = s.in.Read(s.buf[s.e : len(s.buf)-1]) // -1 to leave space for sentinel
		if n < 0 {
			panic("negative read") // incorrect underlying io.Reader implementation
		}
		if n > 0 || s.ioerr != nil {
			s.e += n
			s.buf[s.e] = sentinel
			return
		}
		// n == 0
	}

	s.buf[s.e] = sentinel
	s.ioerr = io.ErrNoProgress
}

// 返回2的倍数大小
// nextSize returns the next bigger size for a buffer of a given size.
func nextSize(size int) int {
	const min = 4 << 10 // 4K: minimum buffer size
	const max = 1 << 20 // 1M: maximum buffer size which is still doubled
	if size < min {
		return min
	}
	if size <= max {
		return size << 1
	}
	return size + max
}
