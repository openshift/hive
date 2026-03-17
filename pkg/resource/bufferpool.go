package resource

import (
	"bytes"
	"sync"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

var bufferPool = sync.Pool{
	New: func() interface{} {
		return &bytes.Buffer{}
	},
}

func getBuffer() *bytes.Buffer {
	return bufferPool.Get().(*bytes.Buffer)
}

func returnBuffer(buf *bytes.Buffer) {
	buf.Reset()
	bufferPool.Put(buf)
}

func getIOStreams() genericclioptions.IOStreams {
	return genericclioptions.IOStreams{
		In:     getBuffer(),
		Out:    getBuffer(),
		ErrOut: getBuffer(),
	}
}

func returnIOStreams(ios genericclioptions.IOStreams) {
	if buf, ok := ios.In.(*bytes.Buffer); ok {
		returnBuffer(buf)
	}
	if buf, ok := ios.Out.(*bytes.Buffer); ok {
		returnBuffer(buf)
	}
	if buf, ok := ios.ErrOut.(*bytes.Buffer); ok {
		returnBuffer(buf)
	}
}
