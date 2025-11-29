package ast

import (
	"bytes"
	"strings"
	"sync"

	"github.com/open-policy-agent/opa/v1/util"
)

var (
	TermPtrPool     = util.NewSyncPool[Term]()
	BytesReaderPool = util.NewSyncPool[bytes.Reader]()
	IndexResultPool = util.NewSyncPool[IndexResult]()
	bbPool          = util.NewSyncPool[bytes.Buffer]()
	// Needs custom pool because of custom Put logic.
	sbPool = &stringBuilderPool{
		pool: sync.Pool{
			New: func() any {
				return &strings.Builder{}
			},
		},
	}
	// Needs custom pool because of custom Put logic.
	varVisitorPool = &vvPool{
		pool: sync.Pool{
			New: func() any {
				return NewVarVisitor()
			},
		},
	}

	// Slice pools for MarshalJSON operations
	// Custom pool for []map[string]any to properly clear maps before reuse
	mapStringAnySlicePool = &mapStringAnySlicePoolType{
		pool: sync.Pool{
			New: func() any {
				s := make([]map[string]any, 8)
				return &s
			},
		},
	}
	// Standard pool for [][2]*Term - no need for custom cleanup
	termPair2SlicePool = util.NewSlicePool[[2]*Term](8)
)

type (
	stringBuilderPool       struct{ pool sync.Pool }
	vvPool                  struct{ pool sync.Pool }
	mapStringAnySlicePoolType struct{ pool sync.Pool }
)

func (p *stringBuilderPool) Get() *strings.Builder {
	return p.pool.Get().(*strings.Builder)
}

func (p *stringBuilderPool) Put(sb *strings.Builder) {
	sb.Reset()
	p.pool.Put(sb)
}

func (p *vvPool) Get() *VarVisitor {
	return p.pool.Get().(*VarVisitor)
}

func (p *vvPool) Put(vv *VarVisitor) {
	if vv != nil {
		vv.Clear()
		p.pool.Put(vv)
	}
}

func (p *mapStringAnySlicePoolType) Get(length int) *[]map[string]any {
	s := p.pool.Get().(*[]map[string]any)
	d := *s

	// Grow capacity if needed
	if cap(d) < length {
		// Need to allocate new slice with more capacity
		newSlice := make([]map[string]any, length)
		d = newSlice
	} else {
		d = d[:length]
	}

	// Clear each map in the slice and ensure we have fresh maps
	for i := range d {
		if d[i] == nil {
			d[i] = make(map[string]any, 4) // Pre-allocate small capacity
		} else {
			clear(d[i]) // Clear existing map
		}
	}

	*s = d
	return s
}

func (p *mapStringAnySlicePoolType) Put(s *[]map[string]any) {
	if s != nil {
		p.pool.Put(s)
	}
}
