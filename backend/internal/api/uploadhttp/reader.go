package uploadhttp

import "io"

// rewindReader concatenates an already-consumed prefix (the bytes read for MIME
// sniffing) with the remaining stream, so the full file is stored without needing
// to Seek the multipart part (which is not always seekable). It avoids buffering
// the whole upload in memory.
type rewindReader struct {
	head []byte
	rest io.Reader
}

// newRewindReader returns a reader that yields head followed by rest.
func newRewindReader(head []byte, rest io.Reader) io.Reader {
	return &rewindReader{head: head, rest: rest}
}

func (r *rewindReader) Read(p []byte) (int, error) {
	if len(r.head) > 0 {
		n := copy(p, r.head)
		r.head = r.head[n:]
		return n, nil
	}
	return r.rest.Read(p)
}
