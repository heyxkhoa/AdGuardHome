// translations downloads translations, uploads translations, prints summary
// for translations, prints unused strings.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AdguardTeam/AdGuardHome/internal/aghio"
	"github.com/AdguardTeam/golibs/errors"
	"github.com/AdguardTeam/golibs/log"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

const (
	twoskyConfFile = "../../.twosky.json"
	localesDir     = "../../client/src/__locales"
	baseFile       = "en.json"
	projectID      = "home"
	srcDir         = "../../client/src"
	twoskyURI      = "https://twosky.int.agrd.dev/api/v1"

	readLimit = 1 * 1024 * 1024
)

// languages is a map, where key is language code and value is display name.
type languages map[string]string

func main() {
	if len(os.Args) == 1 {
		usage("need a command")
	}

	uriStr := os.Getenv("TWOSKY_URI")
	if uriStr == "" {
		uriStr = twoskyURI
	}

	id := os.Getenv("TWOSKY_PROJECT_ID")
	if id == "" {
		id = projectID
	}

	t := readTwosky()

	switch os.Args[1] {
	case "summary":
		summary(t.Languages)
	case "download":
		var w int
		fs := flag.NewFlagSet("download", flag.ExitOnError)
		fs.IntVar(&w, "w", 1, "number of workers")
		fs.IntVar(&w, "workers", 1, "number of workers")

		err := fs.Parse(os.Args[2:])
		check(err)

		if w < 1 {
			w = 1
		}

		download(uriStr, id, t.Languages, w)
	case "unused":
		unused()
	case "upload":
		upload(uriStr, id, t.BaseLocale)
	case "help", "-help", "--help":
		usage("")
	default:
		usage("unknown command")
	}
}

// check is a simple error-checking helper for scripts.
func check(err error) {
	if err != nil {
		panic(err)
	}
}

// mark is a simple error-printing helper for scripts.
func mark(err error) {
	if err != nil {
		log.Println(err)
	}
}

// usage prints usage.  If s is not empty string print s and exit with code 1,
// otherwise exit with code 0.
func usage(s string) {
	if s != "" {
		fmt.Println(s)
	}

	fmt.Println(`Usage: go run main.go <command> [args]
Commands:
  help
    	Print usage
  summary
    	Print summary
  download [n]
    	Download translations. n is number of workers.
  unused
    	Print unused strings
  upload
    	Upload translations`)

	if s != "" {
		os.Exit(1)
	}

	os.Exit(0)
}

// readTwosky returns configuration.
func readTwosky() (t twoskyConf) {
	b, err := os.ReadFile(twoskyConfFile)
	check(err)

	var ts []twoskyConf
	err = json.Unmarshal(b, &ts)
	err = errors.Annotate(err, "unmarshalling %s: %w", twoskyConfFile)
	check(err)

	if len(ts) == 0 {
		log.Fatalf("%s is empty", twoskyConfFile)
	}

	return ts[0]
}

// readLocales reads file with name fn and returns a map, where key is text
// label and value is localization.
func readLocales(fn string) (locales map[string]string) {
	b, err := os.ReadFile(fn)
	check(err)

	locales = make(map[string]string)
	err = json.Unmarshal(b, &locales)
	err = errors.Annotate(err, "unmarshalling %s: %w", fn)
	check(err)

	return locales
}

// summary prints summary for translations.
func summary(lns languages) {
	basePath := filepath.Join(localesDir, baseFile)
	baseLocale := readLocales(basePath)
	bl := float64(len(baseLocale))

	sum := make(map[string]float64)

	for ln := range lns {
		path := filepath.Join(localesDir, ln+".json")
		l := readLocales(path)

		if path == basePath {
			continue
		}

		sum[ln] = float64(len(l)) * 100 / bl
	}

	printSummary(sum)
}

// printSummary to stdout.
func printSummary(sum map[string]float64) {
	keys := maps.Keys(sum)
	slices.Sort(keys)

	for _, v := range keys {
		fmt.Printf("%s\t %6.2f\n", v, sum[v])
	}
}

