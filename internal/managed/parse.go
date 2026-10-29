package managed

import (
	"bytes"
	"errors"
)

// maxNestingDepth bounds how deeply arrays and dictionaries may nest. Real profiles are only a few levels deep.
const maxNestingDepth = 32

var errTooDeep = errors.New("plist: nested too deeply")

// parse decodes a profile in either format and returns its top-level dictionary. A binary plist starts with "bplist";
// anything else is treated as XML.
func parse(data []byte) (map[string]any, error) {
	var (
		v   any
		err error
	)
	if bytes.HasPrefix(data, []byte("bplist")) {
		v, err = parseBinary(data)
	} else {
		v, err = parseXML(data)
	}
	if err != nil {
		return nil, err
	}
	root, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New("plist: top level is not a dictionary")
	}
	return root, nil
}
