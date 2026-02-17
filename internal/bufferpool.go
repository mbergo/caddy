// Copyright 2015 Matthew Holt and The Caddy Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package internal

import (
	"bytes"
	"sync"
)

// MaxBufferSize is the maximum size of a buffer in bytes
// that will be returned to a pool. Buffers larger than this
// are discarded so memory can be reclaimed by the garbage
// collector.
const MaxBufferSize = 64 * 1024

// PutBuffer returns a buffer to the pool after resetting it,
// but only if it is smaller than MaxBufferSize. This prevents
// memory bloat from large buffers being kept in the pool.
func PutBuffer(pool *sync.Pool, buf *bytes.Buffer) {
	if buf.Cap() > MaxBufferSize {
		return
	}
	buf.Reset()
	pool.Put(buf)
}
