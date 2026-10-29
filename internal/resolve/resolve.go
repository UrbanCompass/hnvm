// Package resolve turns a version spec (exact, dist-tag or semver range) into an exact version.
//
// Same algorithm as resolve_ver in the bash implementation, minus the node it needed to run semver:
//
//  1. an exact version is returned as is
//  2. a result cached in $HNVM_PATH/.tmp/<name>/<sanitized spec> within HNVM_RANGE_CACHE seconds is reused
//  3. the registry is asked for <name>/<spec>, which answers dist-tags and exact versions
//  4. an installed version in $HNVM_PATH/<name> that satisfies the range is used (the highest one)
//  5. otherwise the highest published version that satisfies the range, or the dist-tag of that name
package resolve

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/UrbanCompass/hnvm/internal/output"
	"github.com/UrbanCompass/hnvm/internal/semver"
)

// DefaultRegistry is where versions are resolved, as in bash (HNVM_PNPM_REGISTRY only affects pnpm downloads).
const DefaultRegistry = "https://registry.npmjs.org"

var exact = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
var unsafeChars = regexp.MustCompile(`[^a-z0-9_]`)

// IsExact reports whether v is a plain MAJOR.MINOR.PATCH version.
func IsExact(v string) bool { return exact.MatchString(v) }

// Resolver resolves versions for one invocation.
type Resolver struct {
	Root     string        // $HNVM_PATH
	CacheTTL time.Duration // HNVM_RANGE_CACHE
	Registry string        // defaults to DefaultRegistry
	Out      *output.Output
	Client   *http.Client
	Now      func() time.Time
}

// Result is a resolved version and how it was found, for `hnvm current`.
type Result struct {
	Version string
	How     string // "exact", "cache", "registry", "local", "range", "dist-tag"
}

// Resolve resolves spec for package name and prints the same "Resolved ..." line bash did. Errors
// are already printed when they are install.ErrReported-like; the returned error is for the caller to exit on.
func (r *Resolver) Resolve(ctx context.Context, name, spec string) (Result, error) {
	res, err := r.resolve(ctx, name, spec)
	if err != nil {
		return res, err
	}
	// No quotes: bash's """Resolved $name ""${initial_ver}"" to ..."" was only string concatenation
	r.Out.Info(`Resolved %s %s to %s`, name, spec, res.Version)
	return res, nil
}

// Quiet resolves without printing anything, for the CLI.
func (r *Resolver) Quiet(ctx context.Context, name, spec string) (Result, error) {
	out := r.Out
	r.Out = output.Discard()
	defer func() { r.Out = out }()
	return r.resolve(ctx, name, spec)
}

func (r *Resolver) resolve(ctx context.Context, name, spec string) (Result, error) {
	if IsExact(spec) {
		return Result{spec, "exact"}, nil
	}

	cacheFile := filepath.Join(r.Root, ".tmp", name, unsafeChars.ReplaceAllString(spec, "_"))
	if v, ok := r.cached(cacheFile); ok {
		return Result{v, "cache"}, nil
	}

	r.Out.Caution(`Resolving %s "%s" is slower than providing an exact version.`, name, spec)

	res, err := r.lookup(ctx, name, spec)
	if err != nil {
		return res, err
	}

	if err := os.MkdirAll(filepath.Dir(cacheFile), 0o755); err == nil {
		// Written to a temp file and renamed, so a concurrent reader never sees a partial version.
		tmp := fmt.Sprintf("%s.%d", cacheFile, os.Getpid())
		if os.WriteFile(tmp, []byte(res.Version+"\n"), 0o644) == nil {
			os.Rename(tmp, cacheFile)
		}
	}
	return res, nil
}

func (r *Resolver) cached(file string) (string, bool) {
	fi, err := os.Stat(file)
	if err != nil {
		return "", false
	}
	now := time.Now
	if r.Now != nil {
		now = r.Now
	}
	// Same comparison as bash: the file counts while now - ttl <= mtime (second precision).
	if now().Add(-r.CacheTTL).Unix() > fi.ModTime().Unix() {
		return "", false
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return "", false
	}
	v := strings.TrimSpace(string(data))
	return v, IsExact(v)
}

func (r *Resolver) lookup(ctx context.Context, name, spec string) (Result, error) {
	// The registry answers dist-tags and exact versions directly. Ranges get an error, which is fine.
	var doc struct {
		Version string `json:"version"`
	}
	if r.getJSON(ctx, r.registry()+"/"+name+"/"+url.PathEscape(spec), &doc) == nil && IsExact(doc.Version) {
		return Result{doc.Version, "registry"}, nil
	}

	if v, ok := semver.MaxSatisfying(Installed(r.Root, name), spec); ok {
		return Result{v, "local"}, nil
	}

	r.Out.Caution(`"%s" is not satisfied by any local version and must be resolved asynchronously.`, spec)

	var pkg struct {
		Versions map[string]json.RawMessage `json:"versions"`
		DistTags map[string]string          `json:"dist-tags"`
	}
	if err := r.getJSON(ctx, r.registry()+"/"+name, &pkg); err != nil {
		r.Out.Error(`Failed to resolve "%s" for %s: %v`, spec, name, err)
		return Result{}, ErrReported
	}

	if semver.ValidRange(spec) {
		versions := make([]string, 0, len(pkg.Versions))
		for v := range pkg.Versions {
			versions = append(versions, v)
		}
		if v, ok := semver.MaxSatisfying(versions, spec); ok && IsExact(v) {
			return Result{v, "range"}, nil
		}
		r.Out.Error(`Failed to find a matching version for "%s"!`, spec)
		return Result{}, ErrReported
	}
	if v, ok := pkg.DistTags[spec]; ok && IsExact(v) {
		return Result{v, "dist-tag"}, nil
	}
	r.Out.Error(`"%s" was not a valid semver range, nor was it a dist-tag of %s`, spec, name)
	return Result{}, ErrReported
}

// ErrReported means the failure was already printed.
var ErrReported = fmt.Errorf("")

func (r *Resolver) registry() string {
	if r.Registry != "" {
		return strings.TrimSuffix(r.Registry, "/")
	}
	return DefaultRegistry
}

func (r *Resolver) getJSON(ctx context.Context, u string, v any) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	// The abbreviated packument is much smaller and has everything needed here.
	req.Header.Set("Accept", "application/vnd.npm.install-v1+json; q=1.0, application/json; q=0.8")
	client := r.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", u, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// Installed lists the versions of tool in $HNVM_PATH/<tool>, as directory names.
func Installed(root, tool string) []string {
	entries, err := os.ReadDir(filepath.Join(root, tool))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && IsExact(e.Name()) {
			out = append(out, e.Name())
		}
	}
	return out
}
