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
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gorilla/mux"
	"golang.org/x/mod/module"
)

var CacheDir, DestRepoToken, DestRepo, SrcRepo, Port string

var user = "dummy"

func main() {

	Port = os.Getenv("PORT")
	if Port == "" {
		Port = "8078"
	}

	CacheDir = os.Getenv("CACHE_DIR")
	if CacheDir == "" {
		CacheDir = "/tmp/cache"
	}

	DestRepoToken = os.Getenv("REPO_TOKEN")
	if DestRepoToken == "" {
		log.Fatal("Error: REPO_TOKEN environment variable not set")
	}

	SrcRepo = os.Getenv("SRC_REPO")
	if SrcRepo == "" {
		log.Fatal("Error: SRC_REPO environment variable not set")
	}

	DestRepo = os.Getenv("DEST_REPO")
	if DestRepo == "" {
		log.Fatal("Error: DEST_REPO environment variable not set")
	}

	log.Println("Proxy Module Cache Directory:", CacheDir)

	if err := os.MkdirAll(CacheDir, 0755); err != nil {
		log.Fatalf("creating cache: %v", err)
	}

	log.Println("Mapping module from", SrcRepo, "to", DestRepo)
	log.Println("Token is required for", DestRepo, ":", DestRepoToken)
	log.Println("Starting server on :", Port)

	router := mux.NewRouter()
	router.HandleFunc("/{module:.+}/@v/list", list).Methods(http.MethodGet)
	router.HandleFunc("/{module:.+}/@v/{version}.info", info).Methods(http.MethodGet)
	router.HandleFunc("/{module:.+}/@v/{version}.mod", mod).Methods(http.MethodGet)
	router.HandleFunc("/{module:.+}/@v/{version}.zip", zip).Methods(http.MethodGet)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", Port), isValidPkg(router)))
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

	// filename := "/workspaces/trusted-cloud-proxy/cache/pegasus-cloud.com/aes/toolkits/v0.4.5/v0.4.5.info"
	log.Println("info", r.URL.Path)

	module := mux.Vars(r)["module"]
	version := mux.Vars(r)["version"]

	filename := filepath.Join(CacheDir, module, version, version+".info")

	if serveCachedFile(w, r, filename, "application/json") {
		return
	}

	if err := fetchAndCache(module, version); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if serveCachedFile(w, r, filename, "application/json") {
		return
	}
}

func mod(w http.ResponseWriter, r *http.Request) {

	// filename := "/workspaces/trusted-cloud-proxy/cache/pegasus-cloud.com/aes/toolkits/v0.4.5/go.mod"
	log.Println("mod", r.URL.Path)

	module := mux.Vars(r)["module"]
	version := mux.Vars(r)["version"]

	filename := filepath.Join(CacheDir, module, version, "go.mod")

	if serveCachedFile(w, r, filename, "text/plain; charset=UTF-8") {
		return
	}

	if err := fetchAndCache(module, version); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if serveCachedFile(w, r, filename, "text/plain; charset=UTF-8") {
		return
	}
}

func zip(w http.ResponseWriter, r *http.Request) {

	// filename := "/workspaces/trusted-cloud-proxy/cache/pegasus-cloud.com/aes/toolkits/v0.4.5/source.zip"
	log.Println("zip", r.URL.Path)
	module := mux.Vars(r)["module"]
	version := mux.Vars(r)["version"]

	filename := filepath.Join(CacheDir, module, version, "source.zip")

	if serveCachedFile(w, r, filename, "application/zip") {
		return
	}

	if err := fetchAndCache(module, version); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if serveCachedFile(w, r, filename, "application/zip") {
		return
	}
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

	segment := strings.Split(name, "/")
	pkg := segment[len(segment)-1]

	repoURL := filepath.Join(DestRepo, pkg)
	log.Println("git", repoURL)

	// Create a temporary directory for the git clone
	cloneTempDir, err := os.MkdirTemp("", "git-clone-temp")
	if err != nil {
		log.Fatalf("Error creating temporary directory: %s", err)
	}
	// defer os.RemoveAll(cloneTempDir) // Clean up the clone temp dir when the program exits

	// create cached directory
	destDir := filepath.Join(CacheDir, name, version)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	// 5. Construct the git clone command with the token and branch
	cloneURL := fmt.Sprintf("https://dummy:%s@%s", DestRepoToken, repoURL)

	cmd := exec.Command("git", "clone", "-b", version, cloneURL, cloneTempDir)

	// 6. Execute the git clone command
	if _, err := cmd.CombinedOutput(); err != nil {
		return err
	}

	// 7. Get the git log date
	logCmd := exec.Command("git", "log", "-1", "--format=%cI")
	logCmd.Dir = cloneTempDir // Set the working directory to the cloned repo

	// Set the GIT_PAGER environment variable to "cat"
	env := os.Environ()
	env = append(env, "GIT_PAGER=cat")
	logCmd.Env = env

	logOutput, err := logCmd.CombinedOutput()
	if err != nil {
		log.Fatalf("Error getting git log date: %s\nOutput: %s", err, string(logOutput))
	}

	logDate := strings.TrimSpace(string(logOutput))

	// 8. Create the Info struct
	info := Info{
		Version: version,
		Time:    logDate,
	}

	// 9. Marshal the Info struct to JSON
	jsonData, err := json.Marshal(info)
	if err != nil {
		log.Fatalf("Error marshaling JSON: %s", err)
	}

	// 10. Create the filename and destination path for info
	infoFilename := fmt.Sprintf("%s.info", version)
	infoDestPath := filepath.Join(destDir, infoFilename)

	// 11. Write the JSON data to the file in the tmp directory
	err = os.WriteFile(infoDestPath, jsonData, 0644)
	if err != nil {
		log.Fatalf("Error writing file: %s", err)
	}

	// 12. Copy go.mod to the tmp directory
	sourceGoMod := filepath.Join(cloneTempDir, "go.mod") // Source path in the cloned repo
	destGoMod := filepath.Join(destDir, "go.mod")        // Destination in the tmp directory

	err = copyFile(sourceGoMod, destGoMod)
	if err != nil {
		log.Fatalf("Error copying go.mod: %s", err)
	}

	// 13. Create the zip archive
	prefix := fmt.Sprintf("%s@%s/", name, version) // Correct prefix format
	zipCmd := exec.Command("git", "archive",
		fmt.Sprintf("--prefix=%s", prefix), // Use formatted prefix
		"--format", "zip",
		"--output", "source.zip",
		version, // Specify the tag for the archive
		".")

	zipCmd.Dir = cloneTempDir // Execute the command within the cloned repo

	zipOutput, err := zipCmd.CombinedOutput()
	if err != nil {
		log.Fatalf("Error creating zip archive: %s\nOutput: %s", err, string(zipOutput))
	}

	sourceZip := filepath.Join(cloneTempDir, "source.zip") // Source path in the cloned repo
	destZip := filepath.Join(destDir, "source.zip")        // Destination in the tmp directory

	err = copyFile(sourceZip, destZip)
	if err != nil {
		log.Fatalf("Error copying go.mod: %s", err)
	}

	return nil
}

// copyFile copies a file from source to destination
func copyFile(source, destination string) error {
	sourceFile, err := os.Open(source)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	return nil
}

type Info struct {
	Version string `json:"Version"`
	Time    string `json:"Time"`
}
