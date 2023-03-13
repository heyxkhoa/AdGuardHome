// translations downloads translations, uploads translations, prints summary
// for translations, prints unused strings.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/AdguardTeam/golibs/errors"
	"github.com/AdguardTeam/golibs/log"
	"golang.org/x/exp/slices"
)

const (
	twoskyConf = "../../.twosky.json"
	localesDir = "../../client/src/__locales"
	baseFile   = "en.json"
	projectID  = "home"
	srcDir     = "../../client/src"

	twoskyURI = "https://twosky.int.agrd.dev/api/v1"
)

// locale is a key-value pairs of translation.
type locale map[string]string

// languages is a key-value pairs of languages.
type languages map[string]string

func main() {
	if len(os.Args) == 1 {
		usage()
	}

	t := readTwosky()

	switch os.Args[1] {
	case "count":
		count(t.Languages)
	case "download":
		w := 1
		if len(os.Args) > 2 {
			i, err := strconv.Atoi(os.Args[2])
			if err != nil {
				err = errors.Annotate(err, "number of workers: %w")
				check(err)
			}

			if i > 1 {
				w = i
			}
		}

		download(t.Languages, w)
	case "unused":
		unused()
	case "upload":
		upload()
	default:
		usage()
	}
}

// check is a simple error-checking helper for scripts.
func check(err error) {
	if err != nil {
		panic(err)
	}
}

// usage prints usage.
func usage() {
	fmt.Println(`usage: go run main.go <command> [args]
Commands:
   count
        Print summary
   download [n]
        Download translations. n is number of workers.
   unused
        Print unused strings
   upload
        Upload translations`)

	os.Exit(1)
}

// readTwosky returns configuration.
func readTwosky() twosky {
	b, err := os.ReadFile(twoskyConf)
	check(err)

	var ts []twosky
	err = json.Unmarshal(b, &ts)
	err = errors.Annotate(err, "unmarshalling %s: %w", twoskyConf)
	check(err)

	if len(ts) == 0 {
		log.Fatalf("%s is empty", twoskyConf)
	}

	return ts[0]
}

// readLocale returns locale.
func readLocale(fn string) locale {
	b, err := os.ReadFile(fn)
	check(err)

	var l locale
	err = json.Unmarshal(b, &l)
	err = errors.Annotate(err, "unmarshalling %s: %w", fn)
	check(err)

	return l
}

// count prints summary for translations.
func count(lns languages) {
	basePath := filepath.Join(localesDir, baseFile)
	baseLocale := readLocale(basePath)

	sum := make(map[string]int)

	for ln := range lns {
		path := filepath.Join(localesDir, ln+".json")
		l := readLocale(path)

		if path == basePath {
			continue
		}

		sum[ln] = len(l) * 100 / len(baseLocale)
	}

	printSummary(sum)
}

// printSummary to stdout.
func printSummary(sum map[string]int) {
	keys := make([]string, 0, len(sum))
	for k := range sum {
		keys = append(keys, k)
	}

	slices.Sort(keys)
	for _, v := range keys {
		fmt.Printf("%s\t %d\n", v, sum[v])
	}
}

// download translations. w is number of workers.
func download(lns languages, w int) {
	uri := os.Getenv("TWOSKY_URI")
	if uri == "" {
		uri = twoskyURI
	}

	downloadURI, err := url.JoinPath(uri, "download")
	check(err)

	id := os.Getenv("TWOSKY_PROJECT_ID")
	if id == "" {
		id = projectID
	}

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
	var client http.Client

	for u := range urls {
		resp, err := client.Get(u)
		check(err)

		fmt.Println(u)

		if resp.StatusCode != http.StatusOK {
			log.Fatalf("status code is not ok: %s", http.StatusText(resp.StatusCode))
		}

		url, err := url.Parse(u)
		check(err)

		v := url.Query()
		locale := v.Get("language")

		buf, err := io.ReadAll(resp.Body)
		check(err)

		path := filepath.Join(localesDir, locale+".json")
		err = os.WriteFile(path, buf, 0o664)
		check(err)

		fmt.Println(path)

		err = resp.Body.Close()
		check(err)
	}
}

// unused prints unused strings.
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

	files := []string{}
	for _, n := range names {
		var buf []byte
		buf, err = os.ReadFile(n)
		check(err)

		files = append(files, string(buf))
	}

	printUnused(files)
}

// printUnused to stdout.
func printUnused(files []string) {
	basePath := filepath.Join(localesDir, baseFile)
	baseLocale := readLocale(basePath)

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
			if strings.Contains(f, k) {
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
func upload() {
	uri := os.Getenv("TWOSKY_URI")
	if uri == "" {
		uri = twoskyURI
	}

	uploadURI, err := url.JoinPath(uri, "upload")
	check(err)

	id := os.Getenv("TWOSKY_PROJECT_ID")
	if id == "" {
		id = projectID
	}

	v := url.Values{}
	v.Set("format", "json")
	v.Set("language", "en")
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

// twosky contains configuration.
type twosky struct {
	Languages        languages `json:"languages"`
	ProjectID        string    `json:"project_id"`
	BaseLocale       string    `json:"base_locale"`
	LocalizableFiles []string  `json:"localizable_files"`
}
