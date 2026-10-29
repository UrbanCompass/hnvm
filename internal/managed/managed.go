// Package managed reads the MDM-delivered managed preferences profile for the com.compass.hnvm domain. A managed value
// wins over the environment and every .hnvmrc, so it is the first source the config layer consults. It is the Go
// counterpart of lib/hnvm/managed-prefs.sh, which reads the same file through /usr/bin/defaults.
package managed

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// DefaultPath is where MDM writes the profile on macOS. Only an administrator can change it.
const DefaultPath = "/Library/Managed Preferences/com.compass.hnvm.plist"

// maxFileSize bounds how much of a profile is read. Real profiles are a few kilobytes; the cap keeps the parser's
// allocations proportional to a small input.
const maxFileSize = 1 << 20

// Prefs is a parsed managed preferences profile. The zero value, and a nil *Prefs, both mean no profile: every lookup
// reports the key as unset.
type Prefs struct {
	root map[string]any
}

// opaque stands in for valid plist values that hnvm never reads (date, data, uid, null). They count as set for Keys
// but are never a Bool, a List or a Raw value.
type opaque struct{ kind string }

// Load reads and parses the profile at path. A missing file is not an error: it returns an empty Prefs. Other read or
// parse failures return an error.
func Load(path string) (*Prefs, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &Prefs{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, maxFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFileSize {
		return nil, fmt.Errorf("%s: larger than %d bytes", path, maxFileSize)
	}
	root, err := parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &Prefs{root: root}, nil
}

// LoadDefault returns the profile at DefaultPath on macOS and an empty Prefs elsewhere. It never fails: an unreadable or
// corrupt profile counts as no profile, as it does for the shell, which ignores `defaults` failures.
func LoadDefault() *Prefs {
	if runtime.GOOS != "darwin" {
		return &Prefs{}
	}
	return loadOrEmpty(DefaultPath)
}

func loadOrEmpty(path string) *Prefs {
	p, err := Load(path)
	if err != nil {
		return &Prefs{}
	}
	return p
}

func (p *Prefs) lookup(key string) (any, bool) {
	if p == nil || p.root == nil {
		return nil, false
	}
	v, ok := p.root[key]
	return v, ok
}

// Bool returns the managed boolean for key. It accepts 1, true and yes as true, and 0, false and no as false, ignoring
// case. Booleans and integers 1 and 0 are accepted too, and so are reals that print as 1 or 0, which is how `defaults`
// would show them. Anything else, or an unset key, gives ok=false.
func (p *Prefs) Bool(key string) (val bool, ok bool) {
	v, found := p.lookup(key)
	if !found {
		return false, false
	}
	s, isScalar := scalarText(v)
	if !isScalar {
		return false, false
	}
	switch strings.ToLower(s) {
	case "1", "true", "yes":
		return true, true
	case "0", "false", "no":
		return false, true
	}
	return false, false
}

// List returns the managed value for key as a comma-separated list. A string passes through unchanged. An array has its
// scalar elements joined with commas; elements that are not scalars (dictionaries, dates, data) are skipped. An empty
// string or array gives ("", true), which is a managed value that adds nothing. An unset key, or a dictionary, gives
// ok=false.
func (p *Prefs) List(key string) (csv string, ok bool) {
	v, found := p.lookup(key)
	if !found {
		return "", false
	}
	if arr, isArray := v.([]any); isArray {
		parts := make([]string, 0, len(arr))
		for _, el := range arr {
			if s, isScalar := scalarText(el); isScalar {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ","), true
	}
	s, isScalar := scalarText(v)
	if !isScalar {
		return "", false
	}
	return s, true
}

// Raw returns the value for key as text for display, such as `hnvm config`. Scalars print as they are (booleans as
// true or false), and arrays print in parentheses with their scalar elements separated by ", ". Dictionaries and
// values hnvm does not read give ok=false.
func (p *Prefs) Raw(key string) (string, bool) {
	v, found := p.lookup(key)
	if !found {
		return "", false
	}
	if arr, isArray := v.([]any); isArray {
		parts := make([]string, 0, len(arr))
		for _, el := range arr {
			if s, isScalar := scalarText(el); isScalar {
				parts = append(parts, s)
			}
		}
		return "(" + strings.Join(parts, ", ") + ")", true
	}
	return scalarText(v)
}

// Keys returns the keys set in the profile, sorted.
func (p *Prefs) Keys() []string {
	if p == nil {
		return nil
	}
	keys := make([]string, 0, len(p.root))
	for k := range p.root {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// DistrustSettings is the effective HNVM_DISTRUST_CERTS* configuration after the managed profile has been applied. The
// *Managed flags are true for the keys the profile sets, so messages do not suggest a setting the user cannot change.
type DistrustSettings struct {
	Enabled, Extra, Strict                      string
	EnabledManaged, ExtraManaged, StrictManaged bool
}

// ApplyDistrust applies the managed distrust settings over the environment and .hnvmrc values passed in, following
// apply_managed_distrust_certs in lib/hnvm/managed-prefs.sh:
//   - HNVM_DISTRUST_CERTS and HNVM_DISTRUST_CERTS_STRICT: a managed boolean replaces the given value.
//   - HNVM_DISTRUST_CERTS_EXTRA: managed fingerprints come first, then the given ones. An empty managed list adds
//     nothing and leaves the flag unset.
//
// A key the profile does not set, or sets to a non-boolean, leaves the given value alone.
func (p *Prefs) ApplyDistrust(enabled, extra, strict string) DistrustSettings {
	s := DistrustSettings{Enabled: enabled, Extra: extra, Strict: strict}

	if v, ok := p.Bool("HNVM_DISTRUST_CERTS"); ok {
		s.Enabled = strconv.FormatBool(v)
		s.EnabledManaged = true
	}

	if managed, ok := p.List("HNVM_DISTRUST_CERTS_EXTRA"); ok && managed != "" {
		s.Extra = managed
		if extra != "" {
			s.Extra = managed + "," + extra
		}
		s.ExtraManaged = true
	}

	if v, ok := p.Bool("HNVM_DISTRUST_CERTS_STRICT"); ok {
		s.Strict = strconv.FormatBool(v)
		s.StrictManaged = true
	}

	return s
}

// scalarText returns the text of a string, boolean, integer or real value. Other kinds give ok=false.
func scalarText(v any) (string, bool) {
	switch t := v.(type) {
	case string:
		return t, true
	case bool:
		return strconv.FormatBool(t), true
	case int64:
		return strconv.FormatInt(t, 10), true
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), true
	}
	return "", false
}
