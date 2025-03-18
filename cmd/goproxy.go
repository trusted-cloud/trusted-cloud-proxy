// gomodproxy is a simple reference implementation of the core of a Go
// module proxy (https://golang.org/ref/mod), for pedagogical purposes.
// Each HTTP request is handled by directly executing the 'go' command.
//
// A realistic implementation would offer additional features, such as:
//
//   - Caching, so that sequential requests for the same module do not
//     necessarily result in repeated execution of the go command.
//   - Duplicate suppression, so that concurrent requests for the same
//     module do not result in duplicate work.
//   - Replication and load balancing, so that the server can be run on
//     multiple hosts sharing persistent storage.
//   - Cache eviction, to prevent unbounded growth of storage.
//   - A checksum database, to avoid the need for "trust on first use".
//   - Transport-layer security, to prevent eavesdropping in the network.
//   - Authentication, so that only permitted users are served.
//   - Access control, so that authenticated users may only read permitted packages.
//   - Persistent storage, so that deletion or temporary downtime of a
//     repository does not break existing clients.
//   - A content-delivery network, so that large .zip files can be
//     served from caches closer in the network to the requesting user.
//   - Monitoring, logging, tracing, profiling, and other observability
//     features for maintainers.
//
// Examples of production-grade proxies are:
// - The Go Module Mirror, https://proxy.golang.org/
// - The Athens Project,  https://docs.gomods.io/
// - GoFig, https://gofig.dev/
//
// The Go module proxy protocol (golang.org/ref/mod#goproxy-protocol) defines five endpoints:
//
// - MODULE/@v/VERSION.info
// - MODULE/@v/VERSION.mod
// - MODULE/@v/VERSION.zip
//
//	These three endpoints accept version query (such as a semver or
//	branch name), and are implemented by a 'go mod download' command,
//	which resolves the version query, downloads the content of the
//	module from its version-control system (VCS) repository, and
//	saves its content (.zip, .mod) and metadata (.info) in separate
//	files in the cache directory.
//
//	Although the client could extract the .mod file from the .zip
//	file, it is more efficient to request the .mod file alone during
//	the initial "minimum version selection" phase and then request
//	the complete .zip later only if needed.
//
//	The results of these requests may be cached indefinitely, using
//	the pair (module, resolved version) as the key.  The 'go mod
//	download' command effectively does this for us, storing previous
//	results in its cache directory.
//
// - MODULE/@v/list
// - MODULE/@latest (optional)
//
//	These two endpoints request information about the available
//	versions of a module, and are implemented by 'go list -m'
//	commands: /@v/list uses -versions to query the tags in the
//	version-control system that hosts the module, and /@latest uses
//	the query "module@latest" to obtain the current version.
//
//	Because the set of versions may change at any moment, caching the
//	results of these queries inevitably results in the delivery of
//	stale information to some users at least some of the time.
//
// To use this proxy:
//
//	$ go run . &
//	$ export GOPROXY=http://localhost:8000/mod
//	$ go get <module>
package main

// TODO: when should we emit StatusGone? (see github.com/golang/go/issues/30134)

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"golang.org/x/mod/module"
)

// var cachedir = filepath.Join(os.Getenv("HOME"), "gomodproxy-cache")
var cachedir = "/workspaces/trusted-cloud-proxy/cache"

var DestRepoToken = os.Getenv("GITHUB_TOKEN")

var SrcRepo = "pegasus-cloud.com/aes"
var DestRepo = "github.com/trusted-cloud"
var user = "dummy"

func main() {
	log.Println("Proxy Module Cache Directory:", cachedir)

	if err := os.MkdirAll(cachedir, 0755); err != nil {
		log.Fatalf("creating cache: %v", err)
	}

	log.Println("Mapping module from", SrcRepo, "to", DestRepo)
	log.Println("Token is required for", DestRepo, ":", DestRepoToken)
	log.Println("Starting server on :8000")

	// http.HandleFunc("/mod/", handleMod)
	// log.Fatal(http.ListenAndServe(":8000", nil))

	router := mux.NewRouter()
	router.HandleFunc("/{module:.+}/@v/list", list).Methods(http.MethodGet)
	router.HandleFunc("/{module:.+}/@v/{version}.info", info).Methods(http.MethodGet)
	router.HandleFunc("/{module:.+}/@v/{version}.mod", mod).Methods(http.MethodGet)
	router.HandleFunc("/{module:.+}/@v/{version}.zip", zip).Methods(http.MethodGet)
	log.Fatal(http.ListenAndServe(":8000", isValidPkg(router)))
}

