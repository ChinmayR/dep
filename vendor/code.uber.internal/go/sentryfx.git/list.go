package sentryfx

import (
	"sync"

	"go.uber.org/zap/zapcore"
)

var _pool = sync.Pool{New: func() interface{} {
	return &encoder{make([]zapcore.Field, 0, 16)}
}}

// Most loggers accumulate context, but never end up writing a message that
// needs to be sent to Sentry. To avoid constantly copying maps, represent
// accumulated context as a linked list. This makes actually sending requests
// to Sentry slower, but that's relatively rare and already quite slow.
type node struct {
	next  *node
	field zapcore.Field
}

// Our linked list of fields is stored in reverse order (so that adding
// context to a logger doesn't mutate the receiver). Because fields mutate the
// state of the encoder, we need to reverse our linked list before encoding
// the fields. Since we don't want to reverse the list in-place, we should
// pool the extra storage required.
type encoder struct {
	fields []zapcore.Field
}

func newEncoder() *encoder {
	e := _pool.Get().(*encoder)
	e.fields = e.fields[:0]
	return e
}

func (enc *encoder) encode(context *node) map[string]interface{} {
	for head := context; head != nil; head = head.next {
		enc.fields = append(enc.fields, head.field)
	}

	m := zapcore.NewMapObjectEncoder()
	for i := len(enc.fields) - 1; i >= 0; i-- {
		enc.fields[i].AddTo(m)
	}

	return m.Fields
}

func (enc *encoder) free() {
	_pool.Put(enc)
}
