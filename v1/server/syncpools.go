package server

import (
	"bytes"
	"strings"
	"sync"

	"github.com/open-policy-agent/opa/v1/util"
)

var (
	bytesReaderPool   = util.NewSyncPool[bytes.Reader]()
	stringBuilderPool = util.NewSyncPool[strings.Builder]()
	stringInternPool  = &internPool{interned: make(map[string]string)}
)

type internPool struct {
	mu       sync.RWMutex
	interned map[string]string
}

func (p *internPool) intern(s string) string {
	if s == "" {
		return ""
	}

	p.mu.RLock()
	if interned, ok := p.interned[s]; ok {
		p.mu.RUnlock()
		return interned
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	if interned, ok := p.interned[s]; ok {
		return interned
	}
	p.interned[s] = s
	return s
}

func (p *internPool) internMultiple(values ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, v := range values {
		if v == "" {
			continue
		}
		if _, ok := p.interned[v]; !ok {
			p.interned[v] = v
		}
	}
}

func internString(s string) string {
	return stringInternPool.intern(s)
}

func InternServerStrings(values ...string) {
	stringInternPool.internMultiple(values...)
}

func acquireBytesReader(b []byte) *bytes.Reader {
	reader := bytesReaderPool.Get()
	reader.Reset(b)
	return reader
}

func releaseBytesReader(reader *bytes.Reader) {
	if reader == nil {
		return
	}
	reader.Reset(nil)
	bytesReaderPool.Put(reader)
}

func acquireStringBuilder() *strings.Builder {
	builder := stringBuilderPool.Get()
	builder.Reset()
	return builder
}

func releaseStringBuilder(builder *strings.Builder) {
	if builder == nil {
		return
	}
	builder.Reset()
	stringBuilderPool.Put(builder)
}