func isValidPkg(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/"+SrcRepo) {
			http.Error(w, fmt.Sprintf("%s is ignored", r.URL), http.StatusNotFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func handleMod(w http.ResponseWriter, req *http.Request) {
	path := strings.TrimPrefix(req.URL.Path, "/mod/")

	fmt.Println(path)

	// if _, ok := prefixed(path, SrcRepo+"/"); !ok {
	// 	http.Error(w, fmt.Sprintf("This proxy only for package under %s", SrcRepo), http.StatusNotFound)
	// 	return
	// }

	// MODULE/@v/list
	if mod, ok := suffixed(path, "/@v/list"); ok {
		mod, err := module.UnescapePath(mod)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		log.Println("list", mod)

		versions, err := listVersionsGit(mod)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		w.Header().Set("Cache-Control", "no-store")
		for _, v := range versions {
			fmt.Fprintln(w, v)
		}
		return
	}

	// MODULE/@latest
	if mod, ok := suffixed(path, "/@latest"); ok {
		mod, err := module.UnescapePath(mod)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		log.Println("latest", mod)

		latest, err := resolve(mod, "latest")
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		info := InfoJSON{Version: latest.Version, Time: latest.Time}
		json.NewEncoder(w).Encode(info)
		return
	}

	// MODULE/@v/VERSION.{info,mod,zip}
	if rest, ext, ok := lastCut(path, "."); ok && isOneOf(ext, "mod", "info", "zip") {
		if mod, version, ok := cut(rest, "/@v/"); ok {
			mod, err := module.UnescapePath(mod)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			version, err := module.UnescapeVersion(version)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			log.Printf("%s %s@%s", ext, mod, version)

			m, err := download(mod, version)
			if err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}

			// The version may be a query such as a branch name.
			// Branches move, so we suppress HTTP caching in that case.
			// (To avoid repeated calls to download, the proxy could use
			// the module name and resolved m.Version as a key in a cache.)
			if version != m.Version {
				w.Header().Set("Cache-Control", "no-store")
				log.Printf("%s %s@%s => %s", ext, mod, version, m.Version)
			}

			// Return the relevant cached file.
			var filename, mimetype string
			switch ext {
			case "info":
				filename = m.Info
				mimetype = "application/json"
			case "mod":
				filename = m.GoMod
				mimetype = "text/plain; charset=UTF-8"
			case "zip":
				filename = m.Zip
				mimetype = "application/zip"
			}
			w.Header().Set("Content-Type", mimetype)
			if err := copyFile(w, filename); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			return
		}
	}

	http.Error(w, "bad request", http.StatusBadRequest)
}

// download runs 'go mod download' and returns information about a
// specific module version. It also downloads the module's dependencies.
func download(name, version string) (*ModuleDownloadJSON, error) {
	var mod ModuleDownloadJSON
	if err := runGo(&mod, "mod", "download", "-json", name+"@"+version); err != nil {
		return nil, err
	}
	if mod.Error != "" {
		return nil, fmt.Errorf("failed to download module %s: %v", name, mod.Error)
	}
	return &mod, nil
}

func list(w http.ResponseWriter, r *http.Request) {

	log.Println("list", r.URL.Path)

	mod := mux.Vars(r)["module"]

	mod, err := module.UnescapePath(mod)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	versions, err := listVersionsGit(mod)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	for _, v := range versions {
		fmt.Fprintln(w, v)
	}
}

// listVersionsGit runs 'git ls-remote --tags <GIT_HTTP_REPO>'
// and returns an unordered list of tags of the specified repo.
func listVersionsGit(name string) ([]string, error) {

	result := []string{}

	segment := strings.Split(name, "/")
	pkg := segment[len(segment)-1]

	// Construct the git command
	repoURL := fmt.Sprintf("%s/%s", DestRepo, pkg)
	log.Println("git", repoURL)

	gitURL := fmt.Sprintf("https://%s:%s@%s", user, DestRepoToken, repoURL)
	cmd := exec.Command("git", "ls-remote", "--tags", gitURL)

	// Execute the git command
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// Use rev | cut -d/ -f1 | rev to extract tag names
	reader := bufio.NewReader(stdout)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		line = strings.TrimSpace(line) // Remove leading/trailing whitespace
		segments := strings.Split(line, "/")

		// Check if the line contains enough segments to be a tag
		if len(segments) > 2 && strings.Contains(line, "refs/tags/") { // More robust tag check
			tagName := segments[len(segments)-1] // Get the last element

			// fmt.Println(tagName)
			result = append(result, tagName)
		}

	}

	if err := cmd.Wait(); err != nil {
		return nil, err
	}

	return result, nil
}

func info(w http.ResponseWriter, r *http.Request) {

	// filename := "/workspaces/trusted-cloud-proxy/vendor/pegasus-cloud.com/aes/toolkits/v0.4.5/v0.4.5.info"
	log.Println("info", r.URL.Path)

	filename := filepath.Join(cachedir, mux.Vars(r)["module"], mux.Vars(r)["version"], mux.Vars(r)["version"]+".info")

	if serveCachedFile(w, r, filename, "application/json") {
		return
	}

	http.Error(w, "info not found", http.StatusNotFound)

	//todo: download file
}

func mod(w http.ResponseWriter, r *http.Request) {

	// filename := "/workspaces/trusted-cloud-proxy/vendor/pegasus-cloud.com/aes/toolkits/v0.4.5/go.mod"
	log.Println("mod", r.URL.Path)

	filename := filepath.Join(cachedir, mux.Vars(r)["module"], mux.Vars(r)["version"], "go.mod")

	if serveCachedFile(w, r, filename, "text/plain; charset=UTF-8") {
		return
	}
	http.Error(w, "mod not found", http.StatusNotFound)

	//todo: download file
}