// download translations.  w is number of workers.
func download(uriStr, id string, lns languages, w int) {
	downloadURI, err := url.JoinPath(uriStr, "download")
	check(err)

	urls := make(chan string)

	for i := 0; i < w; i++ {
		go downloadWorker(urls)
	}

	for ln := range lns {
		v := url.Values{}
		v.Set("format", "json")
		v.Set("language", ln)
		v.Set("filename", baseFile)
		v.Set("project", id)

		urls <- downloadURI + "?" + v.Encode()
	}

	close(urls)
}

// downloadWorker downloads translations by received urls.
func downloadWorker(urls <-chan string) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	for u := range urls {
		url, err := url.Parse(u)
		check(err)

		lang := url.Query().Get("language")

		if lang == "" {
			log.Fatalf("language is empty")
		}

		resp, err := client.Get(u)
		if err != nil {
			mark(err)

			continue
		}

		if resp.StatusCode != http.StatusOK {
			err = fmt.Errorf("url: %s; status code: %s", u, http.StatusText(resp.StatusCode))
			mark(err)

			continue
		}

		limitReader, err := aghio.LimitReader(resp.Body, readLimit)
		if err != nil {
			mark(err)

			continue
		}

		buf, err := io.ReadAll(limitReader)
		if err != nil {
			mark(err)

			continue
		}

		path := filepath.Join(localesDir, lang+".json")
		err = os.WriteFile(path, buf, 0o664)
		mark(err)

		err = resp.Body.Close()
		mark(err)

		fmt.Println(path)
	}
}

// unused prints unused text labels.
func unused() {
	names := []string{}
	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if strings.HasPrefix(path, localesDir) {
			return nil
		}

		if strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".json") {
			names = append(names, path)
		}

		return nil
	})

	err = errors.Annotate(err, "filepath walking %s: %w", srcDir)
	check(err)

	files := [][]byte{}
	for _, n := range names {
		var buf []byte
		buf, err = os.ReadFile(n)
		check(err)

		files = append(files, buf)
	}

	printUnused(files)
}

// printUnused text labels to stdout.
func printUnused(files [][]byte) {
	basePath := filepath.Join(localesDir, baseFile)
	baseLocale := readLocales(basePath)

	knownUsed := map[string]bool{
		"blocking_mode_refused":   true,
		"blocking_mode_nxdomain":  true,
		"blocking_mode_custom_ip": true,
	}

	unused := []string{}
	for k := range baseLocale {
		if knownUsed[k] {
			continue
		}

		used := false
		for _, f := range files {
			if bytes.Contains(f, []byte(k)) {
				used = true

				break
			}
		}

		if !used {
			unused = append(unused, k)
		}
	}

	slices.Sort(unused)
	for _, v := range unused {
		fmt.Println(v)
	}
}

// upload base translation.
func upload(uriStr, id, baseLocale string) {
	uploadURI, err := url.JoinPath(uriStr, "upload")
	check(err)

	lang := os.Getenv("UPLOAD_LANGUAGE")
	if lang == "" {
		lang = baseLocale
	}

	v := url.Values{}
	v.Set("format", "json")
	v.Set("language", lang)
	v.Set("filename", baseFile)
	v.Set("project", id)

	basePath := filepath.Join(localesDir, baseFile)
	b, err := os.ReadFile(basePath)
	check(err)

	url := uploadURI + "?" + v.Encode()

	var buf bytes.Buffer
	buf.Write(b)

	var client http.Client
	resp, err := client.Post(url, "application/json", &buf)
	check(err)

	defer func() {
		err = resp.Body.Close()
		check(err)
	}()

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("status code is not ok: %s", http.StatusText(resp.StatusCode))
	}
}

// twoskyConf is the configuration structure for localization.
type twoskyConf struct {
	Languages        languages `json:"languages"`
	ProjectID        string    `json:"project_id"`
	BaseLocale       string    `json:"base_locale"`
	LocalizableFiles []string  `json:"localizable_files"`
}
