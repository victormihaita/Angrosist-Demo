package uploadhttp

import (
	"path/filepath"
	"strings"
)

// ownerTypeAllow is the allowlist of polymorphic owner types an upload may bind
// to (documents.owner_type). Anything else is rejected at the handler boundary.
var ownerTypeAllow = map[string]bool{
	"lead":             true,
	"conversation":     true,
	"listing":          true,
	"offer":            true,
	"sourcing_request": true,
}

// kindMIME maps an allowed document kind (documents.kind) to its MIME allowlist.
// MIME is decided by sniffing the bytes (http.DetectContentType), never trusting
// the client extension alone (SECURITY §1.6: MIME allow-list + magic-byte sniff).
//
// http.DetectContentType only recognizes a fixed set of signatures; CSV/plain
// text sniffs as "text/plain", and several office formats (xlsx/docx) are ZIP
// containers that sniff as "application/zip". The allowlists below account for
// what the sniffer actually returns so legitimate product lists are accepted
// while executables/scripts (which sniff as octet-stream or text) are not
// silently allowed under the wrong kind.
var kindMIME = map[string]map[string]bool{
	"product_list": {
		"application/pdf": true,
		// xlsx/docx/pptx and other OOXML are ZIP containers.
		"application/zip": true,
		// Legacy office (xls/doc) and other OLE compound files.
		"application/x-ole-storage": true,
		"application/octet-stream":  true, // legacy office often sniffs here
		"text/plain":                true, // csv / plain text
		"text/plain; charset=utf-8": true,
		"text/csv":                  true,
		"application/vnd.ms-excel":  true,
		"application/msword":        true,
	},
	"photo": {
		"image/jpeg": true,
		"image/png":  true,
		"image/webp": true,
	},
	"offer": {
		"application/pdf":           true,
		"application/zip":           true,
		"image/jpeg":                true,
		"image/png":                 true,
		"application/octet-stream":  true,
		"text/plain":                true,
		"text/plain; charset=utf-8": true,
	},
}

// extAllow restricts the *declared* filename extension per kind as a second,
// cheap guard layered on top of the byte sniff. Empty extension is rejected.
var extAllow = map[string]map[string]bool{
	"product_list": {".xlsx": true, ".xls": true, ".csv": true, ".doc": true, ".docx": true, ".pdf": true},
	"photo":        {".jpg": true, ".jpeg": true, ".png": true, ".webp": true},
	"offer":        {".pdf": true, ".jpg": true, ".jpeg": true, ".png": true, ".xlsx": true, ".docx": true},
}

// allowedKind reports whether kind is a known document kind.
func allowedKind(kind string) bool {
	_, ok := kindMIME[kind]
	return ok
}

// mimeAllowedForKind reports whether the sniffed MIME is acceptable for kind.
func mimeAllowedForKind(kind, mime string) bool {
	allow, ok := kindMIME[kind]
	if !ok {
		return false
	}
	return allow[strings.ToLower(strings.TrimSpace(mime))]
}

// extAllowedForKind reports whether the declared filename extension is acceptable
// for kind. The check is case-insensitive.
func extAllowedForKind(kind, filename string) bool {
	allow, ok := extAllow[kind]
	if !ok {
		return false
	}
	ext := strings.ToLower(filepath.Ext(filename))
	return allow[ext]
}