func zip(w http.ResponseWriter, r *http.Request) {

	// filename := "/workspaces/trusted-cloud-proxy/vendor/pegasus-cloud.com/aes/toolkits/v0.4.5/source.zip"
	log.Println("zip", r.URL.Path)

	filename := filepath.Join(cachedir, mux.Vars(r)["module"], mux.Vars(r)["version"], "source.zip")

	if serveCachedFile(w, r, filename, "application/zip") {
		return
	}
	http.Error(w, "zip not found", http.StatusNotFound)
	//todo: download file
}

func serveCachedFile(w http.ResponseWriter, r *http.Request, cachePath string, mime string) bool {

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", mime)

	if _, err := os.Stat(cachePath); err == nil {
		http.ServeFile(w, r, cachePath)
		return true
	}
	return false
}

func fetchAndCache(name, version string) error {
	return nil
}

// TODO:
func downloadGit(name, version string) (*ModuleDownloadJSON, error) {
	return &ModuleDownloadJSON{}, nil
}

// resolve runs 'go list -m' to resolve a module version query to a specific version.
func resolve(name, query string) (*ModuleListJSON, error) {
	var mod ModuleListJSON
	if err := runGo(&mod, "list", "-m", "-json", name+"@"+query); err != nil {
		return nil, err
	}
	if mod.Error != nil {
		return nil, fmt.Errorf("failed to list module %s: %v", name, mod.Error.Err)
	}
	return &mod, nil
}

// runGo runs the Go command and decodes its JSON output into result.
func runGo(result interface{}, args ...string) error {
	tmpdir, err := os.MkdirTemp("", "")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpdir)

	cmd := exec.Command("go", args...)
	cmd.Dir = tmpdir
	// Construct environment from scratch, for hygiene.
	cmd.Env = []string{
		"USER=" + os.Getenv("USER"),
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"NETRC=", // don't allow go command to read user's secrets
		"GOPROXY=direct",
		"GOCACHE=" + cachedir,
		"GOMODCACHE=" + cachedir,
		"GOSUMDB=",
	}
	cmd.Stdout = new(bytes.Buffer)
	cmd.Stderr = new(bytes.Buffer)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s failed: %v (stderr=<<%s>>)", cmd, err, cmd.Stderr)
	}
	if err := json.Unmarshal(cmd.Stdout.(*bytes.Buffer).Bytes(), result); err != nil {
		return fmt.Errorf("internal error decoding %s JSON output: %v", cmd, err)
	}
	return nil
}

// -- JSON schemas --

// ModuleDownloadJSON is the JSON schema of the output of 'go help mod download'.
type ModuleDownloadJSON struct {
	Path     string // module path
	Version  string // module version
	Error    string // error loading module
	Info     string // absolute path to cached .info file
	GoMod    string // absolute path to cached .mod file
	Zip      string // absolute path to cached .zip file
	Dir      string // absolute path to cached source root directory
	Sum      string // checksum for path, version (as in go.sum)
	GoModSum string // checksum for go.mod (as in go.sum)
}

// ModuleListJSON is the JSON schema of the output of 'go help list'.
type ModuleListJSON struct {
	Path      string          // module path
	Version   string          // module version
	Versions  []string        // available module versions (with -versions)
	Replace   *ModuleListJSON // replaced by this module
	Time      *time.Time      // time version was created
	Update    *ModuleListJSON // available update, if any (with -u)
	Main      bool            // is this the main module?
	Indirect  bool            // is this module only an indirect dependency of main module?
	Dir       string          // directory holding files for this module, if any
	GoMod     string          // path to go.mod file used when loading this module, if any
	GoVersion string          // go version used in module
	Retracted string          // retraction information, if any (with -retracted or -u)
	Error     *ModuleError    // error loading module
}

type ModuleError struct {
	Err string // the error itself
}

// InfoJSON is the JSON schema of the .info and @latest endpoints.
type InfoJSON struct {
	Version string
	Time    *time.Time
}

// -- helpers --

// suffixed reports whether x has the specified suffix,
// and returns the prefix.
func suffixed(x, suffix string) (rest string, ok bool) {
	if y := strings.TrimSuffix(x, suffix); y != x {
		return y, true
	}
	return
}

func prefixed(x, prefix string) (rest string, ok bool) {
	if y := strings.TrimPrefix(x, prefix); y != x {
		return y, true
	}
	return
}

// See https://github.com/golang/go/issues/46336
func cut(s, sep string) (before, after string, found bool) {
	if i := strings.Index(s, sep); i >= 0 {
		return s[:i], s[i+len(sep):], true
	}
	return s, "", false
}

func lastCut(s, sep string) (before, after string, found bool) {
	if i := strings.LastIndex(s, sep); i >= 0 {
		return s[:i], s[i+len(sep):], true
	}
	return s, "", false
}

// copyFile writes the content of the named file to dest.
func copyFile(dest io.Writer, name string) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(dest, f)
	return err
}

func isOneOf(s string, items ...string) bool {
	for _, item := range items {
		if s == item {
			return true
		}
	}
	return false
}
