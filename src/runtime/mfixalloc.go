// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Fixed-size object allocator. Returned memory is not zeroed.
//
// See malloc.go for overview.

package runtime

import (
	"runtime/internal/sys"
	"unsafe"
)

// FixAlloc is a simple free-list allocator for fixed size objects.
// Malloc uses a FixAlloc wrapped around sysAlloc to manage its
// mcache and mspan objects.
//
// Memory returned by fixalloc.alloc is zeroed by default, but the
// caller may take responsibility for zeroing allocations by setting
// the zero flag to false. This is only safe if the memory never
// contains heap pointers.
//
// The caller is responsible for locking around FixAlloc calls.
// Callers can keep state in the object but the first word is
// smashed by freeing and reallocating.
//
// Consider marking fixalloc'd types not in heap by embedding
// runtime/internal/sys.NotInHeap.
// 只能分配固定size大小的内存分配器
type fixalloc struct {
	size   uintptr                     // 每次分配的内存大小
	first  func(arg, p unsafe.Pointer) // 回调函数,分配的内存第一次返回的时候调用 called first time p is returned
	arg    unsafe.Pointer              // first的参数
	list   *mlink                      // 调用free的时候，释放的内存会放在这里
	chunk  uintptr                     // 当前chunk指针，指向剩余的free的字节 use uintptr instead of unsafe.Pointer to avoid write barriers
	nchunk uint32                      // 当前chunk还剩下多少字节 bytes remaining in current chunk
	nalloc uint32                      // size of new chunks in bytes // 每个新chunk的大小
	inuse  uintptr                     // in-use bytes now 当前fixalloc中有多少正在使用的字节
	stat   *sysMemStat                 // 一个原子变量用来记录某个内存指标
	zero   bool                        // 是否为分配的内存块清零zero allocations
}

// A generic linked list of blocks.  (Typically the block is bigger than sizeof(MLink).)
// Since assignments to mlink.next will result in a write barrier being performed
// this cannot be used by some of the internal GC structures. For example when
// the sweeper is placing an unmarked object on the free list it does not want the
// write barrier to be called since that could result in the object being reachable.
type mlink struct { // 每块内存size不会比这个小，是和分配的内存公用一块内存，相当于C的联合
	_    sys.NotInHeap
	next *mlink
}

func MyPrintStack() {
	buf := make([]byte, 1024)
	for {
		n := Stack(buf, false)
		if n < len(buf) {
			println(string(buf[:n]))
			return
		}
		buf = make([]byte, 2*len(buf))
	}
}

// Initialize f to allocate objects of the given size,
// using the allocator to obtain chunks of memory. // 主要是根据size算出来每个chunk的大小
func (f *fixalloc) init(size uintptr, first func(arg, p unsafe.Pointer), arg unsafe.Pointer, stat *sysMemStat) {
	if size > _FixAllocChunk { //大小不能超过16K
		throw("runtime: fixalloc size too large")
	}
	if min := unsafe.Sizeof(mlink{}); size < min { //因为分配的内存和该结构体用的一块内存
		size = min
	}

	f.size = size   // 每次分配的内存大小
	f.first = first // 一个回调函数
	f.arg = arg     // 传递给first函数的参数
	f.list = nil
	f.chunk = 0
	f.nchunk = 0
	f.nalloc = uint32(_FixAllocChunk / size * size) // 每个chunk的大小，该算法保证chunk大小是size的整数倍，避免内存浪费 Round _FixAllocChunk down to an exact multiple of size to eliminate tail waste
	f.inuse = 0
	f.stat = stat
	f.zero = true
}

// 分配一个size大小的内存
func (f *fixalloc) alloc() unsafe.Pointer {
	if f.size == 0 {
		print("runtime: use of FixAlloc_Alloc before FixAlloc_Init\n")
		throw("runtime: internal error")
	}

	if f.list != nil { //先看看之前释放的内存列表有没有，有的话直接返回回去
		v := unsafe.Pointer(f.list)
		f.list = f.list.next
		f.inuse += f.size
		if f.zero {
			memclrNoHeapPointers(v, f.size) //内存清零
		}
		return v
	}
	if uintptr(f.nchunk) < f.size { // 不够用的，创建一个新的chunk
		f.chunk = uintptr(persistentalloc(uintptr(f.nalloc), 0, f.stat))
		f.nchunk = f.nalloc
	}

	v := unsafe.Pointer(f.chunk)
	if f.first != nil { //该块size大小的内存第一次被从chunk中拿出来使用的时候
		f.first(f.arg, v)
	}
	f.chunk = f.chunk + f.size //chunk指针后移
	f.nchunk -= uint32(f.size) // 当前chunk剩余空间减少
	f.inuse += f.size          // 所有正在使用byte数量
	return v
}

// 释放内存。释放的内存放到list中
func (f *fixalloc) free(p unsafe.Pointer) {
	f.inuse -= f.size
	v := (*mlink)(p)
	v.next = f.list // 释放的内存放到list中
	f.list = v
}
